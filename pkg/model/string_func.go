package model

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	regexCache sync.Map
)

// NormalizeStringOp normalizes a string operation name to canonical lowercase without hyphens/underscores.
func NormalizeStringOp(op string) string {
	op = strings.ToLower(op)
	op = strings.ReplaceAll(op, "-", "")
	op = strings.ReplaceAll(op, "_", "")
	switch op {
	case "left":
		return "left"
	case "right":
		return "right"
	case "contains":
		return "contains"
	case "containsignorecase":
		return "containsignorecase"
	case "equalsignorecase", "equalignorecase":
		return "equalsignorecase"
	case "regexreplace":
		return "regexreplace"
	}
	return op
}

// IsStringOp returns true if op is a recognized text manipulation function.
func IsStringOp(op string) bool {
	norm := NormalizeStringOp(op)
	switch norm {
	case "left", "right", "contains", "containsignorecase", "equalsignorecase", "regexreplace":
		return true
	default:
		return false
	}
}

// GetCompiledRegex retrieves a compiled regular expression from cache or compiles and caches it.
func GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

// EvaluateStringExpr evaluates a StringExpr against variable bindings.
func EvaluateStringExpr(se *StringExpr, bindings map[string]Value) (Value, error) {
	if se == nil {
		return NewSymbol("nil"), fmt.Errorf("nil StringExpr")
	}

	normOp := NormalizeStringOp(se.Op)
	resolvedArgs := make([]Value, len(se.Args))
	for i, a := range se.Args {
		resolvedArgs[i] = ResolveValue(a, bindings)
	}

	switch normOp {
	case "left":
		if len(resolvedArgs) != 2 {
			return NewSymbol("nil"), fmt.Errorf("left expects 2 arguments (target, count), got %d", len(resolvedArgs))
		}
		target := resolvedArgs[0]
		if target.Type() == TypeSymbol && strings.EqualFold(target.Raw().(string), "nil") {
			return NewSymbol("nil"), nil
		}
		text, ok := target.Text()
		if !ok {
			return NewSymbol("nil"), fmt.Errorf("left: first argument must be string or symbol, got %s", target.Type())
		}
		count, err := toIntCount(resolvedArgs[1])
		if err != nil {
			return NewSymbol("nil"), fmt.Errorf("left: %w", err)
		}
		runes := []rune(text)
		if count <= 0 {
			if target.Type() == TypeSymbol {
				return NewSymbol(""), nil
			}
			return NewString(""), nil
		}
		if count >= int64(len(runes)) {
			return target, nil
		}
		sub := string(runes[:count])
		if target.Type() == TypeSymbol {
			return NewSymbol(sub), nil
		}
		return NewString(sub), nil

	case "right":
		if len(resolvedArgs) != 2 {
			return NewSymbol("nil"), fmt.Errorf("right expects 2 arguments (target, count), got %d", len(resolvedArgs))
		}
		target := resolvedArgs[0]
		if target.Type() == TypeSymbol && strings.EqualFold(target.Raw().(string), "nil") {
			return NewSymbol("nil"), nil
		}
		text, ok := target.Text()
		if !ok {
			return NewSymbol("nil"), fmt.Errorf("right: first argument must be string or symbol, got %s", target.Type())
		}
		count, err := toIntCount(resolvedArgs[1])
		if err != nil {
			return NewSymbol("nil"), fmt.Errorf("right: %w", err)
		}
		runes := []rune(text)
		if count <= 0 {
			if target.Type() == TypeSymbol {
				return NewSymbol(""), nil
			}
			return NewString(""), nil
		}
		if count >= int64(len(runes)) {
			return target, nil
		}
		sub := string(runes[len(runes)-int(count):])
		if target.Type() == TypeSymbol {
			return NewSymbol(sub), nil
		}
		return NewString(sub), nil

	case "contains":
		if len(resolvedArgs) != 2 {
			return NewBoolean(false), fmt.Errorf("contains expects 2 arguments, got %d", len(resolvedArgs))
		}
		t1, ok1 := resolvedArgs[0].Text()
		t2, ok2 := resolvedArgs[1].Text()
		if !ok1 || !ok2 {
			return NewBoolean(false), nil
		}
		return NewBoolean(strings.Contains(t1, t2)), nil

	case "containsignorecase":
		if len(resolvedArgs) != 2 {
			return NewBoolean(false), fmt.Errorf("contains-ignore-case expects 2 arguments, got %d", len(resolvedArgs))
		}
		t1, ok1 := resolvedArgs[0].Text()
		t2, ok2 := resolvedArgs[1].Text()
		if !ok1 || !ok2 {
			return NewBoolean(false), nil
		}
		return NewBoolean(strings.Contains(strings.ToLower(t1), strings.ToLower(t2))), nil

	case "equalsignorecase":
		if len(resolvedArgs) != 2 {
			return NewBoolean(false), fmt.Errorf("equal-ignore-case expects 2 arguments, got %d", len(resolvedArgs))
		}
		t1, ok1 := resolvedArgs[0].Text()
		t2, ok2 := resolvedArgs[1].Text()
		if !ok1 || !ok2 {
			return NewBoolean(false), nil
		}
		return NewBoolean(strings.EqualFold(t1, t2)), nil

	case "regexreplace":
		if len(resolvedArgs) != 3 {
			return NewSymbol("nil"), fmt.Errorf("regex-replace expects 3 arguments (pattern, template, target), got %d", len(resolvedArgs))
		}
		patText, okP := resolvedArgs[0].Text()
		tmplText, okT := resolvedArgs[1].Text()
		target := resolvedArgs[2]
		if target.Type() == TypeSymbol && strings.EqualFold(target.Raw().(string), "nil") {
			return NewSymbol("nil"), nil
		}
		targetText, okTarget := target.Text()
		if !okP || !okT || !okTarget {
			return NewSymbol("nil"), fmt.Errorf("regex-replace arguments must be text (string or symbol)")
		}

		re, err := GetCompiledRegex(patText)
		if err != nil {
			return NewSymbol("nil"), fmt.Errorf("regex-replace: invalid pattern: %w", err)
		}
		replaced := re.ReplaceAllString(targetText, tmplText)
		if target.Type() == TypeSymbol {
			return NewSymbol(replaced), nil
		}
		return NewString(replaced), nil

	default:
		return NewSymbol("nil"), fmt.Errorf("unknown string operation: %s", se.Op)
	}
}

func toIntCount(v Value) (int64, error) {
	if v.Type() == TypeInteger {
		return v.Raw().(int64), nil
	}
	if v.Type() == TypeFloat {
		return int64(v.Raw().(float64)), nil
	}
	return 0, fmt.Errorf("count argument must be integer or float, got %s", v.Type())
}
