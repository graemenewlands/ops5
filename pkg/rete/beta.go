package rete

import (
	"fmt"
	"sync"

	"ops5/pkg/model"
)

// LeftActivatable represents a beta node that accepts left activations (tokens).
type LeftActivatable interface {
	LeftActivation(token *Token, tag PropagationTag)
}

// ConflictSetListener is notified when full rule instantiations are added or removed.
type ConflictSetListener interface {
	OnActivationAdd(rule *model.Rule, token *Token)
	OnActivationRemove(rule *model.Rule, token *Token)
}

// BetaMemory stores beta tokens and propagates them to child beta nodes.
type BetaMemory struct {
	mu         sync.RWMutex
	tokens     map[string]*Token // Keyed by token signature
	successors []LeftActivatable
}

// NewBetaMemory creates a new BetaMemory.
func NewBetaMemory() *BetaMemory {
	return &BetaMemory{
		tokens:     make(map[string]*Token),
		successors: make([]LeftActivatable, 0),
	}
}

func tokenSignature(t *Token) string {
	tags := t.Timetags()
	return fmt.Sprintf("%v", tags)
}

// AddSuccessor registers a child beta node.
func (bm *BetaMemory) AddSuccessor(node LeftActivatable) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.successors = append(bm.successors, node)

	// Catch-up: send existing tokens to new successor
	for _, tok := range bm.tokens {
		node.LeftActivation(tok, TagAdd)
	}
}

// Tokens returns a snapshot of stored tokens.
func (bm *BetaMemory) Tokens() []*Token {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	res := make([]*Token, 0, len(bm.tokens))
	for _, t := range bm.tokens {
		res = append(res, t)
	}
	return res
}

// LeftActivation processes an incoming token.
func (bm *BetaMemory) LeftActivation(token *Token, tag PropagationTag) {
	bm.mu.Lock()
	sig := tokenSignature(token)
	if tag == TagAdd {
		bm.tokens[sig] = token
	} else {
		delete(bm.tokens, sig)
	}
	succs := append([]LeftActivatable(nil), bm.successors...)
	bm.mu.Unlock()

	for _, s := range succs {
		s.LeftActivation(token, tag)
	}
}

// JoinTest specifies a relational test between a WME attribute and a variable bound in earlier tokens.
type JoinTest struct {
	Attribute   string
	Op          model.Operator
	Variable    string
	VectorIndex int // -1 for scalar/membership, >= 0 for positional element in vector
}

// JoinNode joins left tokens with right WMEs from an AlphaMemory.
type JoinNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	alphaMemory *AlphaMemory
	joinTests   []JoinTest
	ce          *model.ConditionElement
	successors  []LeftActivatable
}

// NewJoinNode creates a new two-input join node.
func NewJoinNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, tests []JoinTest) *JoinNode {
	return &JoinNode{
		betaMemory:  betaMem,
		alphaMemory: alphaMem,
		joinTests:   tests,
		ce:          ce,
		successors:  make([]LeftActivatable, 0),
	}
}

// Attach connects the join node to its parent memories and triggers catch-up.
func (jn *JoinNode) Attach() {
	if jn.alphaMemory != nil {
		jn.alphaMemory.AddSuccessor(jn)
	}
	if jn.betaMemory != nil {
		jn.betaMemory.AddSuccessor(jn)
	}
}

// AddSuccessor registers a downstream beta node.
func (jn *JoinNode) AddSuccessor(node LeftActivatable) {
	jn.mu.Lock()
	defer jn.mu.Unlock()
	jn.successors = append(jn.successors, node)
}

func evalJoinTest(jt JoinTest, boundVal model.Value, wmeVal model.Value) bool {
	if wmeVal.IsVector() {
		elems := wmeVal.VectorElements()
		if boundVal.IsVector() {
			return evalOp(wmeVal, jt.Op, boundVal)
		}
		if jt.VectorIndex >= 0 {
			if jt.VectorIndex < len(elems) {
				return evalOp(elems[jt.VectorIndex], jt.Op, boundVal)
			}
			return false
		}
		// Membership test (VectorIndex == -1):
		for _, elem := range elems {
			if evalOp(elem, jt.Op, boundVal) {
				return true
			}
		}
		return false
	}
	return evalOp(wmeVal, jt.Op, boundVal)
}

func matchesJoinTests(tests []JoinTest, token *Token, wme *model.WME) bool {
	for _, jt := range tests {
		boundVal, exists := token.Bindings[jt.Variable]
		if !exists {
			return false
		}
		wmeVal, ok := wme.Get(jt.Attribute)
		if !ok {
			return false
		}
		if !evalJoinTest(jt, boundVal, wmeVal) {
			return false
		}
	}
	return true
}

// matches evaluates join tests between token bindings and right WME.
func (jn *JoinNode) matches(token *Token, wme *model.WME) bool {
	return matchesJoinTests(jn.joinTests, token, wme)
}

func matchValueOrVector(targetVal, existing model.Value) bool {
	if targetVal.Equal(existing) {
		return true
	}
	if targetVal.IsVector() && !existing.IsVector() {
		for _, el := range targetVal.VectorElements() {
			if el.Equal(existing) {
				return true
			}
		}
		return false
	}
	if !targetVal.IsVector() && existing.IsVector() {
		for _, el := range existing.VectorElements() {
			if el.Equal(targetVal) {
				return true
			}
		}
		return false
	}
	return false
}

// extractBindings extracts new variable bindings introduced by this condition element on wme.
func (jn *JoinNode) extractBindings(token *Token, wme *model.WME) (map[string]model.Value, bool) {
	newBindings := make(map[string]model.Value)

	if jn.ce != nil {
		if jn.ce.ElementVariable != "" {
			newBindings[jn.ce.ElementVariable] = model.NewInt(wme.Timetag)
		}

		for _, at := range jn.ce.Tests {
			wmeVal, hasVal := wme.Get(at.Attribute)
			isMulti := len(at.Constraints) > 1
			for idx, c := range at.Constraints {
				if c.Value.IsVariable() {
					vName := c.Value.VariableName()
					if c.Op == model.OpEqual {
						var targetVal model.Value
						if hasVal {
							if wmeVal.IsVector() {
								elems := wmeVal.VectorElements()
								if isMulti {
									if idx < len(elems) {
										targetVal = elems[idx]
									} else {
										return nil, false
									}
								} else {
									targetVal = wmeVal
								}
							} else {
								targetVal = wmeVal
							}
						}

						// Check if already bound in parent token
						if existing, ok := token.Bindings[vName]; ok {
							if !hasVal || !matchValueOrVector(targetVal, existing) {
								return nil, false
							}
						} else if intraVal, ok := newBindings[vName]; ok {
							// Check intra-condition consistency
							if !hasVal || !matchValueOrVector(targetVal, intraVal) {
								return nil, false
							}
						} else {
							if hasVal {
								newBindings[vName] = targetVal
							}
						}
					}
				}
			}
		}
	}

	return newBindings, true
}

// LeftActivation handles an incoming token from the parent BetaMemory.
func (jn *JoinNode) LeftActivation(token *Token, tag PropagationTag) {
	if jn.alphaMemory == nil {
		return
	}

	wmes := jn.alphaMemory.Items()
	for _, wme := range wmes {
		if jn.matches(token, wme) {
			newBindings, ok := jn.extractBindings(token, wme)
			if ok {
				childToken := NewToken(token, wme, newBindings)
				jn.propagate(childToken, tag)
			}
		}
	}
}

// RightActivation handles an incoming WME from the AlphaMemory.
func (jn *JoinNode) RightActivation(wme *model.WME, tag PropagationTag) {
	if jn.betaMemory == nil {
		return
	}

	tokens := jn.betaMemory.Tokens()
	for _, token := range tokens {
		if jn.matches(token, wme) {
			newBindings, ok := jn.extractBindings(token, wme)
			if ok {
				childToken := NewToken(token, wme, newBindings)
				jn.propagate(childToken, tag)
			}
		}
	}
}

func (jn *JoinNode) propagate(token *Token, tag PropagationTag) {
	jn.mu.RLock()
	succs := append([]LeftActivatable(nil), jn.successors...)
	jn.mu.RUnlock()

	for _, s := range succs {
		s.LeftActivation(token, tag)
	}
}

// NegativeJoinNode handles negated condition elements -(class ...)
// A token passes through when NO matching right WMEs exist.
type NegativeJoinNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	alphaMemory *AlphaMemory
	joinTests   []JoinTest
	ce          *model.ConditionElement
	// matches stores matching WME timetags per token signature
	matches    map[string]map[int64]bool
	tokens     map[string]*Token
	successors []LeftActivatable
}

// NewNegativeJoinNode creates a new NegativeJoinNode.
func NewNegativeJoinNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, tests []JoinTest) *NegativeJoinNode {
	return &NegativeJoinNode{
		betaMemory:  betaMem,
		alphaMemory: alphaMem,
		joinTests:   tests,
		ce:          ce,
		matches:     make(map[string]map[int64]bool),
		tokens:      make(map[string]*Token),
		successors:  make([]LeftActivatable, 0),
	}
}

// Attach connects the negative join node to its parent memories and triggers catch-up.
func (njn *NegativeJoinNode) Attach() {
	if njn.alphaMemory != nil {
		njn.alphaMemory.AddSuccessor(njn)
	}
	if njn.betaMemory != nil {
		njn.betaMemory.AddSuccessor(njn)
	}
}

// AddSuccessor registers a downstream beta node.
func (njn *NegativeJoinNode) AddSuccessor(node LeftActivatable) {
	njn.mu.Lock()
	defer njn.mu.Unlock()
	njn.successors = append(njn.successors, node)
}

func (njn *NegativeJoinNode) match(token *Token, wme *model.WME) bool {
	return matchesJoinTests(njn.joinTests, token, wme)
}

// LeftActivation handles an incoming token.
func (njn *NegativeJoinNode) LeftActivation(token *Token, tag PropagationTag) {
	njn.mu.Lock()
	sig := tokenSignature(token)

	if tag == TagAdd {
		njn.tokens[sig] = token
		matchedWmes := make(map[int64]bool)

		if njn.alphaMemory != nil {
			for _, wme := range njn.alphaMemory.Items() {
				if njn.match(token, wme) {
					matchedWmes[wme.Timetag] = true
				}
			}
		}
		njn.matches[sig] = matchedWmes

		// If no matching right WMEs exist, the negative condition is satisfied!
		if len(matchedWmes) == 0 {
			succs := append([]LeftActivatable(nil), njn.successors...)
			njn.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagAdd)
			}
			return
		}
		njn.mu.Unlock()
	} else {
		matchedWmes := njn.matches[sig]
		delete(njn.tokens, sig)
		delete(njn.matches, sig)

		if len(matchedWmes) == 0 {
			succs := append([]LeftActivatable(nil), njn.successors...)
			njn.mu.Unlock()
			for _, s := range succs {
				s.LeftActivation(token, TagRemove)
			}
			return
		}
		njn.mu.Unlock()
	}
}

// RightActivation handles an incoming WME from AlphaMemory.
func (njn *NegativeJoinNode) RightActivation(wme *model.WME, tag PropagationTag) {
	njn.mu.Lock()
	defer njn.mu.Unlock()

	for sig, token := range njn.tokens {
		if !njn.match(token, wme) {
			continue
		}

		matchedWmes := njn.matches[sig]
		if matchedWmes == nil {
			matchedWmes = make(map[int64]bool)
			njn.matches[sig] = matchedWmes
		}

		if tag == TagAdd {
			prevCount := len(matchedWmes)
			matchedWmes[wme.Timetag] = true
			if prevCount == 0 {
				// Condition was satisfied, now blocked by this WME!
				succs := append([]LeftActivatable(nil), njn.successors...)
				for _, s := range succs {
					s.LeftActivation(token, TagRemove)
				}
			}
		} else {
			if matchedWmes[wme.Timetag] {
				delete(matchedWmes, wme.Timetag)
				if len(matchedWmes) == 0 {
					// Blocking WME was removed, negative condition is now satisfied!
					succs := append([]LeftActivatable(nil), njn.successors...)
					for _, s := range succs {
						s.LeftActivation(token, TagAdd)
					}
				}
			}
		}
	}
}

// TerminalNode represents the completion of a rule's LHS and manages activation events.
type TerminalNode struct {
	rule     *model.Rule
	listener ConflictSetListener
}

// NewTerminalNode creates a new TerminalNode for a rule.
func NewTerminalNode(rule *model.Rule, listener ConflictSetListener) *TerminalNode {
	return &TerminalNode{
		rule:     rule,
		listener: listener,
	}
}

// LeftActivation handles a fully matched token arriving at the terminal node.
func (tn *TerminalNode) LeftActivation(token *Token, tag PropagationTag) {
	if tn.listener == nil {
		return
	}
	if tag == TagAdd {
		tn.listener.OnActivationAdd(tn.rule, token)
	} else {
		tn.listener.OnActivationRemove(tn.rule, token)
	}
}
