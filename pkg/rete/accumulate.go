package rete

import (
	"sort"
	"strings"
	"sync"

	"ops5/pkg/model"
)

// AccumulateNode implements beta-network aggregation over sets of matching WMEs.
// Supported operations: :count, :sum, :average (:avg), :min, :max, :collect.
type AccumulateNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	alphaMemory *AlphaMemory
	joinTests   []JoinTest
	alphaIndex  *AlphaIndex
	tokensIndex *BetaIndex
	ce          *model.ConditionElement
	spec        *model.AccumulateSpec

	// parentTokens stores incoming left tokens keyed by tokenSignature(token)
	parentTokens map[string]*Token

	// matchedWmes stores matched right WMEs per parent token signature
	matchedWmes map[string]map[int64]*model.WME

	// activeTokens stores the currently emitted downstream token per parent token signature
	activeTokens map[string]*Token

	successors []LeftActivatable
}

// NewAccumulateNode creates a new AccumulateNode.
func NewAccumulateNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, spec *model.AccumulateSpec, tests []JoinTest) *AccumulateNode {
	var leftVars []string
	var rightSpecs []AlphaIndexSpec

	for _, t := range tests {
		if t.Op == model.OpEqual {
			leftVars = append(leftVars, t.Variable)
			rightSpecs = append(rightSpecs, AlphaIndexSpec{
				Attribute:   t.Attribute,
				VectorIndex: t.VectorIndex,
			})
		}
	}

	var ai *AlphaIndex
	if alphaMem != nil {
		ai = alphaMem.GetOrCreateIndex(rightSpecs)
	}

	return &AccumulateNode{
		betaMemory:   betaMem,
		alphaMemory:  alphaMem,
		joinTests:    tests,
		alphaIndex:   ai,
		tokensIndex:  NewBetaIndex(leftVars),
		ce:           ce,
		spec:         spec,
		parentTokens: make(map[string]*Token),
		matchedWmes:  make(map[string]map[int64]*model.WME),
		activeTokens: make(map[string]*Token),
		successors:   make([]LeftActivatable, 0),
	}
}

// Attach connects the accumulate node to its parent alpha and beta memories.
func (an *AccumulateNode) Attach() {
	if an.alphaMemory != nil {
		an.alphaMemory.AddSuccessor(an)
	}
	if an.betaMemory != nil {
		an.betaMemory.AddSuccessor(an)
	}
}

// AddSuccessor registers a downstream beta node.
func (an *AccumulateNode) AddSuccessor(node LeftActivatable) {
	an.mu.Lock()
	defer an.mu.Unlock()
	an.successors = append(an.successors, node)

	// Catch-up: send existing active aggregate tokens to new successor
	for _, tok := range an.activeTokens {
		node.LeftActivation(tok, TagAdd)
	}
}

// RemoveSuccessor unregisters a downstream beta node.
func (an *AccumulateNode) RemoveSuccessor(node LeftActivatable) {
	an.mu.Lock()
	defer an.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range an.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	an.successors = newSuccs
}

func (an *AccumulateNode) match(token *Token, wme *model.WME) bool {
	return matchesJoinTests(an.joinTests, token, wme)
}

func int64SliceEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// extractTargetValue extracts the evaluated target operand from a matching WME.
func (an *AccumulateNode) extractTargetValue(parentToken *Token, wme *model.WME) model.Value {
	if an.spec == nil {
		return model.NewInt(0)
	}

	target := an.spec.Target

	// Build local bindings combining parent token bindings and matching WME attribute variables
	bindings := make(map[string]model.Value)
	if parentToken != nil {
		for k, v := range parentToken.Bindings {
			bindings[k] = v
		}
	}

	if an.ce != nil {
		for _, at := range an.ce.Tests {
			val, ok := wme.Get(at.Attribute)
			if !ok {
				val = model.NewSymbol("nil")
			}
			for _, c := range at.Constraints {
				if c.Value.IsVariable() {
					bindings[c.Value.VariableName()] = val
				}
			}
		}
	}

	if target.IsVariable() {
		vName := target.VariableName()
		if v, ok := bindings[vName]; ok {
			return v
		}
		if v, ok := wme.Get(vName); ok {
			return v
		}
		return model.NewInt(0)
	}

	if target.IsCompute() {
		res, err := model.EvaluateCompute(target.ComputeExpr(), bindings)
		if err == nil {
			return res
		}
		return model.NewInt(0)
	}

	if target.Type() == model.TypeSymbol {
		sym := target.String()
		attrName := model.NormalizeAttribute(sym)
		if v, ok := wme.Get(attrName); ok {
			return v
		}
		if v, ok := bindings[sym]; ok {
			return v
		}
	}

	return target
}

// evaluateAggregate calculates the aggregate value and sorted timetags for a set of matched WMEs.
func (an *AccumulateNode) evaluateAggregate(parentToken *Token, wmes map[int64]*model.WME) (model.Value, []int64, bool) {
	timetags := make([]int64, 0, len(wmes))
	for tt := range wmes {
		timetags = append(timetags, tt)
	}
	sort.Slice(timetags, func(i, j int) bool { return timetags[i] < timetags[j] })

	count := len(wmes)

	switch an.spec.Op {
	case model.AccCount:
		return model.NewInt(int64(count)), timetags, true

	case model.AccSum:
		if count == 0 {
			return model.NewInt(0), timetags, true
		}
		isFloat := false
		var totalInt int64
		var totalFloat float64
		for _, tt := range timetags {
			val := an.extractTargetValue(parentToken, wmes[tt])
			if val.Type() == model.TypeFloat {
				isFloat = true
				totalFloat += val.Raw().(float64)
			} else if val.Type() == model.TypeInteger {
				iv := val.Raw().(int64)
				totalInt += iv
				totalFloat += float64(iv)
			}
		}
		if isFloat {
			return model.NewFloat(totalFloat), timetags, true
		}
		return model.NewInt(totalInt), timetags, true

	case model.AccAverage:
		if count == 0 {
			return model.Value{}, nil, false
		}
		var totalFloat float64
		for _, tt := range timetags {
			val := an.extractTargetValue(parentToken, wmes[tt])
			if val.Type() == model.TypeFloat {
				totalFloat += val.Raw().(float64)
			} else if val.Type() == model.TypeInteger {
				totalFloat += float64(val.Raw().(int64))
			}
		}
		avg := totalFloat / float64(count)
		return model.NewFloat(avg), timetags, true

	case model.AccMin:
		if count == 0 {
			return model.Value{}, nil, false
		}
		var minVal model.Value
		first := true
		for _, tt := range timetags {
			val := an.extractTargetValue(parentToken, wmes[tt])
			if first {
				minVal = val
				first = false
			} else {
				cmp, err := val.Compare(minVal)
				if err == nil && cmp < 0 {
					minVal = val
				}
			}
		}
		return minVal, timetags, true

	case model.AccMax:
		if count == 0 {
			return model.Value{}, nil, false
		}
		var maxVal model.Value
		first := true
		for _, tt := range timetags {
			val := an.extractTargetValue(parentToken, wmes[tt])
			if first {
				maxVal = val
				first = false
			} else {
				cmp, err := val.Compare(maxVal)
				if err == nil && cmp > 0 {
					maxVal = val
				}
			}
		}
		return maxVal, timetags, true

	case model.AccCollect:
		elements := make([]model.Value, 0, count)
		for _, tt := range timetags {
			elements = append(elements, an.extractTargetValue(parentToken, wmes[tt]))
		}
		return model.NewVector(elements), timetags, true

	default:
		return model.NewInt(0), timetags, true
	}
}

// LeftActivation handles an incoming parent token from BetaMemory.
func (an *AccumulateNode) LeftActivation(token *Token, tag PropagationTag) {
	an.mu.Lock()
	sig := tokenSignature(token)

	if tag == TagAdd {
		an.parentTokens[sig] = token
		an.tokensIndex.Add(token)
		matched := make(map[int64]*model.WME)

		var candidates []*model.WME
		if an.alphaIndex != nil {
			key := an.tokensIndex.KeyForToken(token)
			candidates = an.alphaIndex.Lookup(key)
		} else if an.alphaMemory != nil {
			candidates = an.alphaMemory.Items()
		}

		for _, wme := range candidates {
			if an.match(token, wme) {
				matched[wme.Timetag] = wme
			}
		}
		an.matchedWmes[sig] = matched

		val, timetags, ok := an.evaluateAggregate(token, matched)
		if ok {
			resVar := strings.TrimPrefix(strings.TrimSuffix(an.spec.ResultVar, ">"), "<")
			childToken := NewAccumulateToken(token, map[string]model.Value{resVar: val}, timetags)
			an.activeTokens[sig] = childToken
			succs := append([]LeftActivatable(nil), an.successors...)
			an.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(childToken, TagAdd)
			}
			return
		}
		an.mu.Unlock()
	} else {
		childToken := an.activeTokens[sig]
		delete(an.parentTokens, sig)
		an.tokensIndex.Remove(token)
		delete(an.matchedWmes, sig)
		delete(an.activeTokens, sig)

		if childToken != nil {
			succs := append([]LeftActivatable(nil), an.successors...)
			an.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(childToken, TagRemove)
			}
			return
		}
		an.mu.Unlock()
	}
}

// RightActivation handles an incoming WME from AlphaMemory.
func (an *AccumulateNode) RightActivation(wme *model.WME, tag PropagationTag) {
	an.mu.Lock()
	defer an.mu.Unlock()

	var candidateTokens []*Token
	if an.alphaIndex != nil {
		keys := an.alphaIndex.KeysForWME(wme)
		seen := make(map[string]bool)
		for _, key := range keys {
			for _, tok := range an.tokensIndex.Lookup(key) {
				sig := tokenSignature(tok)
				if !seen[sig] {
					seen[sig] = true
					candidateTokens = append(candidateTokens, tok)
				}
			}
		}
	} else {
		for _, tok := range an.parentTokens {
			candidateTokens = append(candidateTokens, tok)
		}
	}

	resVar := strings.TrimPrefix(strings.TrimSuffix(an.spec.ResultVar, ">"), "<")

	for _, token := range candidateTokens {
		if !an.match(token, wme) {
			continue
		}
		sig := tokenSignature(token)
		matched := an.matchedWmes[sig]
		if matched == nil {
			matched = make(map[int64]*model.WME)
			an.matchedWmes[sig] = matched
		}

		if tag == TagAdd {
			if _, exists := matched[wme.Timetag]; exists {
				continue
			}
			matched[wme.Timetag] = wme
		} else {
			if _, exists := matched[wme.Timetag]; !exists {
				continue
			}
			delete(matched, wme.Timetag)
		}

		oldChild := an.activeTokens[sig]
		val, timetags, ok := an.evaluateAggregate(token, matched)

		// If nothing changed in aggregate value or underlying WME timetags, skip churn
		if oldChild != nil && ok {
			oldVal := oldChild.Bindings[resVar]
			if oldVal.Equal(val) && int64SliceEqual(oldChild.ExtraTimetags, timetags) {
				continue
			}
		}

		// If an active token previously existed, retract it
		if oldChild != nil {
			succs := append([]LeftActivatable(nil), an.successors...)
			for _, s := range succs {
				s.LeftActivation(oldChild, TagRemove)
			}
			delete(an.activeTokens, sig)
		}

		// If the new aggregate is valid, assert it
		if ok {
			newChild := NewAccumulateToken(token, map[string]model.Value{resVar: val}, timetags)
			an.activeTokens[sig] = newChild
			succs := append([]LeftActivatable(nil), an.successors...)
			for _, s := range succs {
				s.LeftActivation(newChild, TagAdd)
			}
		}
	}
}
