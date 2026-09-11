package conflict

import (
	"testing"

	"ops5/pkg/model"
	"ops5/pkg/rete"
)

func makeDummyToken(timetags []int64) *rete.Token {
	tok := rete.DummyRootToken()
	for _, tag := range timetags {
		wme := model.NewWME(tag, "class", nil)
		tok = rete.NewToken(tok, wme, nil)
	}
	return tok
}

func TestLexCompare(t *testing.T) {
	ruleA := model.NewRule("ruleA")
	ruleA.Index = 1
	ruleA.AddCondition(model.NewPositiveCE("c1").AddEqualTest("a", model.NewInt(1)))

	ruleB := model.NewRule("ruleB")
	ruleB.Index = 2
	ruleB.AddCondition(model.NewPositiveCE("c1").AddEqualTest("a", model.NewInt(1)))

	// Case 1: Recency dominance
	// a: [10, 5] -> sorted [10, 5]
	// b: [9, 8]  -> sorted [9, 8]
	actA := NewActivation(ruleA, makeDummyToken([]int64{10, 5}))
	actB := NewActivation(ruleB, makeDummyToken([]int64{9, 8}))

	if LexCompare(actA, actB) <= 0 {
		t.Fatalf("expected actA to dominate actB under LEX")
	}

	// Case 2: Specificity tie-breaker when timetags match
	// Both [10, 5]
	ruleMoreSpecific := model.NewRule("ruleSpec")
	ruleMoreSpecific.Index = 3
	ruleMoreSpecific.AddCondition(model.NewPositiveCE("c1").
		AddEqualTest("a", model.NewInt(1)).
		AddEqualTest("b", model.NewInt(2))) // higher specificity

	actC := NewActivation(ruleMoreSpecific, makeDummyToken([]int64{10, 5}))
	if LexCompare(actC, actA) <= 0 {
		t.Fatalf("expected actC to dominate actA under LEX due to specificity")
	}
}

func TestMeaVsLex(t *testing.T) {
	// Rule 1: Goal A (older goal 4) with recent fact (15)
	// Rule 2: Goal B (newer goal 10) with older fact (2)
	rule1 := model.NewRule("rule1")
	rule1.Index = 1
	rule1.AddCondition(model.NewPositiveCE("goal"))
	rule1.AddCondition(model.NewPositiveCE("fact"))

	rule2 := model.NewRule("rule2")
	rule2.Index = 2
	rule2.AddCondition(model.NewPositiveCE("goal"))
	rule2.AddCondition(model.NewPositiveCE("fact"))

	act1 := NewActivation(rule1, makeDummyToken([]int64{4, 15}))
	act2 := NewActivation(rule2, makeDummyToken([]int64{10, 2}))

	// Under LEX:
	// act1 sorted is [15, 4]
	// act2 sorted is [10, 2]
	// 15 > 10 => act1 dominates
	if LexCompare(act1, act2) <= 0 {
		t.Fatalf("under LEX, act1 [15, 4] should dominate act2 [10, 2]")
	}

	// Under MEA:
	// CE1 of act1 is 4
	// CE1 of act2 is 10
	// 10 > 4 => act2 dominates!
	if MeaCompare(act2, act1) <= 0 {
		t.Fatalf("under MEA, act2 (CE1=10) should dominate act1 (CE1=4)")
	}
}

func TestConflictSetRefraction(t *testing.T) {
	cs := NewSet()
	rule := model.NewRule("test-rule")
	rule.AddCondition(model.NewPositiveCE("test"))
	token := makeDummyToken([]int64{100})

	cs.OnActivationAdd(rule, token)
	if cs.Count() != 1 {
		t.Fatalf("expected 1 activation, got %d", cs.Count())
	}

	dom, ok := cs.SelectDominant()
	if !ok || dom.Rule.Name != "test-rule" {
		t.Fatalf("failed to select dominant activation")
	}

	// Fire activation (refraction)
	cs.MarkFired(dom)
	if cs.Count() != 0 {
		t.Fatalf("expected 0 activations after firing, got %d", cs.Count())
	}

	// Attempting to re-add the same activation should be rejected due to refraction
	cs.OnActivationAdd(rule, token)
	if cs.Count() != 0 {
		t.Fatalf("expected refracted activation not to be added back to conflict set")
	}
}

func TestConflictSetRemoveRule(t *testing.T) {
	cs := NewSet()
	ruleA := model.NewRule("ruleA")
	ruleB := model.NewRule("ruleB")
	tok1 := makeDummyToken([]int64{10})
	tok2 := makeDummyToken([]int64{20})

	cs.OnActivationAdd(ruleA, tok1)
	cs.OnActivationAdd(ruleA, tok2)
	cs.OnActivationAdd(ruleB, tok1)

	if cs.Count() != 3 {
		t.Fatalf("expected 3 activations, got %d", cs.Count())
	}

	removed := cs.RemoveRule("ruleA")
	if removed != 2 {
		t.Errorf("expected 2 activations removed for ruleA, got %d", removed)
	}
	if cs.Count() != 1 {
		t.Errorf("expected 1 activation remaining (ruleB), got %d", cs.Count())
	}

	dom, ok := cs.SelectDominant()
	if !ok || dom.Rule.Name != "ruleB" {
		t.Errorf("expected dominant to be ruleB, got %v", dom)
	}
}
