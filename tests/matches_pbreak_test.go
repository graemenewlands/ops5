package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/harness"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

func runDiagnosticScript(t *testing.T, eng *engine.Engine, script string) {
	p, err := parser.NewParser(script)
	if err != nil {
		t.Fatalf("parser init error: %v", err)
	}

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("syntax error: %v", err)
		}
		if stmt == nil {
			break
		}

		switch stmt.Type {
		case parser.StmtRule:
			eng.AddRule(stmt.Rule)
		case parser.StmtMake:
			eng.Make(stmt.MakeClass, stmt.MakeAttributes)
		case parser.StmtMatches:
			eng.FormatMatches(stmt.MatchesRules...)
		case parser.StmtPBreak:
			for _, name := range stmt.PBreakRules {
				eng.SetBreakpoint(name)
			}
		case parser.StmtUnpbreak:
			if len(stmt.UnpbreakRules) == 0 || stmt.UnpbreakRules[0] == "*" || strings.ToLower(stmt.UnpbreakRules[0]) == "nil" {
				eng.ClearBreakpoints()
			} else {
				for _, name := range stmt.UnpbreakRules {
					eng.RemoveBreakpoint(name)
				}
			}
		}
	}
}

func TestMatchesSingleCondition(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p find-goal
	(goal ^status pending)
-->
	(write "found pending")
)
`
	runDiagnosticScript(t, eng, script)

	// Initially no WMEs
	rep, ok := eng.RuleMatches("find-goal")
	if !ok || rep == nil {
		t.Fatalf("expected find-goal to be found")
	}
	if len(rep.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(rep.Conditions))
	}
	if len(rep.Conditions[0].WMEs) != 0 {
		t.Fatalf("expected 0 matching WMEs, got %d", len(rep.Conditions[0].WMEs))
	}
	if len(rep.Activations) != 0 {
		t.Fatalf("expected 0 activations, got %d", len(rep.Activations))
	}

	// Add non-matching WME and matching WME
	eng.Make("goal", map[string]model.Value{"status": model.NewSymbol("active")})
	wme2 := eng.Make("goal", map[string]model.Value{"status": model.NewSymbol("pending")})

	rep, ok = eng.RuleMatches("find-goal")
	if !ok {
		t.Fatalf("expected find-goal to be found")
	}
	if len(rep.Conditions[0].WMEs) != 1 {
		t.Fatalf("expected 1 matching WME, got %d", len(rep.Conditions[0].WMEs))
	}
	if rep.Conditions[0].WMEs[0].Timetag != wme2.Timetag {
		t.Fatalf("expected timetag %d, got %d", wme2.Timetag, rep.Conditions[0].WMEs[0].Timetag)
	}
	if len(rep.Activations) != 1 {
		t.Fatalf("expected 1 activation, got %d", len(rep.Activations))
	}
	if rep.Activations[0][0] != wme2.Timetag {
		t.Fatalf("expected activation timetag %d, got %v", wme2.Timetag, rep.Activations[0])
	}

	formatted := eng.FormatMatches("find-goal")
	if !strings.Contains(formatted, "** Matches for rule 'find-goal' **") {
		t.Errorf("expected header in formatted matches, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "pending") {
		t.Errorf("expected pending condition in formatted matches, got:\n%s", formatted)
	}
}

func TestMatchesMultipleConditionsAndPartialMatches(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p multi-join
	(step ^id 1)
	(item ^step 1 ^val <v>)
	(target ^step 1 ^thresh <t>)
	(flag ^step 1 ^active yes)
-->
	(write "multi matched")
)
`
	runDiagnosticScript(t, eng, script)

	// Assert CE 1
	w1 := eng.Make("step", map[string]model.Value{"id": model.NewInt(1)})

	rep, _ := eng.RuleMatches("multi-join")
	if len(rep.Conditions[0].WMEs) != 1 {
		t.Fatalf("expected 1 WME for CE 1, got %d", len(rep.Conditions[0].WMEs))
	}
	if len(rep.PartialMatches) != 2 {
		t.Fatalf("expected 2 partial match spans (1-2 and 1-3), got %d", len(rep.PartialMatches))
	}
	if len(rep.PartialMatches[0].Timetags) != 0 {
		t.Fatalf("expected 0 partial matches for 1-2, got %d", len(rep.PartialMatches[0].Timetags))
	}

	// Assert 2 matching items for CE 2
	w2 := eng.Make("item", map[string]model.Value{"step": model.NewInt(1), "val": model.NewInt(100)})
	w3 := eng.Make("item", map[string]model.Value{"step": model.NewInt(1), "val": model.NewInt(200)})

	rep, _ = eng.RuleMatches("multi-join")
	if len(rep.PartialMatches[0].Timetags) != 2 {
		t.Fatalf("expected 2 partial matches for 1-2, got %d", len(rep.PartialMatches[0].Timetags))
	}
	// Verify partial matches 1-2 contains [w1, w2] and [w1, w3]
	expectedSpan0 := rep.PartialMatches[0].CESpan
	if expectedSpan0 != "1-2" {
		t.Fatalf("expected span 1-2, got %s", expectedSpan0)
	}
	if len(rep.PartialMatches[1].Timetags) != 0 {
		t.Fatalf("expected 0 partial matches for 1-3 yet, got %d", len(rep.PartialMatches[1].Timetags))
	}

	// Assert 1 matching target for CE 3
	w4 := eng.Make("target", map[string]model.Value{"step": model.NewInt(1), "thresh": model.NewInt(50)})

	rep, _ = eng.RuleMatches("multi-join")
	if len(rep.PartialMatches[1].Timetags) != 2 {
		t.Fatalf("expected 2 partial matches for 1-3, got %d", len(rep.PartialMatches[1].Timetags))
	}
	if len(rep.Activations) != 0 {
		t.Fatalf("expected 0 activations before CE 4, got %d", len(rep.Activations))
	}

	// Assert matching flag for CE 4
	w5 := eng.Make("flag", map[string]model.Value{"step": model.NewInt(1), "active": model.NewSymbol("yes")})

	rep, _ = eng.RuleMatches("multi-join")
	if len(rep.Activations) != 2 {
		t.Fatalf("expected 2 activations after CE 4, got %d", len(rep.Activations))
	}

	formatted := engine.FormatRuleMatchReport(rep)
	if !strings.Contains(formatted, "Partial matches for CEs 1-2:") {
		t.Errorf("expected CE 1-2 header, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Partial matches for CEs 1-3:") {
		t.Errorf("expected CE 1-3 header, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Activations:") {
		t.Errorf("expected Activations header, got:\n%s", formatted)
	}
	_ = w1
	_ = w2
	_ = w3
	_ = w4
	_ = w5
}

func TestMatchesNegativeCondition(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p rule-with-neg
	(job ^id 42)
	-(job-lock ^id 42)
-->
	(write "unlocked job 42")
)
`
	runDiagnosticScript(t, eng, script)

	eng.Make("job", map[string]model.Value{"id": model.NewInt(42)})

	// Should be unblocked
	rep, ok := eng.RuleMatches("rule-with-neg")
	if !ok {
		t.Fatalf("rule-with-neg not found")
	}
	if !rep.Conditions[1].IsNegative {
		t.Fatalf("expected condition 2 to be negative")
	}
	if len(rep.Conditions[1].WMEs) != 0 {
		t.Fatalf("expected 0 blocking WMEs, got %d", len(rep.Conditions[1].WMEs))
	}
	if len(rep.Activations) != 1 {
		t.Fatalf("expected 1 activation when unblocked, got %d", len(rep.Activations))
	}
	fmtUnblocked := engine.FormatRuleMatchReport(rep)
	if !strings.Contains(fmtUnblocked, "None (no blocking WMEs)") {
		t.Errorf("expected 'None (no blocking WMEs)', got:\n%s", fmtUnblocked)
	}

	// Now add blocking WME
	eng.Make("job-lock", map[string]model.Value{"id": model.NewInt(42)})
	rep, _ = eng.RuleMatches("rule-with-neg")
	if len(rep.Conditions[1].WMEs) != 1 {
		t.Fatalf("expected 1 blocking WME, got %d", len(rep.Conditions[1].WMEs))
	}
	if len(rep.Activations) != 0 {
		t.Fatalf("expected 0 activations when blocked, got %d", len(rep.Activations))
	}
	fmtBlocked := engine.FormatRuleMatchReport(rep)
	if !strings.Contains(fmtBlocked, "(blocking WME)") {
		t.Errorf("expected '(blocking WME)', got:\n%s", fmtBlocked)
	}
}

func TestMatchesPredicateTestAndNCC(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p test-ncc-rule
	(num ^val <x>)
	(test (> <x> 10))
	-( (item ^id 1)
	   (item ^id 2) )
-->
	(write "matched")
)
`
	runDiagnosticScript(t, eng, script)

	rep, ok := eng.RuleMatches("test-ncc-rule")
	if !ok {
		t.Fatalf("test-ncc-rule not found")
	}
	foundTest := false
	foundNCC := false
	for _, c := range rep.Conditions {
		if c.IsTest {
			foundTest = true
		}
		if c.IsNCC {
			foundNCC = true
		}
	}
	if !foundTest {
		t.Errorf("expected to find predicate test condition")
	}
	if !foundNCC {
		t.Errorf("expected to find NCC condition")
	}

	formatted := engine.FormatRuleMatchReport(rep)
	if !strings.Contains(formatted, "[Predicate Test]") {
		t.Errorf("expected [Predicate Test] in output, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "[Negated Conjunction]") {
		t.Errorf("expected [Negated Conjunction] in output, got:\n%s", formatted)
	}
}

func TestMatchesWildcardAndNonExistent(t *testing.T) {
	eng := engine.New()

	script := `
(p r1 (foo ^x 1) --> (halt))
(p r2 (bar ^y 2) --> (halt))
`
	runDiagnosticScript(t, eng, script)

	allReps := eng.Matches("*")
	if len(allReps) != 2 {
		t.Fatalf("expected 2 reports from Matches('*'), got %d", len(allReps))
	}

	allRepsEmpty := eng.Matches()
	if len(allRepsEmpty) != 2 {
		t.Fatalf("expected 2 reports from Matches(), got %d", len(allRepsEmpty))
	}

	_, ok := eng.RuleMatches("unknown-rule")
	if ok {
		t.Fatalf("expected unknown-rule to return false")
	}

	fmtMissing := eng.FormatMatches("unknown-rule")
	if !strings.Contains(fmtMissing, "Rule 'unknown-rule' not found") {
		t.Errorf("expected not found message, got: %s", fmtMissing)
	}
}

func TestPBreakPauseAndResume(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p step-1
	(stage ^val 1)
-->
	(make stage ^val 2)
)

(p step-2
	(stage ^val 2)
-->
	(make stage ^val 3)
)

(p step-3
	(stage ^val 3)
-->
	(halt)
)
`
	runDiagnosticScript(t, eng, script)
	eng.Make("stage", map[string]model.Value{"val": model.NewInt(1)})

	// Set breakpoint on step-2
	eng.SetBreakpoint("step-2")
	if !eng.HasBreakpoint("step-2") {
		t.Fatalf("expected breakpoint to be set on step-2")
	}
	bps := eng.Breakpoints()
	if len(bps) != 1 || bps[0] != "step-2" {
		t.Fatalf("unexpected breakpoints list: %v", bps)
	}

	// Run with Run(-1): should execute step-1, then break before firing step-2
	cycles, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected error during Run: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle before break, got %d", cycles)
	}
	if eng.HitBreakpoint() != "step-2" {
		t.Fatalf("expected HitBreakpoint to be step-2, got '%s'", eng.HitBreakpoint())
	}

	// Verify that step-2 has NOT fired yet (stage 3 does not exist in WM)
	for _, w := range eng.WorkingMemory().All() {
		if w.Class == "stage" {
			if w.Attributes["val"].Raw().(int64) == 3 {
				t.Fatalf("stage 3 should not exist yet before step-2 fires")
			}
		}
	}

	// Step once: fires step-2
	ok, err := eng.Step()
	if err != nil || !ok {
		t.Fatalf("Step failed: %v", err)
	}
	if eng.HitBreakpoint() != "" {
		t.Fatalf("expected HitBreakpoint to be cleared after Step, got '%s'", eng.HitBreakpoint())
	}

	// Now stage 3 exists
	foundStage3 := false
	for _, w := range eng.WorkingMemory().All() {
		if w.Class == "stage" {
			if w.Attributes["val"].Raw().(int64) == 3 {
				foundStage3 = true
				break
			}
		}
	}
	if !foundStage3 {
		t.Fatalf("expected stage 3 to exist in WM after step-2")
	}

	// Continue with Run(-1): should fire step-3 and halt
	cycles2, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected error running remaining cycles: %v", err)
	}
	if cycles2 != 1 {
		t.Fatalf("expected 1 cycle to halt, got %d", cycles2)
	}
	if !eng.IsHalted() {
		t.Fatalf("expected engine to be halted")
	}
}

func TestPBreakResumeDirectlyWithRun(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p step-1
	(counter ^val 1)
-->
	(make counter ^val 2)
)

(p step-2
	(counter ^val 2)
-->
	(make counter ^val 3)
)

(p step-3
	(counter ^val 3)
-->
	(halt)
)
`
	runDiagnosticScript(t, eng, script)
	eng.Make("counter", map[string]model.Value{"val": model.NewInt(1)})

	// Set breakpoint on step-2 and step-3
	eng.SetBreakpoint("step-2")
	eng.SetBreakpoint("step-3")

	// First Run stops at step-2
	c1, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if c1 != 1 || eng.HitBreakpoint() != "step-2" {
		t.Fatalf("expected break on step-2, got cycle=%d, hit=%s", c1, eng.HitBreakpoint())
	}

	// Second Run resumes step-2, executes it, and stops at step-3!
	c2, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if c2 != 1 || eng.HitBreakpoint() != "step-3" {
		t.Fatalf("expected break on step-3, got cycle=%d, hit=%s", c2, eng.HitBreakpoint())
	}

	// Unbreak step-3 and continue to halt
	eng.RemoveBreakpoint("step-3")
	if eng.HasBreakpoint("step-3") {
		t.Fatalf("expected step-3 breakpoint removed")
	}

	c3, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if c3 != 1 || !eng.IsHalted() {
		t.Fatalf("expected halt after 1 cycle, got cycle=%d, halted=%v", c3, eng.IsHalted())
	}
}

func TestPBreakClearAndUnpbreak(t *testing.T) {
	eng := engine.New()
	eng.SetBreakpoint("r1")
	eng.SetBreakpoint("r2")
	eng.SetBreakpoint("r3")

	if len(eng.Breakpoints()) != 3 {
		t.Fatalf("expected 3 breakpoints, got %d", len(eng.Breakpoints()))
	}

	eng.RemoveBreakpoint("r2")
	if eng.HasBreakpoint("r2") {
		t.Fatalf("expected r2 removed")
	}
	if len(eng.Breakpoints()) != 2 {
		t.Fatalf("expected 2 breakpoints, got %d", len(eng.Breakpoints()))
	}

	eng.ClearBreakpoints()
	if len(eng.Breakpoints()) != 0 {
		t.Fatalf("expected 0 breakpoints after ClearBreakpoints, got %d", len(eng.Breakpoints()))
	}
}

func TestTopLevelMatchesAndPBreakHarness(t *testing.T) {
	runner := harness.NewRunner()
	tc := &harness.TestCase{
		Name: "test-matches-pbreak",
		Source: `
(p sample-rule
	(token ^val ok)
-->
	(halt)
)

(pbreak sample-rule other-rule)
(matches sample-rule)
(unpbreak sample-rule)
(unbreak *)
`,
	}
	res := runner.Run(tc)
	if !res.Passed {
		t.Fatalf("test case failed: %v", res.Error)
	}
}
