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
