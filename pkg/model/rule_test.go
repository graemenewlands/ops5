package model

import (
	"strings"
	"testing"
)

func TestRuleStringFormatting(t *testing.T) {
	rule := NewRule("FindAncestors")
	
	ce1 := NewPositiveCE("Request").
		AddEqualTest("type", NewSymbol("ancestor")).
		AddEqualTest("target", NewVariable("<p>"))
	
	ce2 := NewPositiveCE("Person").
		WithElementVariable("pers").
		AddEqualTest("name", NewVariable("<p>")).
		AddEqualTest("father", NewVariable("<f>"))

	ce3 := NewNegativeCE("Visited").
		AddEqualTest("name", NewVariable("<f>"))

	rule.AddCondition(ce1)
	rule.AddCondition(ce2)
	rule.AddCondition(ce3)

	rule.AddAction(MakeAction{
		Class: "Request",
		Attributes: map[string]Value{
			"type":   NewSymbol("ancestor"),
			"target": NewVariable("<f>"),
		},
	})
	rule.AddAction(ModifyAction{
		TargetElementVar: "pers",
		Attributes: map[string]Value{
			"status": NewSymbol("checked"),
		},
	})
	rule.AddAction(WriteAction{
		Args: []WriteArg{
			WriteCRLF(),
			WriteValue(NewVariable("<f>")),
			WriteValue(NewString("is an ancestor")),
		},
	})
	rule.AddAction(RemoveAction{
		TargetElementVar: "pers",
	})
	rule.AddAction(BindAction{
		Variable: "<val>",
		Value:    NewInt(100),
	})
	rule.AddAction(CBindAction{
		Variable: "<last>",
	})
	rule.AddAction(OpenFileAction{
		LogicalName: "outf",
		Filespec:    NewString("out.txt"),
		Mode:        "out",
	})
	rule.AddAction(CloseFileAction{
		LogicalName: "outf",
	})
	rule.AddAction(DefaultAction{
		LogicalName: "outf",
		Subsystem:   "write",
	})
	rule.AddAction(CustomAction{
		Name: "customHook",
	})
	rule.AddAction(HaltAction{})

	str := rule.String()

	expectedSnippets := []string{
		"(p FindAncestors",
		"(Request ^type ancestor ^target <p>)",
		"<pers> (Person ^name <p> ^father <f>)",
		"-(Visited ^name <f>)",
		"-->",
		"(make Request ^target <f> ^type ancestor)",
		"(modify <pers> ^status checked)",
		`(write (crlf) <f> "is an ancestor")`,
		"(remove <pers>)",
		"(bind <val> 100)",
		"(cbind <last>)",
		`(openfile outf "out.txt" out)`,
		"(closefile outf)",
		"(default outf write)",
		"(call customHook)",
		"(halt)",
		")",
	}

	for _, exp := range expectedSnippets {
		if !strings.Contains(str, exp) {
			t.Errorf("expected rule.String() to contain %q, but got:\n%s", exp, str)
		}
	}
}

func TestConditionElementMatches(t *testing.T) {
	wme := NewWME(1, "City", map[string]Value{
		"name":     NewSymbol("Pittsburgh"),
		"state":    NewSymbol("Pennsylvania"),
		"location": NewVector([]Value{NewFloat(40.44), NewFloat(-79.99)}),
	})

	// 1. Exact match
	ce1 := NewPositiveCE("City").AddEqualTest("state", NewSymbol("Pennsylvania"))
	if !ce1.Matches(wme) {
		t.Errorf("expected ce1 to match wme")
	}

	// 2. Wildcard class
	ce2 := NewPositiveCE("*").AddEqualTest("state", NewSymbol("Pennsylvania"))
	if !ce2.Matches(wme) {
		t.Errorf("expected wildcard class to match wme")
	}

	// 3. Case-insensitive class
	ce3 := NewPositiveCE("city").AddEqualTest("name", NewSymbol("Pittsburgh"))
	if !ce3.Matches(wme) {
		t.Errorf("expected case-insensitive class match")
	}

	// 4. Vector membership match
	ce4 := NewPositiveCE("City").AddEqualTest("location", NewFloat(40.44))
	if !ce4.Matches(wme) {
		t.Errorf("expected vector membership match for 40.44")
	}

	// 5. Vector full equality match
	ce5 := NewPositiveCE("City").AddEqualTest("location", NewVector([]Value{NewFloat(40.44), NewFloat(-79.99)}))
	if !ce5.Matches(wme) {
		t.Errorf("expected vector equality match")
	}

	// 6. Nil attribute match (attribute does not exist on WME)
	ce6 := NewPositiveCE("City").AddEqualTest("country", NewSymbol("nil"))
	if !ce6.Matches(wme) {
		t.Errorf("expected nil attribute match for missing attribute")
	}

	// 7. Non-matching class
	ce7 := NewPositiveCE("Person").AddEqualTest("state", NewSymbol("Pennsylvania"))
	if ce7.Matches(wme) {
		t.Errorf("did not expect Person to match City WME")
	}

	// 8. Non-matching attribute
	ce8 := NewPositiveCE("City").AddEqualTest("state", NewSymbol("Ohio"))
	if ce8.Matches(wme) {
		t.Errorf("did not expect Ohio to match Pennsylvania WME")
	}

	// 9. Nil WME
	if ce1.Matches(nil) {
		t.Errorf("did not expect match against nil WME")
	}
}
