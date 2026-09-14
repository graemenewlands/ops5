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

func TestEngineEnsureNewline(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	if eng.CurrentCol() != 1 {
		t.Errorf("expected initial currentCol 1, got %d", eng.CurrentCol())
	}
	eng.EnsureNewline()
	if outBuf.Len() != 0 {
		t.Errorf("EnsureNewline should do nothing when currentCol == 1")
	}

	rule := model.NewRule("write-partial")
	rule.AddCondition(model.NewPositiveCE("start"))
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{
			model.WriteCRLF(),
			model.WriteValue(model.NewSymbol("hello")),
		},
	})
	eng.AddRule(rule)
	eng.Make("start", nil)

	_, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if eng.CurrentCol() <= 1 {
		t.Errorf("expected currentCol > 1 after writing without trailing newline, got %d", eng.CurrentCol())
	}

	eng.EnsureNewline()
	if eng.CurrentCol() != 1 {
		t.Errorf("expected currentCol to be reset to 1, got %d", eng.CurrentCol())
	}
	if !strings.HasSuffix(outBuf.String(), "hello\n") {
		t.Errorf("expected output to end with newline, got %q", outBuf.String())
	}
}

func TestEngineExciseRule(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	r1 := model.NewRule("rule-1")
	r1.AddCondition(model.NewPositiveCE("trigger"))
	r1.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("RULE-1-FIRED"))},
	})
	eng.AddRule(r1)

	r2 := model.NewRule("rule-2")
	r2.AddCondition(model.NewPositiveCE("trigger"))
	r2.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("RULE-2-FIRED"))},
	})
	eng.AddRule(r2)

	if len(eng.Rules()) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(eng.Rules()))
	}
	if eng.Rule("rule-1") == nil || eng.Rule("rule-2") == nil {
		t.Fatalf("expected rule-1 and rule-2 to be found")
	}

	eng.Make("trigger", nil)

	if eng.ConflictSet().Count() != 2 {
		t.Fatalf("expected 2 activations in CS, got %d", eng.ConflictSet().Count())
	}

	// Excise rule-1
	if !eng.ExciseRule("rule-1") {
		t.Fatalf("expected ExciseRule('rule-1') to return true")
	}
	if eng.ExciseRule("rule-1") {
		t.Fatalf("expected second ExciseRule('rule-1') to return false")
	}
	if eng.Rule("rule-1") != nil {
		t.Fatalf("expected rule-1 to be nil after excision")
	}
	if len(eng.Rules()) != 1 || eng.Rules()[0].Name != "rule-2" {
		t.Fatalf("expected only rule-2 in engine rules, got %v", eng.Rules())
	}

	// Conflict set should only contain rule-2
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation in CS after excision, got %d", eng.ConflictSet().Count())
	}

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	out := outBuf.String()
	if strings.Contains(out, "RULE-1-FIRED") {
		t.Errorf("excised rule-1 should not have fired, got output: %q", out)
	}
	if !strings.Contains(out, "RULE-2-FIRED") {
		t.Errorf("expected rule-2 to fire, got output: %q", out)
	}

	// Working memory trigger element should still be present
	triggers := eng.WorkingMemory().FindByClass("trigger")
	if len(triggers) != 1 {
		t.Errorf("expected working memory to be preserved, got %d triggers", len(triggers))
	}
}

func TestEngineExciseMultipleAndRedefine(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	rA := model.NewRule("rule-a")
	rA.AddCondition(model.NewPositiveCE("task").AddEqualTest("status", model.NewSymbol("ready")))
	rA.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("A"))},
	})
	eng.AddRule(rA)

	rB := model.NewRule("rule-b")
	rB.AddCondition(model.NewPositiveCE("task").AddEqualTest("status", model.NewSymbol("ready")))
	rB.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("B"))},
	})
	eng.AddRule(rB)

	rC := model.NewRule("rule-c")
	rC.AddCondition(model.NewPositiveCE("task").AddEqualTest("status", model.NewSymbol("ready")))
	rC.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("C"))},
	})
	eng.AddRule(rC)

	// Excise rule-a and rule-c, plus a non-existent rule
	excised := eng.ExciseRules("rule-a", "rule-c", "unknown-rule")
	if len(excised) != 2 || excised[0] != "rule-a" || excised[1] != "rule-c" {
		t.Fatalf("unexpected excised rules: %v", excised)
	}

	eng.Make("task", map[string]model.Value{
		"status": model.NewSymbol("ready"),
	})

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle for rule-b, got %d", cycles)
	}

	// Re-add rule-a (redefinition) and verify it compiles and can fire on new WME
	newRA := model.NewRule("rule-a")
	newRA.AddCondition(model.NewPositiveCE("task").AddEqualTest("status", model.NewSymbol("reloaded")))
	newRA.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("RELOADED-A"))},
	})
	eng.AddRule(newRA)

	eng.Make("task", map[string]model.Value{
		"status": model.NewSymbol("reloaded"),
	})

	cycles2, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if cycles2 != 1 {
		t.Fatalf("expected 1 cycle for reloaded rule-a, got %d", cycles2)
	}
	if !strings.Contains(outBuf.String(), "RELOADED-A") {
		t.Errorf("expected reloaded rule-a to fire, got: %q", outBuf.String())
	}
}

func TestEnginePrintRuleAndRules(t *testing.T) {
	eng := New()

	rule := model.NewRule("FindAncestors")
	rule.AddCondition(model.NewPositiveCE("Request").
		AddEqualTest("type", model.NewSymbol("ancestor")).
		AddEqualTest("target", model.NewVariable("<p>")))
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewString("Found ancestor"))},
	})
	eng.AddRule(rule)

	text, ok := eng.PrintRule("FindAncestors")
	if !ok {
		t.Fatalf("expected FindAncestors to be found")
	}
	if !strings.Contains(text, "(p FindAncestors") || !strings.Contains(text, "(write (crlf) \"Found ancestor\")") {
		t.Errorf("unexpected PrintRule output:\n%s", text)
	}

	_, ok = eng.PrintRule("nonexistent")
	if ok {
		t.Errorf("expected nonexistent rule to return false")
	}

	all := eng.PrintRules()
	if len(all) != 1 {
		t.Fatalf("expected 1 rule in PrintRules(), got %d", len(all))
	}

	star := eng.PrintRules("*")
	if len(star) != 1 {
		t.Fatalf("expected 1 rule in PrintRules('*'), got %d", len(star))
	}
}

func TestEngineRemoveAll(t *testing.T) {
	eng := New()

	rule := model.NewRule("match-item")
	rule.AddCondition(model.NewPositiveCE("item"))
	rule.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("MATCHED"))},
	})
	eng.AddRule(rule)

	eng.Make("item", map[string]model.Value{"id": model.NewInt(1)})
	eng.Make("item", map[string]model.Value{"id": model.NewInt(2)})
	eng.Make("item", map[string]model.Value{"id": model.NewInt(3)})

	if eng.WorkingMemory().Count() != 3 {
		t.Fatalf("expected 3 WMEs in WM, got %d", eng.WorkingMemory().Count())
	}
	if eng.ConflictSet().Count() != 3 {
		t.Fatalf("expected 3 activations in CS, got %d", eng.ConflictSet().Count())
	}

	removed := eng.RemoveAll()
	if len(removed) != 3 {
		t.Fatalf("expected 3 WMEs removed by RemoveAll(), got %d", len(removed))
	}
	if eng.WorkingMemory().Count() != 0 {
		t.Fatalf("expected 0 WMEs in WM after RemoveAll(), got %d", eng.WorkingMemory().Count())
	}
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected 0 activations in CS after RemoveAll(), got %d", eng.ConflictSet().Count())
	}

	// Verify next timetag continues monotonically
	wme4 := eng.Make("item", map[string]model.Value{"id": model.NewInt(4)})
	if wme4.Timetag != 4 {
		t.Fatalf("expected timetag 4, got %d", wme4.Timetag)
	}
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation in CS for new WME, got %d", eng.ConflictSet().Count())
	}
}

func TestEngineRuleRemoveWildcard(t *testing.T) {
	eng := New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	// Rule that removes all working memory elements
	flushRule := model.NewRule("flush-all")
	flushRule.AddCondition(model.NewPositiveCE("flush-command"))
	flushRule.AddAction(model.RemoveAction{Wildcard: true})
	flushRule.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("ALL-WMES-FLUSHED"))},
	})
	eng.AddRule(flushRule)

	// Another rule that would match data
	dataRule := model.NewRule("process-data")
	dataRule.AddCondition(model.NewPositiveCE("data"))
	dataRule.AddAction(model.WriteAction{
		Args: []model.WriteArg{model.WriteCRLF(), model.WriteValue(model.NewSymbol("DATA-PROCESSED"))},
	})
	eng.AddRule(dataRule)

	// Assert data and flush-command
	eng.Make("data", map[string]model.Value{"val": model.NewInt(100)})
	eng.Make("flush-command", nil) // timetag 2 -> more recent than data, MEA/LEX picks flush-command

	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle (flush-all fires and clears WM before process-data can fire), got %d", cycles)
	}

	if eng.WorkingMemory().Count() != 0 {
		t.Fatalf("expected empty working memory after (remove *), got %d", eng.WorkingMemory().Count())
	}
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected empty conflict set after (remove *), got %d", eng.ConflictSet().Count())
	}
	out := outBuf.String()
	if !strings.Contains(out, "ALL-WMES-FLUSHED") {
		t.Errorf("expected ALL-WMES-FLUSHED output, got: %q", out)
	}
	if strings.Contains(out, "DATA-PROCESSED") {
		t.Errorf("data-processed should NOT have fired because data was removed by (remove *), got: %q", out)
	}
}

func TestEngineWatchLevels(t *testing.T) {
	// 1. Initial watch level must be 1
	eng := New()
	if eng.WatchLevel() != 1 {
		t.Fatalf("expected initial watch level 1, got %d", eng.WatchLevel())
	}

	// 2. SetWatchLevel validation
	for _, lvl := range []int{0, 1, 2} {
		if err := eng.SetWatchLevel(lvl); err != nil {
			t.Fatalf("SetWatchLevel(%d) failed: %v", lvl, err)
		}
		if eng.WatchLevel() != lvl {
			t.Fatalf("expected watch level %d, got %d", lvl, eng.WatchLevel())
		}
	}
	if err := eng.SetWatchLevel(3); err == nil {
		t.Fatalf("expected error for watch level 3, got nil")
	}

	// 3. Watch Level 0: give no report of firings or changes to working memory
	{
		eng0 := New()
		_ = eng0.SetWatchLevel(0)
		var traceBuf bytes.Buffer
		eng0.SetTraceWriter(&traceBuf)

		r := model.NewRule("silent-rule")
		r.AddCondition(model.NewPositiveCE("trigger"))
		r.AddAction(model.MakeAction{Class: "item", Attributes: map[string]model.Value{"id": model.NewInt(1)}})
		eng0.AddRule(r)

		eng0.Make("trigger", nil)
		_, err := eng0.Run(10)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		if traceBuf.Len() != 0 {
			t.Fatalf("watch 0: expected no report of firings or WM changes, got: %q", traceBuf.String())
		}
	}

	// 4. Watch Level 1: report rule name and time tags of each WME for each firing; no WM changes
	{
		eng1 := New()
		_ = eng1.SetWatchLevel(1)
		var traceBuf bytes.Buffer
		eng1.SetTraceWriter(&traceBuf)

		r := model.NewRule("trace-firing-rule")
		r.AddCondition(model.NewPositiveCE("task").WithElementVariable("t").AddEqualTest("val", model.NewInt(10)))
		r.AddAction(model.MakeAction{Class: "result", Attributes: map[string]model.Value{"ok": model.NewSymbol("yes")}})
		r.AddAction(model.RemoveAction{TargetElementVar: "t"})
		eng1.AddRule(r)

		// Assertion before run should NOT be reported in watch 1
		eng1.Make("task", map[string]model.Value{"val": model.NewInt(10)}) // timetag 1
		if traceBuf.Len() != 0 {
			t.Fatalf("watch 1: expected no WM change reports, got: %q", traceBuf.String())
		}

		_, err := eng1.Run(10)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		traceOut := traceBuf.String()
		if !strings.Contains(traceOut, "Fired rule 'trace-firing-rule' with WMEs [1]") {
			t.Fatalf("watch 1: expected rule name and time tags, got: %q", traceOut)
		}
		if strings.Contains(traceOut, "=>WM:") || strings.Contains(traceOut, "<=WM:") {
			t.Fatalf("watch 1: should NOT report WM changes, got: %q", traceOut)
		}
	}

	// 5. Watch Level 2: report watch 1 info + report each change to working memory
	{
		eng2 := New()
		_ = eng2.SetWatchLevel(2)
		var traceBuf bytes.Buffer
		eng2.SetTraceWriter(&traceBuf)

		// Assertions should report =>WM:
		eng2.Make("init", map[string]model.Value{"tag": model.NewInt(100)}) // timetag 1
		if !strings.Contains(traceBuf.String(), "=>WM: (1: init ^tag 100)") {
			t.Fatalf("watch 2: expected =>WM assertion report, got: %q", traceBuf.String())
		}

		r := model.NewRule("modify-and-clear")
		r.AddCondition(model.NewPositiveCE("init").WithElementVariable("i"))
		r.AddAction(model.ModifyAction{
			TargetElementVar: "i",
			Attributes:       map[string]model.Value{"tag": model.NewInt(200)},
		})
		r.AddAction(model.RemoveAction{Wildcard: true})
		eng2.AddRule(r)

		traceBuf.Reset()
		_, err := eng2.Run(10)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}
		traceOut := traceBuf.String()

		// Should report firing (watch 1 info)
		if !strings.Contains(traceOut, "Fired rule 'modify-and-clear' with WMEs [1]") {
			t.Fatalf("watch 2: expected firing report, got: %q", traceOut)
		}

		// Should report modify: retract old (1), assert new (2)
		if !strings.Contains(traceOut, "<=WM: (1: init ^tag 100)") {
			t.Fatalf("watch 2: expected modify retraction report, got: %q", traceOut)
		}
		if !strings.Contains(traceOut, "=>WM: (2: init ^tag 200)") {
			t.Fatalf("watch 2: expected modify assertion report, got: %q", traceOut)
		}

		// Should report (remove *): retract (2)
		if !strings.Contains(traceOut, "<=WM: (2: init ^tag 200)") {
			t.Fatalf("watch 2: expected wildcard retraction report, got: %q", traceOut)
		}
	}
}

func TestEngineWatchRHSAction(t *testing.T) {
	eng := New()
	_ = eng.SetWatchLevel(0) // start at 0
	var traceBuf bytes.Buffer
	eng.SetTraceWriter(&traceBuf)

	// Rule that elevates to watch 2 and makes an item
	r := model.NewRule("elevate-watch")
	lvl2 := 2
	r.AddCondition(model.NewPositiveCE("start"))
	r.AddAction(model.WatchAction{Level: &lvl2})
	r.AddAction(model.MakeAction{Class: "payload", Attributes: map[string]model.Value{"val": model.NewInt(999)}})
	r.AddAction(model.WatchAction{Level: nil}) // query watch level
	eng.AddRule(r)

	eng.Make("start", nil) // timetag 1, watch level is 0 so no trace
	if traceBuf.Len() != 0 {
		t.Fatalf("expected empty trace before firing, got: %q", traceBuf.String())
	}

	_, err := eng.Run(10)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if eng.WatchLevel() != 2 {
		t.Fatalf("expected watch level to be updated to 2 by RHS action, got %d", eng.WatchLevel())
	}

	traceOut := traceBuf.String()
	// Since watch level was elevated to 2 in RHS, the payload make should be reported!
	if !strings.Contains(traceOut, "=>WM: (2: payload ^val 999)") {
		t.Fatalf("expected =>WM for payload, got: %q", traceOut)
	}
	// Query (watch) writes current watch level: 2
	if !strings.Contains(traceOut, "Current watch level: 2") {
		t.Fatalf("expected 'Current watch level: 2', got: %q", traceOut)
	}
}

func TestEnginePPWM(t *testing.T) {
	eng := New()

	eng.DeclareClass("City", []string{"name", "state", "population"})
	eng.DeclareVectorAttribute("coords")

	eng.Make("City", map[string]model.Value{
		"name":       model.NewSymbol("Pittsburgh"),
		"state":      model.NewSymbol("Pennsylvania"),
		"population": model.NewInt(300000),
	})
	eng.Make("City", map[string]model.Value{
		"name":       model.NewSymbol("Philadelphia"),
		"state":      model.NewSymbol("Pennsylvania"),
		"population": model.NewInt(1500000),
	})
	eng.Make("City", map[string]model.Value{
		"name":       model.NewSymbol("Boston"),
		"state":      model.NewSymbol("Massachusetts"),
		"population": model.NewInt(675000),
	})
	eng.Make("Person", map[string]model.Value{
		"name":  model.NewSymbol("Franklin"),
		"state": model.NewSymbol("Pennsylvania"),
	})

	// 1. PPWM with user's exact example: (ppwm City ^state Pennsylvania)
	wmes, err := eng.PPWM("City ^state Pennsylvania")
	if err != nil {
		t.Fatalf("PPWM failed: %v", err)
	}
	if len(wmes) != 2 {
		t.Fatalf("expected 2 WMEs matching (ppwm City ^state Pennsylvania), got %d", len(wmes))
	}
	if wmes[0].Timetag != 1 || wmes[1].Timetag != 2 {
		t.Errorf("expected timetags 1 and 2, got %d and %d", wmes[0].Timetag, wmes[1].Timetag)
	}

	// 2. PrintPPWM
	lines := eng.PrintPPWM(model.NewPositiveCE("City").AddEqualTest("state", model.NewSymbol("Pennsylvania")))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines from PrintPPWM, got %d", len(lines))
	}
	if lines[0] != "(1: City ^name Pittsburgh ^population 300000 ^state Pennsylvania)" {
		t.Errorf("unexpected line 0: %q", lines[0])
	}
	if lines[1] != "(2: City ^name Philadelphia ^population 1500000 ^state Pennsylvania)" {
		t.Errorf("unexpected line 1: %q", lines[1])
	}

	// 3. PPWM wildcard
	allWMEs, err := eng.PPWM("*")
	if err != nil {
		t.Fatalf("PPWM * failed: %v", err)
	}
	if len(allWMEs) != 4 {
		t.Fatalf("expected all 4 WMEs for wildcard, got %d", len(allWMEs))
	}

	// 4. Multiple attributes: City ^state Pennsylvania ^name Pittsburgh
	singleWME, err := eng.PPWM("(ppwm City ^state Pennsylvania ^name Pittsburgh)")
	if err != nil {
		t.Fatalf("PPWM multiple attrs failed: %v", err)
	}
	if len(singleWME) != 1 || singleWME[0].Timetag != 1 {
		t.Fatalf("expected only timetag 1, got %v", singleWME)
	}

	// 5. PPWM with non-matching pattern
	none, err := eng.PPWM("City ^state Ohio")
	if err != nil {
		t.Fatalf("PPWM non-matching failed: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 matches for Ohio, got %d", len(none))
	}

	// 6. PPWM error on forbidden constructs
	if _, err := eng.PPWM("City ^state <x>"); err == nil {
		t.Fatal("expected error on variable in PPWM, got nil")
	}
	if _, err := eng.PPWM("City ^population > 500000"); err == nil {
		t.Fatal("expected error on predicate in PPWM, got nil")
	}
}

func TestEngineSubstr(t *testing.T) {
	eng := New()
	eng.DeclareClass("string", []string{"sequence"})
	eng.DeclareVectorAttribute("sequence")

	w1 := eng.Make("string", map[string]model.Value{
		"sequence": model.NewVector([]model.Value{
			model.NewSymbol("A"),
			model.NewSymbol("B"),
			model.NewSymbol("C"),
			model.NewSymbol("D"),
		}),
	})

	// 1. Single element access: (substr 1 sequence sequence) -> scalar 'A'
	res1 := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w1.Timetag),
		Start:      model.NewSymbol("sequence"),
		End:        model.NewSymbol("sequence"),
	}, nil)
	if res1.Type() != model.TypeSymbol || res1.Raw().(string) != "A" {
		t.Fatalf("expected scalar 'A', got %v (%s)", res1, res1.Type())
	}

	// 2. Numeric range: (substr 1 2 4) -> vector [A, B, C]
	res2 := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w1.Timetag),
		Start:      model.NewInt(2),
		End:        model.NewInt(4),
	}, nil)
	if !res2.IsVector() || len(res2.VectorElements()) != 3 {
		t.Fatalf("expected vector of 3 elements, got %v", res2)
	}
	if res2.VectorElements()[0].Raw() != "A" || res2.VectorElements()[1].Raw() != "B" || res2.VectorElements()[2].Raw() != "C" {
		t.Fatalf("expected [A, B, C], got %v", res2)
	}

	// 3. inf range: (substr 1 3 inf) -> vector [B, C, D]
	res3 := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w1.Timetag),
		Start:      model.NewInt(3),
		End:        model.NewSymbol("inf"),
	}, nil)
	if !res3.IsVector() || len(res3.VectorElements()) != 3 {
		t.Fatalf("expected vector of 3 elements, got %v", res3)
	}
	if res3.VectorElements()[0].Raw() != "B" || res3.VectorElements()[1].Raw() != "C" || res3.VectorElements()[2].Raw() != "D" {
		t.Fatalf("expected [B, C, D], got %v", res3)
	}

	// 4. Element variable lookup with bindings: <str> -> timetag
	bindings := map[string]model.Value{
		"str": model.NewInt(w1.Timetag),
	}
	res4 := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewVariable("str"),
		Start:      model.NewSymbol("sequence"),
		End:        model.NewSymbol("sequence"),
	}, bindings)
	if res4.Type() != model.TypeSymbol || res4.Raw().(string) != "A" {
		t.Fatalf("expected scalar 'A' via element variable, got %v", res4)
	}

	// 5. Multiple attributes schema: person ^name Graeme ^age 30 ^scores 10 20 30 40
	eng.DeclareClass("person", []string{"name", "age", "scores"})
	eng.DeclareVectorAttribute("scores")

	w2 := eng.Make("person", map[string]model.Value{
		"name": model.NewSymbol("Graeme"),
		"age":  model.NewInt(30),
		"scores": model.NewVector([]model.Value{
			model.NewInt(10),
			model.NewInt(20),
			model.NewInt(30),
			model.NewInt(40),
		}),
	})

	// Name access: (substr 2 name name) -> Graeme
	resName := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w2.Timetag),
		Start:      model.NewSymbol("name"),
		End:        model.NewSymbol("name"),
	}, nil)
	if resName.Raw() != "Graeme" {
		t.Fatalf("expected Graeme, got %v", resName)
	}

	// Age access: (substr 2 age age) -> 30
	resAge := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w2.Timetag),
		Start:      model.NewSymbol("age"),
		End:        model.NewSymbol("age"),
	}, nil)
	if resAge.Raw() != int64(30) {
		t.Fatalf("expected 30, got %v", resAge)
	}

	// Scores single access: (substr 2 scores scores) -> 10
	resScore := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w2.Timetag),
		Start:      model.NewSymbol("scores"),
		End:        model.NewSymbol("scores"),
	}, nil)
	if resScore.Raw() != int64(10) {
		t.Fatalf("expected 10, got %v", resScore)
	}

	// Scores inf range: (substr 2 5 inf) -> [20, 30, 40]
	resScoresInf := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w2.Timetag),
		Start:      model.NewInt(5),
		End:        model.NewSymbol("inf"),
	}, nil)
	if !resScoresInf.IsVector() || len(resScoresInf.VectorElements()) != 3 {
		t.Fatalf("expected 3 scores, got %v", resScoresInf)
	}
	if resScoresInf.VectorElements()[0].Raw() != int64(20) || resScoresInf.VectorElements()[2].Raw() != int64(40) {
		t.Fatalf("expected [20, 30, 40], got %v", resScoresInf)
	}

	// Out of bounds
	resEmpty := eng.EvaluateSubstr(&model.SubstrExpr{
		ElementRef: model.NewInt(w2.Timetag),
		Start:      model.NewInt(99),
		End:        model.NewSymbol("inf"),
	}, nil)
	if !resEmpty.IsVector() || len(resEmpty.VectorElements()) != 0 {
		t.Fatalf("expected empty vector for out of bounds inf, got %v", resEmpty)
	}
}

func TestEngineSubstrRuleExecution(t *testing.T) {
	eng := New()
	eng.SetTrace(false)

	eng.DeclareClass("string", []string{"sequence"})
	eng.DeclareVectorAttribute("sequence")
	eng.DeclareClass("output", []string{"val"})

	// Rule:
	// (p pop-string
	//    <sVal> (string ^sequence <first> <second>)
	//    -->
	//    (bind <head> (substr <sVal> sequence sequence))
	//    (bind <next> (compute (litval sequence) + 1))
	//    (modify <sVal> ^sequence (substr <sVal> <next> inf))
	//    (make output ^val <head>)
	// )
	r := model.NewRule("pop-string")
	ce := model.NewPositiveCE("string").
		WithElementVariable("sVal").
		AddEqualTest("sequence", model.NewVariable("first")).
		AddEqualTest("sequence", model.NewVariable("second"))
	r.AddCondition(ce)

	r.AddAction(model.BindAction{
		Variable: "head",
		Value: model.NewSubstr(
			model.NewVariable("sVal"),
			model.NewSymbol("sequence"),
			model.NewSymbol("sequence"),
		),
	})
	r.AddAction(model.BindAction{
		Variable: "next",
		Value: model.NewCompute(
			[]model.Value{model.NewLitval("", model.NewSymbol("sequence")), model.NewInt(1)},
			[]model.ComputeOp{model.ComputeOpAdd},
		),
	})
	r.AddAction(model.ModifyAction{
		TargetElementVar: "sVal",
		Attributes: map[string]model.Value{
			"sequence": model.NewSubstr(
				model.NewVariable("sVal"),
				model.NewVariable("next"),
				model.NewSymbol("inf"),
			),
		},
	})
	r.AddAction(model.MakeAction{
		Class: "output",
		Attributes: map[string]model.Value{
			"val": model.NewVariable("head"),
		},
	})

	eng.AddRule(r)

	// Make initial WME with sequence [A, B, C]
	eng.Make("string", map[string]model.Value{
		"sequence": model.NewVector([]model.Value{
			model.NewSymbol("A"),
			model.NewSymbol("B"),
			model.NewSymbol("C"),
		}),
	})

	// Run to quiescence
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	// Should fire 2 times (A then B, leaving [C] which doesn't match <first> <second>)
	if cycles != 2 {
		t.Fatalf("expected 2 cycles, got %d", cycles)
	}

	outputs := eng.FindWMEsMatching(model.NewPositiveCE("output"))
	if len(outputs) != 2 {
		t.Fatalf("expected 2 output WMEs, got %d", len(outputs))
	}

	out1, _ := outputs[0].Get("val")
	out2, _ := outputs[1].Get("val")
	if out1.Raw() != "A" || out2.Raw() != "B" {
		t.Fatalf("expected outputs A and B, got %v and %v", out1, out2)
	}

	// Final string element should have sequence [C]
	stringsWMEs := eng.FindWMEsMatching(model.NewPositiveCE("string"))
	if len(stringsWMEs) != 1 {
		t.Fatalf("expected 1 string WME remaining, got %d", len(stringsWMEs))
	}
	remSeq, _ := stringsWMEs[0].Get("sequence")
	if !remSeq.IsVector() || len(remSeq.VectorElements()) != 1 || remSeq.VectorElements()[0].Raw() != "C" {
		t.Fatalf("expected remaining sequence [C], got %v", remSeq)
	}
}









