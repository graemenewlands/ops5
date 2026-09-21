package rete

import (
	"sync"

	"ops5/pkg/model"
)

// ExistentialJoinNode handles existential condition elements (exists (class ...)).
// A token passes through when AT LEAST ONE matching right WME exists.
// Unlike standard JoinNode, it does NOT duplicate tokens for multiple matching WMEs.
type ExistentialJoinNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	alphaMemory *AlphaMemory
	joinTests   []JoinTest
	alphaIndex  *AlphaIndex
	tokensIndex *BetaIndex
	ce          *model.ConditionElement
	// matches stores matching WME timetags per token signature
	matches    map[string]map[int64]bool
	tokens     map[string]*Token
	successors []LeftActivatable
}

// NewExistentialJoinNode creates a new ExistentialJoinNode.
func NewExistentialJoinNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, tests []JoinTest) *ExistentialJoinNode {
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

	return &ExistentialJoinNode{
		betaMemory:  betaMem,
		alphaMemory: alphaMem,
		joinTests:   tests,
		alphaIndex:  ai,
		tokensIndex: NewBetaIndex(leftVars),
		ce:          ce,
		matches:     make(map[string]map[int64]bool),
		tokens:      make(map[string]*Token),
		successors:  make([]LeftActivatable, 0),
	}
}

// Attach connects the existential join node to its parent memories and triggers catch-up.
func (ejn *ExistentialJoinNode) Attach() {
	if ejn.alphaMemory != nil {
		ejn.alphaMemory.AddSuccessor(ejn)
	}
	if ejn.betaMemory != nil {
		ejn.betaMemory.AddSuccessor(ejn)
	}
}

// AddSuccessor registers a downstream beta node.
func (ejn *ExistentialJoinNode) AddSuccessor(node LeftActivatable) {
	ejn.mu.Lock()
	defer ejn.mu.Unlock()
	ejn.successors = append(ejn.successors, node)
}

// RemoveSuccessor unregisters a downstream beta node.
func (ejn *ExistentialJoinNode) RemoveSuccessor(node LeftActivatable) {
	ejn.mu.Lock()
	defer ejn.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range ejn.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	ejn.successors = newSuccs
}

func (ejn *ExistentialJoinNode) match(token *Token, wme *model.WME) bool {
	return matchesJoinTests(ejn.joinTests, token, wme)
}

// LeftActivation handles an incoming token from the parent BetaMemory.
func (ejn *ExistentialJoinNode) LeftActivation(token *Token, tag PropagationTag) {
	ejn.mu.Lock()
	sig := tokenSignature(token)

	if tag == TagAdd {
		ejn.tokens[sig] = token
		ejn.tokensIndex.Add(token)
		matchedWmes := make(map[int64]bool)

		if ejn.alphaIndex != nil {
			key := ejn.tokensIndex.KeyForToken(token)
			candidates := ejn.alphaIndex.Lookup(key)
			for _, wme := range candidates {
				if ejn.match(token, wme) {
					matchedWmes[wme.Timetag] = true
				}
			}
		}
		ejn.matches[sig] = matchedWmes

		// If at least one matching right WME exists, condition is satisfied!
		if len(matchedWmes) > 0 {
			succs := append([]LeftActivatable(nil), ejn.successors...)
			ejn.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagAdd)
			}
			return
		}
		ejn.mu.Unlock()
	} else {
		matchedWmes := ejn.matches[sig]
		delete(ejn.tokens, sig)
		ejn.tokensIndex.Remove(token)
		delete(ejn.matches, sig)

		// If it was satisfied, propagate TagRemove downstream
		if len(matchedWmes) > 0 {
			succs := append([]LeftActivatable(nil), ejn.successors...)
			ejn.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagRemove)
			}
			return
		}
		ejn.mu.Unlock()
	}
}

// RightActivation handles an incoming WME from AlphaMemory.
func (ejn *ExistentialJoinNode) RightActivation(wme *model.WME, tag PropagationTag) {
	ejn.mu.Lock()
	defer ejn.mu.Unlock()

	var candidateTokens []*Token
	if ejn.alphaIndex != nil {
		keys := ejn.alphaIndex.KeysForWME(wme)
		seen := make(map[string]bool)
		for _, key := range keys {
			for _, tok := range ejn.tokensIndex.Lookup(key) {
				sig := tokenSignature(tok)
				if !seen[sig] {
					seen[sig] = true
					candidateTokens = append(candidateTokens, tok)
				}
			}
		}
	} else {
		for _, tok := range ejn.tokens {
			candidateTokens = append(candidateTokens, tok)
		}
	}

	for _, token := range candidateTokens {
		if !ejn.match(token, wme) {
			continue
		}
		sig := tokenSignature(token)

		matchedWmes := ejn.matches[sig]
		if matchedWmes == nil {
			matchedWmes = make(map[int64]bool)
			ejn.matches[sig] = matchedWmes
		}

		if tag == TagAdd {
			prevCount := len(matchedWmes)
			matchedWmes[wme.Timetag] = true
			if prevCount == 0 {
				// Transition 0 -> 1: Condition was not satisfied, now satisfied!
				succs := append([]LeftActivatable(nil), ejn.successors...)
				for _, s := range succs {
					s.LeftActivation(token, TagAdd)
				}
			}
			// If prevCount > 0, it was already satisfied; do not duplicate token!
		} else {
			if matchedWmes[wme.Timetag] {
				delete(matchedWmes, wme.Timetag)
				if len(matchedWmes) == 0 {
					// Last matching WME removed, condition is no longer satisfied!
					succs := append([]LeftActivatable(nil), ejn.successors...)
					for _, s := range succs {
						s.LeftActivation(token, TagRemove)
					}
				}
			}
		}
	}
}
