package runtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// The parser and the published contract agree line by line: what one accepts
// the other accepts, and what one refuses the other refuses.
func TestProgramProgressParserAgreesWithItsContract(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		`{"schema_version":"program-progress/1","phase":"product","current":4200,"total":6994}`,
		`{"schema_version":"program-progress/1","phase":"baseline","message":"comparing with main"}`,
		`{"schema_version":"program-progress/1","phase":"a","current":0}`,
		`{"schema_version":"program-progress/1","phase":"a","verdict":"pass"}`,
		`{"schema_version":"program-progress/1","phase":"<b>"}`,
		`{"schema_version":"program-progress/1","phase":"a","total":5}`,
		`{"schema_version":"program-progress/1","phase":"a","current":-1}`,
		`{"schema_version":"program-progress/1","phase":"a","message":"bell\u0007"}`,
		`{"schema_version":"program-progress/1","phase":"` + strings.Repeat("p", 65) + `"}`,
		`{"schema_version":"program-progress/2","phase":"a"}`,
	} {
		_, parseErr := local.ParseProgramProgress([]byte(line))
		var value any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatal(err)
		}
		schemaErr := validateInBundle(t, programProgressContracts, "ProgramProgress", value)
		if (parseErr == nil) != (schemaErr == nil) {
			t.Errorf("parser %v, contract %v: %s", parseErr, schemaErr, line)
		}
	}
}

func TestProgramProgressReadMatchesItsContract(t *testing.T) {
	t.Parallel()
	current, total := int64(4200), int64(6994)
	r := Run{ID: "run:p", Attempts: map[string]*Attempt{"attempt:a": {ID: "attempt:a", StepID: "step:a", ActivationID: "activation:a", Status: "running", Process: &local.ProcessIdentity{LaunchID: "l"}}, "attempt:b": {ID: "attempt:b", Status: "running"}}, Activations: map[string]*Activation{"activation:a": {ID: "activation:a", StageID: "tests"}}}
	records := map[string]local.AttemptProgress{"attempt:a": {AttemptID: "attempt:a", LaunchID: "l", Progress: local.ProgramProgress{SchemaVersion: local.ProgramProgressVersion, Phase: "product", Current: &current, Total: &total}, Observed: time.Now().UTC(), Accepted: 3, Rejected: 1}}
	view := programProgressView(r, 7, records, true, true, Observation{UTC: time.Now().UTC().Format(time.RFC3339Nano)})
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if err := validateInBundle(t, programProgressContracts, "ProgramProgressRead", value); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
}
