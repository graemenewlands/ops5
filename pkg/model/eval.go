package model

import (
	"fmt"
	"strings"
)

// EvalComparison represents a single comparison in a (test ...) condition element.
type EvalComparison struct {
	Left     Value
	Op       Operator
	Right    Value
	HasRight bool
}

// String returns the string representation of the comparison.
func (cmp EvalComparison) String() string {
	if !cmp.HasRight {
		return fmt.Sprintf("(%s)", cmp.Left.String())
	}
	return fmt.Sprintf("(%s %s %s)", cmp.Left.String(), cmp.Op.String(), cmp.Right.String())
}

// Evaluate evaluates the comparison against token variable bindings.
func (cmp EvalComparison) Evaluate(bindings map[string]Value) (bool, error) {
	leftVal := ResolveValue(cmp.Left, bindings)
	if leftVal.IsVariable() {
		return false, fmt.Errorf("unbound variable in test: %s", leftVal.VariableName())
	}
	if !cmp.HasRight {
		return isTruthy(leftVal), nil
	}
	rightVal := ResolveValue(cmp.Right, bindings)
	if rightVal.IsVariable() {
		return false, fmt.Errorf("unbound variable in test: %s", rightVal.VariableName())
	}
	return evalOp(leftVal, cmp.Op, rightVal), nil
}

func isTruthy(v Value) bool {
	switch v.Type() {
	case TypeBoolean:
		return v.Raw().(bool)
	case TypeInteger:
		return v.Raw().(int64) != 0
	case TypeFloat:
		return v.Raw().(float64) != 0.0
	case TypeSymbol:
		s := strings.ToLower(v.Raw().(string))
		return s != "nil" && s != "false"
	case TypeString:
		return v.Raw().(string) != ""
	case TypeVector:
		return len(v.VectorElements()) > 0
	default:
		return true
	}
}

// EvalTest represents all comparisons inside a (test ...) condition element.
type EvalTest struct {
	Comparisons []EvalComparison
}

// NewEvalTest creates a new EvalTest with the given comparisons.
func NewEvalTest(comparisons ...EvalComparison) *EvalTest {
	return &EvalTest{
		Comparisons: comparisons,
	}
}

// Evaluate evaluates all comparisons in the test (conjunction AND).
func (et *EvalTest) Evaluate(bindings map[string]Value) (bool, error) {
	if et == nil {
		return true, nil
	}
	for _, cmp := range et.Comparisons {
		ok, err := cmp.Evaluate(bindings)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// String returns the string representation of all comparisons in the test.
func (et *EvalTest) String() string {
	if et == nil || len(et.Comparisons) == 0 {
		return "(test)"
	}
	var parts []string
	for _, cmp := range et.Comparisons {
		parts = append(parts, cmp.String())
	}
	return fmt.Sprintf("(test %s)", strings.Join(parts, " "))
}
