package parser

import (
	"fmt"
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

func TestParseBindAndCompute(t *testing.T) {
	src := `
	(p compute-test
	   (item ^price <p> ^tax-rate <r>)
	   -->
	   (bind <tax> (compute <p> * <r>))
	   (bind <total> (compute <p> + (compute <p> * <r>)))
	   (bind <step> 1)
	   (make invoice ^amount <total> ^status "paid")
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
	if len(r.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(r.Actions))
	}

	// Action 1: (bind <tax> (compute <p> * <r>))
	b1, ok := r.Actions[0].(model.BindAction)
	if !ok {
		t.Fatalf("action 0 not BindAction: %T", r.Actions[0])
	}
	if b1.Variable != "tax" {
		t.Errorf("expected tax, got %s", b1.Variable)
	}
	if !b1.Value.IsCompute() {
		t.Fatalf("expected b1.Value to be compute expr")
	}

	// Action 2: (bind <total> (compute <p> + (compute <p> * <r>)))
	b2, ok := r.Actions[1].(model.BindAction)
	if !ok {
		t.Fatalf("action 1 not BindAction: %T", r.Actions[1])
	}
	if b2.Variable != "total" {
		t.Errorf("expected total, got %s", b2.Variable)
	}
	if !b2.Value.IsCompute() {
		t.Fatalf("expected b2.Value to be compute expr")
	}
	c2 := b2.Value.ComputeExpr()
	if len(c2.Operands) != 2 || len(c2.Operators) != 1 {
		t.Fatalf("expected 2 operands and 1 operator in outer compute")
	}
	if !c2.Operands[1].IsCompute() {
		t.Fatalf("expected operand 1 to be nested compute")
	}

	// Action 3: (bind <step> 1)
	b3, ok := r.Actions[2].(model.BindAction)
	if !ok {
		t.Fatalf("action 2 not BindAction: %T", r.Actions[2])
	}
	if b3.Variable != "step" || !b3.Value.Equal(model.NewInt(1)) {
		t.Errorf("expected step = 1, got %v = %v", b3.Variable, b3.Value)
	}

	// Action 4: (make invoice ^amount <total> ^status "paid")
	m, ok := r.Actions[3].(model.MakeAction)
	if !ok {
		t.Fatalf("action 3 not MakeAction: %T", r.Actions[3])
	}
	if m.Class != "invoice" {
		t.Errorf("expected invoice, got %s", m.Class)
	}
}

func TestParseCBind(t *testing.T) {
	src := `
(p test-cbind
   (goal ^status start)
   -->
   (make person ^name "Alice")
   (cbind <p>)
   (modify <p> ^age 30)
   (cbind p2)
)
`
	rules, err := ParseRules(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if len(r.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(r.Actions))
	}

	cb1, ok := r.Actions[1].(model.CBindAction)
	if !ok {
		t.Fatalf("action 1 not CBindAction: %T", r.Actions[1])
	}
	if cb1.Variable != "p" {
		t.Errorf("expected variable p, got %s", cb1.Variable)
	}

	cb2, ok := r.Actions[3].(model.CBindAction)
	if !ok {
		t.Fatalf("action 3 not CBindAction: %T", r.Actions[3])
	}
	if cb2.Variable != "p2" {
		t.Errorf("expected variable p2, got %s", cb2.Variable)
	}

	// Error test: missing argument
	errSrc := `(p err-rule (goal) --> (cbind))`
	if _, err := ParseRules(errSrc); err == nil {
		t.Errorf("expected error for (cbind) with no argument")
	}

	// Error test: extra argument
	errSrc2 := `(p err-rule (goal) --> (cbind <p> <extra>))`
	if _, err := ParseRules(errSrc2); err == nil {
		t.Errorf("expected error for (cbind <p> <extra>) with extra argument")
	}
}

func TestLexerVerticalBarSymbol(t *testing.T) {
	input := `|RuleTrace.ops| |Hello World| |simple|`
	l := NewLexer(input)
	tok1, err := l.NextToken()
	if err != nil || tok1.Type != TokenSymbol || tok1.Value != "RuleTrace.ops" {
		t.Fatalf("expected RuleTrace.ops symbol, got %v err=%v", tok1, err)
	}
	tok2, err := l.NextToken()
	if err != nil || tok2.Type != TokenSymbol || tok2.Value != "Hello World" {
		t.Fatalf("expected Hello World symbol, got %v err=%v", tok2, err)
	}
	tok3, err := l.NextToken()
	if err != nil || tok3.Type != TokenSymbol || tok3.Value != "simple" {
		t.Fatalf("expected simple symbol, got %v err=%v", tok3, err)
	}

	unterminated := `|unclosed`
	l2 := NewLexer(unterminated)
	if _, err := l2.NextToken(); err == nil {
		t.Fatalf("expected error for unterminated vertical bar symbol")
	}
}

func TestParseFileIOAndAccept(t *testing.T) {
	src := `
(p file-rule
   (goal ^status start)
   -->
   (openfile ruletrace |RuleTrace.ops| out)
   (default ruletrace accept)
   (bind <user-val> (accept))
   (bind <line-val> (acceptline ruletrace))
   (write (accept) (crlf))
   (closefile ruletrace)
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
	if len(r.Actions) != 6 {
		t.Fatalf("expected 6 actions, got %d", len(r.Actions))
	}

	// 0: openfile
	a0, ok := r.Actions[0].(model.OpenFileAction)
	if !ok || a0.LogicalName != "ruletrace" || a0.Filespec.String() != "RuleTrace.ops" || a0.Mode != "out" {
		t.Errorf("unexpected a0: %+v", r.Actions[0])
	}

	// 1: default
	a1, ok := r.Actions[1].(model.DefaultAction)
	if !ok || a1.LogicalName != "ruletrace" || a1.Subsystem != "accept" {
		t.Errorf("unexpected a1: %+v", r.Actions[1])
	}

	// 2: bind <user-val> (accept)
	a2, ok := r.Actions[2].(model.BindAction)
	if !ok || a2.Variable != "user-val" || !a2.Value.IsAccept() {
		t.Errorf("unexpected a2: %+v", r.Actions[2])
	}
	if a2.Value.AcceptExpr().LogicalFile != "" || a2.Value.AcceptExpr().IsLine {
		t.Errorf("unexpected accept expr: %+v", a2.Value.AcceptExpr())
	}

	// 3: bind <line-val> (acceptline ruletrace)
	a3, ok := r.Actions[3].(model.BindAction)
	if !ok || a3.Variable != "line-val" || !a3.Value.IsAccept() {
		t.Errorf("unexpected a3: %+v", r.Actions[3])
	}
	if a3.Value.AcceptExpr().LogicalFile != "ruletrace" || !a3.Value.AcceptExpr().IsLine {
		t.Errorf("unexpected acceptline expr: %+v", a3.Value.AcceptExpr())
	}

	// 4: write (accept) (crlf)
	a4, ok := r.Actions[4].(model.WriteAction)
	if !ok || len(a4.Args) != 2 || !a4.Args[0].Value.IsAccept() {
		t.Errorf("unexpected a4: %+v", r.Actions[4])
	}

	// 5: closefile
	a5, ok := r.Actions[5].(model.CloseFileAction)
	if !ok || a5.LogicalName != "ruletrace" {
		t.Errorf("unexpected a5: %+v", r.Actions[5])
	}
}

func TestParseGenatom(t *testing.T) {
	src := `
	(p test-genatom
	   (goal ^status active)
	   -->
	   (bind <id> (genatom))
	   (make task ^id (genatom) ^parent <id>)
	   (modify 1 ^id (genatom))
	   (write "New atom:" (genatom) (crlf))
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
	if len(r.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(r.Actions))
	}

	// 0: bind <id> (genatom)
	b, ok := r.Actions[0].(model.BindAction)
	if !ok || b.Variable != "id" || !b.Value.IsGenatom() {
		t.Errorf("expected bind action with genatom, got %+v", r.Actions[0])
	}

	// 1: make task ^id (genatom) ^parent <id>
	m, ok := r.Actions[1].(model.MakeAction)
	if !ok || !m.Attributes["id"].IsGenatom() {
		t.Errorf("expected make action with genatom, got %+v", r.Actions[1])
	}

	// 2: modify 1 ^id (genatom)
	mod, ok := r.Actions[2].(model.ModifyAction)
	if !ok || !mod.Attributes["id"].IsGenatom() {
		t.Errorf("expected modify action with genatom, got %+v", r.Actions[2])
	}

	// 3: write "New atom:" (genatom) (crlf)
	w, ok := r.Actions[3].(model.WriteAction)
	if !ok || len(w.Args) != 3 || !w.Args[1].Value.IsGenatom() {
		t.Errorf("expected write action with genatom, got %+v", r.Actions[3])
	}
}

func TestParseLitval(t *testing.T) {
	src := `
	(p test-litval
		(City ^name <n>)
		-->
		(bind <idx> (litval name))
		(bind <idx2> (litval ^state))
		(bind <idx3> (litval City name))
		(make Record ^slot (litval name))
		(write (litval name) (crlf))
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
	if len(r.Actions) != 5 {
		t.Fatalf("expected 5 actions, got %d", len(r.Actions))
	}

	// 0: bind <idx> (litval name)
	b0, ok := r.Actions[0].(model.BindAction)
	if !ok || b0.Variable != "idx" || !b0.Value.IsLitval() {
		t.Fatalf("expected bind with litval, got %+v", r.Actions[0])
	}
	if b0.Value.LitvalExpr().Class != "" || b0.Value.LitvalExpr().Attribute.String() != "name" {
		t.Errorf("expected litval attribute 'name', got %+v", b0.Value.LitvalExpr())
	}

	// 1: bind <idx2> (litval ^state)
	b1, ok := r.Actions[1].(model.BindAction)
	if !ok || b1.Variable != "idx2" || !b1.Value.IsLitval() {
		t.Fatalf("expected bind with litval, got %+v", r.Actions[1])
	}
	if b1.Value.LitvalExpr().Attribute.String() != "state" {
		t.Errorf("expected litval attribute 'state', got %+v", b1.Value.LitvalExpr())
	}

	// 2: bind <idx3> (litval City name)
	b2, ok := r.Actions[2].(model.BindAction)
	if !ok || b2.Variable != "idx3" || !b2.Value.IsLitval() {
		t.Fatalf("expected bind with litval, got %+v", r.Actions[2])
	}
	if b2.Value.LitvalExpr().Class != "City" || b2.Value.LitvalExpr().Attribute.String() != "name" {
		t.Errorf("expected litval City name, got %+v", b2.Value.LitvalExpr())
	}
}

func TestParseConditionElementConjunctions(t *testing.T) {
	src := `
	(p test-conjunctions
		{(Start) <initialize>}
		{(Request ^type ancestor ^target {<myparents> <> nil}) <req>}
		(Counter {<c> > 0 < 100})
		-->
		(remove <initialize>)
		(remove <req>)
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

	if len(rule.Conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(rule.Conditions))
	}

	// CE 1: {(Start) <initialize>}
	ce1 := rule.Conditions[0]
	if ce1.Class != "Start" || ce1.ElementVariable != "initialize" {
		t.Errorf("expected Start with elemVar initialize, got %v", ce1)
	}

	// CE 2: {(Request ^type ancestor ^target {<myparents> <> nil}) <req>}
	ce2 := rule.Conditions[1]
	if ce2.Class != "Request" || ce2.ElementVariable != "req" {
		t.Errorf("expected Request with elemVar req, got %v", ce2)
	}
	var targetTest *model.AttributeTest
	for i := range ce2.Tests {
		if ce2.Tests[i].Attribute == "target" {
			targetTest = &ce2.Tests[i]
			break
		}
	}
	if targetTest == nil || len(targetTest.Constraints) != 2 {
		t.Fatalf("expected 2 constraints on ^target, got %v", targetTest)
	}
	if !targetTest.Constraints[0].Value.IsVariable() || targetTest.Constraints[0].Value.VariableName() != "myparents" {
		t.Errorf("expected first constraint on ^target to be <myparents>, got %v", targetTest.Constraints[0])
	}
	if targetTest.Constraints[1].Op != model.OpNotEqual || !targetTest.Constraints[1].Value.Equal(model.NewSymbol("nil")) {
		t.Errorf("expected second constraint on ^target to be <> nil, got %v", targetTest.Constraints[1])
	}

	// CE 3: (Counter {<c> > 0 < 100}) positional conjunction
	ce3 := rule.Conditions[2]
	if ce3.Class != "Counter" {
		t.Errorf("expected Counter, got %s", ce3.Class)
	}
	if len(ce3.Tests) != 1 || len(ce3.Tests[0].Constraints) != 3 {
		t.Fatalf("expected 1 test with 3 constraints on Counter, got %v", ce3.Tests)
	}
}

func TestParseBareMakeAndAttributeWithoutCaret(t *testing.T) {
	src := `
	(Person ^name Penelope ^mother Jessica ^father Jeremy)
	(Person ^name Jessica mother Mary-Elizabeth ^father Homer)
	`
	stmts, err := ParseProgram(src)
	if err != nil {
		t.Fatalf("failed to parse program: %v", err)
	}

	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}

	// Stmt 1: (Person ^name Penelope ^mother Jessica ^father Jeremy)
	s1 := stmts[0]
	if s1.Type != StmtMake || s1.MakeClass != "Person" {
		t.Fatalf("expected StmtMake Person, got %+v", s1)
	}
	if s1.MakeAttributes["name"].String() != "Penelope" ||
		s1.MakeAttributes["mother"].String() != "Jessica" ||
		s1.MakeAttributes["father"].String() != "Jeremy" {
		t.Errorf("unexpected attrs in stmt 1: %v", s1.MakeAttributes)
	}

	// Stmt 2: (Person ^name Jessica mother Mary-Elizabeth ^father Homer)
	// 'mother' has no caret, but should be recognized as an attribute because Person schema has it
	s2 := stmts[1]
	if s2.Type != StmtMake || s2.MakeClass != "Person" {
		t.Fatalf("expected StmtMake Person, got %+v", s2)
	}
	if s2.MakeAttributes["name"].String() != "Jessica" ||
		s2.MakeAttributes["mother"].String() != "Mary-Elizabeth" ||
		s2.MakeAttributes["father"].String() != "Homer" {
		t.Errorf("unexpected attrs in stmt 2: %v", s2.MakeAttributes)
	}
}

func TestParseIntegration2_4_3_aFile(t *testing.T) {
	stmts, err := ParseProgram(`
(p FindAncestors::Initialize
        {(Start) <initialize>}
    -->
        (remove <initialize>)
        (write (crlf) |Please type the first name of a person|
            (crlf) |whose ancestors you would like to find:|
            (crlf))
        (make Request ^type ancestor ^target (accept))
)

(p PrintAncestors
        {(Request ^type ancestor
            ^target {<myparents> <> nil}) <request1>}
        (Person ^name <myparents> ^mother <mother-name>
            ^father <father-name>)
    -->
        (remove <request1>)
        (write (crlf) <mother-name> and
                      <father-name> are ancestors
                        via <myparents>)
        (make Request ^type ancestor ^target <mother-name>)
        (make Request ^type ancestor ^target <father-name>)    
)

(Person ^name Penelope ^mother Jessica ^father Jeremy)
(Person ^name Jessica mother Mary-Elizabeth ^father Homer)
(Person ^name Jeremy ^mother Jenny ^father Steven)
(Person ^name Steven ^mother Loree)
(Person ^name Loree ^father Jason)
(Person ^name Homer ^mother Stephanie)
`)
	if err != nil {
		t.Fatalf("failed to parse 2_4_3_a.ops5: %v", err)
	}

	rulesCount := 0
	makesCount := 0
	for _, s := range stmts {
		if s.Type == StmtRule {
			rulesCount++
		} else if s.Type == StmtMake {
			makesCount++
		}
	}

	if rulesCount != 2 {
		t.Errorf("expected 2 rules, got %d", rulesCount)
	}
	if makesCount != 6 {
		t.Errorf("expected 6 makes, got %d", makesCount)
	}
}

func TestParseIntegration2_4_3_bFile(t *testing.T) {
	stmts, err := ParseProgram(`
(p FindAncestors::Initialize
        {(Start) <initialize>}
    -->
        (remove <initialize>)
        (write (crlf) |Please type the first name of a person|
            (crlf) |whose ancestors you would like to find:|
            (crlf))
        (make Request ^type ancestor ^target (accept))
)

(p FindAncestors
        (Request ^type ancestor ^target {<name> <> nil})
        (Person ^name <name> ^mother <mother-name> 
            ^father <father-name>)
    -->
        (make Request ^type ancestor ^target <mother-name>)
        (make Request ^type ancestor ^target <father-name>)    
)

(p FindAncestors::Print
        {(Request ^type ancestor ^target {<name> <> nil}) <request1>}
    -->
        (write (crlf) <name> is an ancestor)
        (remove <request1>)
)

(Person ^name Penelope ^mother Jessica ^father Jeremy)
(Person ^name Jessica mother Mary-Elizabeth ^father Homer)
(Person ^name Jeremy ^mother Jenny ^father Steven)
(Person ^name Steven ^mother Loree)
(Person ^name Loree ^father Jason)
(Person ^name Homer ^mother Stephanie)
`)
	if err != nil {
		t.Fatalf("failed to parse 2_4_3_b.ops5: %v", err)
	}

	rulesCount := 0
	makesCount := 0
	for _, s := range stmts {
		if s.Type == StmtRule {
			rulesCount++
		} else if s.Type == StmtMake {
			makesCount++
		}
	}

	if rulesCount != 3 {
		t.Errorf("expected 3 rules, got %d", rulesCount)
	}
	if makesCount != 6 {
		t.Errorf("expected 6 makes, got %d", makesCount)
	}
}

func TestParseIntegration2_4_3_cFile(t *testing.T) {
	stmts, err := ParseProgram(`
(p FindAncestors::Initialize
        {(Start) <initialize>}
    -->
        (remove <initialize>)
        (write (crlf) |Please type the first name of a person|
            (crlf) |whose ancestors you would like to find:|
            (crlf))
        (make Request ^type ancestor ^target (accept))
)

(p FindAncestors
        (Request ^type ancestor ^target {<name> <> nil})
        (Person ^name <name> ^mother <mother-name> 
            ^father <father-name>)
    -->
        (make Request ^type ancestor ^target <mother-name>)
        (make Request ^type ancestor ^target <father-name>)    
)

(p FindAncestors::Print
        {(Request ^type ancestor ^target {<name> <> nil}) <request1>}
    -->
        (write (crlf) <name> is an ancestor)
        (remove <request1>)
)

(p FindAncestors::Stop
        (Request ^type ancestor ^target {<name1> <> nil})
        - (Request ^type ancestor ^target {<> <name1> <> nil})
    -->
        (write (crlf) No More Ancestors (crlf))
        (halt)
)

(Person ^name Penelope ^mother Jessica ^father Jeremy)
(Person ^name Jessica mother Mary-Elizabeth ^father Homer)
(Person ^name Jeremy ^mother Jenny ^father Steven)
(Person ^name Steven ^mother Loree)
(Person ^name Loree ^father Jason)
(Person ^name Homer ^mother Stephanie)
`)
	if err != nil {
		t.Fatalf("failed to parse 2_4_3_c.ops5: %v", err)
	}

	rulesCount := 0
	makesCount := 0
	var stopRule *model.Rule
	for _, s := range stmts {
		if s.Type == StmtRule {
			rulesCount++
			if s.Rule.Name == "FindAncestors::Stop" {
				stopRule = s.Rule
			}
		} else if s.Type == StmtMake {
			makesCount++
		}
	}

	if rulesCount != 4 {
		t.Errorf("expected 4 rules, got %d", rulesCount)
	}
	if makesCount != 6 {
		t.Errorf("expected 6 makes, got %d", makesCount)
	}
	if stopRule == nil {
		t.Fatalf("expected FindAncestors::Stop rule to be parsed")
	}
	if len(stopRule.Conditions) != 2 {
		t.Fatalf("expected 2 conditions in FindAncestors::Stop, got %d", len(stopRule.Conditions))
	}
	if stopRule.Conditions[0].IsNegative {
		t.Errorf("expected condition 1 to be positive")
	}
	if !stopRule.Conditions[1].IsNegative {
		t.Errorf("expected condition 2 to be negative")
	}
}

func TestParseIntegration2_5_1_aFile(t *testing.T) {
	stmts, err := ParseProgram(`
(p FindAncestors::Initialize
        {(Start) <initialize>}
    -->
        (remove <initialize>)
        (write (crlf) |Please type the first name of a person|
            (crlf) |whose ancestors you would like to find:|
            (crlf))
        (make Request ^type ancestor ^target (accept))
)

(p FindAncestors
        (Request ^type ancestor ^target {<name> <> nil})
        (Person ^name <name> ^mother <mother-name> 
            ^father <father-name>)
    -->
        (make Request ^type ancestor ^target <mother-name>)
        (make Request ^type ancestor ^target <father-name>)    
)

(p FindAncestors::Print
        {(Request ^type ancestor ^target {<name> <> nil}) <request1>}
    -->
        (write (crlf) <name> is an ancestor)
        (remove <request1>)
)

(p FindAncestors::Stop
        (Request ^type ancestor ^target {<name1> <> nil})
        - (Request ^type ancestor ^target {<> <name1> <> nil})
    -->
        (write (crlf) No More Ancestors (crlf))
        (halt)
)

(p Initialize
        {(InitWM) <initialize>}
    -->
        (make Person ^name Penelope ^mother Jessica ^father Jeremy)
        (make Person ^name Jessica mother Mary-Elizabeth ^father Homer)
        (make Person ^name Jeremy ^mother Jenny ^father Steven)
        (make Person ^name Steven ^mother Loree)
        (make Person ^name Loree ^father Jason)
        (make Person ^name Homer ^mother Stephanie)
        (remove <initialize>)
)
`)
	if err != nil {
		t.Fatalf("failed to parse 2_5_1_a.ops5: %v", err)
	}

	if len(stmts) != 5 {
		t.Fatalf("expected 5 statements, got %d", len(stmts))
	}

	var initRule *model.Rule
	for _, s := range stmts {
		if s.Type != StmtRule {
			t.Errorf("expected statement to be a rule, got %s", s.Type)
		}
		if s.Rule.Name == "Initialize" {
			initRule = s.Rule
		}
	}

	if initRule == nil {
		t.Fatalf("expected to find rule Initialize")
	}

	if len(initRule.Conditions) != 1 || initRule.Conditions[0].Class != "InitWM" {
		t.Fatalf("expected Initialize condition to match InitWM, got %v", initRule.Conditions)
	}

	if len(initRule.Actions) != 7 {
		t.Fatalf("expected 7 actions in Initialize, got %d", len(initRule.Actions))
	}

	for i := 0; i < 6; i++ {
		makeAct, ok := initRule.Actions[i].(model.MakeAction)
		if !ok {
			t.Fatalf("action %d should be MakeAction, got %T", i, initRule.Actions[i])
		}
		if makeAct.Class != "Person" {
			t.Errorf("action %d class should be Person, got %s", i, makeAct.Class)
		}
	}

	if _, ok := initRule.Actions[6].(model.RemoveAction); !ok {
		t.Fatalf("action 6 should be RemoveAction, got %T", initRule.Actions[6])
	}
}

func TestParseIntegration2_5_2File(t *testing.T) {
	stmts, err := ParseProgram(`
(p FindAncestors::Initialize
        {(Start) <initialize>}
    -->
        (remove <initialize>)
        (write (crlf) |Please type the first name of a person|
            (crlf) |whose ancestors you would like to find:|
            (crlf))
        (make Request ^type ancestor ^target (accept))
)

(p FindAncestors
        (Request ^type ancestor ^target {<name> <> nil})
        (Person ^name <name> ^mother <mother-name> 
            ^father <father-name>)
    -->
        (make Request ^type ancestor ^target <mother-name>)
        (make Request ^type ancestor ^target <father-name>)    
)

(p FindAncestors::Print
        {(Request ^type ancestor ^target {<name> <> nil}) <request1>}
    -->
        (write (crlf) <name> is an ancestor)
        (remove <request1>)
)

(p FindAncestors::Stop
        (Request ^type ancestor ^target {<name1> <> nil})
        - (Request ^type ancestor ^target {<> <name1> <> nil})
    -->
        (write (crlf) No More Ancestors (crlf))
        (halt)
)

(p Initialize
        {(InitWM) <initialize>}
    -->
        (make Person ^name Penelope ^mother Jessica ^father Jeremy)
        (make Person ^name Jessica mother Mary-Elizabeth ^father Homer)
        (make Person ^name Jeremy ^mother Jenny ^father Steven)
        (make Person ^name Steven ^mother Loree)
        (make Person ^name Loree ^father Jason)
        (make Person ^name Homer ^mother Stephanie)
        (remove <initialize>)
)

(literalize TestCase
    type
    name
)

(p Test::Ancestor:null
        {(TestCase ^type nulldb ^name ancestornull) <nulltest>}
    -->
        (remove <nulltest>)
        (make Start)
)

(p Test::Ancestor:Single
        {(TestCase ^type singledb ^name ancestorsingle) <test>}
    -->
        (remove <test>)
        (make Person ^name Orphan)
        (make Request ^type ancestor ^target Orphan)
)

(p Test::Ancestor:General
        {(TestCase ^type generaldb ^name ancestorgeneral) <gentest>}
    -->
        (remove <gentest>)
        (make Person ^name Penelope ^mother Jessica ^father Jeremy)
        (make Person ^name Jessica mother Mary-Elizabeth ^father Homer)
        (make Person ^name Jeremy ^mother Jenny ^father Steven)
        (make Request ^type ancestor ^target Penelope)
)
`)
	if err != nil {
		t.Fatalf("failed to parse 2_5_2.ops5: %v", err)
	}

	if len(stmts) != 9 {
		t.Fatalf("expected 9 statements (8 rules + 1 schema), got %d", len(stmts))
	}

	litCount := 0
	rulesCount := 0
	for _, s := range stmts {
		if s.Type == StmtLiteralize {
			litCount++
			if s.LiteralizeClass != "TestCase" {
				t.Errorf("expected literalize class TestCase, got %s", s.LiteralizeClass)
			}
			if len(s.LiteralizeAttrs) != 2 || s.LiteralizeAttrs[0] != "type" || s.LiteralizeAttrs[1] != "name" {
				t.Errorf("unexpected literalize attributes: %v", s.LiteralizeAttrs)
			}
		} else if s.Type == StmtRule {
			rulesCount++
		}
	}

	if litCount != 1 {
		t.Errorf("expected 1 literalize statement, got %d", litCount)
	}
	if rulesCount != 8 {
		t.Errorf("expected 8 rules, got %d", rulesCount)
	}
}

func TestParseExcise(t *testing.T) {
	src := `(excise rule-1 rule-2 rule-3)`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	stmt, err := p.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse excise statement: %v", err)
	}
	if stmt.Type != StmtExcise {
		t.Fatalf("expected statement type StmtExcise, got %v", stmt.Type)
	}
	if len(stmt.ExciseRules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(stmt.ExciseRules))
	}
	expected := []string{"rule-1", "rule-2", "rule-3"}
	for i, name := range expected {
		if stmt.ExciseRules[i] != name {
			t.Errorf("expected rule %d to be %s, got %s", i, name, stmt.ExciseRules[i])
		}
	}

	// Test single rule excise
	p2, _ := NewParser(`(excise solo-rule)`)
	stmt2, err := p2.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse single excise: %v", err)
	}
	if len(stmt2.ExciseRules) != 1 || stmt2.ExciseRules[0] != "solo-rule" {
		t.Fatalf("expected solo-rule, got %v", stmt2.ExciseRules)
	}

	// Test empty excise error
	p3, _ := NewParser(`(excise)`)
	_, err = p3.NextStatement()
	if err == nil {
		t.Fatalf("expected error for empty (excise), got nil")
	}
}

func TestParsePM(t *testing.T) {
	src := `(pm FindAncestors CheckGoal)`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmt, err := p.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse pm statement: %v", err)
	}
	if stmt.Type != StmtPM {
		t.Fatalf("expected StmtPM, got %v", stmt.Type)
	}
	if len(stmt.PMRules) != 2 || stmt.PMRules[0] != "FindAncestors" || stmt.PMRules[1] != "CheckGoal" {
		t.Fatalf("unexpected PMRules: %v", stmt.PMRules)
	}

	// Test wildcard (pm *)
	pStar, _ := NewParser(`(pm *)`)
	stmtStar, err := pStar.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse (pm *): %v", err)
	}
	if stmtStar.Type != StmtPM || len(stmtStar.PMRules) != 1 || stmtStar.PMRules[0] != "*" {
		t.Fatalf("unexpected PMRules for wildcard: %v", stmtStar.PMRules)
	}

	// Test empty (pm)
	pEmpty, _ := NewParser(`(pm)`)
	stmtEmpty, err := pEmpty.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse empty (pm): %v", err)
	}
	if stmtEmpty.Type != StmtPM || len(stmtEmpty.PMRules) != 0 {
		t.Fatalf("expected empty PMRules for (pm), got %v", stmtEmpty.PMRules)
	}
}

func TestParseTopLevelRemove(t *testing.T) {
	// (remove *)
	pWild, err := NewParser(`(remove *)`)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmtWild, err := pWild.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse (remove *): %v", err)
	}
	if stmtWild.Type != StmtRemove || !stmtWild.RemoveWildcard {
		t.Fatalf("expected StmtRemove with RemoveWildcard true, got %+v", stmtWild)
	}

	// (remove 1 2 3)
	pTags, _ := NewParser(`(remove 10 20 30)`)
	stmtTags, err := pTags.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse (remove 10 20 30): %v", err)
	}
	if stmtTags.Type != StmtRemove || stmtTags.RemoveWildcard || len(stmtTags.RemoveTimetags) != 3 {
		t.Fatalf("expected 3 timetags, got %+v", stmtTags)
	}
	if stmtTags.RemoveTimetags[0] != 10 || stmtTags.RemoveTimetags[1] != 20 || stmtTags.RemoveTimetags[2] != 30 {
		t.Fatalf("unexpected timetags: %v", stmtTags.RemoveTimetags)
	}

	// Empty (remove) error
	pErr, _ := NewParser(`(remove)`)
	_, err = pErr.NextStatement()
	if err == nil {
		t.Fatalf("expected error for empty (remove), got nil")
	}
}

func TestParseRuleRemoveWildcard(t *testing.T) {
	ruleSrc := `
	(p clear-all
		(cleanup)
		-->
		(remove *)
	)
	`
	p, err := NewParser(ruleSrc)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	rule, err := p.ParseRule()
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if len(rule.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(rule.Actions))
	}
	rmAct, ok := rule.Actions[0].(model.RemoveAction)
	if !ok {
		t.Fatalf("expected RemoveAction, got %T", rule.Actions[0])
	}
	if !rmAct.Wildcard {
		t.Fatalf("expected RemoveAction.Wildcard to be true")
	}
	if rmAct.String() != "(remove *)" {
		t.Fatalf("expected String() to be '(remove *)', got %q", rmAct.String())
	}
}

func TestParseWatchTopLevel(t *testing.T) {
	// 1. (watch) -> nil Level
	pQuery, err := NewParser(`(watch)`)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmtQuery, err := pQuery.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse (watch): %v", err)
	}
	if stmtQuery.Type != StmtWatch || stmtQuery.WatchLevel != nil {
		t.Fatalf("expected StmtWatch with nil WatchLevel, got %+v", stmtQuery)
	}

	// 2. (watch 0), (watch 1), (watch 2)
	for _, lvl := range []int{0, 1, 2} {
		p, err := NewParser(fmt.Sprintf(`(watch %d)`, lvl))
		if err != nil {
			t.Fatalf("failed to create parser for level %d: %v", lvl, err)
		}
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("failed to parse (watch %d): %v", lvl, err)
		}
		if stmt.Type != StmtWatch || stmt.WatchLevel == nil || *stmt.WatchLevel != lvl {
			t.Fatalf("expected StmtWatch with WatchLevel %d, got %+v", lvl, stmt)
		}
	}

	// 3. Invalid watch levels
	for _, invalid := range []string{`(watch 3)`, `(watch -1)`, `(watch abc)`} {
		p, _ := NewParser(invalid)
		_, err := p.NextStatement()
		if err == nil {
			t.Fatalf("expected error for %s, got nil", invalid)
		}
	}
}

func TestParseRuleWatchAction(t *testing.T) {
	ruleSrc := `
	(p configure-tracing
		(start)
		-->
		(watch 2)
		(watch)
	)
	`
	p, err := NewParser(ruleSrc)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	rule, err := p.ParseRule()
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if len(rule.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(rule.Actions))
	}
	act1, ok := rule.Actions[0].(model.WatchAction)
	if !ok || act1.Level == nil || *act1.Level != 2 {
		t.Fatalf("expected WatchAction with level 2, got %#v", rule.Actions[0])
	}
	if act1.String() != "(watch 2)" {
		t.Fatalf("expected (watch 2), got %q", act1.String())
	}
	act2, ok := rule.Actions[1].(model.WatchAction)
	if !ok || act2.Level != nil {
		t.Fatalf("expected WatchAction with nil level, got %#v", rule.Actions[1])
	}
	if act2.String() != "(watch)" {
		t.Fatalf("expected (watch), got %q", act2.String())
	}
}

func TestParsePPWM(t *testing.T) {
	// 1. Basic (ppwm City ^state Pennsylvania)
	src1 := `(ppwm City ^state Pennsylvania)`
	p1, err := NewParser(src1)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmt1, err := p1.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse ppwm: %v", err)
	}
	if stmt1.Type != StmtPPWM || stmt1.PPWMPattern == nil {
		t.Fatalf("expected StmtPPWM, got %+v", stmt1)
	}
	if stmt1.PPWMPattern.Class != "City" {
		t.Errorf("expected class City, got %s", stmt1.PPWMPattern.Class)
	}
	if len(stmt1.PPWMPattern.Tests) != 1 || stmt1.PPWMPattern.Tests[0].Attribute != "state" {
		t.Fatalf("expected 1 test on state, got %+v", stmt1.PPWMPattern.Tests)
	}
	if stmt1.PPWMPattern.Tests[0].Constraints[0].Value.String() != "Pennsylvania" {
		t.Errorf("expected Pennsylvania, got %s", stmt1.PPWMPattern.Tests[0].Constraints[0].Value.String())
	}

	// 2. Inner parens (ppwm (City ^state Pennsylvania))
	src2 := `(ppwm (City ^state Pennsylvania))`
	p2, err := NewParser(src2)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmt2, err := p2.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse ppwm with inner parens: %v", err)
	}
	if stmt2.PPWMPattern.Class != "City" || len(stmt2.PPWMPattern.Tests) != 1 {
		t.Fatalf("unexpected pattern: %+v", stmt2.PPWMPattern)
	}

	// 3. (ppwm City) without attributes
	src3 := `(ppwm City)`
	p3, err := NewParser(src3)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	stmt3, err := p3.NextStatement()
	if err != nil {
		t.Fatalf("failed to parse (ppwm City): %v", err)
	}
	if stmt3.PPWMPattern.Class != "City" || len(stmt3.PPWMPattern.Tests) != 0 {
		t.Fatalf("expected City with 0 tests, got %+v", stmt3.PPWMPattern)
	}

	// 4. (ppwm *) and (ppwm) wildcard
	for _, wildcardSrc := range []string{`(ppwm *)`, `(ppwm)`} {
		p, err := NewParser(wildcardSrc)
		if err != nil {
			t.Fatalf("failed to create parser for %s: %v", wildcardSrc, err)
		}
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("failed to parse %s: %v", wildcardSrc, err)
		}
		if stmt.PPWMPattern.Class != "*" {
			t.Errorf("expected wildcard class '*' for %s, got %s", wildcardSrc, stmt.PPWMPattern.Class)
		}
	}

	// 5. Positional with schema
	schemaSrc := `
	(literalize City name state population)
	(ppwm City Pittsburgh Pennsylvania 1500000)
	`
	pSchema, err := NewParser(schemaSrc)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	s1, err := pSchema.NextStatement()
	if err != nil || s1.Type != StmtLiteralize {
		t.Fatalf("expected literalize, got %v: %v", s1, err)
	}
	s2, err := pSchema.NextStatement()
	if err != nil || s2.Type != StmtPPWM {
		t.Fatalf("expected ppwm, got %v: %v", s2, err)
	}
	if len(s2.PPWMPattern.Tests) != 3 {
		t.Fatalf("expected 3 positional tests, got %d", len(s2.PPWMPattern.Tests))
	}
	if s2.PPWMPattern.Tests[0].Attribute != "name" || s2.PPWMPattern.Tests[1].Attribute != "state" || s2.PPWMPattern.Tests[2].Attribute != "population" {
		t.Fatalf("unexpected attribute mapping: %+v", s2.PPWMPattern.Tests)
	}

	// 6. Vector attributes
	vecSrc := `
	(vector-attribute coords)
	(ppwm Point ^coords 10 20)
	`
	pVec, err := NewParser(vecSrc)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	v1, err := pVec.NextStatement()
	if err != nil || v1.Type != StmtVectorAttribute {
		t.Fatalf("expected vector-attribute, got %v: %v", v1, err)
	}
	v2, err := pVec.NextStatement()
	if err != nil || v2.Type != StmtPPWM {
		t.Fatalf("expected ppwm, got %v: %v", v2, err)
	}
	if len(v2.PPWMPattern.Tests) != 1 || !v2.PPWMPattern.Tests[0].Constraints[0].Value.IsVector() {
		t.Fatalf("expected vector test, got %+v", v2.PPWMPattern.Tests)
	}

	// 7. Forbidden constructs tests
	forbiddenCases := []struct {
		name string
		src  string
	}{
		{"variable value", `(ppwm City ^state <x>)`},
		{"variable class", `(ppwm <c> ^state Pennsylvania)`},
		{"variable element", `(ppwm <c> (City ^state Pennsylvania))`},
		{"predicate greater", `(ppwm City ^population > 1000)`},
		{"predicate less-equal", `(ppwm City ^population <= 1000)`},
		{"predicate not-equal", `(ppwm City ^state != Pennsylvania)`},
		{"predicate equal op", `(ppwm City ^state = Pennsylvania)`},
		{"quote operator", `(ppwm City ^state //)`},
		{"curly braces", `(ppwm City ^state { PA NY })`},
		{"angle brackets", `(ppwm City ^state <PA>)`},
		{"negation", `(ppwm -(City ^state Pennsylvania))`},
	}

	for _, fc := range forbiddenCases {
		t.Run(fc.name, func(t *testing.T) {
			p, err := NewParser(fc.src)
			if err == nil {
				_, parseErr := p.NextStatement()
				if parseErr == nil {
					t.Fatalf("expected error for forbidden construct %q (%s), but got nil", fc.name, fc.src)
				}
			}
		})
	}
}

func TestParseStrategyTopLevel(t *testing.T) {
	src := `
	(strategy mea)
	(strategy lex)
	(strategy MEA)
	`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	s1, err := p.NextStatement()
	if err != nil || s1.Type != StmtStrategy || s1.Strategy != "MEA" {
		t.Fatalf("expected strategy MEA, got %+v (err: %v)", s1, err)
	}

	s2, err := p.NextStatement()
	if err != nil || s2.Type != StmtStrategy || s2.Strategy != "LEX" {
		t.Fatalf("expected strategy LEX, got %+v (err: %v)", s2, err)
	}

	s3, err := p.NextStatement()
	if err != nil || s3.Type != StmtStrategy || s3.Strategy != "MEA" {
		t.Fatalf("expected strategy MEA, got %+v (err: %v)", s3, err)
	}

	invalidSrc := `(strategy invalid)`
	pInv, err := NewParser(invalidSrc)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	_, err = pInv.NextStatement()
	if err == nil {
		t.Fatalf("expected error for invalid strategy, got nil")
	}
}

func TestParseSubstr(t *testing.T) {
	src := `
	(substr <str> sequence sequence)
	(substr <str> 2 4)
	(substr <str> (compute (litval sequence) + 1) inf)
	(substr 1 ^sequence inf)
	`
	p, err := NewParser(src)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 1: (substr <str> sequence sequence)
	s1, err := p.NextStatement()
	if err != nil || s1.Type != StmtSubstr {
		t.Fatalf("expected StmtSubstr, got %+v (err: %v)", s1, err)
	}
	if !s1.Substr.ElementRef.IsVariable() || s1.Substr.ElementRef.VariableName() != "str" {
		t.Errorf("expected ElementRef <str>, got %v", s1.Substr.ElementRef)
	}
	if s1.Substr.Start.Raw() != "sequence" || s1.Substr.End.Raw() != "sequence" {
		t.Errorf("expected sequence/sequence, got %v / %v", s1.Substr.Start, s1.Substr.End)
	}

	// 2: (substr <str> 2 4)
	s2, err := p.NextStatement()
	if err != nil || s2.Type != StmtSubstr {
		t.Fatalf("expected StmtSubstr, got %+v (err: %v)", s2, err)
	}
	if s2.Substr.Start.Raw() != int64(2) || s2.Substr.End.Raw() != int64(4) {
		t.Errorf("expected 2 / 4, got %v / %v", s2.Substr.Start, s2.Substr.End)
	}

	// 3: (substr <str> (compute (litval sequence) + 1) inf)
	s3, err := p.NextStatement()
	if err != nil || s3.Type != StmtSubstr {
		t.Fatalf("expected StmtSubstr, got %+v (err: %v)", s3, err)
	}
	if !s3.Substr.Start.IsCompute() {
		t.Errorf("expected compute expr for start, got %v", s3.Substr.Start)
	}
	if s3.Substr.End.Raw() != "inf" {
		t.Errorf("expected inf for end, got %v", s3.Substr.End)
	}

	// 4: (substr 1 ^sequence inf)
	s4, err := p.NextStatement()
	if err != nil || s4.Type != StmtSubstr {
		t.Fatalf("expected StmtSubstr, got %+v (err: %v)", s4, err)
	}
	if s4.Substr.ElementRef.Raw() != int64(1) {
		t.Errorf("expected ElementRef 1, got %v", s4.Substr.ElementRef)
	}
	if s4.Substr.Start.Raw() != "sequence" || s4.Substr.End.Raw() != "inf" {
		t.Errorf("expected sequence / inf, got %v / %v", s4.Substr.Start, s4.Substr.End)
	}
}

func TestParseSubstrInRule(t *testing.T) {
	src := `
	(p process-string
	   <sVal> (string ^sequence <first> <second>)
	   -->
	   (bind <head> (substr <sVal> sequence sequence))
	   (bind <next> (compute (litval sequence) + 1))
	   (bind <tail> (substr <sVal> <next> inf))
	   (modify <sVal> ^sequence (substr <sVal> <next> inf))
	)
	`
	rules, err := ParseRules(src)
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	rule := rules[0]
	if len(rule.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(rule.Actions))
	}

	// Action 0: bind <head> (substr <sVal> sequence sequence)
	b0, ok := rule.Actions[0].(model.BindAction)
	if !ok || !b0.Value.IsSubstr() {
		t.Fatalf("expected BindAction with substr, got %T: %v", rule.Actions[0], rule.Actions[0])
	}
	if b0.Variable != "head" {
		t.Errorf("expected var head, got %s", b0.Variable)
	}

	// Action 3: modify <sVal> ^sequence (substr ...)
	m3, ok := rule.Actions[3].(model.ModifyAction)
	if !ok {
		t.Fatalf("expected ModifyAction, got %T: %v", rule.Actions[3], rule.Actions[3])
	}
	seqVal, ok := m3.Attributes["sequence"]
	if !ok || !seqVal.IsSubstr() {
		t.Errorf("expected substr in modify sequence, got %v", seqVal)
	}
}




