package model

import (
	"fmt"
	"strings"
)

// Operator represents a comparison operator in a condition test.
type Operator int

const (
	OpEqual Operator = iota
	OpNotEqual
	OpLess
	OpLessEqual
	OpGreater
	OpGreaterEqual
)

func (op Operator) String() string {
	switch op {
	case OpEqual:
		return "="
	case OpNotEqual:
		return "<>"
	case OpLess:
		return "<"
	case OpLessEqual:
		return "<="
	case OpGreater:
		return ">"
	case OpGreaterEqual:
		return ">="
	default:
		return "="
	}
}

// ParseOperator parses a string operator like "=", "<>", "!=", "<", "<=", ">", ">=".
func ParseOperator(s string) Operator {
	switch s {
	case "<>", "!=":
		return OpNotEqual
	case "<":
		return OpLess
	case "<=":
		return OpLessEqual
	case ">":
		return OpGreater
	case ">=":
		return OpGreaterEqual
	case "=":
		fallthrough
	default:
		return OpEqual
	}
}

// TestConstraint represents a single operator and target value test on an attribute.
type TestConstraint struct {
	Op    Operator
	Value Value
}

func (tc TestConstraint) String() string {
	if tc.Op == OpEqual {
		return tc.Value.String()
	}
	return fmt.Sprintf("%s %s", tc.Op.String(), tc.Value.String())
}

// AttributeTest represents all constraints applied to a single attribute in a condition element.
type AttributeTest struct {
	Attribute   string
	Constraints []TestConstraint
}

// ConditionElement represents a single positive or negative condition element on the LHS.
type ConditionElement struct {
	IsNegative      bool
	ElementVariable string // e.g. "g" if bound with <g>
	Class           string
	Tests           []AttributeTest
}

// NewPositiveCE creates a new positive ConditionElement.
func NewPositiveCE(class string) *ConditionElement {
	return &ConditionElement{
		IsNegative: false,
		Class:      class,
		Tests:      make([]AttributeTest, 0),
	}
}

// NewNegativeCE creates a new negative (negated) ConditionElement.
func NewNegativeCE(class string) *ConditionElement {
	return &ConditionElement{
		IsNegative: true,
		Class:      class,
		Tests:      make([]AttributeTest, 0),
	}
}

// WithElementVariable sets an element variable binding (e.g. <g>).
func (ce *ConditionElement) WithElementVariable(varName string) *ConditionElement {
	ce.ElementVariable = strings.TrimPrefix(strings.TrimSuffix(varName, ">"), "<")
	return ce
}

// AddTest adds an attribute test with operator and value.
func (ce *ConditionElement) AddTest(attr string, op Operator, val Value) *ConditionElement {
	normAttr := NormalizeAttribute(attr)
	for i := range ce.Tests {
		if ce.Tests[i].Attribute == normAttr {
			ce.Tests[i].Constraints = append(ce.Tests[i].Constraints, TestConstraint{Op: op, Value: val})
			return ce
		}
	}
	ce.Tests = append(ce.Tests, AttributeTest{
		Attribute:   normAttr,
		Constraints: []TestConstraint{{Op: op, Value: val}},
	})
	return ce
}

// AddEqualTest is a convenience method for equality test on an attribute.
func (ce *ConditionElement) AddEqualTest(attr string, val Value) *ConditionElement {
	return ce.AddTest(attr, OpEqual, val)
}

// SpecificityScore calculates the number of tests contributed by this condition element.
// In OPS5:
// - Class name test = 1
// - Each constraint on an attribute = 1
func (ce *ConditionElement) SpecificityScore() int {
	score := 1 // For class match
	for _, at := range ce.Tests {
		score += len(at.Constraints)
	}
	return score
}

// Variables returns all variable names referenced in this condition element.
func (ce *ConditionElement) Variables() []string {
	varMap := make(map[string]bool)
	if ce.ElementVariable != "" {
		varMap[ce.ElementVariable] = true
	}
	for _, at := range ce.Tests {
		for _, c := range at.Constraints {
			if c.Value.IsVariable() {
				varMap[c.Value.VariableName()] = true
			}
		}
	}

	var res []string
	for v := range varMap {
		res = append(res, v)
	}
	return res
}

// String returns a string representation of the ConditionElement.
func (ce *ConditionElement) String() string {
	var b strings.Builder
	if ce.ElementVariable != "" {
		b.WriteString(fmt.Sprintf("<%s> ", ce.ElementVariable))
	}
	if ce.IsNegative {
		b.WriteString("-(")
	} else {
		b.WriteString("(")
	}
	b.WriteString(ce.Class)
	for _, at := range ce.Tests {
		b.WriteString(fmt.Sprintf(" ^%s", at.Attribute))
		for _, c := range at.Constraints {
			b.WriteString(" ")
			b.WriteString(c.String())
		}
	}
	b.WriteString(")")
	return b.String()
}

// Matches evaluates whether this condition element pattern matches the given WME.
// Class matching is case-insensitive, with "*" matching any class.
// All attribute tests must be satisfied by the WME.
func (ce *ConditionElement) Matches(wme *WME) bool {
	if wme == nil {
		return false
	}
	if ce.Class != "" && ce.Class != "*" {
		if !strings.EqualFold(wme.Class, ce.Class) {
			return false
		}
	}

	for _, at := range ce.Tests {
		val, ok := wme.Get(at.Attribute)
		if !ok {
			val = NewSymbol("nil")
		}

		for _, c := range at.Constraints {
			if !evalConstraint(val, c.Op, c.Value) {
				return false
			}
		}
	}

	return true
}

func evalConstraint(val Value, op Operator, target Value) bool {
	if val.IsVector() {
		elems := val.VectorElements()
		if target.IsVector() {
			return evalOp(val, op, target)
		}
		// Membership test for scalar target against vector attribute
		for _, elem := range elems {
			if evalOp(elem, op, target) {
				return true
			}
		}
		return false
	}

	return evalOp(val, op, target)
}

func evalOp(val Value, op Operator, target Value) bool {
	switch op {
	case OpEqual:
		return val.Equal(target)
	case OpNotEqual:
		return !val.Equal(target)
	case OpLess:
		cmp, err := val.Compare(target)
		return err == nil && cmp < 0
	case OpLessEqual:
		cmp, err := val.Compare(target)
		return err == nil && cmp <= 0
	case OpGreater:
		cmp, err := val.Compare(target)
		return err == nil && cmp > 0
	case OpGreaterEqual:
		cmp, err := val.Compare(target)
		return err == nil && cmp >= 0
	default:
		return false
	}
}
