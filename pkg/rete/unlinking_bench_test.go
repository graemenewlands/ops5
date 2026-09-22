package rete

import (
	"fmt"
	"testing"

	"ops5/pkg/model"
)

// BenchmarkJoinRightUnlinked measures throughput of sending left tokens to a BetaMemory
// when the child JoinNode is right-unlinked (AlphaMemory has 0 items).
func BenchmarkJoinRightUnlinked(b *testing.B) {
	bm := NewBetaMemory()
	am := NewAlphaMemory()
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))
	jn := NewJoinNode(bm, am, ce, []JoinTest{
		{Attribute: "id", Op: model.OpEqual, Variable: "id", VectorIndex: -1},
	})
	jn.Attach()

	tokens := make([]*Token, b.N)
	for i := 0; i < b.N; i++ {
		tokens[i] = NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(int64(i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bm.LeftActivation(tokens[i], TagAdd)
	}
}

// BenchmarkJoinRightLinkedEmptyMatches measures throughput when child JoinNode is linked
// (AlphaMemory has 1 non-matching item).
func BenchmarkJoinRightLinkedEmptyMatches(b *testing.B) {
	bm := NewBetaMemory()
	am := NewAlphaMemory()
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))
	jn := NewJoinNode(bm, am, ce, []JoinTest{
		{Attribute: "id", Op: model.OpEqual, Variable: "id", VectorIndex: -1},
	})
	jn.Attach()

	// Assert 1 non-matching WME so join is linked
	nonMatchingWME := model.NewWME(-1, "item", map[string]model.Value{"id": model.NewInt(-1)})
	am.Activation(nonMatchingWME, TagAdd)

	tokens := make([]*Token, b.N)
	for i := 0; i < b.N; i++ {
		tokens[i] = NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(int64(i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bm.LeftActivation(tokens[i], TagAdd)
	}
}

// BenchmarkJoinLeftUnlinked measures throughput of sending right WMEs to an AlphaMemory
// when the child JoinNode is left-unlinked (BetaMemory has 0 tokens).
func BenchmarkJoinLeftUnlinked(b *testing.B) {
	bm := NewBetaMemory()
	am := NewAlphaMemory()
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))
	jn := NewJoinNode(bm, am, ce, []JoinTest{
		{Attribute: "id", Op: model.OpEqual, Variable: "id", VectorIndex: -1},
	})
	jn.Attach()

	wmes := make([]*model.WME, b.N)
	for i := 0; i < b.N; i++ {
		wmes[i] = model.NewWME(int64(i+1), "item", map[string]model.Value{"id": model.NewInt(int64(i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		am.Activation(wmes[i], TagAdd)
	}
}

// BenchmarkJoinLeftLinkedEmptyMatches measures throughput when child JoinNode is linked
// (BetaMemory has 1 non-matching token).
func BenchmarkJoinLeftLinkedEmptyMatches(b *testing.B) {
	bm := NewBetaMemory()
	am := NewAlphaMemory()
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))
	jn := NewJoinNode(bm, am, ce, []JoinTest{
		{Attribute: "id", Op: model.OpEqual, Variable: "id", VectorIndex: -1},
	})
	jn.Attach()

	// Assert 1 non-matching token so join is right-linked
	nonMatchingTok := NewToken(nil, nil, map[string]model.Value{"id": model.NewInt(-1)})
	bm.LeftActivation(nonMatchingTok, TagAdd)

	wmes := make([]*model.WME, b.N)
	for i := 0; i < b.N; i++ {
		wmes[i] = model.NewWME(int64(i+1), "item", map[string]model.Value{"id": model.NewInt(int64(i))})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		am.Activation(wmes[i], TagAdd)
	}
}

// BenchmarkUnlinkingTransitionCycle measures the overhead of repeated 0 <-> 1 unlinking transitions.
func BenchmarkUnlinkingTransitionCycle(b *testing.B) {
	bm := NewBetaMemory()
	am := NewAlphaMemory()
	ce := model.NewPositiveCE("item")
	ce.AddEqualTest("id", model.NewVariable("<id>"))
	jn := NewJoinNode(bm, am, ce, []JoinTest{
		{Attribute: "id", Op: model.OpEqual, Variable: "id", VectorIndex: -1},
	})
	jn.Attach()

	wme := model.NewWME(1, "item", map[string]model.Value{"id": model.NewInt(1)})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Transition 0 -> 1: links
		am.Activation(wme, TagAdd)
		// Transition 1 -> 0: unlinks
		am.Activation(wme, TagRemove)
	}
}

// BenchmarkMultiJoinAsymmetricMemories simulates 10 join nodes attached to a single AlphaMemory,
// where 9 are unlinked (their BetaMemories are empty) and only 1 is linked.
func BenchmarkMultiJoinAsymmetricMemories(b *testing.B) {
	am := NewAlphaMemory()
	for i := 0; i < 10; i++ {
		bm := NewBetaMemory()
		ce := model.NewPositiveCE(fmt.Sprintf("item%d", i))
		jn := NewJoinNode(bm, am, ce, nil)
		jn.Attach()
		if i == 0 {
			// Only the first beta memory has tokens
			tok := NewToken(nil, nil, nil)
			bm.LeftActivation(tok, TagAdd)
		}
	}

	wmes := make([]*model.WME, b.N)
	for i := 0; i < b.N; i++ {
		wmes[i] = model.NewWME(int64(i+1), "item0", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		am.Activation(wmes[i], TagAdd)
	}
}
