package main

import (
	"context"
	"strconv"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func (c *cli) storage(ctx context.Context, args []string) error {
	if len(args) < 2 || args[0] != "budget" {
		return usageError("storage budget show | storage budget set --bytes N --reason TEXT --prepare")
	}
	f := flags("storage budget " + args[1])
	bytes := f.String("bytes", "", "finite allocated logical-page ceiling in exact integer bytes")
	reason := f.String("reason", "", "owner explanation")
	prepare := f.Bool("prepare", false, "review without changing storage")
	digest := f.String("expected-review-digest", "", "digest returned by --prepare")
	command := f.String("command", "", "stable explicit command identity")
	if err := parse(f, args[2:]); err != nil {
		return err
	}
	if args[1] != "show" && args[1] != "set" {
		return usageError("storage budget requires show or set")
	}
	if args[1] == "show" && (*bytes != "" || *reason != "" || *prepare || *digest != "" || *command != "") {
		return usageError("storage budget show takes no mutation flags")
	}
	e, err := prifly.Open(c.project, true)
	if err != nil {
		return err
	}
	if args[1] == "show" {
		defer e.Close()
		view, err := e.StorageBudgetStatus(ctx)
		if emitErr := c.emit(view); emitErr != nil {
			return emitErr
		}
		return err
	}
	target, parseErr := strconv.ParseInt(*bytes, 10, 64)
	if parseErr != nil || *bytes == "" {
		e.Close()
		return &prifly.Fault{Code: "storage_budget_invalid_limit", Message: "--bytes requires exact integer bytes within 64 KiB..64 GiB"}
	}
	if *prepare {
		defer e.Close()
		if *digest != "" || *command != "" {
			return usageError("--prepare takes target and reason; reviewed digest and command apply only to set")
		}
		review, err := e.PrepareStorageBudget(ctx, target, *reason)
		if err != nil {
			return err
		}
		return c.emit(review)
	}
	request := prifly.StorageBudgetRequest{CommandID: *command, TargetBytes: target, Reason: *reason, ExpectedReviewDigest: *digest}
	duplicate, err := e.CheckStorageBudgetSet(ctx, request)
	closeErr := e.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if duplicate != nil {
		return c.emit(duplicate)
	}
	writer, err := prifly.Open(c.project, false)
	if err != nil {
		return err
	}
	defer writer.Close()
	result, err := writer.SetStorageBudget(ctx, request)
	if err != nil {
		return err
	}
	return c.emit(result)
}
