package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func namingSnapshotFixture(t *testing.T, padding int) *WorktreeNamingSnapshot {
	t.Helper()
	schema := json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	value, _ := json.Marshal(map[string]string{"id": "issue-19", "padding": strings.Repeat("x", padding)})
	sd, _ := flow.Digest(schema)
	vd, _ := flow.Digest(value)
	return &WorktreeNamingSnapshot{SchemaVersion: "project-worktree-naming-snapshot/1", Declaration: json.RawMessage(`{"template":"demo-{id}","inputs":{"identity":{"schema":".prifly/naming/identity.json"}},"bindings":{"id":{"naming_input":"identity","pointer":"/id"}}}`), Inputs: map[string]WorktreeNamingInput{"identity": {SchemaPath: ".prifly/naming/identity.json", ValueSource: "/tmp/identity.json", Schema: schema, SchemaDigest: sd, Value: value, ValueDigest: vd}}}
}

func TestNamingSnapshotAggregateByteBoundary(t *testing.T) {
	t.Parallel()
	base := namingSnapshotFixture(t, 0)
	data, err := canonical(base)
	if err != nil {
		t.Fatal(err)
	}
	exact := namingSnapshotFixture(t, MaxWorktreeNamingSnapshotBytes-len(data))
	data, err = canonical(exact)
	if err != nil || len(data) != 131072 {
		t.Fatal(len(data), err)
	}
	if err := ValidateWorktreeNamingSnapshot(exact); err != nil {
		t.Fatal(err)
	}
	over := namingSnapshotFixture(t, MaxWorktreeNamingSnapshotBytes-lenMustCanonical(t, base)+1)
	var f *Fault
	if err := ValidateWorktreeNamingSnapshot(over); !errors.As(err, &f) || f.Code != "project_worktree_snapshot_too_large" {
		t.Fatal(err)
	}
	// The before/after command remains inside its unchanged 1 MiB budget.
	payload, err := canonical(map[string]any{"before": WorktreeClaim{NamingSnapshot: exact}, "after": WorktreeClaim{NamingSnapshot: exact}})
	if err != nil || len(payload) >= local.MaxCommandBytes {
		t.Fatal(len(payload), err)
	}
	copy := CloneWorktreeNamingSnapshot(exact)
	copy.Inputs["identity"] = base.Inputs["identity"]
	if EqualWorktreeNamingSnapshot(copy, exact) {
		t.Fatal("copy aliases original snapshot")
	}
}

func lenMustCanonical(t *testing.T, value any) int {
	t.Helper()
	data, err := canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return len(data)
}

func TestNamedSnapshotPersistenceAndSemanticIdempotency(t *testing.T) {
	t.Parallel()
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	snapshot := namingSnapshotFixture(t, 1)
	request := ClaimRequest{CommandID: "command:snapshot", Repository: repository, OwnerID: "owner:snapshot", Naming: &WorktreeNamingRequest{Base: "demo-issue-19", Policy: WorktreeNamingPolicy, Snapshot: snapshot}}
	claim, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	duplicate, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
	if err != nil || created || duplicate.ID != claim.ID {
		t.Fatal(duplicate, created, err)
	}
	changed := request
	changed.Naming = &WorktreeNamingRequest{Base: request.Naming.Base, Policy: WorktreeNamingPolicy, Snapshot: namingSnapshotFixture(t, 2)}
	if _, _, err := e.ClaimWorktreeWithOutcome(ctx, changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatal("snapshot absent from command identity", err)
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	record, err := reader.Claims(ctx)
	if err != nil || record.SchemaVersion != SnapshotAuthorityClaimsVersion || len(record.Claims) != 1 || !EqualWorktreeNamingSnapshot(record.Claims[0].NamingSnapshot, snapshot) {
		t.Fatal(record, err)
	}
	raw, err := reader.Store.ReadAuthority(ctx, AuthorityClaimsKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "AuthorityClaimsV5", json.RawMessage(raw.Data)); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(string(raw.Data), SnapshotAuthorityClaimsVersion, NamedAuthorityClaimsVersion, 1)
	if _, err := decodeClaimRecord(local.AuthoritySnapshot{Version: 1, Data: []byte(legacy)}, e.Installation.ID); err == nil {
		t.Fatal("old record accepted new snapshot")
	}
}

func TestSnapshotClaimsUseBoundedStrictStateReader(t *testing.T) {
	t.Parallel()
	record := ClaimRecord{SchemaVersion: SnapshotAuthorityClaimsVersion, AuthorityID: "authority:test", Claims: []WorktreeClaim{}}
	for i := 1; i <= 20; i++ {
		path, branch := worktreeCandidate("large", i)
		record.Claims = append(record.Claims, WorktreeClaim{Mode: "worktree", Path: path, Branch: branch, Naming: &WorktreeNameAllocation{Base: "large", Policy: WorktreeNamingPolicy, Ordinal: i, Phase: "tree"}, NamingSnapshot: namingSnapshotFixture(t, 120000)})
	}
	data, err := canonicalState(record)
	if err != nil || len(data) <= flow.MaxDocumentBytes {
		t.Fatal(len(data), err)
	}
	if _, err := decodeClaimRecord(local.AuthoritySnapshot{Version: 1, Data: data}, record.AuthorityID); err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	json.Unmarshal(data, &object)
	object["unknown"] = true
	bad, _ := json.Marshal(object)
	if _, err := decodeClaimRecord(local.AuthoritySnapshot{Version: 1, Data: bad}, record.AuthorityID); err == nil {
		t.Fatal("unknown state field accepted")
	}
	object["schema_version"] = NamedAuthorityClaimsVersion
	delete(object, "unknown")
	old, _ := json.Marshal(object)
	if _, err := decodeClaimRecord(local.AuthoritySnapshot{Version: 1, Data: old}, record.AuthorityID); err == nil {
		t.Fatal("legacy reader boundary widened")
	}
	if err := checkNamingRecordCapacity(ClaimRecord{Claims: make([]WorktreeClaim, 0)}, WorktreeClaim{NamingSnapshot: namingSnapshotFixture(t, local.MaxSnapshotBytes)}); err == nil {
		t.Fatal("oversized record accepted")
	}
}
