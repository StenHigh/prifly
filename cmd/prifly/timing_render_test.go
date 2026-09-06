package main

import (
	"bytes"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The pilot's live tree printed `elapsed=known 0ms` for an attempt whose host
// had worked for seventeen minutes: the known part is the prefix one clock
// session could measure, and the tree carries the wall-clock value that spans
// the whole attempt. The line must lead with the one that answers the question.
func TestRenderTimingNamesTheClockBehindEachNumber(t *testing.T) {
	measured, estimate, known := int64(1413), int64(1062000), int64(0)
	tree := prifly.TimingTree{
		CalculatorRevision: prifly.TimingCalculatorRevisionContext, RunID: "run:pilot",
		AsOf: prifly.Observation{UTC: "2026-09-06T12:00:00Z"},
		Root: prifly.TimingNode{Kind: "run", ID: "run:pilot", Status: "running", Metrics: map[string]prifly.Duration{
			"elapsed": {Quality: "measured", ValueMS: &measured, KnownMS: &measured},
		}, Children: []prifly.TimingNode{
			{Kind: "attempt", ID: "attempt:host", Status: "completed", Metrics: map[string]prifly.Duration{
				"elapsed": {Quality: "partial", KnownMS: &known, EstimateMS: &estimate, Reasons: []string{"incomparable_clock_domains", "authority_wall_estimate"}},
			}},
			{Kind: "attempt", ID: "attempt:lower-bound", Status: "running", Metrics: map[string]prifly.Duration{
				"elapsed": {Quality: "partial", KnownMS: &known, IsOpen: true, Reasons: []string{"incomparable_clock_domains"}},
			}},
		}},
	}
	var out bytes.Buffer
	if err := renderTiming(&out, tree, 42); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected tree shape: %q", out.String())
	}
	for line, want := range map[int]string{
		1: "elapsed=1413ms measured",
		2: "elapsed=~1062000ms by wall clock",
		3: "elapsed=at least 0ms measured",
	} {
		if !strings.Contains(lines[line], want) {
			t.Fatalf("line %d does not say which clock produced its number: %q want %q", line, lines[line], want)
		}
	}
	if strings.Contains(out.String(), "known 0ms") {
		t.Fatalf("a control-loop prefix is still printed as the elapsed value: %q", out.String())
	}
}
