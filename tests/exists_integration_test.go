package tests

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
)

// TestExistsIntegrationSingleRule verifies that an existential semi-join
// matches when at least one matching WME exists and prevents token multiplication.
func TestExistsIntegrationSingleRule(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p flag-batch-with-defects
	(batch ^id <bid> ^department <dept>)
	(exists (sample ^batch-id <bid> ^status defective))
-->
	(make inspection-alert ^batch-id <bid> ^department <dept>)
	(write "Defect alert for batch" <bid> "in department" <dept> (crlf))
)

(make batch ^id 101 ^department QA)
(make sample ^batch-id 101 ^status ok)
(make sample ^batch-id 101 ^status ok)
(make sample ^batch-id 101 ^status defective)
(make sample ^batch-id 101 ^status defective)
(make sample ^batch-id 101 ^status defective)
(make sample ^batch-id 101 ^status ok)
`
	runScript(t, eng, script)

	// Even though there are 3 defective samples, exists must produce EXACTLY 1 firing
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected exactly 1 firing (no token multiplication), got %d", fired)
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Defect alert for batch 101 in department QA") {
		t.Errorf("unexpected output: %s", output)
	}

	alerts := eng.WorkingMemory().FindByClass("inspection-alert")
	if len(alerts) != 1 {
		t.Fatalf("expected 1 inspection-alert WME, got %d", len(alerts))
	}
	bid, _ := alerts[0].Get("batch-id")
	if !bid.Equal(model.NewInt(101)) {
		t.Errorf("expected batch-id 101, got %v", bid)
	}
}

// TestExistsIntegrationRandomData verifies semi-join semantics across a random population of batches
// where each batch has a random number of passing or defective items.
func TestExistsIntegrationRandomData(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p alert-defective-batch
	(batch ^id <bid>)
	(exists (sample ^batch-id <bid> ^status defective))
-->
	(make alert ^batch <bid>)
)
`
	runScript(t, eng, ruleSrc)

	rng := rand.New(rand.NewSource(9999))
	numBatches := 20
	expectedDefectiveBatches := make(map[int64]bool)

	for b := 1; b <= numBatches; b++ {
		bid := int64(b)
		eng.Make("batch", map[string]model.Value{
			"id": model.NewInt(bid),
		})

		// Random number of samples between 1 and 8
		numSamples := rng.Intn(8) + 1
		hasDefect := false
		for s := 0; s < numSamples; s++ {
			status := "ok"
			// 30% chance of defect
			if rng.Float64() < 0.30 {
				status = "defective"
				hasDefect = true
			}
			eng.Make("sample", map[string]model.Value{
				"batch-id": model.NewInt(bid),
				"status":   model.NewSymbol(status),
			})
		}
		if hasDefect {
			expectedDefectiveBatches[bid] = true
		}
	}

	t.Logf("Total batches: %d, Expected defective batches: %d", numBatches, len(expectedDefectiveBatches))

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != len(expectedDefectiveBatches) {
		t.Fatalf("expected %d firings (one per defective batch), got %d", len(expectedDefectiveBatches), fired)
	}

	alerts := eng.WorkingMemory().FindByClass("alert")
	if len(alerts) != len(expectedDefectiveBatches) {
		t.Fatalf("expected %d alert WMEs, got %d", len(expectedDefectiveBatches), len(alerts))
	}

	for _, alert := range alerts {
		bVal, _ := alert.Get("batch")
		bId := bVal.Raw().(int64)
		if !expectedDefectiveBatches[bId] {
			t.Errorf("batch %d was not expected to have defects", bId)
		}
	}
}

// TestExistsIntegrationDynamicTransitions verifies the full lifecycle transitions:
// 0 -> 1 (activation added) -> N (suppressed) -> 1 (still active) -> 0 (activation removed).
func TestExistsIntegrationDynamicTransitions(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p monitor-warehouse
	(warehouse ^id <wid> ^active yes)
	(exists (item ^warehouse-id <wid> ^status in-stock))
-->
	(write "WAREHOUSE-READY:" <wid> (crlf))
)

(make warehouse ^id 1 ^active yes)
`
	runScript(t, eng, ruleSrc)

	// Step 1: 0 items in stock -> 0 activations
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations with 0 matching items, got %d", eng.ConflictSet().Count())
	}

	// Step 2: Add 1st matching item (0 -> 1 transition) -> 1 activation
	item1 := eng.Make("item", map[string]model.Value{
		"warehouse-id": model.NewInt(1),
		"status":       model.NewSymbol("in-stock"),
	})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation after 1st item added, got %d", eng.ConflictSet().Count())
	}

	// Step 3: Add 2nd and 3rd matching items (1 -> 2, 3 transitions) -> still exactly 1 activation!
	item2 := eng.Make("item", map[string]model.Value{
		"warehouse-id": model.NewInt(1),
		"status":       model.NewSymbol("in-stock"),
	})
	item3 := eng.Make("item", map[string]model.Value{
		"warehouse-id": model.NewInt(1),
		"status":       model.NewSymbol("in-stock"),
	})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation with multiple items (no multiplication), got %d", eng.ConflictSet().Count())
	}

	// Step 4: Retract item 3 and item 2 (3 -> 2, 2 -> 1) -> activation must remain active!
	eng.Remove(item3.Timetag)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to stay when items remain, got %d", eng.ConflictSet().Count())
	}
	eng.Remove(item2.Timetag)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to stay when 1 item remains, got %d", eng.ConflictSet().Count())
	}

	// Step 5: Retract final item 1 (1 -> 0 transition) -> activation must be retracted from conflict set!
	eng.Remove(item1.Timetag)
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after all matching items removed, got %d", eng.ConflictSet().Count())
	}

	// Step 6: Add an item again (0 -> 1 transition) and fire
	eng.Make("item", map[string]model.Value{
		"warehouse-id": model.NewInt(1),
		"status":       model.NewSymbol("in-stock"),
	})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to return, got %d", eng.ConflictSet().Count())
	}

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule firing, got %d", fired)
	}
	if !strings.Contains(out.String(), "WAREHOUSE-READY: 1") {
		t.Errorf("unexpected output: %s", out.String())
	}
}
