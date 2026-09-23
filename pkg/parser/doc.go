// Package parser provides the lexical scanner and parser for the OPS5 S-expression language and statements.
//
// The parser translates human-readable OPS5 source code into the structured domain models defined in package model.
// It supports all standard OPS5 syntactic constructs as well as modern extensions such as rule salience,
// non-conjunctive conditions (NCC), existential conditions, accumulate aggregations, and procedural test elements.
//
// # Supported Statements
//
//   - (p name [salience N] LHS --> RHS): Production rule declarations with condition elements and actions.
//   - (literalize class attr1 attr2 ...): Class schema declarations defining attribute ordering and indexing.
//   - (vector-attribute attr): Declares attributes that hold variable-length vector sequences.
//   - (make class ^attr1 val1 ...): Standalone working memory assertions executed at load time.
//   - (openfile logical-name filespec mode): Opens a file stream for reading ('in') or writing ('out').
//   - (closefile logical-name): Closes an open file stream.
//   - (default logical-name [subsystem]): Redirects default I/O streams for write or accept.
//
// # Supported Condition Elements (LHS)
//
//   - Positive Conditions: Standard pattern matching against WME classes and attributes (e.g. (goal ^status active)).
//   - Negative Conditions: Negated condition elements prefixed with '-' asserting the absence of matching facts.
//   - Existential Conditions: Prefixed with '?' matching when one or more matching facts exist.
//   - Accumulate Elements: Aggregates subsets of facts using count, sum, min, max, or avg.
//   - Non-Conjunctive Conditions (NCC): Clustered negated condition element blocks ({ - (c1) (c2) ... }).
//   - Procedural Tests: Evaluates Boolean relational tests on bound variables via (test (...)).
//
// # Supported Right-Hand Side (RHS) Actions & Functions
//
//   - Actions: make, modify, remove, write (with crlf and tabto formatting), bind, cbind, halt, and build.
//   - Value Functions: (compute <expr>), (accept [file]), (acceptline [file]), (genatom), (litval class attr),
//     and (substr elem start count).
//
// # Entry Points
//
//   - ParseRules: Parses a string containing one or more production rules, ignoring top-level commands.
//   - ParseProgram: Parses a complete OPS5 script containing mixed rules, schemas, make statements, and I/O commands.
//
// # Example Usage
//
//	// Parse production rules from source text
//	rules, err := parser.ParseRules(`
//	    (p find-unseated-guest [salience 10]
//	       (context ^state seating)
//	       (guest ^name <name> ^seated no)
//	       -->
//	       (write "Found unseated guest: " <name> (crlf))
//	    )
//	`)
//	if err != nil {
//	    log.Fatalf("Parse error: %v", err)
//	}
//	fmt.Printf("Successfully parsed %d rule(s)\n", len(rules))
package parser
