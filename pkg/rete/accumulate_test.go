package rete

import (
	"math"
	"testing"

	"ops5/pkg/model"
)

func TestAccumulateNodeCount(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (accumulate (item ^order-id <oid>) :count <total-items>)
	spec := &model.AccumulateSpec{
		Op:        model.AccCount,
		ResultVar: "total-items",
	}
	ce := model.NewAccumulateCE("item", spec)
	ce.AddEqualTest("order-id", model.NewVariable("<oid>"))

	joinTests := []JoinTest{
		{
			Attribute:   "order-id",
			Op:          model.OpEqual,
			Variable:    "oid",
			VectorIndex: -1,
		},
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, spec, joinTests)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	// 1. Assert parent order token: oid=100
	orderTok := NewToken(nil, nil, map[string]model.Value{"oid": model.NewInt(100)})
	betaMem.LeftActivation(orderTok, TagAdd)

	// Since count works on empty sets, it should immediately emit count=0!
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token for count=0, got %d", len(receiver.tokens))
	}
	if receiver.tags[0] != TagAdd {
		t.Fatalf("expected TagAdd, got %v", receiver.tags[0])
	}
	if receiver.tokens[0].Bindings["total-items"].Raw().(int64) != 0 {
		t.Fatalf("expected count 0, got %v", receiver.tokens[0].Bindings["total-items"])
	}

	// 2. Assert first item
	item1 := model.NewWME(10, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item1, TagAdd)

	// Receiver should receive TagRemove for count=0, then TagAdd for count=1
	if len(receiver.tokens) != 3 {
		t.Fatalf("expected 3 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[1] != TagRemove || receiver.tokens[1].Bindings["total-items"].Raw().(int64) != 0 {
		t.Errorf("expected TagRemove count=0, got %v with %v", receiver.tags[1], receiver.tokens[1].Bindings["total-items"])
	}
	if receiver.tags[2] != TagAdd || receiver.tokens[2].Bindings["total-items"].Raw().(int64) != 1 {
		t.Errorf("expected TagAdd count=1, got %v with %v", receiver.tags[2], receiver.tokens[2].Bindings["total-items"])
	}

	// 3. Assert second item
	item2 := model.NewWME(11, "item", map[string]model.Value{"order-id": model.NewInt(100)})
	alphaMem.Activation(item2, TagAdd)

	if len(receiver.tokens) != 5 {
		t.Fatalf("expected 5 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[3] != TagRemove || receiver.tokens[3].Bindings["total-items"].Raw().(int64) != 1 {
		t.Errorf("expected TagRemove count=1, got %v", receiver.tokens[3].Bindings["total-items"])
	}
	if receiver.tags[4] != TagAdd || receiver.tokens[4].Bindings["total-items"].Raw().(int64) != 2 {
		t.Errorf("expected TagAdd count=2, got %v", receiver.tokens[4].Bindings["total-items"])
	}

	// 4. Retract item1
	alphaMem.Activation(item1, TagRemove)

	if len(receiver.tokens) != 7 {
		t.Fatalf("expected 7 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[5] != TagRemove || receiver.tokens[5].Bindings["total-items"].Raw().(int64) != 2 {
		t.Errorf("expected TagRemove count=2, got %v", receiver.tokens[5].Bindings["total-items"])
	}
	if receiver.tags[6] != TagAdd || receiver.tokens[6].Bindings["total-items"].Raw().(int64) != 1 {
		t.Errorf("expected TagAdd count=1, got %v", receiver.tokens[6].Bindings["total-items"])
	}

	// 5. Retract parent order token
	betaMem.LeftActivation(orderTok, TagRemove)
	if len(receiver.tokens) != 8 {
		t.Fatalf("expected 8 total events, got %d", len(receiver.tokens))
	}
	if receiver.tags[7] != TagRemove || receiver.tokens[7].Bindings["total-items"].Raw().(int64) != 1 {
		t.Errorf("expected final TagRemove count=1, got %v", receiver.tokens[7].Bindings["total-items"])
	}
}

func TestAccumulateNodeSum(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (accumulate (line-item ^order-id <oid> ^price <p>) :sum <p> <total>)
	spec := &model.AccumulateSpec{
		Op:        model.AccSum,
		Target:    model.NewVariable("<p>"),
		ResultVar: "total",
	}
	ce := model.NewAccumulateCE("line-item", spec)
	ce.AddEqualTest("order-id", model.NewVariable("<oid>"))
	ce.AddEqualTest("price", model.NewVariable("<p>"))

	joinTests := []JoinTest{
		{
			Attribute:   "order-id",
			Op:          model.OpEqual,
			Variable:    "oid",
			VectorIndex: -1,
		},
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, spec, joinTests)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	// 1. Assert order token
	orderTok := NewToken(nil, nil, map[string]model.Value{"oid": model.NewInt(50)})
	betaMem.LeftActivation(orderTok, TagAdd)

	if len(receiver.tokens) != 1 || receiver.tokens[0].Bindings["total"].Raw().(int64) != 0 {
		t.Fatalf("expected initial sum=0, got %v", receiver.tokens[0].Bindings["total"])
	}

	// 2. Assert line 1 (price 25)
	l1 := model.NewWME(1, "line-item", map[string]model.Value{
		"order-id": model.NewInt(50),
		"price":    model.NewInt(25),
	})
	alphaMem.Activation(l1, TagAdd)

	latest := receiver.tokens[len(receiver.tokens)-1]
	if latest.Bindings["total"].Raw().(int64) != 25 {
		t.Fatalf("expected sum=25, got %v", latest.Bindings["total"])
	}

	// 3. Assert line 2 (price 50.5 float)
	l2 := model.NewWME(2, "line-item", map[string]model.Value{
		"order-id": model.NewInt(50),
		"price":    model.NewFloat(50.5),
	})
	alphaMem.Activation(l2, TagAdd)

	latest = receiver.tokens[len(receiver.tokens)-1]
	if math.Abs(latest.Bindings["total"].Raw().(float64)-75.5) > 1e-6 {
		t.Fatalf("expected sum=75.5, got %v", latest.Bindings["total"])
	}
}

func TestAccumulateNodeAverageMinMax(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (accumulate (student ^course <c> ^grade <g>) :average <g> <gpa>)
	avgSpec := &model.AccumulateSpec{
		Op:        model.AccAverage,
		Target:    model.NewVariable("<g>"),
		ResultVar: "gpa",
	}
	ce := model.NewAccumulateCE("student", avgSpec)
	ce.AddEqualTest("course", model.NewVariable("<c>"))
	ce.AddEqualTest("grade", model.NewVariable("<g>"))

	joinTests := []JoinTest{
		{
			Attribute:   "course",
			Op:          model.OpEqual,
			Variable:    "c",
			VectorIndex: -1,
		},
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, avgSpec, joinTests)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	// Parent token for course "cs101"
	tok := NewToken(nil, nil, map[string]model.Value{"c": model.NewSymbol("cs101")})
	betaMem.LeftActivation(tok, TagAdd)

	// For average on 0 items, NO token should be emitted!
	if len(receiver.tokens) != 0 {
		t.Fatalf("expected 0 tokens for average with 0 items, got %d", len(receiver.tokens))
	}

	// Assert student 1: grade 80
	s1 := model.NewWME(1, "student", map[string]model.Value{
		"course": model.NewSymbol("cs101"),
		"grade":  model.NewInt(80),
	})
	alphaMem.Activation(s1, TagAdd)

	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token after student 1, got %d", len(receiver.tokens))
	}
	if receiver.tokens[0].Bindings["gpa"].Raw().(float64) != 80.0 {
		t.Errorf("expected gpa 80.0, got %v", receiver.tokens[0].Bindings["gpa"])
	}

	// Assert student 2: grade 90
	s2 := model.NewWME(2, "student", map[string]model.Value{
		"course": model.NewSymbol("cs101"),
		"grade":  model.NewInt(90),
	})
	alphaMem.Activation(s2, TagAdd)

	latest := receiver.tokens[len(receiver.tokens)-1]
	if latest.Bindings["gpa"].Raw().(float64) != 85.0 {
		t.Errorf("expected gpa 85.0, got %v", latest.Bindings["gpa"])
	}

	// Retract both students -> drops to 0, should retract token with no new assertion
	alphaMem.Activation(s1, TagRemove)
	alphaMem.Activation(s2, TagRemove)

	lastTag := receiver.tags[len(receiver.tags)-1]
	if lastTag != TagRemove {
		t.Errorf("expected final event to be TagRemove when count drops to 0, got %v", lastTag)
	}
}

func TestAccumulateNodeCollect(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	// Condition: (accumulate (item ^batch <bname> ^id <id>) :collect <id> <item-list>)
	spec := &model.AccumulateSpec{
		Op:        model.AccCollect,
		Target:    model.NewVariable("<id>"),
		ResultVar: "item-list",
	}
	ce := model.NewAccumulateCE("item", spec)
	ce.AddEqualTest("batch", model.NewVariable("<bname>"))
	ce.AddEqualTest("id", model.NewVariable("<id>"))

	joinTests := []JoinTest{
		{
			Attribute:   "batch",
			Op:          model.OpEqual,
			Variable:    "bname",
			VectorIndex: -1,
		},
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, spec, joinTests)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	tok := NewToken(nil, nil, map[string]model.Value{"bname": model.NewSymbol("b1")})
	betaMem.LeftActivation(tok, TagAdd)

	// Collect on empty set produces empty vector
	if len(receiver.tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(receiver.tokens))
	}
	v0 := receiver.tokens[0].Bindings["item-list"]
	if !v0.IsVector() || len(v0.VectorElements()) != 0 {
		t.Fatalf("expected empty vector, got %v", v0)
	}

	// Assert items
	i1 := model.NewWME(1, "item", map[string]model.Value{"batch": model.NewSymbol("b1"), "id": model.NewSymbol("item-A")})
	alphaMem.Activation(i1, TagAdd)
	i2 := model.NewWME(2, "item", map[string]model.Value{"batch": model.NewSymbol("b1"), "id": model.NewSymbol("item-B")})
	alphaMem.Activation(i2, TagAdd)

	latest := receiver.tokens[len(receiver.tokens)-1]
	vec := latest.Bindings["item-list"].VectorElements()
	if len(vec) != 2 {
		t.Fatalf("expected vector of 2 elements, got %d", len(vec))
	}
	if vec[0].String() != "item-A" || vec[1].String() != "item-B" {
		t.Errorf("expected [item-A item-B], got %v", vec)
	}
}

func TestAccumulateNodeComputeTarget(t *testing.T) {
	betaMem := NewBetaMemory()
	alphaMem := NewAlphaMemory()

	operands := []model.Value{model.NewVariable("<qty>"), model.NewVariable("<price>")}
	operators := []model.ComputeOp{model.ComputeOpMul}

	spec := &model.AccumulateSpec{
		Op:        model.AccSum,
		Target:    model.NewCompute(operands, operators),
		ResultVar: "grand-total",
	}
	ce := model.NewAccumulateCE("cart-item", spec)
	ce.AddEqualTest("cart-id", model.NewVariable("<cid>"))
	ce.AddEqualTest("qty", model.NewVariable("<qty>"))
	ce.AddEqualTest("price", model.NewVariable("<price>"))

	joinTests := []JoinTest{
		{
			Attribute:   "cart-id",
			Op:          model.OpEqual,
			Variable:    "cid",
			VectorIndex: -1,
		},
	}

	accNode := NewAccumulateNode(betaMem, alphaMem, ce, spec, joinTests)
	receiver := &mockBetaReceiver{}
	accNode.AddSuccessor(receiver)
	accNode.Attach()

	cartTok := NewToken(nil, nil, map[string]model.Value{"cid": model.NewInt(1)})
	betaMem.LeftActivation(cartTok, TagAdd)

	// Add item 1: 3 * 10 = 30
	alphaMem.Activation(model.NewWME(1, "cart-item", map[string]model.Value{
		"cart-id": model.NewInt(1),
		"qty":     model.NewInt(3),
		"price":   model.NewInt(10),
	}), TagAdd)

	// Add item 2: 2 * 25 = 50 -> total 80
	alphaMem.Activation(model.NewWME(2, "cart-item", map[string]model.Value{
		"cart-id": model.NewInt(1),
		"qty":     model.NewInt(2),
		"price":   model.NewInt(25),
	}), TagAdd)

	latest := receiver.tokens[len(receiver.tokens)-1]
	if latest.Bindings["grand-total"].Raw().(int64) != 80 {
		t.Fatalf("expected grand-total 80, got %v", latest.Bindings["grand-total"])
	}
}
