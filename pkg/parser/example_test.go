package parser_test

import (
	"fmt"
	"log"

	"github.com/graemenewlands/ops5/pkg/parser"
)

func ExampleParseRules() {
	source := `
		(p find-available-driver [salience 50]
		   (driver ^id <d> ^status available ^location <loc>)
		   (request ^id <r> ^pickup <loc> ^status unassigned)
		   -->
		   (modify 2 ^status assigned ^driver <d>)
		   (modify 1 ^status busy)
		   (write "Assigned driver" <d> "to request" <r> (crlf))
		)
	`

	rules, err := parser.ParseRules(source)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Parsed %d rule: %s (salience %d, %d conditions, %d actions)\n",
		len(rules), rules[0].Name, rules[0].Salience, len(rules[0].Conditions), len(rules[0].Actions))

	// Output:
	// Parsed 1 rule: find-available-driver (salience 50, 2 conditions, 3 actions)
}

func ExampleParseProgram() {
	source := `
		(literalize order id customer status total)
		(vector-attribute items)

		(make order ^id 1001 ^customer "Alice" ^status pending ^total 49.99)

		(p discount-order
		   <o> (order ^status pending ^total > 40.0)
		   -->
		   (write "Order qualifies for discount" (crlf))
		)
	`

	stmts, err := parser.ParseProgram(source)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Parsed %d statements successfully\n", len(stmts))

	// Output:
	// Parsed 4 statements successfully
}
