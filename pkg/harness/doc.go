// Package harness provides an automated test runner and JSON test case specification for the OPS5 engine.
//
// The harness package allows developers to write declarative, reproducible JSON test cases that verify
// rule parsing, working memory manipulation, conflict resolution, refraction, and execution termination.
//
// # Test Case Specification (TestCase)
//
// A JSON test case specifies:
//   - Name & Description: Informative metadata identifying the test scenario.
//   - Strategy: The conflict resolution strategy to use ("lex" or "mea").
//   - MaxCycles: Cycle limit preventing infinite loops during faulty rule executions.
//   - Schemas: Initial class definitions and attribute layouts.
//   - Rules: Source strings of OPS5 production rules to compile.
//   - InitialWM: Initial facts to assert before execution begins.
//   - ExpectedWM: Set of WME patterns that must exist in working memory when the run finishes.
//   - ExpectedHalted: Boolean asserting whether the engine should have halted via an explicit (halt) action.
//   - ExpectedCycles: Optional exact or upper-bound cycle assertions.
//
// # Runner & Results
//
// Runner coordinates test execution by:
//  1. Constructing a pristine engine.Engine instance.
//  2. Registering schemas and compiling production rules.
//  3. Asserting initial working memory facts.
//  4. Running the engine until quiescence, halt, or cycle limit.
//  5. Evaluating expected WME assertions against final working memory state.
//  6. Returning a Result containing cycle counts, execution timing, and diagnostic failure messages.
//
// # Example Usage
//
//	runner := harness.NewRunner()
//	result, err := runner.RunFile("tests/fixtures/order_processing.json")
//	if err != nil {
//	    log.Fatalf("Test execution failed: %v", err)
//	}
//	if !result.Passed {
//	    log.Fatalf("Test assertions failed: %s", result.FailureReason)
//	}
//	fmt.Printf("Test passed in %d cycles (%v)\n", result.Cycles, result.Duration)
package harness
