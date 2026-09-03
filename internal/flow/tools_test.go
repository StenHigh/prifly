package flow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func toolDescriptorFixture() ToolDescriptor {
	ref := Ref{ID: "core:schema/context-json", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	return ToolDescriptor{SchemaVersion: ToolDescriptorVersion, ID: "example:tool/announce", Version: "1.2.3", AdapterRef: Ref{ID: "core:adapter/local-process", Version: "2.0.0", Digest: "sha256:" + strings.Repeat("b", 64)}, Operation: "announce", ArgumentsSchemaRef: ref, ResultSchemaRef: ref, EffectClass: "external_write", RetryClass: "deduplicated", RequiredCapabilities: []string{"network"}}
}

func TestToolDescriptorClosedContract(t *testing.T) {
	descriptor := toolDescriptorFixture()
	data, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseToolDescriptor(data)
	if err != nil || !reflect.DeepEqual(parsed, descriptor) || ValidateToolDescriptor(descriptor) != nil {
		t.Fatalf("descriptor did not round trip: %+v %v", parsed, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"unknown_field":  func(v map[string]any) { v["delivery"] = "now" },
		"missing_field":  func(v map[string]any) { delete(v, "operation") },
		"unknown_effect": func(v map[string]any) { v["effect_class"] = "magic" },
		"live_adapter":   func(v map[string]any) { v["adapter_ref"].(map[string]any)["latest"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			bad, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseToolDescriptor(bad); err == nil {
				t.Fatal("invalid descriptor was accepted")
			}
		})
	}
}
