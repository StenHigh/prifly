package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// WorkspacePath is where that claim actually is. The record's own path is
	// relative to the authority that holds it, and beside repository.toplevel
	// it reads as relative to the repository: a cold start looked for the tree
	// under the repository, found nothing, and fell back to git worktree list.
	WorkspacePath string                `json:"workspace_path,omitempty"`
	LaunchSummary *projectLaunchSummary `json:"launch_summary,omitempty"`
	Recovery      *prifly.RecoveryPlan  `json:"recovery,omitempty"`
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
	return c.projectPrepareAndStart(ctx, args, false, false, false)
}

func (c *cli) projectContinue(ctx context.Context, args []string) error {
	if index := slices.Index(args, "--prepare"); index >= 0 {
		return c.projectPrepareAndStart(ctx, append(append([]string{}, args[:index]...), args[index+1:]...), true, true, false)
	}
	return c.projectPrepareAndStart(ctx, args, false, true, false)
}

func (c *cli) projectRecover(ctx context.Context, args []string) error {
	if index := slices.Index(args, "--prepare"); index >= 0 {
		return c.projectPrepareAndStart(ctx, append(append([]string{}, args[:index]...), args[index+1:]...), true, false, true)
	}
	return c.projectPrepareAndStart(ctx, args, false, false, true)
}

func (c *cli) projectPrepareAndStart(ctx context.Context, args []string, prepare, continuation, recovering bool) error {
	f := flags("project start")
	repository := f.String("repository", ".", "directory that owns the shared Pri-Fly profile")
	launchID := f.String("launch", "", "declared launch ID from project.yaml")
	host := f.String("host", "", "host entry point that selects project skills")
	brief := f.String("brief", "", "confirmed RunBrief JSON file")
	workspace := f.String("workspace", "", "explicit worktree or checkout for assisted repository writes")
	allowExecution := f.Bool("allow-execution", false, "approve the selected workflow programs, arguments and supporting files")
	packageProfile := f.String("package-profile", "", "per-Run package profile")
	decisionPolicy := f.String("decision-policy", "", "attended or autonomous declared-decision policy; unnamed, the project's answers.decision_policy in extend.yaml, then attended")
	expectedCatalog := f.String("expected-decision-catalog-digest", "", "catalog digest returned by project questionnaire")
	expectedLaunch := f.String("expected-launch-digest", "", "review digest returned by project questionnaire --prepare")
	sourceRun := f.String("source-run", "", "completed partial or rejected Run to continue")
	workspaceCommit := f.String("workspace-commit", "", "full commit ID to claim a new tree at instead of taking over the source Run's tree (project continue)")
	allowDuplicateContinuation := f.Bool("allow-duplicate-continuation", false, "explicitly start another continuation while one from the same source Run is active")
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
	if (continuation || recovering) && *sourceRun == "" || !continuation && !recovering && *sourceRun != "" {
		return usageError("project continue requires --source-run; project start does not accept it")
	}
	if (continuation || recovering) && !prepare && *expectedLaunch == "" {
		return usageError("project continuation or recovery requires --expected-launch-digest from --prepare")
	}
	if *workspaceCommit != "" && !continuation {
		return usageError("--workspace-commit is only valid for project continue")
	}
	if *allowDuplicateContinuation && !continuation {
		return usageError("--allow-duplicate-continuation is only valid for project continue")
	}
	if *workspace != "" && *workspace != "worktree" && *workspace != "checkout" {
		return refusal("project_start_invalid_workspace", "use worktree or checkout")
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
	if (continuation || recovering) && !neutral {
		return refusal("project_continue_requires_profile_3", "continuation and recovery require Project profile /3")
	}
	if !neutral {
		if prepare || *expectedLaunch != "" {
			return refusal("project_questionnaire_prepare_requires_profile_3", "exact launch review requires an explicit Project profile /3 migration; legacy start remains supported")
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
		return refusal("project_start_unknown_launch", *launchID)
	}
	// An unnamed workspace mode is the launch's standing one, applied only
	// where the workflow needs a workspace at all: a standing choice is the
	// answer to the question, not a flag that a Git-less workflow refuses.
	standingWorkspace := launch.Workspace
	if _, err := projectCompileSkillsRoot(root, profile, *host); err != nil {
		return err
	}
	// The project's preflight runs before the tree is read for compilation
	// and before anything is taken; the read-only review does not run it.
	if !prepare {
		if err := runProjectLaunchPreflight(ctx, root, *launchID, launch.Preflight); err != nil {
			return err
		}
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
	if recovering {
		if len(inputs) != 0 || len(refFiles) != 0 || *workspace != "" {
			return refusal("recover_input_override", "recovery uses the exact source inputs and takes over the source Run's tree")
		}
	} else if err := projectStartInputs(*details, inputs, refFiles, !neutral); err != nil {
		return err
	}
	preflight, err := projectStartPreflight(root, profile, packageName, *packageProfile, *decisionPolicy, answers, runtimeAnswers)
	if err != nil {
		return err
	}
	if *expectedCatalog != "" && *expectedCatalog != preflight.Sheet.CatalogDigest {
		return refusal("project_start_stale_decision_catalog", "questionnaire differs from the current project catalog")
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
			return refusal("project_start_invalid_brief", "RunBrief requires explicit owner confirmation")
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
	var continuationReview *projectContinuationReview
	var recoveryRequest *prifly.RecoveryRequest
	if recovering {
		reader, err := prifly.Open(c.project, true)
		if err != nil {
			return err
		}
		request, carried, prepareErr := projectRecoverySource(ctx, reader, *sourceRun)
		closeErr := reader.Close()
		if prepareErr != nil {
			return prepareErr
		}
		if closeErr != nil {
			return closeErr
		}
		recoveryRequest = &request
		decisionPorts := map[string]bool{}
		for _, decision := range preflight.Catalog.Decisions {
			if decision.Destination.Kind == "launch_input" {
				decisionPorts[decision.Destination.Name] = true
			}
		}
		for _, input := range details.Inputs {
			if input.Configured {
				decisionPorts[input.Name] = true
			}
		}
		for name, value := range carried {
			if !decisionPorts[name] {
				inputValues[name] = value
			}
		}
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
	var recoveryPlan *prifly.RecoveryPlan
	needsWorkspace := !neutral
	if neutral {
		preflightEngine, err := prifly.Open(c.project, true)
		if err != nil {
			return err
		}
		launch, err := projectCompileLaunch(preflightEngine, compiled, workflowPath)
		if err == nil && continuation {
			var review projectContinuationReview
			var carried map[string]json.RawMessage
			review, carried, err = projectContinuationPrepare(ctx, preflightEngine, root, launch.plan, *sourceRun, *workspace, standingWorkspace, *workspaceCommit, *allowDuplicateContinuation)
			for name, value := range carried {
				if _, supplied := inputValues[name]; supplied || refs[name] != (prifly.ArtifactRef{}) {
					err = refusal("project_continue_input_override", name+" is carried from the source Run by the continuation this workflow declares")
					break
				}
				inputValues[name] = value
			}
			continuationReview = &review
		}
		if err == nil {
			execution, requirements, err = projectValidateLaunch(ctx, preflightEngine, root, compiled, launch, *host, *workspace, standingWorkspace, *allowExecution || recovering && prepare, inputValues, refs)
		}
		if err == nil {
			*workspace = requirements.WorkspaceMode
			if recovering {
				var planned prifly.RecoveryPlan
				planned, err = preflightEngine.PlanRecovery(ctx, *recoveryRequest, requirements.plan, requirements.definitions, requirements.resources)
				if err == nil {
					recoveryPlan = &planned
					recoveryRequest.ReviewDigest = planned.ReviewDigest
				}
			}
		}
		needsWorkspace = requirements.GitWorkspace
		closeErr := preflightEngine.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	// What this launch would seal as the meaning of each declared profile
	// name, resolved once: the summary shows it, the digest covers it and the
	// start seals the same table. Reading it twice would let the two disagree.
	reviewedProfiles, err := projectModelProfileTranslations(root, compiled.ModelProfiles, *host)
	if err != nil {
		return err
	}
	if recovering {
		reader, err := prifly.Open(c.project, true)
		if err != nil {
			return err
		}
		checkErr := reader.CheckRecoveryContext(ctx, *recoveryRequest, &preflight.Catalog, &preflight.Sheet, reviewedProfiles)
		closeErr := reader.Close()
		if checkErr != nil {
			return checkErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	var summary projectLaunchSummary
	if neutral {
		summary = projectLaunchSummary{SchemaVersion: "project-launch-summary/3", Repository: root, Authority: c.project, Launch: *launchID, Host: *host, WorkspaceMode: *workspace, Package: compiled.Package, AuthorPackage: compiled.AuthorPackage, BuildKey: compiled.BuildKey, InputDigests: map[string]string{}, InputRefs: refs, ConfigurationDigest: configurationDigest, DecisionSheet: preflight.Sheet, DecisionStates: projectDecisionStates(preflight), KnownQuestionsOnly: true, SessionLimits: requirements.sessionLimits, ModelProfiles: reviewedProfiles}
		summary.Continuation = continuationReview
		summary.AllowDuplicateContinuation = *allowDuplicateContinuation
		summary.Recovery = recoveryPlan
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
			return refusal("project_start_stale_launch", "sources, inputs, bindings or decisions changed; repeat project questionnaire --prepare and review the new summary")
		}
		if prepare {
			// Read after the digest, never into it.
			preview, err := prifly.Open(c.project, true)
			if err != nil {
				return err
			}
			capacity, held, err := preview.AdmissionCapacity(ctx)
			if err != nil {
				_ = preview.Close()
				return err
			}
			waiting, err := preview.AdmissionQueue(ctx)
			if err != nil {
				_ = preview.Close()
				return err
			}
			budget, err := projectRegistryBudgetAfter(ctx, preview, compiled)
			_ = preview.Close()
			if err != nil {
				return err
			}
			summary.Admission = projectAdmissionState(capacity, len(held), len(waiting))
			summary.RegistryBudget = &budget
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

	engine, err := c.openWithMonitor(c.project, false)
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
		// Re-read, not copied: the whole point of this check is that a
		// machine-local file may have changed since the summary was written,
		// and a copy of the reviewed value would agree with itself forever.
		currentProfiles, err := projectModelProfileTranslations(root, compiled.ModelProfiles, *host)
		if err != nil {
			return err
		}
		currentSummary := summary
		currentSummary.ConfigurationDigest, currentSummary.Execution, currentSummary.ReviewDigest = currentConfiguration, currentExecution, ""
		currentSummary.ModelProfiles = currentProfiles
		if continuation {
			currentReview, _, err := projectContinuationPrepare(ctx, engine, root, requirements.plan, *sourceRun, *workspace, standingWorkspace, *workspaceCommit, true)
			if err != nil {
				return err
			}
			currentSummary.Continuation = &currentReview
		}
		currentDigest, err := projectReviewDigest(currentSummary)
		if err != nil {
			return err
		}
		if currentDigest != summary.ReviewDigest {
			return refusal("project_start_stale_launch", "local execution configuration changed after the summary; prepare and review again")
		}
		// Keep the resolved reviewed path in the request. A later retarget of a
		// machine-local symlink must not select a different installed program.
		for index := range execution.Bindings {
			execution.Bindings[index].Config.Executable = summary.Execution[index].Executable
		}
	}
	if continuation && !*allowDuplicateContinuation {
		if err := projectCheckActiveContinuation(ctx, engine, *sourceRun); err != nil {
			return err
		}
	}
	var claim *prifly.WorktreeClaim
	createdClaim := false
	// A continuation or recovery takes the source Run's tree over as it was
	// left, uncommitted files included; the Run creation binds it. Only a
	// continuation told to start at another commit, or one whose source tree
	// is gone, claims anew.
	var handed *prifly.ContinuationClaim
	if recovering && recoveryPlan != nil {
		handed = recoveryPlan.Claim
	}
	if continuation && continuationReview.WorkspaceCommit == "" {
		handed = continuationReview.Source.Claim
	}
	if handed != nil {
		record, err := engine.Claims(ctx)
		if err != nil {
			return err
		}
		for _, held := range record.Claims {
			if held.ID == handed.ID && held.Generation == handed.Generation && held.Status == "active" {
				selected := held
				claim = &selected
			}
		}
		if claim == nil {
			return refusal("project_continue_stale_workspace", "the source Run's tree changed hands after the review; prepare again")
		}
		*workspace = handed.Mode
	} else if recovering && needsWorkspace {
		return refusal("recover_workspace_missing", "the source Run held no tree, and the target workflow needs one to run the failed stage again")
	} else if needsWorkspace {
		before, err := engine.Claims(ctx)
		if err != nil {
			return err
		}
		claimRequest := prifly.ClaimRequest{CommandID: *command + ":workspace", Repository: root, OwnerID: "project-launch:" + *command, WorkspaceMode: *workspace}
		if continuation {
			claimRequest.BaseRef = continuationReview.WorkspaceCommit
		}
		selected, err := engine.ClaimWorktree(ctx, claimRequest)
		if err != nil {
			return err
		}
		claim, createdClaim = &selected, true
		for _, previous := range before.Claims {
			if previous.ID == selected.ID {
				createdClaim = false
			}
		}
		if continuation && continuationReview.WorkspaceCommit != "" && selected.BaseCommit != continuationReview.WorkspaceCommit {
			if createdClaim {
				_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: selected.ID, Generation: selected.Generation})
			}
			return refusal("project_continue_stale_head", "claimed workspace HEAD differs from the reviewed commit")
		}
	}
	importedPackage := false
	if err := projectPackageAvailable(ctx, engine, compiled.Package, *command); err != nil {
		if !errors.Is(err, local.ErrNotFound) {
			if createdClaim {
				_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
			}
			return err
		}
		// An import the authority records and rejects returns no error, so the
		// launch used to walk on and refuse further down with "package not
		// installed", naming neither the real reason nor the claim it had
		// taken. The rejection is the reason, and it stops the launch here.
		imported, err := engine.ImportPackage(ctx, prifly.PackageImportRequest{CommandID: *command + ":import", Directory: packageDirectory, Reason: "declared project launch " + *launchID})
		if err == nil && imported.Receipt.Rejection != nil {
			err = recordedRejection(imported.Receipt.Rejection, imported.Receipt.ID)
		}
		if err != nil {
			if createdClaim {
				_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
			}
			return err
		}
		// The engine holds the imported package already; closing and reopening
		// the authority only to see it re-verified the store for nothing.
		importedPackage = true
	}
	workflowPath, err = projectInstalledWorkflowPath(ctx, engine, compiled.Package, workflowPath)
	if err != nil {
		// The three refusals above roll the claim back and this one did not, so
		// a start that failed here left the repository claimed by a launch that
		// never became a Run, and the next start was refused by it.
		if createdClaim {
			_, _ = engine.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
		}
		return err
	}
	startOptions := prifly.StartOptions{CommandID: *command, ProjectTitle: profile.Title, WorkflowFile: workflowPath, Brief: briefBytes, Inputs: inputPaths, InputRefs: refs, WorkspaceMode: *workspace}
	startOptions.WorkspaceClaim = claim
	if neutral {
		startOptions.SchemaVersion, startOptions.ExecutionBindings = "2", execution
		startOptions.Inputs, startOptions.InputValues = nil, inputValues
		if continuation {
			startOptions.Continuation = &continuationReview.Source
		}
		if recovering {
			startOptions.Recovery = recoveryRequest
		}
	}
	// Resolved before the summary and reused here, so what the Run seals is
	// exactly what the review covered. Sealed for the same reason the
	// machine's environment is: a setting edited after the start must be
	// visibly not part of this Run.
	if len(reviewedProfiles) != 0 {
		startOptions.ModelProfiles = reviewedProfiles
	}
	if preflight.Declared {
		startOptions.DecisionCatalog, startOptions.DecisionSheet = &preflight.Catalog, &preflight.Sheet
	}
	started, err := engine.Start(ctx, startOptions)
	if err != nil {
		// Under profile /2 a component keeps its authoring version, so two
		// package releases collide under one identity and the refusal reads as
		// the author's fault. The reader's own way out is the profile, and only
		// this layer knows which one the project is on.
		var drift *prifly.Fault
		if errors.As(err, &drift) && drift.Code == "definition_drift" && !neutral {
			err = &prifly.Fault{Code: drift.Code, Message: drift.Message + ". This project is on " + profile.SchemaVersion + ", where a component keeps its authoring version, so every package release meets the previous one under the same identity; " + projectVariantProfileVersion + " names each build by its own key and cannot collide", Cause: drift}
		}
		if importedPackage {
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
	if !recovering {
		if err := engine.Drive(ctx, started.Receipt.RunID); err != nil {
			return &prifly.Fault{Code: "project_start_incomplete", Message: fmt.Sprintf("run %s", started.Receipt.RunID), Cause: err}
		}
	}
	view, err := engine.View(ctx, started.Receipt.RunID)
	if err != nil {
		return err
	}
	// The Run creation bound the claim, and a handover moved its generation:
	// report the claim as it now stands, not the copy taken before the start.
	if claim != nil {
		record, err := engine.Claims(ctx)
		if err != nil {
			return err
		}
		for _, current := range record.Claims {
			if current.ID == claim.ID {
				fresh := current
				claim = &fresh
			}
		}
	}
	result := projectStartResult{SchemaVersion: "project-start/1", Repository: root, Launch: *launchID, Package: compiled.Package, PackageProfile: selectedProfile, Run: view, Workspace: claim}
	if recovering {
		result.SchemaVersion, result.Recovery = "project-recover/1", recoveryPlan
	}
	if claim != nil {
		path, err := engine.ClaimWorkspacePath(*claim)
		if err != nil {
			return err
		}
		result.WorkspacePath = path
	}
	if preflight.Declared {
		if !recovering {
			result.SchemaVersion = "project-start/2"
		}
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
			return refusal("project_start_unknown_input", port)
		}
		if _, ok := refs[port]; ok {
			return refusal("project_start_duplicate_input", port)
		}
	}
	for port := range refs {
		if _, ok := declared[port]; !ok {
			return refusal("project_start_unknown_input", port)
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
					return refusal("project_start_missing_input", port)
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
	if decisionPolicy != "" && decisionPolicy != "attended" && decisionPolicy != "autonomous" {
		return projectPreflight{}, refusal("project_start_invalid_decision_policy", "use attended or autonomous")
	}
	pkg, exists := profile.Packages[packageName]
	if !exists {
		return projectPreflight{}, refusal("project_compile_unknown_package", packageName)
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
	// An unnamed policy is the project's standing one, then attended: the
	// flag overrides extend.yaml, never the reverse.
	if decisionPolicy == "" {
		decisionPolicy = options.Answers.DecisionPolicy
		if decisionPolicy == "" {
			decisionPolicy = "attended"
		}
	}
	answers, err := projectParseDecisionAnswers(rawAnswers)
	if err != nil {
		return projectPreflight{}, err
	}
	answerSources := map[string]string{}
	// origin says where each answer was written, so a refusal sends the reader
	// to the flag or to extend.yaml -- whichever they must correct.
	origin := map[string]string{}
	for id := range answers {
		origin[id] = "--preflight-answer"
	}
	for id, value := range options.Answers.Preflight {
		if _, named := answers[id]; named {
			continue
		}
		answers[id], origin[id], answerSources[id] = value, projectExtensionAnswersSource+".preflight", "project_default"
	}
	definitions := map[string]prifly.DecisionDefinition{}
	for _, definition := range source.DecisionCatalog {
		definitions[definition.ID] = definition
	}
	for id, value := range answers {
		definition, exists := definitions[id]
		if !exists || definition.Phase != "preflight" {
			return projectPreflight{}, unknownDecision(definition, exists, id, origin[id])
		}
		if definition.Destination.Kind == "package_profile" {
			return projectPreflight{}, refusal("project_start_profile_is_selected_with_package_profile", id)
		}
		if err := projectValidateDecisionValue(definition, value); err != nil {
			return projectPreflight{}, refusal("project_start_invalid_decision_answer", id+answerOriginNote(origin[id])+": "+err.Error())
		}
	}
	runtime, err := projectParseDecisionAnswers(rawRuntimeAnswers)
	if err != nil {
		return projectPreflight{}, err
	}
	for id := range runtime {
		origin[id] = "--runtime-answer"
	}
	for id, value := range options.Answers.Runtime {
		if _, named := runtime[id]; named {
			continue
		}
		runtime[id], origin[id], answerSources[id] = value, projectExtensionAnswersSource+".runtime", "project_default"
	}
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
			if decisionPolicy != "autonomous" || !projectPolicyCanAnswer(definition) {
				if !complete {
					continue
				}
				return projectPreflight{}, usageError(projectMissingDecisionRefusal(source.DecisionCatalog, selected, answers, decisionPolicy))
			}
			value, err := flow.Canonical(definition.Recommendation)
			if err != nil || projectValidateDecisionValue(definition, value) != nil {
				return projectPreflight{}, refusal("project_start_invalid_decision_default", definition.ID)
			}
			answers[definition.ID], answerSources[definition.ID] = value, "autonomous_policy"
		}
	}
	// Conditions see the effective answers, including allowed policy choices
	// above. Validating raw runtime preanswers earlier rejects a legitimate
	// dependent answer merely because its predecessor was selected by policy.
	// A standing answer is the project's "if asked, this": a decision the
	// selected profile or an earlier answer keeps out of this Run is not asked,
	// so its standing answer is dropped rather than refused -- one extend.yaml
	// serves every profile. Dropping one can silence a condition on it, hence
	// the fixed point. A flag was typed for this Run and stays a refusal.
	for dropped := true; dropped; {
		dropped = false
		for _, definition := range source.DecisionCatalog {
			if strings.HasPrefix(origin[definition.ID], projectExtensionAnswersSource) && !projectDecisionApplies(definition, selected, answers) {
				delete(answers, definition.ID)
				delete(runtime, definition.ID)
				delete(answerSources, definition.ID)
				delete(origin, definition.ID)
				dropped = true
			}
		}
	}
	for id := range answers {
		if definition, exists := definitions[id]; !projectDecisionApplies(definition, selected, answers) {
			return projectPreflight{}, unknownDecision(definition, exists, id, origin[id])
		}
	}
	for id, value := range runtime {
		definition, exists := definitions[id]
		if !exists || definition.Phase != "runtime" || !projectDecisionApplies(definition, selected, answers) {
			return projectPreflight{}, unknownDecision(definition, exists, id, origin[id])
		}
		if err := projectValidateDecisionValue(definition, value); err != nil {
			return projectPreflight{}, refusal("project_start_invalid_decision_answer", id+answerOriginNote(origin[id])+": "+err.Error())
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
		return refusal("project_start_stale_decision_catalog", "sealed package differs from the reviewed questionnaire")
	}
	return nil
}

func projectParseDecisionAnswers(values []string) (map[string]json.RawMessage, error) {
	answers := make(map[string]json.RawMessage, len(values))
	for _, raw := range values {
		id, value, found := strings.Cut(raw, "=")
		if !found || !projectValueName.MatchString(id) || value == "" {
			return nil, refusal("project_start_invalid_decision_answer", "expected unique ID=JSON")
		}
		if _, duplicate := answers[id]; duplicate {
			return nil, refusal("project_start_invalid_decision_answer", "expected unique ID=JSON")
		}
		canonical, err := flow.Canonical([]byte(value))
		if err != nil {
			return nil, refusal("project_start_invalid_decision_answer", err.Error())
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

// unknownDecision says why a declared decision was refused on this flag. A
// decision that exists in the catalog but belongs to the other phase is the
// common case, and naming only the id sent the reader to the questionnaire to
// find out which of the two flags carries it.
// unknownDecision names the place the answer was written -- a flag or a block
// of extend.yaml -- and the place of the right phase in the same form.
func unknownDecision(definition prifly.DecisionDefinition, exists bool, id, origin string) error {
	if !exists {
		return refusal("project_start_unknown_decision", id+answerOriginNote(origin)+" is not declared by this package; project questionnaire lists the decisions it declares")
	}
	other := "--" + definition.Phase + "-answer"
	if strings.HasPrefix(origin, projectExtensionAnswersSource) {
		other = projectExtensionAnswersSource + "." + definition.Phase
	}
	if definition.Phase != "" && other != origin {
		return refusal("project_start_unknown_decision", id+" is a "+definition.Phase+" decision; pass it with "+other+", not "+origin)
	}
	return refusal("project_start_unknown_decision", id+answerOriginNote(origin)+" does not apply to this launch; project questionnaire reports its applicability for these arguments")
}

// answerOriginNote marks an answer that came from extend.yaml; a flag's origin
// is the command the reader just typed and needs no note.
func answerOriginNote(origin string) string {
	if strings.HasPrefix(origin, projectExtensionAnswersSource) {
		return " (from " + origin + ")"
	}
	return ""
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
				return "", refusal("project_start_ambiguous_package", "launch workflow belongs to multiple packages")
			}
			name = candidate
		}
	}
	if name == "" {
		return "", refusal("project_start_missing_package", "launch workflow has no declared package")
	}
	return name, nil
}

func (c *cli) compileDeclaredProjectPackage(ctx context.Context, root string, profile projectProfile, name, host, packageProfile, output string) (projectCompileResult, error) {
	pkg, exists := profile.Packages[name]
	if !exists {
		return projectCompileResult{}, refusal("project_compile_unknown_package", name)
	}
	skillsRoot, err := projectCompileSkillsRoot(root, profile, host)
	if err != nil {
		return projectCompileResult{}, err
	}
	if projectPathsOverlap(root, output) || projectPathsOverlap(c.project, output) {
		return projectCompileResult{}, refusal("project_compile_unsafe_output", "output must stay outside the repository and local authority")
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
	for alias, logical := range projectAllReferences(source, options) {
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

// projectPackageAvailable finds the declared edition among the trusted
// packages. A removed edition of the very bytes this launch declares is
// re-trusted rather than refused: removed means closed for new resolution,
// and this launch is a new, explicit resolution of exactly this edition -- the
// same bytes it would import were they absent. A start that failed after
// importing rolls its edition back to removed, and the next start of the same
// build used to answer "not trusted" for it, so the operator ran package
// restore by hand. Quarantined and revoked are judgments about the bytes and
// still refuse.
// projectRegistryBudgetAfter is the authority's definition budget as this
// launch would leave it: the current entries plus this edition's components
// when the edition is not trusted yet. Read after the review digest, never into
// it -- other launches move it.
func projectRegistryBudgetAfter(ctx context.Context, engine *prifly.Engine, compiled projectCompileResult) (prifly.RegistryBudget, error) {
	packages, err := engine.Packages(ctx)
	if err != nil {
		return prifly.RegistryBudget{}, err
	}
	for _, entry := range packages.Packages {
		if entry.Ref == compiled.Package && (entry.Status == "" || entry.Status == prifly.PackageTrusted) {
			return engine.RegistryBudget()
		}
	}
	return engine.RegistryBudgetAfter(len(compiled.Components))
}

func projectPackageAvailable(ctx context.Context, engine *prifly.Engine, ref flow.Ref, commandID string) error {
	packages, err := engine.Packages(ctx)
	if err != nil {
		return err
	}
	for _, entry := range packages.Packages {
		if entry.Ref.ID != ref.ID || entry.Ref.Version != ref.Version {
			continue
		}
		if entry.Ref != ref {
			return refusal("project_start_package_identity_conflict", "declared package ID and version already name different bytes")
		}
		if entry.Status == prifly.PackageRemoved {
			restored, err := engine.SetPackageStatus(ctx, prifly.PackageLifecycleRequest{CommandID: commandID + ":restore-package", ID: ref.ID, Version: ref.Version, Status: prifly.PackageTrusted, Reason: "project start declares this edition again"})
			if err != nil {
				return err
			}
			if restored.Receipt.Rejection != nil {
				return recordedRejection(restored.Receipt.Rejection, restored.Receipt.ID)
			}
			return nil
		}
		if entry.Status != "" && entry.Status != prifly.PackageTrusted {
			return refusal("project_start_package_unavailable", "declared package is "+entry.Status+", not trusted; package restore --id "+ref.ID+" --version "+ref.Version+" --reason TEXT re-trusts it")
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
	return "", refusal("project_start_package_not_installed", "the sealed package "+ref.ID+"@"+ref.Version+" was not found among trusted packages: "+reason+"; read package list")
}

// projectPolicyCanAnswer reports whether an autonomous policy may answer this
// decision on the caller's behalf: it has to be declared automatic, ordinary in
// sensitivity, and carry a recommendation to use.
func projectPolicyCanAnswer(definition prifly.DecisionDefinition) bool {
	return definition.Automatic && definition.Sensitivity == "ordinary" && len(definition.Recommendation) != 0
}

// projectMissingDecisionRefusal names every unanswered decision, not the first
// one found, and says which of the two exits applies to each. Until 0.13.12 it
// named one id and nothing else: a caller with six unanswered decisions learned
// them one refusal at a time, and was told neither that the decision was
// declared automatic nor that --decision-policy autonomous would answer it. The
// two exits are not interchangeable -- a decision that is sensitive, not
// automatic, or carries no recommendation is never answered by policy, and
// advising the policy there would send the reader in a circle.
func projectMissingDecisionRefusal(catalog []prifly.DecisionDefinition, selected string, answers map[string]json.RawMessage, decisionPolicy string) string {
	byPolicy, byHand := []string{}, []string{}
	for _, definition := range catalog {
		if definition.Phase != "preflight" || !projectDecisionApplies(definition, selected, answers) || !definition.Required || definition.Destination.Kind == "package_profile" {
			continue
		}
		if _, answered := answers[definition.ID]; answered {
			continue
		}
		if projectPolicyCanAnswer(definition) {
			byPolicy = append(byPolicy, definition.ID)
		} else {
			byHand = append(byHand, definition.ID)
		}
	}
	count := len(byPolicy) + len(byHand)
	subject := " declared decisions are unanswered."
	if count == 1 {
		subject = " declared decision is unanswered."
	}
	message := "project_start_missing_decision: " + strconv.Itoa(count) + subject
	if len(byPolicy) != 0 && decisionPolicy != "autonomous" {
		message += " " + strings.Join(byPolicy, ", ") + " are declared automatic and carry a recommendation, so --decision-policy autonomous answers them without asking"
		if len(byHand) == 0 {
			return message + "; answering each with --preflight-answer ID=JSON does the same explicitly"
		}
		message += "."
	}
	if len(byHand) != 0 {
		message += " " + strings.Join(byHand, ", ") + " must be answered with --preflight-answer ID=JSON: no policy answers them, because they are not declared automatic, not of ordinary sensitivity, or carry no recommendation"
	}
	return message + ". project questionnaire --repository DIR --launch ID lists every declared question with its choices"
}
