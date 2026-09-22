package rete

import (
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
)

// NccPartnerNode is the terminal node of an NCC sub-network.
// It receives completed sub-tokens for the negated conjunction and notifies its partner NccNode.
type NccPartnerNode struct {
	mu                 sync.RWMutex
	nccNode            *NccNode
	subConditionsCount int

	// completions stores matching sub-tokens grouped by parent token signature:
	// completions[sigParentToken][sigSubToken] = subToken
	completions map[string]map[string]*Token
}

// NewNccPartnerNode creates a new NccPartnerNode.
func NewNccPartnerNode(subConditionsCount int) *NccPartnerNode {
	return &NccPartnerNode{
		subConditionsCount: subConditionsCount,
		completions:        make(map[string]map[string]*Token),
	}
}

// SetNccNode associates the partner node with its parent NccNode.
func (npn *NccPartnerNode) SetNccNode(node *NccNode) {
	npn.mu.Lock()
	defer npn.mu.Unlock()
	npn.nccNode = node
}

// getParentToken ascends the sub-token's parent chain by the number of sub-conditions
// to retrieve the original parent token from the main beta pipeline.
func (npn *NccPartnerNode) getParentToken(subToken *Token) *Token {
	curr := subToken
	for i := 0; i < npn.subConditionsCount; i++ {
		if curr == nil {
			return nil
		}
		curr = curr.Parent
	}
	return curr
}

// CompletionCount returns the number of completions for a given parent token signature.
func (npn *NccPartnerNode) CompletionCount(sigParent string) int {
	npn.mu.RLock()
	defer npn.mu.RUnlock()
	return len(npn.completions[sigParent])
}

// ClearCompletions removes all sub-token completions for a given parent token signature.
func (npn *NccPartnerNode) ClearCompletions(sigParent string) {
	npn.mu.Lock()
	defer npn.mu.Unlock()
	delete(npn.completions, sigParent)
}

// LeftActivation receives a sub-token completing the NCC sub-network.
func (npn *NccPartnerNode) LeftActivation(subToken *Token, tag PropagationTag) {
	npn.mu.Lock()
	parentToken := npn.getParentToken(subToken)
	if parentToken == nil {
		npn.mu.Unlock()
		return
	}

	sigParent := tokenSignature(parentToken)
	sigSub := tokenSignature(subToken)

	comps := npn.completions[sigParent]
	if comps == nil {
		comps = make(map[string]*Token)
		npn.completions[sigParent] = comps
	}

	var targetNode *NccNode
	var oldCount, newCount int
	notify := false

	if tag == TagAdd {
		if _, exists := comps[sigSub]; exists {
			npn.mu.Unlock()
			return
		}
		oldCount = len(comps)
		comps[sigSub] = subToken
		newCount = len(comps)
		targetNode = npn.nccNode
		notify = true
		npn.mu.Unlock()
	} else {
		if _, exists := comps[sigSub]; !exists {
			npn.mu.Unlock()
			return
		}
		oldCount = len(comps)
		delete(comps, sigSub)
		newCount = len(comps)
		targetNode = npn.nccNode
		notify = true
		npn.mu.Unlock()
	}

	if notify && targetNode != nil {
		targetNode.OnSubmatchCountChange(parentToken, oldCount, newCount)
	}
}

// NccNode gates the primary token stream on the absence of a conjunction of conditions.
// A token is satisfied and propagated downstream when its match count is 0.
type NccNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	partner     *NccPartnerNode
	ce          *model.ConditionElement
	tokens      map[string]*Token
	matchCounts map[string]int
	successors  []LeftActivatable
}

// NewNccNode creates a new NccNode.
func NewNccNode(betaMem *BetaMemory, partner *NccPartnerNode, ce *model.ConditionElement) *NccNode {
	node := &NccNode{
		betaMemory:  betaMem,
		partner:     partner,
		ce:          ce,
		tokens:      make(map[string]*Token),
		matchCounts: make(map[string]int),
		successors:  make([]LeftActivatable, 0),
	}
	partner.SetNccNode(node)
	return node
}

// AddSuccessor registers a downstream beta node.
func (node *NccNode) AddSuccessor(succ LeftActivatable) {
	node.mu.Lock()
	defer node.mu.Unlock()
	node.successors = append(node.successors, succ)

	// Catch-up: send currently satisfied tokens (match count == 0) to new successor
	for sig, tok := range node.tokens {
		if node.matchCounts[sig] == 0 {
			succ.LeftActivation(tok, TagAdd)
		}
	}
}

// RemoveSuccessor unregisters a downstream beta node.
func (node *NccNode) RemoveSuccessor(succ LeftActivatable) {
	node.mu.Lock()
	defer node.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range node.successors {
		if s != succ {
			newSuccs = append(newSuccs, s)
		}
	}
	node.successors = newSuccs
}

// LeftActivation handles an incoming parent token from BetaMemory.
func (node *NccNode) LeftActivation(token *Token, tag PropagationTag) {
	node.mu.Lock()
	sig := tokenSignature(token)

	if tag == TagAdd {
		node.tokens[sig] = token
		count := node.partner.CompletionCount(sig)
		node.matchCounts[sig] = count

		// If no completions exist in subnetwork, negated conjunction is satisfied!
		if count == 0 {
			succs := append([]LeftActivatable(nil), node.successors...)
			node.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagAdd)
			}
			return
		}
		node.mu.Unlock()
	} else {
		delete(node.tokens, sig)
		count := node.matchCounts[sig]
		delete(node.matchCounts, sig)
		node.partner.ClearCompletions(sig)

		if count == 0 {
			// Was previously satisfied and propagated; emit retraction
			succs := append([]LeftActivatable(nil), node.successors...)
			node.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagRemove)
			}
			return
		}
		node.mu.Unlock()
	}
}

// OnSubmatchCountChange is invoked by NccPartnerNode when the completion count changes for a parent token.
func (node *NccNode) OnSubmatchCountChange(parentToken *Token, oldCount, newCount int) {
	node.mu.Lock()
	sig := tokenSignature(parentToken)

	tok, exists := node.tokens[sig]
	if !exists {
		// Parent token has not yet arrived at NccNode or was already retracted
		node.matchCounts[sig] = newCount
		node.mu.Unlock()
		return
	}

	node.matchCounts[sig] = newCount

	if oldCount == 0 && newCount > 0 {
		// Was satisfied (0), now blocked (>= 1): emit TagRemove downstream!
		succs := append([]LeftActivatable(nil), node.successors...)
		node.mu.Unlock()
		for _, s := range succs {
			s.LeftActivation(tok, TagRemove)
		}
		return
	}

	if oldCount > 0 && newCount == 0 {
		// Was blocked (>= 1), now satisfied (0): emit TagAdd downstream!
		succs := append([]LeftActivatable(nil), node.successors...)
		node.mu.Unlock()
		for _, s := range succs {
			s.LeftActivation(tok, TagAdd)
		}
		return
	}

	node.mu.Unlock()
}
