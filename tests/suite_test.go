package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ops5/pkg/cli"
	"ops5/pkg/engine"
	"ops5/pkg/harness"
	"ops5/pkg/model"
)

// TestCoreAlphaBetaNetwork verifies alpha filtering and beta join mechanics
func TestCoreAlphaBetaNetwork(t *testing.T) {
	eng := engine.New()

	// Rule: matches person with role developer and matching project
	rule := model.NewRule("assign-dev")
	rule.AddCondition(model.NewPositiveCE("person").
		AddEqualTest("role", model.NewSymbol("developer")).
		AddEqualTest("id", model.NewVariable("<pid>")))
	rule.AddCondition(model.NewPositiveCE("project").
		AddEqualTest("lead", model.NewVariable("<pid>")).
		AddEqualTest("status", model.NewSymbol("active")))
	rule.AddAction(model.MakeAction{
		Class: "assignment",
		Attributes: map[string]model.Value{
			"lead-id": model.NewVariable("<pid>"),
			"status":  model.NewSymbol("confirmed"),
		},
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	// Assert developer Alice (id=1)
	alice := eng.Make("person", map[string]model.Value{
		"id":   model.NewInt(1),
		"role": model.NewSymbol("developer"),
	})
	// Assert manager Bob (id=2)
	eng.Make("person", map[string]model.Value{
		"id":   model.NewInt(2),
		"role": model.NewSymbol("manager"),
	})
	// Assert project led by Bob (id=2) -> should NOT match Alice's rule
	eng.Make("project", map[string]model.Value{
		"lead":   model.NewInt(2),
		"status": model.NewSymbol("active"),
	})

	cycles, _ := eng.Run(10)
	if cycles != 0 {
		t.Fatalf("expected 0 firings before Alice's project is created, got %d", cycles)
	}

	// Assert project led by Alice (id=1)
	eng.Make("project", map[string]model.Value{
		"lead":   model.NewInt(1),
		"status": model.NewSymbol("active"),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 firing, got %d", cycles)
	}

	assignments := eng.WorkingMemory().FindByClass("assignment")
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	leadId, _ := assignments[0].Get("lead-id")
	if !leadId.Equal(model.NewInt(1)) {
		t.Fatalf("expected lead-id 1, got %v", leadId)
	}

	_ = alice
}

// TestDynamicWorkingMemoryRetraction verifies that removing a WME prevents activation
func TestDynamicWorkingMemoryRetraction(t *testing.T) {
	eng := engine.New()

	rule := model.NewRule("simple-fire")
	rule.AddCondition(model.NewPositiveCE("item").AddEqualTest("ready", model.NewSymbol("yes")))
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	// Assert WME
	item := eng.Make("item", map[string]model.Value{
		"ready": model.NewSymbol("yes"),
	})

	// Before running, retract it
	_, err := eng.Remove(item.Timetag)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	// Conflict set should now be empty
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected conflict set to be empty after retraction, got %d", eng.ConflictSet().Count())
	}

	cycles, _ := eng.Run(10)
	if cycles != 0 {
		t.Fatalf("expected 0 cycles after retraction, ran %d", cycles)
	}
}

// TestSalienceMEAversusLEX verifies salience ordering difference between MEA and LEX
func TestSalienceMEAversusLEX(t *testing.T) {
	runner := harness.NewRunner()

	// 1. Run mea_lex test with MEA strategy
	meaTestCase, err := runner.LoadTestCaseFromJSON(filepath.Join("fixtures", "mea_lex.json"))
	if err != nil {
		t.Fatalf("failed to load mea_lex.json: %v", err)
	}

	resMEA := runner.Run(meaTestCase)
	if !resMEA.Passed {
		t.Fatalf("MEA test failed: %v", resMEA.Error)
	}

	// 2. Change strategy to LEX on the exact same scenario
	lexTestCase := *meaTestCase
	lexTestCase.Name = "lex_strategy_verification"
	lexTestCase.Strategy = "LEX"
	// Under LEX:
	// Initial asserts:
	// 1: (goal ^id 1)
	// 2: (fact ^tag x)
	// 3: (goal ^id 2)
	// 4: (fact ^tag y)
	// Rule fire-goal-1 matches goal 1 and fact y (tags 1, 4) -> sorted [4, 1]
	// Rule fire-goal-2 matches goal 2 and fact x (tags 3, 2) -> sorted [3, 2]
	// 4 > 3 => fire-goal-1 dominates under LEX!
	lexTestCase.ExpectedWM = []harness.WMEAssertion{
		{
			Class:      "result",
			Attributes: map[string]string{"selected": "goal-1"},
		},
	}

	resLEX := runner.Run(&lexTestCase)
	if !resLEX.Passed {
		t.Fatalf("LEX test failed: %v", resLEX.Error)
	}
}

// TestExternalFileWorkflow runs the full workflow test loaded from JSON fixture referencing .ops file
func TestExternalFileWorkflow(t *testing.T) {
	runner := harness.NewRunner()
	tc, err := runner.LoadTestCaseFromJSON(filepath.Join("fixtures", "simple_workflow.json"))
	if err != nil {
		t.Fatalf("failed to load fixture: %v", err)
	}

	result := runner.Run(tc)
	if !result.Passed {
		t.Fatalf("test failed: %v (ran %d cycles, output:\n%s)", result.Error, result.CyclesRan, result.Output)
	}
}

// TestAllFixtureTestCases discovers and runs all JSON test cases in the fixtures folder
func TestAllFixtureTestCases(t *testing.T) {
	runner := harness.NewRunner()
	defer func() {
		_ = os.Remove("file_io_test.tmp")
		_ = os.Remove(filepath.Join("fixtures", "file_io_test.tmp"))
	}()
	matches, err := filepath.Glob(filepath.Join("fixtures", "*.json"))
	if err != nil {
		t.Fatalf("failed to glob fixtures: %v", err)
	}

	if len(matches) == 0 {
		t.Fatalf("no test fixture files found")
	}

	for _, file := range matches {
		tc, err := runner.LoadTestCaseFromJSON(file)
		if err != nil {
			t.Fatalf("failed to load test case %s: %v", file, err)
		}
		t.Run(tc.Name, func(t *testing.T) {
			res := runner.Run(tc)
			if !res.Passed {
				t.Errorf("fixture test %s failed: %v", file, res.Error)
			}
		})
	}
}

// TestIntegration2_4_3 verifies Section 2.4.3 Part A integration test from Brownston et al.
func TestIntegration2_4_3(t *testing.T) {
	TestIntegration2_4_3_a(t)
}

// TestIntegration2_4_3_a verifies Section 2.4.3 Part A integration test from Brownston et al.
func TestIntegration2_4_3_a(t *testing.T) {
	var outBuf bytes.Buffer
	inBuf := strings.NewReader("Penelope\n")
	repl := cli.NewREPL(inBuf, &outBuf)

	err := repl.LoadFile(filepath.Join("integration", "2_4_3_a.ops5"))
	if err != nil {
		t.Fatalf("failed to load 2_4_3_a.ops5: %v", err)
	}

	// Assert Start to trigger initialization
	repl.Engine().Make("Start", nil)

	cycles, err := repl.Engine().Run(20)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 7 {
		t.Errorf("expected 7 cycles, got %d", cycles)
	}

	out := outBuf.String()
	expectedAncestors := []string{
		"Jessica and Jeremy are ancestors via Penelope",
		"Jenny and Steven are ancestors via Jeremy",
		"Mary-Elizabeth and Homer are ancestors via Jessica",
		"Stephanie and nil are ancestors via Homer",
		"nil and Jason are ancestors via Loree",
		"Loree and nil are ancestors via Steven",
	}

	for _, exp := range expectedAncestors {
		if !strings.Contains(out, exp) {
			t.Errorf("expected output to contain %q, but got:\n%s", exp, out)
		}
	}
}

// TestIntegration2_4_3_b verifies Section 2.4.3 Part B integration test using specificity conflict resolution.
func TestIntegration2_4_3_b(t *testing.T) {
	var outBuf bytes.Buffer
	repl := cli.NewREPL(strings.NewReader(""), &outBuf)

	err := repl.LoadFile(filepath.Join("integration", "2_4_3_b.ops5"))
	if err != nil {
		t.Fatalf("failed to load 2_4_3_b.ops5: %v", err)
	}

	// Assert Request to trigger ancestor search for Penelope
	repl.Engine().Make("Request", map[string]model.Value{
		"type":   model.NewSymbol("ancestor"),
		"target": model.NewSymbol("Penelope"),
	})

	cycles, err := repl.Engine().Run(30)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 16 {
		t.Errorf("expected 16 cycles, got %d", cycles)
	}

	out := outBuf.String()
	expectedAncestors := []string{
		"Jason is an ancestor",
		"Loree is an ancestor",
		"Steven is an ancestor",
		"Jenny is an ancestor",
		"Jeremy is an ancestor",
		"Stephanie is an ancestor",
		"Homer is an ancestor",
		"Mary-Elizabeth is an ancestor",
		"Jessica is an ancestor",
		"Penelope is an ancestor",
	}

	for _, exp := range expectedAncestors {
		if !strings.Contains(out, exp) {
			t.Errorf("expected output to contain %q, but got:\n%s", exp, out)
		}
	}
}

