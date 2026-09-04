package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type taskPrepared struct {
	SchemaVersion  string             `json:"schema_version"`
	TaskID         string             `json:"task_id"`
	BriefPath      string             `json:"brief_path"`
	Brief          prifly.Brief       `json:"brief"`
	SourceSnapshot prifly.ArtifactRef `json:"source_snapshot"`
}

func (c *cli) task(e *prifly.Engine, args []string) error {
	if len(args) == 0 || args[0] != "prepare" {
		return usageError("task requires prepare")
	}
	f := flags("task prepare")
	inputPath := f.String("input", "", "TaskInput/1 JSON file or - for stdin")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if *inputPath == "" {
		return usageError("task prepare requires --input TASK.json")
	}
	data, err := readFile(c.requestFile(*inputPath), prifly.MaxDefinitionBytes)
	if err != nil {
		return err
	}
	input, err := prifly.ParseTaskInput(data)
	if err != nil {
		return err
	}
	for _, ref := range input.SourceRefs {
		if _, err := e.SourceSnapshot(ref); err != nil {
			return err
		}
	}
	briefPath := taskBriefPath(e.Root, data)
	if prepared, found, err := existingTaskPreparation(e, briefPath, data, input); err != nil || found {
		if err != nil {
			return err
		}
		return c.emit(prepared)
	}
	sourceFile, cleanup, err := materializeTaskInput(e.Root, data)
	if err != nil {
		return err
	}
	defer cleanup()
	snapshot, err := e.ImportSource(prifly.SourceImportOptions{
		Path: sourceFile, Format: "blob", MediaType: "application/json",
		ExternalIdentity: input.Source.ExternalID, ExternalVersion: input.Source.Version, ExternalScope: input.Source.URL,
	})
	if err != nil {
		return err
	}
	sources := append(append([]prifly.ArtifactRef{}, input.SourceRefs...), snapshot.Ref())
	brief := input.RunBrief(sources)
	briefBytes, err := canonicalTaskBrief(brief)
	if err != nil {
		return err
	}
	if err := flow.ValidateProtocol("RunBrief", briefBytes); err != nil {
		return err
	}
	if err := writeNewTaskBrief(briefPath, briefBytes); err != nil {
		return err
	}
	return c.emit(taskPrepared{SchemaVersion: "task-prepared/1", TaskID: input.ID, BriefPath: briefPath, Brief: brief, SourceSnapshot: snapshot.Ref()})
}

func taskBriefPath(root string, input []byte) string {
	digest := sha256.Sum256(input)
	return filepath.Join(root, ".prifly", "intake", hex.EncodeToString(digest[:])+".brief.json")
}

func materializeTaskInput(root string, data []byte) (string, func(), error) {
	directory, err := os.MkdirTemp(filepath.Dir(root), ".prifly-task-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "task-input.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func writeNewTaskBrief(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	if written, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	} else if written != len(data) {
		_ = f.Close()
		return io.ErrShortWrite
	}
	return f.Close()
}

func existingTaskPreparation(e *prifly.Engine, path string, inputBytes []byte, input prifly.TaskInput) (taskPrepared, bool, error) {
	data, err := readFile(path, prifly.MaxDefinitionBytes)
	if errors.Is(err, os.ErrNotExist) {
		return taskPrepared{}, false, nil
	}
	if err != nil {
		return taskPrepared{}, false, err
	}
	if err := flow.ValidateProtocol("RunBrief", data); err != nil {
		return taskPrepared{}, false, &prifly.Fault{Code: "task_brief_conflict", Message: "existing intake brief is not valid", Cause: err}
	}
	var brief prifly.Brief
	if err := json.Unmarshal(data, &brief); err != nil {
		return taskPrepared{}, false, err
	}
	if len(brief.SourceRefs) != len(input.SourceRefs)+1 {
		return taskPrepared{}, false, &prifly.Fault{Code: "task_brief_conflict", Message: "existing intake brief has different source references"}
	}
	for i, ref := range input.SourceRefs {
		if brief.SourceRefs[i] != ref {
			return taskPrepared{}, false, &prifly.Fault{Code: "task_brief_conflict", Message: "existing intake brief has different source references"}
		}
	}
	snapshotRef := brief.SourceRefs[len(brief.SourceRefs)-1]
	snapshot, err := e.SourceSnapshot(snapshotRef)
	if err != nil {
		return taskPrepared{}, false, err
	}
	_, source, err := e.Artifact(snapshot.ContentRef)
	if err != nil {
		return taskPrepared{}, false, err
	}
	want := input.RunBrief(brief.SourceRefs)
	wantBytes, err := canonicalTaskBrief(want)
	if err != nil {
		return taskPrepared{}, false, err
	}
	if !bytes.Equal(source, inputBytes) || !bytes.Equal(data, wantBytes) {
		return taskPrepared{}, false, &prifly.Fault{Code: "task_brief_conflict", Message: "existing intake brief does not match the selected task"}
	}
	return taskPrepared{SchemaVersion: "task-prepared/1", TaskID: input.ID, BriefPath: path, Brief: brief, SourceSnapshot: snapshotRef}, true, nil
}

func canonicalTaskBrief(value prifly.Brief) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return flow.Canonical(data)
}
