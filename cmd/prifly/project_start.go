package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectStartResult struct {
	SchemaVersion  string                `json:"schema_version"`
	Repository     string                `json:"repository"`
	Launch         string                `json:"launch"`
	Package        flow.Ref              `json:"package"`
	AuthorPackage  *projectBuildIdentity `json:"author_package,omitempty"`
	BuildKey       string                `json:"build_key,omitempty"`
	PackageProfile string                `json:"package_profile,omitempty"`
	DecisionSheet  *prifly.DecisionSheet `json:"decision_sheet,omitempty"`
	// A pointer so an autonomous launch with nothing blocked still reports an
	// empty list: an absent field would read as "nothing to say" rather than
	// "the policy can take every declared runtime decision".
	AutonomyUnanswered *[]prifly.UnansweredDecision `json:"autonomy_unanswered,omitempty"`
	Run                prifly.RunView               `json:"run"`
	Workspace          *prifly.WorktreeClaim        `json:"workspace,omitempty"`
	LaunchSummary      *projectLaunchSummary        `json:"launch_summary,omitempty"`
}

type projectPreflight struct {
	PackageProfile string
	Catalog        prifly.DecisionCatalog
	Sheet          prifly.DecisionSheet
	Declared       bool
}

// project start is intentionally the only executable Project entry point. It
// seals declared YAML into a disposable package and uses the existing engine.
// An assisted step waits for its host; a managed step uses its approved worker.
func (c *cli) projectStart(ctx context.Context, args []string) error {
	return c.projectPrepareAndStart(ctx, args, false)
}

func (c *cli) projectPrepareAndStart(ctx context.Context, args []string, prepare bool) error {
	f := flags("project start")
	repository := f.String("repository", ".", "directory that owns the shared Pri-Fly profile")
	launchID := f.String("launch", "", "declared launch ID from project.yaml")
	host := f.String("host", "", "host entry point that selects project skills")
	brief := f.String("brief", "", "confirmed RunBrief JSON file")
	workspace := f.String("workspace", "", "explicit worktree or checkout for assisted repository writes")
	allowExecution := f.Bool("allow-execution", false, "approve the selected workflow programs, arguments and supporting files")
	packageProfile := f.String("package-profile", "", "per-Run package profile")
	decisionPolicy := f.String("decision-policy", "attended", "attended or autonomous declared-decision policy")
	expectedCatalog := f.String("expected-decision-catalog-digest", "", "catalog digest returned by project questionnaire")
	expectedLaunch := f.String("expected-launch-digest", "", "review digest returned by project questionnaire --prepare")
	command := f.String("command-id", "", "stable command identity for an explicit retry")
	inputs := bindings{}
	refFiles := bindings{}
	answers := stringsFlag{}
	runtimeAnswers := stringsFlag{}
	f.Var(inputs, "input", "declared input PORT=FILE")
	f.Var(refFiles, "input-ref", "declared input PORT=ARTIFACT_REF.json")
	f.Var(&answers, "preflight-answer", "declared preflight decision ID=JSON")
	f.Var(&runtimeAnswers, "runtime-answer", "declared runtime decision ID=JSON, sealed before the Run starts")
	if err := parse(f, args); err != nil {
		return err
	}
	if *launchID == "" {
		return usageError("project start requires --launch")
	}
	if *workspace != "" && *workspace != "worktree" && *workspace != "checkout" {
		return usageError("project_start_invalid_workspace: use worktree or checkout")
	}
	if *command == "" {
		*command = commandID()
	}
	root, err := projectRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	neutral := profile.SchemaVersion == projectVariantProfileVersion
	if !neutral {
		if prepare || *expectedLaunch != "" {
			return usageError("project_questionnaire_prepare_requires_profile_3: exact launch review requires an explicit Project profile /3 migration; legacy start remains supported")
		}
		if *host == "" || *brief == "" {
			return usageError("project start requires --launch, --host and --brief for profile /2")
		}
		if *workspace == "" {
			*workspace = "worktree"
		}
	}
	if err := c.projectAuthority(root, profile); err != nil {
		return err
	}
	launch, exists := profile.Launches[*launchID]
	if !exists || launch.Kind != "workflow" {
		return usageError("project_start_unknown_launch: " + *launchID)
	}
	if _, err := projectCompileSkillsRoot(root, profile, *host); err != nil {
		return err
	}
	packageName, err := profile.packageForLaunch(root, launch)
	if err != nil {
		return err
	}
	launches, err := profile.launchDetails(root)
	if err != nil {
		return err
	}
	var details *projectLaunchDetail
	for i := range launches {
		if launches[i].ID == *launchID {
			details = &launches[i]
			break
		}
	}
	if details == nil {
		return local.ErrIntegrity
	}
	if err := projectStartInputs(*details, inputs, refFiles, !neutral); err != nil {
		return err
	}
	preflight, err := projectStartPreflight(root, profile, packageName, *packageProfile, *decisionPolicy, answers, runtimeAnswers)
	if err != nil {
		return err
	}
	if *expectedCatalog != "" && *expectedCatalog != preflight.Sheet.CatalogDigest {
		return usageError("project_start_stale_decision_catalog: questionnaire differs from the current project catalog")
	}
	selectedProfile := preflight.PackageProfile
	// Project launch is the only route that needs a context-capable authority;
	// reject an older one before reading sources or recording any claim/package.
	authority, err := prifly.Open(c.project, true)
	if err != nil {
		return err
	}
	if err := checkProjectAuthority(authority); err != nil {
		_ = authority.Close()
		return err
	}
	configurationDigest, err := projectReviewConfiguration(authority.Config)
	if err != nil {
		_ = authority.Close()
		return err
	}
	if err := authority.Close(); err != nil {
		return err
	}
	var briefBytes []byte
	if *brief != "" {
		briefBytes, err = readFile(projectRequestFile(root, *brief), prifly.MaxDefinitionBytes)
		if err != nil {
			return err
		}
		if err := flow.ValidateProtocol("RunBrief", briefBytes); err != nil {
			return err
		}
		var confirmed prifly.Brief
		if err := json.Unmarshal(briefBytes, &confirmed); err != nil || confirmed.Confirmation != "explicit" {
			return usageError("project_start_invalid_brief: RunBrief requires explicit owner confirmation")
		}
	}
	refs := map[string]prifly.ArtifactRef{}
	for port, path := range refFiles {
		var ref prifly.ArtifactRef
		if err := readJSON(projectRequestFile(root, path), &ref); err != nil {
			return err
		}
		refs[port] = ref
	}
	// Freeze caller bytes before any authority mutation; preflight and Start
	// see the same bytes even when an original file changes during compilation.
	inputValues := map[string]json.RawMessage{}
	inputPaths := map[string]string{}
	for port, path := range inputs {
		resolved := projectRequestFile(root, path)
		data, err := readFile(resolved, prifly.MaxArtifactBytes)
		if err != nil {
			return err
		}
		inputValues[port] = data
		inputPaths[port] = resolved
	}
	if err := projectDecisionInputs(preflight, inputValues, refs, neutral); err != nil {
		return err
	}

	temporary, err := os.MkdirTemp("", "prifly-project-start-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	packageDirectory := filepath.Join(temporary, "package")
	compiled, err := c.compileDeclaredProjectPackage(ctx, root, profile, packageName, *host, selectedProfile, packageDirectory)
	if err != nil {
		return err
	}
	workflowPath, err := projectCompiledLaunchPath(root, launch, compiled)
	if err != nil {
		return err
	}
	if err := projectVerifySealedDecisionCatalog(packageDirectory, preflight); err != nil {
		return err
	}
	var execution *prifly.ExecutionBindings
	var requirements projectLaunchRequirements
	needsWorkspace := !neutral
	if neutral {
		preflightEngine, err := prifly.Open(c.project, true)
		if err != nil {
			return err
		}
		execution, requirements, err = projectValidateLaunch(ctx, preflightEngine, root, compiled, workflowPath, *host, *workspace, *allowExecution, inputValues, refs)
		needsWorkspace = requirements.GitWorkspace
		closeErr := preflightEngine.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	var summary projectLaunchSummary
	if neutral {
		summary = projectLaunchSummary{SchemaVersion: "project-launch-summary/2", Repository: root, Authority: c.project, Launch: *launchID, Host: *host, WorkspaceMode: *workspace, Package: compiled.Package, AuthorPackage: compiled.AuthorPackage, BuildKey: compiled.BuildKey, InputDigests: map[string]string{}, InputRefs: refs, ConfigurationDigest: configurationDigest, DecisionSheet: preflight.Sheet, DecisionStates: projectDecisionStates(preflight), KnownQuestionsOnly: true, SessionLimits: requirements.sessionLimits}
		summary.Requirements = &requirements
		for _, component := range compiled.Components {
			if component.Path == workflowPath {
				summary.Workflow = component.Ref
			}
		}
		for name, data := range inputValues {
			summary.InputDigests[name] = projectBytesDigest(data)
		}
		if len(briefBytes) != 0 {
			summary.BriefDigest = projectBytesDigest(briefBytes)
		}
		summary.Execution, err = projectReviewExecutors(execution)
		if err != nil {
			return err
		}
		summary.ReviewDigest, err = projectReviewDigest(summary)
		if err != nil {
			return err
		}
		if *expectedLaunch != "" && *expectedLaunch != summary.ReviewDigest {
			return usageError("project_start_stale_launch: sources, inputs, bindings or decisions changed; repeat project questionnaire --prepare and review the new summary")
		}
		if prepare {
			return c.emit(summary)
		}
		// Keep stdout's one final result intact. The pre-dispatch summary is on
		// stderr, and a failed write stops before registration, claim or Run.
		if c.errout != nil {
			if err := json.NewEncoder(c.errout).Encode(summary); err != nil {
				return err
			}
		}
	} else if preflight.Declared && c.errout != nil {
		// Legacy still presents its decisions before dispatch, but does not
		// advertise checked inputs/executors under the neutral review contract.
		if err := json.NewEncoder(c.errout).Encode(map[string]any{"decision_sheet": preflight.Sheet, "autonomy_unanswered": prifly.DecisionsAutonomyCannotTake(&preflight.Catalog, &preflight.Sheet)}); err != nil {
			return err
		}
	}

	engine, err := prifly.Open(c.project, false)
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()
	if neutral {
		// Recheck mutable machine configuration after publishing the summary.
		// Package/source/input bytes are already held in the prepared request.
		currentConfiguration, err := projectReviewConfiguration(engine.Config)
		if err != nil {
			return err
		}
		currentExecution, err := projectReviewExecutors(execution)
		if err != nil {
			return err
		}
		currentSummary := summary
		currentSummary.ConfigurationDigest, currentSummary.Execution, currentSummary.ReviewDigest = currentConfiguration, currentExecution, ""
		currentDigest, err := projectReviewDigest(currentSummary)
		if err != nil {
			return err
		}
		if currentDigest != summary.ReviewDigest {
			return usageError("project_start_stale_launch: local execution configuration changed after the summary; prepare and review again")
		}
		// Keep the resolved reviewed path in the request. A later retarget of a
		// machine-local symlink must not select a different installed program.
		for index := range execution.Bindings {
			execution.Bindings[index].Config.Executable = summary.Execution[index].Executable
		}
	}
	var claim *prifly.WorktreeClaim
	createdClaim := false
	if needsWorkspace {
		before, err := engine.Claims(ctx)
		if err != nil {
			return err
		}
		selected, err := engine.ClaimWorktree(ctx, prifly.ClaimRequest{CommandID: *command + ":workspace", Repository: root, OwnerID: "project-launch:" + *command, WorkspaceMode: *workspace})
		if err != nil {
			return err
		}
		claim, createdClaim = &selected, true
		for _, previous := range before.Claims {
			if previous.ID == selected.ID {
				createdClaim = false
			}
		}
	}
	imported := false
	if err := projectPackageAvailable(ctx, engine, compiled.Package); err != nil {
		if !errors.Is(err, local.ErrNotFound) {
			if createdClaim {
				_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
			}
			return err
		}
		if _, err := engine.ImportPackage(ctx, prifly.PackageImportRequest{CommandID: *command + ":import", Directory: packageDirectory, Reason: "declared project launch " + *launchID}); err != nil {
			if createdClaim {
				_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
			}
			return err
		}
		// The engine holds the imported package already; closing and reopening
		// the authority only to see it re-verified the store for nothing.
		imported = true
	}
	workflowPath, err = projectInstalledWorkflowPath(ctx, engine, compiled.Package, workflowPath)
	if err != nil {
		return err
	}
	startOptions := prifly.StartOptions{CommandID: *command, WorkflowFile: workflowPath, Brief: briefBytes, Inputs: inputPaths, InputRefs: refs, WorkspaceMode: *workspace}
	if neutral {
		startOptions.SchemaVersion, startOptions.ExecutionBindings = "2", execution
		startOptions.Inputs, startOptions.InputValues = nil, inputValues
	}
	if preflight.Declared {
		startOptions.DecisionCatalog, startOptions.DecisionSheet = &preflight.Catalog, &preflight.Sheet
	}
	started, err := engine.Start(ctx, startOptions)
	if err != nil {
		if imported {
			_, _ = engine.SetPackageStatus(ctx, prifly.PackageLifecycleRequest{CommandID: *command + ":rollback-package", ID: compiled.Package.ID, Version: compiled.Package.Version, Status: prifly.PackageRemoved, Reason: "project start did not create a Run"})
		}
		if createdClaim {
			_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
		}
		return err
	}
	if neutral {
		expected := map[string]string{}
		for _, reviewed := range summary.Execution {
			expected[reviewed.DefinitionRef.String()] = reviewed.ExecutableDigest
		}
		if err := engine.CheckPinnedExecutables(ctx, started.Receipt.RunID, expected); err != nil {
			return &prifly.Fault{Code: "project_start_incomplete", Message: fmt.Sprintf("run %s was not driven: inspect its pinned executors before explicit continuation", started.Receipt.RunID), Cause: err}
		}
	}
	if err := engine.Drive(ctx, started.Receipt.RunID); err != nil {
		return &prifly.Fault{Code: "project_start_incomplete", Message: fmt.Sprintf("run %s", started.Receipt.RunID), Cause: err}
	}
	view, err := engine.View(ctx, started.Receipt.RunID)
	if err != nil {
		return err
	}
	result := projectStartResult{SchemaVersion: "project-start/1", Repository: root, Launch: *launchID, Package: compiled.Package, PackageProfile: selectedProfile, Run: view, Workspace: claim}
	if preflight.Declared {
		result.SchemaVersion = "project-start/2"
		result.DecisionSheet = &preflight.Sheet
		if preflight.Sheet.DecisionPolicy == "autonomous" {
			blocked := prifly.DecisionsAutonomyCannotTake(&preflight.Catalog, &preflight.Sheet)
			result.AutonomyUnanswered = &blocked
		}
	}
	if compiled.AuthorPackage != nil {
		result.SchemaVersion = "project-start/3"
		result.AuthorPackage, result.BuildKey = compiled.AuthorPackage, compiled.BuildKey
		result.LaunchSummary = &summary
	}
	return c.emit(result)
}

func projectRequestFile(repository, name string) string {
	if name == "-" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(repository, name)
}

func projectStartInputs(launch projectLaunchDetail, inputs, refs bindings, requireExplicit bool) error {
	declared := map[string]projectLaunchInput{}
	for _, input := range launch.Inputs {
		declared[input.Name] = input
	}
	for port := range inputs {
		if _, ok := declared[port]; !ok {
			return usageError("project_start_unknown_input: " + port)
		}
		if _, ok := refs[port]; ok {
			return usageError("project_start_duplicate_input: " + port)
		}
	}
	for port := range refs {
		if _, ok := declared[port]; !ok {
			return usageError("project_start_unknown_input: " + port)
		}
	}
	// Profile /3 resolves defaults and project settings against the compiled
	// contract; the source listing alone cannot decide requiredness.
	if !requireExplicit {
		return nil
	}
	for port, input := range declared {
		if input.Required {
			if _, file := inputs[port]; !file {
				if _, ref := refs[port]; !ref {
					return usageError("project_start_missing_input: " + port)
				}
			}
		}
	}
	return nil
}

func projectStartPreflight(root string, profile projectProfile, packageName, requestedProfile, decisionPolicy string, rawAnswers, rawRuntimeAnswers []string) (projectPreflight, error) {
	return projectDecisionPreflight(root, profile, packageName, requestedProfile, decisionPolicy, rawAnswers, rawRuntimeAnswers, true)
}

// The questionnaire validates the same selections as Start, but can display
// missing required answers while the owner is still filling out the form.
func projectDecisionPreflight(root string, profile projectProfile, packageName, requestedProfile, decisionPolicy string, rawAnswers, rawRuntimeAnswers []string, complete bool) (projectPreflight, error) {
	if decisionPolicy != "attended" && decisionPolicy != "autonomous" {
		return projectPreflight{}, usageError("project_start_invalid_decision_policy: use attended or autonomous")
	}
	pkg, exists := profile.Packages[packageName]
	if !exists {
		return projectPreflight{}, usageError("project_compile_unknown_package: " + packageName)
	}
	folder, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return projectPreflight{}, err
	}
	source, err := readProjectWorkflowFolder(root, folder)
	if err != nil {
		return projectPreflight{}, err
	}
	options, err := projectReadWorkflowOptions(root, source, map[string]any{})
	if err != nil {
		return projectPreflight{}, err
	}
	selected := options.Profile
	profileSource := "none"
	if len(source.Profiles) != 0 {
		profileSource = "package_default"
	}
	if options.Profile != "" {
		profileSource = "project_default"
	}
	if requestedProfile != "" {
		selected = requestedProfile
		profileSource = "actor"
	}
	if err := projectApplyPackageProfile(source, selected, map[string]any{}); err != nil {
		return projectPreflight{}, err
	}
	if selected == "" && len(source.Profiles) != 0 {
		selected = source.DefaultProfile
	}
	answers, err := projectParseDecisionAnswers(rawAnswers)
	if err != nil {
		return projectPreflight{}, err
	}
	definitions := map[string]prifly.DecisionDefinition{}
	for _, definition := range source.DecisionCatalog {
		definitions[definition.ID] = definition
	}
	for id, value := range answers {
		definition, exists := definitions[id]
		if !exists || definition.Phase != "preflight" {
			return projectPreflight{}, usageError("project_start_unknown_decision: " + id)
		}
		if definition.Destination.Kind == "package_profile" {
			return projectPreflight{}, usageError("project_start_profile_is_selected_with_package_profile: " + id)
		}
		if err := projectValidateDecisionValue(definition, value); err != nil {
			return projectPreflight{}, usageError("project_start_invalid_decision_answer: " + id + ": " + err.Error())
		}
	}
	runtime, err := projectParseDecisionAnswers(rawRuntimeAnswers)
	if err != nil {
		return projectPreflight{}, err
	}
	answerSources := map[string]string{}
	for _, definition := range source.DecisionCatalog {
		if definition.Phase != "preflight" || !projectDecisionApplies(definition, selected, answers) {
			continue
		}
		if definition.Destination.Kind == "package_profile" {
			if selected != "" {
				value, err := json.Marshal(selected)
				if err != nil {
					return projectPreflight{}, err
				}
				answers[definition.ID], answerSources[definition.ID] = value, profileSource
			}
			continue
		}
		if !definition.Required {
			continue
		}
		if _, answered := answers[definition.ID]; !answered {
			if decisionPolicy != "autonomous" || !definition.Automatic || definition.Sensitivity != "ordinary" || len(definition.Recommendation) == 0 {
				if !complete {
					continue
				}
				return projectPreflight{}, usageError("project_start_missing_decision: " + definition.ID)
			}
			value, err := flow.Canonical(definition.Recommendation)
			if err != nil || projectValidateDecisionValue(definition, value) != nil {
				return projectPreflight{}, usageError("project_start_invalid_decision_default: " + definition.ID)
			}
			answers[definition.ID], answerSources[definition.ID] = value, "autonomous_policy"
		}
	}
	// Conditions see the effective answers, including allowed policy choices
	// above. Validating raw runtime preanswers earlier rejects a legitimate
	// dependent answer merely because its predecessor was selected by policy.
	for id := range answers {
		if !projectDecisionApplies(definitions[id], selected, answers) {
			return projectPreflight{}, usageError("project_start_unknown_decision: " + id)
		}
	}
	for id, value := range runtime {
		definition, exists := definitions[id]
		if !exists || definition.Phase != "runtime" || !projectDecisionApplies(definition, selected, answers) {
			return projectPreflight{}, usageError("project_start_unknown_decision: " + id)
		}
		if err := projectValidateDecisionValue(definition, value); err != nil {
			return projectPreflight{}, usageError("project_start_invalid_decision_answer: " + id + ": " + err.Error())
		}
	}
	catalog := prifly.DecisionCatalog{SchemaVersion: prifly.DecisionCatalogVersion, Decisions: source.DecisionCatalog}
	digest, err := prifly.DecisionCatalogDigest(catalog)
	if err != nil {
		return projectPreflight{}, err
	}
	sheet := prifly.DecisionSheet{SchemaVersion: prifly.DecisionSheetVersion, CatalogDigest: digest, PackageProfile: selected, ProfileSource: profileSource, DecisionPolicy: decisionPolicy, Records: []prifly.DecisionRecord{}}
	for _, definition := range source.DecisionCatalog {
		if definition.Phase != "preflight" && definition.Phase != "runtime" {
			continue
		}
		if !projectDecisionApplies(definition, selected, answers) {
			continue
		}
		value, answered := answers[definition.ID]
		if definition.Phase == "runtime" {
			value, answered = runtime[definition.ID]
		}
		recordSource := "actor"
		if source, exists := answerSources[definition.ID]; exists {
			recordSource = source
		}
		if definition.Destination.Kind == "package_profile" {
			if selected == "" {
				continue
			}
			encoded, err := json.Marshal(selected)
			if err != nil {
				return projectPreflight{}, err
			}
			value, answered, recordSource = encoded, true, profileSource
		}
		if !answered {
			continue
		}
		definitionDigest, err := prifly.DecisionDefinitionDigest(definition)
		if err != nil {
			return projectPreflight{}, err
		}
		sheet.Records = append(sheet.Records, prifly.DecisionRecord{SchemaVersion: prifly.DecisionRecordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, Status: "answered", Source: recordSource, Value: value})
	}
	if err := prifly.ValidateDecisionSheet(catalog, sheet); err != nil {
		return projectPreflight{}, err
	}
	return projectPreflight{PackageProfile: selected, Catalog: catalog, Sheet: sheet, Declared: len(catalog.Decisions) != 0 || selected != ""}, nil
}

func projectVerifySealedDecisionCatalog(packageDirectory string, preflight projectPreflight) error {
	data, err := os.ReadFile(filepath.Join(packageDirectory, projectDecisionCatalogFile))
	if errors.Is(err, os.ErrNotExist) && len(preflight.Catalog.Decisions) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	var sealed prifly.DecisionCatalog
	if err := json.Unmarshal(data, &sealed); err != nil {
		return err
	}
	digest, err := prifly.DecisionCatalogDigest(sealed)
	if err != nil || digest != preflight.Sheet.CatalogDigest {
		return usageError("project_start_stale_decision_catalog: sealed package differs from the reviewed questionnaire")
	}
	return nil
}

func projectParseDecisionAnswers(values []string) (map[string]json.RawMessage, error) {
	answers := make(map[string]json.RawMessage, len(values))
	for _, raw := range values {
		id, value, found := strings.Cut(raw, "=")
		if !found || !projectValueName.MatchString(id) || value == "" {
			return nil, usageError("project_start_invalid_decision_answer: expected unique ID=JSON")
		}
		if _, duplicate := answers[id]; duplicate {
			return nil, usageError("project_start_invalid_decision_answer: expected unique ID=JSON")
		}
		canonical, err := flow.Canonical([]byte(value))
		if err != nil {
			return nil, usageError("project_start_invalid_decision_answer: " + err.Error())
		}
		answers[id] = canonical
	}
	return answers, nil
}

func projectDecisionApplies(definition prifly.DecisionDefinition, profile string, answers map[string]json.RawMessage) bool {
	if definition.When == nil {
		return true
	}
	if len(definition.When.Profiles) != 0 {
		matchesProfile := false
		for _, candidate := range definition.When.Profiles {
			matchesProfile = matchesProfile || candidate == profile
		}
		if !matchesProfile {
			return false
		}
	}
	for id, expected := range definition.When.Answers {
		actual, exists := answers[id]
		if !exists {
			return false
		}
		canonicalActual, actualErr := flow.Canonical(actual)
		canonicalExpected, expectedErr := flow.Canonical(expected)
		if actualErr != nil || expectedErr != nil || string(canonicalActual) != string(canonicalExpected) {
			return false
		}
	}
	return true
}

func projectValidateDecisionValue(definition prifly.DecisionDefinition, value json.RawMessage) error {
	return prifly.ValidateDecisionValue(definition, value)
}

func (profile projectProfile) packageForLaunch(root string, launch projectLaunch) (string, error) {
	workflow, err := projectLaunchSource(root, launch.Workflow)
	if err != nil {
		return "", err
	}
	folder := filepath.Dir(workflow)
	name := ""
	for candidate, entry := range profile.Packages {
		source, err := projectPackageSourceLocation(root, entry.Source)
		if err != nil {
			return "", err
		}
		if source == folder {
			if name != "" {
				return "", usageError("project_start_ambiguous_package: launch workflow belongs to multiple packages")
			}
			name = candidate
		}
	}
	if name == "" {
		return "", usageError("project_start_missing_package: launch workflow has no declared package")
	}
	return name, nil
}

func (c *cli) compileDeclaredProjectPackage(ctx context.Context, root string, profile projectProfile, name, host, packageProfile, output string) (projectCompileResult, error) {
	pkg, exists := profile.Packages[name]
	if !exists {
		return projectCompileResult{}, usageError("project_compile_unknown_package: " + name)
	}
	skillsRoot, err := projectCompileSkillsRoot(root, profile, host)
	if err != nil {
		return projectCompileResult{}, err
	}
	if projectPathsOverlap(root, output) || projectPathsOverlap(c.project, output) {
		return projectCompileResult{}, usageError("project_compile_unsafe_output: output must stay outside the repository and local authority")
	}
	engine, err := prifly.Open(c.project, true)
	if err != nil {
		return projectCompileResult{}, err
	}
	defer engine.Close()
	_, registry, err := engine.Inventory()
	if err != nil {
		return projectCompileResult{}, err
	}
	packages, err := engine.Packages(ctx)
	if err != nil {
		return projectCompileResult{}, err
	}
	sourcePath, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return projectCompileResult{}, err
	}
	source, err := readProjectWorkflowFolder(root, sourcePath)
	if err != nil {
		return projectCompileResult{}, err
	}
	values := map[string]any{}
	options, err := projectReadWorkflowOptions(root, source, values)
	if err != nil {
		return projectCompileResult{}, err
	}
	for alias, logical := range source.References {
		ref, err := projectLogicalRef(registry, logical)
		if err != nil {
			return projectCompileResult{}, usageError("project_compile_reference " + alias + ": " + err.Error())
		}
		values[alias] = projectRefValue(ref)
	}
	if err := projectApplyPackageProfile(source, packageProfile, values); err != nil {
		return projectCompileResult{}, err
	}
	if err := os.Mkdir(output, 0755); err != nil {
		return projectCompileResult{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(output)
		}
	}()
	result, err := compileAndSealProjectPackage(root, skillsRoot, output, profile.SchemaVersion, packageProfile, source, registry, packages, values, options)
	if err != nil {
		return projectCompileResult{}, err
	}
	complete = true
	return result, nil
}

func projectCompiledLaunchWorkflow(root string, launch projectLaunch, components []projectCompileComponent) (string, error) {
	path, err := projectLaunchSource(root, launch.Workflow)
	if err != nil {
		return "", err
	}
	value, err := projectYAMLDocument(path)
	if err != nil {
		return "", err
	}
	workflow, err := projectFolderWorkflowDefinition(value)
	if err != nil {
		return "", err
	}
	id, idOK := workflow["id"].(string)
	version, versionOK := workflow["version"].(string)
	if !idOK || !versionOK {
		return "", local.ErrIntegrity
	}
	for _, component := range components {
		if component.Kind == "workflow" && component.Ref.ID == id && component.Ref.Version == version {
			return component.Path, nil
		}
	}
	return "", local.ErrIntegrity
}

func projectPackageAvailable(ctx context.Context, engine *prifly.Engine, ref flow.Ref) error {
	packages, err := engine.Packages(ctx)
	if err != nil {
		return err
	}
	for _, entry := range packages.Packages {
		if entry.Ref.ID != ref.ID || entry.Ref.Version != ref.Version {
			continue
		}
		if entry.Ref != ref {
			return usageError("project_start_package_identity_conflict: declared package ID and version already name different bytes")
		}
		if entry.Status != "" && entry.Status != prifly.PackageTrusted {
			return usageError("project_start_package_unavailable: declared package is not trusted")
		}
		return nil
	}
	return local.ErrNotFound
}

func projectInstalledWorkflowPath(ctx context.Context, engine *prifly.Engine, ref flow.Ref, componentPath string) (string, error) {
	packages, err := engine.Packages(ctx)
	if err != nil {
		return "", err
	}
	installed := ""
	for _, entry := range packages.Packages {
		if entry.Ref == ref && (entry.Status == "" || entry.Status == prifly.PackageTrusted) {
			return filepath.ToSlash(filepath.Join(entry.Root, componentPath)), nil
		}
		if entry.Ref.ID == ref.ID && entry.Ref.Version == ref.Version {
			installed = entry.Status
			if entry.Ref.Digest != ref.Digest {
				installed = "different bytes"
			}
		}
	}
	// The engine knows exactly what it was resolving here. Reported as a bare
	// not_found it reads as a missing file, and the reader looks everywhere
	// except at the package that was just sealed.
	reason := "it is not installed"
	if installed != "" {
		reason = "the installed one has " + installed
	}
	return "", usageError("project_start_package_not_installed: the sealed package " + ref.ID + "@" + ref.Version + " was not found among trusted packages: " + reason + "; read package list")
}
