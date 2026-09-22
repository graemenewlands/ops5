package rete

import (
	"bytes"
	"strings"
	"testing"

	"ops5/pkg/model"
)

type dummyListener struct{}

func (d *dummyListener) OnActivationAdd(rule *model.Rule, token *Token)    {}
func (d *dummyListener) OnActivationRemove(rule *model.Rule, token *Token) {}

func TestDOTExportEmptyNetwork(t *testing.T) {
	net := NewNetwork()
	var buf bytes.Buffer
	err := net.ExportDOT(&buf)
	if err != nil {
		t.Fatalf("ExportDOT returned error: %v", err)
	}

	dot := buf.String()
	if !strings.HasPrefix(dot, "digraph ReteNetwork {") {
		t.Errorf("expected digraph ReteNetwork header, got:\n%s", dot)
	}
	if !strings.Contains(dot, "alpha_root [label=\"Alpha Root\"") {
		t.Errorf("expected alpha_root node in DOT output")
	}
	if !strings.Contains(dot, "Root BetaMemory (bm0)") {
		t.Errorf("expected Root BetaMemory in DOT output")
	}
	if !strings.HasSuffix(strings.TrimSpace(dot), "}") {
		t.Errorf("expected closing brace in DOT output")
	}
}

func TestDOTExportSingleConditionRule(t *testing.T) {
	net := NewNetwork()
	listener := &dummyListener{}

	rule := model.NewRule("high-temp-alert").SetSalience(100)
	rule.AddCondition(model.NewPositiveCE("sensor").
		AddTest("temp", model.OpGreater, model.NewInt(500)))
	rule.AddAction(model.HaltAction{})

	net.AddRule(rule, listener)

	var buf bytes.Buffer
	if err := net.ExportDOT(&buf); err != nil {
		t.Fatalf("ExportDOT error: %v", err)
	}

	dot := buf.String()

	// Verify Alpha Network components
	if !strings.Contains(dot, "type_sensor [label=\"Type: sensor\"") {
		t.Errorf("missing type_sensor node in:\n%s", dot)
	}
	if !strings.Contains(dot, "^temp > 500") {
		t.Errorf("missing ^temp > 500 constant test node in:\n%s", dot)
	}
	if !strings.Contains(dot, "AlphaMemory") {
		t.Errorf("missing AlphaMemory in:\n%s", dot)
	}

	// Verify Beta Network components
	if !strings.Contains(dot, "Join: (sensor)") {
		t.Errorf("missing Join: (sensor) node in:\n%s", dot)
	}
	if !strings.Contains(dot, "Rule: high-temp-alert\\n[salience: 100]") {
		t.Errorf("missing terminal node with salience in:\n%s", dot)
	}

	// Verify cross-network right activation edge
	if !strings.Contains(dot, "[label=\"right\", style=dashed, color=\"#4285f4\"") {
		t.Errorf("missing dashed right activation edge in:\n%s", dot)
	}

	// Verify terminal activation edge
	if !strings.Contains(dot, "[label=\"activate\", style=solid, color=\"#8430ce\"") {
		t.Errorf("missing terminal activate edge in:\n%s", dot)
	}
}

func TestDOTExportStructuralBetaSharing(t *testing.T) {
	net := NewNetwork()
	listener := &dummyListener{}

	// Rule 1: (A ^id <x>) (B ^a-id <x>) (C ^val 10)
	r1 := model.NewRule("rule1")
	r1.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewVariable("<x>")))
	r1.AddCondition(model.NewPositiveCE("B").AddEqualTest("a-id", model.NewVariable("<x>")))
	r1.AddCondition(model.NewPositiveCE("C").AddEqualTest("val", model.NewInt(10)))

	// Rule 2: (A ^id <x>) (B ^a-id <x>) (D ^val 20)
	r2 := model.NewRule("rule2")
	r2.AddCondition(model.NewPositiveCE("A").AddEqualTest("id", model.NewVariable("<x>")))
	r2.AddCondition(model.NewPositiveCE("B").AddEqualTest("a-id", model.NewVariable("<x>")))
	r2.AddCondition(model.NewPositiveCE("D").AddEqualTest("val", model.NewInt(20)))

	net.AddRule(r1, listener)
	net.AddRule(r2, listener)

	var buf bytes.Buffer
	if err := net.ExportDOT(&buf); err != nil {
		t.Fatalf("ExportDOT error: %v", err)
	}

	dot := buf.String()

	// Both terminal nodes should be present
	if !strings.Contains(dot, "term_rule1") || !strings.Contains(dot, "term_rule2") {
		t.Errorf("expected both rule1 and rule2 terminal nodes in DOT output")
	}

	// Join node for B should test ^a-id = <x>
	if !strings.Contains(dot, "^a-id = <x>") {
		t.Errorf("missing join test ^a-id = <x> in DOT output:\n%s", dot)
	}

	// Join node for C should be present
	if !strings.Contains(dot, "Join: (C)") {
		t.Errorf("missing Join: (C) in DOT output")
	}
	// Join node for D should be present
	if !strings.Contains(dot, "Join: (D)") {
		t.Errorf("missing Join: (D) in DOT output")
	}
}

func TestDOTExportNegativeJoinAndExistential(t *testing.T) {
	net := NewNetwork()
	listener := &dummyListener{}

	// Rule with negative CE: (goal ^status active) -(block ^goal-id <gid>)
	rNeg := model.NewRule("neg-rule")
	rNeg.AddCondition(model.NewPositiveCE("goal").AddEqualTest("id", model.NewVariable("<gid>")))
	rNeg.AddCondition(model.NewNegativeCE("block").AddEqualTest("goal-id", model.NewVariable("<gid>")))
	net.AddRule(rNeg, listener)

	// Rule with existential CE: (order ^id <oid>) (exists (item ^order-id <oid>))
	rExists := model.NewRule("exists-rule")
	rExists.AddCondition(model.NewPositiveCE("order").AddEqualTest("id", model.NewVariable("<oid>")))
	rExists.AddCondition(model.NewExistentialCE("item").AddEqualTest("order-id", model.NewVariable("<oid>")))
	net.AddRule(rExists, listener)

	var buf bytes.Buffer
	if err := net.ExportDOT(&buf); err != nil {
		t.Fatalf("ExportDOT error: %v", err)
	}

	dot := buf.String()

	if !strings.Contains(dot, "NegativeJoin: -(block)") {
		t.Errorf("expected NegativeJoin: -(block) in DOT output:\n%s", dot)
	}
	if !strings.Contains(dot, "[label=\"right (neg)\", style=dashed, color=\"#c5221f\"") {
		t.Errorf("expected right (neg) dashed edge in DOT output:\n%s", dot)
	}

	if !strings.Contains(dot, "ExistsJoin: (exists item)") {
		t.Errorf("expected ExistsJoin: (exists item) in DOT output:\n%s", dot)
	}
	if !strings.Contains(dot, "[label=\"right (exists)\", style=dashed, color=\"#b06000\"") {
		t.Errorf("expected right (exists) dashed edge in DOT output:\n%s", dot)
	}
}

func TestDOTExportNCCSubnetwork(t *testing.T) {
	net := NewNetwork()
	listener := &dummyListener{}

	// NCC Rule: (request ^id <rid>) -( (step1 ^req <rid>) (step2 ^req <rid>) )
	ceNCC := model.NewNccCE([]*model.ConditionElement{
		model.NewPositiveCE("step1").AddEqualTest("req", model.NewVariable("<rid>")),
		model.NewPositiveCE("step2").AddEqualTest("req", model.NewVariable("<rid>")),
	})
	rNCC := model.NewRule("ncc-rule")
	rNCC.AddCondition(model.NewPositiveCE("request").AddEqualTest("id", model.NewVariable("<rid>")))
	rNCC.AddCondition(ceNCC)

	net.AddRule(rNCC, listener)

	var buf bytes.Buffer
	if err := net.ExportDOT(&buf); err != nil {
		t.Fatalf("ExportDOT error: %v", err)
	}

	dot := buf.String()

	if !strings.Contains(dot, "NccNode") {
		t.Errorf("expected NccNode in DOT output:\n%s", dot)
	}
	if !strings.Contains(dot, "NccPartner") {
		t.Errorf("expected NccPartner in DOT output:\n%s", dot)
	}
	if !strings.Contains(dot, "[label=\"inhibit\", style=dotted, color=\"#c5221f\"") {
		t.Errorf("expected dotted inhibit edge in DOT output:\n%s", dot)
	}
}
