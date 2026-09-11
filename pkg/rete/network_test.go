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

