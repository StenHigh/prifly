package runtime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// programExternalWrite shapes the driver fixture into a program that declares
// a bounded external write with the given retry class. The names are
// placeholders: the system, operations and target are the workflow author's,
// and this engine knows none of them.
func programExternalWrite(retryClass string) driverShape {
	defs, _, err := Builtins()
	if err != nil {
		panic(err)
	}
	return driverShape{
		Engine: func(e *Engine) {
			e.Config.Configuration.SemanticsProfile = flow.CoreProfile
			e.Config.Configuration.SchemaVersion = CoreConfigVersion
			e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
		},
		Step: func(step *flow.StepDefinition) {
			step.SchemaVersion = "14"
			step.Effects.Class, step.Effects.RetryClass = "external_write", retryClass
			step.ExternalWrite = &flow.ExternalWriteBoundary{System: "example-system", Operations: []string{"stack.up", "stack.down"}, Target: "example/target"}
		},
		Workflow: func(w *flow.WorkflowRevision) {
			w.PolicyRef = builtinVersionRef(defs, "core:policy/local", "4.0.0")
		},
	}
}

func driveUntilSettled(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	// A settlement error is the Run's business, recorded in it; the test reads
	// the Run, not the error the drive returned.
	_ = e.Drive(context.Background(), runID)
	return driverRun(t, e, runID)
}

func unreconciled(r Run) *Diagnostic {
	for index := range r.Diagnostics {
		if r.Diagnostics[index].Code == "external_write_unreconciled" {
			return &r.Diagnostics[index]
		}
	}
	return nil
}

// The program declares what it changes outside and the Run records it. Without
// contract 14 it had to say class none, and the journal said nothing changed
// while the program created and removed containers.
func TestAProgramDeclaresItsExternalWriteAndTheRunSealsIt(t *testing.T) {
	t.Parallel()
	e, runID := driverProject(t, "pass", 10000, programExternalWrite("idempotent"))
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreExternalWriteStateVersion {
		t.Fatalf("sealed at %s, not the state that records the boundary", r.SchemaVersion)
	}
	plan, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps["work"]
	if step.SchemaVersion != "14" || step.Effects.Class != "external_write" || step.ExternalWrite == nil || step.ExternalWrite.Target != "example/target" {
		t.Fatalf("the sealed plan lost the declaration: %+v", step)
	}
	if r = driveUntilSettled(t, e, runID); r.Status != "completed" {
		t.Fatalf("a program declaring an external write did not run: %s %+v", r.Status, r.Diagnostics)
	}
}

// Every retry class but idempotent says running the program again needs a
// person to know what the first run changed. A program that ended without an
// accepted result -- a failing exit, a timeout that stopped it -- leaves the
// Run waiting for that person, even though its process group is proven empty.
func TestAnInterruptedProgramWriteWaitsForTheOwnerUnlessIdempotent(t *testing.T) {
	t.Parallel()
	for _, interruption := range []struct {
		mode    string
		timeout int64
	}{{"nonzero", 10000}, {"wait", 2000}} {
		for _, retryClass := range []string{"deduplicated", "reconcile_required", "never"} {
			t.Run(interruption.mode+"/"+retryClass, func(t *testing.T) {
				// The wait deadline is long enough that the program has
				// started before it expires even on a loaded machine: one that
				// never started changed nothing, and the Run then fails
				// honestly instead of waiting for the owner.
				t.Parallel()
				e, runID := driverProject(t, interruption.mode, interruption.timeout, programExternalWrite(retryClass))
				r := driveUntilSettled(t, e, runID)
				if r.Status != "uncertain" || !r.HasUnresolvedEffects {
					t.Fatalf("an unreconciled external write settled %s without the owner", r.Status)
				}
				d := unreconciled(r)
				if d == nil {
					t.Fatalf("the stop is not named: %+v", r.Diagnostics)
				}
				for _, part := range []string{"work", retryClass, "run resolve", "applied", "not_applied"} {
					if !strings.Contains(d.Message, part) {
						t.Fatalf("the diagnostic does not name %q: %s", part, d.Message)
					}
				}
				view, err := e.View(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if view.Failure == nil || view.Failure.Code != "external_write_unreconciled" {
					t.Fatalf("run status does not say why the Run stopped: %+v", view.Failure)
				}
				next, err := e.Next(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if next.Action != "uncertain" || !slices.Contains(next.SafeNextActions, "run.resolve") {
					t.Fatalf("run next does not offer the one action that ends it: %+v", next)
				}
				plan, err := r.planFor(r.RootInvocationID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := recoveryPointOf(r, plan, plan, nil, ""); refusalCode(err) != "recover_source_unsettled" || !strings.Contains(err.Error(), "run resolve") {
					t.Fatalf("resuming an unresolved Run was not refused with the way out: %v", err)
				}
				_, loaded, err := e.load(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := e.ResolveObligation(context.Background(), runID, newID("command"), d.AttemptID, "", ResolveOutcomeNotApplied, "the external state was checked by hand", loaded.Snapshot.Version); err != nil {
					t.Fatalf("resolve: %v", err)
				}
				resolved := driverRun(t, e, runID)
				if resolved.Status != "failed" || resolved.HasUnresolvedEffects {
					t.Fatalf("the owner's attestation did not settle the Run: %s", resolved.Status)
				}
				if _, _, err := recoveryFrontier(resolved); err != nil {
					t.Fatalf("an attested Run is not resumable at its step: %v", err)
				}
			})
		}
	}
}

// idempotent is the author saying the program brings the external state to
// what it should be by itself. The interrupted program fails as any program
// does and is taken again without anyone attesting.
func TestAnInterruptedIdempotentProgramWriteFailsAndResumes(t *testing.T) {
	t.Parallel()
	e, runID := driverProject(t, "nonzero", 10000, programExternalWrite("idempotent"))
	r := driveUntilSettled(t, e, runID)
	if r.Status != "failed" || r.HasUnresolvedEffects || unreconciled(r) != nil {
		t.Fatalf("an idempotent program was held for the owner: %s %+v", r.Status, r.Diagnostics)
	}
	if _, _, err := recoveryFrontier(r); err != nil {
		t.Fatalf("a failed idempotent program is not resumable at its step: %v", err)
	}
}

// A result the authority accepted says what the program did, whatever its
// retry class: the verdict routes, nothing waits for the owner.
func TestAProgramWriteThatReportedIsSettledByItsVerdict(t *testing.T) {
	t.Parallel()
	e, runID := driverProject(t, "pass", 10000, programExternalWrite("reconcile_required"))
	if r := driveUntilSettled(t, e, runID); r.Status != "completed" || r.HasUnresolvedEffects {
		t.Fatalf("an accepted result was held for the owner: %s %+v", r.Status, r.Diagnostics)
	}
}

// A driver that died after the group was observed empty closes the attempt
// from saved evidence. The program did start, so what it changed outside is as
// unknown as after any other interruption.
func TestALostDriverLeavesAnUnreconciledProgramWriteForTheOwner(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		retryClass, status string
	}{{"reconcile_required", "uncertain"}, {"idempotent", "failed"}} {
		t.Run(test.retryClass, func(t *testing.T) {
			e, runID := driverProject(t, "pass", 10000, programExternalWrite(test.retryClass))
			a := driverAdmit(t, e, runID)
			driverDispatchFixture(t, e, runID, a.ID)
			identity := local.ProcessIdentity{PID: os.Getpid(), OwnerPID: os.Getpid()}
			for _, kind := range []string{"start_returned", "group_empty"} {
				if err := e.observe(context.Background(), runID, a.ID, local.ProcessObservation{Kind: kind, Identity: identity}); err != nil {
					t.Fatal(err)
				}
			}
			r := driveUntilSettled(t, e, runID)
			if r.Status != test.status || r.HasUnresolvedEffects != (test.status == "uncertain") {
				t.Fatalf("a lost driver settled %s, want %s: %+v", r.Status, test.status, r.Diagnostics)
			}
		})
	}
}

// The assisted line is not asked about its retry class: a lost session is not
// proof the session stopped, so idempotent does not make it certain.
func TestIdempotentDoesNotMakeALostAssistedSessionCertain(t *testing.T) {
	t.Parallel()
	e, runID, err := externalWriteFixture(t, func(s *flow.StepDefinition) { s.Effects.RetryClass = "idempotent" })
	if err != nil {
		t.Fatalf("an assisted idempotent external write did not start: %v", err)
	}
	task := handOver(t, e, runID)
	ctx := context.Background()
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.apply(ctx, e.owner, newID("command"), runID, "diagnostic.recorded", map[string]any{"expire": true}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		attempt := r.Attempts[task.AttemptID]
		attempt.Deadline = attempt.Admitted
		return local.Change{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.MarkSessionDisconnected(ctx, runID, task.AttemptID); err != nil {
		t.Fatal(err)
	}
	if r := driverRun(t, e, runID); r.Status != "uncertain" || !r.HasUnresolvedEffects {
		t.Fatalf("a lost idempotent session was settled without the owner: %s", r.Status)
	}
}

// The declared effect is a readable fact of the Run: the stage, its class, its
// retry class and the boundary, from the definition the Run sealed. A package
// changed afterwards does not change what the Run answers.
func TestARunNamesTheEffectEachStepDeclared(t *testing.T) {
	t.Parallel()
	e, runID := driverProject(t, "pass", 10000, programExternalWrite("idempotent"))
	r := driveUntilSettled(t, e, runID)
	// A later edition of the step on disk declares something else.
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), &registry)
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			var step flow.StepDefinition
			readRuntimeJSON(t, filepath.Join(e.Root, entry.Path), &step)
			step.Effects.Class, step.Effects.RetryClass, step.ExternalWrite = "none", "never", nil
			writeRuntimeJSON(t, filepath.Join(e.Root, entry.Path), step)
		}
	}
	// Through View, the read run status makes: it strips the sealed
	// definitions, and an answer computed after that would name nothing.
	view, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Run.Definitions) != 0 {
		t.Fatal("the view now dumps raw definitions; the effect list is no longer the only way to read them")
	}
	effects := view.StepEffects
	if len(effects) != 1 || len(r.Steps) != 1 {
		t.Fatalf("one step ran and %d effects were named: %+v", len(effects), effects)
	}
	effect := effects[0]
	if effect.StageID != "work" || effect.Class != "external_write" || effect.RetryClass != "idempotent" || effect.ExternalWrite == nil || effect.ExternalWrite.Target != "example/target" || !slices.Equal(effect.ExternalWrite.Operations, []string{"stack.up", "stack.down"}) {
		t.Fatalf("the Run does not name what its step declared: %+v", effect)
	}
	// The JSON a host reads: run next and run explain answer 42 with the same
	// list, and it validates against the published contract.
	next, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if next.SchemaVersion != CoreAwaitingHostNextVersion || len(next.StepEffects) != 1 || next.StepEffects[0].ExternalWrite == nil || next.StepEffects[0].ExternalWrite.System != "example-system" {
		t.Fatalf("run next does not name the declared boundary: %+v", next)
	}
	if err := validatePublic(t, "CoreNextViewV42", next); err != nil {
		t.Fatalf("next 42 rejects its own answer: %v", err)
	}
}
