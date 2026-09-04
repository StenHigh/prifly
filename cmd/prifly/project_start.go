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
	PackageProfile string                `json:"package_profile,omitempty"`
	DecisionSheet  *prifly.DecisionSheet `json:"decision_sheet,omitempty"`
	Run            prifly.RunView        `json:"run"`
	Workspace      prifly.WorktreeClaim  `json:"workspace"`
}

type projectPreflight struct {
	PackageProfile string
	Catalog        prifly.DecisionCatalog
	Sheet          prifly.DecisionSheet
	Declared       bool
}

// project start is intentionally the only executable Project entry point. It
// seals declared YAML into a disposable package, then hands the first task to
// an existing host; it never starts a provider or background worker.
func (c *cli) projectStart(ctx context.Context, args []string) error {
	f := flags("project start")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	launchID := f.String("launch", "", "declared launch ID from project.yaml")
	host := f.String("host", "", "host entry point that selects project skills")
	brief := f.String("brief", "", "confirmed RunBrief JSON file")
	workspace := f.String("workspace", "worktree", "worktree or checkout")
	packageProfile := f.String("package-profile", "", "per-Run package profile")
	decisionPolicy := f.String("decision-policy", "attended", "attended or autonomous declared-decision policy")
	expectedCatalog := f.String("expected-decision-catalog-digest", "", "catalog digest returned by project questionnaire")
	command := f.String("command-id", "", "stable command identity for an explicit retry")
	inputs := bindings{}
	refFiles := bindings{}
	answers := stringsFlag{}
	f.Var(inputs, "input", "declared input PORT=FILE")
	f.Var(refFiles, "input-ref", "declared input PORT=ARTIFACT_REF.json")
	f.Var(&answers, "preflight-answer", "declared decision ID=JSON")
	if err := parse(f, args); err != nil {
		return err
	}
	if *launchID == "" || *host == "" || *brief == "" {
		return usageError("project start requires --launch, --host and --brief")
	}
	if *workspace != "worktree" && *workspace != "checkout" {
		return usageError("project_start_invalid_workspace: use worktree or checkout")
	}
	if *command == "" {
		*command = commandID()
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	launch, exists := profile.Launches[*launchID]
	if !exists || launch.Kind != "workflow" {
		return usageError("project_start_unknown_launch: " + *launchID)
	}
	if _, err := profile.skillsRoot(*host); err != nil {
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
	if err := projectStartInputs(*details, inputs, refFiles); err != nil {
		return err
	}
	preflight, err := projectStartPreflight(root, profile, packageName, *packageProfile, *decisionPolicy, answers)
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
	if err := authority.Close(); err != nil {
		return err
	}
	briefPath := projectRequestFile(root, *brief)
	briefBytes, err := readFile(briefPath, prifly.MaxDefinitionBytes)
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
	refs := map[string]prifly.ArtifactRef{}
	for port, path := range refFiles {
		var ref prifly.ArtifactRef
		if err := readJSON(projectRequestFile(root, path), &ref); err != nil {
			return err
		}
		refs[port] = ref
	}
	// Read input files before any authority mutation. Their schema is checked by
	// Start against the exact sealed workflow after it is registered.
	inputPaths := bindings{}
	for port, path := range inputs {
		resolved := projectRequestFile(root, path)
		if _, err := readFile(resolved, prifly.MaxArtifactBytes); err != nil {
			return err
		}
		inputPaths[port] = resolved
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
	workflowPath, err := projectCompiledLaunchWorkflow(root, launch, compiled.Components)
	if err != nil {
		return err
	}
	if err := projectVerifySealedDecisionCatalog(packageDirectory, preflight); err != nil {
		return err
	}

	engine, err := prifly.Open(c.project, false)
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()
	before, err := engine.Claims(ctx)
	if err != nil {
		return err
	}
	knownClaims := map[string]bool{}
	for _, claim := range before.Claims {
		knownClaims[claim.ID] = true
	}
	claim, err := engine.ClaimWorktree(ctx, prifly.ClaimRequest{CommandID: *command + ":workspace", Repository: root, OwnerID: "project-launch:" + *command, WorkspaceMode: *workspace})
	if err != nil {
		return err
	}
	createdClaim := !knownClaims[claim.ID]
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
	startOptions := prifly.StartOptions{CommandID: *command, WorkflowFile: workflowPath, BriefFile: briefPath, Inputs: inputPaths, InputRefs: refs, WorkspaceMode: *workspace}
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
	if err := engine.Drive(ctx, started.Receipt.RunID); err != nil {
		return fmt.Errorf("project_start_incomplete: run %s, workspace %s: %w", started.Receipt.RunID, claim.ID, err)
	}
	view, err := engine.View(ctx, started.Receipt.RunID)
	if err != nil {
		return err
	}
	result := projectStartResult{SchemaVersion: "project-start/1", Repository: root, Launch: *launchID, Package: compiled.Package, PackageProfile: selectedProfile, Run: view, Workspace: claim}
	if preflight.Declared {
		result.SchemaVersion = "project-start/2"
		result.DecisionSheet = &preflight.Sheet
	}
	return c.emit(result)
}

func projectRequestFile(repository, name string) string {
	if name == "-" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(repository, name)
}

func projectStartInputs(launch projectLaunchDetail, inputs, refs bindings) error {
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

func projectStartPreflight(root string, profile projectProfile, packageName, requestedProfile, decisionPolicy string, rawAnswers []string) (projectPreflight, error) {
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
		if !exists || definition.Phase != "preflight" || !projectDecisionApplies(definition, selected, answers) {
			return projectPreflight{}, usageError("project_start_unknown_decision: " + id)
		}
		if definition.Destination.Kind == "package_profile" {
			return projectPreflight{}, usageError("project_start_profile_is_selected_with_package_profile: " + id)
		}
		if err := projectValidateDecisionValue(definition, value); err != nil {
			return projectPreflight{}, usageError("project_start_invalid_decision_answer: " + id + ": " + err.Error())
		}
	}
	answerSources := map[string]string{}
	for _, definition := range source.DecisionCatalog {
		if definition.Phase != "preflight" || definition.Destination.Kind == "package_profile" || !definition.Required || !projectDecisionApplies(definition, selected, answers) {
			continue
		}
		if _, answered := answers[definition.ID]; !answered {
			if decisionPolicy != "autonomous" || !definition.Automatic || definition.Sensitivity != "ordinary" || len(definition.Recommendation) == 0 {
				return projectPreflight{}, usageError("project_start_missing_decision: " + definition.ID)
			}
			value, err := flow.Canonical(definition.Recommendation)
			if err != nil || projectValidateDecisionValue(definition, value) != nil {
				return projectPreflight{}, usageError("project_start_invalid_decision_default: " + definition.ID)
			}
			answers[definition.ID], answerSources[definition.ID] = value, "autonomous_policy"
		}
	}
	catalog := prifly.DecisionCatalog{SchemaVersion: prifly.DecisionCatalogVersion, Decisions: source.DecisionCatalog}
	digest, err := prifly.DecisionCatalogDigest(catalog)
	if err != nil {
		return projectPreflight{}, err
	}
	sheet := prifly.DecisionSheet{SchemaVersion: prifly.DecisionSheetVersion, CatalogDigest: digest, PackageProfile: selected, ProfileSource: profileSource, DecisionPolicy: decisionPolicy, Records: []prifly.DecisionRecord{}}
	for _, definition := range source.DecisionCatalog {
		if definition.Phase != "preflight" || !projectDecisionApplies(definition, selected, answers) {
			continue
		}
		value, answered := answers[definition.ID]
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
	skillsRoot, err := profile.skillsRoot(host)
	if err != nil {
		return projectCompileResult{}, err
	}
	skillsRoot, err = canonicalProjectPath(filepath.Join(root, filepath.FromSlash(skillsRoot)))
	if err != nil {
		return projectCompileResult{}, err
	}
	if relative, err := filepath.Rel(root, skillsRoot); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return projectCompileResult{}, usageError("project_compile_invalid_host_root: selected host skills root must stay inside the repository")
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
	components, err := compileProjectPackage(root, skillsRoot, output, source, registry, values)
	if err != nil {
		return projectCompileResult{}, err
	}
	packageRef, err := writeProjectPackageManifest(output, source, components, packages)
	if err != nil {
		return projectCompileResult{}, err
	}
	complete = true
	return projectCompileResult{SchemaVersion: "project-compile/1", Repository: root, Package: packageRef, Output: output, Components: components}, nil
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
