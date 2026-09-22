package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/wm"
)

type recordAlphaSuccessor struct {
	adds    []*model.WME
	removes []*model.WME
}

func (r *recordAlphaSuccessor) Activation(wme *model.WME, tag PropagationTag) {
	if tag == TagAdd {
		r.adds = append(r.adds, wme)
	} else {
		r.removes = append(r.removes, wme)
	}
}

func TestAlphaSwitchNodeDirect(t *testing.T) {
	asn := NewAlphaSwitchNode("status")

	succActive := &recordAlphaSuccessor{}
	succPending := &recordAlphaSuccessor{}

	asn.AddSuccessor("s:active", succActive)
	asn.AddSuccessor("s:pending", succPending)

	if asn.BranchCount() != 2 {
		t.Fatalf("expected 2 branches, got %d", asn.BranchCount())
	}
	if len(asn.CaseKeys()) != 2 {
		t.Fatalf("expected 2 case keys, got %d", len(asn.CaseKeys()))
	}
	if len(asn.SuccessorsForCase("s:active")) != 1 {
		t.Fatalf("expected 1 successor for active case")
	}

	// 1. Assert WME with status = active
	wme1 := model.NewWME(1, "task", map[string]model.Value{
		"status": model.NewSymbol("active"),
	})
	asn.Activation(wme1, TagAdd)

	if len(succActive.adds) != 1 || succActive.adds[0] != wme1 {
		t.Errorf("expected active successor to receive wme1 add")
	}
	if len(succPending.adds) != 0 {
		t.Errorf("expected pending successor to receive 0 adds, got %d", len(succPending.adds))
	}

	// 2. Assert WME with status = pending
	wme2 := model.NewWME(2, "task", map[string]model.Value{
		"status": model.NewSymbol("pending"),
	})
	asn.Activation(wme2, TagAdd)

	if len(succPending.adds) != 1 || succPending.adds[0] != wme2 {
		t.Errorf("expected pending successor to receive wme2 add")
	}
	if len(succActive.adds) != 1 {
		t.Errorf("expected active successor to remain at 1 add")
	}

	// 3. Assert WME with non-matching status
	wme3 := model.NewWME(3, "task", map[string]model.Value{
		"status": model.NewSymbol("completed"),
	})
	asn.Activation(wme3, TagAdd)
	if len(succActive.adds) != 1 || len(succPending.adds) != 1 {
		t.Errorf("expected no additional activations for unknown status")
	}

	// 4. Assert WME with missing status (evaluates to nil)
	wme4 := model.NewWME(4, "task", map[string]model.Value{})
	asn.Activation(wme4, TagAdd)
	if len(succActive.adds) != 1 || len(succPending.adds) != 1 {
		t.Errorf("expected no additional activations for missing status")
	}

	// 5. Retract WME1
	asn.Activation(wme1, TagRemove)
	if len(succActive.removes) != 1 || succActive.removes[0] != wme1 {
		t.Errorf("expected active successor to receive wme1 remove")
	}
	if len(succPending.removes) != 0 {
		t.Errorf("expected pending successor to receive 0 removes")
	}
}

func TestAlphaSwitchNodeVectorAttribute(t *testing.T) {
	// Positional vector switch: vals[1]
	asnPos := NewIndexedAlphaSwitchNode("vals", 1)
	succB := &recordAlphaSuccessor{}
	asnPos.AddSuccessor("s:b", succB)

	// WME with vector [a, b, c] -> vals[1] == b
	wmeVec := model.NewWME(10, "data", map[string]model.Value{
		"vals": model.NewVector([]model.Value{
			model.NewSymbol("a"),
			model.NewSymbol("b"),
			model.NewSymbol("c"),
		}),
	})
	asnPos.Activation(wmeVec, TagAdd)
	if len(succB.adds) != 1 {
		t.Errorf("expected positional match on vals[1] == b")
	}

	// Membership vector switch: vecIdx == -1
	asnMem := NewIndexedAlphaSwitchNode("tags", -1)
	succGold := &recordAlphaSuccessor{}
	succVip := &recordAlphaSuccessor{}
	asnMem.AddSuccessor("s:gold", succGold)
	asnMem.AddSuccessor("s:vip", succVip)

	wmeMember := model.NewWME(20, "customer", map[string]model.Value{
		"tags": model.NewVector([]model.Value{
			model.NewSymbol("silver"),
			model.NewSymbol("gold"),
			model.NewSymbol("gold"), // Duplicate to test deduplication
		}),
	})
	asnMem.Activation(wmeMember, TagAdd)
	if len(succGold.adds) != 1 {
		t.Errorf("expected exactly 1 add on membership match, got %d", len(succGold.adds))
	}
	if len(succVip.adds) != 0 {
		t.Errorf("expected 0 adds for vip")
	}
}

func TestAlphaSwitchNodeCrossTypeNormalization(t *testing.T) {
	asnNum := NewAlphaSwitchNode("priority")
	succNum := &recordAlphaSuccessor{}
	asnNum.AddSuccessor("i:10", succNum)

	// Assert with integer 10
	wmeInt := model.NewWME(1, "task", map[string]model.Value{
		"priority": model.NewInt(10),
	})
	asnNum.Activation(wmeInt, TagAdd)

	// Assert with float 10.0 (should normalize to i:10)
	wmeFloat := model.NewWME(2, "task", map[string]model.Value{
		"priority": model.NewFloat(10.0),
	})
	asnNum.Activation(wmeFloat, TagAdd)

	if len(succNum.adds) != 2 {
		t.Fatalf("expected 2 adds matching priority 10 across int/float, got %d", len(succNum.adds))
	}

	// Boolean vs Symbol normalization
	asnBool := NewAlphaSwitchNode("flag")
	succBool := &recordAlphaSuccessor{}
	asnBool.AddSuccessor("b:true", succBool)

	wmeBool := model.NewWME(3, "item", map[string]model.Value{
		"flag": model.NewBoolean(true),
	})
	asnBool.Activation(wmeBool, TagAdd)

	wmeSym := model.NewWME(4, "item", map[string]model.Value{
		"flag": model.NewSymbol("true"),
	})
	asnBool.Activation(wmeSym, TagAdd)

	if len(succBool.adds) != 2 {
		t.Fatalf("expected 2 adds matching flag true across bool/symbol, got %d", len(succBool.adds))
	}
}

func TestAlphaConstantDispatchInNetwork(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Compile 4 rules testing different states on class "context"
	states := []string{"start", "assign_seats", "make_path", "check_done"}
	for _, st := range states {
		rule := model.NewRule("rule-" + st)
		ce := model.NewPositiveCE("context").AddEqualTest("state", model.NewSymbol(st))
		rule.AddCondition(ce)
		net.AddRule(rule, listener)
	}

	// Assert WME for make_path
	wmePath := mem.Make("context", map[string]model.Value{
		"state": model.NewSymbol("make_path"),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected exactly 1 activation, got %d", len(listener.adds))
	}
	if listener.adds[0] != "rule-make_path:Token[timetags=[1]]" {
		t.Errorf("unexpected activation: %s", listener.adds[0])
	}

	// Assert WME for assign_seats
	wmeAssign := mem.Make("context", map[string]model.Value{
		"state": model.NewSymbol("assign_seats"),
	})
	if len(listener.adds) != 2 {
		t.Fatalf("expected 2 activations, got %d", len(listener.adds))
	}

	// Retract make_path
	mem.Remove(wmePath.Timetag)
	if len(listener.removes) != 1 {
		t.Fatalf("expected 1 retraction, got %d", len(listener.removes))
	}
	if listener.removes[0] != "rule-make_path:Token[timetags=[1]]" {
		t.Errorf("unexpected retraction: %s", listener.removes[0])
	}

	_ = wmeAssign
}

func TestAlphaSwitchNodeMultiAttributeChaining(t *testing.T) {
	net := NewNetwork()
	mem := wm.New()
	mem.AddListener(net)

	listener := &recordListener{}

	// Rule 1: (edge ^joined false ^label unknown)
	r1 := model.NewRule("r-unknown")
	r1.AddCondition(model.NewPositiveCE("edge").
		AddEqualTest("joined", model.NewSymbol("false")).
		AddEqualTest("label", model.NewSymbol("unknown")))
	net.AddRule(r1, listener)

	// Rule 2: (edge ^joined false ^label horizontal)
	r2 := model.NewRule("r-horizontal")
	r2.AddCondition(model.NewPositiveCE("edge").
		AddEqualTest("joined", model.NewSymbol("false")).
		AddEqualTest("label", model.NewSymbol("horizontal")))
	net.AddRule(r2, listener)

	// Rule 3: (edge ^joined true)
	r3 := model.NewRule("r-true")
	r3.AddCondition(model.NewPositiveCE("edge").
		AddEqualTest("joined", model.NewSymbol("true")))
	net.AddRule(r3, listener)

	// Assert edge with joined=false, label=unknown -> should only trigger r-unknown
	wme1 := mem.Make("edge", map[string]model.Value{
		"joined": model.NewSymbol("false"),
		"label":  model.NewSymbol("unknown"),
	})

	if len(listener.adds) != 1 {
		t.Fatalf("expected 1 activation for wme1, got %d", len(listener.adds))
	}
	if listener.adds[0] != "r-unknown:Token[timetags=[1]]" {
		t.Errorf("expected r-unknown activation, got %s", listener.adds[0])
	}

	// Assert edge with joined=true -> should only trigger r-true
	mem.Make("edge", map[string]model.Value{
		"joined": model.NewSymbol("true"),
	})
	if len(listener.adds) != 2 {
		t.Fatalf("expected 2 total activations, got %d", len(listener.adds))
	}
	if listener.adds[1] != "r-true:Token[timetags=[2]]" {
		t.Errorf("expected r-true activation, got %s", listener.adds[1])
	}

	_ = wme1
}
