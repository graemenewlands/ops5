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
	id         int
	tokens     map[string]*Token // Keyed by token signature
	successors []LeftActivatable
	indexes    []*BetaIndex
}

// NewBetaMemory creates a new BetaMemory.
func NewBetaMemory() *BetaMemory {
	return &BetaMemory{
		tokens:     make(map[string]*Token),
		successors: make([]LeftActivatable, 0),
		indexes:    make([]*BetaIndex, 0),
	}
}

// ID returns the unique ID of this BetaMemory.
func (bm *BetaMemory) ID() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.id
}

// TokenCount returns the number of tokens currently stored in this BetaMemory.
func (bm *BetaMemory) TokenCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.tokens)
}

func tokenSignature(t *Token) string {
	tags := t.Timetags()
	return fmt.Sprintf("%v", tags)
}

// GetOrCreateIndex returns an existing BetaIndex matching variables or creates and populates a new one.
func (bm *BetaMemory) GetOrCreateIndex(variables []string) *BetaIndex {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, idx := range bm.indexes {
		if strSliceEqual(idx.variables, variables) {
			return idx
		}
	}

	idx := NewBetaIndex(variables)
	for _, tok := range bm.tokens {
		idx.Add(tok)
	}
	bm.indexes = append(bm.indexes, idx)
	return idx
}

func strSliceEqual(a, b []string) bool {
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

// RemoveSuccessor unregisters a child beta node.
func (bm *BetaMemory) RemoveSuccessor(node LeftActivatable) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range bm.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	bm.successors = newSuccs
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
		for _, idx := range bm.indexes {
			idx.Add(token)
		}
	} else {
		delete(bm.tokens, sig)
		for _, idx := range bm.indexes {
			idx.Remove(token)
		}
	}
	succs := append([]LeftActivatable(nil), bm.successors...)
	bm.mu.Unlock()

	for _, s := range succs {
		s.LeftActivation(token, tag)
	}
}

// JoinTest specifies a relational test between a WME attribute and a variable bound in earlier tokens,
// or a disjunction constraint << ... >>.
type JoinTest struct {
	Attribute   string
	Op          model.Operator
	Variable    string
	VectorIndex int // -1 for scalar/membership, >= 0 for positional element in vector
	Disjunction []model.TestConstraint
}

// JoinNode joins left tokens with right WMEs from an AlphaMemory.
type JoinNode struct {
	mu          sync.RWMutex
	betaMemory  *BetaMemory
	alphaMemory *AlphaMemory
	joinTests   []JoinTest
	betaIndex   *BetaIndex
	alphaIndex  *AlphaIndex
	ce          *model.ConditionElement
	successors  []LeftActivatable
}

// NewJoinNode creates a new two-input join node.
func NewJoinNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, tests []JoinTest) *JoinNode {
	var leftVars []string
	var rightSpecs []AlphaIndexSpec

	for _, t := range tests {
		if len(t.Disjunction) == 0 && t.Op == model.OpEqual && t.Variable != "" {
			leftVars = append(leftVars, t.Variable)
			rightSpecs = append(rightSpecs, AlphaIndexSpec{
				Attribute:   t.Attribute,
				VectorIndex: t.VectorIndex,
			})
		}
	}

	var bi *BetaIndex
	if betaMem != nil {
		bi = betaMem.GetOrCreateIndex(leftVars)
	}

	var ai *AlphaIndex
	if alphaMem != nil {
		ai = alphaMem.GetOrCreateIndex(rightSpecs)
	}

	return &JoinNode{
		betaMemory:  betaMem,
		alphaMemory: alphaMem,
		joinTests:   tests,
		betaIndex:   bi,
		alphaIndex:  ai,
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

// RemoveSuccessor unregisters a downstream beta node.
func (jn *JoinNode) RemoveSuccessor(node LeftActivatable) {
	jn.mu.Lock()
	defer jn.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range jn.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	jn.successors = newSuccs
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
		wmeVal, ok := wme.Get(jt.Attribute)
		if !ok {
			wmeVal = model.NewSymbol("nil")
		}

		if len(jt.Disjunction) > 0 {
			matchedAny := false
			for _, dj := range jt.Disjunction {
				var targetVal model.Value
				if dj.Value.IsVariable() {
					if token == nil {
						continue
					}
					var found bool
					targetVal, found = token.Bindings[dj.Value.VariableName()]
					if !found {
						continue
					}
				} else {
					targetVal = dj.Value
				}
				if evalJoinTestValue(wmeVal, dj.Op, targetVal, jt.VectorIndex) {
					matchedAny = true
					break
				}
			}
			if !matchedAny {
				return false
			}
			continue
		}

		boundVal, exists := token.Bindings[jt.Variable]
		if !exists {
			return false
		}
		if !evalJoinTest(jt, boundVal, wmeVal) {
			return false
		}
	}
	return true
}

func evalJoinTestValue(wmeVal model.Value, op model.Operator, boundVal model.Value, vecIdx int) bool {
	if wmeVal.IsVector() {
		elems := wmeVal.VectorElements()
		if boundVal.IsVector() {
			return evalOp(wmeVal, op, boundVal)
		}
		if vecIdx >= 0 {
			if vecIdx < len(elems) {
				return evalOp(elems[vecIdx], op, boundVal)
			}
			return false
		}
		for _, elem := range elems {
			if evalOp(elem, op, boundVal) {
				return true
			}
		}
		return false
	}
	return evalOp(wmeVal, op, boundVal)
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
			if !hasVal {
				wmeVal = model.NewSymbol("nil")
				hasVal = true
			}
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
	if jn.alphaIndex == nil {
		return
	}

	key := jn.betaIndex.KeyForToken(token)
	wmes := jn.alphaIndex.Lookup(key)
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
	if jn.betaIndex == nil {
		return
	}

	keys := jn.alphaIndex.KeysForWME(wme)
	seen := make(map[string]bool)
	for _, key := range keys {
		tokens := jn.betaIndex.Lookup(key)
		for _, token := range tokens {
			sig := tokenSignature(token)
			if seen[sig] {
				continue
			}
			seen[sig] = true
			if jn.matches(token, wme) {
				newBindings, ok := jn.extractBindings(token, wme)
				if ok {
					childToken := NewToken(token, wme, newBindings)
					jn.propagate(childToken, tag)
				}
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
	alphaIndex  *AlphaIndex
	tokensIndex *BetaIndex
	ce          *model.ConditionElement
	// matches stores matching WME timetags per token signature
	matches    map[string]map[int64]bool
	tokens     map[string]*Token
	successors []LeftActivatable
}

// NewNegativeJoinNode creates a new NegativeJoinNode.
func NewNegativeJoinNode(betaMem *BetaMemory, alphaMem *AlphaMemory, ce *model.ConditionElement, tests []JoinTest) *NegativeJoinNode {
	var leftVars []string
	var rightSpecs []AlphaIndexSpec

	for _, t := range tests {
		if len(t.Disjunction) == 0 && t.Op == model.OpEqual && t.Variable != "" {
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

	return &NegativeJoinNode{
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

// RemoveSuccessor unregisters a downstream beta node.
func (njn *NegativeJoinNode) RemoveSuccessor(node LeftActivatable) {
	njn.mu.Lock()
	defer njn.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range njn.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	njn.successors = newSuccs
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
		njn.tokensIndex.Add(token)
		matchedWmes := make(map[int64]bool)

		if njn.alphaIndex != nil {
			key := njn.tokensIndex.KeyForToken(token)
			candidates := njn.alphaIndex.Lookup(key)
			for _, wme := range candidates {
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
		njn.tokensIndex.Remove(token)
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

	var candidateTokens []*Token
	if njn.alphaIndex != nil {
		keys := njn.alphaIndex.KeysForWME(wme)
		seen := make(map[string]bool)
		for _, key := range keys {
			for _, tok := range njn.tokensIndex.Lookup(key) {
				sig := tokenSignature(tok)
				if !seen[sig] {
					seen[sig] = true
					candidateTokens = append(candidateTokens, tok)
				}
			}
		}
	} else {
		for _, tok := range njn.tokens {
			candidateTokens = append(candidateTokens, tok)
		}
	}

	for _, token := range candidateTokens {
		if !njn.match(token, wme) {
			continue
		}
		sig := tokenSignature(token)

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
	active   bool
}

// NewTerminalNode creates a new TerminalNode for a rule.
func NewTerminalNode(rule *model.Rule, listener ConflictSetListener) *TerminalNode {
	return &TerminalNode{
		rule:     rule,
		listener: listener,
		active:   true,
	}
}

// Deactivate disables this terminal node so no further activations are propagated.
func (tn *TerminalNode) Deactivate() {
	tn.active = false
}

// LeftActivation handles a fully matched token arriving at the terminal node.
func (tn *TerminalNode) LeftActivation(token *Token, tag PropagationTag) {
	if !tn.active || tn.listener == nil {
		return
	}
	if tag == TagAdd {
		tn.listener.OnActivationAdd(tn.rule, token)
	} else {
		tn.listener.OnActivationRemove(tn.rule, token)
	}
}
