package model

import (
	"fmt"
	"math"
	"strconv"
)

// ApplyArithmeticOp applies an arithmetic operator to two numeric values.
func ApplyArithmeticOp(a Value, op ComputeOp, b Value) (Value, error) {
	// If both are integers
	if a.Type() == TypeInteger && b.Type() == TypeInteger {
		i1 := a.Raw().(int64)
		i2 := b.Raw().(int64)
		switch op {
		case ComputeOpAdd:
			return NewInt(i1 + i2), nil
		case ComputeOpSub:
			return NewInt(i1 - i2), nil
		case ComputeOpMul:
			return NewInt(i1 * i2), nil
		case ComputeOpDiv:
			if i2 == 0 {
				return NewInt(0), fmt.Errorf("division by zero in compute")
			}
			return NewInt(i1 / i2), nil
		case ComputeOpMod:
			if i2 == 0 {
				return NewInt(0), fmt.Errorf("division by zero in compute modulo")
			}
			return NewInt(i1 % i2), nil
		}
	}

	// Floating point arithmetic if either operand is Float (or numeric string)
	var f1, f2 float64
	if a.Type() == TypeInteger {
		f1 = float64(a.Raw().(int64))
	} else if a.Type() == TypeFloat {
		f1 = a.Raw().(float64)
	} else if aStr, ok := a.Raw().(string); ok {
		if val, err := strconv.ParseFloat(aStr, 64); err == nil {
			f1 = val
		} else {
			return NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", a)
		}
	} else {
		return NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", a)
	}

	if b.Type() == TypeInteger {
		f2 = float64(b.Raw().(int64))
	} else if b.Type() == TypeFloat {
		f2 = b.Raw().(float64)
	} else if bStr, ok := b.Raw().(string); ok {
		if val, err := strconv.ParseFloat(bStr, 64); err == nil {
			f2 = val
		} else {
			return NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", b)
		}
	} else {
		return NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", b)
	}

	switch op {
	case ComputeOpAdd:
		return NewFloat(f1 + f2), nil
	case ComputeOpSub:
		return NewFloat(f1 - f2), nil
	case ComputeOpMul:
		return NewFloat(f1 * f2), nil
	case ComputeOpDiv:
		if f2 == 0 {
			return NewInt(0), fmt.Errorf("division by zero in compute")
		}
		return NewFloat(f1 / f2), nil
	case ComputeOpMod:
		if f2 == 0 {
			return NewInt(0), fmt.Errorf("division by zero in compute modulo")
		}
		return NewFloat(math.Mod(f1, f2)), nil
	default:
		return NewInt(0), fmt.Errorf("unknown compute operator: %v", op)
	}
}

// EvaluateCompute evaluates a (compute ...) expression using the provided variable bindings.
func EvaluateCompute(expr *ComputeExpr, bindings map[string]Value) (Value, error) {
	if expr == nil || len(expr.Operands) == 0 {
		return NewInt(0), nil
	}

	current := ResolveValue(expr.Operands[0], bindings)
	if current.IsCompute() {
		var err error
		current, err = EvaluateCompute(current.ComputeExpr(), bindings)
		if err != nil {
			return NewInt(0), err
		}
	}

	for i, op := range expr.Operators {
		if i+1 >= len(expr.Operands) {
			break
		}
		next := ResolveValue(expr.Operands[i+1], bindings)
		if next.IsCompute() {
			var err error
			next, err = EvaluateCompute(next.ComputeExpr(), bindings)
			if err != nil {
				return NewInt(0), err
			}
		}

		res, err := ApplyArithmeticOp(current, op, next)
		if err != nil {
			return NewInt(0), err
		}
		current = res
	}

	return current, nil
}

// ResolveValue substitutes variable placeholders and evaluates compute expressions.
func ResolveValue(val Value, bindings map[string]Value) Value {
	if val.IsVariable() {
		vName := val.VariableName()
		if bound, ok := bindings[vName]; ok {
			return bound
		}
		return val
	}
	if val.IsVector() {
		elems := val.VectorElements()
		resolved := make([]Value, 0, len(elems))
		for _, el := range elems {
			r := ResolveValue(el, bindings)
			if r.IsVector() {
				resolved = append(resolved, r.VectorElements()...)
			} else {
				resolved = append(resolved, r)
			}
		}
		return NewVector(resolved)
	}
	if val.IsCompute() {
		res, err := EvaluateCompute(val.ComputeExpr(), bindings)
		if err == nil {
			return res
		}
	}
	return val
}
