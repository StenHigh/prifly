package runtime

import (
	"path/filepath"

	"github.com/stenhigh/prifly/internal/flow"
)

// WorkspaceTreeGuideFile is handed to the executor beside the step manifest.
// The manifest cannot carry this: its version string is embedded in every
// generation of the Run read contracts, and those contracts are frozen and are
// required to keep reading a current Run, so raising it would make a fresh Run
// unreadable by a contract obliged to read it. A sibling document moves no
// contract and reaches the same reader, who has the workspace by construction.
const WorkspaceTreeGuideFile = "workspace-trees.json"

// WorkspaceTreeGuideVersion is this document's own contract. Nothing else
// carries it, so it moves without dragging the Run state behind it.
const WorkspaceTreeGuideVersion = "workspace-tree-guide/1"

// workspaceTreeGuideNote states the half of the defect that a missing field
// does not cover: the manifest already answers "where do I write this port",
// and for a captured port it answers wrongly. A reader who found an answer
// stops reading, so the wrong answer has to be named, not merely supplemented.
const workspaceTreeGuideNote = "The engine captures these ports from the workspace itself. context.json still shows each of them an output slot, and that slot is real — but it belongs to the engine, which writes the sealed manifest into it after capturing. It is not yours to fill: writing there earns workspace_tree_output_host_supplied, and writing the capture path while also declaring the slot earns workspace_tree_capture_conflict. Produce the result at the capture path, in the declared shape, and do not declare the port in your submission."

// WorkspaceTreeGuide shows an executor the shape of the result it must produce,
// before it produces it. Until this existed the contract was reachable only by
// earning two refusals in sequence, or by reading the step definition past the
// envelope.
type WorkspaceTreeGuide struct {
	SchemaVersion string                   `json:"schema_version"`
	Note          string                   `json:"note"`
	Ports         []WorkspaceTreeGuidePort `json:"ports"`
}

// WorkspaceTreeGuidePort is a copy of what the step declared, not a summary of
// it. A field carrying one path would repeat the manifest's mistake in a new
// place: a direct_child_tree policy needs its kind and entrypoint too, and an
// executor given only a path puts a file where a tree is expected.
type WorkspaceTreeGuidePort struct {
	OutputPort string                          `json:"output_port"`
	InputPort  string                          `json:"input_port,omitempty"`
	Capture    flow.WorkspaceTreeCapturePolicy `json:"capture"`
	// EngineSlotPath repeats the address the manifest prints for this port, so
	// the reader can match the two documents by sight instead of inferring
	// which of the two answers is the live one. The slot cannot be dropped from
	// the manifest: capture requires it and writes the sealed manifest there,
	// so what separates the engine's slot from the executor's is stated here
	// rather than shown by absence.
	EngineSlotPath string `json:"engine_slot_path,omitempty"`
}

// workspaceTreeGuide builds the guide for one step, or reports that the step
// declares no capture and needs no second document.
func workspaceTreeGuide(step flow.StepDefinition, manifest ContextManifest) (WorkspaceTreeGuide, bool) {
	if len(step.WorkspaceTrees) == 0 {
		return WorkspaceTreeGuide{}, false
	}
	guide := WorkspaceTreeGuide{SchemaVersion: WorkspaceTreeGuideVersion, Note: workspaceTreeGuideNote, Ports: make([]WorkspaceTreeGuidePort, 0, len(step.WorkspaceTrees))}
	for _, binding := range step.WorkspaceTrees {
		guide.Ports = append(guide.Ports, WorkspaceTreeGuidePort{
			OutputPort:     binding.OutputPort,
			InputPort:      binding.InputPort,
			Capture:        binding.Capture,
			EngineSlotPath: manifest.Outputs[binding.OutputPort].Path,
		})
	}
	return guide, true
}

// writeWorkspaceTreeGuide places the guide beside the manifest. A step without
// declared capture gets no file at all: an empty guide would be one more
// document to read that answers nothing.
func (e *Engine) writeWorkspaceTreeGuide(workspace string, step flow.StepDefinition, manifest ContextManifest) error {
	guide, declared := workspaceTreeGuide(step, manifest)
	if !declared {
		return nil
	}
	data, err := canonical(guide)
	if err != nil {
		return err
	}
	return writeExclusive(filepath.Join(workspace, WorkspaceTreeGuideFile), data)
}
