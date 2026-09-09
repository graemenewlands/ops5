# OPS5 Go Runtime

A minimal, robust, and idiomatic Go implementation of the classic **OPS5** production rule system using the **Rete** algorithm.

## Features

- **Rete Algorithm Architecture**:
  - **Alpha Network**: Intra-condition filtering (`TypeNode`, `ConstantTestNode`, `AlphaMemory`) with shared nodes across matching patterns.
  - **Beta Network**: Inter-condition joins (`JoinNode`, `BetaMemory`), cross-condition variable binding and equality tests.
  - **Negative Condition Elements**: Support for `- (class ...)` conditions with dynamic blocking and unblocking.
  - **Terminal Nodes**: Activates candidate instantiations for the Conflict Set upon full LHS match.
- **Dynamic Working Memory**:
  - Immutable integer timetags assigned monotonically.
  - `Make`: Asserts a new Working Memory Element (WME) with a new timetag.
  - `Remove`: Retracts a WME by timetag and propagates retraction waves through Rete.
  - `Modify`: OPS5-compliant semantic modification (retracts old WME and asserts a new WME with updated attributes and a new timetag, preserving unmodified fields).
- **Conflict Resolution & Salience**:
  - **Refraction**: Prevents the exact same rule instantiation with identical WME timetags from firing more than once.
  - **LEX Strategy**: Lexicographic recency vector comparison, then rule specificity, then definition index tie-breaker.
  - **MEA Strategy**: Means-Ends Analysis prioritizes the recency of the first condition element (goal CE 1), followed by lexicographical comparison of remaining elements.
- **OPS5 Parser**:
  - Standard S-expression OPS5 syntax tokenizer and parser.
  - Supports `(p <name> <LHS> --> <RHS>)`, `(make <class> [^attr val ...])`, `(modify <var/idx> ...)`, `(remove <var/idx>)`, `(write ...)`, `(halt)`.
- **Test Harness**:
  - Automated test runner capable of executing test cases defined in Go, JSON, or standard OPS5 `.ops` files.
  - Ready to accept external test suites.

## Directory Structure

```
.
├── go.mod
├── pkg/
│   ├── model/         # Core primitives: WME, Value (Symbol, Int, Float, String, Variable), Rule, Actions
│   ├── wm/            # Dynamic Working Memory, timetag generator, listener event dispatch
│   ├── rete/          # Rete Alpha & Beta networks, Tokens, JoinNode, NegativeJoinNode, TerminalNode
│   ├── conflict/      # Conflict Set, Instantiations, LEX & MEA salience computation, Refraction
│   ├── engine/        # Engine coordinating Match-Resolve-Act cycle, step execution, and actions
│   ├── parser/        # OPS5 S-expression Lexer & Parser
│   └── harness/       # Test runner and assertion validator for test suites
└── tests/             # End-to-end integration tests & test fixture suites
    └── fixtures/      # External test cases (.json and .ops)
```

## Running Tests

Run all unit and integration tests:

```bash
go test -v ./...
```

Run test suite with coverage:

```bash
go test -cover ./...
```

## External Test Suite Compatibility

External tests can be supplied either as:
1. **JSON Test Definitions** placed in `tests/fixtures/`:
   ```json
   {
     "name": "my_test",
     "strategy": "LEX",
     "source_file": "fixtures/my_rules.ops",
     "initial_wm": [
       { "class": "goal", "attributes": { "status": "start" } }
     ],
     "expected_cycles": 3,
     "expected_wm": [
       { "class": "goal", "attributes": { "status": "completed" } }
     ]
   }
   ```
2. **Standard `.ops` files** that define rules and initial WME assertions.
