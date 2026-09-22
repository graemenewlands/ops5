package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
)

// TestDisjunctionAttributeMatching verifies that << ... >> in condition elements
// correctly matches any of the specified constant values and filters non-matching ones.
func TestDisjunctionAttributeMatching(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p process-task
	(task ^id <tid> ^status << pending active in-progress >>)
-->
	(make processed ^task-id <tid>)
	(write "Processed task" <tid> (crlf))
)

(make task ^id 1 ^status pending)
(make task ^id 2 ^status active)
(make task ^id 3 ^status completed)
(make task ^id 4 ^status in-progress)
(make task ^id 5 ^status cancelled)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// Tasks 1 (pending), 2 (active), and 4 (in-progress) must match; 3 and 5 must not
	if fired != 3 {
		t.Fatalf("expected 3 rule firings, got %d", fired)
	}

	processed := eng.WorkingMemory().FindByClass("processed")
	if len(processed) != 3 {
		t.Fatalf("expected 3 processed WMEs, got %d", len(processed))
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Processed task 1") ||
		!strings.Contains(output, "Processed task 2") ||
		!strings.Contains(output, "Processed task 4") {
		t.Errorf("expected tasks 1, 2, and 4 to be processed")
	}
	if strings.Contains(output, "Processed task 3") || strings.Contains(output, "Processed task 5") {
		t.Errorf("tasks 3 and 5 should NOT have been processed")
	}
}

// TestDisjunctionRelationalOperators verifies disjunctions with comparison operators,
// such as << < 10 >= 100 >>.
func TestDisjunctionRelationalOperators(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p alert-extreme-reading
	(sensor ^id <sid> ^temp << < 10 >= 100 >>)
-->
	(make extreme-alert ^sensor-id <sid>)
	(write "Extreme sensor:" <sid> (crlf))
)

; 5 < 10 -> matches!
(make sensor ^id 1 ^temp 5)
; 50 is neither < 10 nor >= 100 -> does not match
(make sensor ^id 2 ^temp 50)
; 100 >= 100 -> matches!
(make sensor ^id 3 ^temp 100)
; 120 >= 100 -> matches!
(make sensor ^id 4 ^temp 120)
; 10 is not < 10 and not >= 100 -> does not match
(make sensor ^id 5 ^temp 10)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// Sensors 1 (5), 3 (100), and 4 (120) qualify
	if fired != 3 {
		t.Fatalf("expected 3 rule firings, got %d", fired)
	}

	alerts := eng.WorkingMemory().FindByClass("extreme-alert")
	if len(alerts) != 3 {
		t.Fatalf("expected 3 extreme-alert WMEs, got %d", len(alerts))
	}
}

// TestDisjunctionWithVariablesAndJoin verifies that disjunctions containing
// previously bound variables function as beta join constraints.
func TestDisjunctionWithVariablesAndJoin(t *testing.T) {
	eng := engine.New()

	script := `
(p route-packet
	(config ^primary <p> ^backup <b>)
	(packet ^id <pid> ^route << <p> <b> >>)
-->
	(make forwarded ^packet-id <pid>)
)

(make config ^primary route-A ^backup route-B)

(make packet ^id 101 ^route route-A)
(make packet ^id 102 ^route route-B)
(make packet ^id 103 ^route route-C)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 2 {
		t.Fatalf("expected 2 packet routings, got %d", fired)
	}

	forwarded := eng.WorkingMemory().FindByClass("forwarded")
	if len(forwarded) != 2 {
		t.Fatalf("expected 2 forwarded WMEs, got %d", len(forwarded))
	}
}

// TestBuildDynamicRuleGeneration verifies that the (build ...) RHS action compiles a new rule
// dynamically at runtime, substitutes parent variable bindings, and enables immediate firing.
func TestBuildDynamicRuleGeneration(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p learn-shortcut
	(edge ^from <src> ^to <dst>)
-->
	(build (p route-shortcut
		(traveler ^location <src>)
	-->
		(make arrived ^at <dst>)
		(write "Traveled from" <src> "to" <dst> (crlf))
	))
)

; Pre-existing traveler at Boston
(make traveler ^location Boston)

; Discovery of edge from Boston to Montreal
(make edge ^from Boston ^to Montreal)
`
	runScript(t, eng, script)

	// Step 1: Run 1 cycle -> learn-shortcut fires and builds rule route-shortcut
	cycles, err := eng.Run(1)
	if err != nil {
		t.Fatalf("cycle 1 failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 firing in cycle 1, got %d", cycles)
	}

	// Verify that the new rule now exists in production memory!
	if len(eng.Rules()) != 2 {
		t.Fatalf("expected 2 rules in engine after build, got %d", len(eng.Rules()))
	}

	// The traveler at Boston should now match the newly built rule!
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 pending activation for the built rule, got %d", eng.ConflictSet().Count())
	}

	// Step 2: Run cycle 2 -> newly built rule fires!
	cycles, err = eng.Run(1)
	if err != nil {
		t.Fatalf("cycle 2 failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 firing in cycle 2, got %d", cycles)
	}

	arrived := eng.WorkingMemory().FindByClass("arrived")
	if len(arrived) != 1 {
		t.Fatalf("expected 1 arrived WME, got %d", len(arrived))
	}
	destVal, _ := arrived[0].Get("at")
	if !destVal.Equal(model.NewSymbol("Montreal")) {
		t.Errorf("expected arrived at Montreal, got %v", destVal)
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Traveled from Boston to Montreal") {
		t.Errorf("unexpected output: %s", output)
	}
}

// TestBuildTopLevelStatement verifies that top-level (build (p ...)) statements are recognized and compiled.
func TestBuildTopLevelStatement(t *testing.T) {
	eng := engine.New()

	script := `
(build (p built-at-toplevel
	(item ^active yes)
-->
	(make result ^success true)
))

(make item ^active yes)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing, got %d", fired)
	}

	results := eng.WorkingMemory().FindByClass("result")
	if len(results) != 1 {
		t.Fatalf("expected 1 result WME, got %d", len(results))
	}
}
