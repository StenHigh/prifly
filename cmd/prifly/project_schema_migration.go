package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Recovery of a saved choice must not depend on parsing the workflow profile.
func projectMigrationRoot(repository string) (string, error) {
	root, err := canonicalProjectPath(repository)
	if err != nil {
		return "", err
	}
	for parent := root; ; parent = filepath.Dir(parent) {
		for _, name := range []string{"project.yaml", "local.yaml"} {
			if _, err := os.Stat(filepath.Join(parent, ".prifly", name)); err == nil {
				return parent, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return root, nil
}

func (c *cli) projectMigrationBuild(ctx context.Context, root, launchID, host, requestedProfile string) (prifly.LaunchSchemaMigrationBuild, error) {
	build := prifly.LaunchSchemaMigrationBuild{}
	profile, err := readProjectProfile(root)
	if err != nil {
		return build, err
	}
	if !projectNeutralProfile(profile.SchemaVersion) {
		return build, refusal("project_questionnaire_prepare_requires_profile_3", "saved migration choices require reviewed Project profile /3 or later")
	}
	launch, ok := profile.Launches[launchID]
	if !ok {
		return build, refusal("project_start_unknown_launch", launchID)
	}
	name, err := profile.packageForLaunch(root, launch)
	if err != nil {
		return build, err
	}
	preflight, err := projectDecisionPreflight(root, profile, name, requestedProfile, "", nil, nil, false)
	if err != nil {
		return build, err
	}
	scratch, err := os.MkdirTemp("", "prifly-launch-choice-")
	if err != nil {
		return build, err
	}
	defer os.RemoveAll(scratch)
	compiled, err := c.compileDeclaredProjectPackage(ctx, root, profile, name, host, preflight.PackageProfile, filepath.Join(scratch, "package"))
	if err != nil {
		return build, err
	}
	workflow, err := projectCompiledLaunchPath(root, launch, compiled)
	if err != nil {
		return build, err
	}
	build.BuildKey = compiled.BuildKey
	build.ConsumerRef = compiled.Package
	for _, component := range compiled.Components {
		if component.Path == workflow && component.Kind == "workflow" {
			build.WorkflowRef = component.Ref
		}
	}
	return build, nil
}

func (c *cli) projectSchemaMigration(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "inspect" && args[0] != "set" && args[0] != "clear" {
		return usageError("project schema-migration inspect|set|clear")
	}
	operation := args[0]
	f := flags("project schema-migration " + operation)
	repository := f.String("repository", ".", "project repository")
	launch := f.String("launch", "", "declared launch ID")
	host := f.String("host", "", "host used by the exact launch build")
	profile := f.String("package-profile", "", "per-launch package profile")
	id := f.String("id", "", "existing schema migration decision ID")
	expectedBuild := f.String("expected-build-key", "", "exact build key from inspect")
	generation := f.Int64("expected-generation", -1, "choice generation from inspect; 0 when absent")
	reason := f.String("reason", "", "owner reason for changing the saved choice")
	command := f.String("command-id", "", "stable retry identity")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if !projectLaunchID(*launch) {
		return usageError("project schema-migration requires --launch ID")
	}
	if operation != "inspect" && (*generation < 0 || *reason == "") {
		return usageError("changing a launch choice requires --expected-generation N and --reason TEXT")
	}
	if operation == "set" && (*id == "" || *expectedBuild == "") {
		return usageError("set requires --id ID and --expected-build-key KEY from inspect")
	}
	if operation == "clear" && (*id != "" || *expectedBuild != "" || *host != "" || *profile != "") {
		return usageError("clear uses the recorded scope and does not accept build or decision overrides")
	}
	root, err := projectMigrationRoot(*repository)
	if err != nil {
		return err
	}
	if err := c.projectAuthority(root, projectProfile{SchemaVersion: "prifly-project-profile/3"}); err != nil {
		return err
	}
	engine, err := prifly.Open(c.project, operation == "inspect")
	if err != nil {
		return err
	}
	defer engine.Close()
	scope := prifly.LaunchSchemaMigrationScope{AuthorityID: engine.Installation.ID, ProjectID: engine.Config.ID, Repository: root, Launch: *launch}
	if operation == "inspect" {
		view, err := engine.InspectLaunchSchemaMigration(ctx, scope)
		if err != nil {
			return err
		}
		build, buildErr := c.projectMigrationBuild(ctx, root, *launch, *host, *profile)
		if buildErr != nil {
			view.Valid = false
			view.Reasons = append(view.Reasons, buildErr.Error())
		} else {
			view.Current = &build
			if view.State.Choice != nil && view.State.Choice.LaunchSchemaMigrationBuild != build {
				view.Valid = false
				view.Reasons = append(view.Reasons, "saved build differs; review and replace or clear the choice")
			}
			if *id != "" {
				candidate, err := engine.CheckLaunchSchemaMigration(ctx, *id, build)
				if err != nil {
					view.Reasons = append(view.Reasons, err.Error())
				} else {
					view.Candidate = &candidate
				}
			}
		}
		return c.emit(view)
	}
	if *command == "" {
		*command = commandID()
	}
	request := prifly.LaunchSchemaMigrationRequest{Scope: scope, ExpectedGeneration: *generation, CommandID: *command, Reason: *reason}
	if operation == "set" {
		request.Choice = &prifly.LaunchSchemaMigrationChoice{LaunchSchemaMigrationBuild: prifly.LaunchSchemaMigrationBuild{BuildKey: *expectedBuild}, DecisionID: *id}
	}
	replay, found, replayErr := engine.LaunchSchemaMigrationReplay(ctx, request)
	if replayErr != nil {
		return replayErr
	}
	if found {
		if replay.Receipt.Rejection != nil {
			return recordedRejection(replay.Receipt.Rejection, replay.Receipt.ID)
		}
		return c.emit(replay)
	}
	if operation == "set" {
		build, err := c.projectMigrationBuild(ctx, root, *launch, *host, *profile)
		if err != nil {
			return err
		}
		if build.BuildKey != *expectedBuild {
			return refusal("schema_migration_scope_mismatch", "build changed; inspect the launch and review its current build key")
		}
		choice, err := engine.CheckLaunchSchemaMigration(ctx, *id, build)
		if err != nil {
			return err
		}
		request.Choice = &choice
	}
	result, err := engine.SetLaunchSchemaMigration(ctx, request)
	if err != nil {
		return err
	}
	if result.Receipt.Rejection != nil {
		return recordedRejection(result.Receipt.Rejection, result.Receipt.ID)
	}
	return c.emit(result)
}
