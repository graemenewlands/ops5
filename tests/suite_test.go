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
	"ops5/pkg/parser"
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
	inBuf := strings.NewReader("Penelope\n")
	repl := cli.NewREPL(inBuf, &outBuf)

	err := repl.LoadFile(filepath.Join("integration", "2_4_3_b.ops5"))
	if err != nil {
		t.Fatalf("failed to load 2_4_3_b.ops5: %v", err)
	}

	// Assert Start to trigger initialization
	repl.Engine().Make("Start", nil)

	cycles, err := repl.Engine().Run(30)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 17 {
		t.Errorf("expected 17 cycles, got %d", cycles)
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

// TestIntegration2_4_3_c verifies Section 2.4.3 Part C integration test using FindAncestors::Stop with negative condition.
func TestIntegration2_4_3_c(t *testing.T) {
	var outBuf bytes.Buffer
	inBuf := strings.NewReader("Penelope\n")
	repl := cli.NewREPL(inBuf, &outBuf)

	err := repl.LoadFile(filepath.Join("integration", "2_4_3_c.ops5"))
	if err != nil {
		t.Fatalf("failed to load 2_4_3_c.ops5: %v", err)
	}

	// Assert Start to trigger initialization
	repl.Engine().Make("Start", nil)

	cycles, err := repl.Engine().Run(30)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 17 {
		t.Errorf("expected 17 cycles, got %d", cycles)
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
		"No More Ancestors",
	}

	for _, exp := range expectedAncestors {
		if !strings.Contains(out, exp) {
			t.Errorf("expected output to contain %q, but got:\n%s", exp, out)
		}
	}

	// In Part C, Penelope is the root request and should NOT be printed as an ancestor
	if strings.Contains(out, "Penelope is an ancestor") {
		t.Errorf("did not expect Penelope to be printed as an ancestor, but got:\n%s", out)
	}
}

// TestIntegration2_5_1_a verifies Section 2.5.1 Part A integration test using Initialize production rule to populate working memory.
func TestIntegration2_5_1_a(t *testing.T) {
	var outBuf bytes.Buffer
	inBuf := strings.NewReader("Penelope\n")
	repl := cli.NewREPL(inBuf, &outBuf)

	err := repl.LoadFile(filepath.Join("integration", "2_5_1_a.ops5"))
	if err != nil {
		t.Fatalf("failed to load 2_5_1_a.ops5: %v", err)
	}

	// Verify working memory is initially empty (no static load-time assertions)
	if count := repl.Engine().WorkingMemory().Count(); count != 0 {
		t.Errorf("expected empty working memory on load, got %d elements", count)
	}

	// Phase 1: Assert InitWM to trigger the Initialize production rule
	repl.Engine().Make("InitWM", nil)

	initCycles, err := repl.Engine().Run(10)
	if err != nil {
		t.Fatalf("initialization run failed: %v", err)
	}
	if initCycles != 1 {
		t.Errorf("expected 1 initialization cycle, got %d", initCycles)
	}

	// Verify that working memory now contains all 6 Person facts asserted by Initialize
	personWMEs := repl.Engine().WorkingMemory().FindByClass("Person")
	if len(personWMEs) != 6 {
		t.Errorf("expected 6 Person WMEs asserted by Initialize, got %d", len(personWMEs))
	}

	// Verify (InitWM) was consumed and removed by Initialize
	initWMEs := repl.Engine().WorkingMemory().FindByClass("InitWM")
	if len(initWMEs) != 0 {
		t.Errorf("expected InitWM token to be removed by Initialize, got %d", len(initWMEs))
	}

	// Phase 2: Assert Start to trigger interactive search via FindAncestors::Initialize
	repl.Engine().Make("Start", nil)

	searchCycles, err := repl.Engine().Run(30)
	if err != nil {
		t.Fatalf("search run failed: %v", err)
	}
	if searchCycles != 17 {
		t.Errorf("expected 17 search cycles (1 init prompt + 16 search), got %d", searchCycles)
	}

	// Verify (Start) was consumed and removed by FindAncestors::Initialize
	startWMEs := repl.Engine().WorkingMemory().FindByClass("Start")
	if len(startWMEs) != 0 {
		t.Errorf("expected Start token to be removed, got %d", len(startWMEs))
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
		"No More Ancestors",
	}

	for _, exp := range expectedAncestors {
		if !strings.Contains(out, exp) {
			t.Errorf("expected output to contain %q, but got:\n%s", exp, out)
		}
	}

	if strings.Contains(out, "Penelope is an ancestor") {
		t.Errorf("did not expect Penelope to be printed as an ancestor, but got:\n%s", out)
	}
}

// TestIntegration2_5_2 verifies Section 2.5.2 parameterized test cases using TestCase WMEs.
func TestIntegration2_5_2(t *testing.T) {
	t.Run("ancestornull", func(t *testing.T) {
		var outBuf bytes.Buffer
		inBuf := strings.NewReader("Penelope\n")
		repl := cli.NewREPL(inBuf, &outBuf)

		err := repl.LoadFile(filepath.Join("integration", "2_5_2.ops5"))
		if err != nil {
			t.Fatalf("failed to load 2_5_2.ops5: %v", err)
		}

		if count := repl.Engine().WorkingMemory().Count(); count != 0 {
			t.Errorf("expected 0 WMEs on load, got %d", count)
		}

		repl.Engine().Make("TestCase", map[string]model.Value{
			"type": model.NewSymbol("nulldb"),
			"name": model.NewSymbol("ancestornull"),
		})

		cycles, err := repl.Engine().Run(20)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if cycles != 3 {
			t.Errorf("expected 3 cycles for ancestornull, got %d", cycles)
		}

		out := outBuf.String()
		if !strings.Contains(out, "Please type the first name of a person") {
			t.Errorf("expected prompt in output, got:\n%s", out)
		}
		if !strings.Contains(out, "No More Ancestors") {
			t.Errorf("expected 'No More Ancestors' in output, got:\n%s", out)
		}
		if strings.Contains(out, "Penelope is an ancestor") {
			t.Errorf("did not expect Penelope to be printed as ancestor, got:\n%s", out)
		}
		if len(repl.Engine().WorkingMemory().FindByClass("TestCase")) != 0 {
			t.Errorf("expected TestCase to be consumed and removed")
		}
		if len(repl.Engine().WorkingMemory().FindByClass("Start")) != 0 {
			t.Errorf("expected Start to be consumed and removed")
		}
	})

	t.Run("ancestorsingle", func(t *testing.T) {
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		err := repl.LoadFile(filepath.Join("integration", "2_5_2.ops5"))
		if err != nil {
			t.Fatalf("failed to load 2_5_2.ops5: %v", err)
		}

		repl.Engine().Make("TestCase", map[string]model.Value{
			"type": model.NewSymbol("singledb"),
			"name": model.NewSymbol("ancestorsingle"),
		})

		cycles, err := repl.Engine().Run(20)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if cycles != 3 {
			t.Errorf("expected 3 cycles for ancestorsingle, got %d", cycles)
		}

		out := outBuf.String()
		if !strings.Contains(out, "No More Ancestors") {
			t.Errorf("expected 'No More Ancestors' in output, got:\n%s", out)
		}
		if strings.Contains(out, "Orphan is an ancestor") {
			t.Errorf("did not expect Orphan to be printed as ancestor, got:\n%s", out)
		}
		if strings.Contains(out, "is an ancestor") {
			t.Errorf("expected no ancestors to be printed, got:\n%s", out)
		}
		if len(repl.Engine().WorkingMemory().FindByClass("TestCase")) != 0 {
			t.Errorf("expected TestCase to be consumed and removed")
		}
		if len(repl.Engine().WorkingMemory().FindByClass("Person")) != 1 {
			t.Errorf("expected 1 Person WME (Orphan), got %d", len(repl.Engine().WorkingMemory().FindByClass("Person")))
		}
	})

	t.Run("ancestorgeneral", func(t *testing.T) {
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		err := repl.LoadFile(filepath.Join("integration", "2_5_2.ops5"))
		if err != nil {
			t.Fatalf("failed to load 2_5_2.ops5: %v", err)
		}

		repl.Engine().Make("TestCase", map[string]model.Value{
			"type": model.NewSymbol("generaldb"),
			"name": model.NewSymbol("ancestorgeneral"),
		})

		cycles, err := repl.Engine().Run(30)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if cycles != 11 {
			t.Errorf("expected 11 cycles for ancestorgeneral, got %d", cycles)
		}

		out := outBuf.String()
		expectedAncestors := []string{
			"Steven is an ancestor",
			"Jenny is an ancestor",
			"Jeremy is an ancestor",
			"Homer is an ancestor",
			"Mary-Elizabeth is an ancestor",
			"Jessica is an ancestor",
			"No More Ancestors",
		}
		for _, exp := range expectedAncestors {
			if !strings.Contains(out, exp) {
				t.Errorf("expected output to contain %q, but got:\n%s", exp, out)
			}
		}

		if strings.Contains(out, "Penelope is an ancestor") {
			t.Errorf("did not expect Penelope to be printed as ancestor, but got:\n%s", out)
		}
		// Stephanie and Jason should NOT be printed because they are not in this generaldb subset
		if strings.Contains(out, "Stephanie is an ancestor") {
			t.Errorf("did not expect Stephanie to be in generaldb subset")
		}
		if strings.Contains(out, "Jason is an ancestor") {
			t.Errorf("did not expect Jason to be in generaldb subset")
		}
		if len(repl.Engine().WorkingMemory().FindByClass("TestCase")) != 0 {
			t.Errorf("expected TestCase to be consumed and removed")
		}
	})
}

// TestExciseRuleIntegration verifies that excising rules via source file directives
// and programmatic engine calls evicts them from the Rete network, conflict set, and production memory.
func TestExciseRuleIntegration(t *testing.T) {
	t.Run("source_directive_excise", func(t *testing.T) {
		opsSrc := `
		(p rule-normal
			(job ^type normal)
			-->
			(write (crlf) "NORMAL JOB PROCESSED")
		)

		(p rule-priority
			(job ^type normal ^priority high)
			-->
			(write (crlf) "PRIORITY JOB PROCESSED")
		)

		; Excise rule-priority so only rule-normal can fire
		(excise rule-priority)

		(make job ^type normal ^priority high)
		`

		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)
		
		// Parse statements and execute
		p, err := parser.NewParser(opsSrc)
		if err != nil {
			t.Fatalf("failed to parse ops5 source: %v", err)
		}

		for {
			stmt, err := p.NextStatement()
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if stmt == nil {
				break
			}
			switch stmt.Type {
			case parser.StmtRule:
				repl.Engine().AddRule(stmt.Rule)
			case parser.StmtMake:
				repl.Engine().Make(stmt.MakeClass, stmt.MakeAttributes)
			case parser.StmtExcise:
				repl.Engine().ExciseRules(stmt.ExciseRules...)
			}
		}

		if repl.Engine().Rule("rule-priority") != nil {
			t.Fatalf("expected rule-priority to be excised")
		}
		if repl.Engine().Rule("rule-normal") == nil {
			t.Fatalf("expected rule-normal to remain in production memory")
		}

		cycles, err := repl.Engine().Run(10)
		if err != nil {
			t.Fatalf("engine run error: %v", err)
		}
		if cycles != 1 {
			t.Fatalf("expected 1 cycle, got %d", cycles)
		}

		out := outBuf.String()
		if strings.Contains(out, "PRIORITY JOB PROCESSED") {
			t.Errorf("excised rule-priority should not have fired, got:\n%s", out)
		}
		if !strings.Contains(out, "NORMAL JOB PROCESSED") {
			t.Errorf("expected rule-normal to fire, got:\n%s", out)
		}

		// Verify working memory was preserved
		jobs := repl.Engine().WorkingMemory().FindByClass("job")
		if len(jobs) != 1 {
			t.Errorf("expected 1 job WME in WM, got %d", len(jobs))
		}
	})

	t.Run("repl_interactive_excise", func(t *testing.T) {
		replInput := `
		(p first-rule
			(data ^num 42)
			-->
			(write (crlf) "FIRST FIRED")
		)
		(p second-rule
			(data ^num 42)
			-->
			(write (crlf) "SECOND FIRED")
		)
		make data ^num 42
		cs
		excise first-rule
		cs
		run
		exit
		`
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(replInput), &outBuf)
		repl.Start()

		out := outBuf.String()
		if !strings.Contains(out, "Conflict Set (2 activations") {
			t.Errorf("expected 2 activations initially, got:\n%s", out)
		}
		if !strings.Contains(out, "Excised rule 'first-rule'") {
			t.Errorf("expected excise confirmation, got:\n%s", out)
		}
		if !strings.Contains(out, "Conflict Set (1 activations") {
			t.Errorf("expected 1 activation after excise, got:\n%s", out)
		}
		if strings.Contains(out, "FIRST FIRED") {
			t.Errorf("excised rule should not fire, got:\n%s", out)
		}
		if !strings.Contains(out, "SECOND FIRED") {
			t.Errorf("un-excised rule should fire, got:\n%s", out)
		}
	})
}
