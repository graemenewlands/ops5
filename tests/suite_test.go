package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/cli"
	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/harness"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
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

// TestPMCommandIntegration verifies the (pm ...) directive and REPL command for printing production rules.
func TestPMCommandIntegration(t *testing.T) {
	t.Run("source_directive_pm", func(t *testing.T) {
		opsSrc := `
		(p FindAncestors
			(Request ^type ancestor ^target <p>)
			(Person ^name <p> ^father <f>)
			-->
			(make Request ^type ancestor ^target <f>)
			(write (crlf) <f> "is an ancestor")
		)
		(pm FindAncestors)
		`
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		p, err := parser.NewParser(opsSrc)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}

		var pmOutput []string
		for {
			stmt, err := p.NextStatement()
			if err != nil {
				t.Fatalf("next statement error: %v", err)
			}
			if stmt == nil {
				break
			}
			switch stmt.Type {
			case parser.StmtRule:
				repl.Engine().AddRule(stmt.Rule)
			case parser.StmtPM:
				pmOutput = append(pmOutput, repl.Engine().PrintRules(stmt.PMRules...)...)
			}
		}

		if len(pmOutput) != 1 {
			t.Fatalf("expected 1 printed rule from (pm FindAncestors), got %d", len(pmOutput))
		}
		if !strings.Contains(pmOutput[0], "(p FindAncestors") {
			t.Errorf("expected FindAncestors in pm output, got:\n%s", pmOutput[0])
		}
		if !strings.Contains(pmOutput[0], "(Request ^type ancestor ^target <p>)") {
			t.Errorf("expected Request CE in pm output, got:\n%s", pmOutput[0])
		}
		if !strings.Contains(pmOutput[0], "(Person ^name <p> ^father <f>)") {
			t.Errorf("expected Person CE in pm output, got:\n%s", pmOutput[0])
		}
		if !strings.Contains(pmOutput[0], `(write (crlf) <f> "is an ancestor")`) {
			t.Errorf("expected write action in pm output, got:\n%s", pmOutput[0])
		}
	})

	t.Run("repl_pm_wildcard", func(t *testing.T) {
		replInput := `
		(p rule-1 (item ^id 1) --> (halt))
		(p rule-2 (item ^id 2) --> (halt))
		(pm FindAncestors)
		pm *
		(pm rule-1)
		exit
		`
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(replInput), &outBuf)
		repl.Start()

		out := outBuf.String()
		if !strings.Contains(out, "Rule 'FindAncestors' not found") {
			t.Errorf("expected not found message for missing rule, got:\n%s", out)
		}
		if !strings.Contains(out, "(p rule-1") || !strings.Contains(out, "(p rule-2") {
			t.Errorf("expected rules in pm * output, got:\n%s", out)
		}
	})
}

// TestRemoveWildcardIntegration tests (remove *) in source files, REPL sessions, and RHS rule actions.
func TestRemoveWildcardIntegration(t *testing.T) {
	t.Run("source_top_level_remove_wildcard", func(t *testing.T) {
		opsSrc := `
		(make person ^name Alice)
		(make person ^name Bob)
		; Retract all working memory elements
		(remove *)
		; Assert single element
		(make person ^name Charlie)
		`
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		p, err := parser.NewParser(opsSrc)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}

		for {
			stmt, err := p.NextStatement()
			if err != nil {
				t.Fatalf("parse statement error: %v", err)
			}
			if stmt == nil {
				break
			}
			switch stmt.Type {
			case parser.StmtMake:
				repl.Engine().Make(stmt.MakeClass, stmt.MakeAttributes)
			case parser.StmtRemove:
				if stmt.RemoveWildcard {
					repl.Engine().RemoveAll()
				}
			}
		}

		// Only Charlie should remain
		persons := repl.Engine().WorkingMemory().FindByClass("person")
		if len(persons) != 1 {
			t.Fatalf("expected exactly 1 person remaining, got %d", len(persons))
		}
		nameVal, _ := persons[0].Get("name")
		if !nameVal.Equal(model.NewSymbol("Charlie")) {
			t.Errorf("expected Charlie, got %v", nameVal)
		}
		// Charlie should have timetag 3 (1=Alice, 2=Bob, 3=Charlie)
		if persons[0].Timetag != 3 {
			t.Errorf("expected timetag 3 for Charlie, got %d", persons[0].Timetag)
		}
	})

	t.Run("rhs_rule_remove_wildcard", func(t *testing.T) {
		opsSrc := `
		(p abort-mission
			(Emergency ^level critical)
			-->
			(remove *)
			(write (crlf) "MISSION ABORTED - WORKING MEMORY CLEARED")
		)
		`
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		p, _ := parser.NewParser(opsSrc)
		stmt, _ := p.NextStatement()
		repl.Engine().AddRule(stmt.Rule)

		repl.Engine().Make("Task", map[string]model.Value{"status": model.NewSymbol("in-progress")})
		repl.Engine().Make("Task", map[string]model.Value{"status": model.NewSymbol("in-progress")})
		repl.Engine().Make("Emergency", map[string]model.Value{"level": model.NewSymbol("critical")})

		if repl.Engine().WorkingMemory().Count() != 3 {
			t.Fatalf("expected 3 WMEs before run, got %d", repl.Engine().WorkingMemory().Count())
		}

		cycles, err := repl.Engine().Run(10)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if cycles != 1 {
			t.Fatalf("expected 1 cycle, got %d", cycles)
		}

		if repl.Engine().WorkingMemory().Count() != 0 {
			t.Errorf("expected 0 WMEs after (remove *), got %d", repl.Engine().WorkingMemory().Count())
		}
		if repl.Engine().ConflictSet().Count() != 0 {
			t.Errorf("expected 0 activations after (remove *), got %d", repl.Engine().ConflictSet().Count())
		}
		if !strings.Contains(outBuf.String(), "MISSION ABORTED - WORKING MEMORY CLEARED") {
			t.Errorf("expected abort message, got: %q", outBuf.String())
		}
	})
}

// TestWatchIntegration tests the watch command and watch levels 0, 1, and 2
// across source directives, RHS actions, and REPL/engine execution.
func TestWatchIntegration(t *testing.T) {
	t.Run("source_directive_and_rhs_watch_transitions", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "watch_test.ops5")

		opsSrc := `
		(watch)
		(watch 2)

		(p step-one
			<c> (Counter ^val 0)
			-->
			(modify <c> ^val 1)
			(watch 1)
		)

		(p step-two
			<c> (Counter ^val 1)
			-->
			(modify <c> ^val 2)
			(watch 0)
		)

		(p step-three
			<c> (Counter ^val 2)
			-->
			(remove <c>)
		)

		(make Counter ^val 0)
		`

		if err := os.WriteFile(srcFile, []byte(opsSrc), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(""), &outBuf)

		if err := repl.LoadFile(srcFile); err != nil {
			t.Fatalf("failed to load file: %v", err)
		}

		// Initial (watch) query in source file should report current watch level 1
		outOnLoad := outBuf.String()
		if !strings.Contains(outOnLoad, "Current watch level: 1") {
			t.Errorf("expected initial watch query output, got:\n%s", outOnLoad)
		}
		// (make Counter ^val 0) occurred while (watch 2) was active
		if !strings.Contains(outOnLoad, "=>WM: (1: Counter ^val 0)") {
			t.Errorf("expected =>WM: for make under watch 2, got:\n%s", outOnLoad)
		}

		// Run step 1: (watch 2) is active initially
		// step-one fires, modifies Counter to val 1, and changes watch to 1
		cycles, err := repl.Engine().Run(1)
		if err != nil || cycles != 1 {
			t.Fatalf("step-one run failed: err=%v, cycles=%d", err, cycles)
		}

		outStep1 := outBuf.String()
		if !strings.Contains(outStep1, "Fired rule 'step-one'") {
			t.Errorf("expected firing trace for step-one, got:\n%s", outStep1)
		}
		if !strings.Contains(outStep1, "<=WM: (1: Counter ^val 0)") {
			t.Errorf("expected <=WM: retraction on modify in step-one, got:\n%s", outStep1)
		}
		if !strings.Contains(outStep1, "=>WM: (2: Counter ^val 1)") {
			t.Errorf("expected =>WM: assertion on modify in step-one, got:\n%s", outStep1)
		}

		if repl.Engine().WatchLevel() != 1 {
			t.Fatalf("expected watch level 1 after step-one, got %d", repl.Engine().WatchLevel())
		}

		// Run step 2: (watch 1) is active
		// step-two fires, modifies Counter to val 2, and changes watch to 0
		cycles, err = repl.Engine().Run(1)
		if err != nil || cycles != 1 {
			t.Fatalf("step-two run failed: err=%v, cycles=%d", err, cycles)
		}

		outStep2 := outBuf.String()
		if !strings.Contains(outStep2, "Fired rule 'step-two'") {
			t.Errorf("expected firing trace for step-two under watch 1, got:\n%s", outStep2)
		}
		// Under watch 1, WME modifications should NOT be logged
		if strings.Contains(outStep2, "<=WM: (2: Counter ^val 1)") {
			t.Errorf("did not expect <=WM: for step-two under watch 1, got:\n%s", outStep2)
		}
		if strings.Contains(outStep2, "=>WM: (3: Counter ^val 2)") {
			t.Errorf("did not expect =>WM: for step-two under watch 1, got:\n%s", outStep2)
		}

		if repl.Engine().WatchLevel() != 0 {
			t.Fatalf("expected watch level 0 after step-two, got %d", repl.Engine().WatchLevel())
		}

		// Run step 3: (watch 0) is active
		// step-three fires and removes Counter
		markLen := outBuf.Len()
		cycles, err = repl.Engine().Run(1)
		if err != nil || cycles != 1 {
			t.Fatalf("step-three run failed: err=%v, cycles=%d", err, cycles)
		}

		outStep3 := outBuf.String()[markLen:]
		if strings.Contains(outStep3, "Fired rule 'step-three'") {
			t.Errorf("did not expect firing trace for step-three under watch 0, got:\n%s", outStep3)
		}
		if strings.Contains(outStep3, "<=WM:") || strings.Contains(outStep3, "=>WM:") {
			t.Errorf("did not expect any WM traces under watch 0, got:\n%s", outStep3)
		}

		if repl.Engine().WorkingMemory().Count() != 0 {
			t.Errorf("expected empty working memory at end, got %d", repl.Engine().WorkingMemory().Count())
		}
	})
}

func TestPPWMIntegration(t *testing.T) {
	runner := harness.NewRunner()
	src := `
	(literalize City name state population)
	(make City ^name Pittsburgh ^state Pennsylvania ^population 300000)
	(make City ^name Philadelphia ^state Pennsylvania ^population 1500000)
	(make City ^name Boston ^state Massachusetts ^population 675000)
	(make Person ^name Franklin ^state Pennsylvania)
	(ppwm City ^state Pennsylvania)
	(ppwm City ^name Boston)
	`
	tc := &harness.TestCase{
		Name:     "ppwm_integration_test",
		Strategy: "lex",
		Source:   src,
	}

	res := runner.Run(tc)
	if !res.Passed {
		t.Fatalf("runner failed: %v", res.Error)
	}

	output := res.Output
	if !strings.Contains(output, "(1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)") {
		t.Errorf("expected Pittsburgh in output, got:\n%s", output)
	}
	if !strings.Contains(output, "(2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)") {
		t.Errorf("expected Philadelphia in output, got:\n%s", output)
	}
	if !strings.Contains(output, "(3: City ^name Boston ^population 675000 ^state Massachusetts)") {
		t.Errorf("expected Boston in output, got:\n%s", output)
	}
	// Person Franklin should not match (ppwm City ^state Pennsylvania)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, l := range lines {
		if strings.Contains(l, "Person") {
			t.Errorf("did not expect Person in output of City queries, got line: %s", l)
		}
	}
}

// TestIntegration3_MonkeyBananas verifies Section 3 Monkey and Bananas Problem
// from Brownston et al. (1985) across all required environments and initial conditions.
func TestIntegration3_MonkeyBananas(t *testing.T) {
	loadMonkeyBananas := func(t *testing.T, in string) (*cli.REPL, *bytes.Buffer) {
		t.Helper()
		var outBuf bytes.Buffer
		repl := cli.NewREPL(strings.NewReader(in), &outBuf)
		err := repl.LoadFile(filepath.Join("integration", "3_monkey_bananas.ops5"))
		if err != nil {
			t.Fatalf("failed to load 3_monkey_bananas.ops5: %v", err)
		}
		if repl.Engine().ConflictSet().Strategy() != conflict.StrategyMEA {
			t.Fatalf("expected strategy MEA, got %v", repl.Engine().ConflictSet().Strategy())
		}
		return repl, &outBuf
	}

	t.Run("ceiling_bananas", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("ceiling"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("ceiling run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"jump onto the floor",
			"walk to 9-5",
			"grab ladder",
			"walk to 2-2",
			"drop ladder",
			"climb onto ladder",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}

		// Verify final WM state
		monkeys := repl.Engine().WorkingMemory().FindByClass("monkey")
		if len(monkeys) != 1 {
			t.Fatalf("expected 1 monkey in WM, got %d", len(monkeys))
		}
		holdsVal, _ := monkeys[0].Get("holds")
		if !holdsVal.Equal(model.NewSymbol("bananas")) {
			t.Errorf("expected monkey to hold bananas, got %v", holdsVal)
		}
		onVal, _ := monkeys[0].Get("on")
		if !onVal.Equal(model.NewSymbol("ladder")) {
			t.Errorf("expected monkey on ladder, got %v", onVal)
		}
	})

	t.Run("floor_bananas", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("floor"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("floor run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"jump onto the floor",
			"walk to 2-2",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}

		monkeys := repl.Engine().WorkingMemory().FindByClass("monkey")
		if len(monkeys) != 1 {
			t.Fatalf("expected 1 monkey in WM, got %d", len(monkeys))
		}
		holdsVal, _ := monkeys[0].Get("holds")
		if !holdsVal.Equal(model.NewSymbol("bananas")) {
			t.Errorf("expected monkey to hold bananas, got %v", holdsVal)
		}
	})

	t.Run("couch_bananas", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("couch"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("couch run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"walk to 5-7",
			"climb onto couch",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}

		monkeys := repl.Engine().WorkingMemory().FindByClass("monkey")
		if len(monkeys) != 1 {
			t.Fatalf("expected 1 monkey in WM, got %d", len(monkeys))
		}
		holdsVal, _ := monkeys[0].Get("holds")
		if !holdsVal.Equal(model.NewSymbol("bananas")) {
			t.Errorf("expected monkey to hold bananas, got %v", holdsVal)
		}
		onVal, _ := monkeys[0].Get("on")
		if !onVal.Equal(model.NewSymbol("couch")) {
			t.Errorf("expected monkey on couch, got %v", onVal)
		}
	})

	t.Run("ladder_bananas", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("ladder"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("ladder run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"jump onto the floor",
			"walk to 9-5",
			"climb onto ladder",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}

		monkeys := repl.Engine().WorkingMemory().FindByClass("monkey")
		if len(monkeys) != 1 {
			t.Fatalf("expected 1 monkey in WM, got %d", len(monkeys))
		}
		holdsVal, _ := monkeys[0].Get("holds")
		if !holdsVal.Equal(model.NewSymbol("bananas")) {
			t.Errorf("expected monkey to hold bananas, got %v", holdsVal)
		}
	})

	t.Run("holding_blanket", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("blanket"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("blanket run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"jump onto the floor",
			"walk to 9-5",
			"drop blanket",
			"grab ladder",
			"walk to 2-2",
			"drop ladder",
			"climb onto ladder",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}

		monkeys := repl.Engine().WorkingMemory().FindByClass("monkey")
		if len(monkeys) != 1 {
			t.Fatalf("expected 1 monkey in WM, got %d", len(monkeys))
		}
		holdsVal, _ := monkeys[0].Get("holds")
		if !holdsVal.Equal(model.NewSymbol("bananas")) {
			t.Errorf("expected monkey to hold bananas, got %v", holdsVal)
		}
	})

	t.Run("couch_holding_blanket", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("couch-blanket"),
		})
		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("couch-blanket run completed in %d cycles", cycles)
		out := outBuf.String()

		expectedCmds := []string{
			"drop blanket",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		lastIdx := -1
		for _, cmd := range expectedCmds {
			idx := strings.Index(out, cmd)
			if idx == -1 {
				t.Errorf("expected output to contain command %q, got:\n%s", cmd, out)
			} else if idx < lastIdx {
				t.Errorf("command %q appeared out of order, got:\n%s", cmd, out)
			}
			lastIdx = idx
		}
	})

	t.Run("heavy_couch_constraint", func(t *testing.T) {
		repl, outBuf := loadMonkeyBananas(t, "")
		repl.Engine().Make("TestCase", map[string]model.Value{
			"name": model.NewSymbol("heavy-couch"),
		})
		cycles, err := repl.Engine().Run(10)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("heavy-couch run completed in %d cycles", cycles)
		out := outBuf.String()

		if !strings.Contains(out, "The monkey is incapable of moving heavy object couch") {
			t.Errorf("expected heavy object error message, got:\n%s", out)
		}
	})

	t.Run("interactive_input_scenario_choice", func(t *testing.T) {
		// Test selecting "ceiling" via interactive selection prompt
		input := "ceiling\n"
		repl, outBuf := loadMonkeyBananas(t, input)
		repl.Engine().Make("Start", nil)

		cycles, err := repl.Engine().Run(50)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("interactive scenario selection completed in %d cycles", cycles)
		out := outBuf.String()

		if !strings.Contains(out, "=== Monkey & Bananas Problem (10'x10'x10' Room) ===") {
			t.Errorf("expected initial prompt header, got:\n%s", out)
		}
		if !strings.Contains(out, "grab bananas") {
			t.Errorf("expected grab bananas in output, got:\n%s", out)
		}
		if !strings.Contains(out, "SUCCESS: The monkey has grabbed the bananas!") {
			t.Errorf("expected success message, got:\n%s", out)
		}
	})

	t.Run("interactive_input_custom_locations", func(t *testing.T) {
		// Test interactive step-by-step entry
		// monkey-at: 5-7, monkey-on: couch, monkey-holds: nil, couch-at: 5-7, ladder-at: 9-5, bananas-at: 2-2, bananas-on: ceiling
		input := "interactive\n5-7\ncouch\nnil\n5-7\n9-5\n2-2\nceiling\n"
		repl, outBuf := loadMonkeyBananas(t, input)
		repl.Engine().Make("Start", nil)

		cycles, err := repl.Engine().Run(60)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		t.Logf("interactive custom location setup completed in %d cycles", cycles)
		out := outBuf.String()

		expectedPrompts := []string{
			"Enter monkey location (e.g. 5-7):",
			"Enter monkey surface (floor or couch):",
			"Enter monkey held item (nil or blanket):",
			"Enter couch location (e.g. 5-7):",
			"Enter ladder location (e.g. 9-5):",
			"Enter bananas location (e.g. 2-2):",
			"Enter bananas surface (ceiling, couch, ladder, or floor):",
			"Planning action sequence for monkey...",
			"jump onto the floor",
			"walk to 9-5",
			"grab ladder",
			"walk to 2-2",
			"drop ladder",
			"climb onto ladder",
			"grab bananas",
			"SUCCESS: The monkey has grabbed the bananas!",
		}

		for _, p := range expectedPrompts {
			if !strings.Contains(out, p) {
				t.Errorf("expected output to contain %q, but got:\n%s", p, out)
			}
		}
	})
}

func TestSubstrFunctionComprehensive(t *testing.T) {
	program := `
	(literalize string sequence)
	(vector-attribute sequence)
	(literalize person first-name last-name age)
	(literalize log val)

	(p test-element-variable
	   <str> (string ^sequence <first> <second>)
	   -->
	   (bind <head> (substr <str> sequence sequence))
	   (bind <next> (compute (litval sequence) + 1))
	   (bind <tail> (substr <str> <next> inf))
	   (make log ^val <head>)
	   (modify <str> ^sequence <tail>)
	)

	(p test-scalar-attribute
	   <p> (person ^first-name <f> ^last-name <l> ^age <body>)
	   -->
	   (bind <fn> (substr <p> first-name first-name))
	   (bind <ln> (substr <p> last-name last-name))
	   (bind <ag> (substr <p> age age))
	   (make log ^val <fn>)
	   (make log ^val <ln>)
	   (make log ^val <ag>)
	   (remove <p>)
	)

	(p test-ce-number
	   (string ^sequence <val>)
	   -->
	   (bind <last-val> (substr 1 sequence sequence))
	   (make log ^val <last-val>)
	   (remove 1)
	)
	`
	p, err := parser.NewParser(program)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	eng := engine.New()
	eng.SetTrace(false)

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("parser error: %v", err)
		}
		if stmt == nil {
			break
		}
		switch stmt.Type {
		case parser.StmtLiteralize:
			eng.DeclareClass(stmt.LiteralizeClass, stmt.LiteralizeAttrs)
		case parser.StmtVectorAttribute:
			for _, va := range stmt.VectorAttrs {
				eng.DeclareVectorAttribute(va)
			}
		case parser.StmtRule:
			eng.AddRule(stmt.Rule)
		}
	}

	// Assert string with sequence X Y Z
	eng.Make("string", map[string]model.Value{
		"sequence": model.NewVector([]model.Value{
			model.NewSymbol("X"),
			model.NewSymbol("Y"),
			model.NewSymbol("Z"),
		}),
	})

	// Assert person
	eng.Make("person", map[string]model.Value{
		"first-name": model.NewSymbol("Alice"),
		"last-name":  model.NewSymbol("Smith"),
		"age":        model.NewInt(25),
	})

	cycles, err := eng.Run(20)
	if err != nil {
		t.Fatalf("engine run error: %v", err)
	}

	t.Logf("completed in %d cycles", cycles)

	// Collect log values
	logs := eng.FindWMEsMatching(model.NewPositiveCE("log"))
	var logVals []string
	for _, l := range logs {
		v, _ := l.Get("val")
		logVals = append(logVals, v.String())
	}

	// We expect:
	// From person: Alice, Smith, 25
	// From string: X, Y, Z
	expectedValues := []string{"Alice", "Smith", "25", "X", "Y", "Z"}
	for _, exp := range expectedValues {
		found := false
		for _, lv := range logVals {
			if lv == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected log value %q not found in %v", exp, logVals)
		}
	}
}





