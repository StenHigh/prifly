package runtime

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/mattn/go-sqlite3"
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

func persistenceFailure(err error) bool {
	var sqlite sqlite3.Error
	var path *os.PathError
	return err != nil && (errors.As(err, &sqlite) || errors.As(err, &path) || errors.Is(err, local.ErrIntegrity) || errors.Is(err, local.ErrIncompatible))
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
			if detail := refusalDetail(base); detail != "" {
				p.Violations = []Violation{{"", detail}}
			}
			if strings.HasPrefix(code, "unsupported") {
				exit = 5
			} else if strings.Contains(code, "conflict") || strings.Contains(code, "drift") {
				exit = 3
			} else if strings.Contains(code, "recovery") {
				exit = 6
			}
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
	}[p.Code]; ok {
		p.SafeNextActions = actions
	}
	var occurrence *DiagnosticError
	if errors.As(err, &occurrence) && occurrence.ID != "" {
		p.CorrelationID = occurrence.ID
	}
	return p, exit
}
