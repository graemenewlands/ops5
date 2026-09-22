package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestNccNodeUnitLifecycle(t *testing.T) {
	betaMem := NewBetaMemory()
	// NCC subnetwork with 2 sub-conditions
	partner := NewNccPartnerNode(2)
	ce := model.NewNccCE([]*model.ConditionElement{
		model.NewPositiveCE("item"),
		model.NewPositiveCE("hazard"),
	})

	nccNode := NewNccNode(betaMem, partner, ce)
	receiver := &mockBetaReceiver{}
	nccNode.AddSuccessor(receiver)

	// 1. Assert parent order token (timetag 1)
	orderWME := model.NewWME(1, "order", map[string]model.Value{"id": model.NewInt(10)})
	parentTok := NewTokenWithMap(nil, orderWME, map[string]model.Value{"id": model.NewInt(10)})
	betaMem.LeftActivation(parentTok, TagAdd)
	nccNode.LeftActivation(parentTok, TagAdd)

	// Since subnetwork has 0 completions, NCC is satisfied!
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token propagated for empty subnetwork, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Errorf("expected TagAdd, got %v", receiver.tags[0])
	}

	// 2. Build a mock sub-token completing the 2 conditions:
	// subTok1 has parent = parentTok
	// subTok2 has parent = subTok1
	subWME1 := model.NewWME(2, "item", map[string]model.Value{"id": model.NewInt(10)})
	subTok1 := NewToken(parentTok, subWME1, nil)
	subWME2 := model.NewWME(3, "hazard", map[string]model.Value{"risk": model.NewSymbol("high")})
	subTok2 := NewToken(subTok1, subWME2, nil)

	// Sub-token arrives at partner node
	partner.LeftActivation(subTok2, TagAdd)

	// Transition 0 -> 1 completion: NccNode should emit TagRemove!
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Errorf("expected TagRemove when submatch arrives, got %v", receiver.tags[1])
	}

	// 3. Second sub-token arrives (transition 1 -> 2)
	subWME3 := model.NewWME(4, "hazard", map[string]model.Value{"risk": model.NewSymbol("extreme")})
	subTok2b := NewToken(subTok1, subWME3, nil)
	partner.LeftActivation(subTok2b, TagAdd)

	// Still blocked: NO redundant event emitted
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected still 2 events (no redundant event for 1->2), got %d", len(receiver.tokens))
	}

	// 4. Retract first sub-token (transition 2 -> 1)
	partner.LeftActivation(subTok2, TagRemove)
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected still 2 events (still blocked with 1 match), got %d", len(receiver.tokens))
	}

	// 5. Retract second sub-token (transition 1 -> 0)
	partner.LeftActivation(subTok2b, TagRemove)

	// Condition unblocked: NccNode should emit TagAdd!
	if len(receiver.tokens) != 3 {
		t.Fatalf("expected 3 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[2] != TagAdd {
		t.Errorf("expected TagAdd when completions drop to 0, got %v", receiver.tags[2])
	}

	// 6. Retract parent token from main pipeline
	nccNode.LeftActivation(parentTok, TagRemove)
	if len(receiver.tokens) != 4 {
		t.Fatalf("expected 4 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[3] != TagRemove {
		t.Errorf("expected TagRemove on parent token retraction, got %v", receiver.tags[3])
	}
}
