package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectProfileSource = `schema_version: prifly-project-profile/2
hosts:
  codex-cli: .codex/skills
  codex-app: .agents/skills
  claude-code: .claude/skills
packages: {}
launches: {}
`

const projectLocalExample = `# Copy this file to local.yaml for machine-only overrides.
# local.yaml is ignored by Git and must not contain shared workflow rules.
# authority_root: /absolute/path/to/your/local/Pri-Fly/state
# prifly_executable: /absolute/path/to/prifly
`

const projectRunnerSkillTemplate = `---
name: prifly-run
description: Start and host one declared Pri-Fly project workflow.
---

# Pri-Fly project host

You are the host of the Pri-Fly run. Do not ask the developer to operate
Pri-Fly's internal session protocol: do that work yourself and ask the
developer only at the workflow's declared decision points.

1. Read .prifly/local.yaml to find authority_root and prifly_executable. Use
   that exact executable as PRIFLY_BIN; do not assume prifly is on PATH. List
   available launches with PRIFLY_BIN project workflows --repository "$PWD"
   --json. If the developer did not name one exact launch ID, show this list
   and wait for a choice. Never infer a default from the task wording.
2. This host is {{host}}. Do not read skills from another host directory or
   infer host from directories on disk. Before starting a selected launch, read
   ` + "`PRIFLY_BIN project questionnaire --repository \"$PWD\" --launch ID --json`" + `.
   Run one continuous questionnaire: first worktree or checkout, then package
   profile when listed, then only preflight decisions whose ` + "`when`" + ` clause matches
   that profile and the preceding typed answers, then attended or autonomous policy, and finally a summary and
   explicit confirmation. A supplied exact choice may prefill its question;
   never infer a missing one. Autonomous may use only entries marked automatic;
   it does not authorize an undeclared or scope-changing choice. Until the
   confirmation, do not compile, claim a repository, register a package or
   create a Run.
3. For a selected workflow, read all listed inputs, require every required
   value and ask only for values that are missing; optional values are passed
   only when the developer supplies them. After the one confirmation invoke
   exactly
   ` + "`PRIFLY_BIN --project \"$authority_root\" project start --repository \"$PWD\" --launch ID --host {{host}} --brief FILE --workspace worktree|checkout`" + `
   plus the selected ` + "`--package-profile`" + `, ` + "`--decision-policy`" + ` and repeatable
   ` + "`--preflight-answer ID=JSON`" + ` values, and the questionnaire's
   ` + "`--expected-decision-catalog-digest`" + `, with normal ` + "`--input`" + ` or ` + "`--input-ref`" + ` bindings. This command seals the
   declared package and drives only to the first honest handoff; it does not
   start a model, provider or background agent.
4. For every outstanding session task, read its pinned skill and sealed
   context from ` + "`workspace`" + `. Treat ` + "`decision_sheet`" + ` and
   ` + "`decision_context`" + ` as exact choices already made by the developer:
   carry applicable values into the skill invocation and never ask the same
   declared question again. A package profile is part of the sheet; follow the
   pinned package instructions for its argument form. For a workspace-write task, make repository
   changes only in ` + "`repository_workspace`" + `; never put context or outputs there.
   Submit the typed result, then drive the Run. When two reviewer tasks
   are present, handle both; use separate agent sessions only when the host
   platform actually provides them.
5. Continue until the Run completes. When ` + "`run next RUN_ID`" + ` says
   ` + "`waiting_decision`" + `, read ` + "`run decisions RUN_ID`" + `, ask the one declared
   question, then call ` + "`run decision RUN_ID answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON`" + `
   with the exact values from that read, and resume the same session task. Never treat a skill's native question as a Pri-Fly decision
   unless it arrived through that protocol. Other questions declared by the
   workflow, such as accepted improvements or another bounded batch, are for
   the developer; successful exits are silent. A gate's suggested next action is never an
   automatic command. TaskInput is a local, provider-neutral intake boundary,
   not a GitLab/GitHub/Jira adapter: do not claim automatic tracker reading or
   edit the external issue unless the developer
   separately asks for that action.
`

// projectPreviousRunnerSkillTemplate is the exact runner emitted before the
// decision questionnaire. It remains here solely so update can recognize it;
// init never writes it again.
const projectPreviousRunnerSkillTemplate = `---
name: prifly-run
description: Start and host one declared Pri-Fly project workflow.
---

# Pri-Fly project host

You are the host of the Pri-Fly run. Do not ask the developer to operate
Pri-Fly's internal session protocol: do that work yourself and ask the
developer only at the workflow's declared decision points.

1. Read .prifly/local.yaml to find authority_root and prifly_executable. Use
   that exact executable as PRIFLY_BIN; do not assume prifly is on PATH. List
   available launches with PRIFLY_BIN project workflows --repository "$PWD"
   --json. If the developer did not name one exact launch ID, show this list
   and wait for a choice. Never infer a default from the task wording.
2. This host is {{host}}. Do not read skills from another host directory or
   infer host from directories on disk. Before starting any selected launch,
   always ask the developer one question: worktree or checkout? If the request
   already states one of those exact words, use it. Otherwise wait: do not
   compile, claim a repository, register a package or create a Run.
3. For a selected workflow, read all listed inputs, require every required
   value and ask only for values that are missing; optional values are passed
   only when the developer supplies them. Then invoke exactly
   ` + "`PRIFLY_BIN --project \"$authority_root\" project start --repository \"$PWD\" --launch ID --host {{host}} --brief FILE --workspace worktree|checkout`" + `
   with normal ` + "`--input`" + ` or ` + "`--input-ref`" + ` bindings. This command seals the
   declared package and drives only to the first honest handoff; it does not
   start a model, provider or background agent.
4. For every outstanding session task, read its pinned skill and sealed
   context from ` + "`workspace`" + `. For a workspace-write task, make repository
   changes only in ` + "`repository_workspace`" + `; never put context or outputs there.
   Submit the typed result, then drive the Run. When two reviewer tasks
   are present, handle both; use separate agent sessions only when the host
   platform actually provides them.
5. Continue until the Run completes. Questions declared by the workflow, such
   as accepted improvements or another bounded batch, are for the developer;
   successful exits are silent. A gate's suggested next action is never an
   automatic command. TaskInput is a local, provider-neutral intake boundary,
   not a GitLab/GitHub/Jira adapter: do not claim automatic tracker reading or
   edit the external issue unless the developer
   separately asks for that action.
`

const projectCodexQuestionInstructions = `

## Native developer decisions

This section controls how you present every developer question above. For a
known finite decision, use ` + "`request_user_input`" + ` when that tool is available.
Do not imitate buttons in Markdown. Ask one dependent decision at a time,
offer only mutually exclusive real options, identify the recommended option
and describe the consequence of each option. In particular, use it to select
a launch, worktree or checkout, package profile, declared preflight choices
and attended or autonomous policy before any compilation, claim, package
registration or Run creation.

For a known finite set larger than the tool accepts, present deterministic
pages with native navigation and refusal; do not hide an option or promote a
recommendation to a default. If the native tool is unavailable, ask one
explicit text question and wait without mutation. RunBrief text, a file path
and arbitrary JSON or text input remain ordinary free-form input.
`

const projectClaudeQuestionInstructions = `

## Native developer decisions

This section controls how you present every developer question above. For a
known finite decision, use ` + "`AskUserQuestion`" + ` when that tool is available.
Do not imitate buttons in Markdown. Ask one dependent decision at a time,
offer only mutually exclusive real options, identify the recommended option
and describe the consequence of each option. In particular, use it to select
a launch, worktree or checkout, package profile, declared preflight choices
and attended or autonomous policy before any compilation, claim, package
registration or Run creation.

For a known finite set larger than the tool accepts, present deterministic
pages with native navigation and refusal; do not hide an option or promote a
recommendation to a default. If the native tool is unavailable, ask one
explicit text question and wait without mutation. RunBrief text, a file path
and arbitrary JSON or text input remain ordinary free-form input.
`

const projectDecisionBridgeInstructions = `

## Compatible runtime decisions

Only a pinned package adapter that names one exact declared runtime decision
may turn that question into a Pri-Fly decision. When it instructs you to do
so, read the current session task and call:

` + "`PRIFLY_BIN --project \"$authority_root\" run decision RUN_ID request --attempt ATTEMPT_ID --envelope-digest ENVELOPE_DIGEST --decision ID --expected-run-version RUN_VERSION`" + `.

Those values come verbatim from that task. Then read ` + "`run decisions RUN_ID`" + `.
If the Run waits, ask the declared question and use the normal
` + "`run decision RUN_ID answer ...`" + ` flow; if an autonomous policy applies the
recommendation, re-read the redelivered task. Do not send a request for a raw
native skill question unless the pinned adapter supplied its exact declared ID.
`

const projectCatalogInstructions = `

## Finding and installing a workflow

When the developer explicitly asks to find or install a workflow, run
` + "`PRIFLY_BIN project workflows search [QUERY] --json`" + ` (add
` + "`--catalog URL`" + ` only when the developer names another catalog) and
present its categories and entries as one native finite question. After an
explicit choice invoke exactly
` + "`PRIFLY_BIN project workflows add NAME --repository \"$PWD\" --json`" + `
(` + "`add URL`" + ` for a repository the developer named), then ask the
developer to review and commit the .prifly changes. Never install without a
choice, never treat a search result as a request to install, and never start
a Run as part of installing: nothing is sealed, trusted or executed until
project start.
`

const projectPreviousCodexQuestionInstructions = `

## Native developer decisions

This section controls how you present every developer question above. For a
known finite decision, use ` + "`request_user_input`" + ` when that tool is available.
Do not imitate buttons in Markdown. Ask one dependent decision at a time,
offer only mutually exclusive real options, identify the recommended option
and describe the consequence of each option. In particular, use it to select
a launch and to select worktree or checkout before any compilation, claim,
package registration or Run creation.

For a known finite set larger than the tool accepts, present deterministic
pages with native navigation and refusal; do not hide an option or promote a
recommendation to a default. If the native tool is unavailable, ask one
explicit text question and wait without mutation. RunBrief text, a file path
and arbitrary JSON or text input remain ordinary free-form input.
`

const projectPreviousClaudeQuestionInstructions = `

## Native developer decisions

This section controls how you present every developer question above. For a
known finite decision, use ` + "`AskUserQuestion`" + ` when that tool is available.
Do not imitate buttons in Markdown. Ask one dependent decision at a time,
offer only mutually exclusive real options, identify the recommended option
and describe the consequence of each option. In particular, use it to select
a launch and to select worktree or checkout before any compilation, claim,
package registration or Run creation.

For a known finite set larger than the tool accepts, present deterministic
pages with native navigation and refusal; do not hide an option or promote a
recommendation to a default. If the native tool is unavailable, ask one
explicit text question and wait without mutation. RunBrief text, a file path
and arbitrary JSON or text input remain ordinary free-form input.
`

type projectProfileInit struct {
	SchemaVersion string `json:"schema_version"`
	Repository    string `json:"repository"`
	Profile       string `json:"profile"`
	AuthorityRoot string `json:"authority_root"`
}

type projectProfile struct {
	SchemaVersion   string
	HostSkillsRoots map[string]string
	Packages        map[string]projectPackage
	Launches        map[string]projectLaunch
}

type projectHost struct {
	ID         string
	SkillsRoot string
}

var projectHosts = []projectHost{
	{ID: "codex-cli", SkillsRoot: ".codex/skills"},
	{ID: "codex-app", SkillsRoot: ".agents/skills"},
	{ID: "claude-code", SkillsRoot: ".claude/skills"},
}

type projectPackage struct {
	Source string
	Origin *projectWorkflowOrigin
}

type projectLaunch struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Workflow    string `json:"workflow"`
}

type projectWorkflowList struct {
	SchemaVersion string                `json:"schema_version"`
	Repository    string                `json:"repository"`
	Launches      []projectLaunchDetail `json:"launches"`
}

type projectLaunchDetail struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Kind        string               `json:"kind"`
	Source      string               `json:"source"`
	Inputs      []projectLaunchInput `json:"inputs"`
}

type projectLaunchInput struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Format      string `json:"format"`
	Description string `json:"description,omitempty"`
}

type projectQuestionnaireProfile struct {
	ID      string `json:"id"`
	Default bool   `json:"default"`
}

// projectQuestionnaire is a read-only preflight description. Its decisions
// remain source declarations until project start seals an exact Run choice.
type projectQuestionnaire struct {
	SchemaVersion string                        `json:"schema_version"`
	Repository    string                        `json:"repository"`
	Package       flow.Ref                      `json:"package"`
	Profiles      []projectQuestionnaireProfile `json:"profiles"`
	Preflight     []prifly.DecisionDefinition   `json:"preflight"`
	CatalogDigest string                        `json:"catalog_digest"`
}

func (c *cli) projectCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("project requires init, workflows, questionnaire, compile, start, extend or runners")
	}
	switch args[0] {
	case "init":
		return c.projectInit(ctx, args[1:])
	case "workflows":
		return c.projectWorkflows(ctx, args[1:])
	case "questionnaire":
		return c.projectQuestionnaire(ctx, args[1:])
	case "compile":
		return c.projectCompile(ctx, args[1:])
	case "start":
		return c.projectStart(ctx, args[1:])
	case "extend":
		return c.projectExtend(ctx, args[1:])
	case "runners":
		return c.projectRunners(ctx, args[1:])
	default:
		return usageError("project requires init, workflows, questionnaire, compile, start, extend or runners")
	}
}

func (c *cli) projectRunners(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "update" {
		return usageError("project runners requires update --repository DIR")
	}
	f := flags("project runners update")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	if _, err := readProjectProfile(root); err != nil {
		return err
	}
	updated, err := updateProjectRunners(root)
	if err != nil {
		return err
	}
	return c.emit(map[string]any{"schema_version": "project-runners-update/1", "repository": root, "updated_hosts": updated})
}

func (c *cli) projectQuestionnaire(ctx context.Context, args []string) error {
	f := flags("project questionnaire")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	name := f.String("package", "", "named package from project.yaml")
	launchID := f.String("launch", "", "declared launch ID from project.yaml")
	if err := parse(f, args); err != nil {
		return err
	}
	if (*name == "" && *launchID == "") || (*name != "" && *launchID != "") {
		return usageError("project questionnaire requires exactly one of --package or --launch")
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	if *launchID != "" {
		launch, exists := profile.Launches[*launchID]
		if !exists || launch.Kind != "workflow" {
			return usageError("project_start_unknown_launch: " + *launchID)
		}
		*name, err = profile.packageForLaunch(root, launch)
		if err != nil {
			return err
		}
	}
	pkg, exists := profile.Packages[*name]
	if !exists {
		return usageError("project_compile_unknown_package: " + *name)
	}
	folder, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return err
	}
	source, err := readProjectWorkflowFolder(root, folder)
	if err != nil {
		return err
	}
	profiles := make([]projectQuestionnaireProfile, 0, len(source.Profiles))
	for id := range source.Profiles {
		profiles = append(profiles, projectQuestionnaireProfile{ID: id, Default: id == source.DefaultProfile})
	}
	sort.Slice(profiles, func(left, right int) bool { return profiles[left].ID < profiles[right].ID })
	preflight := make([]prifly.DecisionDefinition, 0, len(source.DecisionCatalog))
	for _, definition := range source.DecisionCatalog {
		if definition.Phase == "preflight" {
			preflight = append(preflight, definition)
		}
	}
	catalog := prifly.DecisionCatalog{SchemaVersion: prifly.DecisionCatalogVersion, Decisions: source.DecisionCatalog}
	digest, err := prifly.DecisionCatalogDigest(catalog)
	if err != nil {
		return err
	}
	return c.emit(projectQuestionnaire{SchemaVersion: "project-questionnaire/2", Repository: root, Package: flow.Ref{ID: source.ID, Version: source.Version}, Profiles: profiles, Preflight: preflight, CatalogDigest: digest})
}

func (c *cli) projectInit(ctx context.Context, args []string) error {
	f := flags("project init")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	stateRoot := f.String("state-root", "", "machine-local authority root outside the repository")
	if err := parse(f, args); err != nil {
		return err
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	if *stateRoot == "" {
		*stateRoot, err = defaultProjectAuthorityRoot(root)
		if err != nil {
			return err
		}
	}
	authority, err := canonicalProjectPath(*stateRoot)
	if err != nil {
		return err
	}
	executable, err := projectExecutable()
	if err != nil {
		return err
	}
	if projectPathsOverlap(root, authority) {
		return usageError("unsafe_authority_root: local authority data must be outside the repository")
	}
	profile := filepath.Join(root, ".prifly")
	existing, err := existingProjectProfile(root, profile)
	if err != nil {
		return err
	}
	if !existing {
		if err := checkProjectRunners(root); err != nil {
			return err
		}
	}
	if err := ensureProjectAuthority(authority); err != nil {
		return err
	}
	if existing {
		if err := writeProjectLocal(profile, authority, executable); err != nil {
			return err
		}
	} else {
		if err := writeProjectProfile(profile, authority, executable); err != nil {
			return err
		}
		if err := writeProjectRunners(root); err != nil {
			return err
		}
	}
	return c.emit(projectProfileInit{SchemaVersion: "project-profile-init/1", Repository: root, Profile: profile, AuthorityRoot: authority})
}

func (c *cli) projectWorkflows(ctx context.Context, args []string) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return c.projectWorkflowsCommand(ctx, args)
	}
	f := flags("project workflows")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	if err := parse(f, args); err != nil {
		return err
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	launches, err := profile.launchDetails(root)
	if err != nil {
		return err
	}
	return c.emit(projectWorkflowList{SchemaVersion: "project-workflows/1", Repository: root, Launches: launches})
}

type projectWorkflowExtension struct {
	ID       string
	Workflow string
	From     string
	To       string
	Step     string
	On       map[string]string
}

type projectWorkflowOptions struct {
	Extensions []projectWorkflowExtension
	Settings   map[string]map[string]any
	Exclude    []string
	Profile    string
}

func (c *cli) projectExtend(_ context.Context, args []string) error {
	f := flags("project extend")
	workflowPath := f.String("workflow", "", "compiled YAML workflow source")
	workflowID := f.String("workflow-id", "", "exact workflow ID named by extensions")
	extensionsPath := f.String("extensions", "", "YAML extension list")
	outputPath := f.String("output", "", "compiled workflow output")
	stepRefs := stringsFlag{}
	stepSources := stringsFlag{}
	f.Var(&stepRefs, "step-ref", "exact step reference NAME=JSON")
	f.Var(&stepSources, "step-source", "compiled no-input step YAML NAME=FILE")
	if err := parse(f, args); err != nil {
		return err
	}
	if *workflowPath == "" || *workflowID == "" || *extensionsPath == "" || *outputPath == "" {
		return usageError("project extend requires --workflow, --workflow-id, --extensions and --output")
	}
	workflowBytes, err := readFile(*workflowPath, prifly.MaxDefinitionBytes)
	if err != nil {
		return err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(*workflowPath)), ".")
	machine, err := flow.WorkflowJSONBytes(workflowBytes, format)
	if err != nil {
		return usageError("project_extension_invalid_workflow: " + err.Error())
	}
	var workflow map[string]any
	if err := json.Unmarshal(machine, &workflow); err != nil {
		return usageError("project_extension_invalid_workflow: workflow is not an object")
	}
	if id, _ := workflow["id"].(string); id != *workflowID {
		return usageError("project_extension_invalid_workflow: --workflow-id does not match source")
	}
	extensionBytes, err := readFile(*extensionsPath, flow.MaxDocumentBytes)
	if err != nil {
		return err
	}
	extensions, err := parseProjectExtensions(extensionBytes)
	if err != nil {
		return err
	}
	refs, err := parseProjectExtensionRefs(stepRefs)
	if err != nil {
		return err
	}
	sources, err := parseProjectExtensionSources(stepSources)
	if err != nil {
		return err
	}
	for _, extension := range extensions {
		if extension.Workflow != projectExtensionWorkflowName(*workflowID) {
			return usageError("project_extension_unknown_workflow: " + extension.Workflow)
		}
		if _, exists := refs[extension.Step]; !exists {
			return usageError("project_extension_unknown_step: " + extension.Step)
		}
		if source, exists := sources[extension.Step]; !exists {
			return usageError("project_extension_missing_step_source: " + extension.Step)
		} else if err := projectExtensionNoInputStep(source); err != nil {
			return err
		}
		if err := applyProjectExtension(workflow, extension, refs[extension.Step]); err != nil {
			return err
		}
	}
	output, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(*outputPath, append(output, '\n'), 0644); err != nil {
		return err
	}
	return c.emit(map[string]any{"schema_version": "project-extension/1", "workflow_id": *workflowID, "extensions": len(extensions)})
}

func parseProjectExtensions(data []byte) ([]projectWorkflowExtension, error) {
	options, err := parseProjectWorkflowOptions(data)
	if err != nil {
		return nil, err
	}
	if len(options.Settings) != 0 || len(options.Exclude) != 0 || options.Profile != "" {
		return nil, usageError("project_extension_invalid: settings, exclude and profile require project compile")
	}
	return options.Extensions, nil
}

func parseProjectWorkflowOptions(data []byte) (projectWorkflowOptions, error) {
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return projectWorkflowOptions{}, usageError("project_extension_invalid: " + err.Error())
	}
	root, ok := value.(map[string]any)
	if !ok || len(root) == 0 {
		return projectWorkflowOptions{}, usageError("project_extension_invalid: extend.yaml must be a non-empty object")
	}
	for key := range root {
		switch key {
		case "extensions", "settings", "exclude", "profile":
		default:
			return projectWorkflowOptions{}, usageError("project_extension_invalid: unknown field " + key)
		}
	}
	result := projectWorkflowOptions{Extensions: []projectWorkflowExtension{}, Settings: map[string]map[string]any{}, Exclude: []string{}}
	if raw, exists := root["profile"]; exists {
		profile, ok := raw.(string)
		if !ok || !projectValueName.MatchString(profile) {
			return projectWorkflowOptions{}, usageError("project_extension_invalid: profile must be a valid name")
		}
		result.Profile = profile
	}
	if raw, exists := root["settings"]; exists {
		settings, ok := raw.(map[string]any)
		if !ok {
			return projectWorkflowOptions{}, usageError("project_extension_invalid: settings must be an object")
		}
		for workflow, rawInputs := range settings {
			if !projectValueName.MatchString(workflow) {
				return projectWorkflowOptions{}, usageError("project_extension_invalid: settings workflow must be a valid name")
			}
			inputs, ok := rawInputs.(map[string]any)
			if !ok || len(inputs) == 0 {
				return projectWorkflowOptions{}, usageError("project_extension_invalid: settings " + workflow + " must be a non-empty object")
			}
			result.Settings[workflow] = inputs
		}
	}
	if raw, exists := root["exclude"]; exists {
		items, ok := raw.([]any)
		if !ok {
			return projectWorkflowOptions{}, usageError("project_extension_invalid: exclude must be a list")
		}
		seen := map[string]bool{}
		for index, item := range items {
			feature, ok := item.(string)
			if !ok || !projectValueName.MatchString(feature) || seen[feature] {
				return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: exclude/%d must be a unique valid name", index))
			}
			seen[feature] = true
			result.Exclude = append(result.Exclude, feature)
		}
	}
	raw, exists := root["extensions"]
	if !exists {
		return result, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return projectWorkflowOptions{}, usageError("project_extension_invalid: extensions must be a list")
	}
	extensions := make([]projectWorkflowExtension, 0, len(items))
	seen := map[string]bool{}
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d must be an object", index))
		}
		for key := range object {
			switch key {
			case "id", "workflow", "between", "step", "on":
			default:
				return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d has unknown field %s", index, key))
			}
		}
		text := func(name string) (string, bool) { value, ok := object[name].(string); return value, ok && value != "" }
		extension := projectWorkflowExtension{}
		var okID, okWorkflow, okStep bool
		if extension.ID, okID = text("id"); !okID || seen[extension.ID] {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d id must be unique and non-empty", index))
		}
		seen[extension.ID] = true
		if extension.Workflow, okWorkflow = text("workflow"); !okWorkflow {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d workflow must be non-empty", index))
		}
		if extension.Step, okStep = text("step"); !okStep {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d step must be non-empty", index))
		}
		between, ok := object["between"].(map[string]any)
		if !ok || len(between) != 2 {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d between requires from and to", index))
		}
		if extension.From, ok = between["from"].(string); !ok || extension.From == "" {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d between.from must be non-empty", index))
		}
		if extension.To, ok = between["to"].(string); !ok || extension.To == "" {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d between.to must be non-empty", index))
		}
		routes, ok := object["on"].(map[string]any)
		if !ok || len(routes) == 0 {
			return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d on must be a non-empty object", index))
		}
		extension.On = make(map[string]string, len(routes))
		for verdict, target := range routes {
			target, ok := target.(string)
			if verdict == "" || !ok || target == "" {
				return projectWorkflowOptions{}, usageError(fmt.Sprintf("project_extension_invalid: extensions/%d on routes must be non-empty strings", index))
			}
			extension.On[verdict] = target
		}
		extensions = append(extensions, extension)
	}
	result.Extensions = extensions
	return result, nil
}

func parseProjectExtensionRefs(values []string) (map[string]any, error) {
	refs := make(map[string]any, len(values))
	for _, value := range values {
		name, raw, ok := strings.Cut(value, "=")
		if !ok || name == "" || raw == "" || refs[name] != nil {
			return nil, usageError("project_extension_invalid_step_ref: expected unique NAME=JSON")
		}
		var ref any
		if err := json.Unmarshal([]byte(raw), &ref); err != nil {
			return nil, usageError("project_extension_invalid_step_ref: " + err.Error())
		}
		data, _ := json.Marshal(ref)
		if err := flow.ValidateProtocol("ImmutableRef", data); err != nil {
			return nil, usageError("project_extension_invalid_step_ref: " + err.Error())
		}
		refs[name] = ref
	}
	return refs, nil
}

func parseProjectExtensionSources(values []string) (map[string]string, error) {
	sources := make(map[string]string, len(values))
	for _, value := range values {
		name, source, ok := strings.Cut(value, "=")
		if !ok || name == "" || source == "" || sources[name] != "" {
			return nil, usageError("project_extension_invalid_step_source: expected unique NAME=FILE")
		}
		sources[name] = source
	}
	return sources, nil
}

func projectExtensionNoInputStep(path string) error {
	data, err := readFile(path, prifly.MaxDefinitionBytes)
	if err != nil {
		return err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	value, err := flow.Parse(data, format)
	if err != nil {
		return usageError("project_extension_invalid_step_source: " + err.Error())
	}
	machine, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var step flow.StepDefinition
	if err := json.Unmarshal(machine, &step); err != nil {
		return usageError("project_extension_invalid_step_source: step is not a StepDefinition")
	}
	if len(step.Inputs) != 0 {
		return usageError("project_extension_requires_full_yaml: a simple extension step cannot declare inputs")
	}
	return nil
}

func projectExtensionWorkflowName(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

func applyProjectExtension(workflow map[string]any, extension projectWorkflowExtension, ref any) error {
	definition, ok := workflow["definition"].(map[string]any)
	if !ok {
		return usageError("project_extension_invalid_workflow: definition must be an object")
	}
	stages, ok := definition["stages"].(map[string]any)
	if !ok {
		return usageError("project_extension_invalid_workflow: definition.stages must be an object")
	}
	if _, exists := stages[extension.Step]; exists {
		return usageError("project_extension_stage_conflict: " + extension.Step)
	}
	rawFrom, exists := stages[extension.From]
	if !exists {
		return usageError("project_extension_unknown_stage: " + extension.From)
	}
	if _, exists := stages[extension.To]; !exists {
		return usageError("project_extension_unknown_stage: " + extension.To)
	}
	from, ok := rawFrom.(map[string]any)
	if !ok {
		return usageError("project_extension_invalid_workflow: source stage must be an object")
	}
	type route struct{ group, verdict string }
	routes := []route{}
	for _, group := range []string{"on", "on_complete"} {
		values, ok := from[group].(map[string]any)
		if !ok {
			continue
		}
		for verdict, target := range values {
			if target == extension.To {
				routes = append(routes, route{group, verdict})
			}
		}
	}
	for _, field := range []string{"on_limit", "on_error", "on_unknown", "default"} {
		if from[field] == extension.To {
			routes = append(routes, route{field, ""})
		}
	}
	if len(routes) == 0 {
		return usageError("project_extension_route_missing: " + extension.From + " → " + extension.To)
	}
	if len(routes) != 1 {
		return usageError("project_extension_route_ambiguous: " + extension.From + " → " + extension.To)
	}
	for _, target := range extension.On {
		if _, exists := stages[target]; !exists {
			return usageError("project_extension_unknown_stage: " + target)
		}
	}
	selected := routes[0]
	if selected.verdict == "" {
		from[selected.group] = extension.Step
	} else {
		from[selected.group].(map[string]any)[selected.verdict] = extension.Step
	}
	stages[extension.Step] = map[string]any{"kind": "step", "step_ref": ref, "input_bindings": map[string]any{}, "on": extension.On}
	return nil
}

func readProjectProfile(root string) (projectProfile, error) {
	profilePath := filepath.Join(root, ".prifly", "project.yaml")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return projectProfile{}, usageError("project_profile_missing: initialize the repository with project init first")
		}
		return projectProfile{}, err
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return projectProfile{}, usageError("project_profile_invalid: " + err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok {
		return projectProfile{}, usageError("project_profile_invalid: profile must be an object")
	}
	schema, _ := object["schema_version"].(string)
	if schema != "prifly-project-profile/2" {
		return projectProfile{}, usageError("project_profile_invalid: schema_version must be prifly-project-profile/2")
	}
	for key := range object {
		switch key {
		case "schema_version", "hosts", "packages", "launches":
		default:
			return projectProfile{}, usageError("project_profile_invalid: unknown field " + key)
		}
	}
	rawLaunches, exists := object["launches"]
	if !exists {
		return projectProfile{}, usageError("project_profile_invalid: launches is required")
	}
	launches, ok := rawLaunches.(map[string]any)
	if !ok {
		return projectProfile{}, usageError("project_profile_invalid: launches must be an object")
	}
	profile := projectProfile{SchemaVersion: schema, HostSkillsRoots: make(map[string]string, len(projectHosts)), Launches: make(map[string]projectLaunch, len(launches))}
	rawHosts, exists := object["hosts"]
	if !exists {
		return projectProfile{}, usageError("project_profile_invalid: hosts is required by prifly-project-profile/2")
	}
	hosts, ok := rawHosts.(map[string]any)
	if !ok || len(hosts) != len(projectHosts) {
		return projectProfile{}, usageError("project_profile_invalid: hosts must declare every supported host")
	}
	for _, host := range projectHosts {
		root, ok := hosts[host.ID].(string)
		if !ok || root != host.SkillsRoot {
			return projectProfile{}, usageError("project_profile_invalid: host " + host.ID + " must use " + host.SkillsRoot)
		}
		profile.HostSkillsRoots[host.ID] = root
	}
	rawPackages, exists := object["packages"]
	if !exists {
		return projectProfile{}, usageError("project_profile_invalid: packages is required by prifly-project-profile/2")
	}
	packages, ok := rawPackages.(map[string]any)
	if !ok {
		return projectProfile{}, usageError("project_profile_invalid: packages must be an object")
	}
	profile.Packages = make(map[string]projectPackage, len(packages))
	for name, raw := range packages {
		if !projectLaunchID(name) {
			return projectProfile{}, usageError("project_profile_invalid: package name must contain lowercase letters, digits, - or _")
		}
		entry, ok := raw.(map[string]any)
		if !ok {
			return projectProfile{}, usageError("project_profile_invalid: package " + name + " must be an object")
		}
		for key := range entry {
			switch key {
			case "source", "origin":
			default:
				return projectProfile{}, usageError("project_profile_invalid: package " + name + " has unknown field " + key)
			}
		}
		source, ok := entry["source"].(string)
		if !ok || source == "" {
			return projectProfile{}, usageError("project_profile_invalid: package " + name + " source must be a non-empty string")
		}
		if _, err := projectPackageSourceLocation(root, source); err != nil {
			return projectProfile{}, err
		}
		pkg := projectPackage{Source: source}
		if raw, exists := entry["origin"]; exists {
			origin, err := parseProjectWorkflowOrigin(name, raw)
			if err != nil {
				return projectProfile{}, err
			}
			pkg.Origin = &origin
		}
		profile.Packages[name] = pkg
	}
	for id, raw := range launches {
		if !projectLaunchID(id) {
			return projectProfile{}, usageError("project_profile_invalid: launch ID must contain lowercase letters, digits, - or _")
		}
		object, ok := raw.(map[string]any)
		if !ok {
			return projectProfile{}, usageError("project_profile_invalid: launch " + id + " must be an object")
		}
		for key := range object {
			switch key {
			case "title", "description", "kind", "workflow":
			default:
				return projectProfile{}, usageError("project_profile_invalid: unknown field in launch " + id + ": " + key)
			}
		}
		launch := projectLaunch{}
		for _, field := range []struct {
			name   string
			target *string
		}{
			{"title", &launch.Title}, {"description", &launch.Description}, {"kind", &launch.Kind}, {"workflow", &launch.Workflow},
		} {
			if value, exists := object[field.name]; exists {
				text, ok := value.(string)
				if !ok {
					return projectProfile{}, usageError("project_profile_invalid: launch " + id + " field " + field.name + " must be a string")
				}
				*field.target = text
			}
		}
		if launch.Title == "" || launch.Description == "" {
			return projectProfile{}, usageError("project_profile_invalid: launch " + id + " requires title and description")
		}
		if launch.Kind != "workflow" || launch.Workflow == "" {
			return projectProfile{}, usageError("project_profile_invalid: launch " + id + " kind must be workflow with workflow source")
		}
		profile.Launches[id] = launch
	}
	return profile, nil
}

func (profile projectProfile) skillsRoot(hostID string) (string, error) {
	root, ok := profile.HostSkillsRoots[hostID]
	if !ok {
		return "", usageError("project_compile_unknown_host: " + hostID)
	}
	return root, nil
}

func (profile projectProfile) launchDetails(root string) ([]projectLaunchDetail, error) {
	ids := make([]string, 0, len(profile.Launches))
	for id := range profile.Launches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]projectLaunchDetail, 0, len(ids))
	for _, id := range ids {
		launch := profile.Launches[id]
		detail := projectLaunchDetail{ID: id, Title: launch.Title, Description: launch.Description, Kind: launch.Kind}
		path, err := projectLaunchSource(root, launch.Workflow)
		if err != nil {
			return nil, err
		}
		if filepath.Base(path) != "workflow.yaml" {
			return nil, usageError("project_profile_invalid: workflow " + id + " must name a workflow folder root")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		value, err := flow.Parse(data, "yaml")
		if err != nil {
			return nil, usageError("project_profile_invalid: workflow " + id + ": " + err.Error())
		}
		object, ok := value.(map[string]any)
		if !ok || object["authoring"] != projectWorkflowFolderVersion {
			return nil, usageError("project_profile_invalid: workflow " + id + " must be a prifly-project-workflow/1 folder root")
		}
		inputs, err := projectFolderLaunchInputs(value)
		if err != nil {
			return nil, usageError("project_profile_invalid: workflow " + id + ": " + err.Error())
		}
		detail.Source, detail.Inputs = launch.Workflow, inputs
		result = append(result, detail)
	}
	return result, nil
}

func projectFolderLaunchInputs(value any) ([]projectLaunchInput, error) {
	workflow, err := projectFolderWorkflowDefinition(value)
	if err != nil {
		return nil, err
	}
	raw, exists := workflow["inputs"]
	if !exists {
		return nil, errors.New("workflow.yaml requires inputs")
	}
	inputs, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("workflow.yaml inputs must be an object")
	}
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]projectLaunchInput, 0, len(names))
	for _, name := range names {
		input := projectLaunchInput{Name: name, Required: true, Format: "json"}
		if details, ok := inputs[name].(map[string]any); ok {
			if required, exists := details["required"]; exists {
				value, ok := required.(bool)
				if !ok {
					return nil, errors.New("workflow.yaml input required must be boolean")
				}
				input.Required = value
			}
			if format, exists := details["format"]; exists {
				value, ok := format.(string)
				if !ok || value == "" {
					return nil, errors.New("workflow.yaml input format must be a non-empty string")
				}
				input.Format = value
			}
		}
		result = append(result, input)
	}
	return result, nil
}

func projectLaunchSource(root, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", usageError("project_profile_invalid: launch source must be a relative .prifly path")
	}
	profileRoot, err := canonicalProjectPath(filepath.Join(root, ".prifly"))
	if err != nil {
		return "", err
	}
	path, err := canonicalProjectPath(filepath.Join(root, source))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(profileRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", usageError("project_profile_invalid: launch source must stay inside .prifly")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", usageError("project_profile_invalid: launch source does not exist: " + source)
	}
	if !info.Mode().IsRegular() {
		return "", usageError("project_profile_invalid: launch source must be a regular file: " + source)
	}
	return path, nil
}

func projectPackageSourceLocation(root, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", usageError("project_profile_invalid: package source must be a relative .prifly path")
	}
	profileRoot, err := canonicalProjectPath(filepath.Join(root, ".prifly"))
	if err != nil {
		return "", err
	}
	path, err := canonicalProjectPath(filepath.Join(root, source))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(profileRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", usageError("project_profile_invalid: package source must stay inside .prifly")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", usageError("project_profile_invalid: package source does not exist: " + source)
	}
	if !info.IsDir() {
		return "", usageError("project_profile_invalid: package source must be a workflow folder: " + source)
	}
	return path, nil
}

func projectLaunchID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, character := range id {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func projectRepositoryRoot(ctx context.Context, path string) (string, error) {
	directory, err := canonicalProjectPath(path)
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", usageError("repository_required: project init requires an existing Git repository")
	}
	return canonicalProjectPath(strings.TrimSpace(string(output)))
}

func canonicalProjectPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	for parent := absolute; ; parent = filepath.Dir(parent) {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			relative, err := filepath.Rel(parent, absolute)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", err
		}
	}
}

func projectPathsOverlap(first, second string) bool {
	inside := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	return inside(first, second) || inside(second, first)
}

func defaultProjectAuthorityRoot(repository string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("local_state_root_unavailable: %w", err)
	}
	digest := sha256.Sum256([]byte(repository))
	return filepath.Join(base, "Pri-Fly", "projects", hex.EncodeToString(digest[:])), nil
}

func projectExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("prifly_executable_unavailable: %w", err)
	}
	return canonicalProjectPath(path)
}

func ensureProjectAuthority(root string) error {
	engine, err := prifly.Open(root, true)
	if err == nil {
		defer engine.Close()
		return checkProjectAuthority(engine)
	}
	return prifly.InitProjectProfile(root)
}

func checkProjectAuthority(engine *prifly.Engine) error {
	if engine.Config.Configuration.SemanticsProfile != flow.CoreProfile {
		return usageError("authority_profile_incompatible: project workflows require core-workflow/1 authority")
	}
	if engine.Config.Configuration.SchemaVersion != prifly.CoreContextConfigVersion {
		return usageError("authority_configuration_incompatible: project workflow launches require an authority created by project init with this Pri-Fly version")
	}
	return nil
}

func projectLocalSource(authority, executable string) string {
	return "# This file tells the local CLI where its machine-only authority lives.\n" +
		"# It is ignored by Git because its paths are user-specific.\n" +
		"authority_root: " + strconv.Quote(authority) + "\n" +
		"prifly_executable: " + strconv.Quote(executable) + "\n"
}

func projectRunnerPath(root string, host projectHost) string {
	return filepath.Join(root, filepath.FromSlash(host.SkillsRoot), "prifly-run")
}

func projectRunnerSkill(host projectHost) string {
	return projectRunnerSkillBeforeCatalog(host) + projectCatalogInstructions
}

func projectRunnerSkillBeforeCatalog(host projectHost) string {
	return projectRunnerSkillBeforeDecisionBridge(host) + projectDecisionBridgeInstructions
}

func projectRunnerSkillBeforeDecisionBridge(host projectHost) string {
	return projectRunnerSkillFromTemplate(host, projectRunnerSkillTemplate, projectRunnerQuestions(host))
}

func projectPreviousRunnerSkill(host projectHost) string {
	questions := projectPreviousCodexQuestionInstructions
	if host.ID == "claude-code" {
		questions = projectPreviousClaudeQuestionInstructions
	}
	return projectRunnerSkillFromTemplate(host, projectPreviousRunnerSkillTemplate, questions)
}

func projectRunnerQuestions(host projectHost) string {
	questions := projectCodexQuestionInstructions
	if host.ID == "claude-code" {
		questions = projectClaudeQuestionInstructions
	}
	return questions
}

func projectRunnerSkillFromTemplate(host projectHost, template, questions string) string {
	return strings.ReplaceAll(template, "{{host}}", host.ID) + questions
}

func projectRunnerSkillAccepted(host projectHost, skill string) bool {
	return skill == projectRunnerSkill(host) || skill == projectRunnerSkillBeforeCatalog(host) || skill == projectRunnerSkillBeforeDecisionBridge(host) || skill == projectPreviousRunnerSkill(host)
}

func checkProjectRunners(root string) error {
	for _, host := range projectHosts {
		path := projectRunnerPath(root, host)
		if _, err := os.Lstat(path); err == nil {
			return usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// existingProjectProfile accepts the tracked half of a freshly cloned project
// without treating it as an instruction to replace the team's workflow rules.
func existingProjectProfile(root, profile string) (bool, error) {
	info, err := os.Lstat(profile)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, usageError("project_profile_invalid: .prifly must be a real directory")
	}
	if _, err := readProjectProfile(root); err != nil {
		return false, err
	}
	if err := checkExistingProjectRunners(root); err != nil {
		return false, err
	}
	if _, err := os.Lstat(filepath.Join(profile, "local.yaml")); err == nil {
		return false, usageError("project_local_conflict: existing .prifly/local.yaml was not overwritten")
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	ignored, err := os.ReadFile(filepath.Join(profile, ".gitignore"))
	if err != nil || !strings.Contains("\n"+strings.TrimSpace(string(ignored))+"\n", "\nlocal.yaml\n") {
		return false, usageError("project_local_ignore_missing: .prifly/.gitignore must ignore local.yaml")
	}
	return true, nil
}

func checkExistingProjectRunners(root string) error {
	for _, host := range projectHosts {
		path := filepath.Join(projectRunnerPath(root, host), "SKILL.md")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return usageError("project_runner_missing: existing project profile requires " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run", "SKILL.md")))
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !projectRunnerSkillAccepted(host, string(data)) {
			return usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
		}
	}
	return nil
}

// updateProjectRunners validates every tracked runner before replacing any
// file. This keeps a local customization from being partly overwritten.
func updateProjectRunners(root string) ([]string, error) {
	type runnerUpdate struct {
		host projectHost
		path string
	}
	updates := make([]runnerUpdate, 0, len(projectHosts))
	for _, host := range projectHosts {
		path := filepath.Join(projectRunnerPath(root, host), "SKILL.md")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, usageError("project_runner_missing: project runners update requires " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run", "SKILL.md")))
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		switch string(data) {
		case projectRunnerSkill(host):
			continue
		case projectRunnerSkillBeforeCatalog(host), projectRunnerSkillBeforeDecisionBridge(host), projectPreviousRunnerSkill(host):
			updates = append(updates, runnerUpdate{host: host, path: path})
		default:
			return nil, usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
		}
	}
	updated := make([]string, 0, len(updates))
	for _, update := range updates {
		if err := replaceProjectRunner(update.path, projectRunnerSkill(update.host)); err != nil {
			return nil, err
		}
		updated = append(updated, update.host.ID)
	}
	return updated, nil
}

func replaceProjectRunner(path, contents string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".prifly-runner-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func writeProjectRunners(root string) error {
	for _, host := range projectHosts {
		parent := filepath.Dir(projectRunnerPath(root, host))
		if err := os.MkdirAll(parent, 0755); err != nil {
			return err
		}
		runner := projectRunnerPath(root, host)
		// Mkdir makes an existing project skill a conflict instead of silently
		// replacing instructions the team has reviewed.
		if err := os.Mkdir(runner, 0755); err != nil {
			if errors.Is(err, os.ErrExist) {
				return usageError("project_runner_conflict: existing " + filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run")) + " was not overwritten")
			}
			return err
		}
		skill := projectRunnerSkill(host)
		if err := os.WriteFile(filepath.Join(runner, "SKILL.md"), []byte(skill), 0644); err != nil {
			return err
		}
	}
	return nil
}

func writeProjectProfile(profile, authority, executable string) error {
	parent := filepath.Dir(profile)
	temporary, err := os.MkdirTemp(parent, ".prifly-profile-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, directory := range []string{"workflows"} {
		if err := os.Mkdir(filepath.Join(temporary, directory), 0755); err != nil {
			return err
		}
	}
	for _, file := range []struct {
		name, content string
	}{{"project.yaml", projectProfileSource}, {".gitignore", "local.yaml\n"}, {"local.example.yaml", projectLocalExample}, {"local.yaml", projectLocalSource(authority, executable)}} {
		if err := os.WriteFile(filepath.Join(temporary, file.name), []byte(file.content), 0644); err != nil {
			return err
		}
	}
	// Mkdir is the no-overwrite commit point. Rename could replace an empty
	// profile directory another developer or process created after validation.
	if err := os.Mkdir(profile, 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return usageError("project_profile_conflict: existing .prifly profile was not overwritten")
		}
		return err
	}
	entries, err := os.ReadDir(temporary)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(temporary, entry.Name()), filepath.Join(profile, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func writeProjectLocal(profile, authority, executable string) error {
	path := filepath.Join(profile, "local.yaml")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return usageError("project_local_conflict: existing .prifly/local.yaml was not overwritten")
	}
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(projectLocalSource(authority, executable))
	return err
}
