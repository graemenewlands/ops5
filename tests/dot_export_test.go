package tests

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/cli"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/harness"
	"github.com/graemenewlands/ops5/pkg/parser"
)

func TestEngineExportDOTComprehensive(t *testing.T) {
	eng := engine.New()

	script := `
(literalize sensor id temp status)
(literalize reading sensor-id value)
(literalize command action target)

(p alert-high-temp [salience 500]
   (sensor ^id <sid> ^temp > 100)
   (reading ^sensor-id <sid> ^value > 200)
   -(command ^target <sid>)
   -->
   (write "High temp alert for sensor" <sid>)
)

(p normal-ops
   (sensor ^id <sid> ^status active)
   -->
   (write "Sensor" <sid> "active")
)
`
	p, err := parser.NewParser(script)
	if err != nil {
		t.Fatalf("parser init error: %v", err)
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
		case parser.StmtLiteralize:
			eng.DeclareClass(stmt.LiteralizeClass, stmt.LiteralizeAttrs)
		case parser.StmtRule:
			eng.AddRule(stmt.Rule)
		}
	}

	var buf bytes.Buffer
	if err := eng.ExportDOT(&buf); err != nil {
		t.Fatalf("ExportDOT error: %v", err)
	}

	dot := buf.String()

	// 1. Verify clusters
	if !strings.Contains(dot, "subgraph cluster_alpha") {
		t.Errorf("expected cluster_alpha in DOT output")
	}
	if !strings.Contains(dot, "subgraph cluster_beta") {
		t.Errorf("expected cluster_beta in DOT output")
	}

	// 2. Verify Alpha Nodes
	if !strings.Contains(dot, "alpha_root [label=\"Alpha Root\"") {
		t.Errorf("expected alpha_root node")
	}
	if !strings.Contains(dot, "type_sensor") || !strings.Contains(dot, "type_reading") || !strings.Contains(dot, "type_command") {
		t.Errorf("expected type nodes for sensor, reading, command")
	}
	if !strings.Contains(dot, "^temp > 100") {
		t.Errorf("expected constant test ^temp > 100")
	}

	// 3. Verify Beta Nodes
	if !strings.Contains(dot, "Join: (reading)") {
		t.Errorf("expected Join: (reading)")
	}
	if !strings.Contains(dot, "NegativeJoin: -(command)") {
		t.Errorf("expected NegativeJoin: -(command)")
	}
	if !strings.Contains(dot, "term_alert_high_temp") {
		t.Errorf("expected term_alert_high_temp")
	}
	if !strings.Contains(dot, "[salience: 500]") {
		t.Errorf("expected salience: 500 on terminal node")
	}
	if !strings.Contains(dot, "term_normal_ops") {
		t.Errorf("expected term_normal_ops")
	}

	// 4. Verify Edges
	if !strings.Contains(dot, "[label=\"right\", style=dashed") {
		t.Errorf("expected dashed right activation edges")
	}
	if !strings.Contains(dot, "[label=\"right (neg)\", style=dashed") {
		t.Errorf("expected dashed negative right activation edges")
	}
	if !strings.Contains(dot, "[label=\"activate\", style=solid") {
		t.Errorf("expected solid terminal activate edges")
	}
}

func TestHarnessRunnerDOTStatement(t *testing.T) {
	tmpDir := t.TempDir()
	dotFile := filepath.Join(tmpDir, "harness_export.dot")
	tcFile := filepath.Join(tmpDir, "testcase.json")

	tcJSON := fmt.Sprintf(`{
		"name": "test-dot-harness",
		"source": "(p check-val (val ^x 1) --> (write ok))\n(dot \"%s\")",
		"initial_wm": [
			{"class": "val", "attributes": {"x": "1"}}
		],
		"expected_outputs": ["ok"],
		"max_cycles": 10
	}`, strings.ReplaceAll(dotFile, `\`, `\\`))

	if err := os.WriteFile(tcFile, []byte(tcJSON), 0644); err != nil {
		t.Fatalf("failed to write tcFile: %v", err)
	}

	runner := harness.NewRunner()
	tc, err := runner.LoadTestCaseFromJSON(tcFile)
	if err != nil {
		t.Fatalf("LoadTestCaseFromJSON error: %v", err)
	}

	res := runner.Run(tc)
	if !res.Passed {
		t.Fatalf("test case failed: %v", res.Error)
	}

	content, err := os.ReadFile(dotFile)
	if err != nil {
		t.Fatalf("failed to read exported DOT file: %v", err)
	}
	if !strings.Contains(string(content), "digraph ReteNetwork") || !strings.Contains(string(content), "check-val") {
		t.Fatalf("invalid DOT file generated: %s", string(content))
	}
}

func TestREPLLoadFileWithDOTStatement(t *testing.T) {
	tmpDir := t.TempDir()
	dotFile := filepath.Join(tmpDir, "file_export.dot")
	opsFile := filepath.Join(tmpDir, "rules.ops")

	opsContent := fmt.Sprintf(`
(p rule-1
   (task ^status done)
   -->
   (write "completed")
)
(dot "%s")
`, strings.ReplaceAll(dotFile, `\`, `\\`))

	if err := os.WriteFile(opsFile, []byte(opsContent), 0644); err != nil {
		t.Fatalf("failed to write ops file: %v", err)
	}

	var out bytes.Buffer
	repl := cli.NewREPL(strings.NewReader("exit\n"), &out)
	if err := repl.LoadFile(opsFile); err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	content, err := os.ReadFile(dotFile)
	if err != nil {
		t.Fatalf("failed to read exported DOT file: %v", err)
	}
	if !strings.Contains(string(content), "digraph ReteNetwork") || !strings.Contains(string(content), "rule-1") {
		t.Fatalf("invalid DOT content: %s", string(content))
	}
}

func TestCLIDOTFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dotFile := filepath.Join(tmpDir, "cli_export.dot")
	opsFile := filepath.Join(tmpDir, "sample.ops")

	opsContent := `
(literalize person name age)
(p greet-adult
   (person ^name <n> ^age >= 18)
   -->
   (write "Hello" <n>)
)
`
	if err := os.WriteFile(opsFile, []byte(opsContent), 0644); err != nil {
		t.Fatalf("failed to write ops file: %v", err)
	}

	cmd := exec.Command("go", "run", "../cmd/ops5", "--dot", dotFile, opsFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI command failed: %v\nOutput: %s", err, string(out))
	}

	content, err := os.ReadFile(dotFile)
	if err != nil {
		t.Fatalf("failed to read dotFile created by CLI: %v", err)
	}

	dotStr := string(content)
	if !strings.Contains(dotStr, "digraph ReteNetwork") {
		t.Errorf("missing digraph header in CLI DOT export")
	}
	if !strings.Contains(dotStr, "type_person") {
		t.Errorf("missing type_person in CLI DOT export")
	}
	if !strings.Contains(dotStr, "greet-adult") {
		t.Errorf("missing greet-adult in CLI DOT export")
	}
	if !strings.Contains(dotStr, "^age >= 18") {
		t.Errorf("missing ^age >= 18 constant test in CLI DOT export")
	}
}
