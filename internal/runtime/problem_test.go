package runtime

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// A refusal names its subject, and it names it in one place. The explanation is
// always `message`; `violations` names places in the document the caller
// supplied and stays empty when there is no such place. Until 0.13.8 an
// engine-authored detail went to `violations` with an empty pointer while a
// rejection's went to `message`, so a reader could not know which to read.
func TestProblemForKeepsStableCodeAndEngineDetail(t *testing.T) {
	// `foreign` marks the one route whose detail did not come from the engine —
	// a `code: detail` error, which refusal-check forbids in non-test code, so
	// its text is a failing component's own output. That is evidence and stays
	// in violations; every explanation is in `message`.
	for _, c := range []struct {
		name    string
		err     error
		code    string
		exit    int
		detail  string
		foreign bool
	}{
		{"bare code", errors.New("workspace_tree_location_missing"), "workspace_tree_location_missing", 2, "", false},
		{"foreign detail", errors.New("output_required_missing: plan"), "output_required_missing", 2, "plan", true},
		{"free text", errors.New("missing required output: plan"), "invalid_input", 2, "", false},
		{"wrapped cause", fmt.Errorf("unsafe_archive: %w", errors.New("/private/tmp/secret: bad header")), "unsafe_archive", 2, "", false},
		{"conflict exit", errors.New("workspace_tree_capture_conflict"), "workspace_tree_capture_conflict", 3, "", false},
		{"foreign detail, unsupported exit", errors.New("unsupported_evidence: local output checks"), "unsupported_evidence", 5, "local output checks", true},
	} {
		problem, exit := ProblemFor(c.err)
		if problem.Code != c.code || exit != c.exit {
			t.Fatalf("%s: got %s exit %d, want %s exit %d", c.name, problem.Code, exit, c.code, c.exit)
		}
		if !c.foreign && len(problem.Violations) != 0 {
			t.Fatalf("%s: an explanation was put in violations, which names places: %+v", c.name, problem.Violations)
		}
		if c.foreign {
			if len(problem.Violations) != 1 || problem.Violations[0] != (Violation{"", c.detail}) {
				t.Fatalf("%s: foreign output did not stay in violations: %+v", c.name, problem.Violations)
			}
			if strings.Contains(problem.Message, c.detail) {
				t.Fatalf("%s: a failing component's own output reached the message: %q", c.name, problem.Message)
			}
		}
		if !strings.Contains(problem.Message, "refused") && !strings.Contains(problem.Message, "could not be applied") {
			t.Fatalf("%s: a refusal with no words of its own lost its sentence: %q", c.name, problem.Message)
		}
	}
}

// An engine-authored refusal that also carries a cause keeps its own words. The
// detail used to be recovered by splitting the error text, which any wrapped
// cause disqualified, so every wrapFault refusal reached the client as a bare
// code with the generic sentence — including the one that says the authority is
// open in another process. The cause's own text stays out either way.
func TestProblemForKeepsAuthoredDetailThroughAWrappedCause(t *testing.T) {
	cause := errors.New("flock /private/tmp/authority/.prifly/state/driver.lock: resource temporarily unavailable")
	for _, c := range []struct {
		name    string
		err     error
		code    string
		exit    int
		message string
	}{
		{"driver lock", wrapFault("driver_already_active", driverBusyMessage("run:abc"), cause), "driver_already_active", 2, driverBusyMessage("run:abc")},
		{"storage busy", wrapFault("storage_busy", "authority is open in another process; retry after it closes", cause), "storage_busy", 5, "authority is open in another process; retry after it closes"},
	} {
		problem, exit := ProblemFor(c.err)
		if problem.Code != c.code || exit != c.exit {
			t.Fatalf("%s: got %s exit %d, want %s exit %d", c.name, problem.Code, exit, c.code, c.exit)
		}
		if problem.Message != c.message {
			t.Fatalf("%s: the authored detail did not reach the client: %q", c.name, problem.Message)
		}
		if len(problem.Violations) != 0 {
			t.Fatalf("%s: an explanation was put in violations, which names places: %+v", c.name, problem.Violations)
		}
		for _, foreign := range []string{"flock", "driver.lock", "resource temporarily unavailable"} {
			if strings.Contains(problem.Message, foreign) {
				t.Fatalf("%s: the cause's own text leaked: %q", c.name, foreign)
			}
		}
	}
}

// A refusal with no words of its own is still that refusal, not invalid_input.
func TestProblemForKeepsTheCodeOfAWordlessWrappedFault(t *testing.T) {
	problem, exit := ProblemFor(wrapFault("pinned_workflow_unreadable", "", errors.New("open /nowhere: no such file")))
	if problem.Code != "pinned_workflow_unreadable" || exit != 2 || len(problem.Violations) != 0 {
		t.Fatalf("a wordless refusal lost its subject: %+v exit %d", problem, exit)
	}
}

// A recorded occurrence only re-presents its cause, so the refusal it carries
// keeps its code and detail while the correlation ID names the diagnostic.
func TestProblemForReadsThroughDiagnosticOccurrence(t *testing.T) {
	err := &DiagnosticError{ID: "diagnostic:abc", Err: errors.New("recovery_required: no process was launched")}
	problem, exit := ProblemFor(err)
	if problem.Code != "recovery_required" || exit != 6 {
		t.Fatalf("occurrence lost its refusal: %s exit %d", problem.Code, exit)
	}
	if problem.CorrelationID != "diagnostic:abc" {
		t.Fatalf("occurrence lost its correlation: %s", problem.CorrelationID)
	}
	// The occurrence's text is a failing component's, so it stays evidence.
	if len(problem.Violations) != 1 || problem.Violations[0].Reason != "no process was launched" {
		t.Fatalf("occurrence lost its detail: %+v", problem.Violations)
	}
	if strings.Contains(problem.Message, "no process was launched") {
		t.Fatalf("foreign output reached the message: %q", problem.Message)
	}
}

// Typed refusals keep their existing meaning: a code-shaped sentinel branch is
// still chosen before the default branch reads any text.
func TestProblemForPrefersTypedRefusals(t *testing.T) {
	problem, exit := ProblemFor(fmt.Errorf("read state: %w", os.ErrNotExist))
	if problem.Code != "not_found" || exit != 2 || len(problem.Violations) != 0 {
		t.Fatalf("a typed refusal changed shape: %+v exit %d", problem, exit)
	}
}

// refusalCode is what a caller of the engine sees: the stable code the refusal
// is reported under, whatever error value carried it.
func refusalCode(err error) string {
	problem, _ := ProblemFor(err)
	return problem.Code
}

// Reading a refusal out of nothing used to crash: a caller that stopped
// refusing got a segmentation fault instead of an answer, which hid the real
// failure behind a panic in the reporting path.
func TestProblemForNoErrorDoesNotCrash(t *testing.T) {
	problem, exit := ProblemFor(nil)
	if problem.Code != "invalid_input" || exit != 2 || problem.CorrelationID == "" {
		t.Fatalf("a missing error did not produce the default problem: %+v %d", problem, exit)
	}
}

// A place, when there is one. Nine of the nineteen problems this engine raises
// carry no path, and duplicating their message into a place-less violation left
// exactly the shape the explanation was moved out of — a filled reason beside an
// empty pointer. Both cases are pinned because the fix has two ways to be wrong:
// leaving the duplicate, or dropping a real pointer with it.
func TestFlowProblemFillsViolationsOnlyWhereThereIsAPlace(t *testing.T) {
	placed, _ := ProblemFor(&flow.Problem{Code: "missing_stage", Path: "/definition", Message: "transition target does not exist"})
	if len(placed.Violations) != 1 || placed.Violations[0] != (Violation{"/definition", "transition target does not exist"}) {
		t.Fatalf("a refusal that names a place lost it: %+v", placed.Violations)
	}
	if placed.Message != "transition target does not exist" {
		t.Fatalf("a refusal that names a place lost its explanation: %q", placed.Message)
	}
	placeless, _ := ProblemFor(&flow.Problem{Code: "schema_invalid", Message: "value does not satisfy the declared contract"})
	if len(placeless.Violations) != 0 {
		t.Fatalf("an explanation was duplicated into a place-less violation: %+v", placeless.Violations)
	}
	if placeless.Message != "value does not satisfy the declared contract" {
		t.Fatalf("the explanation did not stay in the message: %q", placeless.Message)
	}
}
