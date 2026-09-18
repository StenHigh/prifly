package runtime

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// environmentSourceValue is deliberately awkward: an equals sign, a hash and a
// quote are exactly what a reader that guesses at a grammar gets wrong.
const environmentSourceValue = `p@ss=word#1"'`

func environmentSourceFile(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnvironmentSourceReadsExactlyTheDeclaredPlace(t *testing.T) {
	dotenv := environmentSourceFile(t, "env", "# PASSWORD=commented-out\nOTHER=ignored\nPASSWORD="+environmentSourceValue+"\nPASSWORD=a later line never wins\n")
	whole := environmentSourceFile(t, "token", environmentSourceValue+"\n")
	t.Setenv("PRIFLY_TEST_SOURCE", environmentSourceValue)
	for _, test := range []struct {
		name   string
		source EnvironmentSource
		want   string
	}{
		{"env", EnvironmentSource{Env: "PRIFLY_TEST_SOURCE"}, environmentSourceValue},
		{"file", EnvironmentSource{File: whole}, environmentSourceValue},
		{"dotenv", EnvironmentSource{DotEnv: dotenv, Key: "PASSWORD"}, environmentSourceValue},
		{"dotenv-other-key", EnvironmentSource{DotEnv: dotenv, Key: "OTHER"}, "ignored"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := resolveEnvironmentSources(ExecutorConfig{EnvironmentFrom: map[string]EnvironmentSource{"SECRET": test.source}})
			if err != nil || len(resolved) != 1 || resolved["SECRET"] != test.want {
				t.Fatalf("read %#v: %#v %v", test.source, resolved, err)
			}
		})
	}
	for _, test := range []struct {
		name   string
		source EnvironmentSource
		place  string
	}{
		{"unset-variable", EnvironmentSource{Env: "PRIFLY_TEST_SOURCE_ABSENT"}, "PRIFLY_TEST_SOURCE_ABSENT"},
		{"missing-file", EnvironmentSource{File: filepath.Join(t.TempDir(), "absent")}, "absent"},
		{"empty-file", EnvironmentSource{File: environmentSourceFile(t, "empty", "")}, "empty"},
		{"missing-key", EnvironmentSource{DotEnv: dotenv, Key: "ABSENT"}, "ABSENT"},
		{"commented-key", EnvironmentSource{DotEnv: environmentSourceFile(t, "env", "# PASSWORD=commented-out\n"), Key: "PASSWORD"}, "PASSWORD"},
		{"quoted-value", EnvironmentSource{DotEnv: environmentSourceFile(t, "env", `PASSWORD="`+environmentSourceValue+`"`+"\n"), Key: "PASSWORD"}, "quoted"},
		{"oversized-file", EnvironmentSource{File: environmentSourceFile(t, "big", strings.Repeat("x", maxEnvironmentSourceBytes+1))}, "larger"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := resolveEnvironmentSources(ExecutorConfig{EnvironmentFrom: map[string]EnvironmentSource{"SECRET": test.source}})
			if err == nil || resolved != nil {
				t.Fatalf("unusable source admitted: %#v", resolved)
			}
			var fault *Fault
			if !errors.As(err, &fault) || fault.Code != "execution_environment_unavailable" {
				t.Fatalf("refusal is not named: %v", err)
			}
			if !strings.Contains(fault.Message, "SECRET") || !strings.Contains(fault.Message, test.place) {
				t.Fatalf("refusal names neither the variable nor the place: %s", fault.Message)
			}
			if strings.Contains(fault.Message, environmentSourceValue) {
				t.Fatalf("refusal printed the value: %s", fault.Message)
			}
		})
	}
}

// A declared source reaches the program that was started and nothing else: the
// value is read at dispatch, so no state, artifact or document written by this
// Run can hold it.
func TestDriverEnvironmentSourceReachesTheProgramWithoutEnteringState(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	dotenv := environmentSourceFile(t, "env", "PASSWORD="+environmentSourceValue+"\n")
	environmentSourceExecutor(t, e, map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {DotEnv: dotenv, Key: "PASSWORD"}})
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || r.SchemaVersion != CoreEnvironmentSourceStateVersion {
		t.Fatalf("declared source did not run under its own state: %s %s %+v", r.Status, r.SchemaVersion, r.Diagnostics)
	}
	workspace := ""
	for _, a := range r.Attempts {
		workspace = a.Workspace
	}
	sourced, err := os.ReadFile(filepath.Join(workspace, "worker-sourced"))
	if err != nil || string(sourced) != environmentSourceValue {
		t.Fatalf("the program was not handed the declared value: %q %v", sourced, err)
	}
	declared := 0
	for _, pinned := range r.Executors {
		if pinned.Config.EnvironmentFrom["DRIVER_TEST_SOURCED"].DotEnv == dotenv {
			declared++
		}
	}
	if declared != 1 {
		t.Fatal("the sealed binding lost the declaration it was started with")
	}
	// Everything this authority wrote, not only the Run state: an envelope, an
	// artifact or a projection would each be a copy nobody asked for.
	if err := filepath.WalkDir(e.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() || path == filepath.Join(workspace, "worker-sourced") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), environmentSourceValue) {
			rel, _ := filepath.Rel(e.Root, path)
			t.Fatalf("the value was written to %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDriverMissingEnvironmentSourceRefusesBeforeStart(t *testing.T) {
	for _, test := range []struct {
		name   string
		source EnvironmentSource
	}{
		{"env", EnvironmentSource{Env: "PRIFLY_TEST_SOURCE_ABSENT"}},
		{"file", EnvironmentSource{File: filepath.Join(t.TempDir(), "absent")}},
		{"dotenv", EnvironmentSource{DotEnv: environmentSourceFile(t, "env", "OTHER=ignored\n"), Key: "PASSWORD"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, options := contextDriverProject(t, nil)
			environmentSourceExecutor(t, e, map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": test.source})
			started, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			runID := started.Receipt.RunID
			err = e.Drive(context.Background(), runID)
			if err == nil || !strings.Contains(err.Error(), "DRIVER_TEST_SOURCED") {
				t.Fatalf("the caller was not told which variable was missing: %v", err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "failed" || len(r.Active) != 0 || len(r.Attempts) != 1 {
				t.Fatalf("a missing source left the Run running: %s %+v", r.Status, r.Diagnostics)
			}
			for _, a := range r.Attempts {
				if a.Dispatch != nil || a.Started != nil || a.Settled == nil || a.ProcessOutcome == nil || a.ProcessOutcome.Started {
					t.Fatalf("a missing source started or lost the program: %+v", a)
				}
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); !os.IsNotExist(err) {
					t.Fatalf("the program ran anyway: %v", err)
				}
			}
			if r.Diagnostics[0].Code != "execution_environment_unavailable" {
				t.Fatalf("refusal is not named in the Run: %+v", r.Diagnostics)
			}
		})
	}
}

// The state ladder is cumulative, so the declaration cannot be sealed into a
// flat Run: it is refused at Start rather than written under a version whose
// published contract has no place for it.
func TestStartRefusesADeclaredSourceOutsideTheScopedState(t *testing.T) {
	e, _ := driverProject(t, "pass", 5000)
	config := e.Config.Configuration.Executors["test:step/driver"]
	config.EnvironmentFrom = map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {Env: "PRIFLY_TEST_SOURCE"}}
	e.Config.Configuration.Executors["test:step/driver"] = config
	_, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported_environment_source") {
		t.Fatalf("a flat Run sealed a declaration its contract cannot carry: %v", err)
	}
}

// environmentSourceExecutor declares the sources on the one program the context
// fixture runs, before the Run pins it.
func environmentSourceExecutor(t *testing.T, e *Engine, sources map[string]EnvironmentSource) {
	t.Helper()
	config, exists := e.Config.Configuration.Executors["test:step/context"]
	if !exists {
		t.Fatal("fixture must bind the context step to a program")
	}
	config.EnvironmentFrom = sources
	e.Config.Configuration.Executors["test:step/context"] = config
}

// The owner asks what a program will be given before the call that runs it.
// The answer is read from what this Run sealed, so a setting changed after the
// start is visibly not part of it — which is how a cold start lost an evening
// to a password its program could never have received.
func TestNextAnswersWhatTheProgramWouldBeGiven(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	dotenv := environmentSourceFile(t, "env", "PASSWORD="+environmentSourceValue+"\n")
	environmentSourceExecutor(t, e, map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {DotEnv: dotenv, Key: "PASSWORD"}})
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(context.Background(), started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if next.SchemaVersion != CoreProgramEnvironmentNextVersion || next.Action != "stage" || next.StageWork != StageWorkProgram {
		t.Fatalf("the ready program stage was not named: %+v", next)
	}
	if next.ProgramEnvironment == nil {
		t.Fatal("the answer does not say what the program would be given")
	}
	if !slices.Contains(next.ProgramEnvironment.Names, "DRIVER_TEST_SOURCED") || !slices.IsSorted(next.ProgramEnvironment.Names) {
		t.Fatalf("the declared name is missing or unordered: %+v", next.ProgramEnvironment.Names)
	}
	if next.ProgramEnvironment.Sources["DRIVER_TEST_SOURCED"] != "dotenv:"+dotenv+":PASSWORD" {
		t.Fatalf("the place the value comes from is not named: %+v", next.ProgramEnvironment.Sources)
	}
	// A literal of this machine is named, never printed, and the value of a
	// source never appears at all.
	literal := ""
	for name := range e.Config.Configuration.Executors["test:step/context"].Environment {
		literal = name
	}
	if literal == "" || !slices.Contains(next.ProgramEnvironment.Names, literal) {
		t.Fatalf("a literal variable of this machine is not named: %+v", next.ProgramEnvironment)
	}
	encoded, err := canonical(next)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), environmentSourceValue) || strings.Contains(string(encoded), e.Config.Configuration.Executors["test:step/context"].Environment[literal]) {
		t.Fatalf("the answer printed a value: %s", encoded)
	}
	// The answer costs the Run nothing: its state is the one it was sealed at,
	// and 33 mints no state version of its own.
	r := driverRun(t, e, started.Receipt.RunID)
	if r.SchemaVersion != CoreEnvironmentSourceStateVersion {
		t.Fatalf("answering what the program gets moved the Run's own state: %s", r.SchemaVersion)
	}
}

// "inspect recorded evidence" named nothing a reader could open: a cold start
// recovered the cause of a failed program only because the program happened to
// write its own files. The refusal now names what the engine did record and
// what it does not keep.
func TestNonzeroExitNamesTheCodeAndTheBoundaryOfWhatIsKept(t *testing.T) {
	e, runID := driverProject(t, "nonzero", 30000)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "nonzero_exit" {
		t.Fatalf("the failure is not the one this test arranges: %+v", r.Diagnostics)
	}
	message := r.Diagnostics[0].Message
	var exit string
	for _, a := range r.Attempts {
		if a.ProcessOutcome != nil && a.ProcessOutcome.ExitCode != nil {
			exit = strconv.Itoa(*a.ProcessOutcome.ExitCode)
		}
	}
	if exit == "" || exit == "0" {
		t.Fatalf("the fixture did not record a nonzero exit: %+v", r.Attempts)
	}
	if !strings.Contains(message, "exited "+exit) {
		t.Fatalf("the refusal does not name the exit code %s: %s", exit, message)
	}
	if !strings.Contains(message, "not its own output") {
		t.Fatalf("the refusal does not say what is not kept: %s", message)
	}
}
