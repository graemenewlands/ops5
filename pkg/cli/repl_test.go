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

