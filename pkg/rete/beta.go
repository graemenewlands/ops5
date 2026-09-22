package rete

import (
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
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

// LeftLink represents a doubly-linked list node connecting a LeftActivatable to a BetaMemory's active list.
type LeftLink struct {
	prev     *LeftLink
	next     *LeftLink
	target   LeftActivatable
	isLinked bool
}

// Target returns the LeftActivatable node associated with this link.
func (l *LeftLink) Target() LeftActivatable {
	return l.target
}

// IsLinked returns true if this link is currently linked into its BetaMemory.
func (l *LeftLink) IsLinked() bool {
	return l.isLinked
}

// LeftUnlinkable represents a two-input beta node that can be unlinked from its AlphaMemory
// when its parent BetaMemory has 0 tokens (Left Unlinking).
type LeftUnlinkable interface {
	OnLeftMemoryEmpty()
	OnLeftMemoryNonEmpty()
	IsRightLinked() bool
}

// LeftLinkProvider allows beta nodes to expose their LeftLink for O(1) doubly-linked unlinking.
type LeftLinkProvider interface {
	LeftLink() *LeftLink
}

// BetaMemory stores beta tokens and propagates them to child beta nodes.
type BetaMemory struct {
	mu              sync.RWMutex
	id              int
	tokens          map[string]*Token // Keyed by token signature
	successors      []LeftActivatable // All structural successors
	activeHead      *LeftLink         // Doubly-linked list head for active left activations
	activeTail      *LeftLink         // Doubly-linked list tail
	activeCount     int
	indexes         []*BetaIndex
	unlinkableNodes []LeftUnlinkable // Nodes to notify on 0 <-> 1 token transitions
}

// NewBetaMemory creates a new BetaMemory.
func NewBetaMemory() *BetaMemory {
	return &BetaMemory{
		tokens:          make(map[string]*Token),
		successors:      make([]LeftActivatable, 0),
		indexes:         make([]*BetaIndex, 0),
		unlinkableNodes: make([]LeftUnlinkable, 0),
	}
}

// LinkSuccessor adds a LeftLink to the active doubly-linked list in O(1) time.
func (bm *BetaMemory) LinkSuccessor(link *LeftLink) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if link.isLinked {
		return
	}
	link.prev = bm.activeTail
	link.next = nil
	link.isLinked = true
	if bm.activeTail != nil {
		bm.activeTail.next = link
	} else {
		bm.activeHead = link
	}
	bm.activeTail = link
	bm.activeCount++
}

// UnlinkSuccessor removes a LeftLink from the active doubly-linked list in O(1) time.
func (bm *BetaMemory) UnlinkSuccessor(link *LeftLink) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if !link.isLinked {
		return
	}
	if link.prev != nil {
		link.prev.next = link.next
	} else {
		bm.activeHead = link.next
	}
	if link.next != nil {
		link.next.prev = link.prev
	} else {
		bm.activeTail = link.prev
	}
	link.prev = nil
	link.next = nil
	link.isLinked = false
	bm.activeCount--
}

// ActiveSuccessorCount returns the number of currently linked (active) successors in this BetaMemory.
func (bm *BetaMemory) ActiveSuccessorCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.activeCount
}

// IsSuccessorActive returns true if the specified node is currently linked to receive left activations.
func (bm *BetaMemory) IsSuccessorActive(node LeftActivatable) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	if provider, ok := node.(LeftLinkProvider); ok {
		return provider.LeftLink().IsLinked()
	}
	for curr := bm.activeHead; curr != nil; curr = curr.next {
		if curr.target == node {
			return true
		}
	}
	return false
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
	if t == nil {
		return ""
	}
	return t.Signature()
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
	bm.successors = append(bm.successors, node)

	if unlinkable, ok := node.(LeftUnlinkable); ok {
		bm.unlinkableNodes = append(bm.unlinkableNodes, unlinkable)
	}

	if provider, ok := node.(LeftLinkProvider); ok {
		link := provider.LeftLink()
		link.target = node
		if link.isLinked && link.prev == nil && link.next == nil && bm.activeHead != link {
			link.prev = bm.activeTail
			link.next = nil
			if bm.activeTail != nil {
				bm.activeTail.next = link
			} else {
				bm.activeHead = link
			}
			bm.activeTail = link
			bm.activeCount++
		}
	} else {
		// Non-unlinkable successor (TerminalNode, EvalNode, etc.): permanently active
		link := &LeftLink{target: node, isLinked: true}
		if bm.activeTail != nil {
			bm.activeTail.next = link
			link.prev = bm.activeTail
			bm.activeTail = link
		} else {
			bm.activeHead = link
			bm.activeTail = link
		}
		bm.activeCount++
	}

	// Catch-up: send existing tokens to new successor only if currently linked
	var toks []*Token
	isLinked := true
	if ru, ok := node.(RightUnlinkable); ok {
		isLinked = ru.IsLeftLinked()
	}
	if isLinked {
		for _, tok := range bm.tokens {
			toks = append(toks, tok)
		}
	}
	bm.mu.Unlock()

	for _, tok := range toks {
		node.LeftActivation(tok, TagAdd)
	}
}

// RemoveSuccessor unregisters a child beta node.
func (bm *BetaMemory) RemoveSuccessor(node LeftActivatable) {
	bm.mu.Lock()
	var newSuccs []LeftActivatable
	for _, s := range bm.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	bm.successors = newSuccs

	if unlinkable, ok := node.(LeftUnlinkable); ok {
		var newUnlinkables []LeftUnlinkable
		for _, u := range bm.unlinkableNodes {
			if u != unlinkable {
				newUnlinkables = append(newUnlinkables, u)
			}
		}
		bm.unlinkableNodes = newUnlinkables
	}

	if provider, ok := node.(LeftLinkProvider); ok {
		link := provider.LeftLink()
		if link.isLinked {
			if link.prev != nil {
				link.prev.next = link.next
			} else {
				bm.activeHead = link.next
			}
			if link.next != nil {
				link.next.prev = link.prev
			} else {
				bm.activeTail = link.prev
			}
			link.prev = nil
			link.next = nil
			link.isLinked = false
			bm.activeCount--
		}
	} else {
		for curr := bm.activeHead; curr != nil; curr = curr.next {
			if curr.target == node {
				if curr.prev != nil {
					curr.prev.next = curr.next
				} else {
					bm.activeHead = curr.next
				}
				if curr.next != nil {
					curr.next.prev = curr.prev
				} else {
					bm.activeTail = curr.prev
				}
				curr.prev = nil
				curr.next = nil
				curr.isLinked = false
				bm.activeCount--
				break
			}
		}
	}
	bm.mu.Unlock()
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
	var transition int // 1: 0 -> 1, -1: 1 -> 0

	if tag == TagAdd {
		bm.tokens[sig] = token
		for _, idx := range bm.indexes {
			idx.Add(token)
		}
		if len(bm.tokens) == 1 {
			transition = 1
		}
	} else {
		delete(bm.tokens, sig)
		for _, idx := range bm.indexes {
			idx.Remove(token)
		}
		if len(bm.tokens) == 0 {
			transition = -1
		}
	}

	var notifyNodes []LeftUnlinkable
	if transition != 0 {
		notifyNodes = append([]LeftUnlinkable(nil), bm.unlinkableNodes...)
	}
	bm.mu.Unlock()

	// If transitioning 0 -> 1: Re-link child nodes to their AlphaMemories
	if transition == 1 {
		for _, node := range notifyNodes {
			node.OnLeftMemoryNonEmpty()
		}
	}

	// Snapshot active successors from the doubly-linked list
	bm.mu.RLock()
	if bm.activeHead == nil {
		bm.mu.RUnlock()
	} else if bm.activeHead.next == nil {
		s := bm.activeHead.target
		bm.mu.RUnlock()
		s.LeftActivation(token, tag)
	} else {
		var succs []LeftActivatable
		for curr := bm.activeHead; curr != nil; curr = curr.next {
			succs = append(succs, curr.target)
		}
		bm.mu.RUnlock()

		for _, s := range succs {
			s.LeftActivation(token, tag)
		}
	}

	// If transitioning 1 -> 0: Unlink child nodes from their AlphaMemories (after retraction)
	if transition == -1 {
		for _, node := range notifyNodes {
			node.OnLeftMemoryEmpty()
		}
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

	leftLink  LeftLink  // links to betaMemory.activeHead
	rightLink RightLink // links to alphaMemory.activeHead
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

	jn := &JoinNode{
		betaMemory:  betaMem,
		alphaMemory: alphaMem,
		joinTests:   tests,
		betaIndex:   bi,
		alphaIndex:  ai,
		ce:          ce,
		successors:  make([]LeftActivatable, 0),
	}
	jn.leftLink.target = jn
	jn.rightLink.target = jn
	return jn
}

// Attach connects the join node to its parent memories and sets up unlinking.
func (jn *JoinNode) Attach() {
	if jn.alphaMemory != nil {
		jn.alphaMemory.AddSuccessor(jn)
		// If betaMemory already has tokens, link rightLink into alphaMemory
		if jn.betaMemory != nil && jn.betaMemory.TokenCount() > 0 {
			jn.alphaMemory.LinkSuccessor(&jn.rightLink)
		}
	}
	if jn.betaMemory != nil {
		// If alphaMemory already has WMEs, link leftLink into betaMemory
		if jn.alphaMemory != nil && jn.alphaMemory.ItemCount() > 0 {
			jn.betaMemory.LinkSuccessor(&jn.leftLink)
		}
		jn.betaMemory.AddSuccessor(jn)
	}
}

// LeftLink returns the LeftLink associated with this join node.
func (jn *JoinNode) LeftLink() *LeftLink {
	return &jn.leftLink
}

// RightLink returns the RightLink associated with this join node.
func (jn *JoinNode) RightLink() *RightLink {
	return &jn.rightLink
}

// IsLeftLinked returns true if this join node is linked to receive left activations from its BetaMemory.
func (jn *JoinNode) IsLeftLinked() bool {
	return jn.leftLink.IsLinked()
}

// IsRightLinked returns true if this join node is linked to receive right activations from its AlphaMemory.
func (jn *JoinNode) IsRightLinked() bool {
	return jn.rightLink.IsLinked()
}

// OnRightMemoryNonEmpty is called when alphaMemory item count transitions 0 -> 1.
// Re-links this join node to its BetaMemory (Right Unlinking).
func (jn *JoinNode) OnRightMemoryNonEmpty() {
	if jn.betaMemory != nil {
		jn.betaMemory.LinkSuccessor(&jn.leftLink)
	}
}

// OnRightMemoryEmpty is called when alphaMemory item count transitions 1 -> 0.
// Unlinks this join node from its BetaMemory (Right Unlinking).
func (jn *JoinNode) OnRightMemoryEmpty() {
	if jn.betaMemory != nil {
		jn.betaMemory.UnlinkSuccessor(&jn.leftLink)
	}
}

// OnLeftMemoryNonEmpty is called when betaMemory token count transitions 0 -> 1.
// Re-links this join node to its AlphaMemory (Left Unlinking).
func (jn *JoinNode) OnLeftMemoryNonEmpty() {
	if jn.alphaMemory != nil {
		jn.alphaMemory.LinkSuccessor(&jn.rightLink)
	}
}

// OnLeftMemoryEmpty is called when betaMemory token count transitions 1 -> 0.
// Unlinks this join node from its AlphaMemory (Left Unlinking).
func (jn *JoinNode) OnLeftMemoryEmpty() {
	if jn.alphaMemory != nil {
		jn.alphaMemory.UnlinkSuccessor(&jn.rightLink)
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
					targetVal, found = token.GetBinding(dj.Value.VariableName())
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

		boundVal, exists := token.GetBinding(jt.Variable)
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
func (jn *JoinNode) extractBindings(token *Token, wme *model.WME, buf []Binding) ([]Binding, bool) {
	newBindings := buf

	if jn.ce != nil {
		if jn.ce.ElementVariable != "" {
			newBindings = append(newBindings, Binding{
				Name:  jn.ce.ElementVariable,
				Value: model.NewInt(wme.Timetag),
			})
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
						if existing, ok := token.GetBinding(vName); ok {
							if !hasVal || !matchValueOrVector(targetVal, existing) {
								return nil, false
							}
						} else {
							// Check intra-condition consistency within newBindings
							foundIntra := false
							for _, b := range newBindings {
								if b.Name == vName {
									if !hasVal || !matchValueOrVector(targetVal, b.Value) {
										return nil, false
									}
									foundIntra = true
									break
								}
							}
							if !foundIntra && hasVal {
								newBindings = append(newBindings, Binding{Name: vName, Value: targetVal})
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
	var bindBuf [4]Binding
	for _, wme := range wmes {
		if jn.matches(token, wme) {
			newBindings, ok := jn.extractBindings(token, wme, bindBuf[:0])
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
	if len(keys) == 0 {
		return
	}
	var bindBuf [4]Binding
	if len(keys) == 1 {
		tokens := jn.betaIndex.Lookup(keys[0])
		for _, token := range tokens {
			if jn.matches(token, wme) {
				newBindings, ok := jn.extractBindings(token, wme, bindBuf[:0])
				if ok {
					childToken := NewToken(token, wme, newBindings)
					jn.propagate(childToken, tag)
				}
			}
		}
		return
	}

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
				newBindings, ok := jn.extractBindings(token, wme, bindBuf[:0])
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
	n := len(jn.successors)
	if n == 0 {
		jn.mu.RUnlock()
		return
	}
	if n == 1 {
		s := jn.successors[0]
		jn.mu.RUnlock()
		s.LeftActivation(token, tag)
		return
	}
	if n == 2 {
		s0, s1 := jn.successors[0], jn.successors[1]
		jn.mu.RUnlock()
		s0.LeftActivation(token, tag)
		s1.LeftActivation(token, tag)
		return
	}
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

	leftLink  LeftLink  // always linked to betaMemory (negative conditions need left activations when alpha is empty!)
	rightLink RightLink // unlinked from alphaMemory when betaMemory has 0 tokens
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

	njn := &NegativeJoinNode{
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
	njn.leftLink.target = njn
	njn.rightLink.target = njn
	return njn
}

// Attach connects the negative join node to its parent memories and sets up unlinking.
func (njn *NegativeJoinNode) Attach() {
	if njn.alphaMemory != nil {
		njn.alphaMemory.AddSuccessor(njn)
		if njn.betaMemory != nil && njn.betaMemory.TokenCount() > 0 {
			njn.alphaMemory.LinkSuccessor(&njn.rightLink)
		}
	}
	if njn.betaMemory != nil {
		// Negative joins are ALWAYS left-linked (tokens must pass through when alpha is empty)
		njn.betaMemory.LinkSuccessor(&njn.leftLink)
		njn.betaMemory.AddSuccessor(njn)
	}
}

// LeftLink returns the LeftLink associated with this negative join node.
func (njn *NegativeJoinNode) LeftLink() *LeftLink {
	return &njn.leftLink
}

// RightLink returns the RightLink associated with this negative join node.
func (njn *NegativeJoinNode) RightLink() *RightLink {
	return &njn.rightLink
}

// IsLeftLinked returns true (negative joins are always left-linked).
func (njn *NegativeJoinNode) IsLeftLinked() bool {
	return njn.leftLink.IsLinked()
}

// IsRightLinked returns true if this negative join node is linked to its AlphaMemory.
func (njn *NegativeJoinNode) IsRightLinked() bool {
	return njn.rightLink.IsLinked()
}

// OnLeftMemoryNonEmpty is called when betaMemory token count transitions 0 -> 1.
// Re-links this negative join node to its AlphaMemory (Left Unlinking).
func (njn *NegativeJoinNode) OnLeftMemoryNonEmpty() {
	if njn.alphaMemory != nil {
		njn.alphaMemory.LinkSuccessor(&njn.rightLink)
	}
}

// OnLeftMemoryEmpty is called when betaMemory token count transitions 1 -> 0.
// Unlinks this negative join node from its AlphaMemory (Left Unlinking).
func (njn *NegativeJoinNode) OnLeftMemoryEmpty() {
	if njn.alphaMemory != nil {
		njn.alphaMemory.UnlinkSuccessor(&njn.rightLink)
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
	mu              sync.RWMutex
	rule            *model.Rule
	listener        ConflictSetListener
	active          bool
	activationCount int
}

// NewTerminalNode creates a new TerminalNode for a rule.
func NewTerminalNode(rule *model.Rule, listener ConflictSetListener) *TerminalNode {
	return &TerminalNode{
		rule:     rule,
		listener: listener,
		active:   true,
	}
}

// ActivationCount returns the number of active token instantiations at this terminal node.
func (tn *TerminalNode) ActivationCount() int {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	return tn.activationCount
}

// Deactivate disables this terminal node so no further activations are propagated.
func (tn *TerminalNode) Deactivate() {
	tn.mu.Lock()
	defer tn.mu.Unlock()
	tn.active = false
}

// LeftActivation handles a fully matched token arriving at the terminal node.
func (tn *TerminalNode) LeftActivation(token *Token, tag PropagationTag) {
	tn.mu.Lock()
	if !tn.active || tn.listener == nil {
		tn.mu.Unlock()
		return
	}
	if tag == TagAdd {
		tn.activationCount++
	} else {
		tn.activationCount--
		if tn.activationCount < 0 {
			tn.activationCount = 0
		}
	}
	listener := tn.listener
	rule := tn.rule
	tn.mu.Unlock()

	if tag == TagAdd {
		listener.OnActivationAdd(rule, token)
	} else {
		listener.OnActivationRemove(rule, token)
	}
}
