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

// TestConstraint represents a single operator and target value test on an attribute,
// or a disjunction of multiple constraints << c1 c2 ... >>.
type TestConstraint struct {
	Op          Operator
	Value       Value
	Disjunction []TestConstraint // If non-empty, represents a disjunction << c1 c2 ... >>
}

func (tc TestConstraint) String() string {
	if len(tc.Disjunction) > 0 {
		var parts []string
		for _, d := range tc.Disjunction {
			parts = append(parts, d.String())
		}
		return fmt.Sprintf("<< %s >>", strings.Join(parts, " "))
	}
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

// AccumulateOp represents an aggregation function for an accumulate condition element.
type AccumulateOp int

const (
	AccCount AccumulateOp = iota
	AccSum
	AccAverage
	AccMin
	AccMax
	AccCollect
)

func (op AccumulateOp) String() string {
	switch op {
	case AccCount:
		return ":count"
	case AccSum:
		return ":sum"
	case AccAverage:
		return ":average"
	case AccMin:
		return ":min"
	case AccMax:
		return ":max"
	case AccCollect:
		return ":collect"
	default:
		return ":unknown"
	}
}

// ParseAccumulateOp parses an aggregation function string.
func ParseAccumulateOp(s string) (AccumulateOp, error) {
	norm := strings.ToLower(strings.TrimPrefix(s, ":"))
	switch norm {
	case "count":
		return AccCount, nil
	case "sum":
		return AccSum, nil
	case "avg", "average":
		return AccAverage, nil
	case "min":
		return AccMin, nil
	case "max":
		return AccMax, nil
	case "collect":
		return AccCollect, nil
	default:
		return 0, fmt.Errorf("unknown accumulate operation: %s", s)
	}
}

// AccumulateSpec specifies the aggregation operation, target expression/variable, and result variable.
type AccumulateSpec struct {
	Op        AccumulateOp
	Target    Value  // Target variable or expression, e.g. <p> or (compute ...)
	ResultVar string // Variable name to bind the result to, e.g. "total"
}

// ConditionElement represents a single positive or negative condition element on the LHS,
// or a predicate test condition element (test ...), or an existential/accumulate condition element.
type ConditionElement struct {
	IsNegative      bool
	IsTest          bool
	IsExistential   bool
	IsAccumulate    bool
	IsNCC           bool
	NCCConditions   []*ConditionElement
	Accumulate      *AccumulateSpec
	ElementVariable string // e.g. "g" if bound with <g>
	Class           string
	Tests           []AttributeTest
	EvalTest        *EvalTest
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

// NewNccCE creates a new Negated Conjunctive Condition (NCC) element.
func NewNccCE(subConditions []*ConditionElement) *ConditionElement {
	return &ConditionElement{
		IsNegative:    true,
		IsNCC:         true,
		Class:         "ncc",
		NCCConditions: subConditions,
	}
}

// NewExistentialCE creates a new existential (exists) ConditionElement.
func NewExistentialCE(class string) *ConditionElement {
	return &ConditionElement{
		IsNegative:    false,
		IsTest:        false,
		IsExistential: true,
		Class:         class,
		Tests:         make([]AttributeTest, 0),
	}
}

// NewAccumulateCE creates a new accumulate ConditionElement with the given specification.
func NewAccumulateCE(class string, spec *AccumulateSpec) *ConditionElement {
	return &ConditionElement{
		IsNegative:    false,
		IsTest:        false,
		IsExistential: false,
		IsAccumulate:  true,
		Accumulate:    spec,
		Class:         class,
		Tests:         make([]AttributeTest, 0),
	}
}

// NewTestCE creates a new test ConditionElement with the given EvalTest.
func NewTestCE(evalTest *EvalTest) *ConditionElement {
	return &ConditionElement{
		IsNegative: false,
		IsTest:     true,
		Class:      "test",
		EvalTest:   evalTest,
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

// AddDisjunctionTest adds a disjunctive attribute test << c1 c2 ... >>.
func (ce *ConditionElement) AddDisjunctionTest(attr string, constraints []TestConstraint) *ConditionElement {
	normAttr := NormalizeAttribute(attr)
	for i := range ce.Tests {
		if ce.Tests[i].Attribute == normAttr {
			ce.Tests[i].Constraints = append(ce.Tests[i].Constraints, TestConstraint{Disjunction: constraints})
			return ce
		}
	}
	ce.Tests = append(ce.Tests, AttributeTest{
		Attribute:   normAttr,
		Constraints: []TestConstraint{{Disjunction: constraints}},
	})
	return ce
}

// SpecificityScore calculates the number of tests contributed by this condition element.
// In OPS5:
// - Class name test = 1
// - Each constraint on an attribute = 1
// - For test CEs: each comparison counts as 1
func (ce *ConditionElement) SpecificityScore() int {
	if ce.IsTest {
		if ce.EvalTest != nil && len(ce.EvalTest.Comparisons) > 0 {
			return len(ce.EvalTest.Comparisons)
		}
		return 1
	}
	if ce.IsNCC {
		score := 0
		for _, sub := range ce.NCCConditions {
			score += sub.SpecificityScore()
		}
		return score
	}
	score := 1 // For class match
	for _, at := range ce.Tests {
		score += len(at.Constraints)
	}
	if ce.IsAccumulate {
		score++
	}
	return score
}

// Variables returns all variable names referenced in this condition element.
func (ce *ConditionElement) Variables() []string {
	varMap := make(map[string]bool)
	if ce.IsTest {
		if ce.EvalTest != nil {
			for _, cmp := range ce.EvalTest.Comparisons {
				collectVariables(cmp.Left, varMap)
				if cmp.HasRight {
					collectVariables(cmp.Right, varMap)
				}
			}
		}
	} else if ce.IsNCC {
		for _, sub := range ce.NCCConditions {
			for _, v := range sub.Variables() {
				varMap[v] = true
			}
		}
	} else if ce.IsAccumulate {
		if ce.Accumulate != nil {
			if ce.Accumulate.ResultVar != "" {
				varMap[ce.Accumulate.ResultVar] = true
			}
			collectVariables(ce.Accumulate.Target, varMap)
		}
		for _, at := range ce.Tests {
			for _, c := range at.Constraints {
				if len(c.Disjunction) > 0 {
					for _, dj := range c.Disjunction {
						if dj.Value.IsVariable() {
							varMap[dj.Value.VariableName()] = true
						}
					}
				} else if c.Value.IsVariable() {
					varMap[c.Value.VariableName()] = true
				}
			}
		}
	} else {
		if ce.ElementVariable != "" {
			varMap[ce.ElementVariable] = true
		}
		for _, at := range ce.Tests {
			for _, c := range at.Constraints {
				if len(c.Disjunction) > 0 {
					for _, dj := range c.Disjunction {
						if dj.Value.IsVariable() {
							varMap[dj.Value.VariableName()] = true
						}
					}
				} else if c.Value.IsVariable() {
					varMap[c.Value.VariableName()] = true
				}
			}
		}
	}

	var res []string
	for v := range varMap {
		res = append(res, v)
	}
	return res
}

func collectVariables(v Value, varMap map[string]bool) {
	if v.IsVariable() {
		varMap[v.VariableName()] = true
	} else if v.IsCompute() {
		for _, op := range v.ComputeExpr().Operands {
			collectVariables(op, varMap)
		}
	} else if v.IsVector() {
		for _, el := range v.VectorElements() {
			collectVariables(el, varMap)
		}
	}
}

// String returns a string representation of the ConditionElement.
func (ce *ConditionElement) String() string {
	if ce.IsTest {
		var b strings.Builder
		b.WriteString("(test")
		if ce.EvalTest != nil {
			for _, cmp := range ce.EvalTest.Comparisons {
				b.WriteString(" ")
				b.WriteString(cmp.String())
			}
		}
		b.WriteString(")")
		return b.String()
	}
	if ce.IsExistential {
		var b strings.Builder
		b.WriteString("(exists (")
		b.WriteString(ce.Class)
		for _, at := range ce.Tests {
			b.WriteString(fmt.Sprintf(" ^%s", at.Attribute))
			for _, c := range at.Constraints {
				b.WriteString(" ")
				b.WriteString(c.String())
			}
		}
		b.WriteString("))")
		return b.String()
	}
	if ce.IsNCC {
		var b strings.Builder
		b.WriteString("-(")
		for i, sub := range ce.NCCConditions {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(sub.String())
		}
		b.WriteString(")")
		return b.String()
	}
	if ce.IsAccumulate && ce.Accumulate != nil {
		var b strings.Builder
		b.WriteString("(accumulate (")
		b.WriteString(ce.Class)
		for _, at := range ce.Tests {
			b.WriteString(fmt.Sprintf(" ^%s", at.Attribute))
			for _, c := range at.Constraints {
				b.WriteString(" ")
				b.WriteString(c.String())
			}
		}
		b.WriteString(fmt.Sprintf(") %s", ce.Accumulate.Op.String()))
		if ce.Accumulate.Target.String() != "" && ce.Accumulate.Target.String() != "nil" && ce.Accumulate.Target.String() != `""` {
			b.WriteString(" ")
			b.WriteString(ce.Accumulate.Target.String())
		}
		b.WriteString(fmt.Sprintf(" <%s>)", ce.Accumulate.ResultVar))
		return b.String()
	}
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
	if wme == nil || ce.IsTest {
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
			if len(c.Disjunction) > 0 {
				matchedAny := false
				for _, dj := range c.Disjunction {
					if evalConstraint(val, dj.Op, dj.Value) {
						matchedAny = true
						break
					}
				}
				if !matchedAny {
					return false
				}
			} else if !evalConstraint(val, c.Op, c.Value) {
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
