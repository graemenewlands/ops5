package engine

import (
	"bytes"
	"strings"
	"testing"

	"ops5/pkg/model"
)

func TestEngineAccumulateSumOrderLines(t *testing.T) {
	eng := New()

	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Rule: Calculate total price of order lines and print summary
	ruleSrc := `
(p summarize-order
	(order ^id <oid> ^customer <cust>)
	(accumulate (order-line ^order-id <oid> ^price <p>) :sum <p> <total>)
-->
	(write "Order" <oid> "for" <cust> "has total:" <total> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	// Make order 101 for Alice
	eng.Make("order", map[string]model.Value{
		"id":       model.NewInt(101),
		"customer": model.NewSymbol("Alice"),
	})

	// Make 3 order lines for order 101: 50, 150, 200 -> total = 400
	eng.Make("order-line", map[string]model.Value{
		"order-id": model.NewInt(101),
		"price":    model.NewInt(50),
	})
	eng.Make("order-line", map[string]model.Value{
		"order-id": model.NewInt(101),
		"price":    model.NewInt(150),
	})
	eng.Make("order-line", map[string]model.Value{
		"order-id": model.NewInt(101),
		"price":    model.NewInt(200),
	})

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}

	output := out.String()
	if !strings.Contains(output, "Order 101 for Alice has total: 400") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestEngineAccumulateWithTestFilter(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Rule fires ONLY if the accumulated total exceeds 500
	ruleSrc := `
(p notify-large-order
	(order ^id <oid>)
	(accumulate (order-line ^order-id <oid> ^price <p>) :sum <p> <total>)
	(test (<total> > 500))
-->
	(write "Large order detected:" <oid> "with total" <total> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	eng.Make("order", map[string]model.Value{
		"id": model.NewInt(1),
	})
	line1 := eng.Make("order-line", map[string]model.Value{
		"order-id": model.NewInt(1),
		"price":    model.NewInt(200),
	})
	line2 := eng.Make("order-line", map[string]model.Value{
		"order-id": model.NewInt(1),
		"price":    model.NewInt(250),
	})

	// Total is 450 <= 500 -> rule should NOT fire!
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 0 {
		t.Fatalf("expected 0 rules to fire (total 450 <= 500), got %d", fired)
	}

	// Now modify line2 from 250 to 350 -> total becomes 550 > 500
	_ = line1
	eng.Modify(line2.Timetag, map[string]model.Value{
		"price": model.NewInt(350),
	})

	fired, err = eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire after total exceeds 500, got %d", fired)
	}

	output := out.String()
	if !strings.Contains(output, "Large order detected: 1 with total 550") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestEngineAccumulateCountPendingTasks(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p report-pending
	(system ^status ready)
	(accumulate (task ^status pending) :count <cnt>)
-->
	(write "Ready system has" <cnt> "pending tasks" (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	// Assert ready system
	eng.Make("system", map[string]model.Value{
		"status": model.NewSymbol("ready"),
	})

	// 0 tasks exist -> fires with 0!
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire with count=0, got %d", fired)
	}
	if !strings.Contains(out.String(), "Ready system has 0 pending tasks") {
		t.Errorf("unexpected output: %s", out.String())
	}

	out.Reset()

	// Assert 2 pending tasks and 1 completed task
	eng.Make("task", map[string]model.Value{"status": model.NewSymbol("pending")})
	eng.Make("task", map[string]model.Value{"status": model.NewSymbol("pending")})
	eng.Make("task", map[string]model.Value{"status": model.NewSymbol("completed")})

	// Rule should fire with count=2 because of new facts!
	fired, err = eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire with count=2, got %d", fired)
	}
	if !strings.Contains(out.String(), "Ready system has 2 pending tasks") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestEngineAccumulateAverageGrade(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p compute-class-gpa
	(class-info ^name <cname>)
	(accumulate (student ^class <cname> ^grade <g>) :avg <g> <gpa>)
-->
	(write "Class" <cname> "GPA:" <gpa> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	eng.Make("class-info", map[string]model.Value{
		"name": model.NewSymbol("physics"),
	})

	// 0 students -> avg does NOT fire
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 0 {
		t.Fatalf("expected 0 rules to fire for empty avg, got %d", fired)
	}

	// Add students with grades 70, 80, 90 -> avg = 80.0
	eng.Make("student", map[string]model.Value{"class": model.NewSymbol("physics"), "grade": model.NewInt(70)})
	eng.Make("student", map[string]model.Value{"class": model.NewSymbol("physics"), "grade": model.NewInt(80)})
	eng.Make("student", map[string]model.Value{"class": model.NewSymbol("physics"), "grade": model.NewInt(90)})

	fired, err = eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire for avg=80, got %d", fired)
	}
	if !strings.Contains(out.String(), "Class physics GPA: 80") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestEngineAccumulateRetroactiveCompilation(t *testing.T) {
	eng := New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Pre-populate working memory BEFORE rule is compiled
	eng.Make("order", map[string]model.Value{"id": model.NewInt(99)})
	eng.Make("item", map[string]model.Value{"order-id": model.NewInt(99), "val": model.NewInt(10)})
	eng.Make("item", map[string]model.Value{"order-id": model.NewInt(99), "val": model.NewInt(20)})
	eng.Make("item", map[string]model.Value{"order-id": model.NewInt(99), "val": model.NewInt(30)})

	ruleSrc := `
(p retroactive-sum
	(order ^id <oid>)
	(accumulate (item ^order-id <oid> ^val <v>) :sum <v> <tot>)
-->
	(write "Retroactive sum:" <tot> (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}
	if !strings.Contains(out.String(), "Retroactive sum: 60") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestEngineAccumulateRuleExcision(t *testing.T) {
	eng := New()

	ruleSrc := `
(p excise-target
	(parent ^id 1)
	(accumulate (child ^parent-id 1) :count <c>)
-->
	(write "Firing" (crlf))
)
`
	loadRule(t, eng, ruleSrc)

	eng.Make("parent", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("child", map[string]model.Value{"parent-id": model.NewInt(1)})

	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 pending activation, got %d", eng.ConflictSet().Count())
	}

	// Excise the rule
	if !eng.ExciseRule("excise-target") {
		t.Fatalf("failed to excise rule")
	}

	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 pending activations after excision, got %d", eng.ConflictSet().Count())
	}

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 0 {
		t.Fatalf("expected 0 firings after excision, got %d", fired)
	}
}
