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
	TypeVariable
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
	case TypeVariable:
		return "variable"
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

// NewVariable creates a new variable placeholder (e.g. <x>).
func NewVariable(name string) Value {
	// Strip enclosing angle brackets if present
	trimmed := strings.TrimPrefix(strings.TrimSuffix(name, ">"), "<")
	return Value{typ: TypeVariable, val: trimmed}
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
	case TypeVariable:
		return "<" + v.val.(string) + ">"
	default:
		return fmt.Sprintf("%v", v.val)
	}
}

// Equal checks equality between two values.
// Supports numeric cross-equality between integer and float if values match.
func (v Value) Equal(o Value) bool {
	if v.typ == o.typ {
		return v.val == o.val
	}
	// Numeric cross-comparison
	if v.typ == TypeInteger && o.typ == TypeFloat {
		return float64(v.val.(int64)) == o.val.(float64)
	}
	if v.typ == TypeFloat && o.typ == TypeInteger {
		return v.val.(float64) == float64(o.val.(int64))
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

	return 0, fmt.Errorf("cannot compare incompatible types: %s and %s", v.typ, o.typ)
}

// AutoValue creates an appropriate Value from a string token.
// - If wrapped in quotes, creates a String.
// - If starts with '<' and ends with '>', creates a Variable.
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
	if n, err := strconv.ParseInt(token, 10, 64); err == nil {
		return NewInt(n)
	}
	if f, err := strconv.ParseFloat(token, 64); err == nil && strings.ContainsAny(token, ".eE") {
		return NewFloat(f)
	}
	return NewSymbol(token)
}
