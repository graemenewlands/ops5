package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

func TestRuleSalienceEndToEnd(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	script := `
(p normal-rule
	(task ^status pending)
-->
	(write "Fired normal rule (salience 0)" (crlf))
)

(p emergency-rule [salience 1000]
	(task ^status pending)
-->
	(write "Fired emergency rule (salience 1000)" (crlf))
)

(p cleanup-rule [salience -100]
	(task ^status pending)
-->
	(write "Fired cleanup rule (salience -100)" (crlf))
)
`
	p, err := parser.NewParser(script)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("parser error: %v", err)
		}
		if stmt == nil {
			break
		}
		if stmt.Type == parser.StmtRule {
			eng.AddRule(stmt.Rule)
		}
	}

	// Assert task
	eng.Make("task", map[string]model.Value{
		"status": model.NewSymbol("pending"),
	})

	// Check conflict set
	acts := eng.ConflictSet().All()
	if len(acts) != 3 {
		t.Fatalf("expected 3 activations in conflict set, got %d", len(acts))
	}

	if acts[0].Rule.Name != "emergency-rule" || acts[0].Salience() != 1000 {
		t.Errorf("expected 1st activation to be emergency-rule (1000), got %s (%d)", acts[0].Rule.Name, acts[0].Salience())
	}
	if acts[1].Rule.Name != "normal-rule" || acts[1].Salience() != 0 {
		t.Errorf("expected 2nd activation to be normal-rule (0), got %s (%d)", acts[1].Rule.Name, acts[1].Salience())
	}
	if acts[2].Rule.Name != "cleanup-rule" || acts[2].Salience() != -100 {
		t.Errorf("expected 3rd activation to be cleanup-rule (-100), got %s (%d)", acts[2].Rule.Name, acts[2].Salience())
	}

	// Run 3 cycles
	cycles, err := eng.Run(3)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if cycles != 3 {
		t.Fatalf("expected 3 cycles, got %d", cycles)
	}

	output := out.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d:\n%s", len(lines), output)
	}

	if !strings.Contains(lines[0], "emergency rule") {
		t.Errorf("line 0 should be emergency rule, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "normal rule") {
		t.Errorf("line 1 should be normal rule, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "cleanup rule") {
		t.Errorf("line 2 should be cleanup rule, got: %s", lines[2])
	}
}

func TestSalienceWithMEAStrategy(t *testing.T) {
	eng := engine.New()
	eng.ConflictSet().SetStrategy(conflict.StrategyMEA)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// In MEA:
	// Rule 1 has salience 500, but its CE1 matches an older WME (timetag 1).
	// Rule 2 has salience 0, but its CE1 matches a newer WME (timetag 2).
	// With salience, Rule 1 must dominate despite older CE1 timetag!
	script := `
(p rule-with-older-ce1 [salience 500]
	(goal ^id old)
	(data ^val <v>)
-->
	(write "Rule with older CE1 fired first due to salience 500" (crlf))
	(halt)
)

(p rule-with-newer-ce1 [salience 0]
	(goal ^id new)
	(data ^val <v>)
-->
	(write "Rule with newer CE1 fired" (crlf))
	(halt)
)
`
	p, _ := parser.NewParser(script)
	for {
		stmt, _ := p.NextStatement()
		if stmt == nil {
			break
		}
		if stmt.Type == parser.StmtRule {
			eng.AddRule(stmt.Rule)
		}
	}

	// Assert older goal (timetag 1)
	eng.Make("goal", map[string]model.Value{"id": model.NewSymbol("old")})
	// Assert newer goal (timetag 2)
	eng.Make("goal", map[string]model.Value{"id": model.NewSymbol("new")})
	// Assert shared data (timetag 3)
	eng.Make("data", map[string]model.Value{"val": model.NewInt(42)})

	dom, ok := eng.ConflictSet().SelectDominant()
	if !ok || dom.Rule.Name != "rule-with-older-ce1" {
		t.Fatalf("expected dominant rule to be rule-with-older-ce1 under MEA, got %v", dom)
	}

	cycles, _ := eng.Run(5)
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}
	if !strings.Contains(out.String(), "older CE1 fired first") {
		t.Fatalf("expected rule-with-older-ce1 output, got:\n%s", out.String())
	}
}

func TestSalienceWithLEXStrategy(t *testing.T) {
	eng := engine.New()
	eng.ConflictSet().SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Under LEX:
	// Rule 1 has salience 50, but older timetags [1, 2] and lower specificity (2 tests).
	// Rule 2 has salience 0, newer timetags [10, 20] and higher specificity (4 tests).
	// Salience 50 must dominate.
	script := `
(p rule-salient [salience 50]
	(order ^id 1)
	(item ^val 10)
-->
	(write "Salient rule fired" (crlf))
	(halt)
)

(p rule-recent-specific [salience 0]
	(order ^id 2 ^status active)
	(item ^val 20 ^cost 5)
-->
	(write "Recent rule fired" (crlf))
	(halt)
)
`
	p, _ := parser.NewParser(script)
	for {
		stmt, _ := p.NextStatement()
		if stmt == nil {
			break
		}
		if stmt.Type == parser.StmtRule {
			eng.AddRule(stmt.Rule)
		}
	}

	// Older items for rule-salient
	eng.Make("order", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("item", map[string]model.Value{"val": model.NewInt(10)})

	// Newer items for rule-recent-specific
	eng.Make("order", map[string]model.Value{
		"id":     model.NewInt(2),
		"status": model.NewSymbol("active"),
	})
	eng.Make("item", map[string]model.Value{
		"val":  model.NewInt(20),
		"cost": model.NewInt(5),
	})

	dom, ok := eng.ConflictSet().SelectDominant()
	if !ok || dom.Rule.Name != "rule-salient" {
		t.Fatalf("expected dominant rule to be rule-salient under LEX, got %v", dom)
	}

	eng.Run(5)
	if !strings.Contains(out.String(), "Salient rule fired") {
		t.Fatalf("expected rule-salient output, got:\n%s", out.String())
	}
}

func TestSalienceParenAndDeclareSyntax(t *testing.T) {
	eng := engine.New()

	script := `
(p rule-paren (salience 200)
	(flag ^val 1)
-->
	(halt)
)

(p rule-declare (declare (salience 300))
	(flag ^val 2)
-->
	(halt)
)
`
	p, err := parser.NewParser(script)
	if err != nil {
		t.Fatalf("parser creation failed: %v", err)
	}
	r1, err := p.ParseRule()
	if err != nil || r1.Salience != 200 {
		t.Fatalf("expected salience 200 for (salience 200), got %d (err: %v)", r1.Salience, err)
	}
	r2, err := p.ParseRule()
	if err != nil || r2.Salience != 300 {
		t.Fatalf("expected salience 300 for (declare (salience 300)), got %d (err: %v)", r2.Salience, err)
	}

	eng.AddRule(r1)
	eng.AddRule(r2)

	eng.Make("flag", map[string]model.Value{"val": model.NewInt(1)})
	eng.Make("flag", map[string]model.Value{"val": model.NewInt(2)})

	acts := eng.ConflictSet().All()
	if len(acts) != 2 {
		t.Fatalf("expected 2 activations, got %d", len(acts))
	}
	if acts[0].Rule.Name != "rule-declare" || acts[0].Salience() != 300 {
		t.Errorf("expected 1st activation to be rule-declare (300), got %s", acts[0].Rule.Name)
	}
	if acts[1].Rule.Name != "rule-paren" || acts[1].Salience() != 200 {
		t.Errorf("expected 2nd activation to be rule-paren (200), got %s", acts[1].Rule.Name)
	}
}

func TestSalienceTieBreaking(t *testing.T) {
	eng := engine.New()

	// Both rules have salience 100.
	// Rule 1 matches older WME [1].
	// Rule 2 matches newer WME [2].
	// Because salience is equal, standard recency (WME 2) breaks the tie!
	script := `
(p r1 [salience 100]
	(sensor ^id 1)
-->
	(halt)
)

(p r2 [salience 100]
	(sensor ^id 2)
-->
	(halt)
)
`
	p, _ := parser.NewParser(script)
	for {
		stmt, _ := p.NextStatement()
		if stmt == nil {
			break
		}
		if stmt.Type == parser.StmtRule {
			eng.AddRule(stmt.Rule)
		}
	}

	eng.Make("sensor", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("sensor", map[string]model.Value{"id": model.NewInt(2)})

	dom, ok := eng.ConflictSet().SelectDominant()
	if !ok || dom.Rule.Name != "r2" {
		t.Fatalf("expected r2 to dominate r1 on equal salience due to recency, got %v", dom)
	}
}
