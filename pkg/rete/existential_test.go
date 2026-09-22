package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestExistentialJoinNodeBasicLifecycle(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (exists (item ^order-id <oid>))
	ce := model.NewExistentialCE("item")
	ce.AddEqualTest("order-id", model.NewVariable("<oid>"))

	joinTests := []JoinTest{
		{
			Attribute:   "order-id",
			Op:          model.OpEqual,
			Variable:    "oid",
			VectorIndex: -1,
		},
	}

	existNode := NewExistentialJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	existNode.AddSuccessor(receiver)
	existNode.Attach()

	// Assert order token: oid=100
	orderTok := NewToken(nil, nil, map[string]model.Value{"oid": model.NewInt(100)})
	betaMem.LeftActivation(orderTok, TagAdd)

	// No items yet -> 0 activations
	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens propagated before items exist, got %d", len(receiver.tokens))
	}

	// 1. Assert first matching item (timetag 1)
	item1 := model.NewWME(1, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item1, TagAdd)

	// Transition 0 -> 1: exactly 1 token should be propagated with TagAdd
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token propagated after first item, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Errorf("expected TagAdd, got %v", receiver.tags[0])
	}

	// 2. Assert second matching item (timetag 2) -> NO duplicate token should be emitted!
	item2 := model.NewWME(2, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item2, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected still only 1 token (no duplicates!), got %d", len(receiver.tokens))
	}

	// 3. Assert third matching item (timetag 3)
	item3 := model.NewWME(3, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item3, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected still only 1 token, got %d", len(receiver.tokens))
	}

	// 4. Retract item 1 (transition 3 -> 2 matches) -> NO-OP
	alphaMem.Activation(item1, TagRemove)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected still only 1 token after retracting 1 of 3 items, got %d", len(receiver.tokens))
	}

	// 5. Retract item 2 (transition 2 -> 1 matches) -> NO-OP
	alphaMem.Activation(item2, TagRemove)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected still only 1 token after retracting 2 of 3 items, got %d", len(receiver.tokens))
	}

	// 6. Retract item 3 (transition 1 -> 0 matches) -> should emit TagRemove!
	alphaMem.Activation(item3, TagRemove)
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 events total (Add then Remove), got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Errorf("expected TagRemove when all items are retracted, got %v", receiver.tags[1])
	}

	// 7. Assert item 4 (transition 0 -> 1 matches) -> should re-emit TagAdd!
	item4 := model.NewWME(4, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item4, TagAdd)
	if len(receiver.tokens) != 3 {
		t.Fatalf("expected 3 events total (Add, Remove, Add), got %d", len(receiver.tokens))
	}
	if receiver.tags[2] != TagAdd {
		t.Errorf("expected TagAdd when item re-asserted, got %v", receiver.tags[2])
	}
}

func TestExistentialJoinNodeLeftArrivalWithExistingWMEs(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Pre-populate alpha memory with 3 items for order 42
	item1 := model.NewWME(1, "item", map[string]model.Value{"order-id": model.NewInt(42)})
	item2 := model.NewWME(2, "item", map[string]model.Value{"order-id": model.NewInt(42)})
	item3 := model.NewWME(3, "item", map[string]model.Value{"order-id": model.NewInt(42)})
	alphaMem.Activation(item1, TagAdd)
	alphaMem.Activation(item2, TagAdd)
	alphaMem.Activation(item3, TagAdd)

	ce := model.NewExistentialCE("item")
	ce.AddEqualTest("order-id", model.NewVariable("<oid>"))

	joinTests := []JoinTest{
		{
			Attribute:   "order-id",
			Op:          model.OpEqual,
			Variable:    "oid",
			VectorIndex: -1,
		},
	}

	existNode := NewExistentialJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	existNode.AddSuccessor(receiver)
	existNode.Attach()

	// Order arrives: should match existing items and emit EXACTLY 1 token with TagAdd
	orderTok := NewToken(nil, nil, map[string]model.Value{"oid": model.NewInt(42)})
	betaMem.LeftActivation(orderTok, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected exactly 1 token emitted on left arrival with 3 existing WMEs, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Errorf("expected TagAdd, got %v", receiver.tags[0])
	}

	// Retract the order token -> should emit TagRemove
	betaMem.LeftActivation(orderTok, TagRemove)
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 events (Add then Remove), got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Errorf("expected TagRemove on token retraction, got %v", receiver.tags[1])
	}
}

func TestExistentialJoinNodeNonMatchingWMEs(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	ce := model.NewExistentialCE("item")
	ce.AddEqualTest("order-id", model.NewVariable("<oid>"))

	joinTests := []JoinTest{
		{
			Attribute:   "order-id",
			Op:          model.OpEqual,
			Variable:    "oid",
			VectorIndex: -1,
		},
	}

	existNode := NewExistentialJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	existNode.AddSuccessor(receiver)
	existNode.Attach()

	// Order token with oid=10
	orderTok := NewToken(nil, nil, map[string]model.Value{"oid": model.NewInt(10)})
	betaMem.LeftActivation(orderTok, TagAdd)

	// Items for different orders (oid=20, 30)
	otherItem1 := model.NewWME(1, "item", map[string]model.Value{"order-id": model.NewInt(20)})
	otherItem2 := model.NewWME(2, "item", map[string]model.Value{"order-id": model.NewInt(30)})
	alphaMem.Activation(otherItem1, TagAdd)
	alphaMem.Activation(otherItem2, TagAdd)

	// Receiver must have 0 tokens
	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens for non-matching order IDs, got %d", len(receiver.tokens))
	}

	// Remove successor test
	existNode.RemoveSuccessor(receiver)
	matchingItem := model.NewWME(3, "item", map[string]model.Value{"order-id": model.NewInt(10)})
	alphaMem.Activation(matchingItem, TagAdd)

	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens after RemoveSuccessor, got %d", len(receiver.tokens))
	}
}
