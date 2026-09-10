package parser

import (
	"strings"
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

func TestParseLiteralize(t *testing.T) {
	src := `(literalize person name age job ^salary)`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	class, attrs, err := p.ParseLiteralize()
	if err != nil {
		t.Fatalf("failed to parse literalize: %v", err)
	}

	if class != "person" {
		t.Fatalf("expected class 'person', got %s", class)
	}
	expected := []string{"name", "age", "job", "salary"}
	if len(attrs) != len(expected) {
		t.Fatalf("expected %d attributes, got %d", len(expected), len(attrs))
	}
	for i, exp := range expected {
		if attrs[i] != exp {
			t.Fatalf("at index %d expected %s, got %s", i, exp, attrs[i])
		}
	}
}

func TestLiteralizePositionalMapping(t *testing.T) {
	src := `
	(literalize vector x y z)
	(make vector 10 20 30)
	(p move-vector
	   (vector <x> <y> 30)
	   -->
	   (make result 100)
	)
	`
	stmts, err := ParseProgram(src)
	if err != nil {
		t.Fatalf("failed to parse program: %v", err)
	}

	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(stmts))
	}

	// 1. Literalize
	if stmts[0].Type != StmtLiteralize || stmts[0].LiteralizeClass != "vector" {
		t.Fatalf("stmt 0 unexpected: %v", stmts[0])
	}

	// 2. Make vector with positional values 10, 20, 30 -> mapped to x, y, z!
	if stmts[1].Type != StmtMake || stmts[1].MakeClass != "vector" {
		t.Fatalf("stmt 1 unexpected: %v", stmts[1])
	}
	mAttrs := stmts[1].MakeAttributes
	if !mAttrs["x"].Equal(model.NewInt(10)) || !mAttrs["y"].Equal(model.NewInt(20)) || !mAttrs["z"].Equal(model.NewInt(30)) {
		t.Fatalf("expected x=10, y=20, z=30, got %v", mAttrs)
	}

	// 3. Rule with positional condition
	rule := stmts[2].Rule
	if rule.Name != "move-vector" {
		t.Fatalf("expected rule move-vector, got %s", rule.Name)
	}
	cond := rule.Conditions[0]
	// Check condition tests on x, y, z
	tests := make(map[string]model.Value)
	for _, at := range cond.Tests {
		tests[at.Attribute] = at.Constraints[0].Value
	}
	if !tests["x"].Equal(model.NewVariable("<x>")) {
		t.Fatalf("expected x to match <x>, got %v", tests["x"])
	}
	if !tests["y"].Equal(model.NewVariable("<y>")) {
		t.Fatalf("expected y to match <y>, got %v", tests["y"])
	}
	if !tests["z"].Equal(model.NewInt(30)) {
		t.Fatalf("expected z to match 30, got %v", tests["z"])
	}
}

func TestParseVectorAttribute(t *testing.T) {
	src := `(vector-attribute location airlines-flown ^hotels-stayed)`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	attrs, err := p.ParseVectorAttribute()
	if err != nil {
		t.Fatalf("failed to parse vector-attribute: %v", err)
	}

	expected := []string{"location", "airlines-flown", "hotels-stayed"}
	if len(attrs) != len(expected) {
		t.Fatalf("expected %d attributes, got %d", len(expected), len(attrs))
	}
	for i, exp := range expected {
		if attrs[i] != exp {
			t.Fatalf("expected attrs[%d] == %s, got %s", i, exp, attrs[i])
		}
		if !p.IsVectorAttribute(exp) {
			t.Fatalf("expected p.IsVectorAttribute(%s) == true", exp)
		}
	}
}

func TestVectorAttributeWorkflow(t *testing.T) {
	src := `
	(literalize City name location state country population)
	(vector-attribute location)
	(make City ^name Boston ^location 42.36 -71.05 ^state MA ^country USA ^population 675000)
	(p find-boston
	   (City ^name <name> ^location <lat> <long> ^state MA)
	   -->
	   (write "City:" <name> "at" <lat> <long>)
	)
	`
	stmts, err := ParseProgram(src)
	if err != nil {
		t.Fatalf("failed to parse program: %v", err)
	}

	if len(stmts) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(stmts))
	}

	// 1. Literalize
	if stmts[0].Type != StmtLiteralize || strings.ToLower(stmts[0].LiteralizeClass) != "city" {
		t.Fatalf("expected stmt 0 literalize city, got %v", stmts[0])
	}

	// 2. VectorAttribute
	if stmts[1].Type != StmtVectorAttribute || len(stmts[1].VectorAttrs) != 1 || stmts[1].VectorAttrs[0] != "location" {
		t.Fatalf("expected stmt 1 vector-attribute location, got %v", stmts[1])
	}

	// 3. Make
	if stmts[2].Type != StmtMake || stmts[2].MakeClass != "City" {
		t.Fatalf("expected stmt 2 make City, got %v", stmts[2])
	}
	mAttrs := stmts[2].MakeAttributes
	locVal, ok := mAttrs["location"]
	if !ok || !locVal.IsVector() {
		t.Fatalf("expected location to be vector value, got %v", locVal)
	}
	elems := locVal.VectorElements()
	if len(elems) != 2 || !elems[0].Equal(model.NewFloat(42.36)) || !elems[1].Equal(model.NewFloat(-71.05)) {
		t.Fatalf("expected location [42.36, -71.05], got %v", elems)
	}

	// 4. Rule with vector constraints
	rule := stmts[3].Rule
	cond := rule.Conditions[0]
	var locTest *model.AttributeTest
	for i := range cond.Tests {
		if cond.Tests[i].Attribute == "location" {
			locTest = &cond.Tests[i]
			break
		}
	}
	if locTest == nil {
		t.Fatalf("expected condition to have location test")
	}
	if len(locTest.Constraints) != 2 {
		t.Fatalf("expected 2 constraints on location, got %d", len(locTest.Constraints))
	}
	if !locTest.Constraints[0].Value.Equal(model.NewVariable("<lat>")) ||
		!locTest.Constraints[1].Value.Equal(model.NewVariable("<long>")) {
		t.Fatalf("expected constraints <lat> and <long>, got %v", locTest.Constraints)
	}
}

func TestParseWriteFormatting(t *testing.T) {
	src := `
	(p format-grid
	   (item ^id <id> ^name <name> ^score <score>)
	   -->
	   (write (crlf) (tabto 5) "ID:" (tabto 12) <id> (tabto 25) <name> (tabto 40) <score> (crlf))
	   (write crlf "Done" crlf)
	)
	`
	rules, err := ParseRules(src)
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if len(r.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(r.Actions))
	}

	// Action 1: (write (crlf) (tabto 5) "ID:" (tabto 12) <id> (tabto 25) <name> (tabto 40) <score> (crlf))
	w1, ok := r.Actions[0].(model.WriteAction)
	if !ok {
		t.Fatalf("expected WriteAction, got %T", r.Actions[0])
	}
	// Expected args: CRLF, TabTo(5), Value("ID:"), TabTo(12), Value(<id>), TabTo(25), Value(<name>), TabTo(40), Value(<score>), CRLF
	if len(w1.Args) != 10 {
		t.Fatalf("expected 10 write args in action 1, got %d", len(w1.Args))
	}
	if w1.Args[0].Type != model.WriteArgCRLF {
		t.Errorf("arg 0 expected CRLF, got %v", w1.Args[0].Type)
	}
	if w1.Args[1].Type != model.WriteArgTabTo || !w1.Args[1].Value.Equal(model.NewInt(5)) {
		t.Errorf("arg 1 expected TabTo(5), got %v", w1.Args[1])
	}
	if w1.Args[2].Type != model.WriteArgValue || !w1.Args[2].Value.Equal(model.NewString("ID:")) {
		t.Errorf("arg 2 expected Value(\"ID:\"), got %v", w1.Args[2])
	}
	if w1.Args[3].Type != model.WriteArgTabTo || !w1.Args[3].Value.Equal(model.NewInt(12)) {
		t.Errorf("arg 3 expected TabTo(12), got %v", w1.Args[3])
	}
	if w1.Args[4].Type != model.WriteArgValue || !w1.Args[4].Value.Equal(model.NewVariable("<id>")) {
		t.Errorf("arg 4 expected Value(<id>), got %v", w1.Args[4])
	}
	if w1.Args[9].Type != model.WriteArgCRLF {
		t.Errorf("arg 9 expected CRLF, got %v", w1.Args[9].Type)
	}

	// Action 2: (write crlf "Done" crlf)
	w2, ok := r.Actions[1].(model.WriteAction)
	if !ok {
		t.Fatalf("expected WriteAction, got %T", r.Actions[1])
	}
	if len(w2.Args) != 3 {
		t.Fatalf("expected 3 write args in action 2, got %d", len(w2.Args))
	}
	if w2.Args[0].Type != model.WriteArgCRLF || w2.Args[1].Type != model.WriteArgValue || w2.Args[2].Type != model.WriteArgCRLF {
		t.Errorf("unexpected args in action 2: %v", w2.Args)
	}
}

