package rete

import (
	"github.com/graemenewlands/ops5/pkg/model"
)

// isPositiveCE returns true if the condition element is a standard positive condition
// that produces a WME token in the beta network.
func isPositiveCE(ce *model.ConditionElement) bool {
	return ce != nil && !ce.IsNegative && !ce.IsTest && !ce.IsNCC && !ce.IsAccumulate && !ce.IsExistential
}

// collectPositiveVarDefs collects all variable names that are ever bound by positive condition elements in the rule.
func collectPositiveVarDefs(rule *model.Rule) map[string]bool {
	defs := make(map[string]bool)
	for _, ce := range rule.Conditions {
		if ce.ElementVariable != "" {
			defs[ce.ElementVariable] = true
		}
		if isPositiveCE(ce) {
			for _, at := range ce.Tests {
				for _, c := range at.Constraints {
					if c.Op == model.OpEqual && c.Value.IsVariable() {
						defs[c.Value.VariableName()] = true
					}
				}
			}
		} else if ce.IsAccumulate && ce.Accumulate != nil && ce.Accumulate.ResultVar != "" {
			defs[ce.Accumulate.ResultVar] = true
		}
	}
	return defs
}

// collectVariableCounts counts occurrences of each variable across all condition elements.
func collectVariableCounts(rule *model.Rule) map[string]int {
	counts := make(map[string]int)
	for _, ce := range rule.Conditions {
		for _, v := range ce.Variables() {
			counts[v]++
		}
	}
	return counts
}

type conditionMeta struct {
	ce           *model.ConditionElement
	origIndex    int      // 0-based index in original rule.Conditions
	origPosIndex int      // 1-based index among positive conditions (0 if not positive)
	bindsVars    []string // Variables bound by this condition
	requiresVars []string // Variables that MUST be already bound before this CE can be placed
	allVars      []string // All variables mentioned in this condition
	joinVars     []string // Variables in this condition that appear in multiple conditions (graph edges)
	hasJoinVars  bool     // True if condition participates in any variable join in the rule
	selectivity  int      // Selectivity bonus
}

func computeConditionSelectivity(ce *model.ConditionElement) int {
	if ce == nil {
		return 0
	}
	score := 0
	if ce.Class != "*" && ce.Class != "" {
		score += 10
	}
	for _, at := range ce.Tests {
		for _, c := range at.Constraints {
			if len(c.Disjunction) > 0 {
				score += 25
			} else if !c.Value.IsVariable() && !c.Value.IsCompute() {
				if c.Op == model.OpEqual {
					score += 50 // Constant equality test is highly selective
				} else {
					score += 25 // Constant inequality / range test
				}
			}
		}
	}
	return score
}

func analyzeCondition(ce *model.ConditionElement, origIndex, origPosIndex int, positiveVarDefs map[string]bool, varCounts map[string]int) *conditionMeta {
	meta := &conditionMeta{
		ce:           ce,
		origIndex:    origIndex,
		origPosIndex: origPosIndex,
		selectivity:  computeConditionSelectivity(ce),
	}

	allVarsMap := make(map[string]bool)
	for _, v := range ce.Variables() {
		allVarsMap[v] = true
		if varCounts[v] > 1 {
			meta.joinVars = append(meta.joinVars, v)
			meta.hasJoinVars = true
		}
	}
	for v := range allVarsMap {
		meta.allVars = append(meta.allVars, v)
	}

	localBinds := make(map[string]bool)
	localReqs := make(map[string]bool)

	if ce.IsTest {
		// (test ...) cannot bind variables; all referenced variables must be pre-bound.
		for _, v := range meta.allVars {
			localReqs[v] = true
		}
	} else if ce.IsNegative || ce.IsNCC {
		// Negated conditions / NCC cannot bind variables.
		// Any variable that is bound by a positive CE in the rule is a prerequisite.
		for _, v := range meta.allVars {
			if positiveVarDefs[v] {
				localReqs[v] = true
			}
		}
	} else if ce.IsExistential {
		// Existential condition cannot bind variables.
		for _, v := range meta.allVars {
			if positiveVarDefs[v] {
				localReqs[v] = true
			}
		}
	} else if ce.IsAccumulate {
		if ce.Accumulate != nil && ce.Accumulate.ResultVar != "" {
			localBinds[ce.Accumulate.ResultVar] = true
		}
		for _, v := range meta.allVars {
			if v != ce.Accumulate.ResultVar && positiveVarDefs[v] {
				localReqs[v] = true
			}
		}
	} else {
		// Positive condition element
		if ce.ElementVariable != "" {
			localBinds[ce.ElementVariable] = true
		}
		for _, at := range ce.Tests {
			for _, c := range at.Constraints {
				if len(c.Disjunction) > 0 {
					for _, dj := range c.Disjunction {
						if dj.Value.IsVariable() {
							localReqs[dj.Value.VariableName()] = true
						} else if dj.Value.IsCompute() {
							collectComputeVariables(dj.Value, localReqs)
						}
					}
				} else if c.Value.IsVariable() {
					if c.Op == model.OpEqual {
						localBinds[c.Value.VariableName()] = true
					} else {
						// Non-equality constraint requires variable to be bound previously
						localReqs[c.Value.VariableName()] = true
					}
				} else if c.Value.IsCompute() {
					collectComputeVariables(c.Value, localReqs)
				}
			}
		}

		// A variable bound by an equality test within this condition element satisfies its own requirement
		for b := range localBinds {
			delete(localReqs, b)
		}
	}

	for b := range localBinds {
		meta.bindsVars = append(meta.bindsVars, b)
	}
	for r := range localReqs {
		meta.requiresVars = append(meta.requiresVars, r)
	}

	return meta
}

func collectComputeVariables(v model.Value, varMap map[string]bool) {
	if !v.IsCompute() {
		return
	}
	comp := v.ComputeExpr()
	for _, op := range comp.Operands {
		if op.IsVariable() {
			varMap[op.VariableName()] = true
		} else if op.IsCompute() {
			collectComputeVariables(op, varMap)
		}
	}
}

// OptimizeRuleJoinOrder computes an optimal join ordering for the rule LHS.
// It preserves Condition 1 (C_1) at index 0 to guarantee MEA conflict resolution semantics,
// then greedily orders remaining condition elements to maximize variable connectivity
// (eliminating Cartesian cross-products), prune early with filters/negations,
// and prioritize selective alpha tests.
//
// If the ordering is unchanged, the original rule is returned without allocations.
// If the ordering changes, a cloned rule is returned with remapped action indices.
func OptimizeRuleJoinOrder(rule *model.Rule) *model.Rule {
	if rule == nil || len(rule.Conditions) <= 2 || rule.NoReorder {
		return rule
	}

	positiveVarDefs := collectPositiveVarDefs(rule)
	varCounts := collectVariableCounts(rule)

	// Build condition metadata
	var candidates []*conditionMeta
	posIdx := 1
	var firstMeta *conditionMeta

	for i, ce := range rule.Conditions {
		curPosIdx := 0
		if isPositiveCE(ce) {
			curPosIdx = posIdx
			posIdx++
		}
		meta := analyzeCondition(ce, i, curPosIdx, positiveVarDefs, varCounts)
		if i == 0 {
			firstMeta = meta
		} else {
			candidates = append(candidates, meta)
		}
	}

	// Condition 1 is anchored at index 0 (MEA Invariant)
	orderedConditions := make([]*model.ConditionElement, 0, len(rule.Conditions))
	orderedConditions = append(orderedConditions, firstMeta.ce)

	boundVars := make(map[string]bool)
	for _, v := range firstMeta.bindsVars {
		boundVars[v] = true
	}

	// Greedily choose best remaining candidate
	for len(candidates) > 0 {
		bestIdx := -1
		bestScore := -1

		// First pass: look for eligible candidates whose required variables are all bound
		for i, cand := range candidates {
			eligible := true
			for _, req := range cand.requiresVars {
				if !boundVars[req] {
					eligible = false
					break
				}
			}
			if !eligible {
				continue
			}

			// Compute heuristic score
			score := 0

			// 1. Variable connectivity bonus with currently bound variables (eliminates Cartesian cross-products)
			sharedCount := 0
			for _, v := range cand.allVars {
				if boundVars[v] {
					sharedCount++
				}
			}
			if sharedCount > 0 {
				score += 10000
				score += sharedCount * 200
			}

			// 2. Early pruning bonus: filters (test) and negations prune tokens early once ready
			if cand.ce.IsTest {
				score += 500
			} else if cand.ce.IsNegative || cand.ce.IsNCC {
				score += 400
			}

			// 3. Selectivity bonus from constant tests and future join potential
			if sharedCount > 0 {
				score += cand.selectivity
			} else if cand.hasJoinVars {
				// Has join variables that can connect downstream conditions
				score += len(cand.joinVars) * 50
				score += cand.selectivity
			} else {
				// Isolated condition with no join variables (degree 0 in variable graph).
				// Joining this condition is an unavoidable Cartesian product.
				// Do not give constant selectivity bonus that would disrupt authored condition order.
				score += 0
			}

			// Pick best candidate, breaking ties stably by smaller original index
			if bestIdx == -1 || score > bestScore || (score == bestScore && cand.origIndex < candidates[bestIdx].origIndex) {
				bestScore = score
				bestIdx = i
			}
		}

		// Fallback: If no candidate has all requirements satisfied (e.g. malformed rule with unbound vars),
		// pick the first candidate to guarantee progress.
		if bestIdx == -1 {
			bestIdx = 0
		}

		chosen := candidates[bestIdx]
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)

		orderedConditions = append(orderedConditions, chosen.ce)
		for _, b := range chosen.bindsVars {
			boundVars[b] = true
		}
	}

	// Check if condition order changed
	orderChanged := false
	for i := range rule.Conditions {
		if rule.Conditions[i] != orderedConditions[i] {
			orderChanged = true
			break
		}
	}
	if !orderChanged {
		return rule
	}

	// Condition order changed: build positive condition index remapping
	origPosMap := make(map[*model.ConditionElement]int)
	origPos := 1
	for _, ce := range rule.Conditions {
		if isPositiveCE(ce) {
			origPosMap[ce] = origPos
			origPos++
		}
	}

	posRemap := make(map[int]int)
	newPos := 1
	for _, ce := range orderedConditions {
		if isPositiveCE(ce) {
			if oldPos, ok := origPosMap[ce]; ok {
				posRemap[oldPos] = newPos
			}
			newPos++
		}
	}

	// Create cloned rule with reordered conditions and remapped actions
	cloned := rule.Clone()
	cloned.Conditions = orderedConditions
	cloned.Actions = remapActions(rule.Actions, posRemap)
	return cloned
}

// remapActions remaps positive condition element indices in ModifyAction, RemoveAction, and SubstrExpr.
func remapActions(actions []model.Action, posRemap map[int]int) []model.Action {
	if len(posRemap) == 0 {
		return actions
	}

	remapped := make([]model.Action, len(actions))
	for i, act := range actions {
		switch a := act.(type) {
		case model.ModifyAction:
			newAct := a
			if a.TargetElementVar == "" && a.TargetIndex > 0 {
				if newIdx, ok := posRemap[a.TargetIndex]; ok {
					newAct.TargetIndex = newIdx
				}
			}
			if len(a.Attributes) > 0 {
				newAttrs := make(map[string]model.Value, len(a.Attributes))
				for k, v := range a.Attributes {
					newAttrs[k] = remapSubstrInValue(v, posRemap)
				}
				newAct.Attributes = newAttrs
			}
			remapped[i] = newAct

		case model.RemoveAction:
			newAct := a
			if !a.Wildcard && a.TargetElementVar == "" && a.TargetIndex > 0 {
				if newIdx, ok := posRemap[a.TargetIndex]; ok {
					newAct.TargetIndex = newIdx
				}
			}
			remapped[i] = newAct

		case model.MakeAction:
			newAct := a
			if len(a.Attributes) > 0 {
				newAttrs := make(map[string]model.Value, len(a.Attributes))
				for k, v := range a.Attributes {
					newAttrs[k] = remapSubstrInValue(v, posRemap)
				}
				newAct.Attributes = newAttrs
			}
			remapped[i] = newAct

		case model.WriteAction:
			newAct := a
			if len(a.Args) > 0 {
				newArgs := make([]model.WriteArg, len(a.Args))
				for j, arg := range a.Args {
					if arg.Type == model.WriteArgValue {
						newArgs[j] = model.WriteArg{
							Type:  model.WriteArgValue,
							Value: remapSubstrInValue(arg.Value, posRemap),
						}
					} else if arg.Type == model.WriteArgTabTo {
						newArgs[j] = model.WriteArg{
							Type:  model.WriteArgTabTo,
							Value: remapSubstrInValue(arg.Value, posRemap),
						}
					} else {
						newArgs[j] = arg
					}
				}
				newAct.Args = newArgs
			}
			remapped[i] = newAct

		case model.BindAction:
			newAct := a
			newAct.Value = remapSubstrInValue(a.Value, posRemap)
			remapped[i] = newAct

		case model.OpenFileAction:
			newAct := a
			newAct.Filespec = remapSubstrInValue(a.Filespec, posRemap)
			remapped[i] = newAct

		default:
			remapped[i] = act
		}
	}
	return remapped
}

// remapSubstrInValue traverses values and updates any SubstrExpr whose ElementRef is an integer CE index.
func remapSubstrInValue(v model.Value, posRemap map[int]int) model.Value {
	if v.IsSubstr() {
		se := v.SubstrExpr()
		newElemRef := se.ElementRef
		if se.ElementRef.Type() == model.TypeInteger {
			oldIdx := int(se.ElementRef.Raw().(int64))
			if newIdx, ok := posRemap[oldIdx]; ok {
				newElemRef = model.NewInt(int64(newIdx))
			}
		}
		newStart := remapSubstrInValue(se.Start, posRemap)
		newEnd := remapSubstrInValue(se.End, posRemap)
		return model.NewSubstr(newElemRef, newStart, newEnd)
	}
	if v.IsVector() {
		elems := v.VectorElements()
		newElems := make([]model.Value, len(elems))
		for i, el := range elems {
			newElems[i] = remapSubstrInValue(el, posRemap)
		}
		return model.NewVector(newElems)
	}
	if v.IsCompute() {
		comp := v.ComputeExpr()
		newOps := make([]model.Value, len(comp.Operands))
		for i, op := range comp.Operands {
			newOps[i] = remapSubstrInValue(op, posRemap)
		}
		return model.NewCompute(newOps, comp.Operators)
	}
	return v
}
