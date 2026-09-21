package tests

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"ops5/pkg/engine"
	"ops5/pkg/model"
)

// TestEvalIntegrationSingleRule verifies that an EvalNode evaluating an arithmetic compute
// expression filters activations accurately in an end-to-end OPS5 script.
func TestEvalIntegrationSingleRule(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// An applicant is approved if 3x debt is less than or equal to annual income
	script := `
(p approve-loan
	(applicant ^id <aid> ^name <name> ^income <inc> ^debt <d>)
	(test (compute <d> * 3 <= <inc>))
-->
	(make approved-loan ^applicant-id <aid> ^name <name>)
	(write "Loan approved for:" <name> (crlf))
)

; Applicant 1: income 90000, debt 20000 -> 20000 * 3 = 60000 <= 90000 -> Approved!
(make applicant ^id 1 ^name Alice ^income 90000 ^debt 20000)

; Applicant 2: income 50000, debt 25000 -> 25000 * 3 = 75000 > 50000 -> Denied!
(make applicant ^id 2 ^name Bob ^income 50000 ^debt 25000)

; Applicant 3: income 120000, debt 40000 -> 40000 * 3 = 120000 <= 120000 -> Approved!
(make applicant ^id 3 ^name Charlie ^income 120000 ^debt 40000)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 2 {
		t.Fatalf("expected exactly 2 rule firings (Alice and Charlie), got %d", fired)
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Loan approved for: Alice") {
		t.Errorf("expected Alice to be approved")
	}
	if !strings.Contains(output, "Loan approved for: Charlie") {
		t.Errorf("expected Charlie to be approved")
	}
	if strings.Contains(output, "Bob") {
		t.Errorf("Bob should NOT be approved")
	}

	approved := eng.WorkingMemory().FindByClass("approved-loan")
	if len(approved) != 2 {
		t.Fatalf("expected 2 approved-loan WMEs, got %d", len(approved))
	}
}

// TestEvalIntegrationRandomData generates random transactions and verifies that
// an EvalNode with multiple predicate bounds correctly selects only transactions within range.
func TestEvalIntegrationRandomData(t *testing.T) {
	eng := engine.New()

	// Select items where total cost (qty * price) is between 100 and 500
	ruleSrc := `
(p select-mid-range-order
	(order ^id <oid> ^price <p> ^qty <q>)
	(test (compute <p> * <q> >= 100))
	(test (compute <p> * <q> <= 500))
-->
	(make selected-order ^order-id <oid>)
)
`
	runScript(t, eng, ruleSrc)

	rng := rand.New(rand.NewSource(12345))
	numOrders := 30

	type orderData struct {
		id    int64
		price int64
		qty   int64
		total int64
	}

	var expectedSelected []int64

	for i := 1; i <= numOrders; i++ {
		oid := int64(i)
		price := int64(rng.Intn(100) + 1) // 1 to 100
		qty := int64(rng.Intn(10) + 1)     // 1 to 10
		total := price * qty

		eng.Make("order", map[string]model.Value{
			"id":    model.NewInt(oid),
			"price": model.NewInt(price),
			"qty":   model.NewInt(qty),
		})

		if total >= 100 && total <= 500 {
			expectedSelected = append(expectedSelected, oid)
		}
	}

	t.Logf("Total orders: %d, Expected qualifying orders: %d", numOrders, len(expectedSelected))

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != len(expectedSelected) {
		t.Fatalf("expected %d firings, got %d", len(expectedSelected), fired)
	}

	selectedWMEs := eng.WorkingMemory().FindByClass("selected-order")
	if len(selectedWMEs) != len(expectedSelected) {
		t.Fatalf("expected %d selected WMEs, got %d", len(expectedSelected), len(selectedWMEs))
	}

	expectedMap := make(map[int64]bool)
	for _, oid := range expectedSelected {
		expectedMap[oid] = true
	}

	for _, w := range selectedWMEs {
		oidVal, _ := w.Get("order-id")
		oid := oidVal.Raw().(int64)
		if !expectedMap[oid] {
			t.Errorf("order %d was not expected to be selected", oid)
		}
	}
}

// TestEvalIntegrationDynamicModifications verifies that modifying a WME dynamically
// evaluates the EvalNode condition and adds or removes activations accordingly.
func TestEvalIntegrationDynamicModifications(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p monitor-thermostat
	(climate ^zone <z> ^temp <t> ^target <tgt>)
	(test (<t> > <tgt>))
-->
	(write "COOLING-NEEDED: zone" <z> "temp" <t> "target" <tgt> (crlf))
)
`
	runScript(t, eng, ruleSrc)

	// Step 1: Initial state temp=68, target=72 -> 68 is NOT > 72 -> Eval fails -> 0 activations
	zone := eng.Make("climate", map[string]model.Value{
		"zone":   model.NewSymbol("living-room"),
		"temp":   model.NewInt(68),
		"target": model.NewInt(72),
	})

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations when temp <= target, got %d", eng.ConflictSet().Count())
	}

	// Step 2: Modify temp to 75 -> 75 > 72 -> Eval passes -> 1 activation
	zone, _ = eng.Modify(zone.Timetag, map[string]model.Value{
		"temp": model.NewInt(75),
	})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation when temp > target, got %d", eng.ConflictSet().Count())
	}

	// Step 3: Modify temp back down to 70 -> 70 is NOT > 72 -> Eval fails -> activation retracted!
	zone, _ = eng.Modify(zone.Timetag, map[string]model.Value{
		"temp": model.NewInt(70),
	})

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after cooling down, got %d", eng.ConflictSet().Count())
	}

	// Step 4: Spike temp to 80 -> 1 activation -> Fire engine!
	zone, _ = eng.Modify(zone.Timetag, map[string]model.Value{
		"temp": model.NewInt(80),
	})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation after temp spike, got %d", eng.ConflictSet().Count())
	}

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing, got %d", fired)
	}
	if !strings.Contains(out.String(), "COOLING-NEEDED: zone living-room temp 80 target 72") {
		t.Errorf("unexpected output: %s", out.String())
	}
}
