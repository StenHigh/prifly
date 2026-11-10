package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLISchemaMigrationPublicPrepareApplyInspectAndExplicitStart(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	invoke := func(want int, args ...string) []byte {
		t.Helper()
		var out, errout bytes.Buffer
		argv := append([]string{"--project", project}, args...)
		argv = append(argv, "--json")
		code := execute(context.Background(), argv, &out, &errout)
		if code != want {
			t.Fatalf("%v: exit %d, want %d: %s", args, code, want, errout.String())
		}
		if want != 0 {
			return errout.Bytes()
		}
		return out.Bytes()
	}
	invoke(0, "init", "--profile", flow.CoreProfile, project)
	writeJSON := func(path string, value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return data
	}
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	files := map[string][]byte{"a.json": []byte(`{"type":"object"}`), "b.json": []byte(`{"additionalProperties":true,"type":"object"}`)}
	schemas := []flow.Ref{{ID: "test:schema/cli-a", Version: "1.0.0", Digest: digest(files["a.json"])}, {ID: "test:schema/cli-b", Version: "1.0.0", Digest: digest(files["b.json"])}}
	components := []map[string]any{{"kind": "schema", "ref": schemas[0], "path": "a.json"}, {"kind": "schema", "ref": schemas[1], "path": "b.json"}}
	importPackage := func(id string, payload map[string][]byte, items []map[string]any, deps []flow.Ref) flow.Ref {
		t.Helper()
		source := t.TempDir()
		declared := []map[string]any{}
		for name, data := range payload {
			if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
				t.Fatal(err)
			}
			declared = append(declared, map[string]any{"path": name, "digest": digest(data), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
		}
		manifest := writeJSON(filepath.Join(source, prifly.PackageManifestFile), map[string]any{"schema_version": "1", "id": id, "version": "1.0.0", "description": "Public CLI migration fixture", "requires_core_protocol": "1", "dependencies": deps, "components": items, "files": declared, "requested_capabilities": []string{}, "license": "MIT"})
		invoke(0, "package", "import", "--dir", source, "--reason", "reviewed fixture", "--command-id", "command:import-"+id)
		return flow.Ref{ID: id, Version: "1.0.0", Digest: digest(manifest)}
	}
	legacy := importPackage("test:package/cli-legacy", files, components, []flow.Ref{})
	workflow := emptyCLIWorkflow(t)
	workflow.Inputs = map[string]flow.InputPort{"a": {Port: flow.Port{Format: "json", SchemaRef: &schemas[0]}, Required: true}, "b": {Port: flow.Port{Format: "json", SchemaRef: &schemas[1]}, Required: true}}
	workflowBytes := writeJSON(filepath.Join(project, "workflow.json"), workflow)
	writeJSON(filepath.Join(project, "value.json"), map[string]any{})
	writeJSON(filepath.Join(project, "brief.json"), prifly.Brief{SchemaVersion: "1", ID: "test:brief/migration", Subject: "Shared schemas", DesiredOutcome: "No work", InScope: []string{"Read shared schema inputs"}, OutOfScope: []string{}, CompletionCriteria: []string{"Read inputs"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"})
	start := []string{"run", "start", "--workflow", "workflow.json", "--brief", "brief.json", "--input", "a=value.json", "--input", "b=value.json"}
	invoke(0, append(append([]string{}, start...), "--command-id", "command:old")...)
	invoke(0, "package", "revoke", "--id", legacy.ID, "--version", legacy.Version, "--reason", "mistaken upgrade retirement")
	supplier := importPackage("test:package/cli-supplier", files, components, []flow.Ref{})
	wfDigest, err := flow.Digest(workflowBytes)
	if err != nil {
		t.Fatal(err)
	}
	wfRef := flow.Ref{ID: workflow.ID, Version: workflow.Version, Digest: wfDigest}
	consumer := importPackage("test:package/cli-consumer", map[string][]byte{"workflow.json": workflowBytes}, []map[string]any{{"kind": "workflow", "ref": wfRef, "path": "workflow.json"}}, []flow.Ref{supplier})
	problem := invoke(3, append(append([]string{}, start...), "--command-id", "command:blocked")...)
	if !bytes.Contains(problem, []byte(`"code":"package_not_resolvable"`)) {
		t.Fatalf("original refusal: %s", problem)
	}
	request := prifly.SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: consumer, Selections: []prifly.SchemaMigrationSelection{}}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	writeJSON(requestPath, request)
	var plan prifly.SchemaMigrationPlan
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "prepare", "--request", requestPath), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || len(plan.Schemas) != 2 || len(plan.Runs) != 1 {
		t.Fatalf("public diagnostic: %+v", plan)
	}
	for _, ref := range schemas {
		request.Selections = append(request.Selections, prifly.SchemaMigrationSelection{SchemaRef: ref, SupplierRef: supplier})
	}
	writeJSON(requestPath, request)
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "prepare", "--request", requestPath), &plan); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeJSON(planPath, plan)
	var result prifly.SchemaMigrationResult
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "apply", "--plan", planPath, "--expected-review-digest", plan.ReviewDigest, "--reason", "owner verified shared schemas", "--command-id", "command:cli-migrate"), &result); err != nil {
		t.Fatal(err)
	}
	var inspection prifly.SchemaMigrationInspection
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "inspect", "--id", result.Decision.ID), &inspection); err != nil || !inspection.Valid {
		t.Fatal("public inspect", err, inspection)
	}
	response := invoke(0, append(append([]string{}, start...), "--command-id", "command:migrated", "--schema-migration", result.Decision.ID, "--drive")...)
	if !bytes.Contains(response, []byte("core-read/45")) {
		t.Fatalf("public migration boundary missing: %s", response)
	}
	invoke(3, append(append([]string{}, start...), "--command-id", "command:still-blocked")...)
	// A lost-response retry reads its saved receipt before current payload bytes.
	engine, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := engine.Packages(context.Background())
	engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages.Packages {
		if pkg.Ref == supplier {
			path := filepath.Join(project, pkg.Components[0].Path)
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"type":"null"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	repeated := invoke(0, "package", "schema-migration", "apply", "--plan", planPath, "--expected-review-digest", plan.ReviewDigest, "--reason", "owner verified shared schemas", "--command-id", "command:cli-migrate")
	var replay prifly.SchemaMigrationResult
	if err := json.Unmarshal(repeated, &replay); err != nil || !replay.Duplicate || replay.Receipt.Cut != result.Receipt.Cut {
		t.Fatal("public replay read changed bytes before receipt", err)
	}

}

func projectMigrationFixture(t *testing.T, naming bool, features ...string) (string, string, []string, projectCompileResult, []string) {
	t.Helper()
	var root, authority string
	packageName := "single"
	var launchArgs []string
	if naming {
		root, authority, launchArgs, _ = ownedNamingFixture(t)
		packageName = "inspect"
	} else {
		root, authority = singleLaunchFixture(t)
		launchArgs = []string{"--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task=" + filepath.Join(root, "task.json")}
	}
	for _, feature := range features {
		if feature == "build-variants" {
			workflowPath := filepath.Join(root, ".prifly/workflows/single/workflow.yaml")
			workflow, err := os.ReadFile(workflowPath)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/single/workflow.yaml", strings.Replace(string(workflow), "  references:", "  profiles:\n    default: a\n    values: {a: {work_title: First}, b: {work_title: Second}}\n  references:", 1))
			stepPath := filepath.Join(root, ".prifly/workflows/single/steps/work.yaml")
			step, err := os.ReadFile(stepPath)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/single/steps/work.yaml", strings.Replace(string(step), "kind: worker", "title: \"{{work_title}}\"\nkind: worker", 1))
			writeFixtureFile(t, root, ".prifly/workflows/single/contexts/work.yaml", "id: example-single:context/work\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\nsource: {root: host_skills, path: choice/SKILL.md}\n")
			writeFixtureFile(t, root, ".codex/skills/choice/SKILL.md", "CLI selected work.\n")
			writeFixtureFile(t, root, ".agents/skills/choice/SKILL.md", "App selected work.\n")
		}
		if feature == "compile-values" {
			writeFixtureFile(t, root, ".prifly/workflows/single/compile-parameters.yaml", strings.ReplaceAll(compileParameterDeclaration, "schema_task", "schema_object"))
			writeFixtureFile(t, root, ".prifly/workflows/single/schemas/object.yaml", "id: example-single:schema/object\nversion: 1.0.0\ntype: object\nproperties:\n  description: {type: string, maxLength: \"{{task_description_limit}}\"}\n")
		}
	}
	output := filepath.Join(t.TempDir(), "compiled")
	raw := queueCLICommand(t, "--project", authority, "project", "compile", "--repository", root, "--package", packageName, "--host", "codex-cli", "--output", output)
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(raw), &compiled); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	files := []map[string]any{}
	components := []map[string]any{}
	schemas := []flow.Ref{}
	for _, component := range compiled.Components {
		if component.Kind == "schema" {
			data, err := os.ReadFile(filepath.Join(output, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, source, component.Path, string(data))
			files = append(files, map[string]any{"path": component.Path, "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
			components = append(components, map[string]any{"kind": "schema", "ref": component.Ref, "path": component.Path})
			schemas = append(schemas, component.Ref)
		}
	}
	if len(schemas) == 0 {
		t.Fatal("Project fixture has no shared schema")
	}
	manifest, err := json.Marshal(map[string]any{"schema_version": "1", "id": "test:package/project-legacy-schemas", "version": "1.0.0", "description": "Legacy Project schemas", "requires_core_protocol": "1", "dependencies": []flow.Ref{}, "components": components, "files": files, "requested_capabilities": []string{}, "license": "MIT"})
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, source, prifly.PackageManifestFile, string(manifest))
	queueCLICommand(t, "--project", authority, "package", "import", "--dir", source, "--reason", "reviewed legacy schemas")
	queueCLICommand(t, "--project", authority, "package", "revoke", "--id", "test:package/project-legacy-schemas", "--version", "1.0.0", "--reason", "mistaken retirement")
	queueCLICommand(t, "--project", authority, "package", "import", "--dir", output, "--reason", "reviewed exact consumer")
	engine, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	request := prifly.SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: compiled.Package, Selections: []prifly.SchemaMigrationSelection{}}
	for _, schema := range schemas {
		request.Selections = append(request.Selections, prifly.SchemaMigrationSelection{SchemaRef: schema, SupplierRef: compiled.Package})
	}
	decisions := []string{}
	for _, command := range []string{"command:project-migration-one", "command:project-migration-two"} {
		plan, err := engine.PrepareSchemaMigration(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.ApplySchemaMigration(context.Background(), prifly.SchemaMigrationApplyRequest{CommandID: command, Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "verified exact shared schema"})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatal(err, result.Receipt.Rejection)
		}
		decisions = append(decisions, result.Decision.ID)
	}
	engine.Close()
	return root, authority, launchArgs, compiled, decisions
}

func TestCLIProjectSchemaMigrationIsExplicitReviewedAndSerializedAtTen(t *testing.T) {
	t.Parallel()
	for _, naming := range []bool{false, true} {
		t.Run(fmt.Sprint("owned-naming=", naming), func(t *testing.T) {
			t.Parallel()
			_, authority, launchArgs, _, decisions := projectMigrationFixture(t, naming)
			base := append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, launchArgs...)
			base = append(base, "--schema-migration", decisions[0])
			review := queueCLICommand(t, base...)
			var summary projectLaunchSummary
			if err := json.Unmarshal([]byte(review), &summary); err != nil {
				t.Fatal(err)
			}
			if summary.SchemaVersion != "project-launch-summary/10" || summary.SchemaMigrationID != decisions[0] {
				t.Fatalf("selected decision missing from review: %s", review)
			}
			validateQueueCLI(t, "ProjectLaunchSummaryV10", review)
			start := append([]string{"--project", authority, "project", "start"}, launchArgs...)
			start = append(start, "--expected-launch-digest", summary.ReviewDigest)
			code, _, stderr := runCLI(t, append(append([]string{}, start...), "--schema-migration", decisions[1])...)
			if code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
				t.Fatalf("changed decision escaped review digest: %d %s", code, stderr)
			}
			response := queueCLICommand(t, append(start, "--schema-migration", decisions[0])...)
			var started projectStartResult
			if err := json.Unmarshal([]byte(response), &started); err != nil {
				t.Fatal(err)
			}
			if started.SchemaVersion != "project-start/10" || started.SchemaMigrationID != decisions[0] || started.Run.Run.SchemaMigration == nil {
				t.Fatalf("Project Start lost selected decision: %s", response)
			}
			validateQueueCLI(t, "ProjectStartV10", response)
			if naming && (started.Workspace == nil || started.Workspace.NamingSnapshot == nil || started.LaunchSummary.WorktreeNaming == nil) {
				t.Fatal("migration lost owned naming snapshot")
			}

		})
	}
}

func TestCLIProjectSchemaMigrationRejectsLegacyProfileBeforeEffects(t *testing.T) {
	t.Parallel()
	root, authority := singleLaunchFixture(t)
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "prifly-project-profile/3", "prifly-project-profile/2", 1))
	e, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	_, cut, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := e.Claims(context.Background())
	e.Close()
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, "brief.json", `{"schema_version":"1","id":"test:brief/legacy-migration","subject":"Legacy migration refusal","desired_outcome":"Refuse before launch","in_scope":["profile boundary"],"out_of_scope":[],"completion_criteria":["no authority changes"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
	code, out, stderr := runCLI(t, "--project", authority, "project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--brief", filepath.Join(root, "brief.json"), "--input", "task="+filepath.Join(root, "task.json"), "--schema-migration", "schema-migration:explicit-selection")
	var problem prifly.Problem
	if code != 2 || out != "" || json.Unmarshal([]byte(stderr), &problem) != nil || problem.Code != "project_questionnaire_prepare_requires_profile_3" {
		t.Fatalf("legacy migration did not refuse before launch: exit=%d out=%s stderr=%s", code, out, stderr)
	}
	e, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_, afterCut, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	afterClaims, err := e.Claims(context.Background())
	claimsJSON, _ := json.Marshal(claims)
	afterClaimsJSON, _ := json.Marshal(afterClaims)
	if err != nil || cut != afterCut || !bytes.Equal(claimsJSON, afterClaimsJSON) {
		t.Fatal("legacy refusal changed authority or claims", err)
	}
	t.Log("[FIX:21] legacy profile refusal leaves authority cut and claims unchanged")
}

func TestCLIProjectSavedSchemaMigrationRepeatsWithoutDecisionID(t *testing.T) {
	t.Parallel()
	root, authority, launchArgs, compiled, decisions := projectMigrationFixture(t, false)
	queueCLICommand(t, "--project", authority, "project", "schema-migration", "set", "--repository", root, "--launch", "single", "--host", "codex-cli", "--id", decisions[0], "--expected-build-key", compiled.BuildKey, "--expected-generation", "0", "--reason", "reviewed launch choice", "--command-id", "command:save-launch-migration")
	for i := range 2 {
		writeFixtureFile(t, root, "task.json", fmt.Sprintf(`{"round":%d}`, i))
		review := queueCLICommand(t, append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, launchArgs...)...)
		var summary projectLaunchSummary
		if err := json.Unmarshal([]byte(review), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.SchemaMigrationID != decisions[0] || summary.SchemaMigrationChoice == nil || summary.SchemaMigrationChoice.Source != "saved" || summary.SchemaMigrationChoice.Generation != 1 {
			t.Fatalf("saved migration absent: %s", review)
		}
		validateQueueCLI(t, "ProjectLaunchSummaryV10", review)
		response := queueCLICommand(t, append(append([]string{"--project", authority, "project", "start"}, launchArgs...), "--expected-launch-digest", summary.ReviewDigest, "--command-id", fmt.Sprintf("command:saved-run-%d", i))...)
		var started projectStartResult
		if err := json.Unmarshal([]byte(response), &started); err != nil {
			t.Fatal(err)
		}
		if started.Run.Run.SchemaMigration == nil || started.Run.Run.SchemaMigration.DecisionID != decisions[0] {
			t.Fatalf("saved binding lost: %s", response)
		}
	}
}

func TestCLIProjectSavedSchemaMigrationReviewOverrideClearAndReplay(t *testing.T) {
	t.Parallel()
	root, authority, launchArgs, compiled, decisions := projectMigrationFixture(t, false, "compile-values")
	invoke := func(args ...string) string {
		t.Helper()
		return queueCLICommand(t, append([]string{"--project", authority}, args...)...)
	}
	inspect := func() prifly.LaunchSchemaMigrationInspection {
		t.Helper()
		raw := invoke("project", "schema-migration", "inspect", "--repository", root, "--launch", "single", "--host", "codex-cli")
		var view prifly.LaunchSchemaMigrationInspection
		if err := json.Unmarshal([]byte(raw), &view); err != nil {
			t.Fatal(err)
		}
		validateQueueCLI(t, "LaunchSchemaMigrationInspectionV1", raw)
		return view
	}
	prepare := func(extra ...string) projectLaunchSummary {
		t.Helper()
		args := append(append([]string{"project", "questionnaire", "--prepare"}, launchArgs...), extra...)
		raw := invoke(args...)
		var view projectLaunchSummary
		if err := json.Unmarshal([]byte(raw), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	fail := func(want string, args ...string) {
		t.Helper()
		code, _, stderr := runCLI(t, append([]string{"--project", authority}, args...)...)
		if code == 0 || !strings.Contains(stderr, want) {
			t.Fatalf("wanted %s: exit=%d %s", want, code, stderr)
		}
	}
	absent := inspect()
	if absent.State.Choice != nil || absent.State.Generation != 0 {
		t.Fatal(absent)
	}
	set := []string{"project", "schema-migration", "set", "--repository", root, "--launch", "single", "--host", "codex-cli", "--id", decisions[0], "--expected-build-key", compiled.BuildKey, "--expected-generation", "0", "--reason", "reviewed", "--command-id", "command:choice"}
	invoke(set...)
	selected := inspect()
	if !selected.Valid || selected.State.Generation != 1 {
		t.Fatal(selected)
	}
	review := prepare()
	if review.SchemaMigrationChoice == nil || review.SchemaMigrationChoice.Source != "saved" || review.SchemaMigrationChoice.Generation != 1 {
		t.Fatal("saved provenance absent", review)
	}
	emptyExplicit := prepare("--schema-migration", "")
	if emptyExplicit.ReviewDigest != review.ReviewDigest || emptyExplicit.SchemaMigrationChoice.Source != "saved" {
		t.Fatal("empty explicit flag disabled the saved choice")
	}
	explicit := prepare("--schema-migration", decisions[1])
	if explicit.SchemaMigrationChoice.Source != "explicit" || explicit.SchemaMigrationID != decisions[1] {
		t.Fatal("explicit override lost")
	}
	fail("schema_migration_decision_invalid", append(append([]string{"project", "questionnaire", "--prepare"}, launchArgs...), "--schema-migration", "schema-migration:unknown")...)
	fail("expected-launch-digest", append([]string{"project", "start"}, launchArgs...)...)
	fail("project_start_stale_launch", append(append([]string{"project", "start"}, launchArgs...), "--schema-migration", decisions[0], "--expected-launch-digest", review.ReviewDigest)...)
	start := append(append([]string{"project", "start"}, launchArgs...), "--expected-launch-digest", review.ReviewDigest, "--command-id", "command:saved-replay")
	raw := invoke(start...)
	var first projectStartResult
	if err := json.Unmarshal([]byte(raw), &first); err != nil {
		t.Fatal(err)
	}
	validateQueueCLI(t, "ProjectStartV10", raw)
	// A real source byte changes the build without creating a different approval.
	writeFixtureFile(t, root, ".prifly/workflows/single/project/compile-values.yaml", "authoring: prifly-compile-values/1\nvalues: {task_description_limit: 32000}\n")
	fail("schema_migration_scope_mismatch", append([]string{"project", "questionnaire", "--prepare"}, launchArgs...)...)
	writeFixtureFile(t, root, ".prifly/workflows/single/project/compile-values.yaml", "authoring: prifly-compile-values/1\nvalues: {task_description_limit: 12000}\n")
	contextFile := filepath.Join(root, ".prifly/workflows/single/contexts/work.yaml")
	original, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/single/contexts/work.yaml", strings.Replace(string(original), "Fixture work.", "Changed launch work.", 1))
	fail("schema_migration_scope_mismatch", append([]string{"project", "questionnaire", "--prepare"}, launchArgs...)...)
	drift := inspect()
	if drift.Valid || drift.Current == nil || drift.Current.BuildKey == compiled.BuildKey {
		t.Fatal("build drift not shown", drift)
	}
	profileFile := filepath.Join(root, ".prifly/project.yaml")
	profileBytes, err := os.ReadFile(profileFile)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", "invalid: [")
	// The accepted set replays before reading either changed source or profile.
	replaySet := invoke(set...)
	var replay prifly.LaunchSchemaMigrationResult
	if err := json.Unmarshal([]byte(replaySet), &replay); err != nil || !replay.Duplicate {
		t.Fatal("set replay reread mutable sources", replaySet, err)
	}
	invalid := inspect()
	if invalid.State.Choice == nil || len(invalid.Reasons) == 0 {
		t.Fatal("recorded inspection unavailable", invalid)
	}
	clear := []string{"project", "schema-migration", "clear", "--repository", root, "--launch", "single", "--expected-generation", "1", "--reason", "clear invalid launch", "--command-id", "command:clear-choice"}
	invoke(clear...)
	writeFixtureFile(t, root, ".prifly/project.yaml", string(profileBytes))
	writeFixtureFile(t, root, ".prifly/workflows/single/contexts/work.yaml", string(original))
	cleared := inspect()
	if cleared.State.Choice != nil || cleared.State.Generation != 2 {
		t.Fatal(cleared)
	}
	var repeated projectStartResult
	if err := json.Unmarshal([]byte(invoke(start...)), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.Run.Run.ID != first.Run.Run.ID || repeated.SchemaMigrationChoice.Generation != 1 {
		t.Fatal("accepted Start changed after clear")
	}
	restore := append([]string{}, set...)
	for i := range restore {
		if restore[i] == "command:choice" {
			restore[i] = "command:restore-choice"
		}
		if restore[i] == "--expected-generation" {
			restore[i+1] = "2"
		}
	}
	invoke(restore...)
	fail("project_start_stale_launch", append(append([]string{"project", "start"}, launchArgs...), "--expected-launch-digest", review.ReviewDigest)...)
	invoke(clear...)
	if inspect().State.Generation != 3 {
		t.Fatal("old clear receipt mutated a newer choice")
	}
	// Setting the choice never reverses the terminal package withdrawal.
	e, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	packages, err := e.Packages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages.Packages {
		if pkg.Ref.ID == "test:package/project-legacy-schemas" && pkg.Status != prifly.PackageRevoked {
			t.Fatal("choice restored revoked package")
		}
	}
}

func TestCLIProjectSavedSchemaMigrationBindsHostBytesAndPackageProfile(t *testing.T) {
	t.Parallel()
	root, authority, launchArgs, compiled, decisions := projectMigrationFixture(t, false, "build-variants")
	set := []string{"--project", authority, "project", "schema-migration", "set", "--repository", root, "--launch", "single", "--host", "codex-cli", "--id", decisions[0], "--expected-build-key", compiled.BuildKey, "--expected-generation", "0", "--reason", "exact variant"}
	queueCLICommand(t, set...)
	for _, kind := range []string{"host", "profile", "host-bytes"} {
		args := append([]string{}, launchArgs...)
		switch kind {
		case "host":
			for i := range args {
				if args[i] == "codex-cli" {
					args[i] = "codex-app"
				}
			}
		case "profile":
			args = append(args, "--package-profile", "b")
		case "host-bytes":
			writeFixtureFile(t, root, ".codex/skills/choice/SKILL.md", "Changed CLI selected work.\n")
		}
		code, _, stderr := runCLI(t, append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, args...)...)
		if code == 0 || !strings.Contains(stderr, "schema_migration_scope_mismatch") {
			t.Fatalf("%s inherited choice: %d %s", kind, code, stderr)
		}
	}
	// The same root remains manageable after its launch declaration disappears.
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages: {}\nlaunches: {}\n")
	raw := queueCLICommand(t, "--project", authority, "project", "schema-migration", "clear", "--repository", root, "--launch", "single", "--expected-generation", "1", "--reason", "launch removed")
	var cleared prifly.LaunchSchemaMigrationResult
	if err := json.Unmarshal([]byte(raw), &cleared); err != nil || cleared.State.Choice != nil || cleared.State.Generation != 2 {
		t.Fatal(raw, err)
	}
}

// The first response can fail after admission. A retry still owns that Run,
// even if the owner has since cleared the saved selection.
func TestCLIProjectSavedSchemaMigrationAcceptedReplayAfterDriveFailure(t *testing.T) {
	t.Parallel()
	root, authority, launchArgs, compiled, decisions := projectMigrationFixture(t, false)
	queueCLICommand(t, "--project", authority, "project", "schema-migration", "set", "--repository", root, "--launch", "single", "--host", "codex-cli", "--id", decisions[0], "--expected-build-key", compiled.BuildKey, "--expected-generation", "0", "--reason", "reviewed", "--command-id", "command:save-before-failure")
	reviewRaw := queueCLICommand(t, append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, launchArgs...)...)
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(reviewRaw), &review); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(authority, ".prifly/state/driver.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	commandID := "command:accepted-before-drive-failure"
	args := append(append([]string{"--project", authority, "project", "start"}, launchArgs...), "--expected-launch-digest", review.ReviewDigest, "--command-id", commandID)
	code, _, stderr := runCLI(t, args...)
	if code == 0 || !strings.Contains(stderr, "driver_already_active") {
		t.Fatalf("post-admission failure missing: %d %s", code, stderr)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := engine.Receipt(context.Background(), commandID)
	if err != nil || receipt.Rejection != nil {
		t.Fatal("Run was not accepted", receipt, err)
	}
	view, err := engine.View(context.Background(), receipt.RunID)
	engine.Close()
	if err != nil || view.Run.SchemaMigration == nil || view.Run.SchemaMigration.DecisionID != decisions[0] {
		t.Fatal("original Run missing", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	queueCLICommand(t, "--project", authority, "project", "schema-migration", "clear", "--repository", root, "--launch", "single", "--expected-generation", "1", "--reason", "owner cleared after admission", "--command-id", "command:clear-after-failure")
	t.Logf("[FIX:24] retry accepted Run %s after clear", receipt.RunID)
	raw := queueCLICommand(t, args...)
	var replay projectStartResult
	if err := json.Unmarshal([]byte(raw), &replay); err != nil {
		t.Fatal(err)
	}
	validateQueueCLI(t, "ProjectStartV10", raw)
	if replay.RunID != receipt.RunID || replay.SchemaMigrationChoice == nil || replay.SchemaMigrationChoice.Generation != 1 || replay.SchemaMigrationID != decisions[0] {
		t.Fatalf("accepted context changed: %s", raw)
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	repeated, err := engine.Receipt(context.Background(), commandID)
	if err != nil || repeated.Digest != receipt.Digest || repeated.Cut != receipt.Cut {
		t.Fatal("retry changed original receipt", repeated, err)
	}
	args[len(args)-1] = "command:new-after-clear"
	code, _, stderr = runCLI(t, args...)
	if code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
		t.Fatalf("new command reused stale review: %d %s", code, stderr)
	}
	runs, err := engine.Runs(context.Background())
	if err != nil || len(runs) != 1 {
		t.Fatal("retry admitted another Run", len(runs), err)
	}
	t.Log("[FIX:24] accepted replay preserved; new command refused")
}
