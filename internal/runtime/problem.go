package runtime

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Violation names a place in the document the caller supplied and what is wrong
// there. It is not where a refusal's explanation lives: that is always
// `message`. Both were true at once until 0.13.8 — an authored refusal put its
// text here with an **empty** pointer while a rejection put the same kind of
// text in `message`, so a reader could not know where to look, and a dependent
// session's comparator silently lost the text of every rejection. The empty
// pointer beside a filled reason was the only outward sign that the text was in
// the wrong field.
//
// One case still fills it without a pointer, and it is not an explanation: text
// printed by a failing component, which is evidence and must not enter
// `message`. So an empty pointer now means "foreign bytes", and the reason for
// the refusal is in `message` either way.
type Violation struct {
	Pointer string `json:"pointer"`
	Reason  string `json:"reason"`
}

// Problem is the unchanged public baseline v1 error envelope.
type Problem struct {
	SchemaVersion   string      `json:"schema_version"`
	Code            string      `json:"code"`
	Message         string      `json:"message"`
	Retryable       bool        `json:"retryable"`
	CorrelationID   string      `json:"correlation_id"`
	Violations      []Violation `json:"violations"`
	SafeNextActions []string    `json:"safe_next_actions"`
}

// DiagnosticError links another presentation of a recorded occurrence; rendering
// a Problem does not create a new diagnostic or count traceback lines as errors.
type DiagnosticError struct {
	ID  string
	Err error
}

func (e *DiagnosticError) Error() string { return e.Err.Error() }
func (e *DiagnosticError) Unwrap() error { return e.Err }

// persistenceFailure asks the storage package whether this is its own failure:
// which error types mean that is storage's business, not the runtime's.
func persistenceFailure(err error) bool { return local.IsPersistenceFailure(err) }

// exitForCode maps a refusal to the exit status its class uses: an unsupported
// capability, a conflicting state, or an authority that was moved or restored
// and only allows inspection.
func exitForCode(code string) int {
	switch {
	// An exhausted allowance is the same refusal whether the engine wrote it as
	// a fault or the store rejected it: budget_exhausted exited 2 through here
	// and 5 through the rejection branch, so one code named two classes.
	case strings.HasPrefix(code, "unsupported") || strings.Contains(code, "exhausted") || strings.Contains(code, "busy"):
		return 5
	case strings.Contains(code, "conflict") || strings.Contains(code, "drift"):
		return 3
	case strings.Contains(code, "recovery"):
		return 6
	}
	return 2
}

// retryableRefusals names every refusal where the identical call, unchanged,
// may succeed later because the only obstacle is a resource somebody else is
// holding or a moment that has not arrived. It is deliberately short: until
// 0.13.10 the envelope reported `retryable: false` for all of them, including
// storage_busy, whose own message tells the reader to retry after the holder
// closes. A field that is always false is worse than no field, because a reader
// who believes it will not repeat an operation that would have succeeded.
//
// Two kinds are kept out on purpose. A refusal that asks the caller to do
// something first is not retryable (driver_active wants the driver stopped).
// Neither is one whose retry is not free: capacity_conflict creates and queues
// a Run, so a caller looping on it manufactures work rather than waiting for
// it. An allowance that does not refill by itself — the *_exhausted family —
// is not retryable either.
//
// New codes default to false, which is the safe direction: a reader may always
// retry a refusal marked false, but must not be told to retry one that will
// never clear.
var retryableRefusals = map[string]bool{
	// A lock another process holds, in each of the three places one is taken.
	"storage_busy": true,
	// The driver lock, held by another `run drive` that will release it.
	"driver_already_active": true,
	// Publisher request capacity, which frees as requests complete.
	"publisher_busy": true,
	// The Run is already queued and an older one holds the next free slot;
	// asking again costs nothing and creates nothing.
	"admission_deferred": true,
	// A moment that has not arrived yet by the authority's clock.
	"wait_not_due":         true,
	"deadline_not_reached": true,
}

func retryableCode(code string) bool { return retryableRefusals[code] }

func validProblemCode(s string) bool {
	if len(s) < 1 || len(s) > 64 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// leafError returns the error whose own text may be read. DiagnosticError only
// re-presents its cause, so it adds no text of its own and is transparent here.
func leafError(err error) error {
	var occurrence *DiagnosticError
	if errors.As(err, &occurrence) && occurrence.Err != nil {
		return leafError(occurrence.Err)
	}
	return err
}

// refusalDetail returns the engine-authored remainder of a `code: detail`
// refusal. It is kept only for an error with no wrapped cause: a wrapped cause
// contributes foreign text — a path, a driver message, a parser error — which
// this envelope never exposes.
func refusalDetail(err error) string {
	if errors.Unwrap(err) != nil {
		return ""
	}
	_, detail, found := strings.Cut(err.Error(), ":")
	if !found {
		return ""
	}
	return strings.TrimSpace(detail)
}

// ProblemFor does not expose raw parser input, executable argv, environment,
// worker stderr or arbitrary nested errors. A retry never authorizes a new effect.
func ProblemFor(err error) (Problem, int) {
	p := Problem{"1", "invalid_input", "The command could not be applied. Check its arguments and selected files.", false, newID("correlation"), []Violation{}, []string{"doctor", "run.status"}}
	exit := 2
	// A caller with no error has nothing to describe. Reading the code out of a
	// nil error crashed the process instead, so a test that stopped refusing
	// reported a segmentation fault rather than the refusal it expected.
	if err == nil {
		return p, exit
	}
	var fp *flow.Problem
	var rejected *local.Rejection
	var authored *Fault
	switch {
	case errors.Is(err, context.Canceled):
		p.Code, p.Message, exit = "interrupted", "The client was interrupted; inspect the recorded run before any retry.", 7
	case errors.Is(err, context.DeadlineExceeded):
		p.Code, p.Message, exit = "deadline_exceeded", "The command did not complete within its bound. Inspect its receipt before retrying.", 5
	case errors.As(err, &fp):
		p.Code, p.Message = fp.Code, fp.Message
		// A place, when there is one. Nine of the nineteen problems this
		// engine raises carry no path, and duplicating their message into a
		// place-less violation left exactly the shape the explanation was
		// moved out of: a filled reason beside an empty pointer.
		if fp.Path != "" {
			p.Violations = []Violation{{fp.Path, fp.Message}}
		}
		if strings.HasPrefix(fp.Code, "unsupported") {
			exit = 5
		}
	case errors.As(err, &rejected):
		p.Code, p.Message, exit = rejected.Code, rejected.Message, 3
		if strings.Contains(p.Code, "forbidden") {
			exit = 4
		} else if strings.Contains(p.Code, "exhausted") || strings.Contains(p.Code, "busy") || strings.HasPrefix(p.Code, "unsupported") {
			exit = 5
		}
	case errors.Is(err, local.ErrCommandConflict):
		p.Code, p.Message, exit = "command_conflict", "This command ID already identifies different input; inspect its receipt.", 3
	case errors.Is(err, local.ErrNotFound) || errors.Is(err, os.ErrNotExist):
		p.Code, p.Message = "not_found", "The explicitly selected run, definition, artifact or file was not found."
	case errors.Is(err, local.ErrRecoveryRequired):
		p.Code, p.Message, exit = "recovery_required", "This authority was moved or restored; only inspection is allowed.", 6
	case errors.Is(err, local.ErrIncompatible):
		p.Code, p.Message, exit = "unsupported_storage_version", "This storage or event version is not supported by this build.", 6
	case errors.Is(err, local.ErrIntegrity):
		p.Code, p.Message, exit = "integrity_failure", "Stored evidence did not pass integrity verification; stop new admissions and preserve the directory.", 6
	case errors.Is(err, local.ErrReadOnly) || errors.Is(err, os.ErrPermission):
		p.Code, p.Message, exit = "forbidden", "The selected operation is not permitted for this authority or file.", 4
	case errors.Is(err, local.ErrUnsafePath):
		p.Code, p.Message = "unsafe_path", "Use explicit regular files under separate roots, without symlinks or traversal."
	case errors.Is(err, local.ErrBlobLimit) || errors.Is(err, local.ErrSampleLimit):
		p.Code, p.Message, exit = "quota_exceeded", "A bounded local storage or payload allowance was exceeded.", 5
	// A busy store is a wait, not a lost write: another process held the
	// authority past the busy bound and this command was never tried. Read as
	// a persistence failure it told the operator not to assume the commit.
	case local.IsBusy(err):
		p.Code, p.Message, exit = "storage_busy", "Another process held the authority for longer than the busy bound; nothing was written. Retry.", 5
	case persistenceFailure(err):
		p.Code, p.Message, exit = "persistence_unavailable", "The authority could not persist or read mandatory evidence. Do not assume the operation committed.", 6
	// A refusal this engine authored keeps its own words even when it also
	// carries a cause. Reading the Fault directly exposes engine-authored text
	// only; the cause stays invisible either way.
	case errors.As(err, &authored) && authored.Message != "":
		p.Code, p.Message = authored.Code, authored.Message
		exit = exitForCode(authored.Code)
	default:
		// A refusal carries its stable code whether or not it also carries a
		// message: `code` alone and `code: detail` name the same refusal, and
		// collapsing the first into invalid_input loses the subject entirely.
		base := leafError(err)
		code, _, _ := strings.Cut(base.Error(), ":")
		if validProblemCode(code) {
			p.Code = code
			p.Message = "The selected operation was refused (" + code + "). Inspect status/doctor and the documented capability limits."
			// This branch is the only one whose detail did not come from the
			// engine: refusal-check forbids a refusal code inside error text
			// in non-test code, so a coded error reaching here carries
			// text from outside — an executor's own output, a traceback. That
			// is evidence, not an explanation, and it stays out of `message`,
			// which every other route now uses for the engine's own words.
			// Violations carries it with no pointer because there is no place
			// in a document to point at; a reader who finds one there is
			// looking at foreign bytes, never at the reason.
			if detail := refusalDetail(base); detail != "" {
				p.Message = "The selected operation was refused (" + code + "); violations carries what the failing component printed."
				p.Violations = []Violation{{"", detail}}
			}
			exit = exitForCode(code)
		}
	}
	if !validProblemCode(p.Code) {
		p.Code = "invalid_input"
	}
	p.Retryable = retryableCode(p.Code)
	if len(p.Message) > 2048 {
		p.Message = "The selected contract was rejected; inspect the reported pointer."
	}
	for i := range p.Violations {
		if len(p.Violations[i].Reason) > 2048 {
			p.Violations[i].Reason = "Contract validation failed."
		}
	}
	// Reading state is the safe move only when the object is missing inside a
	// working authority. A missing authority and an absent handoff are answered
	// somewhere else, and the default would send the reader in a circle.
	if actions, ok := map[string][]string{
		"authority_not_found": {"init", "doctor"},
		"no_active_handoff":   {"run.explain", "run.drive"},
		// A claim outlives the Run that took it and is only ended explicitly,
		// so a state diagnostic never shows the way out of one.
		"claim_conflict":       {"claim.list", "claim.release"},
		"claim_owner_unproven": {"claim.list", "claim.release"},
		"claim_owner_conflict": {"claim.list", "claim.release"},
		// Nothing is wrong with the state when the call itself was mistyped, so
		// a state diagnostic only leads away from the form that has to be fixed.
		"invalid_usage": {"help"},
		// A budget filled by superseded package editions is released by removing
		// one, and no state diagnostic reports on the registry that holds them.
		"dependency_limit": {"package.list", "package.remove"},
		// An unresolved execution is ended by the owner stating its outcome.
		// Nothing else moves it, and driving again returns this same refusal.
		"recovery_required": {"run.resolve", "run.status"},
		// A stop outlives the command that hit it and is lifted only by name,
		// so a state diagnostic never shows the way past one.
		"active_stop": {"run.status", "run.release"},
		// A driver lock is not the admission bound and no state diagnostic
		// reports on it: the refusal names the Run that holds it, and reading
		// that Run is the only thing that moves the caller forward.
		"driver_already_active": {"run.status", "run.events"},
		// An authority admits one attempt at a time until someone says
		// otherwise, so the Run that wants a second slot is not in a bad state
		// and no state diagnostic reports on the number that refused it.
		"capacity_conflict": {"capacity.show", "capacity.set"},
		// The expected version and the control epoch are read from run status,
		// and they sit at different levels of it: run_version at the top, the
		// epoch inside the run. A reader who takes both from one place fails
		// twice for one misunderstanding.
		"version_conflict":       {"run.status"},
		"state_version_conflict": {"run.status"},
	}[p.Code]; ok {
		p.SafeNextActions = actions
	}
	var occurrence *DiagnosticError
	if errors.As(err, &occurrence) && occurrence.ID != "" {
		p.CorrelationID = occurrence.ID
	}
	return p, exit
}
