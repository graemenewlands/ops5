package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ops5/pkg/model"
)

func TestEngineGoalProgression(t *testing.T) {
	eng := New()
	var logBuf bytes.Buffer
	eng.SetOutputWriter(&logBuf)

	// Rule 1: Step 1 -> Step 2
	rule1 := model.NewRule("step-1-to-2")
	ce1 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(1))
	rule1.AddCondition(ce1)
	rule1.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"step": model.NewInt(2),
		},
	})
	rule1.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteValue(model.NewSymbol("TRANSITIONED")), model.WriteValue(model.NewSymbol("TO")), model.WriteValue(model.NewInt(2))},
	})
	eng.AddRule(rule1)

	// Rule 2: Step 2 -> Step 3
	rule2 := model.NewRule("step-2-to-3")
	ce2 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(2))
	rule2.AddCondition(ce2)
	rule2.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"step": model.NewInt(3),
		},
	})
	eng.AddRule(rule2)

	// Rule 3: Step 3 -> Finish & Halt
	rule3 := model.NewRule("finish")
	ce3 := model.NewPositiveCE("goal").
		WithElementVariable("g").
		AddEqualTest("status", model.NewSymbol("active")).
		AddEqualTest("step", model.NewInt(3))
	rule3.AddCondition(ce3)
	rule3.AddAction(model.ModifyAction{
		TargetElementVar: "g",
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("completed"),
		},
	})
	rule3.AddAction(model.HaltAction{})
	eng.AddRule(rule3)

	// Initial WME
	eng.Make("goal", map[string]model.Value{
		"status": model.NewSymbol("active"),
		"step":   model.NewInt(1),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if cycles != 3 {
		t.Fatalf("expected exactly 3 cycles, ran %d", cycles)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to be halted")
	}

	// Working memory should now contain completed goal
	goals := eng.WorkingMemory().FindByClass("goal")
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal in WM, got %d", len(goals))
	}
	status, _ := goals[0].Get("status")
	if !status.Equal(model.NewSymbol("completed")) {
		t.Fatalf("expected status 'completed', got %v", status)
	}

	if !strings.Contains(logBuf.String(), "TRANSITIONED TO 2") {
		t.Fatalf("expected write action output in log buffer, got %q", logBuf.String())
	}
}

func TestEngineNegatedConditionControl(t *testing.T) {
	eng := New()

	// Rule: Process pending task if not blocked
	rule := model.NewRule("process-unblocked-task")
	ce1 := model.NewPositiveCE("task").
		WithElementVariable("t").
		AddEqualTest("status", model.NewSymbol("pending")).
		AddEqualTest("id", model.NewVariable("<id>"))
	ce2 := model.NewNegativeCE("lock").
		AddEqualTest("task-id", model.NewVariable("<id>"))

	rule.AddCondition(ce1).AddCondition(ce2)
	rule.AddAction(model.ModifyAction{
		TargetElementVar: "t",
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("running"),
		},
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	// 1. Assert task and lock -> Should not fire!
	eng.Make("task", map[string]model.Value{
		"id":     model.NewInt(99),
		"status": model.NewSymbol("pending"),
	})
	lock := eng.Make("lock", map[string]model.Value{
		"task-id": model.NewInt(99),
	})

	cycles, _ := eng.Run(10)
	if cycles != 0 {
		t.Fatalf("expected 0 cycles when locked, got %d", cycles)
	}

	// 2. Remove lock -> Negative condition becomes satisfied -> Rule fires!
	eng.Remove(lock.Timetag)

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle after unlocking, got %d", cycles)
	}

	tasks := eng.WorkingMemory().FindByClass("task")
	status, _ := tasks[0].Get("status")
	if !status.Equal(model.NewSymbol("running")) {
		t.Fatalf("expected task status to be 'running', got %v", status)
	}
}

func TestEngineWriteFormattingGrid(t *testing.T) {
	eng := New()
	var buf bytes.Buffer
	eng.SetOutputWriter(&buf)

	// Rule to print a grid table
	r1 := model.NewRule("print-row")
	ce := model.NewPositiveCE("city").
		WithElementVariable("c").
		AddEqualTest("id", model.NewVariable("<id>")).
		AddEqualTest("name", model.NewVariable("<name>")).
		AddEqualTest("pop", model.NewVariable("<pop>"))
	r1.AddCondition(ce)
	r1.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteTabTo(model.NewInt(5)),
			model.WriteValue(model.NewVariable("<id>")),
			model.WriteTabTo(model.NewInt(15)),
			model.WriteValue(model.NewVariable("<name>")),
			model.WriteTabTo(model.NewInt(28)),
			model.WriteValue(model.NewVariable("<pop>")),
			model.WriteCRLF(),
		},
	})
	r1.AddAction(model.RemoveAction{TargetElementVar: "c"})
	eng.AddRule(r1)

	eng.Make("city", map[string]model.Value{
		"id":   model.NewInt(101),
		"name": model.NewString("Boston"),
		"pop":  model.NewInt(675000),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	expected := "    101       Boston       675000\n"
	got := buf.String()
	if got != expected {
		t.Fatalf("grid formatting mismatch:\ngot:\n%q\nwant:\n%q", got, expected)
	}
}

func TestEngineBindAndCompute(t *testing.T) {
	eng := New()
	var buf bytes.Buffer
	eng.SetOutputWriter(&buf)

	r1 := model.NewRule("calculate-total")
	ce := model.NewPositiveCE("order").
		WithElementVariable("o").
		AddEqualTest("price", model.NewVariable("<p>")).
		AddEqualTest("tax-rate", model.NewVariable("<r>"))
	r1.AddCondition(ce)

	// (bind <tax> (compute <p> * <r>))
	r1.AddAction(model.BindAction{
		Variable: "tax",
		Value: model.NewCompute(
			[]model.Value{model.NewVariable("<p>"), model.NewVariable("<r>")},
			[]model.ComputeOp{model.ComputeOpMul},
		),
	})

	// (bind <total> (compute <p> + <tax>))
	r1.AddAction(model.BindAction{
		Variable: "total",
		Value: model.NewCompute(
			[]model.Value{model.NewVariable("<p>"), model.NewVariable("<tax>")},
			[]model.ComputeOp{model.ComputeOpAdd},
		),
	})

	// (make invoice ^price <p> ^tax <tax> ^total <total>)
	r1.AddAction(model.MakeAction{
		Class: "invoice",
		Attributes: map[string]model.Value{
			"price": model.NewVariable("<p>"),
			"tax":   model.NewVariable("<tax>"),
			"total": model.NewVariable("<total>"),
		},
	})

	// (write (crlf) |Total:| <total> (crlf))
	r1.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteCRLF(),
			model.WriteValue(model.NewSymbol("Total:")),
			model.WriteValue(model.NewVariable("<total>")),
			model.WriteCRLF(),
		},
	})

	// (remove <o>)
	r1.AddAction(model.RemoveAction{TargetElementVar: "o"})
	eng.AddRule(r1)

	eng.Make("order", map[string]model.Value{
		"price":    model.NewFloat(100.0),
		"tax-rate": model.NewFloat(0.05),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	expectedOut := "\nTotal: 105\n"
	if buf.String() != expectedOut {
		t.Fatalf("expected output %q, got %q", expectedOut, buf.String())
	}

	invoices := eng.WorkingMemory().FindByClass("invoice")
	if len(invoices) != 1 {
		t.Fatalf("expected 1 invoice WME, got %d", len(invoices))
	}
	taxVal, _ := invoices[0].Get("tax")
	if !taxVal.Equal(model.NewFloat(5.0)) {
		t.Fatalf("expected invoice tax 5.0, got %v", taxVal)
	}
	totalVal, _ := invoices[0].Get("total")
	if !totalVal.Equal(model.NewFloat(105.0)) {
		t.Fatalf("expected invoice total 105.0, got %v", totalVal)
	}
}

func TestEngineCBind(t *testing.T) {
	eng := New()

	// Rule 1: makes person, cbinds <p>, modifies person, cbinds <p2>, makes tracker referencing <p2>
	r1 := model.NewRule("create-and-track")
	r1.AddCondition(model.NewPositiveCE("goal").AddEqualTest("status", model.NewSymbol("start")))
	r1.AddAction(model.MakeAction{
		Class: "person",
		Attributes: map[string]model.Value{
			"name": model.NewString("Alice"),
			"age":  model.NewInt(30),
		},
	})
	r1.AddAction(model.CBindAction{Variable: "<p>"})
	r1.AddAction(model.ModifyAction{
		TargetElementVar: "<p>",
		Attributes: map[string]model.Value{
			"age": model.NewInt(31),
		},
	})
	r1.AddAction(model.CBindAction{Variable: "p2"})
	r1.AddAction(model.MakeAction{
		Class: "tracker",
		Attributes: map[string]model.Value{
			"target": model.NewVariable("<p2>"),
		},
	})
	r1.AddAction(model.HaltAction{})
	eng.AddRule(r1)

	eng.Make("goal", map[string]model.Value{"status": model.NewSymbol("start")})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	persons := eng.WorkingMemory().FindByClass("person")
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	ageVal, _ := persons[0].Get("age")
	if !ageVal.Equal(model.NewInt(31)) {
		t.Fatalf("expected age 31, got %v", ageVal)
	}

	trackers := eng.WorkingMemory().FindByClass("tracker")
	if len(trackers) != 1 {
		t.Fatalf("expected 1 tracker, got %d", len(trackers))
	}
	targetVal, _ := trackers[0].Get("target")
	if !targetVal.Equal(model.NewInt(persons[0].Timetag)) {
		t.Fatalf("expected tracker target to equal person timetag %d, got %v", persons[0].Timetag, targetVal)
	}
}

func TestEngineCBindErrorNoElement(t *testing.T) {
	eng := New()

	r1 := model.NewRule("fail-cbind")
	r1.AddCondition(model.NewPositiveCE("goal"))
	r1.AddAction(model.CBindAction{Variable: "<elem>"})
	eng.AddRule(r1)

	eng.Make("goal", nil)
	eng.lastAddedTimetag = 0

	_, err := eng.Step()
	if err == nil || !strings.Contains(err.Error(), "cbind: no element has been added") {
		t.Fatalf("expected error mentioning 'cbind: no element has been added', got %v", err)
	}
}

func TestEngineFileIOAndDefaultWrite(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "output.txt")

	eng := New()
	defer eng.CloseAllFiles()

	rule := model.NewRule("write-file-rule")
	rule.AddCondition(model.NewPositiveCE("start"))
	rule.AddAction(model.OpenFileAction{
		LogicalName: "outfile",
		Filespec:    model.NewSymbol(outPath),
		Mode:        "out",
	})
	rule.AddAction(model.DefaultAction{
		LogicalName: "outfile",
		Subsystem:   "write",
	})
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteValue(model.NewSymbol("FILE")),
			model.WriteValue(model.NewSymbol("OUTPUT")),
			model.WriteValue(model.NewInt(42)),
			model.WriteCRLF(),
		},
	})
	rule.AddAction(model.CloseFileAction{
		LogicalName: "outfile",
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	eng.Make("start", nil)
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output file: %v", err)
	}
	if !strings.Contains(string(content), "FILE OUTPUT 42\n") {
		t.Fatalf("unexpected file content: %q", string(content))
	}
}

func TestEngineFileIOAndAccept(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inPath, []byte("alpha beta 123\n"), 0644); err != nil {
		t.Fatalf("failed writing input file: %v", err)
	}

	eng := New()
	defer eng.CloseAllFiles()

	rule := model.NewRule("read-file-rule")
	rule.AddCondition(model.NewPositiveCE("start"))
	rule.AddAction(model.OpenFileAction{
		LogicalName: "infile",
		Filespec:    model.NewSymbol(inPath),
		Mode:        "in",
	})
	rule.AddAction(model.DefaultAction{
		LogicalName: "infile",
		Subsystem:   "accept",
	})
	rule.AddAction(model.MakeAction{
		Class: "data",
		Attributes: map[string]model.Value{
			"first":  model.NewAccept("", false),
			"second": model.NewAccept("infile", false),
			"third":  model.NewAccept("", false),
		},
	})
	rule.AddAction(model.CloseFileAction{
		LogicalName: "infile",
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	eng.Make("start", nil)
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	items := eng.WorkingMemory().FindByClass("data")
	if len(items) != 1 {
		t.Fatalf("expected 1 data item, got %d", len(items))
	}
	fVal, _ := items[0].Get("first")
	sVal, _ := items[0].Get("second")
	tVal, _ := items[0].Get("third")

	if !fVal.Equal(model.NewSymbol("alpha")) {
		t.Fatalf("expected first 'alpha', got %v", fVal)
	}
	if !sVal.Equal(model.NewSymbol("beta")) {
		t.Fatalf("expected second 'beta', got %v", sVal)
	}
	if !tVal.Equal(model.NewInt(123)) {
		t.Fatalf("expected third 123, got %v", tVal)
	}
}

func TestEngineAcceptLine(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_line.txt")
	if err := os.WriteFile(inPath, []byte("10 20 30\n"), 0644); err != nil {
		t.Fatalf("failed writing input line file: %v", err)
	}

	eng := New()
	defer eng.CloseAllFiles()

	rule := model.NewRule("read-line-rule")
	rule.AddCondition(model.NewPositiveCE("start"))
	rule.AddAction(model.OpenFileAction{
		LogicalName: "infile",
		Filespec:    model.NewSymbol(inPath),
		Mode:        "in",
	})
	rule.AddAction(model.MakeAction{
		Class: "vecdata",
		Attributes: map[string]model.Value{
			"vals": model.NewAccept("infile", true),
		},
	})
	rule.AddAction(model.CloseFileAction{
		LogicalName: "infile",
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	eng.Make("start", nil)
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	items := eng.WorkingMemory().FindByClass("vecdata")
	if len(items) != 1 {
		t.Fatalf("expected 1 vecdata item, got %d", len(items))
	}
	vals, _ := items[0].Get("vals")
	if !vals.IsVector() {
		t.Fatalf("expected vector value, got %v", vals)
	}
	vec := vals.VectorElements()
	if len(vec) != 3 || !vec[0].Equal(model.NewInt(10)) || !vec[1].Equal(model.NewInt(20)) || !vec[2].Equal(model.NewInt(30)) {
		t.Fatalf("unexpected vector: %v", vec)
	}
}

func TestEngineTraceToFile(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "trace.log")

	eng := New()
	defer eng.CloseAllFiles()

	if err := eng.OpenFile("ruletrace", tracePath, "out"); err != nil {
		t.Fatalf("openfile failed: %v", err)
	}
	if err := eng.SetDefault("ruletrace", "trace"); err != nil {
		t.Fatalf("setdefault failed: %v", err)
	}
	eng.SetTrace(true)

	rule := model.NewRule("simple-trace-rule")
	rule.AddCondition(model.NewPositiveCE("goal"))
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	eng.Make("goal", nil)
	_, err := eng.Step()
	if err != nil {
		t.Fatalf("step failed: %v", err)
	}

	if err := eng.CloseFile("ruletrace"); err != nil {
		t.Fatalf("closefile failed: %v", err)
	}

	content, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("failed reading trace file: %v", err)
	}
	if !strings.Contains(string(content), "Fired rule 'simple-trace-rule'") {
		t.Fatalf("expected trace output in file, got %q", string(content))
	}
}

func TestEngineAcceptStdinReader(t *testing.T) {
	eng := New()
	eng.SetInputReader(strings.NewReader("val1 99.5\n"))

	v1, err := eng.ReadAccept("")
	if err != nil {
		t.Fatalf("read accept 1 failed: %v", err)
	}
	if !v1.Equal(model.NewSymbol("val1")) {
		t.Fatalf("expected 'val1', got %v", v1)
	}

	v2, err := eng.ReadAccept("")
	if err != nil {
		t.Fatalf("read accept 2 failed: %v", err)
	}
	if !v2.Equal(model.NewFloat(99.5)) {
		t.Fatalf("expected 99.5, got %v", v2)
	}

	eofVal, err := eng.ReadAccept("")
	if err != nil {
		t.Fatalf("read accept eof failed: %v", err)
	}
	if !eofVal.Equal(model.NewSymbol("end-of-file")) {
		t.Fatalf("expected 'end-of-file', got %v", eofVal)
	}
}

func TestEngineGenatom(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	eng.DeclareClass("task", []string{"id", "label"})
	eng.DeclareClass("node", []string{"id", "ref"})

	rule := model.NewRule("create-nodes")
	rule.AddCondition(model.NewPositiveCE("start"))
	rule.AddAction(model.BindAction{
		Variable: "bound-id",
		Value:    model.NewGenatom(),
	})
	rule.AddAction(model.MakeAction{
		Class: "task",
		Attributes: map[string]model.Value{
			"id":    model.NewVariable("<bound-id>"),
			"label": model.NewSymbol("first"),
		},
	})
	rule.AddAction(model.MakeAction{
		Class: "node",
		Attributes: map[string]model.Value{
			"id":  model.NewGenatom(),
			"ref": model.NewGenatom(),
		},
	})
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteValue(model.NewSymbol("GEN:")),
			model.WriteValue(model.NewGenatom()),
			model.WriteCRLF(),
		},
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	eng.Make("start", nil)
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	tasks := eng.WorkingMemory().FindByClass("task")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	taskId, _ := tasks[0].Get("id")
	if !taskId.Equal(model.NewSymbol("atom1")) {
		t.Fatalf("expected task id 'atom1', got %v", taskId)
	}

	nodes := eng.WorkingMemory().FindByClass("node")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	nodeId, _ := nodes[0].Get("id")
	nodeRef, _ := nodes[0].Get("ref")
	if !nodeId.Equal(model.NewSymbol("atom2")) {
		t.Fatalf("expected node id 'atom2', got %v", nodeId)
	}
	if !nodeRef.Equal(model.NewSymbol("atom3")) {
		t.Fatalf("expected node ref 'atom3', got %v", nodeRef)
	}

	if !strings.Contains(outBuf.String(), "GEN: atom4") {
		t.Fatalf("expected write output to contain 'GEN: atom4', got %q", outBuf.String())
	}

	// Direct engine API test
	atom5 := eng.Genatom()
	if !atom5.Equal(model.NewSymbol("atom5")) {
		t.Fatalf("expected atom5, got %v", atom5)
	}

	// Reset genatom counter
	eng.ResetGenatom()
	atom1Again := eng.Genatom()
	if !atom1Again.Equal(model.NewSymbol("atom1")) {
		t.Fatalf("expected atom1 after reset, got %v", atom1Again)
	}
}

func TestEngineLitval(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	// User's exact scenario:
	// (make City ^name Albuquerque ^state NM)
	// (litval name) will evaluate to 2
	eng.Make("City", map[string]model.Value{
		"name":  model.NewSymbol("Albuquerque"),
		"state": model.NewSymbol("NM"),
	})

	idxName, ok := eng.Litval("", "name")
	if !ok || idxName != 2 {
		t.Fatalf("expected litval name = 2, got %d (ok=%v)", idxName, ok)
	}

	idxState, ok := eng.Litval("", "state")
	if !ok || idxState != 3 {
		t.Fatalf("expected litval state = 3, got %d (ok=%v)", idxState, ok)
	}

	idxCityName, ok := eng.Litval("City", "name")
	if !ok || idxCityName != 2 {
		t.Fatalf("expected litval City name = 2, got %d (ok=%v)", idxCityName, ok)
	}

	// Explicit schema via DeclareClass
	eng.DeclareClass("person", []string{"id", "first-name", "age", "city"})
	idxAge, ok := eng.Litval("person", "age")
	if !ok || idxAge != 4 {
		t.Fatalf("expected litval person age = 4, got %d (ok=%v)", idxAge, ok)
	}

	// Test in rule execution: bind, make, and compute
	rule := model.NewRule("litval-rule")
	rule.AddCondition(model.NewPositiveCE("City"))
	rule.AddAction(model.BindAction{
		Variable: "name-slot",
		Value:    model.NewLitval("", model.NewSymbol("name")),
	})
	rule.AddAction(model.BindAction{
		Variable: "computed-slot",
		Value: model.NewCompute([]model.Value{
			model.NewLitval("", model.NewSymbol("name")),
			model.NewInt(10),
		}, []model.ComputeOp{model.ComputeOpAdd}),
	})
	rule.AddAction(model.MakeAction{
		Class: "result",
		Attributes: map[string]model.Value{
			"pos":   model.NewVariable("<name-slot>"),
			"c-pos": model.NewVariable("<computed-slot>"),
		},
	})
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteValue(model.NewSymbol("Slot:")),
			model.WriteValue(model.NewLitval("", model.NewSymbol("name"))),
			model.WriteCRLF(),
		},
	})
	rule.AddAction(model.HaltAction{})
	eng.AddRule(rule)

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	resWMEs := eng.WorkingMemory().FindByClass("result")
	if len(resWMEs) != 1 {
		t.Fatalf("expected 1 result WME, got %d", len(resWMEs))
	}
	posVal, _ := resWMEs[0].Get("pos")
	if !posVal.Equal(model.NewInt(2)) {
		t.Fatalf("expected pos 2, got %v", posVal)
	}
	cPosVal, _ := resWMEs[0].Get("c-pos")
	if !cPosVal.Equal(model.NewInt(12)) {
		t.Fatalf("expected c-pos 12, got %v", cPosVal)
	}
	if !strings.Contains(outBuf.String(), "Slot: 2") {
		t.Fatalf("expected write output to contain 'Slot: 2', got %q", outBuf.String())
	}
}




