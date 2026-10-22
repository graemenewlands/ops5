package model

import (
	"testing"
)

func TestTextFunctionsLeftAndRight(t *testing.T) {
	bindings := map[string]Value{
		"sym": NewSymbol("hello"),
		"str": NewString("world"),
		"cjk": NewString("日本語"),
		"nil": NewSymbol("nil"),
	}

	// 1. Left on symbol
	eLeftSym := NewStringExpr("left", NewVariable("<sym>"), NewInt(2))
	res, err := EvaluateStringExpr(eLeftSym.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("left error: %v", err)
	}
	if res.Type() != TypeSymbol || res.Raw().(string) != "he" {
		t.Errorf("expected symbol 'he', got %v (%s)", res, res.Type())
	}

	// 2. Right on string
	eRightStr := NewStringExpr("right", NewVariable("<str>"), NewInt(3))
	res, err = EvaluateStringExpr(eRightStr.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("right error: %v", err)
	}
	if res.Type() != TypeString || res.Raw().(string) != "rld" {
		t.Errorf("expected string 'rld', got %v (%s)", res, res.Type())
	}

	// 3. Multibyte CJK runes
	eLeftCJK := NewStringExpr("left", NewVariable("<cjk>"), NewInt(2))
	res, err = EvaluateStringExpr(eLeftCJK.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("left CJK error: %v", err)
	}
	if res.Raw().(string) != "日本" {
		t.Errorf("expected '日本', got %v", res)
	}

	eRightCJK := NewStringExpr("right", NewVariable("<cjk>"), NewInt(1))
	res, err = EvaluateStringExpr(eRightCJK.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("right CJK error: %v", err)
	}
	if res.Raw().(string) != "語" {
		t.Errorf("expected '語', got %v", res)
	}

	// 4. Clamping out of bounds
	eClamp := NewStringExpr("left", NewVariable("<sym>"), NewInt(100))
	res, err = EvaluateStringExpr(eClamp.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("left clamp error: %v", err)
	}
	if res.Raw().(string) != "hello" {
		t.Errorf("expected clamped 'hello', got %v", res)
	}

	// 5. Zero / negative count
	eZero := NewStringExpr("left", NewVariable("<sym>"), NewInt(0))
	res, err = EvaluateStringExpr(eZero.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("left zero error: %v", err)
	}
	if res.Raw().(string) != "" || res.Type() != TypeSymbol {
		t.Errorf("expected empty symbol, got %v (%s)", res, res.Type())
	}

	// 6. Nil propagation
	eNil := NewStringExpr("left", NewVariable("<nil>"), NewInt(2))
	res, err = EvaluateStringExpr(eNil.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("left nil error: %v", err)
	}
	if res.Raw().(string) != "nil" {
		t.Errorf("expected nil symbol, got %v", res)
	}
}

func TestTextPredicates(t *testing.T) {
	bindings := map[string]Value{
		"text": NewString("OPS5 Rules Engine"),
		"sym":  NewSymbol("rules"),
		"num":  NewInt(42),
	}

	// Contains (case sensitive)
	c1 := NewStringExpr("contains", NewVariable("<text>"), NewString("Rules"))
	res, err := EvaluateStringExpr(c1.StringExpr(), bindings)
	if err != nil || !res.Raw().(bool) {
		t.Errorf("expected contains=true, got %v (err: %v)", res, err)
	}

	c2 := NewStringExpr("contains", NewVariable("<text>"), NewString("rules"))
	res, err = EvaluateStringExpr(c2.StringExpr(), bindings)
	if err != nil || res.Raw().(bool) {
		t.Errorf("expected contains=false (case-sensitive), got %v", res)
	}

	// ContainsIgnoreCase (cross-type symbol vs string)
	cic1 := NewStringExpr("contains-ignore-case", NewVariable("<text>"), NewVariable("<sym>"))
	res, err = EvaluateStringExpr(cic1.StringExpr(), bindings)
	if err != nil || !res.Raw().(bool) {
		t.Errorf("expected containsIgnoreCase=true, got %v", res)
	}

	// Non-textual argument returns false
	cBad := NewStringExpr("contains", NewVariable("<text>"), NewVariable("<num>"))
	res, err = EvaluateStringExpr(cBad.StringExpr(), bindings)
	if err != nil || res.Raw().(bool) {
		t.Errorf("expected non-text contains=false, got %v", res)
	}

	// EqualsIgnoreCase
	eqIC := NewStringExpr("equal-ignore-case", NewSymbol("admin"), NewString("ADMIN"))
	res, err = EvaluateStringExpr(eqIC.StringExpr(), bindings)
	if err != nil || !res.Raw().(bool) {
		t.Errorf("expected equalsIgnoreCase=true, got %v", res)
	}
}

func TestRegexReplace(t *testing.T) {
	bindings := map[string]Value{
		"phone": NewString("5551234"),
		"sku":   NewSymbol("US-9876"),
		"nil":   NewSymbol("nil"),
	}

	// Positional groups $1-$2
	rr1 := NewStringExpr("regex-replace", NewString(`^(\d{3})(\d{4})$`), NewString(`$1-$2`), NewVariable("<phone>"))
	res, err := EvaluateStringExpr(rr1.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("regex-replace error: %v", err)
	}
	if res.Type() != TypeString || res.Raw().(string) != "555-1234" {
		t.Errorf("expected '555-1234', got %v (%s)", res, res.Type())
	}

	// Symbol target preserves TypeSymbol
	rr2 := NewStringExpr("regex-replace", NewString(`^([A-Z]{2})-(\d{4})$`), NewString(`SKU:${1}:${2}`), NewVariable("<sku>"))
	res, err = EvaluateStringExpr(rr2.StringExpr(), bindings)
	if err != nil {
		t.Fatalf("regex-replace error: %v", err)
	}
	if res.Type() != TypeSymbol || res.Raw().(string) != "SKU:US:9876" {
		t.Errorf("expected symbol 'SKU:US:9876', got %v (%s)", res, res.Type())
	}

	// Nil target
	rrNil := NewStringExpr("regex-replace", NewString(`.*`), NewString(`replaced`), NewVariable("<nil>"))
	res, err = EvaluateStringExpr(rrNil.StringExpr(), bindings)
	if err != nil || res.Raw().(string) != "nil" {
		t.Errorf("expected nil symbol, got %v", res)
	}
}

func TestOperatorsEqualIgnoreCaseAndRegex(t *testing.T) {
	// 1. =~ operator
	if !EvalOp(NewSymbol("admin"), OpEqualIgnoreCase, NewString("ADMIN")) {
		t.Errorf("expected admin =~ ADMIN to be true")
	}
	if EvalOp(NewSymbol("admin"), OpEqualIgnoreCase, NewString("user")) {
		t.Errorf("expected admin =~ user to be false")
	}
	// Non-text
	if EvalOp(NewInt(123), OpEqualIgnoreCase, NewString("123")) {
		t.Errorf("expected 123 =~ '123' to be false (strong typing)")
	}

	// 2. !~ operator
	if EvalOp(NewSymbol("admin"), OpNotEqualIgnoreCase, NewString("ADMIN")) {
		t.Errorf("expected admin !~ ADMIN to be false")
	}
	if !EvalOp(NewSymbol("admin"), OpNotEqualIgnoreCase, NewString("user")) {
		t.Errorf("expected admin !~ user to be true")
	}
	if !EvalOp(NewInt(123), OpNotEqualIgnoreCase, NewString("123")) {
		t.Errorf("expected 123 !~ '123' to be true (strong typing)")
	}

	// 3. Regex matching in EvalOp
	reVal := NewRegex(`^[A-Z]{2}-\d{4}$`, "country", "id")
	if !EvalOp(NewString("US-1234"), OpEqual, reVal) {
		t.Errorf("expected US-1234 to match regex")
	}
	if EvalOp(NewString("invalid-code"), OpEqual, reVal) {
		t.Errorf("expected invalid-code to NOT match regex")
	}
	if !EvalOp(NewString("invalid-code"), OpNotEqual, reVal) {
		t.Errorf("expected invalid-code != regex to be true")
	}
}
