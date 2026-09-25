package runtime

import (
	"strconv"
	"strings"
	"testing"
)

// A launch review printed 519 definitions against a limit of 512 and said
// nothing about the order of those two numbers; the refusal arrived at start.
// The review is the one place that exists to have made that comparison for the
// reader, and it has to make it on the same side of the bound the refusal does.
func TestTheBudgetNamesTheRefusalItsOwnNumbersImply(t *testing.T) {
	for _, test := range []struct {
		entries int
		refusal string
	}{
		{MaxLocalRegistryEntries - 1, ""},
		{MaxLocalRegistryEntries, ""},
		{MaxLocalRegistryEntries + 1, "dependency_limit"},
		{MaxLocalRegistryEntries + 7, "dependency_limit"},
	} {
		budget := registryBudget(test.entries)
		if budget.Entries != test.entries || budget.Limit != MaxLocalRegistryEntries {
			t.Fatalf("the budget reported %d/%d for %d", budget.Entries, budget.Limit, test.entries)
		}
		if budget.WouldRefuse != test.refusal {
			t.Errorf("at %d entries the review names %q, the refusal answers %q", test.entries, budget.WouldRefuse, test.refusal)
		}
	}
	// The bound is the refusal's own: localRegistry refuses above it, not at
	// it, so a review that refused at it would be wrong in the other direction.
	if registryBudget(MaxLocalRegistryEntries).WouldRefuse != "" {
		t.Fatal("a count that starts was reported as one that would be refused")
	}
}

// A continuation Run is never an admissible continuation source, and the Run
// that is one is recorded in its own provenance. Refusing without naming it
// sent a host to rebuild the chain by hand from a fact the answer was holding.
func TestARefusedContinuationSourceNamesTheOneItCameFrom(t *testing.T) {
	classic := Run{ID: "run:classic"}
	first := Run{ID: "run:first", Fork: &ForkProvenance{SourceRunID: classic.ID, Reason: ContinuationReason}}
	second := Run{ID: "run:second", Fork: &ForkProvenance{SourceRunID: first.ID, Reason: ContinuationReason}}
	stored := map[string]Run{classic.ID: classic, first.ID: first, second.ID: second}
	load := func(id string) (Run, bool) { r, ok := stored[id]; return r, ok }

	if origin := continuationOriginOf(second, load); origin != classic.ID {
		t.Fatalf("a continuation of a continuation named %q as its origin", origin)
	}
	if origin := continuationOriginOf(first, load); origin != classic.ID {
		t.Fatalf("a continuation named %q as its origin", origin)
	}
	// A Run that is not a continuation has no origin to offer: the refusal
	// keeps its own reason rather than pointing at the Run it was given.
	if origin := continuationOriginOf(classic, load); origin != "" {
		t.Fatalf("a Run that is not a continuation was given the origin %q", origin)
	}
	// A chain this build cannot read through is absent, never guessed.
	broken := Run{ID: "run:orphan", Fork: &ForkProvenance{SourceRunID: "run:missing", Reason: ContinuationReason}}
	if origin := continuationOriginOf(broken, load); origin != "" {
		t.Fatalf("an unreadable chain produced the origin %q", origin)
	}
	// A fork made for some other reason is not a continuation chain.
	other := Run{ID: "run:forked", Fork: &ForkProvenance{SourceRunID: classic.ID, Reason: "semantic rework"}}
	if origin := continuationOriginOf(other, load); origin != "" {
		t.Fatalf("a fork that is not a continuation named the origin %q", origin)
	}
}

// The walk is bounded, and the bound is the property being claimed: no stored
// Run can be made to point at itself, so nothing else can demonstrate it.
func TestTheContinuationWalkIsBounded(t *testing.T) {
	chain := map[string]Run{}
	for i := 0; i <= continuationChainLimit+4; i++ {
		id := "run:" + strconv.Itoa(i)
		chain[id] = Run{ID: id, Fork: &ForkProvenance{SourceRunID: "run:" + strconv.Itoa(i+1), Reason: ContinuationReason}}
	}
	load := func(id string) (Run, bool) { r, ok := chain[id]; return r, ok }
	// Longer than the walk: it stops and names nothing rather than reporting
	// whichever Run it happened to reach.
	if origin := continuationOriginOf(chain["run:0"], load); origin != "" {
		t.Fatalf("a chain past the bound named %q", origin)
	}
	// A cycle is the case a bound exists for at all.
	loop := Run{ID: "run:loop", Fork: &ForkProvenance{SourceRunID: "run:loop", Reason: ContinuationReason}}
	cyclic := func(string) (Run, bool) { return loop, true }
	done := make(chan string, 1)
	go func() { done <- continuationOriginOf(loop, cyclic) }()
	if origin := <-done; origin != "" {
		t.Fatalf("a cycle named %q", origin)
	}
}

// The refusal keeps its code and grows only its detail: a code inside error
// text is what refusal-check exists to refuse.
func TestTheNamedSourceIsDetailAndNotANewCode(t *testing.T) {
	r := Run{ID: "run:continuation", Status: "completed", Outcome: stringPointer("partial"),
		Fork: &ForkProvenance{SourceRunID: "run:classic", Reason: ContinuationReason}}
	_, err := continuationRefs(r, 1)
	if err == nil {
		t.Fatal("a continuation Run was accepted as a continuation source")
	}
	if !strings.Contains(err.Error(), "continuation_source_ineligible") {
		t.Fatalf("the refusal lost its code: %v", err)
	}
}
