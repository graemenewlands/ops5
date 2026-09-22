package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

type mockBetaReceiver struct {
	tokens []*Token
	tags   []PropagationTag
}

func (m *mockBetaReceiver) LeftActivation(token *Token, tag PropagationTag) {
	m.tokens = append(m.tokens, token)
	m.tags = append(m.tags, tag)
}

func TestEvalNodeSimpleComparison(t *testing.T) {
	// (test (<x> > <y>))
	cmp := model.EvalComparison{
		Left:     model.NewVariable("<x>"),
		Op:       model.OpGreater,
		Right:    model.NewVariable("<y>"),
		HasRight: true,
	}
	evalTest := model.NewEvalTest(cmp)
	node := NewEvalNode(evalTest)
	receiver := &mockBetaReceiver{}
	node.AddSuccessor(receiver)

	// Token 1: <x>=20, <y>=10 (should pass)
	tok1 := NewTokenWithMap(nil, nil, map[string]model.Value{
		"x": model.NewInt(20),
		"y": model.NewInt(10),
	})
	node.LeftActivation(tok1, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token propagated, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Errorf("expected TagAdd, got %v", receiver.tags[0])
	}

	// Token 2: <x>=5, <y>=10 (should be filtered)
	tok2 := NewTokenWithMap(nil, nil, map[string]model.Value{
		"x": model.NewInt(5),
		"y": model.NewInt(10),
	})
	node.LeftActivation(tok2, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected still 1 token propagated, got %d", len(receiver.tokens))
	}

	// Token 1 retract (TagRemove)
	node.LeftActivation(tok1, TagRemove)
	if len(receiver.tokens) != 2 {
		t.Fatalf("expected 2 tokens total after retract, got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove {
		t.Errorf("expected TagRemove, got %v", receiver.tags[1])
	}
}

func TestEvalNodeComputeArithmetic(t *testing.T) {
	// (test (compute <price> * <qty> >= 100))
	compExpr := model.NewCompute(
		[]model.Value{model.NewVariable("<price>"), model.NewVariable("<qty>")},
		[]model.ComputeOp{model.ComputeOpMul},
	)
	cmp := model.EvalComparison{
		Left:     compExpr,
		Op:       model.OpGreaterEqual,
		Right:    model.NewInt(100),
		HasRight: true,
	}
	evalTest := model.NewEvalTest(cmp)
	node := NewEvalNode(evalTest)
	receiver := &mockBetaReceiver{}
	node.AddSuccessor(receiver)

	// Item 1: price 25 * qty 4 = 100 (passes)
	tok1 := NewTokenWithMap(nil, nil, map[string]model.Value{
		"price": model.NewInt(25),
		"qty":   model.NewInt(4),
	})
	node.LeftActivation(tok1, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token for item 1, got %d", len(receiver.tokens))
	}

	// Item 2: price 15 * qty 5 = 75 (blocked)
	tok2 := NewTokenWithMap(nil, nil, map[string]model.Value{
		"price": model.NewInt(15),
		"qty":   model.NewInt(5),
	})
	node.LeftActivation(tok2, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected item 2 to be filtered, got %d tokens", len(receiver.tokens))
	}
}

func TestEvalNodeMultipleComparisons(t *testing.T) {
	// (test (<x> >= 10) (<x> <= 20))
	cmp1 := model.EvalComparison{
		Left:     model.NewVariable("<x>"),
		Op:       model.OpGreaterEqual,
		Right:    model.NewInt(10),
		HasRight: true,
	}
	cmp2 := model.EvalComparison{
		Left:     model.NewVariable("<x>"),
		Op:       model.OpLessEqual,
		Right:    model.NewInt(20),
		HasRight: true,
	}
	node := NewEvalNode(model.NewEvalTest(cmp1, cmp2))
	receiver := &mockBetaReceiver{}
	node.AddSuccessor(receiver)

	// Test 15 (passes both)
	tok1 := NewTokenWithMap(nil, nil, map[string]model.Value{"x": model.NewInt(15)})
	node.LeftActivation(tok1, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Errorf("expected 15 to pass")
	}

	// Test 5 (fails cmp1)
	tok2 := NewTokenWithMap(nil, nil, map[string]model.Value{"x": model.NewInt(5)})
	node.LeftActivation(tok2, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Errorf("expected 5 to fail")
	}

	// Test 25 (fails cmp2)
	tok3 := NewTokenWithMap(nil, nil, map[string]model.Value{"x": model.NewInt(25)})
	node.LeftActivation(tok3, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Errorf("expected 25 to fail")
	}
}

func TestEvalPredicateNode(t *testing.T) {
	// Custom predicate function checking even number
	node := NewEvalPredicateNode(func(bindings map[string]model.Value) bool {
		val, ok := bindings["num"]
		if !ok || val.Type() != model.TypeInteger {
			return false
		}
		return val.Raw().(int64)%2 == 0
	})
	receiver := &mockBetaReceiver{}
	node.AddSuccessor(receiver)

	tokEven := NewTokenWithMap(nil, nil, map[string]model.Value{"num": model.NewInt(42)})
	tokOdd := NewTokenWithMap(nil, nil, map[string]model.Value{"num": model.NewInt(43)})

	node.LeftActivation(tokEven, TagAdd)
	node.LeftActivation(tokOdd, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected only even token to pass, got %d", len(receiver.tokens))
	}
	if receiver.tokens[0].Bindings()["num"].Raw().(int64) != 42 {
		t.Errorf("expected token with num=42, got %v", receiver.tokens[0].Bindings()["num"])
	}
}

func TestEvalNodeRemoveSuccessor(t *testing.T) {
	node := NewEvalNode(model.NewEvalTest(model.EvalComparison{
		Left:     model.NewVariable("<x>"),
		Op:       model.OpEqual,
		Right:    model.NewInt(1),
		HasRight: true,
	}))
	receiver := &mockBetaReceiver{}
	node.AddSuccessor(receiver)

	tok := NewTokenWithMap(nil, nil, map[string]model.Value{"x": model.NewInt(1)})
	node.LeftActivation(tok, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token")
	}

	node.RemoveSuccessor(receiver)
	node.LeftActivation(tok, TagAdd)
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected no new tokens after RemoveSuccessor, got %d", len(receiver.tokens))
	}
}
