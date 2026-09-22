package conflict

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
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

func TestSalienceDominance(t *testing.T) {
	// Rule Low: salience 0, newer timetag [100], higher specificity (2 conditions)
	ruleLow := model.NewRule("ruleLow").SetSalience(0)
	ruleLow.Index = 1
	ruleLow.AddCondition(model.NewPositiveCE("c1").AddEqualTest("x", model.NewInt(1)))
	ruleLow.AddCondition(model.NewPositiveCE("c2").AddEqualTest("y", model.NewInt(2)))

	// Rule High: salience 100, older timetag [10], lower specificity (1 condition)
	ruleHigh := model.NewRule("ruleHigh").SetSalience(100)
	ruleHigh.Index = 2
	ruleHigh.AddCondition(model.NewPositiveCE("c1"))

	// Rule Negative: salience -50, older timetag [5]
	ruleNeg := model.NewRule("ruleNeg").SetSalience(-50)
	ruleNeg.Index = 3
	ruleNeg.AddCondition(model.NewPositiveCE("c1"))

	actLow := NewActivation(ruleLow, makeDummyToken([]int64{100, 99}))
	actHigh := NewActivation(ruleHigh, makeDummyToken([]int64{10}))
	actNeg := NewActivation(ruleNeg, makeDummyToken([]int64{5}))

	// Under LEX:
	// Without salience, actLow (timetag 100) would dominate actHigh (timetag 10).
	// But with salience 100 vs 0, actHigh MUST dominate actLow!
	if LexCompare(actHigh, actLow) <= 0 {
		t.Errorf("LEX: expected actHigh (salience 100) to dominate actLow (salience 0)")
	}
	if LexCompare(actLow, actNeg) <= 0 {
		t.Errorf("LEX: expected actLow (salience 0) to dominate actNeg (salience -50)")
	}

	// Under MEA:
	// Without salience, actLow (CE1=100) would dominate actHigh (CE1=10).
	// But with salience 100 vs 0, actHigh MUST dominate actLow!
	if MeaCompare(actHigh, actLow) <= 0 {
		t.Errorf("MEA: expected actHigh (salience 100) to dominate actLow (salience 0)")
	}

	// Test conflict set All() ordering
	cs := NewSet()
	cs.OnActivationAdd(ruleLow, makeDummyToken([]int64{100, 99}))
	cs.OnActivationAdd(ruleNeg, makeDummyToken([]int64{5}))
	cs.OnActivationAdd(ruleHigh, makeDummyToken([]int64{10}))

	all := cs.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 activations, got %d", len(all))
	}
	if all[0].Rule.Name != "ruleHigh" {
		t.Errorf("expected 1st dominant activation to be ruleHigh (salience 100), got %s", all[0].Rule.Name)
	}
	if all[1].Rule.Name != "ruleLow" {
		t.Errorf("expected 2nd activation to be ruleLow (salience 0), got %s", all[1].Rule.Name)
	}
	if all[2].Rule.Name != "ruleNeg" {
		t.Errorf("expected 3rd activation to be ruleNeg (salience -50), got %s", all[2].Rule.Name)
	}

	// Dominant selection in ConflictSet
	dom, ok := cs.SelectDominant()
	if !ok || dom.Rule.Name != "ruleHigh" {
		t.Errorf("expected dominant to be ruleHigh, got %v", dom)
	}
}

func TestBinaryHeapAgenda(t *testing.T) {
	cs := NewSet()

	rules := make([]*model.Rule, 5)
	for i := 0; i < 5; i++ {
		rules[i] = model.NewRule(string(rune('A' + i)))
		rules[i].Index = i + 1
		rules[i].AddCondition(model.NewPositiveCE("cond"))
	}

	// Add in non-sorted order with timetags:
	// Rule A: [10]
	// Rule B: [50]
	// Rule C: [30]
	// Rule D: [100]
	// Rule E: [20]
	cs.OnActivationAdd(rules[0], makeDummyToken([]int64{10}))
	cs.OnActivationAdd(rules[1], makeDummyToken([]int64{50}))
	cs.OnActivationAdd(rules[2], makeDummyToken([]int64{30}))
	cs.OnActivationAdd(rules[3], makeDummyToken([]int64{100}))
	cs.OnActivationAdd(rules[4], makeDummyToken([]int64{20}))

	if cs.Count() != 5 {
		t.Fatalf("expected 5 activations, got %d", cs.Count())
	}

	// Dominant should be Rule D [100]
	dom, ok := cs.SelectDominant()
	if !ok || dom.Rule.Name != "D" {
		t.Fatalf("expected dominant 'D', got %v", dom)
	}

	// Fire D -> next dominant should be B [50]
	cs.MarkFired(dom)
	dom, ok = cs.SelectDominant()
	if !ok || dom.Rule.Name != "B" {
		t.Fatalf("expected dominant 'B', got %v", dom)
	}

	// Remove C [30] (from middle of heap)
	cs.OnActivationRemove(rules[2], makeDummyToken([]int64{30}))
	if cs.Count() != 3 {
		t.Fatalf("expected 3 activations, got %d", cs.Count())
	}

	// Dominant should still be B [50]
	dom, ok = cs.SelectDominant()
	if !ok || dom.Rule.Name != "B" {
		t.Fatalf("expected dominant 'B', got %v", dom)
	}

	// Fire B -> next should be E [20]
	cs.MarkFired(dom)
	dom, ok = cs.SelectDominant()
	if !ok || dom.Rule.Name != "E" {
		t.Fatalf("expected dominant 'E', got %v", dom)
	}

	// Fire E -> next should be A [10]
	cs.MarkFired(dom)
	dom, ok = cs.SelectDominant()
	if !ok || dom.Rule.Name != "A" {
		t.Fatalf("expected dominant 'A', got %v", dom)
	}

	// Fire A -> should be empty
	cs.MarkFired(dom)
	dom, ok = cs.SelectDominant()
	if ok || dom != nil {
		t.Fatalf("expected empty agenda, got %v", dom)
	}
	if cs.Count() != 0 {
		t.Fatalf("expected count 0, got %d", cs.Count())
	}
}

