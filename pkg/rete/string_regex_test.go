package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/wm"
)

type captureListener struct {
	tokens []*Token
}

func (cl *captureListener) OnActivationAdd(rule *model.Rule, token *Token) {
	cl.tokens = append(cl.tokens, token)
}

func (cl *captureListener) OnActivationRemove(rule *model.Rule, token *Token) {
	for i, t := range cl.tokens {
		if t == token {
			cl.tokens = append(cl.tokens[:i], cl.tokens[i+1:]...)
			return
		}
	}
}

func TestReteRegexCaptureGroupsAndJoinConstraints(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)
	listener := &captureListener{}

	// Rule:
	// (p test-regex
	//    (filter ^allowed <country>)
	//    (item ^code (regex "^([A-Z]{2})-([0-9]{4})$" <country> <id>))
	//    -->
	// )
	r := model.NewRule("test-regex")
	ce1 := model.NewPositiveCE("filter").AddEqualTest("allowed", model.NewVariable("<country>"))
	ce2 := model.NewPositiveCE("item").AddTest("code", model.OpEqual, model.NewRegex(`^([A-Z]{2})-([0-9]{4})$`, "country", "id"))
	r.AddCondition(ce1)
	r.AddCondition(ce2)

	net.AddRule(r, listener)

	// Assert filter for "US"
	mem.Make("filter", map[string]model.Value{"allowed": model.NewSymbol("US")})

	// Assert item with US code (matches both regex and join constraint with filter)
	mem.Make("item", map[string]model.Value{"code": model.NewSymbol("US-1234")})

	if len(listener.tokens) != 1 {
		t.Fatalf("expected 1 activation for US-1234, got %d", len(listener.tokens))
	}
	tok := listener.tokens[0]
	countryVal, ok1 := tok.GetBinding("country")
	idVal, ok2 := tok.GetBinding("id")
	if !ok1 || countryVal.Raw().(string) != "US" {
		t.Errorf("expected <country> = US, got %v (ok=%v)", countryVal, ok1)
	}
	if !ok2 || idVal.Raw().(string) != "1234" {
		t.Errorf("expected <id> = 1234, got %v (ok=%v)", idVal, ok2)
	}

	// Assert item with CA code (matches regex, but FAILS join constraint because <country> != "US")
	mem.Make("item", map[string]model.Value{"code": model.NewSymbol("CA-5678")})

	if len(listener.tokens) != 1 {
		t.Errorf("expected still 1 activation (CA-5678 should fail join constraint), got %d", len(listener.tokens))
	}

	// Assert non-matching item (fails regex)
	mem.Make("item", map[string]model.Value{"code": model.NewSymbol("INVALID")})

	if len(listener.tokens) != 1 {
		t.Errorf("expected still 1 activation, got %d", len(listener.tokens))
	}
}

func TestReteRegexNamedGroupsAndOptionalFallback(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)
	listener := &captureListener{}

	// Rule with named groups and optional section:
	// (p test-opt
	//    (sku ^value (regex "^(?P<cat>[A-Z]{3})(?:-(?P<num>[0-9]+))?$"))
	//    -->
	// )
	r := model.NewRule("test-opt")
	ce := model.NewPositiveCE("sku").AddTest("value", model.OpEqual, model.NewRegex(`^(?P<cat>[A-Z]{3})(?:-(?P<num>[0-9]+))?$`, "cat", "num"))
	r.AddCondition(ce)

	net.AddRule(r, listener)

	// Item with optional part present
	mem.Make("sku", map[string]model.Value{"value": model.NewString("ABC-999")})

	if len(listener.tokens) != 1 {
		t.Fatalf("expected 1 activation, got %d", len(listener.tokens))
	}
	tok1 := listener.tokens[0]
	cat1, _ := tok1.GetBinding("cat")
	num1, _ := tok1.GetBinding("num")
	if cat1.Raw().(string) != "ABC" || num1.Raw().(string) != "999" {
		t.Errorf("expected ABC and 999, got %v / %v", cat1, num1)
	}

	// Item with optional part absent (should fall back to empty string "")
	mem.Make("sku", map[string]model.Value{"value": model.NewString("XYZ")})

	if len(listener.tokens) != 2 {
		t.Fatalf("expected 2 activations, got %d", len(listener.tokens))
	}
	tok2 := listener.tokens[1]
	cat2, _ := tok2.GetBinding("cat")
	num2, okNum2 := tok2.GetBinding("num")
	if !okNum2 || num2.Raw().(string) != "" || num2.Type() != model.TypeString {
		t.Errorf("expected empty string for optional group, got %v (ok=%v, type=%s)", num2, okNum2, num2.Type())
	}
	if cat2.Raw().(string) != "XYZ" {
		t.Errorf("expected XYZ, got %v", cat2)
	}
}

func TestReteEqualIgnoreCaseAlphaMatching(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)
	listener := &captureListener{}

	// (p test-ic
	//    (account ^role =~ "admin")
	//    -->
	// )
	r := model.NewRule("test-ic")
	ce := model.NewPositiveCE("account").AddTest("role", model.OpEqualIgnoreCase, model.NewString("admin"))
	r.AddCondition(ce)

	net.AddRule(r, listener)

	// Symbol "ADMIN" matches string "admin"
	mem.Make("account", map[string]model.Value{"role": model.NewSymbol("ADMIN")})

	if len(listener.tokens) != 1 {
		t.Fatalf("expected 1 activation for ADMIN, got %d", len(listener.tokens))
	}

	// String "Admin" matches
	mem.Make("account", map[string]model.Value{"role": model.NewString("Admin")})

	if len(listener.tokens) != 2 {
		t.Fatalf("expected 2 activations, got %d", len(listener.tokens))
	}

	// User does NOT match
	mem.Make("account", map[string]model.Value{"role": model.NewSymbol("user")})

	if len(listener.tokens) != 2 {
		t.Fatalf("expected still 2 activations, got %d", len(listener.tokens))
	}
}
