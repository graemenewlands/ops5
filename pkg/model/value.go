package model

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueType represents the type of an OPS5 value.
type ValueType int

const (
	TypeSymbol ValueType = iota
	TypeInteger
	TypeFloat
	TypeString
	TypeBoolean
	TypeVariable
	TypeVector
	TypeCompute
	TypeAccept
)

func (t ValueType) String() string {
	switch t {
	case TypeSymbol:
		return "symbol"
	case TypeInteger:
		return "integer"
	case TypeFloat:
		return "float"
	case TypeString:
		return "string"
	case TypeBoolean:
		return "boolean"
	case TypeVariable:
		return "variable"
	case TypeVector:
		return "vector"
	case TypeCompute:
		return "compute"
	case TypeAccept:
		return "accept"
	default:
		return "unknown"
	}
}

// Value is an immutable representation of an OPS5 datum.
type Value struct {
	typ ValueType
	val any
}

// NewSymbol creates a new symbol value.
func NewSymbol(s string) Value {
	return Value{typ: TypeSymbol, val: s}
}

// NewInt creates a new integer value.
func NewInt(n int64) Value {
	return Value{typ: TypeInteger, val: n}
}

// NewFloat creates a new floating-point value.
func NewFloat(f float64) Value {
	return Value{typ: TypeFloat, val: f}
}

// NewString creates a new string value.
func NewString(s string) Value {
	return Value{typ: TypeString, val: s}
}

// NewBoolean creates a new boolean value.
func NewBoolean(b bool) Value {
	return Value{typ: TypeBoolean, val: b}
}

// NewVariable creates a new variable placeholder (e.g. <x>).
func NewVariable(name string) Value {
	// Strip enclosing angle brackets if present
	trimmed := strings.TrimPrefix(strings.TrimSuffix(name, ">"), "<")
	return Value{typ: TypeVariable, val: trimmed}
}

// NewVector creates a new vector value representing a sequence of Values.
func NewVector(elements []Value) Value {
	copied := make([]Value, len(elements))
	copy(copied, elements)
	return Value{typ: TypeVector, val: copied}
}

// ComputeOp represents an arithmetic operator in a compute expression.
type ComputeOp int

const (
	ComputeOpAdd ComputeOp = iota
	ComputeOpSub
	ComputeOpMul
	ComputeOpDiv
	ComputeOpMod
)

func (op ComputeOp) String() string {
	switch op {
	case ComputeOpAdd:
		return "+"
	case ComputeOpSub:
		return "-"
	case ComputeOpMul:
		return "*"
	case ComputeOpDiv:
		return "/"
	case ComputeOpMod:
		return "//"
	default:
		return "+"
	}
}

// ComputeExpr represents a (compute ...) arithmetic expression.
type ComputeExpr struct {
	Operands  []Value
	Operators []ComputeOp
}

// NewCompute creates a new compute expression value.
func NewCompute(operands []Value, operators []ComputeOp) Value {
	return Value{
		typ: TypeCompute,
		val: &ComputeExpr{
			Operands:  operands,
			Operators: operators,
		},
	}
}

// AcceptExpr represents an (accept) or (acceptline) function invocation.
type AcceptExpr struct {
	LogicalFile string // optional logical file, or empty for default input
	IsLine      bool   // true for acceptline, false for accept
}

// NewAccept creates an accept or acceptline expression value.
func NewAccept(logicalFile string, isLine bool) Value {
	return Value{
		typ: TypeAccept,
		val: &AcceptExpr{
			LogicalFile: logicalFile,
			IsLine:      isLine,
		},
	}
}

// IsAccept returns true if this value is an accept or acceptline function call.
func (v Value) IsAccept() bool {
	return v.typ == TypeAccept
}

// AcceptExpr returns the underlying AcceptExpr.
func (v Value) AcceptExpr() *AcceptExpr {
	if v.typ == TypeAccept {
		return v.val.(*AcceptExpr)
	}
	return nil
}

// Type returns the ValueType.
func (v Value) Type() ValueType {
	return v.typ
}

// Raw returns the underlying primitive value.
func (v Value) Raw() any {
	return v.val
}

// IsVariable returns true if this value is a variable reference.
func (v Value) IsVariable() bool {
	return v.typ == TypeVariable
}

// IsBoolean returns true if this value is a boolean.
func (v Value) IsBoolean() bool {
	return v.typ == TypeBoolean
}

// Boolean returns the underlying boolean if this value is a boolean, otherwise false.
func (v Value) Boolean() bool {
	if v.typ == TypeBoolean {
		return v.val.(bool)
	}
	return false
}

// IsVector returns true if this value is a vector of values.
func (v Value) IsVector() bool {
	return v.typ == TypeVector
}

// VectorElements returns the underlying elements if this value is a vector.
func (v Value) VectorElements() []Value {
	if v.typ == TypeVector {
		return v.val.([]Value)
	}
	return nil
}

// IsCompute returns true if this value is a compute expression.
func (v Value) IsCompute() bool {
	return v.typ == TypeCompute
}

// ComputeExpr returns the underlying ComputeExpr pointer if this value is a compute expression.
func (v Value) ComputeExpr() *ComputeExpr {
	if v.typ == TypeCompute {
		return v.val.(*ComputeExpr)
	}
	return nil
}

// VariableName returns the variable identifier without enclosing brackets.
func (v Value) VariableName() string {
	if v.typ == TypeVariable {
		return v.val.(string)
	}
	return ""
}

// String returns the string representation.
func (v Value) String() string {
	switch v.typ {
	case TypeSymbol:
		return v.val.(string)
	case TypeInteger:
		return strconv.FormatInt(v.val.(int64), 10)
	case TypeFloat:
		return strconv.FormatFloat(v.val.(float64), 'g', -1, 64)
	case TypeString:
		return strconv.Quote(v.val.(string))
	case TypeBoolean:
		if v.val.(bool) {
			return "true"
		}
		return "false"
	case TypeVariable:
		return "<" + v.val.(string) + ">"
	case TypeVector:
		elems := v.val.([]Value)
		var parts []string
		for _, el := range elems {
			parts = append(parts, el.String())
		}
		return strings.Join(parts, " ")
	case TypeCompute:
		ce := v.val.(*ComputeExpr)
		var parts []string
		parts = append(parts, "compute")
		for i, op := range ce.Operands {
			parts = append(parts, op.String())
			if i < len(ce.Operators) {
				parts = append(parts, ce.Operators[i].String())
			}
		}
		return "(" + strings.Join(parts, " ") + ")"
	case TypeAccept:
		ae := v.val.(*AcceptExpr)
		name := "accept"
		if ae.IsLine {
			name = "acceptline"
		}
		if ae.LogicalFile != "" {
			return "(" + name + " " + ae.LogicalFile + ")"
		}
		return "(" + name + ")"
	default:
		return fmt.Sprintf("%v", v.val)
	}
}

// Equal checks equality between two values.
// Supports numeric cross-equality between integer and float if values match.
func (v Value) Equal(o Value) bool {
	if v.typ == o.typ {
		if v.typ == TypeVector {
			v1 := v.val.([]Value)
			v2 := o.val.([]Value)
			if len(v1) != len(v2) {
				return false
			}
			for i := range v1 {
				if !v1[i].Equal(v2[i]) {
					return false
				}
			}
			return true
		}
		if v.typ == TypeCompute {
			c1 := v.val.(*ComputeExpr)
			c2 := o.val.(*ComputeExpr)
			if len(c1.Operands) != len(c2.Operands) || len(c1.Operators) != len(c2.Operators) {
				return false
			}
			for i := range c1.Operators {
				if c1.Operators[i] != c2.Operators[i] {
					return false
				}
			}
			for i := range c1.Operands {
				if !c1.Operands[i].Equal(c2.Operands[i]) {
					return false
				}
			}
			return true
		}
		if v.typ == TypeAccept {
			a1 := v.val.(*AcceptExpr)
			a2 := o.val.(*AcceptExpr)
			return a1.LogicalFile == a2.LogicalFile && a1.IsLine == a2.IsLine
		}
		return v.val == o.val
	}
	// Numeric cross-comparison
	if v.typ == TypeInteger && o.typ == TypeFloat {
		return float64(v.val.(int64)) == o.val.(float64)
	}
	if v.typ == TypeFloat && o.typ == TypeInteger {
		return v.val.(float64) == float64(o.val.(int64))
	}
	// Boolean and Symbol cross-comparison (e.g. true vs "true", false vs "false")
	if v.typ == TypeBoolean && o.typ == TypeSymbol {
		return strings.EqualFold(strconv.FormatBool(v.val.(bool)), o.val.(string))
	}
	if v.typ == TypeSymbol && o.typ == TypeBoolean {
		return strings.EqualFold(v.val.(string), strconv.FormatBool(o.val.(bool)))
	}
	return false
}

// Compare returns:
// -1 if v < o
//  0 if v == o
//  1 if v > o
// Returns an error if types cannot be ordered (e.g., symbols vs numbers).
func (v Value) Compare(o Value) (int, error) {
	// Numeric comparisons
	if (v.typ == TypeInteger || v.typ == TypeFloat) && (o.typ == TypeInteger || o.typ == TypeFloat) {
		var f1, f2 float64
		if v.typ == TypeInteger {
			f1 = float64(v.val.(int64))
		} else {
			f1 = v.val.(float64)
		}

		if o.typ == TypeInteger {
			f2 = float64(o.val.(int64))
		} else {
			f2 = o.val.(float64)
		}

		if f1 < f2 {
			return -1, nil
		} else if f1 > f2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeString && o.typ == TypeString {
		s1 := v.val.(string)
		s2 := o.val.(string)
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeBoolean && o.typ == TypeBoolean {
		b1 := v.val.(bool)
		b2 := o.val.(bool)
		if !b1 && b2 {
			return -1, nil
		} else if b1 && !b2 {
			return 1, nil
		}
		return 0, nil
	}

	if v.typ == TypeSymbol && o.typ == TypeSymbol {
		s1 := v.val.(string)
		s2 := o.val.(string)
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	if (v.typ == TypeBoolean && o.typ == TypeSymbol) || (v.typ == TypeSymbol && o.typ == TypeBoolean) {
		s1 := v.String()
		s2 := o.String()
		if s1 < s2 {
			return -1, nil
		} else if s1 > s2 {
			return 1, nil
		}
		return 0, nil
	}

	return 0, fmt.Errorf("cannot compare incompatible types: %s and %s", v.typ, o.typ)
}

// AutoValue creates an appropriate Value from a string token.
// - If wrapped in quotes, creates a String.
// - If starts with '<' and ends with '>', creates a Variable.
// - If "true" or "false" (case-insensitive), creates a Boolean.
// - If parses as integer, creates an Int.
// - If parses as float, creates a Float.
// - Otherwise, creates a Symbol.
func AutoValue(token string) Value {
	if strings.HasPrefix(token, "\"") && strings.HasSuffix(token, "\"") && len(token) >= 2 {
		unquoted, err := strconv.Unquote(token)
		if err == nil {
			return NewString(unquoted)
		}
		return NewString(token[1 : len(token)-1])
	}
	if strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">") && len(token) > 2 {
		return NewVariable(token)
	}
	lower := strings.ToLower(token)
	if lower == "true" {
		return NewBoolean(true)
	}
	if lower == "false" {
		return NewBoolean(false)
	}
	if n, err := strconv.ParseInt(token, 10, 64); err == nil {
		return NewInt(n)
	}
	if f, err := strconv.ParseFloat(token, 64); err == nil && strings.ContainsAny(token, ".eE") {
		return NewFloat(f)
	}
	return NewSymbol(token)
}
