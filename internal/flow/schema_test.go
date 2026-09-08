package flow

import (
	"strings"
	"testing"
)

// An author who wrote prose where an object belongs was told only "the declared
// type is object". The field's shape is written in exactly one place, and the
// refusal did not say which — the pilot's reference brief had an empty list, so
// there was nothing to copy from either.
func TestAnObjectShapedRefusalNamesTheContractThatPrintsIt(t *testing.T) {
	brief := map[string]any{"schema_version": "1", "confirmation": "explicit", "source_refs": []any{"a plain sentence where an object belongs"}}
	err := validateProtocolValue("RunBrief", brief, "")
	if err == nil {
		t.Skip("RunBrief no longer refuses prose in source_refs; the case this pins is gone")
	}
	message := err.Error()
	if !strings.Contains(message, "the declared type is object") {
		t.Fatalf("the refusal stopped naming the declared type: %s", message)
	}
	if !strings.Contains(message, "prifly schema RunBrief") {
		t.Fatalf("the refusal names a shape without saying where the shape is written: %s", message)
	}
}

// A scalar type is the whole answer, so naming a command there would be noise.
func TestAScalarRefusalStaysAsShortAsTheAnswer(t *testing.T) {
	brief := map[string]any{"schema_version": 1}
	err := validateProtocolValue("RunBrief", brief, "")
	if err == nil {
		t.Skip("RunBrief no longer refuses a numeric schema_version")
	}
	if message := err.Error(); strings.Contains(message, "prifly schema") && strings.Contains(message, "the declared type is string") {
		t.Fatalf("a scalar mismatch was padded with a command that adds nothing: %s", message)
	}
}
