package engine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestEngineNccTwoConditionJoinWithParent(t *testing.T) {
	eng := New()

	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Rule: Request is clear if there is NOT BOTH a pending approval AND an active supervisor in that dept
	ruleSrc := `
(p request-clear
	(request ^id <rid> ^dept <d>)
	-( (pending-approval ^request-id <rid>)
	   (supervisor ^dept <d> ^active yes) )
-->
	(write "Request clear:" <rid> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	// 1. Assert request 10 in dev dept
	eng.Make("request", map[string]model.Value{
		"id":   model.NewInt(10),
		"dept": model.NewSymbol("dev"),
	})

	// Only request exists (0 subnetwork completions) -> rule is satisfied!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation when subnetwork is empty, got %d", eng.ConflictSet().Count())
	}

	// 2. Assert pending approval for request 10
	appr := eng.Make("pending-approval", map[string]model.Value{
		"request-id": model.NewInt(10),
	})

	// Subnetwork has 1 pending-approval, but NO matching supervisor -> conjunction is still FALSE!
	// Therefore negated conjunction is STILL SATISFIED!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected rule to still be satisfied (only 1 of 2 conditions present), got %d", eng.ConflictSet().Count())
	}

	// 3. Assert active supervisor in dev dept
	sup := eng.Make("supervisor", map[string]model.Value{
		"dept":   model.NewSymbol("dev"),
		"active": model.NewSymbol("yes"),
	})

	// Now BOTH pending-approval AND active supervisor exist!
	// Conjunction is TRUE, so negated conjunction is BLOCKED!
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected rule to be blocked when both conditions exist, got %d", eng.ConflictSet().Count())
	}

	// 4. Retract pending-approval -> conjunction broken!
	eng.Remove(appr.Timetag)

	// Rule should be unblocked and re-enter conflict set!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected rule to be unblocked after retracting approval, got %d", eng.ConflictSet().Count())
	}

	// Run engine
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}
	if !strings.Contains(out.String(), "Request clear: 10") {
		t.Errorf("unexpected output: %s", out.String())
	}

	_ = sup
}

func TestEngineNccIntraConjunctionJoin(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Rule: alert if there is a danger in zone <zid> with code <code> that has NO matching detector in zone <zid>
	// Here <code> is an intra-conjunction variable!
	ruleSrc := `
(p unmonitored-danger
	(zone ^id <zid>)
	(danger ^zone-id <zid> ^code <code>)
	-( (sensor ^zone-id <zid> ^detects <code>)
	   (sensor-status ^zone-id <zid> ^operational yes) )
-->
	(write "Unmonitored danger in zone" <zid> "code" <code> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	eng.Make("zone", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("danger", map[string]model.Value{
		"zone-id": model.NewInt(1),
		"code":    model.NewSymbol("fire"),
	})

	// No sensor -> rule is satisfied!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation, got %d", eng.ConflictSet().Count())
	}

	// Assert sensor detecting 'flood' (does not match 'fire')
	eng.Make("sensor", map[string]model.Value{
		"zone-id": model.NewInt(1),
		"detects": model.NewSymbol("flood"),
	})
	eng.Make("sensor-status", map[string]model.Value{
		"zone-id":     model.NewInt(1),
		"operational": model.NewSymbol("yes"),
	})

	// Still satisfied because detector is for flood, not fire!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation (sensor detects flood not fire), got %d", eng.ConflictSet().Count())
	}

	// Assert sensor detecting 'fire'
	sFire := eng.Make("sensor", map[string]model.Value{
		"zone-id": model.NewInt(1),
		"detects": model.NewSymbol("fire"),
	})

	// Now both fire sensor AND operational status exist!
	// Conjunction satisfied -> negated conjunction blocks rule!
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations (blocked by fire sensor + operational status), got %d", eng.ConflictSet().Count())
	}

	// Retract fire sensor
	eng.Remove(sFire.Timetag)

	// Unblocked!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation after retracting fire sensor, got %d", eng.ConflictSet().Count())
	}

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}
	if !strings.Contains(out.String(), "Unmonitored danger in zone 1 code fire") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestEngineNccRetroactiveCompilation(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Pre-populate working memory:
	// Order 1 with pending item, but NO blocker fact
	eng.Make("order", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("item", map[string]model.Value{"order-id": model.NewInt(1), "status": model.NewSymbol("pending")})

	ruleSrc := `
(p retroactive-ncc
	(order ^id <oid>)
	-( (item ^order-id <oid> ^status blocked)
	   (flag ^blocked true) )
-->
	(write "Order" <oid> "is not blocked" (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	// Retroactive check: blocked conjunction is false, so rule should be ready to fire!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 retroactive activation, got %d", eng.ConflictSet().Count())
	}

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}
	if !strings.Contains(out.String(), "Order 1 is not blocked") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestEngineNccRuleExcision(t *testing.T) {
	eng := New()

	ruleSrc := `
(p excise-ncc-rule
	(order ^id 1)
	-( (item ^id 1) (sub ^id 1) )
-->
	(write "Firing" (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	eng.Make("order", map[string]model.Value{"id": model.NewInt(1)})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation before excision, got %d", eng.ConflictSet().Count())
	}

	if !eng.ExciseRule("excise-ncc-rule") {
		t.Fatalf("failed to excise rule")
	}

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after excision, got %d", eng.ConflictSet().Count())
	}
}
