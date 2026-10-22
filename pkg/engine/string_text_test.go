package engine

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

func TestEngineTextFunctionsAndRegexIntegration(t *testing.T) {
	program := `
	(literalize raw-entry text)
	(literalize user id name role)
	(literalize alert level summary)
	(literalize parsed-sku cat num origin)

	; Rule 1: Regex extraction on LHS with named capture groups + RHS formatting
	(p parse-sku-rule
	   (raw-entry ^text (regex "^SKU:(?P<cat>[A-Z]{3})-(?P<num>[0-9]{4})$"))
	   -->
	   (bind <cat-lower> (regex-replace "^([A-Z]+)$" "cat-$1" <cat>))
	   (make parsed-sku ^cat <cat-lower> ^num <num> ^origin (left <cat> 2))
	)

	; Rule 2: Case-insensitive operator =~ on LHS + text functions on RHS
	(p admin-welcome-rule
	   (user ^id <id> ^name <n> ^role =~ "admin")
	   -->
	   (bind <initial> (left <n> 1))
	   (bind <end> (right <n> 2))
	   (bind <is-exact> (equal-ignore-case <n> "superuser"))
	   (make alert ^level info ^summary (regex-replace "^.*$" "Welcome admin!" <n>))
	)

	; Rule 3: Substring check in (test ...)
	(p check-contains-rule
	   (user ^id <id> ^name <n>)
	   (test (contains-ignore-case <n> "smith"))
	   -->
	   (make alert ^level warning ^summary "Found Smith family member")
	)
	`

	p, err := parser.NewParser(program)
	if err != nil {
		t.Fatalf("parser creation failed: %v", err)
	}

	eng := New()
	eng.SetTrace(false)

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("unexpected parser error: %v", err)
		}
		if stmt == nil {
			break
		}
		if stmt.Type == parser.StmtRule {
			eng.AddRule(stmt.Rule)
		} else if stmt.Type == parser.StmtLiteralize {
			eng.RegisterSchema(stmt.Schema)
		}
	}

	// 1. Assert SKU entry
	eng.Make("raw-entry", map[string]model.Value{
		"text": model.NewString("SKU:USA-1234"),
	})

	// 2. Assert Users
	// Admin with mixed-case role "AdMiN"
	eng.Make("user", map[string]model.Value{
		"id":   model.NewInt(101),
		"name": model.NewSymbol("BobSmith"),
		"role": model.NewSymbol("AdMiN"),
	})
	// Regular user without admin
	eng.Make("user", map[string]model.Value{
		"id":   model.NewInt(102),
		"name": model.NewString("AliceJones"),
		"role": model.NewSymbol("guest"),
	})

	// Run engine to quiescence
	cycles, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("engine run error: %v", err)
	}
	if cycles < 3 {
		t.Errorf("expected at least 3 cycles, got %d", cycles)
	}

	// Verify WMEs produced:
	wmes := eng.WorkingMemory().All()
	var parsedSKUs []*model.WME
	var alerts []*model.WME
	for _, w := range wmes {
		if w.Class == "parsed-sku" {
			parsedSKUs = append(parsedSKUs, w)
		} else if w.Class == "alert" {
			alerts = append(alerts, w)
		}
	}

	// 1. Check parsed-sku
	if len(parsedSKUs) != 1 {
		t.Fatalf("expected 1 parsed-sku, got %d", len(parsedSKUs))
	}
	sku := parsedSKUs[0]
	if sku.Attributes["cat"].Raw().(string) != "cat-USA" {
		t.Errorf("expected cat='cat-USA', got %v", sku.Attributes["cat"])
	}
	if sku.Attributes["num"].Raw().(string) != "1234" {
		t.Errorf("expected num='1234', got %v", sku.Attributes["num"])
	}
	if sku.Attributes["origin"].Raw().(string) != "US" {
		t.Errorf("expected origin='US' (left 2), got %v", sku.Attributes["origin"])
	}

	// 2. Check alerts
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts (welcome admin, found smith), got %d", len(alerts))
	}
	foundAdminWelcome := false
	foundSmithAlert := false
	for _, a := range alerts {
		sum := a.Attributes["summary"].Raw().(string)
		if sum == "Welcome admin!" {
			foundAdminWelcome = true
		}
		if sum == "Found Smith family member" {
			foundSmithAlert = true
		}
	}
	if !foundAdminWelcome {
		t.Errorf("expected admin welcome alert")
	}
	if !foundSmithAlert {
		t.Errorf("expected Smith family alert")
	}
}
