package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ops5/pkg/engine"
	"ops5/pkg/model"
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

func TestREPLPMCommand(t *testing.T) {
	commands := `
	(p FindAncestors
		(Request ^type ancestor ^target <p>)
		(Person ^name <p> ^father <f>)
		-->
		(make Request ^type ancestor ^target <f>)
		(write (crlf) <f> "is an ancestor")
	)
	(p DetectLeaf
		(Person ^name <n>)
		-(Person ^father <n>)
		-->
		(write (crlf) <n> "is a leaf")
	)
	pm FindAncestors
	(pm DetectLeaf)
	pm *
	(pm *)
	pm unknown-rule
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "(p FindAncestors") {
		t.Errorf("expected FindAncestors rule text in output, got:\n%s", output)
	}
	if !strings.Contains(output, "(p DetectLeaf") {
		t.Errorf("expected DetectLeaf rule text in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Rule 'unknown-rule' not found") {
		t.Errorf("expected unknown-rule not found warning, got:\n%s", output)
	}
	if !strings.Contains(output, "(make Request ^target <f> ^type ancestor)") {
		t.Errorf("expected formatted make action in pm output, got:\n%s", output)
	}
	if !strings.Contains(output, `(write (crlf) <f> "is an ancestor")`) {
		t.Errorf("expected formatted write action in pm output, got:\n%s", output)
	}

	// Test empty rules in memory
	emptyIn := strings.NewReader("pm\npm *\nexit\n")
	var emptyOut bytes.Buffer
	emptyRepl := NewREPL(emptyIn, &emptyOut)
	emptyRepl.Start()
	emptyOutput := emptyOut.String()
	if !strings.Contains(emptyOutput, "No production rules in memory.") {
		t.Errorf("expected 'No production rules in memory.' message, got:\n%s", emptyOutput)
	}
}

func TestREPLRemoveWildcard(t *testing.T) {
	commands := `
	make item ^id 1
	make item ^id 2
	make item ^id 3
	wm
	(remove *)
	wm
	make item ^id 4
	make item ^id 5
	remove *
	wm
	(remove *)
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Working Memory (3 elements):") {
		t.Errorf("expected 3 WMEs initially, got:\n%s", output)
	}
	if !strings.Contains(output, "Removed all 3 WMEs from working memory.") {
		t.Errorf("expected confirmation for (remove *), got:\n%s", output)
	}
	if !strings.Contains(output, "Removed all 2 WMEs from working memory.") {
		t.Errorf("expected confirmation for remove *, got:\n%s", output)
	}
	if !strings.Contains(output, "Working memory is already empty.") {
		t.Errorf("expected empty working memory message for repeated (remove *), got:\n%s", output)
	}
	if repl.Engine().WorkingMemory().Count() != 0 {
		t.Errorf("expected working memory to have 0 elements, got %d", repl.Engine().WorkingMemory().Count())
	}
}

func TestREPLWatchCommand(t *testing.T) {
	commands := `
	watch
	(watch)
	watch 3
	(watch 9)
	watch 1 2
	(watch 2)
	watch
	(p test-rule
		<g> (goal ^status active)
		-->
		(modify <g> ^status done)
	)
	make goal ^status active
	step
	watch 1
	(p test-rule-2
		<g> (goal ^status done)
		-->
		(modify <g> ^status finished)
	)
	step
	watch 0
	(p test-rule-3
		<g> (goal ^status finished)
		-->
		(remove <g>)
	)
	step
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Current watch level: 1") {
		t.Errorf("expected initial watch level 1, got:\n%s", output)
	}
	if !strings.Contains(output, "Invalid watch level: 3 (expected 0, 1, or 2)") {
		t.Errorf("expected invalid watch level message for 3, got:\n%s", output)
	}
	if !strings.Contains(output, "Invalid watch level: 9 (expected 0, 1, or 2)") {
		t.Errorf("expected invalid watch level message for 9, got:\n%s", output)
	}
	if !strings.Contains(output, "Usage: watch [0|1|2]") {
		t.Errorf("expected usage message for watch 1 2, got:\n%s", output)
	}
	if !strings.Contains(output, "Watch level set to 2") {
		t.Errorf("expected watch level set to 2, got:\n%s", output)
	}
	if !strings.Contains(output, "Current watch level: 2") {
		t.Errorf("expected current watch level 2 after update, got:\n%s", output)
	}
	// Under watch 2: should see =>WM: on make, Fired rule on step, and <=WM: and =>WM: on modify
	if !strings.Contains(output, "=>WM: (1: goal ^status active)") {
		t.Errorf("expected =>WM: assertion trace for make under watch 2, got:\n%s", output)
	}
	if !strings.Contains(output, "Fired rule 'test-rule'") {
		t.Errorf("expected firing trace for test-rule, got:\n%s", output)
	}
	if !strings.Contains(output, "<=WM: (1: goal ^status active)") {
		t.Errorf("expected <=WM: retraction trace on modify under watch 2, got:\n%s", output)
	}
	if !strings.Contains(output, "=>WM: (2: goal ^status done)") {
		t.Errorf("expected =>WM: assertion trace on modify under watch 2, got:\n%s", output)
	}

	// Under watch 1: should see firing trace, but NOT <=WM: or =>WM: for test-rule-2
	if !strings.Contains(output, "Watch level set to 1") {
		t.Errorf("expected watch level set to 1, got:\n%s", output)
	}
	if !strings.Contains(output, "Fired rule 'test-rule-2'") {
		t.Errorf("expected firing trace for test-rule-2 under watch 1, got:\n%s", output)
	}
	if strings.Contains(output, "=>WM: (3: goal ^status finished)") {
		t.Errorf("did not expect =>WM: under watch 1, got:\n%s", output)
	}

	// Under watch 0: should NOT see firing trace or WM traces for test-rule-3
	if !strings.Contains(output, "Watch level set to 0") {
		t.Errorf("expected watch level set to 0, got:\n%s", output)
	}
	if strings.Contains(output, "Fired rule 'test-rule-3'") {
		t.Errorf("did not expect firing trace for test-rule-3 under watch 0, got:\n%s", output)
	}
	if strings.Contains(output, "<=WM: (3: goal ^status finished)") {
		t.Errorf("did not expect <=WM: under watch 0, got:\n%s", output)
	}
}

func TestREPLPPWMCommand(t *testing.T) {
	commands := `
	(literalize City name state population)
	make City ^name Pittsburgh ^state Pennsylvania ^population 300000
	make City ^name Philadelphia ^state Pennsylvania ^population 1500000
	make City ^name Boston ^state Massachusetts ^population 675000
	make Person ^name Franklin ^state Pennsylvania
	(ppwm City ^state Pennsylvania)
	ppwm City ^state Pennsylvania
	(ppwm (City ^state Pennsylvania))
	(ppwm City ^name Boston)
	ppwm Person
	ppwm City ^state Ohio
	(ppwm City ^state <x>)
	(ppwm City ^population > 100000)
	(ppwm City ^state //)
	(ppwm City ^state { PA NY })
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()

	// Verify matching elements
	if !strings.Contains(output, "(1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)") {
		t.Errorf("expected Pittsburgh in ppwm output, got:\n%s", output)
	}
	if !strings.Contains(output, "(2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)") {
		t.Errorf("expected Philadelphia in ppwm output, got:\n%s", output)
	}
	if !strings.Contains(output, "(3: City ^name Boston ^population 675000 ^state Massachusetts)") {
		t.Errorf("expected Boston in ppwm output, got:\n%s", output)
	}
	if !strings.Contains(output, "(4: Person ^name Franklin ^state Pennsylvania)") {
		t.Errorf("expected Franklin in ppwm Person output, got:\n%s", output)
	}

	// Verify error reporting for forbidden constructs
	if !strings.Contains(output, "ppwm error: ppwm pattern cannot contain variables (found '<x>')") {
		t.Errorf("expected variable error message, got:\n%s", output)
	}
	if !strings.Contains(output, "ppwm error: ppwm pattern cannot contain predicates (found '>')") {
		t.Errorf("expected predicate error message, got:\n%s", output)
	}
	if !strings.Contains(output, "ppwm error: ppwm pattern cannot contain the quote operator '//'") {
		t.Errorf("expected quote error message, got:\n%s", output)
	}
	if !strings.Contains(output, "ppwm error: ppwm pattern cannot contain curly braces") {
		t.Errorf("expected curly braces error message, got:\n%s", output)
	}
}

func TestREPLLoadFilePPWMDirective(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test_ppwm.ops")
	opsContent := `
	(literalize City name state)
	(make City ^name Pittsburgh ^state Pennsylvania)
	(make City ^name Boston ^state Massachusetts)
	(ppwm City ^state Pennsylvania)
	`
	if err := os.WriteFile(filePath, []byte(opsContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	commands := fmt.Sprintf("load %s\nexit\n", filePath)
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "(1: City ^name Pittsburgh ^state Pennsylvania)") {
		t.Errorf("expected Pittsburgh from (ppwm ...) directive during load, got:\n%s", output)
	}
	if strings.Contains(output, "Boston") {
		t.Errorf("did not expect Boston in (ppwm City ^state Pennsylvania) output, got:\n%s", output)
	}
}

func TestREPLSubstrInteractive(t *testing.T) {
	commands := `
	(literalize string sequence)
	(vector-attribute sequence)
	make string ^sequence A B C D
	(substr 1 sequence sequence)
	substr 1 2 4
	(substr 1 3 inf)
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	// (substr 1 sequence sequence) should print scalar A
	if !strings.Contains(output, "ops5> A\n") && !strings.Contains(output, "\nA\n") {
		t.Errorf("expected scalar 'A' output for substr 1 sequence sequence, got:\n%s", output)
	}
	// substr 1 2 4 should print A B C
	if !strings.Contains(output, "A B C") {
		t.Errorf("expected 'A B C' output for substr 1 2 4, got:\n%s", output)
	}
	// (substr 1 3 inf) should print B C D
	if !strings.Contains(output, "B C D") {
		t.Errorf("expected 'B C D' output for substr 1 3 inf, got:\n%s", output)
	}
}

func TestREPLLoadFileSubstrDirective(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test_substr.ops")
	opsContent := `
	(literalize string sequence)
	(vector-attribute sequence)
	(make string ^sequence alpha beta gamma delta)
	(substr 1 sequence sequence)
	(substr 1 3 inf)
	`
	if err := os.WriteFile(filePath, []byte(opsContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	commands := fmt.Sprintf("load %s\nexit\n", filePath)
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "alpha") {
		t.Errorf("expected alpha from (substr 1 sequence sequence), got:\n%s", output)
	}
	if !strings.Contains(output, "beta gamma delta") {
		t.Errorf("expected 'beta gamma delta' from (substr 1 3 inf), got:\n%s", output)
	}
}

func TestREPLTableMode(t *testing.T) {
	commands := `
	(literalize Person name age occupation)
	make Person ^name Alice ^age 30 ^occupation Engineer
	make Person ^name Bob ^age 25 ^occupation Designer
	wm --table
	table on
	wm
	table off
	wm -t Person
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
		t.Fatalf("expected WMEs in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Timetag") || !strings.Contains(output, "Class") || !strings.Contains(output, "Attributes") {
		t.Fatalf("expected table headers in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Table mode enabled") || !strings.Contains(output, "Table mode disabled") {
		t.Fatalf("expected table mode status messages, got:\n%s", output)
	}
}

func TestREPLConflictSetTable(t *testing.T) {
	commands := `
	(p rule-1
	   (goal ^status active)
	   -->
	   (write "rule 1")
	)
	(p rule-2
	   (goal ^status active)
	   -->
	   (write "rule 2")
	)
	make goal ^status active
	cs --table
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Conflict Set (2 activations") {
		t.Fatalf("expected 2 activations, got:\n%s", output)
	}
	if !strings.Contains(output, "rule-1") || !strings.Contains(output, "rule-2") {
		t.Fatalf("expected rules in table, got:\n%s", output)
	}
	if !strings.Contains(output, "Specificity") || !strings.Contains(output, "Timetags") {
		t.Fatalf("expected conflict set table columns, got:\n%s", output)
	}
}

func TestREPLSchemasTable(t *testing.T) {
	commands := `
	(literalize inventory id item location)
	(vector-attribute location)
	schemas --table
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "Class Schemas:") {
		t.Fatalf("expected class schemas header, got:\n%s", output)
	}
	if !strings.Contains(output, "inventory") || !strings.Contains(output, "Vector Attributes") {
		t.Fatalf("expected schemas table content, got:\n%s", output)
	}
}

func TestREPLStatusCommand(t *testing.T) {
	commands := `
	(literalize Task id description)
	(p sample-rule
	   (Task ^id <id>)
	   -->
	   (write <id>)
	)
	make Task ^id 101 ^description "Do work"
	status
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "OPS5 Runtime Status") {
		t.Fatalf("expected status header, got:\n%s", output)
	}
	if !strings.Contains(output, "Strategy:") || !strings.Contains(output, "Production Rules:  1") {
		t.Fatalf("expected status details, got:\n%s", output)
	}
	if !strings.Contains(output, "Working Memory:    1 WMEs") {
		t.Fatalf("expected WME count in status, got:\n%s", output)
	}
	if !strings.Contains(output, "Conflict Set:      1 activations") {
		t.Fatalf("expected activation count in status, got:\n%s", output)
	}
}

func TestREPLClearCommand(t *testing.T) {
	commands := "clear\nexit\n"
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()
	if !strings.Contains(output, "\033[H\033[2J") {
		t.Fatalf("expected clear escape sequence, got:\n%s", output)
	}
}

func TestStylerColor(t *testing.T) {
	styler := &Styler{Enabled: true}
	prompt := styler.Prompt()
	if !strings.Contains(prompt, "\033[") {
		t.Errorf("expected ANSI sequence in prompt, got: %q", prompt)
	}

	w := styler.Wrap(ansiBold, "test")
	if w != "\033[1mtest\033[0m" {
		t.Errorf("unexpected wrap output: %q", w)
	}
	if StripANSI(w) != "test" {
		t.Errorf("expected 'test', got %q", StripANSI(w))
	}
	if VisualWidth(w) != 4 {
		t.Errorf("expected visual width 4, got %d", VisualWidth(w))
	}

	styler.Enabled = false
	if styler.Wrap(ansiBold, "test") != "test" {
		t.Errorf("expected plain text when disabled")
	}
	if styler.Prompt() != "ops5> " {
		t.Errorf("expected plain prompt when disabled, got %q", styler.Prompt())
	}
}

func TestCompleter(t *testing.T) {
	eng := engine.New()
	eng.DeclareClass("goal", []string{"status", "priority"})
	eng.DeclareClass("person", []string{"name", "age"})
	eng.AddRule(&model.Rule{Name: "find-person"})

	completer := NewCompleter(eng)

	// Command completion
	candidates, prefix := completer.Complete("st")
	if prefix != "st" {
		t.Errorf("expected prefix 'st', got %q", prefix)
	}
	expectedCmds := []string{"status", "step", "strategy"}
	for _, exp := range expectedCmds {
		found := false
		for _, c := range candidates {
			if c == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected candidate %q for 'st', got %v", exp, candidates)
		}
	}

	// Strategy options completion
	candidates, prefix = completer.Complete("strategy m")
	if prefix != "m" || len(candidates) != 1 || candidates[0] != "mea" {
		t.Errorf("expected ['mea'], got %v with prefix %q", candidates, prefix)
	}

	// Class completion
	candidates, prefix = completer.Complete("make p")
	if prefix != "p" || len(candidates) != 1 || candidates[0] != "person" {
		t.Errorf("expected ['person'], got %v with prefix %q", candidates, prefix)
	}

	// Attribute completion with caret
	candidates, prefix = completer.Complete("make person ^a")
	if prefix != "^a" || len(candidates) != 1 || candidates[0] != "^age" {
		t.Errorf("expected ['^age'], got %v with prefix %q", candidates, prefix)
	}

	// Rule completion
	candidates, prefix = completer.Complete("excise find-")
	if prefix != "find-" || len(candidates) != 1 || candidates[0] != "find-person" {
		t.Errorf("expected ['find-person'], got %v with prefix %q", candidates, prefix)
	}
}

func TestHistory(t *testing.T) {
	h := &History{
		entries: make([]string, 0),
		cursor:  0,
		maxSize: 5,
	}

	h.Add("cmd1")
	h.Add("cmd2")
	h.Add("cmd3")
	// Consecutive duplicate should be ignored
	h.Add("cmd3")

	if len(h.Entries()) != 3 {
		t.Errorf("expected 3 entries, got %d", len(h.Entries()))
	}

	// Navigate backwards (Up arrow)
	val, ok := h.Previous()
	if !ok || val != "cmd3" {
		t.Errorf("expected 'cmd3', got %q (ok=%t)", val, ok)
	}

	val, ok = h.Previous()
	if !ok || val != "cmd2" {
		t.Errorf("expected 'cmd2', got %q (ok=%t)", val, ok)
	}

	val, ok = h.Previous()
	if !ok || val != "cmd1" {
		t.Errorf("expected 'cmd1', got %q (ok=%t)", val, ok)
	}

	// Navigate forwards (Down arrow)
	val, ok = h.Next()
	if !ok || val != "cmd2" {
		t.Errorf("expected 'cmd2', got %q (ok=%t)", val, ok)
	}

	val, ok = h.Next()
	if !ok || val != "cmd3" {
		t.Errorf("expected 'cmd3', got %q (ok=%t)", val, ok)
	}

	val, ok = h.Next()
	if ok {
		t.Errorf("expected end of history, got %q", val)
	}
}

func TestREPLMatchesAndPBreak(t *testing.T) {
	commands := `
	(p r1
	   (stage ^val 1)
	   -->
	   (make stage ^val 2)
	)
	(p r2
	   (stage ^val 2)
	   -->
	   (make stage ^val 3)
	   (halt)
	)
	make stage ^val 1
	matches r1
	pbreak r2
	pbreak
	run
	step
	matches r2
	unpbreak *
	pbreak
	exit
	`

	in := strings.NewReader(commands)
	var out bytes.Buffer

	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()

	if !strings.Contains(output, "** Matches for rule 'r1' **") {
		t.Fatalf("expected matches header for r1, got:\n%s", output)
	}
	if !strings.Contains(output, "Breakpoint set on rule 'r2'") {
		t.Fatalf("expected breakpoint confirmation, got:\n%s", output)
	}
	if !strings.Contains(output, "Breakpoints (1):") {
		t.Fatalf("expected breakpoints list, got:\n%s", output)
	}
	if !strings.Contains(output, "** Break on rule 'r2' after 1 cycles **") {
		t.Fatalf("expected break output, got:\n%s", output)
	}
	if !strings.Contains(output, "All rule breakpoints cleared.") {
		t.Fatalf("expected all breakpoints cleared, got:\n%s", output)
	}
	if !strings.Contains(output, "No breakpoints set.") {
		t.Fatalf("expected no breakpoints set confirmation, got:\n%s", output)
	}
}

func TestREPLSalienceFormatting(t *testing.T) {
	commands := `
	(p normal-operation
	   (sensor ^temp <v>)
	   -->
	   (write "normal")
	)
	(p emergency-shutdown [salience 1000]
	   (sensor ^temp <v>)
	   -->
	   (write "emergency")
	)
	make sensor ^temp 750
	cs
	cs --table
	pm
	step
	exit
	`
	in := strings.NewReader(commands)
	var out bytes.Buffer
	repl := NewREPL(in, &out)
	repl.Start()

	output := out.String()

	// Text cs check: dominant marker on emergency-shutdown, salience displayed
	if !strings.Contains(output, "* 1. emergency-shutdown [salience: 1000]") {
		t.Fatalf("expected dominant emergency-shutdown with salience: 1000 in cs, got:\n%s", output)
	}
	if !strings.Contains(output, "2. normal-operation [salience: 0]") {
		t.Fatalf("expected normal-operation with salience: 0 in cs, got:\n%s", output)
	}

	// cs --table check: Salience column header and values
	if !strings.Contains(output, "Salience") {
		t.Fatalf("expected 'Salience' column header in cs --table, got:\n%s", output)
	}
	if !strings.Contains(output, "1000") {
		t.Fatalf("expected salience 1000 in cs --table, got:\n%s", output)
	}

	// pm check: pm prints [salience 1000]
	if !strings.Contains(output, "(p emergency-shutdown [salience 1000]") {
		t.Fatalf("expected pm to print [salience 1000], got:\n%s", output)
	}

	// Execution check: emergency-shutdown fires first on step
	if !strings.Contains(output, "emergency") {
		t.Fatalf("expected emergency-shutdown to fire first, got:\n%s", output)
	}
}












