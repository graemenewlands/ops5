package engine

import (
	"bytes"
	"strings"
	"testing"

	"ops5/pkg/model"
)

func TestEngineGoalProgression(t *testing.T) {
	eng := New()
	var logBuf bytes.Buffer
	eng.SetOutputWriter(&logBuf)

	// Rule 1: Step 1 -> Step 2
	rule1 := model.NewRule("step-1-to-2")
	ce1 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(1))
	rule1.AddCondition(ce1)
	rule1.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"step": model.NewInt(2),
		},
	})
	rule1.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteValue(model.NewSymbol("TRANSITIONED")), model.WriteValue(model.NewSymbol("TO")), model.WriteValue(model.NewInt(2))},
	})
	eng.AddRule(rule1)

	// Rule 2: Step 2 -> Step 3
	rule2 := model.NewRule("step-2-to-3")
	ce2 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(2))
	rule2.AddCondition(ce2)
	rule2.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"step": model.NewInt(3),
		},
	})
	eng.AddRule(rule2)

	// Rule 3: Step 3 -> Finish & Halt
	rule3 := model.NewRule("finish")
	ce3 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(3))
	rule3.AddCondition(ce3)
	rule3.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("completed"),
		},
	})
	rule3.AddAction(model.HaltAction{})
	eng.AddRule(rule3)

	// Initial WME
	eng.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("active"),
		"step":   model.NewInt(1),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 3 {
		t.Fatalf("expected exactly 3 cycles, ran %d", cycles)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to be halted")
	}

	// Working memory should now contain completed goal
	goals := eng.WorkingMemory().FindByClass("goal")
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal in WM, got %d", len(goals))
	}
	status, _ := goals[0].Get("status")
	if !status.Equal(model.NewSymbol("completed")) {
		t.Fatalf("expected status 'completed', got %v", status)
	}

	if !strings.Contains(logBuf.String(), "TRANSITIONED TO 2") {
		t.Fatalf("expected write action output in log buffer, got %q", logBuf.String())
	}
}

func TestEngineNegatedConditionControl(t *testing.T) {
	eng := New()

	// Rule: Process pending task if not blocked
	rule := model.NewRule("process-unblocked-task")
	ce1 := model.NewPositiveCE("task").
		WithElementVariable("t").
		AddEqualTest("status", model.NewSymbol("pending")).
		AddEqualTest("id", model.NewVariable("<id>"))
	ce2 := model.NewNegativeCE("lock").
		AddEqualTest("task-id", model.NewVariable("<id>"))

	rule.AddCondition(ce1).AddCondition(ce2)
	rule.AddAction(model.ModifyAction{
		TargetElementVar: "t",
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("running"),
		},
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	// 1. Assert task and lock -> Should not fire!
	eng.Make("task", map[string]model.Value{
		"id":     model.NewInt(99),
		"status": model.NewSymbol("pending"),
	})
	lock := eng.Make("lock", map[string]model.Value{
		"task-id": model.NewInt(99),
	})

	cycles, _ := eng.Run(10)
	if cycles != 0 {
		t.Fatalf("expected 0 cycles when locked, got %d", cycles)
	}

	// 2. Remove lock -> Negative condition becomes satisfied -> Rule fires!
	eng.Remove(lock.Timetag)

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle after unlocking, got %d", cycles)
	}

	tasks := eng.WorkingMemory().FindByClass("task")
	status, _ := tasks[0].Get("status")
	if !status.Equal(model.NewSymbol("running")) {
		t.Fatalf("expected task status to be 'running', got %v", status)
	}
}

func TestEngineWriteFormattingGrid(t *testing.T) {
	eng := New()
	var buf bytes.Buffer
	eng.SetOutputWriter(&buf)

	// Rule to print a grid table
	r1 := model.NewRule("print-row")
	ce := model.NewPositiveCE("city").
		WithElementVariable("c").
		AddEqualTest("id", model.NewVariable("<id>")).
		AddEqualTest("name", model.NewVariable("<name>")).
		AddEqualTest("pop", model.NewVariable("<pop>"))
	r1.AddCondition(ce)
	r1.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteTabTo(model.NewInt(5)),
			model.WriteValue(model.NewVariable("<id>")),
			model.WriteTabTo(model.NewInt(15)),
			model.WriteValue(model.NewVariable("<name>")),
			model.WriteTabTo(model.NewInt(28)),
			model.WriteValue(model.NewVariable("<pop>")),
			model.WriteCRLF(),
		},
	})
	r1.AddAction(model.RemoveAction{TargetElementVar: "c"})
	eng.AddRule(r1)

	eng.Make("city", map[string]model.Value{
		"id":   model.NewInt(101),
		"name": model.NewString("Boston"),
		"pop":  model.NewInt(675000),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	expected := "    101       Boston       675000\n"
	got := buf.String()
	if got != expected {
		t.Fatalf("grid formatting mismatch:\ngot:\n%q\nwant:\n%q", got, expected)
	}
}

func TestEngineBindAndCompute(t *testing.T) {
	eng := New()
	var buf bytes.Buffer
	eng.SetOutputWriter(&buf)

	r1 := model.NewRule("calculate-total")
	ce := model.NewPositiveCE("order").
		WithElementVariable("o").
		AddEqualTest("price", model.NewVariable("<p>")).
		AddEqualTest("tax-rate", model.NewVariable("<r>"))
	r1.AddCondition(ce)

	// (bind <tax> (compute <p> * <r>))
	r1.AddAction(model.BindAction{
		Variable: "tax",
		Value: model.NewCompute(
			[]model.Value{model.NewVariable("<p>"), model.NewVariable("<r>")},
			[]model.ComputeOp{model.ComputeOpMul},
		),
	})

	// (bind <total> (compute <p> + <tax>))
	r1.AddAction(model.BindAction{
		Variable: "total",
		Value: model.NewCompute(
			[]model.Value{model.NewVariable("<p>"), model.NewVariable("<tax>")},
			[]model.ComputeOp{model.ComputeOpAdd},
		),
	})

	// (make invoice ^price <p> ^tax <tax> ^total <total>)
	r1.AddAction(model.MakeAction{
		Class: "invoice",
		Attributes: map[string]model.Value{
			"price": model.NewVariable("<p>"),
			"tax":   model.NewVariable("<tax>"),
			"total": model.NewVariable("<total>"),
		},
	})

	// (write (crlf) |Total:| <total> (crlf))
	r1.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteCRLF(),
			model.WriteValue(model.NewSymbol("Total:")),
			model.WriteValue(model.NewVariable("<total>")),
			model.WriteCRLF(),
		},
	})

	// (remove <o>)
	r1.AddAction(model.RemoveAction{TargetElementVar: "o"})
	eng.AddRule(r1)

	eng.Make("order", map[string]model.Value{
		"price":    model.NewFloat(100.0),
		"tax-rate": model.NewFloat(0.05),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	expectedOut := "\nTotal: 105\n"
	if buf.String() != expectedOut {
		t.Fatalf("expected output %q, got %q", expectedOut, buf.String())
	}

	invoices := eng.WorkingMemory().FindByClass("invoice")
	if len(invoices) != 1 {
		t.Fatalf("expected 1 invoice WME, got %d", len(invoices))
	}
	taxVal, _ := invoices[0].Get("tax")
	if !taxVal.Equal(model.NewFloat(5.0)) {
		t.Fatalf("expected invoice tax 5.0, got %v", taxVal)
	}
	totalVal, _ := invoices[0].Get("total")
	if !totalVal.Equal(model.NewFloat(105.0)) {
		t.Fatalf("expected invoice total 105.0, got %v", totalVal)
	}
}

