package flow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	MaxPredicateDepth = 16
	// The budget covers all branches in one choice or one repeat's until.
	// Operand and FieldRef objects are not additional predicate/operator nodes.
	MaxPredicateNodes = 256
	// Bound decision evidence independently from operator count. Repeated refs
	// count again; runtime deduplication can only make the trace smaller.
	MaxPredicateFieldBytes = 256 << 10
)

type Truth string

const (
	TruthTrue    Truth = "true"
	TruthFalse   Truth = "false"
	TruthUnknown Truth = "unknown"
)

type BranchEvaluation struct {
	BranchID string `json:"branch_id"`
	Result   Truth  `json:"result"`
}

// ChoiceResult is a routing calculation, not a persisted decision or permission
// to execute. On error Evaluations retains the completed prefix; ErrorBranchID
// names a failing predicate, not a global ambiguity/unknown/no-transition error.
type ChoiceResult struct {
	Evaluations   []BranchEvaluation
	Route         string
	BranchID      string
	NextStageID   string
	ErrorBranchID string
}

type predicateBudget struct{ nodes, fieldBytes int }

// ValidateWorkflowConditions checks only the bounded condition AST positions
// in authoring bytes before a registry canonicalizes them. It does not grant
// schema/profile/graph validity or execution permission. In particular, JCS
// must not round a fractional control literal into an accepted integer before
// this check. Ordinary schema, literal binding and configuration data is not AST.
func ValidateWorkflowConditions(data []byte, format string) error {
	value, _, err := workflowValue(data, format)
	if err != nil {
		return err
	}
	return preflightConditions("WorkflowRevision", value, "")
}

func (b *predicateBudget) field(value any, path string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return problem("invalid_predicate", path, "field reference cannot be encoded")
	}
	b.fieldBytes += len(data)
	if b.fieldBytes > MaxPredicateFieldBytes {
		return problem("predicate_limit", path, "predicate field references exceed 256 KiB of serialized evidence")
	}
	return nil
}

// preflightConditions runs before recursive union-schema validation. Walk only
// known AST positions in the selected contract, never literals, input defaults
// or JSON Schema resources that happen to contain op/args/branches properties.
func preflightConditions(name string, value any, path string) error {
	object, _ := value.(map[string]any)
	switch name {
	case "WorkflowRevision", "WorkflowRevisionV2", "WorkflowRevisionV3":
		definition, _ := object["definition"].(map[string]any)
		stages, _ := definition["stages"].(map[string]any)
		for _, id := range keys(stages) {
			if err := preflightConditions("Stage", stages[id], path+"/definition/stages/"+escapePointer(id)); err != nil {
				return err
			}
		}
	case "Stage", "ChoiceStage", "RepeatStage":
		// Stage is a union. Inspect both AST-bearing variants even when kind
		// is malformed: schema validation may inspect a rejected variant too.
		if name != "RepeatStage" {
			branches, _ := object["branches"].([]any)
			budget := predicateBudget{}
			for i, branch := range branches {
				branch, _ := branch.(map[string]any)
				if predicate, exists := branch["predicate"]; exists {
					if err := preflightPredicate(predicate, fmt.Sprintf("%s/branches/%d/predicate", path, i), 1, &budget); err != nil {
						return err
					}
				}
			}
		}
		if name != "ChoiceStage" {
			if predicate, exists := object["until"]; exists {
				return preflightPredicate(predicate, path+"/until", 1, &predicateBudget{})
			}
		}
	case "Predicate":
		return preflightPredicate(value, path, 1, &predicateBudget{})
	case "Operand":
		return preflightOperand(value, path, &predicateBudget{})
	case "ControlScalar":
		if _, numeric := value.(json.Number); numeric {
			_, err := controlScalar(value, path)
			return err
		}
	}
	return nil
}

func preflightPredicate(value any, path string, depth int, budget *predicateBudget) error {
	budget.nodes++
	if depth > MaxPredicateDepth || budget.nodes > MaxPredicateNodes {
		return problem("predicate_limit", path, "predicates exceed depth 16 or 256 operators")
	}
	object, _ := value.(map[string]any)
	if ref, exists := object["ref"]; exists {
		if err := budget.field(ref, path+"/ref"); err != nil {
			return err
		}
	}
	for _, side := range []string{"left", "right"} {
		if operand, exists := object[side]; exists {
			if err := preflightOperand(operand, path+"/"+side, budget); err != nil {
				return err
			}
		}
	}
	args, _ := object["args"].([]any)
	for i, child := range args {
		if err := preflightPredicate(child, fmt.Sprintf("%s/args/%d", path, i), depth+1, budget); err != nil {
			return err
		}
	}
	return nil
}

func preflightOperand(value any, path string, budget *predicateBudget) error {
	object, _ := value.(map[string]any)
	if object["kind"] == "literal" {
		if literal, exists := object["value"]; exists {
			_, err := controlScalar(literal, path+"/value")
			return err
		}
	}
	if object["kind"] == "field" {
		if ref, exists := object["ref"]; exists {
			return budget.field(ref, path+"/ref")
		}
	}
	return nil
}

// SelectChoice evaluates only pinned declarations and values returned by the
// caller. The resolver applies FieldRef.Pointer and returns the parsed JSON
// model (nil, bool, string, json.Number, object or array) plus presence. Missing
// and present null are distinct. The runtime owns artifact/schema validation,
// authorization, durable recording and the subsequent state transition.
func (p *Plan) SelectChoice(stageID string, resolve func(FieldRef) (any, bool, error)) (ChoiceResult, error) {
	result := ChoiceResult{Evaluations: []BranchEvaluation{}}
	path := "/definition/stages/" + escapePointer(stageID)
	if p.Profile != CoreProfile {
		return result, problem("unsupported", path, "choice requires "+CoreProfile)
	}
	stage, exists := p.Workflow.Definition.Stages[stageID]
	if !exists || stage.Kind != "choice" {
		return result, problem("invalid_stage", path, "expected a compiled choice stage")
	}
	if stage.Selection != "exclusive" && stage.Selection != "first_match" || len(stage.Branches) == 0 || len(stage.Branches) > 64 {
		return result, problem("invalid_choice", path, "choice needs a declared selection and 1 to 64 branches")
	}
	nodes, matches := 0, 0
	unknown := false
	var selected ChoiceBranch
	for i, branch := range stage.Branches {
		truth, err := evaluatePredicate(branch.Predicate, resolve, fmt.Sprintf("%s/branches/%d/predicate", path, i), 1, &nodes)
		if err != nil {
			result.ErrorBranchID = branch.ID
			return result, err
		}
		result.Evaluations = append(result.Evaluations, BranchEvaluation{BranchID: branch.ID, Result: truth})
		if truth == TruthTrue {
			matches++
			selected = branch
		}
		unknown = unknown || truth == TruthUnknown
		if stage.Selection == "first_match" && truth != TruthFalse {
			break
		}
	}
	if matches > 1 {
		return result, problem("ambiguous_branch", path+"/branches", "exclusive choice matched more than one branch")
	}
	if unknown {
		if stage.OnUnknown == "" {
			return result, problem("condition_unknown", path+"/on_unknown", "unknown condition has no declared route")
		}
		result.Route, result.NextStageID = "on_unknown", stage.OnUnknown
		return result, nil
	}
	if matches == 1 {
		result.Route, result.BranchID, result.NextStageID = "branch", selected.ID, selected.Next
		return result, nil
	}
	if stage.Default == "" {
		return result, problem("no_transition", path+"/default", "all predicates are false and no default is declared")
	}
	result.Route, result.NextStageID = "default", stage.Default
	return result, nil
}

func evaluatePredicate(predicate Predicate, resolve func(FieldRef) (any, bool, error), path string, depth int, nodes *int) (Truth, error) {
	*nodes++
	if depth > MaxPredicateDepth || *nodes > MaxPredicateNodes {
		return "", problem("predicate_limit", path, "predicates exceed depth 16 or 256 operators")
	}
	invalid := func() (Truth, error) {
		return "", problem("invalid_predicate", path, "predicate does not match a closed operator")
	}
	switch predicate.Op {
	case "eq", "ne":
		if predicate.Left == nil || predicate.Right == nil || predicate.Ref != nil || len(predicate.Args) != 0 {
			return invalid()
		}
		left, leftPresent, err := evaluateOperand(*predicate.Left, resolve, path+"/left")
		if err != nil {
			return "", err
		}
		right, rightPresent, err := evaluateOperand(*predicate.Right, resolve, path+"/right")
		if err != nil {
			return "", err
		}
		if !leftPresent || !rightPresent {
			return TruthUnknown, nil
		}
		if left.kind != right.kind {
			return "", conditionTypeMismatch(path)
		}
		equal := left == right
		return truthOf(equal == (predicate.Op == "eq")), nil
	case "exists":
		if predicate.Ref == nil || predicate.Left != nil || predicate.Right != nil || len(predicate.Args) != 0 {
			return invalid()
		}
		if resolve == nil {
			return "", problem("condition_input_invalid", path+"/ref", "field resolver is required")
		}
		_, present, err := resolve(*predicate.Ref)
		if err != nil {
			return "", err
		}
		return truthOf(present), nil
	case "not":
		// Negation flips true and false and leaves unknown alone. It must not
		// be allowed to turn the absence of facts into a permission: "we do not
		// know" negated is still "we do not know", never "yes". The published
		// choice and repeat contracts do not list this operator, so only a
		// caller whose own contract admits it can reach this branch.
		if len(predicate.Args) != 1 || predicate.Left != nil || predicate.Right != nil || predicate.Ref != nil {
			return invalid()
		}
		value, err := evaluatePredicate(predicate.Args[0], resolve, path+"/args/0", depth+1, nodes)
		if err != nil || value == TruthUnknown {
			return value, err
		}
		return truthOf(value == TruthFalse), nil
	case "all", "any":
		if len(predicate.Args) == 0 || len(predicate.Args) > 64 || predicate.Left != nil || predicate.Right != nil || predicate.Ref != nil {
			return invalid()
		}
		result := truthOf(predicate.Op == "all")
		unknown := false
		for i, child := range predicate.Args {
			value, err := evaluatePredicate(child, resolve, fmt.Sprintf("%s/args/%d", path, i), depth+1, nodes)
			if err != nil {
				return "", err
			}
			unknown = unknown || value == TruthUnknown
			if predicate.Op == "all" && value == TruthFalse || predicate.Op == "any" && value == TruthTrue {
				result = value
			}
		}
		if unknown && (predicate.Op == "all" && result == TruthTrue || predicate.Op == "any" && result == TruthFalse) {
			return TruthUnknown, nil
		}
		return result, nil
	default:
		return invalid()
	}
}

func truthOf(value bool) Truth {
	if value {
		return TruthTrue
	}
	return TruthFalse
}

type conditionScalar struct {
	kind    string
	text    string
	boolean bool
	integer int64
}

func evaluateOperand(operand Operand, resolve func(FieldRef) (any, bool, error), path string) (conditionScalar, bool, error) {
	var value any
	present := true
	var err error
	switch operand.Kind {
	case "literal":
		if operand.Ref != nil {
			return conditionScalar{}, false, problem("invalid_predicate", path, "literal operand cannot contain a field reference")
		}
		value, err = Parse(operand.Value, "json")
		if err != nil {
			return conditionScalar{}, false, conditionTypeMismatch(path + "/value")
		}
	case "field":
		if operand.Ref == nil || len(operand.Value) != 0 {
			return conditionScalar{}, false, problem("invalid_predicate", path, "field operand requires only its reference")
		}
		if resolve == nil {
			return conditionScalar{}, false, problem("condition_input_invalid", path+"/ref", "field resolver is required")
		}
		value, present, err = resolve(*operand.Ref)
		if err != nil {
			return conditionScalar{}, false, err
		}
	default:
		return conditionScalar{}, false, problem("invalid_predicate", path, "operand kind is not supported")
	}
	if !present {
		return conditionScalar{}, false, nil
	}
	scalar, err := controlScalar(value, path)
	return scalar, true, err
}

func conditionTypeMismatch(path string) error {
	return problem("condition_type_mismatch", path, "comparison requires same-typed string, boolean, null or safe integer values")
}

func controlScalar(value any, path string) (conditionScalar, error) {
	switch value := value.(type) {
	case nil:
		return conditionScalar{kind: "null"}, nil
	case string:
		return conditionScalar{kind: "string", text: value}, nil
	case bool:
		return conditionScalar{kind: "boolean", boolean: value}, nil
	case json.Number:
		if integer, ok := controlInteger(string(value)); ok {
			return conditionScalar{kind: "integer", integer: integer}, nil
		}
	}
	return conditionScalar{}, conditionTypeMismatch(path)
}

// controlInteger proves exact decimal integrality before floating-point/JCS
// rounding. Coefficient/exponent work is bounded by the document byte limit;
// only a final <=16 digit integer is allocated, never 10^an_external_exponent.
func controlInteger(raw string) (int64, bool) {
	if len(raw) > MaxDocumentBytes || !jsonNumber.MatchString(raw) {
		return 0, false
	}
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	mantissa, exponentText := raw, ""
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		mantissa, exponentText = raw[:index], raw[index+1:]
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return 0, true
	}
	significant := strings.TrimRight(digits, "0")
	var exponent int64
	if exponentText != "" {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil || exponent < -MaxDocumentBytes || exponent > MaxDocumentBytes+16 {
			return 0, false
		}
	}
	shift := exponent - int64(fractionDigits) + int64(len(digits)-len(significant))
	if shift < 0 || int64(len(significant))+shift > 16 {
		return 0, false
	}
	integer, err := strconv.ParseInt(significant+strings.Repeat("0", int(shift)), 10, 64)
	if err != nil || integer > maxSafeInteger {
		return 0, false
	}
	if negative {
		integer = -integer
	}
	return integer, true
}

// EvaluateGuardPredicate evaluates one validated guard predicate against the
// facts the caller resolves. It is the same evaluator a choice uses, with the
// same three-valued result: a second evaluator would be a second answer waiting
// to disagree with this one.
func EvaluateGuardPredicate(predicate Predicate, resolve func(FieldRef) (any, bool, error)) (Truth, error) {
	nodes := 0
	return evaluatePredicate(predicate, resolve, "", 1, &nodes)
}

// ValidateGuardPredicate is the compile-time check for a live guard's closed
// AST. Guards are the one caller whose contract admits `not`, so the operator
// set is checked here rather than by the choice and repeat schemas, which do
// not list it. Nothing about permissions, sources or freshness is decided here.
//
// A comparison between two literals of different types is refused now rather
// than at run time: the mistake is fully visible in the declaration, and a
// registration that has to fail later can only fail while something waits on it.
func ValidateGuardPredicate(predicate Predicate) error {
	encoded, err := json.Marshal(predicate)
	if err != nil {
		return problem("invalid_predicate", "", "predicate cannot be encoded")
	}
	value, err := Parse(encoded, "json")
	if err != nil {
		return err
	}
	if err := preflightPredicate(value, "", 1, &predicateBudget{}); err != nil {
		return err
	}
	return checkGuardPredicate(predicate, "")
}

func checkGuardPredicate(predicate Predicate, path string) error {
	invalid := func() error {
		return problem("invalid_predicate", path, "predicate does not match a closed guard operator")
	}
	switch predicate.Op {
	case "eq", "ne":
		if predicate.Left == nil || predicate.Right == nil || predicate.Ref != nil || len(predicate.Args) != 0 {
			return invalid()
		}
		left, leftLiteral, err := literalOperand(*predicate.Left, path+"/left")
		if err != nil {
			return err
		}
		right, rightLiteral, err := literalOperand(*predicate.Right, path+"/right")
		if err != nil {
			return err
		}
		if leftLiteral && rightLiteral && left.kind != right.kind {
			return conditionTypeMismatch(path)
		}
		return nil
	case "exists":
		if predicate.Ref == nil || predicate.Left != nil || predicate.Right != nil || len(predicate.Args) != 0 {
			return invalid()
		}
		return nil
	case "not":
		if len(predicate.Args) != 1 || predicate.Left != nil || predicate.Right != nil || predicate.Ref != nil {
			return invalid()
		}
		return checkGuardPredicate(predicate.Args[0], path+"/args/0")
	case "all", "any":
		if len(predicate.Args) == 0 || len(predicate.Args) > 64 || predicate.Left != nil || predicate.Right != nil || predicate.Ref != nil {
			return invalid()
		}
		for i, child := range predicate.Args {
			if err := checkGuardPredicate(child, fmt.Sprintf("%s/args/%d", path, i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return invalid()
	}
}

// literalOperand reports the pinned scalar of a literal operand. A field
// operand has no compile-time type here: what a reference will hold is a fact
// about a future artifact, and pretending to know it would be a guess.
func literalOperand(operand Operand, path string) (conditionScalar, bool, error) {
	switch operand.Kind {
	case "literal":
		if operand.Ref != nil {
			return conditionScalar{}, false, problem("invalid_predicate", path, "literal operand cannot contain a field reference")
		}
		value, err := Parse(operand.Value, "json")
		if err != nil {
			return conditionScalar{}, false, conditionTypeMismatch(path + "/value")
		}
		scalar, err := controlScalar(value, path+"/value")
		return scalar, err == nil, err
	case "field":
		if operand.Ref == nil || len(operand.Value) != 0 {
			return conditionScalar{}, false, problem("invalid_predicate", path, "field operand requires only its reference")
		}
		return conditionScalar{}, false, nil
	default:
		return conditionScalar{}, false, problem("invalid_predicate", path, "operand kind is not supported")
	}
}

// WalkPredicateFields visits every field reference a predicate reads, so a
// caller can check them against its own pinned declarations before the
// predicate is ever evaluated.
func WalkPredicateFields(predicate Predicate, visit func(FieldRef) error) error {
	return predicateFields(predicate, "", func(ref FieldRef, _ string) error { return visit(ref) })
}

func predicateFields(predicate Predicate, path string, visit func(FieldRef, string) error) error {
	if predicate.Ref != nil {
		if err := visit(*predicate.Ref, path+"/ref"); err != nil {
			return err
		}
	}
	for _, operand := range []struct {
		name  string
		value *Operand
	}{{"left", predicate.Left}, {"right", predicate.Right}} {
		if operand.value != nil && operand.value.Ref != nil {
			if err := visit(*operand.value.Ref, path+"/"+operand.name+"/ref"); err != nil {
				return err
			}
		}
	}
	for i, child := range predicate.Args {
		if err := predicateFields(child, fmt.Sprintf("%s/args/%d", path, i), visit); err != nil {
			return err
		}
	}
	return nil
}
