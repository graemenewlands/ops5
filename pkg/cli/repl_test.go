package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestREPLInteractiveSession(t *testing.T) {
	commands := `
	(p sample-rule
	   <g> (goal ^status start)
	   -->
	   (modify <g> ^status finished)
	   (write "Rule executed successfully")
	   (halt)
	)
	make goal ^status start
	wm
	cs
	step
	wm
	exit
	`

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()

	if !strings.Contains(output, "Defined rule 'sample-rule'") {
		t.Fatalf("expected rule definition output, got:\n%s", output)
	}
	if !strings.Contains(output, "Asserted: (1: goal ^status start)") {
		t.Fatalf("expected assertion output, got:\n%s", output)
	}
	if !strings.Contains(output, "Conflict Set (1 activations") {
		t.Fatalf("expected conflict set output, got:\n%s", output)
	}
	if !strings.Contains(output, "Rule executed successfully") {
		t.Fatalf("expected write output, got:\n%s", output)
	}
	if !strings.Contains(output, "(2: goal ^status finished)") {
		t.Fatalf("expected modified WME in working memory, got:\n%s", output)
	}
}

func TestREPLStrategySwitch(t *testing.T) {
	commands := `
	strategy mea
	strategy
	strategy lex
	strategy
	exit
	`

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Strategy set to MEA") {
		t.Fatalf("expected MEA strategy confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Strategy set to LEX") {
		t.Fatalf("expected LEX strategy confirmation, got:\n%s", output)
	}
}

func TestREPLVectorAttributeInteractive(t *testing.T) {
	commands := `
	(literalize City name location state country population)
	(vector-attribute location)
	vector-attributes
	schemas City
	make City ^name Boston ^location 42.36 -71.05 ^state MA ^country USA ^population 675000
	(p locate-city
	   (City ^name <c> ^location <lat> <long> ^state MA)
	   -->
	   (write "Located" <c> "at" <lat> <long>)
	   (halt)
	)
	step
	exit
	`

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Declared vector attribute(s): [location]") {
		t.Fatalf("expected vector attribute confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "^location") {
		t.Fatalf("expected ^location in vector-attributes listing, got:\n%s", output)
	}
	if !strings.Contains(output, "vector: [location]") {
		t.Fatalf("expected vector: [location] in schemas output, got:\n%s", output)
	}
	if !strings.Contains(output, "Asserted: (1: City ^country USA ^location 42.36 -71.05 ^name Boston ^population 675000 ^state MA)") {
		t.Fatalf("expected WME assertion with location vector, got:\n%s", output)
	}
	if !strings.Contains(output, "Located Boston at 42.36 -71.05") {
		t.Fatalf("expected rule firing output with extracted coordinates, got:\n%s", output)
	}
}

func TestREPLFileIOCommands(t *testing.T) {
	tmpDir := t.TempDir()
	traceFile := filepath.Join(tmpDir, "RuleTrace.ops")
	inFile := filepath.Join(tmpDir, "data.in")

	commands := fmt.Sprintf(`
	(openfile ruletrace |%s| out)
	(default ruletrace trace)
	(default ruletrace write)
	default
	(closefile ruletrace)
	(openfile inlog |%s| in)
	(default inlog accept)
	default accept
	(default nil accept)
	(closefile inlog)
	exit
	`, traceFile, inFile)

	// Create dummy input file
	if err := os.WriteFile(inFile, []byte("test-data\n"), 0644); err != nil {
		t.Fatalf("failed creating inFile: %v", err)
	}

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	defer repl.Engine().CloseAllFiles()
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Opened file '"+traceFile+"' as ruletrace (out)") {
		t.Fatalf("expected openfile confirmation for ruletrace, got:\n%s", output)
	}
	if !strings.Contains(output, "Default for trace set to 'ruletrace'") {
		t.Fatalf("expected default trace confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Default for write set to 'ruletrace'") {
		t.Fatalf("expected default write confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Closed file 'ruletrace'") {
		t.Fatalf("expected closefile confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Opened file '"+inFile+"' as inlog (in)") {
		t.Fatalf("expected openfile inlog confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Default for accept set to 'inlog'") {
		t.Fatalf("expected default accept confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Default for accept set to 'nil'") {
		t.Fatalf("expected default nil accept confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Closed file 'inlog'") {
		t.Fatalf("expected closefile inlog confirmation, got:\n%s", output)
	}
}

func TestREPLGenatom(t *testing.T) {
	commands := `
	genatom
	(genatom)
	(make item ^id (genatom))
	wm item
	reset
	genatom
	exit
	`

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "atom1\n") {
		t.Fatalf("expected first atom to be atom1, got:\n%s", output)
	}
	if !strings.Contains(output, "atom2\n") {
		t.Fatalf("expected second atom to be atom2, got:\n%s", output)
	}
	if !strings.Contains(output, "(1: item ^id atom3)") {
		t.Fatalf("expected item with id atom3 asserted, got:\n%s", output)
	}
	if !strings.Contains(output, "Working memory, conflict set, and genatom counter reset.") {
		t.Fatalf("expected reset message, got:\n%s", output)
	}
}

func TestREPLLitval(t *testing.T) {
	// User's exact scenario:
	// (make City ^name Albuquerque ^state NM)
	// (litval name) will evaluate to the integer 2
	input := `(make City ^name Albuquerque ^state NM)
(litval name)
(litval state)
litval name
(litval City name)
litval City state
(literalize Point x y)
(litval x)
(litval y)
exit
`
	in := bytes.NewBufferString(input)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	lines := strings.Split(output, "\n")

	// Find the outputs
	var numericOutputs []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		trimmed = strings.TrimPrefix(trimmed, "ops5> ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "2" || trimmed == "3" {
			numericOutputs = append(numericOutputs, trimmed)
		}
	}

	expected := []string{"2", "3", "2", "2", "3", "2", "3"}
	if len(numericOutputs) != len(expected) {
		t.Fatalf("expected %d numeric outputs %v, got %d: %v\nFull output:\n%s", len(expected), expected, len(numericOutputs), numericOutputs, output)
	}
	for i, exp := range expected {
		if numericOutputs[i] != exp {
			t.Errorf("output %d: expected %s, got %s", i, exp, numericOutputs[i])
		}
	}
}

func TestREPLQuiescenceFormattingOnOwnLine(t *testing.T) {
	commands := `
	(p test-write-no-trailing-newline
		(start)
		-->
		(write (crlf) hello world)
	)
	make start
	run
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "hello world\nReached quiescence after 1 cycles.\n") {
		t.Errorf("expected quiescence message on its own line after output, got:\n%s", output)
	}
}

func TestREPLCommentLines(t *testing.T) {
	commands := `
	; This is a comment at start
	;; Another comment style
	(p test-comment-rule
		(goal ^status active) ; inline comment in rule
		-->
		(write (crlf) "Rule fired")
	)
	; Comment before make
	make goal ^status active
	; Comment before run
	run
	; Comment before exit
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if strings.Contains(output, "Unknown command: ;") {
		t.Errorf("REPL should not produce 'Unknown command: ;' for comment lines, got:\n%s", output)
	}
	if !strings.Contains(output, "Rule fired") {
		t.Errorf("expected rule to fire, got:\n%s", output)
	}
}

func TestREPLRunCycleLimit(t *testing.T) {
	commands := `
	(p count-up
		<c> (Counter ^val <v>)
		-->
		(bind <next> (compute <v> + 1))
		(modify <c> ^val <next>)
		(write (crlf) Step <next>)
	)
	make Counter ^val 0
	run 2
	(run 2)
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Step 1") || !strings.Contains(output, "Step 2") {
		t.Errorf("expected steps 1 and 2 from first 'run 2', got:\n%s", output)
	}
	if !strings.Contains(output, "Step 3") || !strings.Contains(output, "Step 4") {
		t.Errorf("expected steps 3 and 4 from second '(run 2)', got:\n%s", output)
	}
	if strings.Contains(output, "Step 5") {
		t.Errorf("did not expect step 5 to run, got:\n%s", output)
	}
	if repl.Engine().CycleCount() != 4 {
		t.Errorf("expected exactly 4 cycles executed, got %d", repl.Engine().CycleCount())
	}
}

func TestREPLExciseCommand(t *testing.T) {
	commands := `
	(p rule-alpha
		(item ^val 1)
		-->
		(write (crlf) "Alpha fired")
	)
	(p rule-beta
		(item ^val 1)
		-->
		(write (crlf) "Beta fired")
	)
	(p rule-gamma
		(item ^val 1)
		-->
		(write (crlf) "Gamma fired")
	)
	excise rule-alpha
	(excise rule-beta)
	excise nonexistent-rule
	excise
	(excise)
	make item ^val 1
	run
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Excised rule 'rule-alpha'") {
		t.Errorf("expected excise confirmation for rule-alpha, got:\n%s", output)
	}
	if !strings.Contains(output, "Excised rule 'rule-beta'") {
		t.Errorf("expected excise confirmation for rule-beta, got:\n%s", output)
	}
	if !strings.Contains(output, "Rule 'nonexistent-rule' not found") {
		t.Errorf("expected not found warning for nonexistent-rule, got:\n%s", output)
	}
	if !strings.Contains(output, "Usage: excise <rule-name>") {
		t.Errorf("expected usage message for bare excise without args, got:\n%s", output)
	}
	if strings.Contains(output, "Alpha fired") {
		t.Errorf("excised rule-alpha should not have fired, got:\n%s", output)
	}
	if strings.Contains(output, "Beta fired") {
		t.Errorf("excised rule-beta should not have fired, got:\n%s", output)
	}
	if !strings.Contains(output, "Gamma fired") {
		t.Errorf("un-excised rule-gamma should have fired, got:\n%s", output)
	}
	if repl.Engine().Rule("rule-alpha") != nil {
		t.Errorf("rule-alpha should be removed from engine")
	}
	if repl.Engine().Rule("rule-beta") != nil {
		t.Errorf("rule-beta should be removed from engine")
	}
	if repl.Engine().Rule("rule-gamma") == nil {
		t.Errorf("rule-gamma should still be present in engine")
	}
}






