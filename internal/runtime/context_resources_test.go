package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func contextRegistryRuntime(t *testing.T) *Engine {
	t.Helper()
	root := t.TempDir()
	if err := InitProfile(root, flow.CoreProfile); err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := os.Mkdir(filepath.Join(root, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	return e
}

func contextRegistryEntry(t *testing.T, e *Engine, path, id, kind, encoding, media string, raw []byte) Definition {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.Root, path), raw, 0600); err != nil {
		t.Fatal(err)
	}
	data := raw
	if encoding != "utf8_text" {
		format := "json"
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			format = "yaml"
		}
		var err error
		data, err = flow.JSONBytes(raw, format)
		if err != nil {
			t.Fatal(err)
		}
		data, err = flow.Canonical(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	return Definition{Ref: flow.Ref{ID: id, Version: "1.0.0", Digest: rawDigest(data)}, Kind: kind, Path: path, ByteEncoding: encoding, MediaType: media}
}

func TestContextResourceInventorySeparatesRepresentationsAndReadOnlyDoctor(t *testing.T) {
	e := contextRegistryRuntime(t)
	text := []byte("\ufeff# Правила\r\nUnicode e\u0301 / é\r\n\x00 trailing  ")
	jsonSource := []byte(" {\"version\":\"9.0.0\",\"id\":\"not:a-definition\",\"digest\":\"sha256:" + strings.Repeat("a", 64) + "\"} \n")
	jsonBytes, err := flow.Canonical(jsonSource)
	if err != nil {
		t.Fatal(err)
	}
	yaml := []byte("message: Привет\nflag: false\n")
	entries := []Definition{
		contextRegistryEntry(t, e, "resources/rules.yaml", "test:context/rules", "resource", "utf8_text", "text/markdown; charset=utf-8", text),
		contextRegistryEntry(t, e, "resources/data.json", "test:context/data", "resource", "json", "application/json", jsonSource),
		contextRegistryEntry(t, e, "resources/data.yml", "test:context/yaml", "resource", "json", "application/json", yaml),
		contextRegistryEntry(t, e, "resources/legacy.json", "test:resource/legacy", "resource", "", "", []byte(" true \n")),
		contextRegistryEntry(t, e, "schemas/boolean.json", "test:schema/boolean", "schema", "", "", []byte("false")),
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	_, before, err := e.Store.ReadAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	defs, registry, pins, err := e.inventoryResources()
	if err != nil {
		t.Fatal(err)
	}
	builtins, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 3 || len(defs) != len(builtins)+2 || len(registry) != len(defs) {
		t.Fatal("typed context was counted as JSON definitions", len(pins), len(defs), len(registry))
	}
	resources, err := resourcesFromPins(pins)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resources[entries[0].Ref].Bytes, text) || !bytes.Equal(resources[entries[1].Ref].Bytes, jsonBytes) {
		t.Fatal("raw bytes or canonical JSON changed")
	}
	for _, pin := range pins {
		if _, exists := registry[pin.Ref]; exists {
			t.Fatal("typed resource leaked into the JSON definition registry")
		}
		if pin.Ref == entries[1].Ref && pin.RawDigest != rawDigest(jsonSource) || pin.Ref == entries[2].Ref && pin.RawDigest != rawDigest(yaml) {
			t.Fatal("original authoring digest was replaced by its canonical digest")
		}
		if len(pin.Bytes) > 0 {
			resources[pin.Ref].Bytes[0] ^= 1
			if bytes.Equal(resources[pin.Ref].Bytes, pin.Bytes) {
				t.Fatal("compiler resources share mutable snapshot bytes")
			}
		}
	}
	oldDefs, oldRegistry, err := e.Inventory()
	if err != nil || !reflect.DeepEqual(oldDefs, defs) || !reflect.DeepEqual(oldRegistry, registry) {
		t.Fatalf("old Inventory signature changed its JSON result: %v", err)
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	check, err := reader.Check(context.Background())
	if err != nil || check["local_definitions"] != len(entries) {
		t.Fatalf("doctor did not count typed resources: %v %v", check, err)
	}
	if err := reader.pinResources(pins); !errors.Is(err, local.ErrReadOnly) {
		t.Fatalf("read-only resource registration: %v", err)
	}
	runs, after, err := e.Store.ReadAll(context.Background(), 100)
	if err != nil || len(runs) != 0 || before != after {
		t.Fatalf("inventory or doctor mutated authority: %v", err)
	}
}

func TestContextResourceRegistryVersionsAndExplicitNull(t *testing.T) {
	e := contextRegistryRuntime(t)
	entry := contextRegistryEntry(t, e, "resources/value.json", "test:resource/value", "resource", "", "", []byte("true"))
	for _, version := range []string{"1", "2", "3"} {
		t.Run("legacy_json_v"+version, func(t *testing.T) {
			// json.Marshal is not JCS: numeric spellings and HTML escaping
			// must retain the same canonical bytes as the old Inventory.
			canonicalEntry := contextRegistryEntry(t, e, "resources/canonical.json", "test:resource/canonical", "resource", "", "", []byte("{\"value\":1e0,\"html\":\"<&>\"}\n"))
			writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: version, Entries: []Definition{entry, canonicalEntry}})
			_, registry, pins, err := e.inventoryResources()
			if err != nil || len(pins) != 0 || !bytes.Equal(registry[entry.Ref], []byte("true")) || string(registry[canonicalEntry.Ref]) != `{"html":"<&>","value":1}` {
				t.Fatalf("untagged JSON interpretation changed: %v", err)
			}
		})
		for _, field := range []string{"byte_encoding", "media_type"} {
			for _, value := range []any{nil, "", "utf8_text"} {
				t.Run(fmt.Sprintf("v%s_%s_%v", version, field, value), func(t *testing.T) {
					wire := map[string]any{"ref": entry.Ref, "kind": entry.Kind, "path": entry.Path, field: value}
					writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), map[string]any{"schema_version": version, "entries": []any{wire}})
					if _, _, _, err := e.inventoryResources(); err == nil {
						t.Fatal("representation field was accepted without the versioned complete pair")
					}
					if _, err := e.workflowAliases(); err == nil {
						t.Fatal("alias loading ignored invalid registry representation fields")
					}
				})
			}
		}
	}
	for _, value := range []any{nil, ""} {
		writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), map[string]any{"schema_version": "3", "entries": []any{map[string]any{"ref": entry.Ref, "kind": entry.Kind, "path": entry.Path, "byte_encoding": value, "media_type": value}}})
		if _, _, _, err := e.inventoryResources(); err == nil {
			t.Fatal("empty paired representation fields were accepted")
		}
	}
	entry.ByteEncoding, entry.MediaType = "utf8_text", "text/plain"
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{entry}})
	_, registry, resources, err := e.inventoryResources()
	if err != nil || len(resources) != 1 || resources[0].ByteEncoding != "utf8_text" || registry[entry.Ref] != nil {
		t.Fatalf("JSON-looking raw bytes were reinterpreted: %v", err)
	}
	e.Config.Configuration.SemanticsProfile = flow.Profile
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("Foundation accepted typed context resources")
	}
}

func TestContextResourceRegistryRejectsUnsafeOrConflictingEntries(t *testing.T) {
	e := contextRegistryRuntime(t)
	base := contextRegistryEntry(t, e, "resources/data.txt", "test:context/data", "resource", "utf8_text", "text/plain", []byte("true"))
	for _, tc := range []struct {
		name string
		edit func(*Definition)
	}{
		{"definition_not_resource", func(d *Definition) { d.Kind = "schema" }},
		{"unknown_kind", func(d *Definition) { d.Kind = "javascript" }},
		{"unknown_encoding", func(d *Definition) { d.ByteEncoding = "base64" }},
		{"wrong_media", func(d *Definition) { d.MediaType = "application/json" }},
		{"wrong_charset", func(d *Definition) { d.MediaType = "text/plain; charset=latin1" }},
		{"wildcard_media", func(d *Definition) { d.MediaType = "text/*" }},
		{"bad_identity", func(d *Definition) { d.Ref.ID = "../outside" }},
		{"bad_version", func(d *Definition) { d.Ref.Version = "latest" }},
		{"wrong_digest", func(d *Definition) { d.Ref.Digest = rawDigest([]byte("false")) }},
		{"reserved_core_identity", func(d *Definition) { d.Ref.ID = "core:context/data" }},
		{"absolute_path", func(d *Definition) { d.Path = filepath.Join(e.Root, d.Path) }},
		{"parent_path", func(d *Definition) { d.Path = "../data.txt" }},
		{"unclean_path", func(d *Definition) { d.Path = "resources/../resources/data.txt" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := base
			tc.edit(&entry)
			writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{entry}})
			if _, _, _, err := e.inventoryResources(); err == nil {
				t.Fatal("invalid resource entry was accepted")
			}
		})
	}
	if err := os.Symlink("data.txt", filepath.Join(e.Root, "resources/link.txt")); err != nil {
		t.Fatal(err)
	}
	linked := base
	linked.Path = "resources/link.txt"
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{linked}})
	if _, _, _, err := e.inventoryResources(); !errors.Is(err, local.ErrUnsafePath) {
		t.Fatalf("symlink resource: %v", err)
	}
	duplicate := base
	duplicate.ByteEncoding, duplicate.MediaType = "", ""
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{base, duplicate}})
	if _, _, _, err := e.inventoryResources(); err == nil || !strings.Contains(err.Error(), "duplicate registry identity") {
		t.Fatalf("JSON and UTF-8 entries reused the same identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, base.Path), []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	base.Ref.Digest = rawDigest([]byte{0xff})
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{base}})
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("invalid UTF-8 with a correct byte digest was accepted")
	}
}

func TestContextResourceRegistryAliasesAndChecks(t *testing.T) {
	e, options := emptyRuntime(t)
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	definition := flow.CheckDefinition{SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/content", Version: "1.0.0", Title: "Content syntax", Kind: "content", Claim: "content_valid", Executor: flow.Executor{AdapterRef: builtinRef(defs, "core:adapter/local-process"), Operation: "validate"}}
	data, err := canonical(definition)
	if err != nil {
		t.Fatal(err)
	}
	entry := contextRegistryEntry(t, e, "steps/content-check.json", definition.ID, "check", "", "", data)
	file := RegistryFile{SchemaVersion: "3", Entries: []Definition{entry}, Aliases: map[string]string{"child": options.WorkflowFile}}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
	_, registry, resources, err := e.inventoryResources()
	if err != nil || len(resources) != 0 || !bytes.Equal(registry[entry.Ref], data) {
		t.Fatalf("check was not loaded as an explicit JSON definition: %v", err)
	}
	aliases, err := e.workflowAliases()
	if err != nil || len(aliases["child"]) == 0 {
		t.Fatalf("Registry3 alias did not resolve: %v", err)
	}
	for _, version := range []string{"1", "2"} {
		file.SchemaVersion, file.Aliases = version, nil
		writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
		if _, _, _, err := e.inventoryResources(); err == nil {
			t.Fatal("legacy registry accepted the new check kind", version)
		}
	}
	file.SchemaVersion = "3"
	file.Entries[0].Ref.ID = "test:check/other"
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
	if _, _, _, err := e.inventoryResources(); err == nil || !strings.Contains(err.Error(), "ref_identity_mismatch") {
		t.Fatalf("check content identity disagreed with ref: %v", err)
	}
	file.Entries[0] = entry
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
	e.Config.Configuration.SemanticsProfile = flow.Profile
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("Foundation accepted automatic check definitions")
	}
}

func TestContextResourceRegistryAdmitsOnlyCoreToolDescriptors(t *testing.T) {
	e := contextRegistryRuntime(t)
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := flow.ToolDescriptor{SchemaVersion: flow.ToolDescriptorVersion, ID: "test:tool/announce", Version: "1.0.0", AdapterRef: builtinVersionRef(defs, "core:adapter/local-process", "2.0.0"), Operation: "announce", ArgumentsSchemaRef: builtinRef(defs, "core:schema/context-json"), ResultSchemaRef: builtinRef(defs, "core:schema/context-json"), EffectClass: "external_write", RetryClass: "deduplicated", RequiredCapabilities: []string{"network"}}
	data, err := canonical(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	entry := contextRegistryEntry(t, e, "announce.json", descriptor.ID, "tool", "", "", data)
	file := RegistryFile{SchemaVersion: "3", Entries: []Definition{entry}}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
	_, registry, resources, err := e.inventoryResources()
	if err != nil || len(resources) != 0 {
		t.Fatalf("tool descriptor was not loaded: %v", err)
	}
	if _, err := flow.ParseToolDescriptor(registry[entry.Ref]); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1", "2"} {
		file.SchemaVersion = version
		writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
		if _, _, _, err := e.inventoryResources(); err == nil {
			t.Fatal("legacy registry accepted a tool descriptor", version)
		}
	}
	file.SchemaVersion = "3"
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
	e.Config.Configuration.SemanticsProfile = flow.Profile
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("Foundation accepted a tool descriptor")
	}
}

func TestContextResourcePinsShareLegacyIdentityAndExactRetry(t *testing.T) {
	data := []byte("true")
	ref := flow.Ref{ID: "test:resource/identity", Version: "1.0.0", Digest: rawDigest(data)}
	jsonPin := PinnedResource{Ref: ref, RawDigest: rawDigest(data), ByteEncoding: "json", MediaType: "application/json", Bytes: data}
	textPin := jsonPin
	textPin.ByteEncoding, textPin.MediaType = "utf8_text", "text/plain"
	legacy := PinnedDefinition{Ref: ref, Kind: "resource", RawDigest: rawDigest(data), Bytes: data}
	for _, firstLegacy := range []bool{true, false} {
		for _, typed := range []PinnedResource{jsonPin, textPin} {
			t.Run(fmt.Sprintf("legacy_first_%v_%s", firstLegacy, typed.ByteEncoding), func(t *testing.T) {
				e := contextRegistryRuntime(t)
				first := func() error { return e.pinDefinitions([]PinnedDefinition{legacy}) }
				second := func() error { return e.pinResources([]PinnedResource{typed}) }
				if !firstLegacy {
					first, second = second, first
				}
				if err := first(); err != nil {
					t.Fatal(err)
				}
				name := fmt.Sprintf("%x.json", sha256.Sum256([]byte(ref.ID+"@"+ref.Version)))
				path := filepath.Join(e.Root, ".prifly/inventory", name)
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				stat, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := first(); err != nil {
					t.Fatal("exact registration retry failed", err)
				}
				if err := second(); err == nil || !strings.Contains(err.Error(), "definition_drift") {
					t.Fatalf("equal content digest allowed changing interpretation: %v", err)
				}
				current, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				now, err := os.Stat(path)
				if err != nil || !bytes.Equal(current, before) || !now.ModTime().Equal(stat.ModTime()) {
					t.Fatal("retry or conflicting registration rewrote immutable identity", err)
				}
				files, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(files) != 1 {
					t.Fatal("resource registration used a second identity namespace", err)
				}
				runs, _, err := e.Store.ReadAll(context.Background(), 100)
				if err != nil || len(runs) != 0 {
					t.Fatal("resource registration created a Run", err)
				}
			})
		}
	}
	for _, tc := range []struct {
		name string
		pin  PinnedResource
	}{
		{"encoding", textPin},
		{"source_formatting", PinnedResource{Ref: ref, RawDigest: rawDigest([]byte(" true\n")), ByteEncoding: "json", MediaType: "application/json", Bytes: data}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := contextRegistryRuntime(t)
			if err := e.pinResources([]PinnedResource{jsonPin}); err != nil {
				t.Fatal(err)
			}
			if err := e.pinResources([]PinnedResource{tc.pin}); err == nil || !strings.Contains(err.Error(), "definition_drift") {
				t.Fatalf("resource representation/source drift was accepted: %v", err)
			}
		})
	}
	t.Run("media", func(t *testing.T) {
		e := contextRegistryRuntime(t)
		if err := e.pinResources([]PinnedResource{textPin}); err != nil {
			t.Fatal(err)
		}
		textPin.MediaType = "text/markdown"
		if err := e.pinResources([]PinnedResource{textPin}); err == nil || !strings.Contains(err.Error(), "definition_drift") {
			t.Fatalf("media drift was accepted: %v", err)
		}
	})
}

func TestContextResourcePinsValidateWholeSnapshotAndConcurrentRetry(t *testing.T) {
	e := contextRegistryRuntime(t)
	data := []byte("true")
	base := PinnedResource{Ref: flow.Ref{ID: "test:context/data", Version: "1.0.0", Digest: rawDigest(data)}, RawDigest: rawDigest(data), ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: data}
	for _, tc := range []struct {
		name string
		edit func(*PinnedResource)
	}{
		{"invalid_raw_digest", func(p *PinnedResource) { p.RawDigest = "sha256:abc" }},
		{"uppercase_raw_digest", func(p *PinnedResource) { p.RawDigest = strings.ToUpper(p.RawDigest) }},
		{"wrong_raw_digest", func(p *PinnedResource) { p.RawDigest = rawDigest([]byte("false")) }},
		{"wrong_bytes", func(p *PinnedResource) { p.Bytes = []byte("false") }},
		{"null_bytes", func(p *PinnedResource) { p.Bytes = nil; p.Ref.Digest = rawDigest(nil); p.RawDigest = p.Ref.Digest }},
		{"uncanonical_json", func(p *PinnedResource) {
			p.ByteEncoding, p.MediaType, p.Bytes = "json", "application/json", []byte(" true ")
		}},
		{"reserved_identity", func(p *PinnedResource) { p.Ref.ID = "core:context/data" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := base
			invalid.Ref.ID = "test:context/invalid"
			tc.edit(&invalid)
			if err := e.pinResources([]PinnedResource{base, invalid}); err == nil {
				t.Fatal("invalid snapshot was registered")
			}
			files, err := os.ReadDir(filepath.Join(e.Root, ".prifly/inventory"))
			if err != nil || len(files) != 0 {
				t.Fatal("invalid later pin partially registered an earlier identity", err)
			}
		})
	}
	if _, err := resourcesFromPins([]PinnedResource{base, base}); err == nil {
		t.Fatal("duplicate snapshot identity was accepted")
	}
	errorsSeen := make(chan error, 2)
	for range 2 {
		go func() { errorsSeen <- e.pinResources([]PinnedResource{base}) }()
	}
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatal("concurrent exact registration was not idempotent", err)
		}
	}
	t.Run("competing_representations", func(t *testing.T) {
		e := contextRegistryRuntime(t)
		other := base
		other.ByteEncoding, other.MediaType = "json", "application/json"
		for _, pin := range []PinnedResource{base, other} {
			go func() { errorsSeen <- e.pinResources([]PinnedResource{pin}) }()
		}
		successes := 0
		for range 2 {
			if err := <-errorsSeen; err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatal("competing representations did not have exactly one immutable winner", successes)
		}
	})
}

func TestContextResourceInventoryCombinedLimits(t *testing.T) {
	e := contextRegistryRuntime(t)
	base := contextRegistryEntry(t, e, "resources/empty.txt", "test:context/base", "resource", "utf8_text", "text/plain", nil)
	legacy := contextRegistryEntry(t, e, "resources/legacy.json", "test:resource/legacy", "resource", "", "", []byte("true"))
	entries := []Definition{legacy}
	for i := range 511 {
		entry := base
		entry.Ref.ID = fmt.Sprintf("test:context/entry-%d", i)
		entries = append(entries, entry)
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	_, _, pins, err := e.inventoryResources()
	if err != nil || len(pins) != 511 {
		t.Fatalf("512 combined entries were refused: %v", err)
	}
	encoded, err := canonical(pins[0])
	if err != nil || !bytes.Contains(encoded, []byte(`"bytes":""`)) {
		t.Fatalf("empty source became null instead of an empty byte string: %v", err)
	}
	extra := pins[0]
	extra.Ref.ID = "test:context/snapshot-extra"
	pins = append(pins, extra)
	if loaded, err := resourcesFromPins(pins); err != nil || len(loaded) != 512 {
		t.Fatalf("512 pinned resources were refused: %v", err)
	}
	extra.Ref.ID = "test:context/snapshot-overflow"
	if _, err := resourcesFromPins(append(pins, extra)); err == nil {
		t.Fatal("513 pinned resources were accepted")
	}
	entries = append(entries, base)
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("513 combined entries were accepted")
	}
	large := bytes.Repeat([]byte("x"), MaxDefinitionBytes)
	base = contextRegistryEntry(t, e, "resources/large.txt", "test:context/large", "resource", "utf8_text", "text/plain", large)
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: []Definition{base}})
	if _, _, pins, err := e.inventoryResources(); err != nil || len(pins) != 1 || len(pins[0].Bytes) != MaxDefinitionBytes {
		t.Fatalf("exact 2 MiB resource was refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, base.Path), append(large, 'x'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := e.inventoryResources(); err == nil {
		t.Fatal("resource larger than 2 MiB was accepted")
	}
	if err := os.WriteFile(filepath.Join(e.Root, base.Path), large, 0600); err != nil {
		t.Fatal(err)
	}
	// The same real file can be registered under independent identities. This
	// exercises cumulative loading without manufacturing giant Go snapshots.
	entries = nil
	for i := range 31 {
		entry := base
		entry.Ref.ID = fmt.Sprintf("test:context/large-%d", i)
		entries = append(entries, entry)
	}
	jsonLarge := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, MaxDefinitionBytes-2)...)
	jsonLarge = append(jsonLarge, '"')
	entries = append(entries, contextRegistryEntry(t, e, "resources/large.json", "test:resource/large", "resource", "", "", jsonLarge))
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	if _, _, pins, err := e.inventoryResources(); err != nil || len(pins) != 31 {
		t.Fatalf("exact 64 MiB mixed inventory was refused: %v", err)
	}
	entries = append(entries, legacy)
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	if _, _, _, err := e.inventoryResources(); err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Fatalf("mixed inventory larger than 64 MiB was accepted: %v", err)
	}
}
