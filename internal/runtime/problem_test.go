package runtime

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// A refusal names its subject. These cases pin what a client may read: the
// stable code with or without a message, the engine-authored detail of a leaf
// refusal, and nothing at all from free text or a wrapped foreign cause.
func TestProblemForKeepsStableCodeAndEngineDetail(t *testing.T) {
	for _, c := range []struct {
		name       string
		err        error
		code       string
		exit       int
		violations []Violation
	}{
		{"bare code", errors.New("workspace_tree_location_missing"), "workspace_tree_location_missing", 2, []Violation{}},
		{"code with detail", errors.New("output_required_missing: plan"), "output_required_missing", 2, []Violation{{"", "plan"}}},
		{"free text", errors.New("missing required output: plan"), "invalid_input", 2, []Violation{}},
		{"wrapped cause", fmt.Errorf("unsafe_archive: %w", errors.New("/private/tmp/secret: bad header")), "unsafe_archive", 2, []Violation{}},
		{"conflict exit", errors.New("workspace_tree_capture_conflict"), "workspace_tree_capture_conflict", 3, []Violation{}},
		{"unsupported exit", errors.New("unsupported_evidence: local output checks"), "unsupported_evidence", 5, []Violation{{"", "local output checks"}}},
	} {
		problem, exit := ProblemFor(c.err)
		if problem.Code != c.code || exit != c.exit {
			t.Fatalf("%s: got %s exit %d, want %s exit %d", c.name, problem.Code, exit, c.code, c.exit)
		}
		if len(problem.Violations) != len(c.violations) {
			t.Fatalf("%s: got violations %+v, want %+v", c.name, problem.Violations, c.violations)
		}
		for i, violation := range c.violations {
			if problem.Violations[i] != violation {
				t.Fatalf("%s: got violation %+v, want %+v", c.name, problem.Violations[i], violation)
			}
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
		if len(problem.Violations) != 1 || problem.Violations[0].Reason != c.message {
			t.Fatalf("%s: the authored detail did not reach the client: %+v", c.name, problem.Violations)
		}
		for _, foreign := range []string{"flock", "driver.lock", "resource temporarily unavailable"} {
			if strings.Contains(problem.Message+problem.Violations[0].Reason, foreign) {
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
	if len(problem.Violations) != 1 || problem.Violations[0].Reason != "no process was launched" {
		t.Fatalf("occurrence lost its detail: %+v", problem.Violations)
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
