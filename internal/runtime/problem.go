package runtime

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

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
	switch {
	case errors.Is(err, context.Canceled):
		p.Code, p.Message, exit = "interrupted", "The client was interrupted; inspect the recorded run before any retry.", 7
	case errors.Is(err, context.DeadlineExceeded):
		p.Code, p.Message, exit = "deadline_exceeded", "The command did not complete within its bound. Inspect its receipt before retrying.", 5
	case errors.As(err, &fp):
		p.Code, p.Message = fp.Code, fp.Message
		p.Violations = []Violation{{fp.Path, fp.Message}}
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
	case persistenceFailure(err):
		p.Code, p.Message, exit = "persistence_unavailable", "The authority could not persist or read mandatory evidence. Do not assume the operation committed.", 6
	default:
		// A refusal carries its stable code whether or not it also carries a
		// message: `code` alone and `code: detail` name the same refusal, and
		// collapsing the first into invalid_input loses the subject entirely.
		base := leafError(err)
		code, _, _ := strings.Cut(base.Error(), ":")
		if validProblemCode(code) {
			p.Code = code
			p.Message = "The selected operation was refused (" + code + "). Inspect status/doctor and the documented capability limits."
			// The detail stays out of the message: a code raised from an
			// executor's own output carries that output with it, and this
			// envelope never puts foreign bytes there. But sending the reader
			// to a state diagnostic while the answer sits in violations is the
			// wrong direction, so the message says where it is.
			if detail := refusalDetail(base); detail != "" {
				p.Message = "The selected operation was refused (" + code + "); violations names the exact subject and what to change."
				p.Violations = []Violation{{"", detail}}
			}
			exit = exitForCode(code)
		}
	}
	if !validProblemCode(p.Code) {
		p.Code = "invalid_input"
	}
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
