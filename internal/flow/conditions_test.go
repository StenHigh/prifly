package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func conditionLiteral(raw string) *Operand {
	return &Operand{Kind: "literal", Value: json.RawMessage(raw)}
}

func conditionField(port string) *Operand {
	return &Operand{Kind: "field", Ref: &FieldRef{From: "workflow_input", Port: port}}
}

func conditionFixturePredicate(value string) Predicate {
	p := Predicate{Op: "eq", Left: conditionLiteral("true"), Right: conditionLiteral("true")}
	switch value {
	case "false":
		p.Right = conditionLiteral("false")
	case "unknown":
		p.Left = conditionField("missing")
	case "error":
		p.Right = conditionLiteral(`"true"`)
	}
	return p
}

func conditionFixturePlan(selection string, values ...string) *Plan {
	p := &Plan{Profile: CoreProfile}
	s := Stage{Kind: "choice", Selection: selection}
	for i, value := range values {
		s.Branches = append(s.Branches, ChoiceBranch{ID: fmt.Sprintf("b%d", i), Predicate: conditionFixturePredicate(value), Next: fmt.Sprintf("next%d", i)})
	}
	p.Workflow.Definition.Stages = map[string]Stage{"choose": s}
	return p
}

func TestControlScalarExactNumbers(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    int64
		invalid bool
	}{
		{"0", 0, false}, {"-0.0", 0, false}, {"1", 1, false}, {"1.0", 1, false}, {"1e0", 1, false},
		{"1E+00000000000000000000001", 10, false}, {"1000000000000000000000000e-24", 1, false},
		{"-3.000e+2", -300, false}, {"9007199254740991", maxSafeInteger, false}, {"-9007199254740991", -maxSafeInteger, false},
		{"9007199254740991000e-3", maxSafeInteger, false}, {"0e-999999999999999999999999999999", 0, false},
		{"1.5", 0, true}, {"1.0000000000000000001", 0, true}, {"9007199254740990.9", 0, true},
		{"9007199254740991.1", 0, true}, {"-9007199254740991.1", 0, true},
		{"9007199254740992", 0, true}, {"-9007199254740992", 0, true},
		{"1e-1000000", 0, true}, {"1e1000000", 0, true}, {"1e-99999999999999999999999999", 0, true},
		{"1e9223372036854775807", 0, true}, {"1e-9223372036854775808", 0, true},
		{"NaN", 0, true}, {"Infinity", 0, true}, {"01", 0, true}, {"+1", 0, true}, {"1e", 0, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			value, ok := controlInteger(tc.raw)
			if ok == tc.invalid || ok && value != tc.want {
				t.Fatalf("exact integer %q = %d, valid=%v", tc.raw, value, ok)
			}
			predicate := Predicate{Op: "eq", Left: conditionField("number"), Right: conditionLiteral(fmt.Sprint(tc.want))}
			nodes := 0
			truth, err := evaluatePredicate(predicate, func(FieldRef) (any, bool, error) { return json.Number(tc.raw), true, nil }, "", 1, &nodes)
			if tc.invalid {
				expectProblem(t, err, "condition_type_mismatch")
			} else if err != nil || truth != TruthTrue {
				t.Fatalf("equal decimal representations disagreed: %s %v", truth, err)
			}
		})
	}
	if _, ok := controlInteger("1e" + strings.Repeat("0", MaxDocumentBytes)); ok {
		t.Fatal("unbounded numeric input accepted")
	}
}

func TestPredicateTruthAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right any
		missing     bool
		want        Truth
		code        string
	}{
		{name: "null equals null", want: TruthTrue},
		{name: "false equals false", left: false, right: false, want: TruthTrue},
		{name: "strings unequal", left: "a", right: "b", want: TruthFalse},
		{name: "safe representations equal", left: json.Number("1e0"), right: json.Number("1.0"), want: TruthTrue},
		{name: "absent is unknown", right: nil, missing: true, want: TruthUnknown},
		{name: "null is not boolean", left: nil, right: false, code: "condition_type_mismatch"},
		{name: "string is not boolean", left: "false", right: false, code: "condition_type_mismatch"},
		{name: "string is not number", left: "1", right: json.Number("1"), code: "condition_type_mismatch"},
		{name: "array is not scalar", left: []any{}, right: []any{}, code: "condition_type_mismatch"},
		{name: "object is not scalar", left: map[string]any{}, right: nil, code: "condition_type_mismatch"},
		{name: "missing does not hide bad peer", missing: true, right: map[string]any{}, code: "condition_type_mismatch"},
		{name: "fraction is not control scalar", left: json.Number("0.5"), right: json.Number("0.5"), code: "condition_type_mismatch"},
		{name: "untyped float already lost exact syntax", left: float64(1), right: json.Number("1"), code: "condition_type_mismatch"},
	} {
		for _, op := range []string{"eq", "ne"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				nodes := 0
				predicate := Predicate{Op: op, Left: conditionField("left"), Right: conditionField("right")}
				truth, err := evaluatePredicate(predicate, func(ref FieldRef) (any, bool, error) {
					if ref.Port == "left" {
						return tc.left, !tc.missing, nil
					}
					return tc.right, true, nil
				}, "", 1, &nodes)
				if tc.code != "" {
					expectProblem(t, err, tc.code)
					return
				}
				want := tc.want
				if op == "ne" && want != TruthUnknown {
					want = truthOf(want == TruthFalse)
				}
				if err != nil || truth != want {
					t.Fatalf("want %s, got %s: %v", want, truth, err)
				}
			})
		}
	}
	for _, present := range []bool{false, true} {
		for _, value := range []any{nil, false, "", json.Number("0.5"), []any{}, map[string]any{}} {
			nodes := 0
			truth, err := evaluatePredicate(Predicate{Op: "exists", Ref: &FieldRef{From: "workflow_input", Port: "value"}}, func(FieldRef) (any, bool, error) { return value, present, nil }, "", 1, &nodes)
			if err != nil || truth != truthOf(present) {
				t.Fatalf("exists coerced a value: %T present=%v result=%s error=%v", value, present, truth, err)
			}
		}
	}
	for _, op := range []string{"all", "any"} {
		for _, left := range []string{"true", "false", "unknown"} {
			for _, right := range []string{"true", "false", "unknown"} {
				t.Run(op+"/"+left+"/"+right, func(t *testing.T) {
					want := TruthUnknown
					if op == "all" {
						if left == "false" || right == "false" {
							want = TruthFalse
						} else if left == "true" && right == "true" {
							want = TruthTrue
						}
					} else if left == "true" || right == "true" {
						want = TruthTrue
					} else if left == "false" && right == "false" {
						want = TruthFalse
					}
					nodes := 0
					truth, err := evaluatePredicate(Predicate{Op: op, Args: []Predicate{conditionFixturePredicate(left), conditionFixturePredicate(right)}}, func(FieldRef) (any, bool, error) { return nil, false, nil }, "", 1, &nodes)
					if err != nil || truth != want {
						t.Fatalf("want %s, got %s: %v", want, truth, err)
					}
				})
			}
		}
		first := "false"
		if op == "any" {
			first = "true"
		}
		nodes := 0
		_, err := evaluatePredicate(Predicate{Op: op, Args: []Predicate{conditionFixturePredicate(first), conditionFixturePredicate("error")}}, nil, "", 1, &nodes)
		expectProblem(t, err, "condition_type_mismatch")
	}
}

func TestChoiceSelection(t *testing.T) {
	for _, tc := range []struct {
		name, selection, values, route, branch, code, errorBranch string
		unknownRoute, defaultRoute                                bool
		prefix                                                    int
	}{
		{name: "exclusive unique", values: "false,true,false", route: "branch", branch: "b1", prefix: 3},
		{name: "exclusive ambiguity", values: "true,true", code: "ambiguous_branch", prefix: 2},
		{name: "ambiguity precedes unknown", values: "true,unknown,true", code: "ambiguous_branch", prefix: 3},
		{name: "actual error precedes ambiguity", values: "true,true,error", code: "condition_type_mismatch", errorBranch: "b2", prefix: 2},
		{name: "actual error precedes unknown", values: "unknown,error", code: "condition_type_mismatch", errorBranch: "b1", prefix: 1},
		{name: "unknown has no implicit default", values: "false,unknown", defaultRoute: true, code: "condition_unknown", prefix: 2},
		{name: "unknown takes explicit route", values: "true,unknown", unknownRoute: true, route: "on_unknown", prefix: 2},
		{name: "all false explicit default", values: "false,false", defaultRoute: true, route: "default", prefix: 2},
		{name: "all false no transition", values: "false,false", code: "no_transition", prefix: 2},
		{name: "first match wins", selection: "first_match", values: "false,true,true", route: "branch", branch: "b1", prefix: 2},
		{name: "later error not evaluated", selection: "first_match", values: "true,error", route: "branch", branch: "b0", prefix: 1},
		{name: "later unknown irrelevant", selection: "first_match", values: "true,unknown", route: "branch", branch: "b0", prefix: 1},
		{name: "preceding unknown blocks true", selection: "first_match", values: "false,unknown,true", code: "condition_unknown", prefix: 2},
		{name: "preceding unknown routes without later error", selection: "first_match", values: "unknown,error", unknownRoute: true, route: "on_unknown", prefix: 1},
		{name: "first match default", selection: "first_match", values: "false,false", defaultRoute: true, route: "default", prefix: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selection := tc.selection
			if selection == "" {
				selection = "exclusive"
			}
			values := strings.Split(tc.values, ",")
			p := conditionFixturePlan(selection, values...)
			stage := p.Workflow.Definition.Stages["choose"]
			stage.OnError = "error-handler"
			if tc.defaultRoute {
				stage.Default = "fallback"
			}
			if tc.unknownRoute {
				stage.OnUnknown = "clarify"
			}
			p.Workflow.Definition.Stages["choose"] = stage
			result, err := p.SelectChoice("choose", func(FieldRef) (any, bool, error) { return nil, false, nil })
			if tc.code != "" {
				expectProblem(t, err, tc.code)
			} else if err != nil {
				t.Fatal(err)
			}
			if result.Route != tc.route || result.BranchID != tc.branch || result.ErrorBranchID != tc.errorBranch || len(result.Evaluations) != tc.prefix {
				t.Fatalf("wrong route or evaluated prefix: %+v", result)
			}
			for i, evaluation := range result.Evaluations {
				if evaluation.BranchID != fmt.Sprintf("b%d", i) || string(evaluation.Result) != values[i] {
					t.Fatalf("pinned branch order changed: %+v", result)
				}
			}
			wantNext := ""
			switch result.Route {
			case "branch":
				wantNext = "next" + strings.TrimPrefix(tc.branch, "b")
			case "default":
				wantNext = "fallback"
			case "on_unknown":
				wantNext = "clarify"
			}
			if result.NextStageID != wantNext {
				t.Fatalf("wrong selected destination: %+v", result)
			}
		})
	}
	p := conditionFixturePlan("exclusive", "unknown", "unknown")
	sentinel := errors.New("verified input could not be read")
	calls := 0
	result, err := p.SelectChoice("choose", func(FieldRef) (any, bool, error) { calls++; return nil, false, sentinel })
	if !errors.Is(err, sentinel) || calls != 1 || result.ErrorBranchID != "b0" || len(result.Evaluations) != 0 {
		t.Fatalf("first input error did not stop evaluation: %+v %v calls=%d", result, err, calls)
	}
	p.Profile = Profile
	_, err = p.SelectChoice("choose", nil)
	expectProblem(t, err, "unsupported")
}

func conditionNested(depth int) Predicate {
	p := conditionFixturePredicate("true")
	for i := 1; i < depth; i++ {
		p = Predicate{Op: "all", Args: []Predicate{p}}
	}
	return p
}

func conditionSizedChoice(nodes int) Stage {
	s := Stage{Kind: "choice", Selection: "exclusive"}
	for i := range 4 {
		count := nodes / 4
		if i < nodes%4 {
			count++
		}
		predicate := Predicate{Op: "all"}
		for j := 1; j < count; j++ {
			predicate.Args = append(predicate.Args, conditionFixturePredicate("true"))
		}
		s.Branches = append(s.Branches, ChoiceBranch{ID: fmt.Sprintf("b%d", i), Predicate: predicate, Next: "done"})
	}
	return s
}

func conditionWorkflow(t *testing.T, choice Stage) (map[string]any, Registry) {
	t.Helper()
	w, registry := fixture(t)
	w["inputs"], w["outputs"] = map[string]InputPort{}, map[string]OutputPort{}
	w["allowed_outcomes"] = []string{"succeeded"}
	w["definition"] = map[string]any{"entry": "choose", "stages": map[string]Stage{
		"choose": choice,
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}},
	}}
	return w, registry
}

func TestPredicatePreflightLimits(t *testing.T) {
	for _, depth := range []int{16, 17} {
		t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
			predicate := conditionNested(depth)
			choice := Stage{Kind: "choice", Selection: "exclusive", Branches: []ChoiceBranch{{ID: "one", Predicate: predicate, Next: "done"}}}
			w, registry := conditionWorkflow(t, choice)
			for _, contract := range []string{"Predicate", "ChoiceStage", "Stage", "WorkflowRevision"} {
				value := any(choice)
				if contract == "Predicate" {
					value = predicate
				} else if contract == "WorkflowRevision" {
					value = w
				}
				err := ValidateProtocol(contract, encoded(t, value))
				if depth > MaxPredicateDepth {
					expectProblem(t, err, "predicate_limit")
				} else if err != nil {
					t.Fatalf("operand/ref objects counted as AST depth: %s %v", contract, err)
				}
			}
			_, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile)
			if depth > MaxPredicateDepth {
				expectProblem(t, err, "predicate_limit")
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, count := range []int{256, 257} {
		t.Run(fmt.Sprintf("operators%d", count), func(t *testing.T) {
			choice := conditionSizedChoice(count)
			w, registry := conditionWorkflow(t, choice)
			for _, contract := range []string{"ChoiceStage", "WorkflowRevision", "WorkflowRevisionV2"} {
				value := any(choice)
				if contract != "ChoiceStage" {
					value = w
					w["schema_version"] = "1"
					if contract == "WorkflowRevisionV2" {
						w["schema_version"] = "2"
					}
				}
				err := ValidateProtocol(contract, encoded(t, value))
				if count > MaxPredicateNodes {
					expectProblem(t, err, "predicate_limit")
				} else if err != nil {
					t.Fatalf("valid aggregate operator count rejected: %s %v", contract, err)
				}
			}
			p, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile)
			if count > MaxPredicateNodes {
				expectProblem(t, err, "predicate_limit")
			} else if err != nil || !slices.Equal(p.Sequence, []string{"choose", "done"}) {
				t.Fatalf("valid bounded choice did not compile: %+v %v", p, err)
			}
		})
	}
	// A malformed discriminator cannot make recursive union validation walk an
	// unbounded branch predicate before reporting the unrelated shape error.
	malformed := map[string]any{"kind": "step", "branches": []any{map[string]any{"predicate": conditionNested(17)}}}
	expectProblem(t, ValidateProtocol("Stage", encoded(t, malformed)), "predicate_limit")
	expectProblem(t, ValidateProtocol("RepeatStage", encoded(t, map[string]any{"until": conditionNested(17)})), "predicate_limit")
	for _, invalid := range []Predicate{{Op: "all", Args: []Predicate{}}, {Op: "eval"}} {
		expectProblem(t, ValidateProtocol("Predicate", encoded(t, invalid)), "schema_invalid")
	}
	// Each choice owns its own budget; two separate 256-operator choices are
	// not a single 512-operator expression.
	w, registry := conditionWorkflow(t, conditionSizedChoice(256))
	graph := w["definition"].(map[string]any)["stages"].(map[string]Stage)
	first := graph["choose"]
	for i := range first.Branches {
		first.Branches[i].Next = "second"
	}
	graph["choose"], graph["second"] = first, conditionSizedChoice(256)
	if _, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile); err != nil {
		t.Fatal("unrelated choices shared an AST budget", err)
	}
}

func TestPredicateEvidenceBudget(t *testing.T) {
	for _, total := range []int{MaxPredicateFieldBytes, MaxPredicateFieldBytes + 1} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			choice := Stage{Kind: "choice", Selection: "exclusive"}
			const refs = 200
			base := len(encoded(t, FieldRef{From: "workflow_input", Port: "value", Pointer: "/"}))
			for branch := range 4 {
				predicate := Predicate{Op: "all"}
				for index := range 50 {
					bytes := total / refs
					if branch*50+index < total%refs {
						bytes++
					}
					ref := FieldRef{From: "workflow_input", Port: "value", Pointer: "/" + strings.Repeat("a", bytes-base)}
					predicate.Args = append(predicate.Args, Predicate{Op: "exists", Ref: &ref})
				}
				choice.Branches = append(choice.Branches, ChoiceBranch{ID: fmt.Sprintf("b%d", branch), Predicate: predicate, Next: "done"})
			}
			err := ValidateProtocol("ChoiceStage", encoded(t, choice))
			if total > MaxPredicateFieldBytes {
				expectProblem(t, err, "predicate_limit")
			} else if err != nil {
				t.Fatal("exact evidence byte ceiling was rejected", err)
			}
		})
	}
}

func TestConditionPreflightIsScopedAndExact(t *testing.T) {
	for _, raw := range []string{"1.0000000000000000001", "9007199254740990.9", "1e-1000000"} {
		p := Predicate{Op: "eq", Left: conditionLiteral(raw), Right: conditionLiteral("1")}
		expectProblem(t, ValidateProtocol("Predicate", encoded(t, p)), "condition_type_mismatch")
		w, registry := conditionWorkflow(t, Stage{Kind: "choice", Selection: "exclusive", Branches: []ChoiceBranch{{ID: "one", Predicate: p, Next: "done"}}})
		_, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile)
		expectProblem(t, err, "condition_type_mismatch")
	}
	for _, raw := range []string{"1", "1.0", "1e0"} {
		w, registry := conditionWorkflow(t, Stage{Kind: "choice", Selection: "exclusive", Branches: []ChoiceBranch{{ID: "one", Predicate: Predicate{Op: "eq", Left: conditionLiteral(raw), Right: conditionLiteral("1")}, Next: "done"}}})
		p, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile)
		if err != nil {
			t.Fatal(err)
		}
		result, err := p.SelectChoice("choose", nil)
		if err != nil || result.BranchID != "one" {
			t.Fatalf("equivalent literal %s changed after canonicalization: %+v %v", raw, result, err)
		}
	}
	// These JSON instances resemble control programs but are ordinary data in
	// a literal/default and inside a JSON Schema const, outside AST positions.
	data := encoded(t, map[string]any{"deep": conditionNested(17), "wide": conditionSizedChoice(257)})
	schema := encoded(t, map[string]any{"type": "object", "const": json.RawMessage(data)})
	digest, err := Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref{ID: "test:schema/control-like-data", Version: "1.0.0", Digest: digest}
	for _, profile := range []string{Profile, CoreProfile} {
		w, registry := fixture(t)
		registry[ref] = schema
		w["inputs"] = map[string]InputPort{}
		w["outputs"] = map[string]OutputPort{"data": {Port: Port{Format: "json", SchemaRef: &ref}, RequiredFor: []string{"succeeded"}}}
		binding := Binding{From: "literal", SchemaRef: &ref, Value: data}
		w["definition"] = map[string]any{"entry": "done", "stages": map[string]Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{"data": binding}}}}
		if profile == CoreProfile {
			w["schema_version"] = "2"
			w["inputs"] = map[string]InputPort{"value": {Port: Port{Format: "json", SchemaRef: &ref}, Configuration: &InputConfiguration{Scope: "run", Default: data}}}
		}
		if err := ValidateProtocol("InputBinding", encoded(t, binding)); err != nil {
			t.Fatal("literal data acquired a predicate budget", err)
		}
		if _, err := CompileProfile(encoded(t, w), "json", registry, profile); err != nil {
			t.Fatal("data or schema became a condition program", err)
		}
	}
}

// Negation is available to guards alone: the published choice and repeat
// contracts do not list it. What matters most here is the unknown row - the
// absence of facts must never become a permission by being negated.
func TestGuardNegationLeavesUnknownAlone(t *testing.T) {
	for value, want := range map[string]Truth{"true": TruthFalse, "false": TruthTrue, "unknown": TruthUnknown} {
		t.Run(value, func(t *testing.T) {
			predicate := Predicate{Op: "not", Args: []Predicate{conditionFixturePredicate(value)}}
			if err := ValidateGuardPredicate(predicate); err != nil {
				t.Fatal(err)
			}
			nodes := 0
			truth, err := evaluatePredicate(predicate, func(FieldRef) (any, bool, error) { return nil, false, nil }, "", 1, &nodes)
			if err != nil || truth != want {
				t.Fatalf("want %s, got %s: %v", want, truth, err)
			}
			// The closed contracts a choice and a repeat are validated against
			// still refuse the operator, so this widening reaches guards only.
			expectProblem(t, ValidateProtocol("Predicate", encoded(t, predicate)), "schema_invalid")
		})
	}
	// One argument exactly. A negation of nothing, or of several things at
	// once, has no defined answer and is refused rather than guessed.
	for _, invalid := range []Predicate{
		{Op: "not"},
		{Op: "not", Args: []Predicate{conditionFixturePredicate("true"), conditionFixturePredicate("true")}},
		{Op: "not", Args: []Predicate{conditionFixturePredicate("true")}, Ref: &FieldRef{From: "workflow_input", Port: "value"}},
	} {
		expectProblem(t, ValidateGuardPredicate(invalid), "invalid_predicate")
	}
}
