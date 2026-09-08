package runtime

import (
	"crypto/sha256"
	"fmt"
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

// workspaceTreeGuideNote says where the result goes. It no longer has to argue
// with context.json: the captured port is absent from outputs there, so all
// three documents an executor reads now say the same thing. Reading order stops
// mattering, and it never could be relied on — slots are the natural place to
// start, and starting there used to earn a refusal for doing the obvious.
const workspaceTreeGuideNote = "The engine captures these ports from the workspace itself, so context.json lists no output slot for them and the submission template leaves them out. Produce the result at the capture path below, in the declared shape, and do not declare the port in your submission: declaring it earns workspace_tree_output_host_supplied, and writing into a slot for it earns workspace_tree_capture_conflict."

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
}

// workspaceTreeGuide builds the guide for one step, or reports that the step
// declares no capture and needs no second document.
func workspaceTreeGuide(step flow.StepDefinition) (WorkspaceTreeGuide, bool) {
	if len(step.WorkspaceTrees) == 0 {
		return WorkspaceTreeGuide{}, false
	}
	guide := WorkspaceTreeGuide{SchemaVersion: WorkspaceTreeGuideVersion, Note: workspaceTreeGuideNote, Ports: make([]WorkspaceTreeGuidePort, 0, len(step.WorkspaceTrees))}
	for _, binding := range step.WorkspaceTrees {
		guide.Ports = append(guide.Ports, WorkspaceTreeGuidePort{OutputPort: binding.OutputPort, InputPort: binding.InputPort, Capture: binding.Capture})
	}
	return guide, true
}

// writeWorkspaceTreeGuide places the guide beside the manifest. A step without
// declared capture gets no file at all: an empty guide would be one more
// document to read that answers nothing.
func (e *Engine) writeWorkspaceTreeGuide(workspace string, step flow.StepDefinition) error {
	guide, declared := workspaceTreeGuide(step)
	if !declared {
		return nil
	}
	data, err := canonical(guide)
	if err != nil {
		return err
	}
	return writeExclusive(filepath.Join(workspace, WorkspaceTreeGuideFile), data)
}

// outputArtifactID is the identity a port's artifact carries. One rule serves
// the ordinary slot and the captured port alike, so the two cannot drift apart
// and a re-issued attempt reports the ids its first delivery did.
func outputArtifactID(attemptID, port string) string {
	return fmt.Sprintf("artifact:%x", sha256.Sum256([]byte(attemptID+"/"+port)))
}
