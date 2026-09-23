package tests

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
)

func TestEngineJoinOptimizer_ExecutionAndActionRemapping(t *testing.T) {
	eng := engine.New()

	// Rule with intentional bad join order:
	// CE 1: (goal ^status run)
	// CE 2: (customer ^cid <cid> ^credit <cr>)   ; disconnected from goal
	// CE 3: (order ^id <oid> ^cid <cid>)         ; connects customer to order
	// CE 4: (line-item ^order-id <oid> ^qty <q>) ; connects order to line-item
	//
	// RHS:
	// modifies CE 3 (order) to ^processed yes
	// removes CE 4 (line-item)
	// makes result referencing (substr 3 id id)
	script := `
(literalize goal status)
(literalize customer cid credit)
(literalize order id cid processed)
(literalize line-item order-id qty)
(literalize result summary)

(p process-order
	(goal ^status run)
	(customer ^cid <cid> ^credit > 500)
	(order ^id <oid> ^cid <cid> ^processed no)
	(line-item ^order-id <oid> ^qty <q>)
-->
	(modify 3 ^processed yes)
	(remove 4)
	(make result ^summary (substr 3 id id))
)
`
	runDiagnosticScript(t, eng, script)

	// Verify that the rule conditions were reordered by the optimizer:
	rep, ok := eng.RuleMatches("process-order")
	if !ok {
		t.Fatalf("expected RuleMatches for process-order")
	}

	// Anchored condition 1: goal
	if rep.Conditions[0].Index != 1 {
		t.Errorf("expected CE 1 at index 1")
	}

	// Populate Working Memory:
	// 1 goal
	eng.Make("goal", map[string]model.Value{"status": model.NewSymbol("run")})

	// 3 customers (only 1 matches credit > 500: cid 99)
	eng.Make("customer", map[string]model.Value{"cid": model.NewInt(10), "credit": model.NewInt(100)})
	eng.Make("customer", map[string]model.Value{"cid": model.NewInt(20), "credit": model.NewInt(200)})
	eng.Make("customer", map[string]model.Value{"cid": model.NewInt(99), "credit": model.NewInt(750)})

	// 2 orders (only 1 matches cid 99)
	eng.Make("order", map[string]model.Value{"id": model.NewInt(1001), "cid": model.NewInt(10), "processed": model.NewSymbol("no")})
	eng.Make("order", map[string]model.Value{"id": model.NewInt(9999), "cid": model.NewInt(99), "processed": model.NewSymbol("no")})

	// 1 line-item
	li := eng.Make("line-item", map[string]model.Value{"order-id": model.NewInt(9999), "qty": model.NewInt(42)})

	// Step engine until quiescence
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("engine Run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule firing, got %d", fired)
	}

	// Verify working memory state:
	// 1. Order 9999 should be modified to ^processed yes
	allOrders := eng.WorkingMemory().FindByClass("order")
	var processedOrder *model.WME
	for _, o := range allOrders {
		if idVal, ok := o.Get("id"); ok && idVal.Raw().(int64) == 9999 {
			processedOrder = o
			break
		}
	}
	if processedOrder == nil {
		t.Fatalf("expected order 9999 in working memory")
	}
	procVal, _ := processedOrder.Get("processed")
	if procVal.String() != "yes" {
		t.Errorf("expected order processed to be 'yes', got %s", procVal.String())
	}

	// 2. Line-item should be removed
	allLIs := eng.WorkingMemory().FindByClass("line-item")
	for _, l := range allLIs {
		if l.Timetag == li.Timetag {
			t.Errorf("expected line-item to be removed from working memory")
		}
	}

	// 3. Result WME created
	results := eng.WorkingMemory().FindByClass("result")
	if len(results) != 1 {
		t.Fatalf("expected 1 result WME, got %d", len(results))
	}
	sumVal, _ := results[0].Get("summary")
	if sumVal.String() != "9999" {
		t.Errorf("expected result summary to be '9999', got %s", sumVal.String())
	}
}

func TestEngineJoinOptimizer_NoReorderExecution(t *testing.T) {
	eng := engine.New()

	script := `
(literalize goal status)
(literalize customer cid)
(literalize order id cid)

(p no-opt [no-reorder]
	(goal ^status run)
	(customer ^cid <cid>)
	(order ^id <oid> ^cid <cid>)
-->
	(modify 2 ^cid 999)
)
`
	runDiagnosticScript(t, eng, script)

	rule := eng.Rule("no-opt")
	if rule == nil || !rule.NoReorder {
		t.Fatalf("expected rule to have NoReorder = true")
	}

	// Working memory
	eng.Make("goal", map[string]model.Value{"status": model.NewSymbol("run")})
	eng.Make("customer", map[string]model.Value{"cid": model.NewInt(10)})
	eng.Make("order", map[string]model.Value{"id": model.NewInt(1), "cid": model.NewInt(10)})

	fired, err := eng.Run(-1)
	if err != nil || fired != 1 {
		t.Fatalf("expected 1 firing, got %d (err: %v)", fired, err)
	}

	// Customer should have cid modified to 999
	custs := eng.WorkingMemory().FindByClass("customer")
	if len(custs) != 1 {
		t.Fatalf("expected 1 customer, got %d", len(custs))
	}
	cidVal, _ := custs[0].Get("cid")
	if cidVal.Raw().(int64) != 999 {
		t.Errorf("expected cid 999, got %v", cidVal)
	}
}
