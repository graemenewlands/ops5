package parser

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestParseTextFunctionsInRHS(t *testing.T) {
	ruleSrc := `
	(p test-text-rhs
	   (customer ^phone <p> ^name <n>)
	   -->
	   (bind <prefix> (left <n> 3))
	   (bind <suffix> (right <n> 2))
	   (bind <is-admin> (equal-ignore-case <n> "admin"))
	   (bind <has-foo> (contains <n> "foo"))
	   (bind <has-bar> (contains-ignore-case <n> "BAR"))
	   (bind <formatted> (regex-replace "^(\\d{3})(\\d{4})$" "$1-$2" <p>))
	   (bind <folded> (left "HelloWorld" 5))
	)
	`
	p, err := NewParser(ruleSrc)
	if err != nil {
		t.Fatalf("parser creation failed: %v", err)
	}
	stmt, err := p.NextStatement()
	if err != nil || stmt.Type != StmtRule {
		t.Fatalf("expected StmtRule, got %v (err: %v)", stmt, err)
	}

	rule := stmt.Rule
	if len(rule.Actions) != 7 {
		t.Fatalf("expected 7 actions, got %d", len(rule.Actions))
	}

	// 1. left <n> 3 (dynamic StringExpr)
	b1 := rule.Actions[0].(model.BindAction)
	if !b1.Value.IsStringExpr() || b1.Value.StringExpr().Op != "left" {
		t.Errorf("action 0: expected StringExpr(left), got %v", b1.Value)
	}

	// 2. right <n> 2 (dynamic StringExpr)
	b2 := rule.Actions[1].(model.BindAction)
	if !b2.Value.IsStringExpr() || b2.Value.StringExpr().Op != "right" {
		t.Errorf("action 1: expected StringExpr(right), got %v", b2.Value)
	}

	// 3. equal-ignore-case <n> "admin"
	b3 := rule.Actions[2].(model.BindAction)
	if !b3.Value.IsStringExpr() || b3.Value.StringExpr().Op != "equalsignorecase" {
		t.Errorf("action 2: expected StringExpr(equalsignorecase), got %v", b3.Value)
	}

	// 4. contains <n> "foo"
	b4 := rule.Actions[3].(model.BindAction)
	if !b4.Value.IsStringExpr() || b4.Value.StringExpr().Op != "contains" {
		t.Errorf("action 3: expected StringExpr(contains), got %v", b4.Value)
	}

	// 5. contains-ignore-case <n> "BAR"
	b5 := rule.Actions[4].(model.BindAction)
	if !b5.Value.IsStringExpr() || b5.Value.StringExpr().Op != "containsignorecase" {
		t.Errorf("action 4: expected StringExpr(containsignorecase), got %v", b5.Value)
	}

	// 6. regex-replace
	b6 := rule.Actions[5].(model.BindAction)
	if !b6.Value.IsStringExpr() || b6.Value.StringExpr().Op != "regexreplace" {
		t.Errorf("action 5: expected StringExpr(regexreplace), got %v", b6.Value)
	}

	// 7. Constant folded (left "HelloWorld" 5) -> "Hello"
	b7 := rule.Actions[6].(model.BindAction)
	if b7.Value.Type() != model.TypeString || b7.Value.Raw().(string) != "Hello" {
		t.Errorf("action 6: expected constant folded 'Hello', got %v (%s)", b7.Value, b7.Value.Type())
	}
}

func TestParseOperatorsAndRegexInLHS(t *testing.T) {
	ruleSrc := `
	(p test-text-lhs
	   (user ^role =~ "admin" ^status !~ "inactive")
	   (item ^code (regex "^(?P<country>[A-Z]{2})-(?P<id>[0-9]{4})$"))
	   (order ^sku (regex "^([A-Z]{3})-([0-9]+)$" <cat> <num>))
	   (test <country> =~ "US")
	   (test <id> !~ "0000")
	   -->
	   (make result ^c <country> ^i <id> ^cat <cat> ^n <num>)
	)
	`
	p, err := NewParser(ruleSrc)
	if err != nil {
		t.Fatalf("parser creation failed: %v", err)
	}
	stmt, err := p.NextStatement()
	if err != nil || stmt.Type != StmtRule {
		t.Fatalf("expected StmtRule, got %v (err: %v)", stmt, err)
	}

	rule := stmt.Rule
	if len(rule.Conditions) != 5 {
		t.Fatalf("expected 5 conditions, got %d", len(rule.Conditions))
	}

	// CE 1: =~ and !~
	ce1 := rule.Conditions[0]
	if len(ce1.Tests) != 2 {
		t.Fatalf("CE 1: expected 2 attribute tests, got %d", len(ce1.Tests))
	}
	if ce1.Tests[0].Constraints[0].Op != model.OpEqualIgnoreCase {
		t.Errorf("CE 1 test 0: expected OpEqualIgnoreCase, got %v", ce1.Tests[0].Constraints[0].Op)
	}
	if ce1.Tests[1].Constraints[0].Op != model.OpNotEqualIgnoreCase {
		t.Errorf("CE 1 test 1: expected OpNotEqualIgnoreCase, got %v", ce1.Tests[1].Constraints[0].Op)
	}

	// CE 2: named capture groups
	ce2 := rule.Conditions[1]
	reVal2 := ce2.Tests[0].Constraints[0].Value
	if !reVal2.IsRegex() {
		t.Fatalf("CE 2: expected RegexExpr, got %v", reVal2)
	}
	re2 := reVal2.RegexExpr()
	if len(re2.Variables) != 2 || re2.Variables[0] != "country" || re2.Variables[1] != "id" {
		t.Errorf("CE 2: expected named vars [country id], got %v", re2.Variables)
	}

	// Check that ce2.Variables() includes country and id
	vars2 := ce2.Variables()
	hasCountry := false
	hasID := false
	for _, v := range vars2 {
		if v == "country" {
			hasCountry = true
		}
		if v == "id" {
			hasID = true
		}
	}
	if !hasCountry || !hasID {
		t.Errorf("CE 2 Variables(): expected country and id, got %v", vars2)
	}

	// CE 3: positional capture groups with explicit variables
	ce3 := rule.Conditions[2]
	reVal3 := ce3.Tests[0].Constraints[0].Value
	if !reVal3.IsRegex() {
		t.Fatalf("CE 3: expected RegexExpr, got %v", reVal3)
	}
	re3 := reVal3.RegexExpr()
	if len(re3.Variables) != 2 || re3.Variables[0] != "cat" || re3.Variables[1] != "num" {
		t.Errorf("CE 3: expected positional vars [cat num], got %v", re3.Variables)
	}

	// CE 4: test <country> =~ "US"
	ce4 := rule.Conditions[3]
	if !ce4.IsTest || ce4.EvalTest.Comparisons[0].Op != model.OpEqualIgnoreCase {
		t.Errorf("CE 4: expected test with =~, got %v", ce4)
	}

	// CE 5: test <id> !~ "0000"
	ce5 := rule.Conditions[4]
	if !ce5.IsTest || ce5.EvalTest.Comparisons[0].Op != model.OpNotEqualIgnoreCase {
		t.Errorf("CE 5: expected test with !~, got %v", ce5)
	}
}
