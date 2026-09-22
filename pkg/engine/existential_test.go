package engine

import (
	"fmt"
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestEngineExistentialJoinNoTokenMultiplication(t *testing.T) {
	eng := New()

	// Rule: notify once if an order has ANY pending items
	ruleSrc := `(p notify-pending-items
		(order ^id <oid>)
		(exists (item ^order-id <oid> ^status pending))
	-->
		(make alert ^order-id <oid> ^msg "has pending items")
	)`
	loadRule(t, eng, ruleSrc)

	// Assert order 101
	eng.Make("order", map[string]model.Value{"id": model.NewInt(101)})

	// Before any items, 0 activations
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations before items exist, got %d", eng.ConflictSet().Count())
	}

	// Assert 50 matching items for order 101
	for i := 1; i <= 50; i++ {
		eng.Make("item", map[string]model.Value{
			"order-id": model.NewInt(101),
			"item-id":  model.NewInt(int64(i)),
			"status":   model.NewSymbol("pending"),
		})
	}

	// In standard join, this would produce 50 activations!
	// With ExistentialJoinNode, it must produce EXACTLY 1 activation!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected exactly 1 activation despite 50 matching items (no token multiplication), got %d", eng.ConflictSet().Count())
	}

	// Run engine: should fire exactly 1 time
	fired, _ := eng.Run(100)
	if fired != 1 {
		t.Fatalf("expected exactly 1 rule firing, got %d", fired)
	}

	alerts := eng.FindWMEsMatching(model.NewPositiveCE("alert"))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert created, got %d", len(alerts))
	}
}

func TestEngineExistentialJoinRetractionLifecycle(t *testing.T) {
	eng := New()

	ruleSrc := `(p unblock-gate
		(gate ^id <gid>)
		(exists (ticket ^gate-id <gid> ^valid yes))
	-->
		(make opened-gate ^id <gid>)
	)`
	loadRule(t, eng, ruleSrc)

	eng.Make("gate", map[string]model.Value{"id": model.NewInt(1)})

	// Add 3 valid tickets
	t1 := eng.Make("ticket", map[string]model.Value{"gate-id": model.NewInt(1), "valid": model.NewSymbol("yes")})
	t2 := eng.Make("ticket", map[string]model.Value{"gate-id": model.NewInt(1), "valid": model.NewSymbol("yes")})
	t3 := eng.Make("ticket", map[string]model.Value{"gate-id": model.NewInt(1), "valid": model.NewSymbol("yes")})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation, got %d", eng.ConflictSet().Count())
	}

	// Retract ticket 1 (2 remain) -> activation must stay
	eng.Remove(t1.Timetag)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to persist after retracting 1 of 3 tickets, got %d", eng.ConflictSet().Count())
	}

	// Retract ticket 2 (1 remains) -> activation must stay
	eng.Remove(t2.Timetag)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to persist after retracting 2 of 3 tickets, got %d", eng.ConflictSet().Count())
	}

	// Retract ticket 3 (0 remain) -> activation must disappear!
	eng.Remove(t3.Timetag)
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected activation to disappear when all tickets retracted, got %d", eng.ConflictSet().Count())
	}

	// Re-add ticket -> activation must return!
	eng.Make("ticket", map[string]model.Value{"gate-id": model.NewInt(1), "valid": model.NewSymbol("yes")})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected activation to return upon new ticket assertion, got %d", eng.ConflictSet().Count())
	}

	fired, _ := eng.Run(10)
	if fired != 1 {
		t.Fatalf("expected 1 firing, got %d", fired)
	}
}

func TestEngineExistentialJoinPipelineChaining(t *testing.T) {
	eng := New()

	// Rule: project has blocked tasks AND manager exists
	ruleSrc := `(p alert-manager-blocked-project
		(project ^id <pid> ^name <pname>)
		(exists (task ^project-id <pid> ^status blocked))
		(manager ^project-id <pid> ^email <email>)
	-->
		(make notification ^to <email> ^project <pname>)
	)`
	loadRule(t, eng, ruleSrc)

	eng.Make("project", map[string]model.Value{"id": model.NewInt(10), "name": model.NewString("Apollo")})
	eng.Make("manager", map[string]model.Value{"project-id": model.NewInt(10), "email": model.NewString("lead@apollo.org")})

	// Non-blocked task does not activate
	eng.Make("task", map[string]model.Value{"project-id": model.NewInt(10), "status": model.NewSymbol("done")})
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations with only done tasks, got %d", eng.ConflictSet().Count())
	}

	// Add 5 blocked tasks -> should produce exactly 1 activation!
	for i := 1; i <= 5; i++ {
		eng.Make("task", map[string]model.Value{
			"project-id": model.NewInt(10),
			"task-name":  model.NewString(fmt.Sprintf("task-%d", i)),
			"status":     model.NewSymbol("blocked"),
		})
	}

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation, got %d", eng.ConflictSet().Count())
	}

	fired, _ := eng.Run(10)
	if fired != 1 {
		t.Fatalf("expected 1 rule firing, got %d", fired)
	}

	notifs := eng.FindWMEsMatching(model.NewPositiveCE("notification"))
	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
}

func TestEngineExistentialJoinRetroactiveCompilation(t *testing.T) {
	eng := New()

	// Assert facts prior to rule compilation
	eng.Make("account", map[string]model.Value{"id": model.NewInt(55)})
	eng.Make("txn", map[string]model.Value{"account-id": model.NewInt(55), "fraud-flag": model.NewSymbol("yes")})
	eng.Make("txn", map[string]model.Value{"account-id": model.NewInt(55), "fraud-flag": model.NewSymbol("yes")})

	ruleSrc := `(p freeze-fraud-account
		(account ^id <aid>)
		(exists (txn ^account-id <aid> ^fraud-flag yes))
	-->
		(make frozen-account ^id <aid>)
	)`
	loadRule(t, eng, ruleSrc)

	// Retroactive catch-up should produce exactly 1 activation
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 retroactive activation, got %d", eng.ConflictSet().Count())
	}

	fired, _ := eng.Run(10)
	if fired != 1 {
		t.Fatalf("expected 1 firing, got %d", fired)
	}
}

func TestEngineExistentialJoinExcise(t *testing.T) {
	eng := New()

	ruleSrc := `(p rule-to-excise
		(account ^id <aid>)
		(exists (txn ^account-id <aid>))
	-->
		(make flag)
	)`
	loadRule(t, eng, ruleSrc)

	eng.Make("account", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("txn", map[string]model.Value{"account-id": model.NewInt(1)})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation before excise")
	}

	if !eng.ExciseRule("rule-to-excise") {
		t.Fatalf("failed to excise rule")
	}

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations after excise, got %d", eng.ConflictSet().Count())
	}
}
