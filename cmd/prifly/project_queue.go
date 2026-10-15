package main

import (
	"context"
	"flag"

	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectQueueSelectionFlags struct {
	id, owner            *string
	revision, generation *int64
}

func projectQueueFlags(f *flag.FlagSet) projectQueueSelectionFlags {
	return projectQueueSelectionFlags{f.String("task-id", "", "claimed queue Task ID"), f.String("task-owner", "", "recorded queue owner session identity"), f.Int64("task-revision", 0, "exact task content revision"), f.Int64("task-claim-generation", 0, "exact queue claim generation")}
}
func (q projectQueueSelectionFlags) present() bool {
	return *q.id != "" || *q.owner != "" || *q.revision != 0 || *q.generation != 0
}
func (q projectQueueSelectionFlags) validate() error {
	if !q.present() {
		return nil
	}
	if *q.id == "" || *q.owner == "" || *q.revision < 1 || *q.generation < 1 {
		return usageError("queue selection requires --task-id, --task-owner, --task-revision and --task-claim-generation together")
	}
	return nil
}
func (q projectQueueSelectionFlags) resolve(ctx context.Context, e *prifly.Engine) (*prifly.QueueTaskSelection, []byte, error) {
	if err := q.validate(); err != nil {
		return nil, nil, err
	}
	if !q.present() {
		return nil, nil, nil
	}
	selection, request, err := e.QueueSelection(ctx, *q.id, *q.owner, *q.revision, *q.generation)
	if err != nil {
		return nil, nil, err
	}
	return &selection, request, nil
}

func (c *cli) projectQueue(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("project queue requires add, list, show, update, claim-next, release, complete or reconcile")
	}
	operation := args[0]
	f := flags("project queue " + operation)
	repository := f.String("repository", ".", "directory that owns the shared Pri-Fly profile")
	command := f.String("command-id", "", "stable queue command identity")
	selectionFlags := projectQueueFlags(f)
	owner := f.String("owner", "", "owner session identity for claim-next")
	input := f.String("input", "", "exact TaskInput/1 JSON file")
	priority := f.Int64("priority", 0, "priority; larger values are claimed first")
	reason := f.String("reason", "", "owner reason for release, completion or reconcile")
	intent := f.String("intent-id", "", "launch intent to inspect or reconcile")
	resolution := f.String("resolution", "", "not_started for reserved, effects_resolved for dispatching")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return usageError("queue commands take named flags")
	}
	readOnly := operation == "list" || operation == "show"
	if !readOnly && *command == "" {
		return usageError("queue mutations require --command-id")
	}
	root, err := projectRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	if err := c.projectAuthority(root, profile); err != nil {
		return err
	}
	engine, err := prifly.Open(c.project, readOnly)
	if err != nil {
		return err
	}
	defer engine.Close()
	if operation == "list" {
		view, err := engine.Queue(ctx)
		if err != nil {
			return err
		}
		return c.emit(view)
	}
	if operation == "show" {
		if *selectionFlags.id == "" {
			return usageError("queue show requires --task-id")
		}
		view, err := engine.QueueTask(ctx, *selectionFlags.id)
		if err != nil {
			return err
		}
		return c.emit(view)
	}
	var result local.AuthorityApplyResult
	switch operation {
	case "add", "update":
		if *input == "" {
			return usageError("queue add/update requires --input TASK.json")
		}
		request, readErr := readFile(projectRequestFile(root, *input), prifly.MaxDefinitionBytes)
		if readErr != nil {
			return readErr
		}
		if operation == "add" {
			result, err = engine.QueueAdd(ctx, prifly.QueueAddRequest{CommandID: *command, RequestBytes: request, Priority: *priority})
		} else {
			result, err = engine.QueueUpdate(ctx, prifly.QueueUpdateRequest{CommandID: *command, TaskID: *selectionFlags.id, ExpectedRevision: *selectionFlags.revision, Owner: *selectionFlags.owner, ClaimGeneration: *selectionFlags.generation, RequestBytes: request, Priority: *priority})
		}
	case "claim-next":
		result, err = engine.QueueClaimNext(ctx, prifly.QueueClaimRequest{CommandID: *command, Owner: *owner})
	case "release", "complete", "reconcile":
		// Digest is resolved from this exact recorded revision, including on a late
		// receipt retry after ownership has moved. Runtime alone spends current CAS.
		if err := selectionFlags.validate(); err != nil {
			return err
		}
		if !selectionFlags.present() {
			return usageError("operation requires queue selection")
		}
		selection, readErr := engine.QueueRecordedSelection(ctx, *selectionFlags.id, *selectionFlags.owner, *selectionFlags.revision, *selectionFlags.generation)
		if readErr != nil {
			return readErr
		}
		if operation == "reconcile" {
			result, err = engine.QueueReconcile(ctx, prifly.QueueReconcileRequest{CommandID: *command, Selection: selection, IntentID: *intent, Resolution: *resolution, Reason: *reason})
		} else {
			request := prifly.QueueFinishRequest{CommandID: *command, Selection: selection, Reason: *reason}
			if operation == "release" {
				result, err = engine.QueueRelease(ctx, request)
			} else {
				result, err = engine.QueueComplete(ctx, request)
			}
		}
	default:
		return usageError("unknown project queue operation")
	}
	if err != nil {
		return err
	}
	if result.Receipt.Rejection != nil {
		return result.Receipt.Rejection
	}
	return c.emit(projectwire.QueueCommand{SchemaVersion: "project-queue-command/1", AuthorityID: engine.Installation.ID, ProjectID: engine.Config.ID, CommandID: *command, Operation: operation, Duplicate: result.Duplicate, Result: result.Receipt.Result})
}
