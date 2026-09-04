package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// ValidateStartInputs runs the same input resolution as Start without sealing
// artifacts. Project launch uses it before importing a package or taking a claim.
func (e *Engine) ValidateStartInputs(plan *flow.Plan, values map[string]json.RawMessage, refs map[string]ArtifactRef) error {
	effective, err := e.effectiveConfigurationWithValues(plan, nil, refs, values, true)
	if err != nil {
		return err
	}
	_, err = e.resolveStartInputs(plan, effective, StartOptions{SchemaVersion: "2", InputValues: values, InputRefs: refs})
	return err
}

type resolvedStartInput struct {
	Ref  ArtifactRef
	Data []byte
}

func (e *Engine) resolveStartInputs(plan *flow.Plan, effective *EffectiveConfiguration, options StartOptions) (map[string]resolvedStartInput, error) {
	for port := range options.Inputs {
		if _, ok := plan.Workflow.Inputs[port]; !ok {
			return nil, fmt.Errorf("unknown input: %s", port)
		}
		if _, ok := options.InputRefs[port]; ok {
			return nil, errors.New("input file and reference cannot both bind the same port")
		}
	}
	for port := range options.InputRefs {
		if _, ok := plan.Workflow.Inputs[port]; !ok {
			return nil, fmt.Errorf("unknown input ref: %s", port)
		}
	}
	for port := range options.InputValues {
		if _, ok := plan.Workflow.Inputs[port]; !ok {
			return nil, fmt.Errorf("unknown input value: %s", port)
		}
		_, byFile := options.Inputs[port]
		_, byRef := options.InputRefs[port]
		if byFile || byRef {
			return nil, errors.New("input value, file and reference cannot bind the same port")
		}
	}
	inputs := map[string]resolvedStartInput{}
	for name, port := range plan.Workflow.Inputs {
		path, hasFile := options.Inputs[name]
		ref, hasRef := options.InputRefs[name]
		value, hasValue := options.InputValues[name]
		var configured json.RawMessage
		if effective != nil {
			configured = effective.Inputs[name].Value
		}
		if options.SchemaVersion != "2" && hasValue && len(configured) > 0 {
			return nil, fmt.Errorf("input value and pinned configuration cannot both bind the same port: %s", name)
		}
		if !hasFile && !hasRef && !hasValue && len(configured) == 0 {
			if port.Required {
				if options.SchemaVersion == "2" {
					return nil, faultf("missing_input", "missing required input: %s", name)
				}
				return nil, fmt.Errorf("missing required input: %s", name)
			}
			continue
		}
		var data []byte
		var err error
		if len(configured) > 0 {
			data = configured
		} else if hasValue {
			data = value
		} else if hasFile {
			data, err = e.inputBytes(path)
		} else {
			var artifact Artifact
			artifact, data, err = e.Artifact(ref)
			if err == nil {
				err = e.validatePortArtifact(plan, port.Port, artifact, data)
			}
		}
		if err != nil {
			return nil, err
		}
		if len(data) > MaxArtifactBytes {
			return nil, local.ErrBlobLimit
		}
		if hasRef {
			inputs[name] = resolvedStartInput{Ref: ref, Data: data}
			continue
		}
		if port.Format == "json" && port.SchemaRef == nil {
			return nil, errors.New("JSON input schema required")
		}
		if port.SchemaRef != nil && port.SchemaRef.ID == sourceSnapshotSchemaID {
			return nil, contextProblem("source_snapshot_invalid", "/producer", "Source input requires an acquired snapshot reference, not a literal or file descriptor.")
		}
		media, err := artifactMediaType(port.Format, []string{portMedia(port.Port)})
		if err != nil {
			return nil, err
		}
		// Validate the eventual descriptor, without manufacturing an artifact ID
		// or publishing bytes merely to discover an invalid input contract.
		artifact := Artifact{Format: port.Format, SchemaRef: port.SchemaRef, MediaType: media, SizeBytes: int64(len(data)), Digest: rawDigest(data)}
		if err := e.validatePortArtifact(plan, port.Port, artifact, data); err != nil {
			return nil, err
		}
		inputs[name] = resolvedStartInput{Data: data}
	}
	return inputs, nil
}
