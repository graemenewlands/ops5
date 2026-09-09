package cli

import (
	"bytes"
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
