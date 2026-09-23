package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestEngine_MakeBatch(t *testing.T) {
	eng := New()
	eng.SetWatchLevel(0)
	eng.SetAlphaWorkers(4)

	eng.DeclareClass("sensor", []string{"id", "reading"})

	r := model.NewRule("high-reading")
	r.AddCondition(model.NewPositiveCE("sensor").AddTest("reading", model.OpGreater, model.NewInt(50)))
	r.AddAction(model.MakeAction{
		Class: "alert",
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("triggered"),
		},
	})
	eng.AddRule(r)

	// Batch assert 100 sensors (50 > 50, 50 <= 50)
	reqs := make([]MakeRequest, 100)
	for i := 0; i < 100; i++ {
		reqs[i] = MakeRequest{
			Class: "sensor",
			Attributes: map[string]model.Value{
				"id":      model.NewInt(int64(i)),
				"reading": model.NewInt(int64(i + 1)),
			},
		}
	}

	wmes := eng.MakeBatch(reqs)
	if len(wmes) != 100 {
		t.Fatalf("expected 100 WMEs, got %d", len(wmes))
	}

	// Conflict set should contain exactly 50 activations (reading 51..100)
	if count := eng.ConflictSet().Count(); count != 50 {
		t.Errorf("expected 50 activations in conflict set, got %d", count)
	}
}

func TestPartitionedEngine_CrossPartitionRouting(t *testing.T) {
	// Router:
	// "order" created in "web" partition routes to "billing" partition.
	// "invoice" created in "billing" routes to "shipping" partition.
	router := func(origin string, class string, attrs map[string]model.Value) []string {
		switch class {
		case "order":
			return []string{"billing"}
		case "invoice":
			return []string{"shipping"}
		default:
			return nil
		}
	}

	pe := NewPartitionedEngine(router)
	pe.DeclareClass("order", []string{"id", "amount"})
	pe.DeclareClass("invoice", []string{"order-id", "paid"})
	pe.DeclareClass("package", []string{"order-id", "shipped"})

	// Partition 1: web (creates orders)
	pWeb, err := pe.AddPartition("web")
	if err != nil {
		t.Fatalf("failed to add web partition: %v", err)
	}

	// Partition 2: billing (processes orders, creates invoices)
	pBilling, err := pe.AddPartition("billing")
	if err != nil {
		t.Fatalf("failed to add billing partition: %v", err)
	}
	rBilling := model.NewRule("process-order")
	rBilling.AddCondition(model.NewPositiveCE("order").WithElementVariable("o").AddEqualTest("amount", model.NewVariable("<amt>")))
	rBilling.AddAction(model.MakeAction{
		Class: "invoice",
		Attributes: map[string]model.Value{
			"order-id": model.NewInt(101),
			"paid":     model.NewSymbol("yes"),
		},
	})
	if err := pe.AddRuleToPartition("billing", rBilling); err != nil {
		t.Fatalf("failed to add rule to billing: %v", err)
	}

	// Partition 3: shipping (receives invoices, creates packages)
	pShipping, err := pe.AddPartition("shipping")
	if err != nil {
		t.Fatalf("failed to add shipping partition: %v", err)
	}
	rShipping := model.NewRule("ship-package")
	rShipping.AddCondition(model.NewPositiveCE("invoice").AddEqualTest("paid", model.NewSymbol("yes")))
	rShipping.AddAction(model.MakeAction{
		Class: "package",
		Attributes: map[string]model.Value{
			"order-id": model.NewInt(101),
			"shipped":  model.NewSymbol("yes"),
		},
	})
	if err := pe.AddRuleToPartition("shipping", rShipping); err != nil {
		t.Fatalf("failed to add rule to shipping: %v", err)
	}

	// Assert order in "web"
	_, err = pe.MakeInPartition("web", "order", map[string]model.Value{
		"id":     model.NewInt(101),
		"amount": model.NewInt(500),
	})
	if err != nil {
		t.Fatalf("MakeInPartition failed: %v", err)
	}

	// Run all 3 partitions in parallel until global quiescence
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cycles, err := pe.RunParallel(ctx, 100)
	if err != nil {
		t.Fatalf("RunParallel failed: %v", err)
	}

	// Billing should have fired 1 rule, Shipping 1 rule, Web 0 rules
	if cycles["billing"] != 1 {
		t.Errorf("expected 1 cycle in billing, got %d", cycles["billing"])
	}
	if cycles["shipping"] != 1 {
		t.Errorf("expected 1 cycle in shipping, got %d", cycles["shipping"])
	}
	if cycles["web"] != 0 {
		t.Errorf("expected 0 cycles in web, got %d", cycles["web"])
	}

	// Verify final package in shipping partition's working memory
	packages := pShipping.WorkingMemory().FindByClass("package")
	if len(packages) != 1 {
		t.Fatalf("expected 1 package in shipping WM, got %d", len(packages))
	}
	shippedVal, _ := packages[0].Get("shipped")
	if shippedVal.String() != "yes" {
		t.Errorf("expected shipped=yes, got %s", shippedVal.String())
	}

	_ = pWeb
	_ = pBilling
}

func TestPartitionedEngine_BroadcastRouting(t *testing.T) {
	// Router broadcasts "broadcast-alert" to all other partitions
	router := func(origin string, class string, attrs map[string]model.Value) []string {
		if class == "broadcast-alert" {
			return []string{"*"}
		}
		return nil
	}

	pe := NewPartitionedEngine(router)
	pe.DeclareClass("broadcast-alert", []string{"msg"})
	pe.DeclareClass("local-log", []string{"text"})

	rLog := model.NewRule("log-alert")
	rLog.AddCondition(model.NewPositiveCE("broadcast-alert").AddEqualTest("msg", model.NewVariable("<m>")))
	rLog.AddAction(model.MakeAction{
		Class: "local-log",
		Attributes: map[string]model.Value{
			"text": model.NewVariable("<m>"),
		},
	})

	// Add 4 partitions: "node-0", "node-1", "node-2", "node-3"
	for i := 0; i < 4; i++ {
		pName := fmt.Sprintf("node-%d", i)
		if _, err := pe.AddPartition(pName); err != nil {
			t.Fatalf("AddPartition failed: %v", err)
		}
		if i > 0 {
			if err := pe.AddRuleToPartition(pName, rLog); err != nil {
				t.Fatalf("AddRuleToPartition failed: %v", err)
			}
		}
	}

	// Send broadcast from node-0
	_, err := pe.MakeInPartition("node-0", "broadcast-alert", map[string]model.Value{
		"msg": model.NewString("SystemMaintenance"),
	})
	if err != nil {
		t.Fatalf("MakeInPartition failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cycles, err := pe.RunParallel(ctx, 10)
	if err != nil {
		t.Fatalf("RunParallel failed: %v", err)
	}

	// node-0 fired 0 (originator), nodes 1, 2, 3 each fired 1
	if cycles["node-0"] != 0 {
		t.Errorf("expected node-0 cycles=0, got %d", cycles["node-0"])
	}
	for i := 1; i < 4; i++ {
		pName := fmt.Sprintf("node-%d", i)
		if cycles[pName] != 1 {
			t.Errorf("expected %s cycles=1, got %d", pName, cycles[pName])
		}
		p, _ := pe.GetPartition(pName)
		logs := p.WorkingMemory().FindByClass("local-log")
		if len(logs) != 1 {
			t.Errorf("expected 1 local-log in %s, got %d", pName, len(logs))
		}
	}
}

func TestPartitionedEngine_ConcurrentStepAll(t *testing.T) {
	pe := NewPartitionedEngine(nil)
	pe.DeclareClass("task", []string{"id"})
	pe.DeclareClass("done", []string{"id"})

	r := model.NewRule("finish-task")
	r.AddCondition(model.NewPositiveCE("task").WithElementVariable("<t>").AddEqualTest("id", model.NewVariable("<id>")))
	r.AddAction(model.MakeAction{
		Class: "done",
		Attributes: map[string]model.Value{
			"id": model.NewVariable("<id>"),
		},
	})
	r.AddAction(model.RemoveAction{TargetElementVar: "t"})

	const numWorkers = 5
	for i := 0; i < numWorkers; i++ {
		pName := fmt.Sprintf("w-%d", i)
		p, _ := pe.AddPartition(pName)
		p.Engine.AddRule(r)
		p.Engine.Make("task", map[string]model.Value{"id": model.NewInt(int64(i))})
	}

	// StepAll: all 5 workers should fire concurrently
	firedMap, err := pe.StepAll()
	if err != nil {
		t.Fatalf("StepAll failed: %v", err)
	}
	if len(firedMap) != numWorkers {
		t.Fatalf("expected %d results, got %d", numWorkers, len(firedMap))
	}
	for k, fired := range firedMap {
		if !fired {
			t.Errorf("expected partition %s to fire", k)
		}
	}

	// Second StepAll: conflict sets empty, none should fire
	firedMap2, err := pe.StepAll()
	if err != nil {
		t.Fatalf("second StepAll failed: %v", err)
	}
	for k, fired := range firedMap2 {
		if fired {
			t.Errorf("expected partition %s NOT to fire on second step", k)
		}
	}
}
