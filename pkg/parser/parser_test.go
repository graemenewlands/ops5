package parser

import (
	"testing"

	"ops5/pkg/model"
)

func TestParseRule(t *testing.T) {
	src := `
	; OPS5 rule example
	(p schedule-task
	   <g> (goal ^status active ^type schedule)
	   (slot ^id <sid> ^room <r> ^capacity > 10)
	   -(booked ^slot-id <sid>)
	   -->
	   (make booking ^slot-id <sid> ^room <r>)
	   (modify <g> ^status completed)
	   (write "Assigned room" <r> "to slot" <sid>)
	   (halt)
	)
	`

	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	rule, err := p.ParseRule()
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}

	if rule.Name != "schedule-task" {
		t.Fatalf("expected rule name schedule-task, got %s", rule.Name)
	}

	if len(rule.Conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(rule.Conditions))
	}

	// CE 1: <g> (goal ^status active ^type schedule)
	ce1 := rule.Conditions[0]
	if ce1.ElementVariable != "g" || ce1.Class != "goal" || ce1.IsNegative {
		t.Fatalf("unexpected CE 1: %v", ce1)
	}

	// CE 2: (slot ^id <sid> ^room <r> ^capacity > 10)
	ce2 := rule.Conditions[1]
	if ce2.Class != "slot" || ce2.IsNegative {
		t.Fatalf("unexpected CE 2: %v", ce2)
	}
	// Check capacity > 10
	foundCap := false
	for _, at := range ce2.Tests {
		if at.Attribute == "capacity" {
			foundCap = true
			if len(at.Constraints) != 1 || at.Constraints[0].Op != model.OpGreater || !at.Constraints[0].Value.Equal(model.NewInt(10)) {
				t.Fatalf("unexpected capacity constraint: %v", at.Constraints)
			}
		}
	}
	if !foundCap {
		t.Fatalf("capacity test not found")
	}

	// CE 3: -(booked ^slot-id <sid>)
	ce3 := rule.Conditions[2]
	if !ce3.IsNegative || ce3.Class != "booked" {
		t.Fatalf("unexpected CE 3 (should be negative booked): %v", ce3)
	}

	// Actions: 4 actions (make, modify, write, halt)
	if len(rule.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(rule.Actions))
	}

	if rule.Actions[0].Type() != model.ActionMake {
		t.Fatalf("action 0 should be Make")
	}
	if rule.Actions[1].Type() != model.ActionModify {
		t.Fatalf("action 1 should be Modify")
	}
	if rule.Actions[2].Type() != model.ActionWrite {
		t.Fatalf("action 2 should be Write")
	}
	if rule.Actions[3].Type() != model.ActionHalt {
		t.Fatalf("action 3 should be Halt")
	}
}

func TestParseMake(t *testing.T) {
	src := `(make goal ^status active ^priority 100 ^label "urgent")`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	class, attrs, err := p.ParseMake()
	if err != nil {
		t.Fatalf("failed to parse make: %v", err)
	}

	if class != "goal" {
		t.Fatalf("expected class goal, got %s", class)
	}
	if !attrs["status"].Equal(model.NewSymbol("active")) {
		t.Fatalf("expected status active, got %v", attrs["status"])
	}
	if !attrs["priority"].Equal(model.NewInt(100)) {
		t.Fatalf("expected priority 100, got %v", attrs["priority"])
	}
	if !attrs["label"].Equal(model.NewString("urgent")) {
		t.Fatalf("expected label 'urgent', got %v", attrs["label"])
	}
}
