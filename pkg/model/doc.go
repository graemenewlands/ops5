// Package model defines the core domain types and data structures for the OPS5 production rule system.
//
// In OPS5, computation proceeds by matching pattern conditions against facts in working memory
// and firing the right-hand side actions of satisfied rules. This package models all components
// of that domain, including Working Memory Elements (WMEs), condition elements, production rules,
// RHS actions, and strongly-typed values.
//
// # Core Types
//
//   - WME: Working Memory Elements represent asserted facts. Each WME has an immutable, monotonically
//     increasing Timetag, an element Class (e.g. "goal", "stage"), and a set of attribute-value pairs.
//
//   - Value: Strongly-typed, immutable representation of OPS5 data types, including symbols, integers,
//     floating-point numbers, strings, booleans, variables, and vectors, as well as dynamic value
//     functions such as (compute ...), (accept), (genatom), (litval ...), and (substr ...).
//
//   - Rule: Represents a production rule containing a Left-Hand Side (LHS) list of ConditionElements
//     and a Right-Hand Side (RHS) list of Actions, along with priority Salience and documentation metadata.
//
//   - ConditionElement: Condition patterns evaluated against WMEs during Rete pattern matching. Supports
//     positive patterns, negative patterns (negated presence), existential tests, accumulate aggregations,
//     non-conjunctive/NCC blocks, and procedural (test ...) predicate evaluations.
//
//   - Action: Right-hand side actions executed when an instantiated rule fires, including make, modify,
//     remove, write, bind, cbind, halt, build, and user-defined CustomActions.
//
//   - ClassSchema: Declarations established by (literalize ...) and (vector-attribute ...) statements,
//     governing attribute order, indexing, and vector extraction.
//
// # Example Usage
//
//	// Construct a production rule programmatically:
//	rule := model.NewRule("order-completed")
//	rule.SetSalience(10)
//
//	// LHS: positive condition matching an order with pending status
//	ce := model.NewPositiveCE("order").
//	    AddEqualTest("status", model.NewSymbol("pending")).
//	    AddVariableTest("id", model.NewVariable("<oid>"))
//	rule.AddCondition(ce)
//
//	// RHS: modify status and write message
//	rule.AddAction(&model.ModifyAction{
//	    Target: model.NewInt(1),
//	    Attributes: map[string]model.Value{
//	        "status": model.NewSymbol("completed"),
//	    },
//	})
//	rule.AddAction(&model.WriteAction{
//	    Args: []model.WriteArg{
//	        model.WriteValue(model.NewString("Order processed: ")),
//	        model.WriteValue(model.NewVariable("<oid>")),
//	        model.WriteCRLF(),
//	    },
//	})
package model
