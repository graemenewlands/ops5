# OPS5 Go Runtime & Engine - Agent Context & Guidelines

> **Project Identity**: High-performance, pure Go implementation of Charles Forgy's classic **OPS5** production rule system using the **Rete** pattern matching algorithm.
> **Current Version**: `v0.3.1` (tagged, pushed, and indexed on `proxy.golang.org` / `pkg.go.dev`).

---

## 1. Architectural Overview & Package Layout

```
pkg/
├── model/        # Domain types: WME, ClassSchema, Value, Rule, ConditionElement, Action
├── wm/           # Dynamic WorkingMemory, timetag generation, mutation listeners
├── rete/         # Alpha/Beta discrimination networks, tokens, unlinking, indexing, Rete-NT
├── conflict/     # Conflict Set (agenda), LEX/MEA strategies, indexed binary heap, refraction
├── engine/       # Match-Resolve-Act lifecycle, compiled RHS actions, ParaOPS5 partitioned concurrency
├── parser/       # S-expression lexer and parser for OPS5 statements and value functions
├── cli/          # Interactive terminal REPL, line editor, auto-completion, table formatting
└── harness/      # Declarative JSON test runner and assertion checker
```

---

## 2. Completed Optimizations (OPT-1 through OPT-7)

All seven next-generation optimizations from `docs/optimizations.md` are fully implemented, verified, and benchmarked:

1. **OPT-1 (Token Prefix Spine Sharing)**: Linked ancestor spine replaces map cloning; zero-allocation variable lookups (`Token.GetBinding`) and single-pass slice materialization (`Timetags`, `WMEs`).
2. **OPT-2 (Binary Heap Agenda)**: $O(1)$ dominant activation peek via indexed max-heap priority queue; $O(\log K)$ push/remove; zero-allocation LEX/MEA comparators with pre-sorted timetags.
3. **OPT-3 (Alpha Switch Nodes)**: $O(1)$ constant-value hash switch dispatch (`AlphaSwitchNode`) with discrimination tree sharing across rules.
4. **OPT-4 (Static Heuristic Join Ordering)**: Compile-time LHS condition reordering via variable-binding graph; eliminates Cartesian cross-products while anchoring Condition 1 for MEA.
5. **OPT-5 (Memoryless Terminal Joins / Rete-NT)**: Terminal beta joins feed directly to `TerminalNode`, bypassing intermediate `BetaMemory` storage and allocations. Lazy promotion when longer rules share prefixes.
6. **OPT-6 (Precompiled RHS Action Closures)**: Actions precompile into typed Go closures (`CompiledAction`), eliminating AST switching and map copying during firing; recycled `ActionContext` via `sync.Pool`.
7. **OPT-7 (Partitioned Multi-Core Concurrency / ParaOPS5)**: `PartitionedEngine` & `Partition` support isolated subnetwork concurrency, cross-partition message channels, dynamic `PartitionRouter`, parallel batch assertions (`MakeBatch`), and atomic global quiescence detection (`RunParallel`).

---

## 3. Critical Invariants & Constraints

When modifying or extending this codebase, adhere strictly to these invariants:

1. **Zero Contention Default**: Standard `engine.New()` instances MUST remain single-threaded by default (`alphaWorkers = 0`) to preserve sub-millisecond execution times for classic sequential workloads.
2. **MEA Goal-Directed Invariant**: In `rete.OptimizeRuleJoinOrder`, Condition 1 ($C_1$) MUST remain anchored at index 0 to guarantee correct MEA conflict resolution semantics.
3. **Refraction Invariant**: Rules fire at most once on any specific combination of WME timetags. Fired keys are recorded in the conflict set refraction table.
4. **OPS5 Modification Semantics**: In `wm.WorkingMemory.Modify`, an update is strictly modeled as retracting the old WME followed by asserting a new WME with an incremented timetag.
5. **Zero-Allocation Hot Paths**: Beta memory indexing, token spine traversal, heap comparators, and RHS variable lookups must not allocate on the heap during execution cycles.
6. **Thread Safety**: All stateful components (`WorkingMemory`, `Network`, `BetaMemory`, `AlphaMemory`, `Set`, `Engine`) synchronize mutations via `sync.RWMutex`.
7. **Push Constraint**: **NEVER push commits or tags to `origin` without explicit user permission.**

---

## 4. Verification & Testing Commands

Always run verification before concluding any work:

```bash
# 1. Fast race test suite across all 13 packages (~7s):
go test -short -race -count=1 ./...

# 2. Run package examples and verify documentation output:
go test -v -run=Example ./pkg/...

# 3. Canonical academic benchmark suite (Manners-16/32, Waltz-12/50, Zebra-5):
go test -v -short ./benchmarks

# 4. Full benchmark suite (including combinatorial Manners-64):
go test -v -run TestBenchmarkSuite ./benchmarks

# 5. Throughput microbenchmarks:
go test -bench=. -benchtime=1x -run=^$ ./benchmarks
```

---

## 5. Performance Baseline Summary (`v0.3.1`)

Tested on 12th Gen Intel Core i7-1260P, Ubuntu 24.04, Go 1.23:

| Benchmark | Quiescence Time | Speedup vs Baseline | Total Heap Allocated | Memory Reduction |
| :--- | :--- | :--- | :--- | :--- |
| **Manners-16** | **316 ms** | **5.5x faster** | 117 MB | -88% |
| **Manners-32** | **598 ms** | **6.4x faster** | 245 MB | -88% |
| **Manners-64** | **6.22 s** | **5.5x faster** | 2.40 GB | -88% (saved 18.2 GB) |
| **Waltz-12** | **11 ms** | **5.9x faster** | 5.5 MB | -86% |
| **Waltz-50** | **52 ms** | **12.7x faster** | 21.4 MB | -95% (saved 426 MB) |
| **Zebra-5** | **0.08 ms** | **2.4x faster** | 68 KB | -49% |
