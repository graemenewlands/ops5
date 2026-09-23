// Package rete implements Charles Forgy's Rete pattern-matching algorithm with modern high-performance enhancements.
//
// The Rete algorithm compiles declarative production rules into an acyclic dataflow discrimination network.
// Facts (WMEs) enter the network at the root, and as they satisfy intra-condition tests (Alpha network)
// and inter-condition joins (Beta network), partial match tokens propagate through the topology until
// reaching TerminalNodes, which instantiate rule activations in the conflict set agenda.
//
// # Network Architecture
//
// 1. Alpha Network (Intra-Condition Discrimination):
//   - AlphaRootNode: Broadcast root receiving asserted and retracted WMEs from working memory.
//   - TypeNode: Partitions WMEs by class symbol (e.g. "goal", "block").
//   - ConstantTestNode: Evaluates scalar attribute constraints (=, <>, <, <=, >, >=) and disjunctive value sets.
//   - AlphaSwitchNode: Provides O(1) constant-value hash dispatch, indexing matching child branches in an internal map.
//   - AlphaMemory: Stores WMEs satisfying all intra-condition tests for a single condition element.
//
// 2. Beta Network (Inter-Condition Joins & Aggregations):
//   - BetaMemory: Stores partial match Tokens matching a prefix sequence of condition elements.
//   - JoinNode: Joins left partial tokens with right WMEs on variable equality/inequality constraints.
//   - NegativeJoinNode: Implements negated condition elements (- (pattern ...)), propagating matches only when
//     no matching right WMEs exist.
//   - ExistentialJoinNode: Implements existential tests (? (pattern ...)), matching when at least one fact exists.
//   - AccumulateNode: Computes aggregate statistics (count, sum, min, max, avg) over matched subsets.
//   - EvalNode: Evaluates procedural (test ...) predicate expressions against bound token variables.
//   - NccNode & NccPartnerNode: Coordinates Non-Conjunctive Conditions (conjunctively negated multi-condition blocks).
//   - TerminalNode: Terminal leaf node instantiated for each production rule, notifying the conflict set agenda.
//
// # Advanced Optimizations
//
//   - Token Prefix Spine Sharing: Tokens link to their parent tokens via direct pointer references rather than
//     cloning binding maps. Variable lookups (GetBinding) traverse the ancestor spine with zero heap allocations.
//
//   - Structural Node Sharing: Rules with shared LHS condition prefixes share identical Alpha and Beta nodes,
//     minimizing memory consumption and eliminating redundant evaluations.
//
//   - Node Unlinking: Implements classic Left and Right node unlinking. Join nodes unlink from empty Alpha
//     or Beta memories, allowing activations to skip evaluation in O(1) until memories become non-empty.
//
//   - Dual-Sided Join Indexing: BetaIndex and AlphaIndex hash join keys on both sides, transforming join lookups
//     from O(N*M) Cartesian scans to O(1) bucket lookups.
//
//   - Memoryless Terminal Joins (Rete-NT): Terminal condition joins bypass intermediate BetaMemory storage and
//     feed directly into TerminalNodes, eliminating millions of transient allocations in deep combinatorial searches.
//     Shared nodes are lazily promoted to full BetaMemories if a newly added rule extends that condition prefix.
//
//   - Static Heuristic Join Ordering: OptimizeRuleJoinOrder re-orders condition elements at compile time using a
//     variable-binding graph and bound-variable heuristics to eliminate cross-product bottlenecks.
//
//   - Network Visualization: Network.ExportDOT serializes the live Rete topology into Graphviz DOT syntax.
//
// # Thread Safety
//
// All mutable network components (Network, AlphaMemory, BetaMemory, TerminalNode, and Join nodes) synchronize
// state transitions using internal sync.RWMutex locks, supporting concurrent WME assertions and multi-partition execution.
package rete
