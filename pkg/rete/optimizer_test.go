package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestOptimizeRuleJoinOrder_CartesianElimination(t *testing.T) {
	// Rule with intentional Cartesian product:
	// CE 1: (order ^id <oid> ^status pending)
	// CE 2: (customer ^cid <cid> ^vip true)      ; disconnected from order!
	// CE 3: (line-item ^order-id <oid> ^pid <pid>) ; connected to order on <oid>!
	// CE 4: (inventory ^pid <pid> ^stock < 5)    ; connected to line-item on <pid>!
	r := model.NewRule("bad-join-order")
	r.AddCondition(model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("oid")).
		AddEqualTest("status", model.NewSymbol("pending")))
	r.AddCondition(model.NewPositiveCE("customer").
		AddEqualTest("cid", model.NewVariable("cid")).
		AddEqualTest("vip", model.NewSymbol("true")))
	r.AddCondition(model.NewPositiveCE("line-item").
		AddEqualTest("order-id", model.NewVariable("oid")).
		AddEqualTest("pid", model.NewVariable("pid")))
	r.AddCondition(model.NewPositiveCE("inventory").
		AddEqualTest("pid", model.NewVariable("pid")).
		AddTest("stock", model.OpLess, model.NewInt(5)))

	opt := OptimizeRuleJoinOrder(r)
	if opt == r {
		t.Fatalf("expected rule to be reordered to eliminate Cartesian cross-product")
	}

	// Condition 1 must be anchored at index 0 (order)
	if opt.Conditions[0].Class != "order" {
		t.Fatalf("expected CE 0 to be 'order', got %s", opt.Conditions[0].Class)
	}

	// Condition 2 must now be 'line-item' (connected on <oid>)
	if opt.Conditions[1].Class != "line-item" {
		t.Fatalf("expected CE 1 to be 'line-item', got %s", opt.Conditions[1].Class)
	}

	// Condition 3 must now be 'inventory' (connected on <pid>)
	if opt.Conditions[2].Class != "inventory" {
		t.Fatalf("expected CE 2 to be 'inventory', got %s", opt.Conditions[2].Class)
	}

	// Condition 4 is 'customer'
	if opt.Conditions[3].Class != "customer" {
		t.Fatalf("expected CE 3 to be 'customer', got %s", opt.Conditions[3].Class)
	}
}

func TestOptimizeRuleJoinOrder_ActionRemapping(t *testing.T) {
	// Rule:
	// CE 1: (order ^id <oid>)
	// CE 2: (customer ^cid <cid>)
	// CE 3: (line-item ^order-id <oid>)
	// RHS:
	// (modify 1 ^status processed)
	// (modify 3 ^qty 10)
	// (remove 2)
	// (make audit ^info (substr 3 1 5))
	r := model.NewRule("remap-actions-rule")
	r.AddCondition(model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("oid")))
	r.AddCondition(model.NewPositiveCE("customer").
		AddEqualTest("cid", model.NewVariable("cid")))
	r.AddCondition(model.NewPositiveCE("line-item").
		AddEqualTest("order-id", model.NewVariable("oid")))

	r.AddAction(model.ModifyAction{
		TargetIndex: 1,
		Attributes:  map[string]model.Value{"status": model.NewSymbol("processed")},
	})
	r.AddAction(model.ModifyAction{
		TargetIndex: 3,
		Attributes:  map[string]model.Value{"qty": model.NewInt(10)},
	})
	r.AddAction(model.RemoveAction{
		TargetIndex: 2,
	})
	r.AddAction(model.MakeAction{
		Class: "audit",
		Attributes: map[string]model.Value{
			"info": model.NewSubstr(model.NewInt(3), model.NewInt(1), model.NewInt(5)),
		},
	})

	opt := OptimizeRuleJoinOrder(r)
	if opt == r {
		t.Fatalf("expected rule to be reordered")
	}

	// Expected condition order:
	// 0: order (original pos 1) -> new pos 1
	// 1: line-item (original pos 3) -> new pos 2
	// 2: customer (original pos 2) -> new pos 3

	// Check remapped actions:
	// Action 0: (modify 1 ...) -> should still be 1 (anchored CE 1)
	mod1 := opt.Actions[0].(model.ModifyAction)
	if mod1.TargetIndex != 1 {
		t.Errorf("expected mod1.TargetIndex to be 1, got %d", mod1.TargetIndex)
	}

	// Action 1: originally (modify 3 ...) -> line-item is now pos 2 -> TargetIndex should be 2
	mod3 := opt.Actions[1].(model.ModifyAction)
	if mod3.TargetIndex != 2 {
		t.Errorf("expected mod3.TargetIndex to be 2, got %d", mod3.TargetIndex)
	}

	// Action 2: originally (remove 2 ...) -> customer is now pos 3 -> TargetIndex should be 3
	rem2 := opt.Actions[2].(model.RemoveAction)
	if rem2.TargetIndex != 3 {
		t.Errorf("expected rem2.TargetIndex to be 3, got %d", rem2.TargetIndex)
	}

	// Action 3: (make audit ^info (substr 3 1 5)) -> Substr ElementRef should be remapped from 3 to 2
	mk := opt.Actions[3].(model.MakeAction)
	infoVal := mk.Attributes["info"]
	if !infoVal.IsSubstr() {
		t.Fatalf("expected SubstrExpr in make action attribute")
	}
	se := infoVal.SubstrExpr()
	if se.ElementRef.Type() != model.TypeInteger || se.ElementRef.Raw().(int64) != 2 {
		t.Errorf("expected substr ElementRef to be remapped to 2, got %v", se.ElementRef)
	}
}

func TestOptimizeRuleJoinOrder_MEAInvariant(t *testing.T) {
	// Condition 1 has lower constant selectivity than Condition 2.
	// MEA invariant mandates that Condition 1 must stay at index 0.
	r := model.NewRule("mea-invariant")
	r.AddCondition(model.NewPositiveCE("context").
		AddEqualTest("state", model.NewSymbol("active"))) // 1 constant test
	r.AddCondition(model.NewPositiveCE("candidate").
		AddEqualTest("t1", model.NewInt(1)).
		AddEqualTest("t2", model.NewInt(2)).
		AddEqualTest("t3", model.NewInt(3)).
		AddEqualTest("t4", model.NewInt(4))) // 4 constant tests

	opt := OptimizeRuleJoinOrder(r)
	if opt.Conditions[0].Class != "context" {
		t.Fatalf("MEA invariant violated: Condition 1 was moved from index 0")
	}
}

func TestOptimizeRuleJoinOrder_PrerequisiteDependencies(t *testing.T) {
	// Condition with non-equality (<>) or (compute ...) requires variable to be bound first.
	// CE 1: (goal ^step 1)
	// CE 2: (worker ^id <w2> ^ref <> <w1>)   ; requires <w1>!
	// CE 3: (worker ^id <w1> ^active true)   ; binds <w1>!
	r := model.NewRule("prereq-rule")
	r.AddCondition(model.NewPositiveCE("goal").
		AddEqualTest("step", model.NewInt(1)))
	r.AddCondition(model.NewPositiveCE("worker").
		AddEqualTest("id", model.NewVariable("w2")).
		AddTest("ref", model.OpNotEqual, model.NewVariable("w1")))
	r.AddCondition(model.NewPositiveCE("worker").
		AddEqualTest("id", model.NewVariable("w1")).
		AddEqualTest("active", model.NewSymbol("true")))

	opt := OptimizeRuleJoinOrder(r)

	// Worker that binds <w1> must be scheduled before worker testing ref <> <w1>
	foundBinding := false
	for _, ce := range opt.Conditions {
		for _, at := range ce.Tests {
			for _, c := range at.Constraints {
				if c.Op == model.OpEqual && c.Value.IsVariable() && c.Value.VariableName() == "w1" {
					foundBinding = true
				}
				if c.Op == model.OpNotEqual && c.Value.IsVariable() && c.Value.VariableName() == "w1" {
					if !foundBinding {
						t.Fatalf("dependency violation: <> <w1> scheduled before <w1> was bound")
					}
				}
			}
		}
	}
}

func TestOptimizeRuleJoinOrder_EarlyNegativePruning(t *testing.T) {
	// CE 1: (context ^state run)
	// CE 2: (item ^id <id>)
	// CE 3: (detail ^id <id> ^sub <subid>)
	// CE 4: -(blocked ^id <id>)
	r := model.NewRule("neg-pruning")
	r.AddCondition(model.NewPositiveCE("context").
		AddEqualTest("state", model.NewSymbol("run")))
	r.AddCondition(model.NewPositiveCE("item").
		AddEqualTest("id", model.NewVariable("id")))
	r.AddCondition(model.NewPositiveCE("detail").
		AddEqualTest("id", model.NewVariable("id")).
		AddEqualTest("sub", model.NewVariable("subid")))
	r.AddCondition(model.NewNegativeCE("blocked").
		AddEqualTest("id", model.NewVariable("id")))

	opt := OptimizeRuleJoinOrder(r)

	// Negative condition -(blocked ^id <id>) should be placed immediately after 'item'
	// before 'detail', so blocked items are pruned before joining with detail!
	itemIdx := -1
	blockedIdx := -1
	detailIdx := -1
	for i, ce := range opt.Conditions {
		if ce.Class == "item" {
			itemIdx = i
		} else if ce.Class == "blocked" && ce.IsNegative {
			blockedIdx = i
		} else if ce.Class == "detail" {
			detailIdx = i
		}
	}

	if itemIdx == -1 || blockedIdx == -1 || detailIdx == -1 {
		t.Fatalf("missing expected condition elements")
	}

	if !(itemIdx < blockedIdx && blockedIdx < detailIdx) {
		t.Errorf("expected item (%d) < blocked (%d) < detail (%d)", itemIdx, blockedIdx, detailIdx)
	}
}

func TestOptimizeRuleJoinOrder_StabilityAlreadyWellOrdered(t *testing.T) {
	// Rule already in optimal join order:
	// CE 1: (context ^state run)
	// CE 2: (order ^id <oid> ^status pending)
	// CE 3: (line-item ^order-id <oid>)
	r := model.NewRule("well-ordered")
	r.AddCondition(model.NewPositiveCE("context").
		AddEqualTest("state", model.NewSymbol("run")))
	r.AddCondition(model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("oid")).
		AddEqualTest("status", model.NewSymbol("pending")))
	r.AddCondition(model.NewPositiveCE("line-item").
		AddEqualTest("order-id", model.NewVariable("oid")))

	opt := OptimizeRuleJoinOrder(r)
	if opt != r {
		t.Errorf("expected already optimal rule to return identical rule pointer without allocations")
	}
}

func TestOptimizeRuleJoinOrder_NoReorderFlag(t *testing.T) {
	// Rule with bad order but NoReorder set to true
	r := model.NewRule("no-reorder-rule")
	r.NoReorder = true
	r.AddCondition(model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("oid")))
	r.AddCondition(model.NewPositiveCE("customer").
		AddEqualTest("cid", model.NewVariable("cid")))
	r.AddCondition(model.NewPositiveCE("line-item").
		AddEqualTest("order-id", model.NewVariable("oid")))

	opt := OptimizeRuleJoinOrder(r)
	if opt != r {
		t.Fatalf("expected NoReorder rule to remain untouched")
	}
	if opt.Conditions[1].Class != "customer" {
		t.Fatalf("expected customer to remain at index 1")
	}
}

func TestNetwork_JoinOptimizerDisable(t *testing.T) {
	net := NewNetwork()
	if !net.JoinOptimizerEnabled() {
		t.Errorf("expected JoinOptimizer to be enabled by default")
	}

	net.SetJoinOptimizer(false)
	if net.JoinOptimizerEnabled() {
		t.Errorf("expected JoinOptimizer to be disabled")
	}

	// Add rule with bad order with optimizer disabled
	r := model.NewRule("bad-order")
	r.AddCondition(model.NewPositiveCE("order").
		AddEqualTest("id", model.NewVariable("oid")))
	r.AddCondition(model.NewPositiveCE("customer").
		AddEqualTest("cid", model.NewVariable("cid")))
	r.AddCondition(model.NewPositiveCE("line-item").
		AddEqualTest("order-id", model.NewVariable("oid")))

	listener := &mockListener{}
	net.AddRule(r, listener)

	info := net.RuleNodeInfo("bad-order")
	if info == nil {
		t.Fatalf("expected RuleNodeInfo")
	}
	if info.Rule.Conditions[1].Class != "customer" {
		t.Errorf("expected customer to remain at index 1 when optimizer is disabled, got %s", info.Rule.Conditions[1].Class)
	}
}

type mockListener struct {
	adds    []string
	removes []string
}

func (m *mockListener) OnActivationAdd(rule *model.Rule, token *Token) {
	m.adds = append(m.adds, rule.Name+":"+token.String())
}

func (m *mockListener) OnActivationRemove(rule *model.Rule, token *Token) {
	m.removes = append(m.removes, rule.Name+":"+token.String())
}
