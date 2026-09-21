package rete

import (
	"testing"

	"ops5/pkg/model"
	"ops5/pkg/wm"
)

type recordListener struct {
	adds    []string
	removes []string
}

func (r *recordListener) OnActivationAdd(rule *model.Rule, token *Token) {
	r.adds = append(r.adds, rule.Name+":"+token.String())
}

func (r *recordListener) OnActivationRemove(rule *model.Rule, token *Token) {
	r.removes = append(r.removes, rule.Name+":"+token.String())
}

func TestSingleConditionMatchAndRetraction(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule: (goal ^status active)
	rule := model.NewRule("goal-active")
	ce := model.NewPositiveCE("goal").AddEqualTest("status", model.NewSymbol("active"))
	rule.AddCondition(ce)
	net.AddRule(rule, listener)

	// Assert matching WME
	wme1 := mem.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("active"),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 add activation, got %d", len(listener.adds))
	}

	// Assert non-matching WME (different class)
	mem.Make("task", map[string]model.Value{
		"status": model.NewSymbol("active"),
	})
	if len(listener.adds) != 1 {
		t.Fatalf("expected still 1 add activation, got %d", len(listener.adds))
	}

	// Assert non-matching WME (different status)
	mem.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("pending"),
	})
	if len(listener.adds) != 1 {
		t.Fatalf("expected still 1 add activation, got %d", len(listener.adds))
	}

	// Retract matching WME
	_, err := mem.Remove(wme1.Timetag)
	if err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove activation, got %d", len(listener.removes))
	}
}

func TestTwoConditionJoinWithVariableBinding(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule:
	// (goal ^id <gid> ^status active)
	// (task ^goal-id <gid> ^name <tname>)
	rule := model.NewRule("join-rule")
	ce1 := model.NewPositiveCE("goal").
		AddEqualTest("id", model.NewVariable("<gid>")).
		AddEqualTest("status", model.NewSymbol("active"))
	ce2 := model.NewPositiveCE("task").
		AddEqualTest("goal-id", model.NewVariable("<gid>")).
		AddEqualTest("name", model.NewVariable("<tname>"))
	rule.AddCondition(ce1).AddCondition(ce2)
	net.AddRule(rule, listener)

	// Assert goal with id 100
	mem.Make("goal", map[string]model.Value{
		"id":     model.NewInt(100),
		"status": model.NewSymbol("active"),
	})

	// No activation yet (task not present)
	if len(listener.adds) != 0 {
		t.Fatalf("expected 0 activations, got %d", len(listener.adds))
	}

	// Assert task with non-matching goal-id 200
	mem.Make("task", map[string]model.Value{
		"goal-id": model.NewInt(200),
		"name":    model.NewString("task-a"),
	})
	if len(listener.adds) != 0 {
		t.Fatalf("expected 0 activations, got %d", len(listener.adds))
	}

	// Assert matching task with goal-id 100
	taskMatching := mem.Make("task", map[string]model.Value{
		"goal-id": model.NewInt(100),
		"name":    model.NewString("task-b"),
	})
	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation, got %d", len(listener.adds))
	}

	// Retract matching task
	mem.Remove(taskMatching.Timetag)
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove activation, got %d", len(listener.removes))
	}
}

func TestNegatedConditionElement(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule:
	// (task ^id <id>)
	// -(blocker ^task-id <id>)
	rule := model.NewRule("negated-rule")
	ce1 := model.NewPositiveCE("task").AddEqualTest("id", model.NewInt(42))
	ce2 := model.NewNegativeCE("blocker").AddEqualTest("task-id", model.NewInt(42))
	rule.AddCondition(ce1).AddCondition(ce2)
	net.AddRule(rule, listener)

	// Assert task 42 -> since no blocker exists, rule should activate!
	mem.Make("task", map[string]model.Value{
		"id": model.NewInt(42),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation for satisfied negated condition, got %d", len(listener.adds))
	}

	// Assert blocker with task-id 42 -> should retract the activation!
	blocker := mem.Make("blocker", map[string]model.Value{
		"task-id": model.NewInt(42),
	})
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove activation when blocker asserted, got %d", len(listener.removes))
	}

	// Retract blocker -> should re-activate the rule!
	mem.Remove(blocker.Timetag)
	if len(listener.adds) != 2 {
		t.Fatalf("expected 2 total add activations (re-activated), got %d", len(listener.adds))
	}
}

func TestRetroactiveRuleAdditionWithExistingWMEs(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	// Pre-assert WMEs before rule is created
	wmeGoal := mem.Make("goal", map[string]model.Value{
		"id":     model.NewInt(10),
		"status": model.NewSymbol("active"),
	})
	mem.Make("task", map[string]model.Value{
		"goal-id": model.NewInt(10),
		"name":    model.NewSymbol("build"),
	})

	listener := &recordListener{}

	// Define 2-condition join rule AFTER WMEs are already in memory
	rule := model.NewRule("retro-join")
	rule.AddCondition(model.NewPositiveCE("goal").
		AddEqualTest("id", model.NewVariable("<gid>")).
		AddEqualTest("status", model.NewSymbol("active")))
	rule.AddCondition(model.NewPositiveCE("task").
		AddEqualTest("goal-id", model.NewVariable("<gid>")).
		AddEqualTest("name", model.NewVariable("<name>")))

	net.AddRuleWithWMEs(rule, listener, mem.All())

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation when rule added after WMEs, got %d", len(listener.adds))
	}

	// Retracting one of the matching WMEs should still propagate retraction
	mem.Remove(wmeGoal.Timetag)
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove activation, got %d", len(listener.removes))
	}
}

func TestNetworkRemoveRule(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	rule := model.NewRule("rule-to-remove")
	ce := model.NewPositiveCE("goal").AddEqualTest("status", model.NewSymbol("active"))
	rule.AddCondition(ce)
	net.AddRule(rule, listener)

	if !net.HasRule("rule-to-remove") {
		t.Fatalf("expected network to have rule-to-remove")
	}

	mem.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("active"),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation before removal, got %d", len(listener.adds))
	}

	if !net.RemoveRule("rule-to-remove") {
		t.Fatalf("expected RemoveRule to return true")
	}
	if net.HasRule("rule-to-remove") {
		t.Fatalf("expected network to not have rule-to-remove after removal")
	}
	if net.RemoveRule("rule-to-remove") {
		t.Fatalf("expected second RemoveRule to return false")
	}

	// Assert another matching WME - should NOT trigger any new activations
	mem.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("active"),
	})
	if len(listener.adds) != 1 {
		t.Fatalf("expected still 1 activation after rule removed, got %d", len(listener.adds))
	}
}

func TestCanonicalValueKeys(t *testing.T) {
	// Int and whole float should hash to same canonical key
	kInt := CanonicalValueKey(model.NewInt(42))
	kFloat := CanonicalValueKey(model.NewFloat(42.0))
	if kInt != kFloat {
		t.Errorf("expected int 42 and float 42.0 to have same key, got %q vs %q", kInt, kFloat)
	}

	// Boolean and symbol equivalents
	kBool := CanonicalValueKey(model.NewBoolean(true))
	kSym := CanonicalValueKey(model.NewSymbol("true"))
	kSymCaps := CanonicalValueKey(model.NewSymbol("TRUE"))
	if kBool != kSym || kBool != kSymCaps {
		t.Errorf("expected boolean and symbol 'true' to have same key, got %q vs %q", kBool, kSym)
	}

	// Distinct types should not collide
	kStr := CanonicalValueKey(model.NewString("42"))
	if kStr == kInt {
		t.Errorf("string '42' should not collide with integer 42")
	}
}

func TestHashedJoinMultiAttribute(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule joining on two variables: <dept> and <role>
	rule := model.NewRule("staff-allocation")
	ce1 := model.NewPositiveCE("worker").
		AddEqualTest("dept", model.NewVariable("<d>")).
		AddEqualTest("role", model.NewVariable("<r>")).
		AddEqualTest("name", model.NewVariable("<wname>"))
	ce2 := model.NewPositiveCE("task").
		AddEqualTest("dept", model.NewVariable("<d>")).
		AddEqualTest("role", model.NewVariable("<r>")).
		AddEqualTest("title", model.NewVariable("<ttitle>"))
	rule.AddCondition(ce1).AddCondition(ce2)
	net.AddRule(rule, listener)

	// Assert workers
	w1 := mem.Make("worker", map[string]model.Value{
		"dept": model.NewSymbol("engineering"),
		"role": model.NewSymbol("lead"),
		"name": model.NewString("Alice"),
	})
	mem.Make("worker", map[string]model.Value{
		"dept": model.NewSymbol("sales"),
		"role": model.NewSymbol("lead"),
		"name": model.NewString("Bob"),
	})

	// Assert task matching only Alice (engineering, lead)
	t1 := mem.Make("task", map[string]model.Value{
		"dept":  model.NewSymbol("engineering"),
		"role":  model.NewSymbol("lead"),
		"title": model.NewString("Architect System"),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation, got %d", len(listener.adds))
	}

	// Retract Alice -> activation should be removed
	mem.Remove(w1.Timetag)
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove activation, got %d", len(listener.removes))
	}

	// Retract task
	mem.Remove(t1.Timetag)
}

func TestHashedJoinNegativeCondition(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule:
	// (order ^id <oid> ^status pending)
	// -(cancellation ^order-id <oid>)
	rule := model.NewRule("process-uncancelled-order")
	ce1 := model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("<oid>")).
		AddEqualTest("status", model.NewSymbol("pending"))
	ce2 := model.NewNegativeCE("cancellation").
		AddEqualTest("order-id", model.NewVariable("<oid>"))
	rule.AddCondition(ce1).AddCondition(ce2)
	net.AddRule(rule, listener)

	// Assert order 101 -> immediately satisfies rule because no cancellation exists
	mem.Make("order", map[string]model.Value{
		"id":     model.NewInt(101),
		"status": model.NewSymbol("pending"),
	})
	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation, got %d", len(listener.adds))
	}

	// Assert cancellation for a DIFFERENT order (999) -> should NOT affect order 101!
	c999 := mem.Make("cancellation", map[string]model.Value{
		"order-id": model.NewInt(999),
	})
	if len(listener.removes) != 0 {
		t.Fatalf("expected 0 removes when unrelated cancellation asserted, got %d", len(listener.removes))
	}

	// Assert cancellation for order 101 -> should retract the activation!
	c101 := mem.Make("cancellation", map[string]model.Value{
		"order-id": model.NewInt(101),
	})
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 remove when matching cancellation asserted, got %d", len(listener.removes))
	}

	// Retract cancellation for order 101 -> rule should reactivate!
	mem.Remove(c101.Timetag)
	if len(listener.adds) != 2 {
		t.Fatalf("expected 2 total add activations after unblocking, got %d", len(listener.adds))
	}

	// Cleanup
	mem.Remove(c999.Timetag)
}

func BenchmarkHashedJoinScaling(b *testing.B) {
	for n := 0; n < b.N; n++ {
		net := NewNetwork()
		mem := wm.New()
		mem.AddListener(net)

		listener := &recordListener{}

		// 2-condition join rule on id
		rule := model.NewRule("join-benchmark")
		ce1 := model.NewPositiveCE("goal").
			AddEqualTest("id", model.NewVariable("<gid>"))
		ce2 := model.NewPositiveCE("task").
			AddEqualTest("goal-id", model.NewVariable("<gid>"))
		rule.AddCondition(ce1).AddCondition(ce2)
		net.AddRule(rule, listener)

		const count = 1000
		for i := 0; i < count; i++ {
			mem.Make("goal", map[string]model.Value{
				"id": model.NewInt(int64(i)),
			})
		}
		for i := 0; i < count; i++ {
			mem.Make("task", map[string]model.Value{
				"goal-id": model.NewInt(int64(i)),
			})
		}

		if len(listener.adds) != count {
			b.Fatalf("expected %d activations, got %d", count, len(listener.adds))
		}
	}
}

func TestStructuralBetaSharingInitialConditions(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule 1: (A ^id <x>) (B ^a-id <x>) (C ^val 10)
	r1 := model.NewRule("r1")
	r1.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewVariable("<x>")))
	r1.AddCondition(model.NewPositiveCE("B").AddEqualTest("a-id", model.NewVariable("<x>")))
	r1.AddCondition(model.NewPositiveCE("C").AddEqualTest("val", model.NewInt(10)))
	net.AddRule(r1, listener)

	// Rule 2: (A ^id <x>) (B ^a-id <x>) (D ^val 20)
	r2 := model.NewRule("r2")
	r2.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewVariable("<x>")))
	r2.AddCondition(model.NewPositiveCE("B").AddEqualTest("a-id", model.NewVariable("<x>")))
	r2.AddCondition(model.NewPositiveCE("D").AddEqualTest("val", model.NewInt(20)))
	net.AddRule(r2, listener)

	info1 := net.RuleNodeInfo("r1")
	info2 := net.RuleNodeInfo("r2")
	if info1 == nil || info2 == nil {
		t.Fatalf("expected node infos for both rules")
	}

	// CE 1 (A): shared BetaMemory
	if info1.BetaMems[0] != info2.BetaMems[0] {
		t.Fatalf("expected BetaMems[0] to be structurally shared across r1 and r2")
	}

	// CE 2 (B): shared BetaMemory
	if info1.BetaMems[1] != info2.BetaMems[1] {
		t.Fatalf("expected BetaMems[1] to be structurally shared across r1 and r2")
	}

	// Total unique beta nodes: 4 (A, B, C, D) instead of 6
	if net.BetaNodeCount() != 4 {
		t.Fatalf("expected 4 beta nodes in pool, got %d", net.BetaNodeCount())
	}

	// Assert WMEs for A and B
	mem.Make("A", map[string]model.Value{"id": model.NewInt(1)})
	mem.Make("B", map[string]model.Value{"a-id": model.NewInt(1)})

	// Partial match [A, B] should be stored exactly once in the shared BetaMemory
	sharedBM := info1.BetaMems[1]
	if sharedBM.TokenCount() != 1 {
		t.Fatalf("expected 1 token in shared BetaMemory, got %d", sharedBM.TokenCount())
	}

	// Assert C -> r1 should activate, r2 should not
	mem.Make("C", map[string]model.Value{"val": model.NewInt(10)})
	if len(listener.adds) != 1 || listener.adds[0][:2] != "r1" {
		t.Fatalf("expected r1 activation, got: %v", listener.adds)
	}

	// Assert D -> r2 should activate
	mem.Make("D", map[string]model.Value{"val": model.NewInt(20)})
	if len(listener.adds) != 2 || listener.adds[1][:2] != "r2" {
		t.Fatalf("expected r2 activation, got: %v", listener.adds)
	}
}

func TestStructuralBetaSharingPruningOnExcise(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule 1: (A ^id 1) (B ^val 2)
	r1 := model.NewRule("r1")
	r1.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewInt(1)))
	r1.AddCondition(model.NewPositiveCE("B").AddEqualTest("val", model.NewInt(2)))
	net.AddRule(r1, listener)

	// Rule 2: (A ^id 1) (B ^val 2)
	r2 := model.NewRule("r2")
	r2.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewInt(1)))
	r2.AddCondition(model.NewPositiveCE("B").AddEqualTest("val", model.NewInt(2)))
	net.AddRule(r2, listener)

	// Both rules share all 2 beta steps (A and B)
	if net.BetaNodeCount() != 2 {
		t.Fatalf("expected 2 shared beta nodes, got %d", net.BetaNodeCount())
	}

	// Remove r1: shared nodes should stay active because r2 is still using them
	removed := net.RemoveRule("r1")
	if !removed {
		t.Fatalf("expected r1 to be removed")
	}
	if net.BetaNodeCount() != 2 {
		t.Fatalf("expected 2 beta nodes to remain for r2, got %d", net.BetaNodeCount())
	}

	// Assert matching WMEs: r2 should still activate properly!
	mem.Make("A", map[string]model.Value{"id": model.NewInt(1)})
	mem.Make("B", map[string]model.Value{"val": model.NewInt(2)})

	if len(listener.adds) != 1 || listener.adds[0][:2] != "r2" {
		t.Fatalf("expected r2 to activate, got: %v", listener.adds)
	}

	// Remove r2: now no rules use the beta nodes -> all 2 pruned!
	removed = net.RemoveRule("r2")
	if !removed {
		t.Fatalf("expected r2 to be removed")
	}
	if net.BetaNodeCount() != 0 {
		t.Fatalf("expected 0 beta nodes in pool after all rules excised, got %d", net.BetaNodeCount())
	}
}

func TestStructuralBetaSharingAttributeOrder(t *testing.T) {
	net := NewNetwork()
	listener := &recordListener{}

	// Rule 1: attributes specified as (color, shape)
	r1 := model.NewRule("r1")
	ce1 := model.NewPositiveCE("item").
		AddEqualTest("color", model.NewSymbol("red")).
		AddEqualTest("shape", model.NewSymbol("square"))
	r1.AddCondition(ce1)
	net.AddRule(r1, listener)

	// Rule 2: attributes specified in reverse order (shape, color)
	r2 := model.NewRule("r2")
	ce2 := model.NewPositiveCE("item").
		AddEqualTest("shape", model.NewSymbol("square")).
		AddEqualTest("color", model.NewSymbol("red"))
	r2.AddCondition(ce2)
	net.AddRule(r2, listener)

	// Despite different attribute order in source, canonical sorting ensures sharing!
	if net.BetaNodeCount() != 1 {
		t.Fatalf("expected 1 shared beta node for different attribute order, got %d", net.BetaNodeCount())
	}
}


