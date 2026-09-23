package engine

import (
	"bytes"
	"testing"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

func TestCompiledAction_MakeModifyRemove(t *testing.T) {
	eng := New()
	eng.SetWatchLevel(0)

	// Declare schema
	eng.DeclareClass("point", []string{"x", "y"})

	r := model.NewRule("test-lifecycle")
	r.AddCondition(model.NewPositiveCE("start").WithElementVariable("s"))
	r.AddAction(model.MakeAction{
		Class: "point",
		Attributes: map[string]model.Value{
			"x": model.NewInt(10),
			"y": model.NewInt(20),
		},
	})
	r.AddAction(model.CBindAction{Variable: "<p>"})
	r.AddAction(model.ModifyAction{
		TargetElementVar: "p",
		Attributes: map[string]model.Value{
			"x": model.NewInt(99),
		},
	})
	r.AddAction(model.RemoveAction{
		TargetElementVar: "s",
	})
	r.AddAction(model.HaltAction{})

	eng.AddRule(r)

	// Assert start WME
	startWME := eng.Make("start", nil)

	// Step engine
	fired, err := eng.Step()
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if !fired {
		t.Fatalf("expected rule to fire")
	}

	// Verify start was removed
	if _, ok := eng.WorkingMemory().Get(startWME.Timetag); ok {
		t.Errorf("start WME should have been removed")
	}

	// Verify point was created and modified
	points := eng.WorkingMemory().FindByClass("point")
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	xVal, _ := points[0].Get("x")
	yVal, _ := points[0].Get("y")
	if xVal.Raw().(int64) != 99 || yVal.Raw().(int64) != 20 {
		t.Errorf("expected point (x=99, y=20), got x=%v, y=%v", xVal, yVal)
	}

	// Engine should be halted
	if !eng.IsHalted() {
		t.Errorf("expected engine to be halted")
	}
}

func TestCompiledAction_ComputeAndDirectVariables(t *testing.T) {
	eng := New()
	eng.SetWatchLevel(0)

	r := model.NewRule("test-compute")
	ce := model.NewPositiveCE("counter").AddEqualTest("val", model.NewVariable("<v>"))
	r.AddCondition(ce)
	r.AddAction(model.MakeAction{
		Class: "next",
		Attributes: map[string]model.Value{
			"val": model.NewCompute(
				[]model.Value{model.NewVariable("<v>"), model.NewInt(5)},
				[]model.ComputeOp{model.ComputeOpAdd},
			),
		},
	})
	eng.AddRule(r)

	eng.Make("counter", map[string]model.Value{"val": model.NewInt(10)})
	fired, err := eng.Step()
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if !fired {
		t.Fatalf("expected rule to fire")
	}

	nexts := eng.WorkingMemory().FindByClass("next")
	if len(nexts) != 1 {
		t.Fatalf("expected 1 next WME, got %d", len(nexts))
	}
	val, _ := nexts[0].Get("val")
	if val.Raw().(int64) != 15 {
		t.Errorf("expected next val=15, got %v", val)
	}
}

func TestCompiledAction_WriteFormatting(t *testing.T) {
	eng := New()
	var buf bytes.Buffer
	eng.SetOutputWriter(&buf)
	eng.SetWatchLevel(0)

	r := model.NewRule("test-write")
	r.AddCondition(model.NewPositiveCE("msg").AddEqualTest("text", model.NewVariable("<t>")))
	r.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			{Type: model.WriteArgValue, Value: model.NewSymbol("Header:")},
			{Type: model.WriteArgTabTo, Value: model.NewInt(12)},
			{Type: model.WriteArgValue, Value: model.NewVariable("<t>")},
			{Type: model.WriteArgCRLF},
		},
	})
	eng.AddRule(r)

	eng.Make("msg", map[string]model.Value{"text": model.NewString("Hello")})
	fired, err := eng.Step()
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if !fired {
		t.Fatalf("expected rule to fire")
	}

	expected := "Header:    Hello\n"
	if buf.String() != expected {
		t.Errorf("expected output %q, got %q", expected, buf.String())
	}
}

func TestCompiledAction_ActionContextPooling(t *testing.T) {
	eng := New()
	tok := rete.NewToken(nil, nil, []rete.Binding{{Name: "x", Value: model.NewInt(100)}})
	act := &conflict.Activation{
		Token: tok,
	}

	ctx1 := eng.acquireActionContext(act)
	if ctx1.GetVariable("x", model.Value{}).Raw().(int64) != 100 {
		t.Errorf("expected x=100")
	}
	ctx1.SetVariable("y", model.NewSymbol("hello"))
	if ctx1.GetVariable("y", model.Value{}).String() != "hello" {
		t.Errorf("expected y=hello")
	}

	eng.releaseActionContext(ctx1)

	// Re-acquire from pool
	ctx2 := eng.acquireActionContext(act)
	// Local bindings should have been cleared on release
	if ctx2.GetVariable("y", model.NewSymbol("nil")).String() != "nil" {
		t.Errorf("expected y to be cleared after release")
	}
	eng.releaseActionContext(ctx2)
}

func TestCompiledAction_TargetResolverErrors(t *testing.T) {
	// 1. Unbound element variable
	resVar := compileTargetResolver("unbound", 0)
	ctx := &ActionContext{}
	if _, err := resVar(ctx); err == nil {
		t.Errorf("expected error for unbound element variable")
	}

	// 2. Out of bounds index
	resIdx := compileTargetResolver("", 5)
	if _, err := resIdx(ctx); err == nil {
		t.Errorf("expected error for out of bounds condition element index")
	}
}
