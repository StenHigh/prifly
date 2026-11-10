package main

import (
	"context"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func (c *cli) schemaMigration(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("package schema-migration prepare|apply|inspect")
	}
	f := flags("package schema-migration " + args[0])
	switch args[0] {
	case "prepare":
		file := f.String("request", "", "schema-migration-request/1 JSON")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return usageError("package schema-migration prepare requires --request FILE")
		}
		data, err := readFile(c.requestFile(*file), prifly.MaxDefinitionBytes)
		if err != nil {
			return err
		}
		request, err := prifly.ParseSchemaMigrationRequest(data)
		if err != nil {
			return err
		}
		plan, err := e.PrepareSchemaMigration(ctx, request)
		if err != nil {
			return err
		}
		return c.emit(plan)
	case "apply":
		file := f.String("plan", "", "schema-migration-plan/1 JSON")
		digest := f.String("expected-review-digest", "", "exact reviewed plan digest")
		reason := f.String("reason", "", "owner explanation of the mistaken revoke")
		id := f.String("command-id", "", "stable retry identity")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" || *digest == "" || *reason == "" {
			return usageError("package schema-migration apply requires --plan FILE --expected-review-digest DIGEST --reason TEXT")
		}
		data, err := readFile(c.requestFile(*file), prifly.MaxDefinitionBytes)
		if err != nil {
			return err
		}
		plan, err := prifly.ParseSchemaMigrationPlan(data)
		if err != nil {
			return err
		}
		if *id == "" {
			*id = commandID()
		}
		result, err := e.ApplySchemaMigration(ctx, prifly.SchemaMigrationApplyRequest{CommandID: *id, Plan: plan, ExpectedReviewDigest: *digest, Reason: *reason})
		if err != nil {
			return err
		}
		if result.Receipt.Rejection != nil {
			return recordedRejection(result.Receipt.Rejection, result.Receipt.ID)
		}
		return c.emit(result)
	case "inspect":
		id := f.String("id", "", "recorded schema migration decision ID")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return usageError("package schema-migration inspect requires --id ID")
		}
		result, err := e.InspectSchemaMigration(ctx, *id)
		if err != nil {
			return err
		}
		return c.emit(result)
	default:
		return usageError("package schema-migration prepare|apply|inspect")
	}
}
