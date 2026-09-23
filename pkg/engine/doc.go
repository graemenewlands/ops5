// Package engine provides the core runtime execution coordinator and multi-partition concurrency for OPS5.
//
// The Engine orchestrates the classic OPS5 Match-Resolve-Act execution cycle:
//
//  1. Match: Facts asserted or retracted in WorkingMemory propagate through the Rete pattern-matching network,
//     producing or removing candidate Activations in the Conflict Set.
//
//  2. Resolve: The Conflict Set selects the single dominant Activation according to the active conflict
//     resolution strategy (LEX or MEA), taking into account explicit rule salience, recency, and specificity.
//
//  3. Act: The Engine fires the selected rule, executing its precompiled Right-Hand Side (RHS) actions in sequence.
//     Actions may assert new facts (make), modify existing facts (modify), retract facts (remove), write to
//     output streams (write), bind local variables (bind, cbind), dynamically compile new rules (build), or halt.
//
// This cycle repeats until the Conflict Set reaches quiescence (no pending activations), the engine executes
// a (halt) action, a rule breakpoint is encountered, or the user-specified maximum cycle limit is reached.
//
// # Performance & Architecture
//
//   - Precompiled RHS Closures: Rules are compiled at insertion time into direct Go closures (CompiledAction).
//     This completely eliminates runtime AST type switching and reflection during action execution.
//
//   - Pooled Execution Context: ActionContext instances are recycled via sync.Pool. Variable lookups navigate
//     the token ancestor spine directly without heap-allocating map copies.
//
//   - Parallel Batch Assertion: MakeBatch allows bulk ingestion of facts, optionally distributing alpha network
//     evaluations across a configurable worker goroutine pool (SetAlphaWorkers).
//
//   - Breakpoint & Execution Tracing: Supports fine-grained execution control with single-stepping (Step),
//     rule-level breakpoints (SetBreakpoint), cycle limits, and configurable watch logging levels (0, 1, 2).
//
//   - Stream & File I/O: Full implementation of OPS5 file stream handling (OpenFile, CloseFile, SetDefault,
//     ReadAccept, ReadAcceptLine).
//
// # ParaOPS5: Partitioned Multi-Core Concurrency
//
// For large-scale distributed systems or independent rule domains, PartitionedEngine coordinates multiple
// isolated Rete partitions running concurrently across multi-core processors:
//   - Each Partition operates its own Engine, WorkingMemory, and ConflictSet with zero lock contention.
//   - Cross-partition fact communication is routed dynamically via PartitionRouter through buffered channels.
//   - RunParallel executes all partition worker loops concurrently and uses atomic in-flight counters and inbox
//     inspection to detect true global quiescence safely without premature termination.
//
// # Example Usage
//
//	// 1. Initialize engine with LEX conflict resolution
//	eng := engine.New()
//	eng.SetStrategy(conflict.StrategyLEX)
//
//	// 2. Parse and register rules
//	rules, err := parser.ParseRules(`
//	    (p process-order
//	       <order> (order ^status pending ^id <id>)
//	       -->
//	       (modify <order> ^status processed)
//	       (write "Order processed: " <id> (crlf))
//	    )
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, r := range rules {
//	    eng.AddRule(r)
//	}
//
//	// 3. Assert initial working memory
//	eng.Make("order", map[string]model.Value{
//	    "id":     model.NewInt(1001),
//	    "status": model.NewSymbol("pending"),
//	})
//
//	// 4. Run to quiescence
//	cycles, err := eng.Run(100)
//	fmt.Printf("Engine completed in %d cycles. Halted: %v\n", cycles, eng.IsHalted())
package engine
