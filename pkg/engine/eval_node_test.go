package engine

import (
	"testing"

	"ops5/pkg/model"
	"ops5/pkg/parser"
)

func loadRule(t *testing.T, eng *Engine, src string) {
	p, err := parser.NewParser(src)
	if err != nil {
		t.Fatalf("NewParser error: %v", err)
	}
	rule, err := p.ParseRule()
	if err != nil {
		t.Fatalf("ParseRule error: %v", err)
	}
	eng.AddRule(rule)
}

func TestEngineEvalNodeBasicComparison(t *testing.T) {
	eng := New()

	// Rule: find pairs where first > second
	ruleSrc := `(p find-greater
		(pair ^first <a> ^second <b>)
		(test (<a> > <b>))
	-->
		(make result ^val <a>)
	)`
	loadRule(t, eng, ruleSrc)

	// Assert pair 10 20 (10 is NOT > 20) -> should NOT fire
	eng.Make("pair", map[string]model.Value{
		"first":  model.NewInt(10),
		"second": model.NewInt(20),
	})
	if count, _ := eng.Run(10); count != 0 {
		t.Fatalf("expected 0 rules fired for (pair 10 20), got %d", count)
	}

	// Assert pair 50 20 (50 > 20) -> should fire
	eng.Make("pair", map[string]model.Value{
		"first":  model.NewInt(50),
		"second": model.NewInt(20),
	})
	if count, _ := eng.Run(10); count != 1 {
		t.Fatalf("expected 1 rule fired for (pair 50 20), got %d", count)
	}

	results := eng.FindWMEsMatching(model.NewPositiveCE("result"))
	if len(results) != 1 {
		t.Fatalf("expected 1 result WME, got %d", len(results))
	}
	val, _ := results[0].Get("val")
	if val.Raw().(int64) != 50 {
		t.Errorf("expected result val 50, got %v", val)
	}
}

func TestEngineEvalNodeComputeArithmetic(t *testing.T) {
	eng := New()

	// Rule: discount applicable if price * qty >= 100
	ruleSrc := `(p apply-bulk-discount
		<item> (cart-item ^id <id> ^price <p> ^qty <q>)
		(test (compute <p> * <q> >= 100))
	-->
		(make discount ^item-id <id> ^rate 0.10)
	)`
	loadRule(t, eng, ruleSrc)

	// 15 * 5 = 75 < 100
	wmeSmall := eng.Make("cart-item", map[string]model.Value{
		"id":    model.NewInt(1),
		"price": model.NewInt(15),
		"qty":   model.NewInt(5),
	})
	if count, _ := eng.Run(10); count != 0 {
		t.Fatalf("expected 0 firings for sub-threshold item, got %d", count)
	}

	// 25 * 4 = 100 >= 100
	eng.Make("cart-item", map[string]model.Value{
		"id":    model.NewInt(2),
		"price": model.NewInt(25),
		"qty":   model.NewInt(4),
	})
	if count, _ := eng.Run(10); count != 1 {
		t.Fatalf("expected 1 firing for qualifying item, got %d", count)
	}

	// Modify wmeSmall to qty 10 (15 * 10 = 150 >= 100) -> should fire!
	eng.Modify(wmeSmall.Timetag, map[string]model.Value{
		"qty": model.NewInt(10),
	})
	if count, _ := eng.Run(10); count != 1 {
		t.Fatalf("expected modified item to qualify and fire, got %d firings", count)
	}
}

func TestEngineEvalNodePipelineChaining(t *testing.T) {
	eng := New()

	// Rule with conditions before and after test:
	// 1. (order ^id <id> ^requested <r>)
	// 2. (inventory ^id <id> ^available <a>)
	// 3. (test (<r> <= <a>))
	// 4. (customer ^order-id <id> ^tier vip)
	ruleSrc := `(p fulfill-vip-order
		(order ^id <id> ^requested <r>)
		(inventory ^id <id> ^available <a>)
		(test (<r> <= <a>))
		(customer ^order-id <id> ^tier vip)
	-->
		(make shipment ^order-id <id> ^status approved)
	)`
	loadRule(t, eng, ruleSrc)

	// Order 1: requested 10, available 5 (insufficient inventory)
	eng.Make("order", map[string]model.Value{"id": model.NewInt(1), "requested": model.NewInt(10)})
	eng.Make("inventory", map[string]model.Value{"id": model.NewInt(1), "available": model.NewInt(5)})
	eng.Make("customer", map[string]model.Value{"order-id": model.NewInt(1), "tier": model.NewSymbol("vip")})

	if count, _ := eng.Run(10); count != 0 {
		t.Fatalf("expected 0 firings when stock insufficient, got %d", count)
	}

	// Order 2: requested 10, available 15 (sufficient inventory)
	eng.Make("order", map[string]model.Value{"id": model.NewInt(2), "requested": model.NewInt(10)})
	inv2 := eng.Make("inventory", map[string]model.Value{"id": model.NewInt(2), "available": model.NewInt(15)})
	eng.Make("customer", map[string]model.Value{"order-id": model.NewInt(2), "tier": model.NewSymbol("vip")})

	// Before running, conflict set should have 1 activation (Order 2)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation in conflict set, got %d", eng.ConflictSet().Count())
	}

	// Now retract inventory for Order 2 -> activation should disappear
	eng.Remove(inv2.Timetag)
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after inventory retracted, got %d", eng.ConflictSet().Count())
	}

	// Re-assert inventory for Order 2 -> activation re-appears
	eng.Make("inventory", map[string]model.Value{"id": model.NewInt(2), "available": model.NewInt(20)})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation after inventory restored, got %d", eng.ConflictSet().Count())
	}

	if count, _ := eng.Run(10); count != 1 {
		t.Fatalf("expected 1 firing, got %d", count)
	}
}

func TestEngineEvalNodeRetroactiveCompilation(t *testing.T) {
	eng := New()

	// Assert facts BEFORE rule is added
	eng.Make("measurement", map[string]model.Value{"val": model.NewInt(50)})
	eng.Make("measurement", map[string]model.Value{"val": model.NewInt(150)})

	// Add rule with test retroactively
	ruleSrc := `(p high-measurement
		(measurement ^val <v>)
		(test (<v> > 100))
	-->
		(make alert ^level high ^val <v>)
	)`
	loadRule(t, eng, ruleSrc)

	// Conflict set should immediately have 1 activation for val=150
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 retroactive activation, got %d", eng.ConflictSet().Count())
	}

	if count, _ := eng.Run(10); count != 1 {
		t.Fatalf("expected 1 firing, got %d", count)
	}

	alerts := eng.FindWMEsMatching(model.NewPositiveCE("alert"))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	alertVal, _ := alerts[0].Get("val")
	if alertVal.Raw().(int64) != 150 {
		t.Errorf("expected alert for val 150, got %v", alertVal)
	}
}

func TestEngineEvalNodeExcise(t *testing.T) {
	eng := New()

	ruleSrc := `(p rule-to-excise
		(sensor ^reading <r>)
		(test (<r> > 100))
	-->
		(make warning)
	)`
	loadRule(t, eng, ruleSrc)

	eng.Make("sensor", map[string]model.Value{"reading": model.NewInt(200)})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation before excise, got %d", eng.ConflictSet().Count())
	}

	// Excise rule
	if !eng.ExciseRule("rule-to-excise") {
		t.Fatalf("excise rule returned false")
	}

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after excise, got %d", eng.ConflictSet().Count())
	}

	// Subsequent assertions should not activate excised rule
	eng.Make("sensor", map[string]model.Value{"reading": model.NewInt(300)})
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected still 0 activations for excised rule, got %d", eng.ConflictSet().Count())
	}
}
