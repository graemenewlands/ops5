package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestJoinNodeRightUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (item ^id <id>) with join on <id> == <order-id>
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))

	joinTests := []JoinTest{
		{
			Attribute:   "id",
			Op:          model.OpEqual,
			Variable:    "id",
			VectorIndex: -1,
		},
	}

	joinNode := NewJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	joinNode.AddSuccessor(receiver)
	joinNode.Attach()

	// 1. Initially, alphaMemory has 0 items. JoinNode must be right-unlinked from betaMemory.
	if joinNode.IsLeftLinked() {
		t.Fatalf("expected JoinNode to be left-unlinked when AlphaMemory is empty")
	}
	if betaMem.ActiveSuccessorCount() != 0 {
		t.Fatalf("expected betaMem active successor count 0, got %d", betaMem.ActiveSuccessorCount())
	}

	// 2. Assert a token into betaMemory.
	tok1 := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(42)})
	betaMem.LeftActivation(tok1, TagAdd)

	// Since join is unlinked from betaMemory, receiver receives nothing yet.
	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens at receiver while join is unlinked, got %d", len(receiver.tokens))
	}

	// 3. Assert first WME into alphaMemory (transition 0 -> 1).
	// This must trigger OnRightMemoryNonEmpty(), re-linking join to betaMemory.
	wme1 := model.NewWME(1, "item", map[string]model.Value{"id": model.NewInt(42)})
	alphaMem.Activation(wme1, TagAdd)

	if !joinNode.IsLeftLinked() {
		t.Fatalf("expected JoinNode to be left-linked after alpha transition 0 -> 1")
	}
	if betaMem.ActiveSuccessorCount() != 1 {
		t.Fatalf("expected betaMem active successor count 1, got %d", betaMem.ActiveSuccessorCount())
	}
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token at receiver after alpha WME arrived, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Fatalf("expected TagAdd, got %v", receiver.tags[0])
	}

	// 4. Assert second matching WME into alphaMemory (transition 1 -> 2).
	wme2 := model.NewWME(2, "item", map[string]model.Value{"id": model.NewInt(42)})
	alphaMem.Activation(wme2, TagAdd)

	if !joinNode.IsLeftLinked() {
		t.Fatalf("expected JoinNode to remain left-linked")
	}
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 tokens at receiver, got %d", len(receiver.tokens))
	}

	// 5. Retract first WME (transition 2 -> 1).
	alphaMem.Activation(wme1, TagRemove)
	if !joinNode.IsLeftLinked() {
		t.Fatalf("expected JoinNode to remain left-linked with 1 item in alpha")
	}
	if len(receiver.tokens) != 3 { // 2 adds + 1 remove
		t.Fatalf("expected 3 events at receiver, got %d", len(receiver.tokens))
	}
	if receiver.tags[2] != TagRemove {
		t.Fatalf("expected TagRemove, got %v", receiver.tags[2])
	}

	// 6. Retract second WME (transition 1 -> 0).
	// This must retract downstream first, then unlink join from betaMemory.
	alphaMem.Activation(wme2, TagRemove)
	if joinNode.IsLeftLinked() {
		t.Fatalf("expected JoinNode to be left-unlinked after alpha transition 1 -> 0")
	}
	if betaMem.ActiveSuccessorCount() != 0 {
		t.Fatalf("expected betaMem active successor count 0, got %d", betaMem.ActiveSuccessorCount())
	}
	if len(receiver.tokens) != 4 { // 2 adds + 2 removes
		t.Fatalf("expected 4 events at receiver, got %d", len(receiver.tokens))
	}
	if receiver.tags[3] != TagRemove {
		t.Fatalf("expected TagRemove, got %v", receiver.tags[3])
	}

	// 7. Add another token to betaMemory while unlinked.
	tok2 := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(99)})
	betaMem.LeftActivation(tok2, TagAdd)
	if len(receiver.tokens) != 4 {
		t.Fatalf("expected no new tokens while unlinked, got %d", len(receiver.tokens))
	}
}

func TestJoinNodeLeftUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))

	joinTests := []JoinTest{
		{
			Attribute:   "id",
			Op:          model.OpEqual,
			Variable:    "id",
			VectorIndex: -1,
		},
	}

	joinNode := NewJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	joinNode.AddSuccessor(receiver)
	joinNode.Attach()

	// 1. Initially, betaMemory has 0 tokens. JoinNode must be right-unlinked from alphaMemory.
	if joinNode.IsRightLinked() {
		t.Fatalf("expected JoinNode to be right-unlinked from alphaMemory when BetaMemory is empty")
	}
	if alphaMem.ActiveSuccessorCount() != 0 {
		t.Fatalf("expected alphaMem active successor count 0, got %d", alphaMem.ActiveSuccessorCount())
	}

	// 2. Assert a WME into alphaMemory.
	wme1 := model.NewWME(10, "item", map[string]model.Value{"id": model.NewInt(100)})
	alphaMem.Activation(wme1, TagAdd)

	// Since join is unlinked from alphaMemory, receiver receives nothing yet.
	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens at receiver, got %d", len(receiver.tokens))
	}

	// 3. Assert first matching token into betaMemory (transition 0 -> 1).
	// This must trigger OnLeftMemoryNonEmpty(), re-linking join to alphaMemory.
	tok1 := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(100)})
	betaMem.LeftActivation(tok1, TagAdd)

	if !joinNode.IsRightLinked() {
		t.Fatalf("expected JoinNode to be right-linked after beta transition 0 -> 1")
	}
	if alphaMem.ActiveSuccessorCount() != 1 {
		t.Fatalf("expected alphaMem active successor count 1, got %d", alphaMem.ActiveSuccessorCount())
	}
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token at receiver after beta token arrived, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Fatalf("expected TagAdd, got %v", receiver.tags[0])
	}

	// 4. Retract token from betaMemory (transition 1 -> 0).
	betaMem.LeftActivation(tok1, TagRemove)

	if joinNode.IsRightLinked() {
		t.Fatalf("expected JoinNode to be right-unlinked after beta transition 1 -> 0")
	}
	if alphaMem.ActiveSuccessorCount() != 0 {
		t.Fatalf("expected alphaMem active successor count 0, got %d", alphaMem.ActiveSuccessorCount())
	}
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 events at receiver (1 add + 1 remove), got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Fatalf("expected TagRemove, got %v", receiver.tags[1])
	}

	// 5. Add another WME into alphaMemory while unlinked.
	wme2 := model.NewWME(11, "item", map[string]model.Value{"id": model.NewInt(200)})
	alphaMem.Activation(wme2, TagAdd)
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected no new tokens while unlinked, got %d", len(receiver.tokens))
	}
}

func TestAsymmetricArrivalOrderEquivalence(t *testing.T) {
	buildEngine := func() (*BetaMemory, *AlphaMemory, *mockBetaReceiver) {
		bm := NewBetaMemory()
		am := NewAlphaMemory()
		ce := model.NewPositiveCE("item")
		ce.AddEqualTest("val", model.NewVariable("<x>"))
		tests := []JoinTest{
			{Attribute: "val", Op: model.OpEqual, Variable: "x", VectorIndex: -1},
		}
		jn := NewJoinNode(bm, am, ce, tests)
		rec := &mockBetaReceiver{}
		jn.AddSuccessor(rec)
		jn.Attach()
		return bm, am, rec
	}

	// Engine 1: Tokens first, then WMEs
	bm1, am1, rec1 := buildEngine()
	toks := []*Token{
		NewToken(nil, nil, map[string]model.Value{"x": model.NewInt(1)}),
		NewToken(nil, nil, map[string]model.Value{"x": model.NewInt(2)}),
		NewToken(nil, nil, map[string]model.Value{"x": model.NewInt(3)}),
	}
	wmes := []*model.WME{
		model.NewWME(1, "item", map[string]model.Value{"val": model.NewInt(1)}),
		model.NewWME(2, "item", map[string]model.Value{"val": model.NewInt(2)}),
		model.NewWME(3, "item", map[string]model.Value{"val": model.NewInt(3)}),
	}

	for _, tok := range toks {
		bm1.LeftActivation(tok, TagAdd)
	}
	for _, wme := range wmes {
		am1.Activation(wme, TagAdd)
	}

	// Engine 2: WMEs first, then Tokens
	bm2, am2, rec2 := buildEngine()
	for _, wme := range wmes {
		am2.Activation(wme, TagAdd)
	}
	for _, tok := range toks {
		bm2.LeftActivation(tok, TagAdd)
	}

	if len(rec1.tokens) != 3 {
		t.Fatalf("engine 1 expected 3 matches, got %d", len(rec1.tokens))
	}
	if len(rec2.tokens) != 3 {
		t.Fatalf("engine 2 expected 3 matches, got %d", len(rec2.tokens))
	}

	// Compare tokens
	for i := 0; i < 3; i++ {
		t1 := rec1.tokens[i]
		t2 := rec2.tokens[i]
		if t1.Bindings["x"].Raw() != t2.Bindings["x"].Raw() {
			t.Errorf("match mismatch at %d: %v vs %v", i, t1.Bindings["x"], t2.Bindings["x"])
		}
	}
}

func TestNegativeJoinNodeUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	ce := model.NewNegativeCE("blocked")
	ce.AddEqualTest("id", model.NewVariable("<id>"))

	joinTests := []JoinTest{
		{
			Attribute:   "id",
			Op:          model.OpEqual,
			Variable:    "id",
			VectorIndex: -1,
		},
	}

	negNode := NewNegativeJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	negNode.AddSuccessor(receiver)
	negNode.Attach()

	// NegativeJoinNode must ALWAYS be left-linked, even when alpha is empty!
	if !negNode.IsLeftLinked() {
		t.Fatalf("expected NegativeJoinNode to always be left-linked")
	}
	// But it should be right-unlinked when beta has 0 tokens
	if negNode.IsRightLinked() {
		t.Fatalf("expected NegativeJoinNode to be right-unlinked when BetaMemory is empty")
	}

	// 1. Assert token when alpha is empty -> condition is satisfied, token propagates immediately!
	tok1 := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(77)})
	betaMem.LeftActivation(tok1, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token propagated for satisfied negative join, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Fatalf("expected TagAdd, got %v", receiver.tags[0])
	}
	// BetaMemory now has 1 token, so NegativeJoinNode must be right-linked to AlphaMemory
	if !negNode.IsRightLinked() {
		t.Fatalf("expected NegativeJoinNode to be right-linked after beta has 1 token")
	}

	// 2. Assert blocking WME into alphaMemory
	wme1 := model.NewWME(100, "blocked", map[string]model.Value{"id": model.NewInt(77)})
	alphaMem.Activation(wme1, TagAdd)

	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 events at receiver (1 add + 1 remove), got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Fatalf("expected TagRemove when blocked, got %v", receiver.tags[1])
	}

	// 3. Retract blocking WME -> satisfied again!
	alphaMem.Activation(wme1, TagRemove)
	if len(receiver.tokens) != 3 {
		t.Fatalf("expected 3 events at receiver, got %d", len(receiver.tokens))
	}
	if receiver.tags[2] != TagAdd {
		t.Fatalf("expected TagAdd when unblocked, got %v", receiver.tags[2])
	}

	// 4. Retract beta token -> transitions 1 -> 0
	betaMem.LeftActivation(tok1, TagRemove)
	if negNode.IsRightLinked() {
		t.Fatalf("expected NegativeJoinNode to be right-unlinked when BetaMemory transitions 1 -> 0")
	}
}

func TestExistentialJoinNodeUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	ce := model.NewExistentialCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))

	joinTests := []JoinTest{
		{
			Attribute:   "id",
			Op:          model.OpEqual,
			Variable:    "id",
			VectorIndex: -1,
		},
	}

	existNode := NewExistentialJoinNode(betaMem, alphaMem, ce, joinTests)
	receiver := &mockBetaReceiver{}
	existNode.AddSuccessor(receiver)
	existNode.Attach()

	// ExistentialJoinNode is always left-linked
	if !existNode.IsLeftLinked() {
		t.Fatalf("expected ExistentialJoinNode to always be left-linked")
	}
	// Right-unlinked when beta has 0 tokens
	if existNode.IsRightLinked() {
		t.Fatalf("expected ExistentialJoinNode to be right-unlinked when Beta is empty")
	}

	// Assert token to BetaMemory
	tok1 := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(55)})
	betaMem.LeftActivation(tok1, TagAdd)

	if !existNode.IsRightLinked() {
		t.Fatalf("expected ExistentialJoinNode to be right-linked after Beta has 1 token")
	}

	// Retract token
	betaMem.LeftActivation(tok1, TagRemove)
	if existNode.IsRightLinked() {
		t.Fatalf("expected ExistentialJoinNode to be right-unlinked after Beta has 0 tokens")
	}
}

func TestAccumulateNodeUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	ce := model.NewPositiveCE("item")
	spec := &model.AccumulateSpec{
		Op:        model.AccCount,
		ResultVar: "<count>",
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, spec, nil)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	// AccumulateNode is always left-linked
	if !accNode.IsLeftLinked() {
		t.Fatalf("expected AccumulateNode to always be left-linked")
	}
	// Right-unlinked when beta has 0 tokens
	if accNode.IsRightLinked() {
		t.Fatalf("expected AccumulateNode to be right-unlinked when Beta is empty")
	}

	tok1 := NewToken(nil, nil, map[string]model.Value{})
	betaMem.LeftActivation(tok1, TagAdd)

	if !accNode.IsRightLinked() {
		t.Fatalf("expected AccumulateNode to be right-unlinked after Beta has 1 token")
	}
	// Aggregation with 0 items produces count = 0
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token from accumulate on empty alpha, got %d", len(receiver.tokens))
	}
	if receiver.tokens[0].Bindings["count"].Raw().(int64) != 0 {
		t.Fatalf("expected count 0, got %v", receiver.tokens[0].Bindings["count"])
	}

	betaMem.LeftActivation(tok1, TagRemove)
	if accNode.IsRightLinked() {
		t.Fatalf("expected AccumulateNode to be right-unlinked after Beta has 0 tokens")
	}
}

func TestMultipleJoinsSharedMemoriesUnlinking(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMemA := NewAlphaMemory()
	alphaMemB := NewAlphaMemory()

	ceA := model.NewPositiveCE("typeA")
	ceA.AddEqualTest("x", model.NewVariable("<x>"))
	jnA := NewJoinNode(betaMem, alphaMemA, ceA, []JoinTest{
		{Attribute: "x", Op: model.OpEqual, Variable: "x", VectorIndex: -1},
	})
	recA := &mockBetaReceiver{}
	jnA.AddSuccessor(recA)
	jnA.Attach()

	ceB := model.NewPositiveCE("typeB")
	ceB.AddEqualTest("x", model.NewVariable("<x>"))
	jnB := NewJoinNode(betaMem, alphaMemB, ceB, []JoinTest{
		{Attribute: "x", Op: model.OpEqual, Variable: "x", VectorIndex: -1},
	})
	recB := &mockBetaReceiver{}
	jnB.AddSuccessor(recB)
	jnB.Attach()

	// Initially both alpha memories are empty -> both joins left-unlinked
	if betaMem.ActiveSuccessorCount() != 0 {
		t.Fatalf("expected 0 active successors on betaMem, got %d", betaMem.ActiveSuccessorCount())
	}

	// Add 1 WME to alphaMemB only -> jnB becomes linked, jnA remains unlinked
	wmeB := model.NewWME(1, "typeB", map[string]model.Value{"x": model.NewInt(10)})
	alphaMemB.Activation(wmeB, TagAdd)

	if betaMem.ActiveSuccessorCount() != 1 {
		t.Fatalf("expected 1 active successor on betaMem, got %d", betaMem.ActiveSuccessorCount())
	}
	if jnA.IsLeftLinked() {
		t.Fatalf("expected jnA to still be unlinked")
	}
	if !jnB.IsLeftLinked() {
		t.Fatalf("expected jnB to be linked")
	}

	// Add token to betaMem -> only jnB receives activation and matches
	tok := NewToken(nil, nil, map[string]model.Value{"x": model.NewInt(10)})
	betaMem.LeftActivation(tok, TagAdd)

	if len(recA.tokens) != 0 {
		t.Fatalf("expected 0 tokens at recA, got %d", len(recA.tokens))
	}
	if len(recB.tokens) != 1 {
		t.Fatalf("expected 1 token at recB, got %d", len(recB.tokens))
	}

	// Now add 1 WME to alphaMemA -> jnA links and immediately matches
	wmeA := model.NewWME(2, "typeA", map[string]model.Value{"x": model.NewInt(10)})
	alphaMemA.Activation(wmeA, TagAdd)

	if betaMem.ActiveSuccessorCount() != 2 {
		t.Fatalf("expected 2 active successors on betaMem, got %d", betaMem.ActiveSuccessorCount())
	}
	if !jnA.IsLeftLinked() {
		t.Fatalf("expected jnA to now be linked")
	}
	if len(recA.tokens) != 1 {
		t.Fatalf("expected 1 token at recA after alphaMemA became non-empty, got %d", len(recA.tokens))
	}
}
