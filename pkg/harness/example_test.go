package harness_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/harness"
)

func ExampleRunner() {
	runner := harness.NewRunner()

	tc := &harness.TestCase{
		Name:     "basic-assertion-test",
		Strategy: "LEX",
		Source: `
			(p mark-complete
			   <t> (task ^id 101 ^status pending)
			   -->
			   (modify <t> ^status complete)
			)
		`,
		InitialWM: []harness.WMEAssertion{
			{Class: "task", Attributes: map[string]string{"id": "101", "status": "pending"}},
		},
		ExpectedWM: []harness.WMEAssertion{
			{Class: "task", Attributes: map[string]string{"id": "101", "status": "complete"}},
		},
	}

	result := runner.Run(tc)
	fmt.Printf("Test: %s, Passed: %v, Cycles: %d\n", result.TestCaseName, result.Passed, result.CyclesRan)

	// Output:
	// Test: basic-assertion-test, Passed: true, Cycles: 1
}
