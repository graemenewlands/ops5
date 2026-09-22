package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
)

func TestEngineStructuralBetaSharingExecution(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Two rules sharing conditions 1 and 2:
	// Rule 1: (A ^id <x>) (B ^a-id <x>) (C ^val 10)
	// Rule 2: (A ^id <x>) (B ^a-id <x>) (D ^val 20)
	script := `
(p r1
	(A ^id <x>)
	(B ^a-id <x>)
	(C ^val 10)
-->
	(write "Fired r1 for" <x> (crlf))
)

(p r2
	(A ^id <x>)
	(B ^a-id <x>)
	(D ^val 20)
-->
	(write "Fired r2 for" <x> (crlf))
)
`
	runDiagnosticScript(t, eng, script)

	// Total beta nodes should be 4 (A, B, C, D) rather than 6 (A1, B1, C, A2, B2, D)
	if eng.BetaNodeCount() != 4 {
		t.Fatalf("expected 4 shared beta nodes, got %d", eng.BetaNodeCount())
	}

	// Verify that the RuleNodeInfo for both rules share the exact same BetaMemory instances
	r1Info, ok1 := eng.RuleMatches("r1")
	r2Info, ok2 := eng.RuleMatches("r2")
	if !ok1 || !ok2 {
		t.Fatalf("expected match reports for both rules")
	}
	if len(r1Info.PartialMatches) != 1 || len(r2Info.PartialMatches) != 1 {
		t.Fatalf("expected 1 partial match span for each rule")
	}

	// Assert A and B
	eng.Make("A", map[string]model.Value{"id": model.NewInt(100)})
	eng.Make("B", map[string]model.Value{"a-id": model.NewInt(100)})

	// Partial match for 1-2 should now be visible in both rules
	r1Info, _ = eng.RuleMatches("r1")
	r2Info, _ = eng.RuleMatches("r2")
	if len(r1Info.PartialMatches[0].Timetags) != 1 || len(r2Info.PartialMatches[0].Timetags) != 1 {
		t.Fatalf("expected 1 partial match [A, B] in both rules, got r1=%d, r2=%d",
			len(r1Info.PartialMatches[0].Timetags), len(r2Info.PartialMatches[0].Timetags))
	}

	// Assert C -> r1 fires
	eng.Make("C", map[string]model.Value{"val": model.NewInt(10)})
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected error during run: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule firing, got %d", fired)
	}
	if !strings.Contains(out.String(), "Fired r1 for 100") {
		t.Fatalf("expected r1 output, got:\n%s", out.String())
	}

	// Assert D -> r2 fires
	eng.Make("D", map[string]model.Value{"val": model.NewInt(20)})
	fired2, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected error during run: %v", err)
	}
	if fired2 != 1 {
		t.Fatalf("expected 1 rule firing for r2, got %d", fired2)
	}
	if !strings.Contains(out.String(), "Fired r2 for 100") {
		t.Fatalf("expected r2 output, got:\n%s", out.String())
	}
}

func TestEngineStructuralBetaSharingDynamicAdditionAndExcise(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Step 1: Assert WMEs first
	eng.Make("item", map[string]model.Value{
		"category": model.NewSymbol("electronics"),
		"active":   model.NewSymbol("yes"),
		"price":    model.NewInt(500),
	})

	// Step 2: Compile Rule 1
	script1 := `
(p alert-discount
	(item ^category <cat> ^active yes)
	(discount ^category <cat> ^rate <pct>)
-->
	(write "Discount:" <cat> <pct> (crlf))
)
`
	runDiagnosticScript(t, eng, script1)
	eng.Make("discount", map[string]model.Value{
		"category": model.NewSymbol("electronics"),
		"rate":     model.NewInt(20),
	})

	fired1, _ := eng.Run(-1)
	if fired1 != 1 {
		t.Fatalf("expected 1 firing for alert-discount, got %d", fired1)
	}
	if !strings.Contains(out.String(), "Discount: electronics 20") {
		t.Fatalf("expected discount output, got:\n%s", out.String())
	}

	// Step 3: Dynamically add Rule 2 sharing the exact same condition 1 (item ^category <cat> ^active yes)
	// and condition 2 (discount ^category <cat> ^rate <pct>) with condition 3 (coupon ^category <cat>)
	nodesBefore := eng.BetaNodeCount()
	script2 := `
(p alert-coupon
	(item ^category <cat> ^active yes)
	(discount ^category <cat> ^rate <pct>)
	(coupon ^category <cat> ^code <code>)
-->
	(write "Coupon:" <cat> <code> (crlf))
)
`
	runDiagnosticScript(t, eng, script2)
	nodesAfter := eng.BetaNodeCount()

	// Only 1 new beta node added (for condition 3), because conditions 1 and 2 are shared!
	if nodesAfter != nodesBefore+1 {
		t.Fatalf("expected exactly 1 new beta node for condition 3, before=%d, after=%d", nodesBefore, nodesAfter)
	}

	// Assert coupon and verify alert-coupon fires
	eng.Make("coupon", map[string]model.Value{
		"category": model.NewSymbol("electronics"),
		"code":     model.NewSymbol("SAVE50"),
	})
	fired2, _ := eng.Run(-1)
	if fired2 != 1 {
		t.Fatalf("expected 1 firing for alert-coupon, got %d", fired2)
	}
	if !strings.Contains(out.String(), "Coupon: electronics SAVE50") {
		t.Fatalf("expected coupon output, got:\n%s", out.String())
	}

	// Step 4: Excise alert-discount: shared beta nodes for conditions 1 and 2 must remain for alert-coupon
	ok := eng.ExciseRule("alert-discount")
	if !ok {
		t.Fatalf("expected alert-discount to be excised")
	}

	// Re-assert another coupon with new code: alert-coupon should still fire using shared beta memory!
	eng.Make("coupon", map[string]model.Value{
		"category": model.NewSymbol("electronics"),
		"code":     model.NewSymbol("SPRING20"),
	})
	fired3, _ := eng.Run(-1)
	if fired3 != 1 {
		t.Fatalf("expected 1 firing for alert-coupon after excise of alert-discount, got %d", fired3)
	}
	if !strings.Contains(out.String(), "Coupon: electronics SPRING20") {
		t.Fatalf("expected coupon output, got:\n%s", out.String())
	}

	// Step 5: Excise alert-coupon: now all beta nodes should be completely pruned!
	ok = eng.ExciseRule("alert-coupon")
	if !ok {
		t.Fatalf("expected alert-coupon to be excised")
	}
	if eng.BetaNodeCount() != 0 {
		t.Fatalf("expected 0 beta nodes after all rules excised, got %d", eng.BetaNodeCount())
	}
}
