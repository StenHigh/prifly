package runtime

import (
	"errors"
	"fmt"
	"testing"
)

func TestSourceWorktreeNamingRetainedEvidence(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"direct", "session", "ancestry", "released", "unrelated", "semantic-fork", "missing", "cycle", "depth", "ambiguous", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			snapshot := namingSnapshotFixture(t, 0)
			claim := WorktreeClaim{ID: "claim:original", RunID: "tail", Naming: &WorktreeNameAllocation{Base: "demo-issue-19", Policy: "suffix"}, NamingSnapshot: snapshot}
			runs := map[string]Run{"source": {ID: "source"}, "tail": {ID: "tail", Fork: &ForkProvenance{SourceRunID: "middle", Reason: ContinuationReason}}, "middle": {ID: "middle", Fork: &ForkProvenance{SourceRunID: "source", Reason: ResumeReason}}}
			want, wantErr := true, false
			switch mode {
			case "direct":
				claim.RunID = "source"
			case "session":
				runs["source"] = Run{ID: "source", Attempts: map[string]*Attempt{"a": {Session: &SessionHandoff{ClaimID: claim.ID}}}}
				delete(runs, "tail")
			case "released":
				claim.Status = "released"
			case "unrelated":
				runs["tail"] = Run{ID: "tail"}
				want = false
			case "semantic-fork":
				runs["tail"] = Run{ID: "tail", Fork: &ForkProvenance{SourceRunID: "source", Reason: "semantic fork"}}
				want = false
			case "missing":
				delete(runs, "middle")
				wantErr = true
			case "cycle":
				runs["middle"] = Run{ID: "middle", Fork: &ForkProvenance{SourceRunID: "tail", Reason: RecoveryReason}}
				wantErr = true
			case "depth":
				for i := 0; i < continuationChainLimit+1; i++ {
					id := fmt.Sprint(i)
					runs[id] = Run{ID: id, Fork: &ForkProvenance{SourceRunID: fmt.Sprint(i + 1), Reason: RecoveryReason}}
				}
				claim.RunID = "0"
				wantErr = true
			case "ambiguous":
				wantErr = true
			case "legacy":
				claim.RunID = "source"
				claim.Naming = nil
				claim.NamingSnapshot = nil
			}
			claims := []WorktreeClaim{claim}
			if mode == "ambiguous" {
				other := claim
				other.ID = "claim:other"
				other.Naming = &WorktreeNameAllocation{Base: "different", Policy: "suffix"}
				claims = append(claims, other)
			}
			missing := errors.New("missing manifest")
			reads := map[string]int{}
			got, err := sourceWorktreeNaming(claims, "source", func(id string) (Run, error) {
				reads[id]++
				run, ok := runs[id]
				if !ok {
					return Run{}, missing
				}
				return run, nil
			})
			if wantErr {
				if err == nil {
					t.Fatal("invalid ancestry accepted", got)
				}
				if mode == "missing" && !errors.Is(err, missing) {
					t.Fatal("load failure hidden", err)
				}
				if mode != "missing" {
					var f *Fault
					if !errors.As(err, &f) || f.Code != "project_worktree_binding_invalid" {
						t.Fatal("untyped ancestry refusal", err)
					}
				}
				return
			}
			if err != nil || (got != nil) != want {
				t.Fatal(got, err)
			}
			if got != nil && (got.ID != claim.ID || !EqualWorktreeNamingSnapshot(got.NamingSnapshot, claim.NamingSnapshot)) {
				t.Fatal("wrong evidence", got)
			}
			if got != nil && got.NamingSnapshot != nil {
				got.NamingSnapshot.Inputs["identity"] = WorktreeNamingInput{}
				if EqualWorktreeNamingSnapshot(got.NamingSnapshot, claim.NamingSnapshot) {
					t.Fatal("snapshot aliases stored evidence")
				}
			}
			for id, count := range reads {
				if count > 1 {
					t.Fatal("manifest not cached", id, count)
				}
			}
		})
	}
}
