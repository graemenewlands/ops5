// Package conflict implements the OPS5 Conflict Set (Agenda), activation lifecycle, and resolution strategies.
//
// When the Rete pattern matching network identifies that all condition elements of a production rule are
// satisfied by working memory elements, it generates an Activation and registers it with the Conflict Set.
// During each execution cycle, the engine resolves conflicts among pending activations by selecting the
// single dominant activation according to the active conflict resolution strategy (LEX or MEA).
//
// # Conflict Resolution Strategies
//
// 1. LEX (Lexicographic Strategy):
//   - Tier 1 (Salience): Higher explicit rule salience strictly dominates.
//   - Tier 2 (Recency): Dominant recency comparison. Compares the highest timetag of any WME in the instantiation.
//   - Tier 3 (Remaining Recency): Compares the remaining WME timetags in descending order vector-wise.
//   - Tier 4 (Specificity): Rules with more tests and condition elements are favored over more general rules.
//   - Tier 5 (Arbitrary Tie-Breaker): Non-deterministic or declaration-order tie resolution.
//
// 2. MEA (Means-Ends Analysis Strategy):
//   - Designed for goal-directed architectures where the first condition element represents an active goal.
//   - Tier 1 (Salience): Higher explicit rule salience strictly dominates.
//   - Tier 2 (First Condition Recency): Compares the timetag of the WME matching the FIRST condition element of the rule.
//   - Tier 3 (Remaining Recency): Compares all other WME timetags in descending order.
//   - Tier 4 (Specificity): Complexity score based on number of condition elements and tests.
//   - Tier 5 (Arbitrary Tie-Breaker): Non-deterministic or declaration-order tie resolution.
//
// # Refraction Semantics
//
// In OPS5 semantics, an instantiated rule fires at most once on a specific set of WME timetags.
// Once an activation fires, its unique key (RuleName + sorted WME timetags) is added to a refraction table.
// Even if the same WMEs remain in memory, the activation will never re-fire unless at least one fact is
// retracted and re-asserted (yielding a new timetag).
//
// # High-Performance Binary Heap Agenda
//
// The conflict set maintains active instantiations in an indexed binary max-heap:
//   - Dominant Selection: O(1) peek at the heap root.
//   - Activation Insertion & Retraction: O(log K) priority queue operations with indexed lookups.
//   - Zero-Allocation Comparators: Activations pre-sort their timetags once upon construction, allowing
//     LexCompare and MeaCompare to execute with zero heap allocations during heap rebalancing.
//
// # Thread Safety
//
// All methods on Set are protected by an internal sync.RWMutex, allowing safe concurrent inspection,
// activation registration from Rete worker threads, and dominant selection.
package conflict
