package rete

import (
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
)

// AlphaNode represents any node in the alpha network capable of processing a WME.
type AlphaNode interface {
	Activation(wme *model.WME, tag PropagationTag)
}

// RightActivatable represents a beta node that receives right activations from an AlphaMemory.
type RightActivatable interface {
	RightActivation(wme *model.WME, tag PropagationTag)
}

// RightLink represents a doubly-linked list node connecting a RightActivatable to an AlphaMemory's active list.
type RightLink struct {
	prev     *RightLink
	next     *RightLink
	target   RightActivatable
	isLinked bool
}

// Target returns the RightActivatable node associated with this link.
func (r *RightLink) Target() RightActivatable {
	return r.target
}

// IsLinked returns true if this link is currently linked into its AlphaMemory.
func (r *RightLink) IsLinked() bool {
	return r.isLinked
}

// RightUnlinkable represents a two-input beta node that can be unlinked from its parent BetaMemory
// when its AlphaMemory has 0 WMEs (Right Unlinking).
type RightUnlinkable interface {
	OnRightMemoryEmpty()
	OnRightMemoryNonEmpty()
	IsLeftLinked() bool
}

// RightLinkProvider allows beta nodes to expose their RightLink for O(1) doubly-linked unlinking.
type RightLinkProvider interface {
	RightLink() *RightLink
}

// AlphaMemory stores WMEs that satisfy all intra-element condition tests for a pattern.
type AlphaMemory struct {
	mu              sync.RWMutex
	items           map[int64]*model.WME
	successors      []RightActivatable // All structural successors (for inspection, export, etc.)
	activeHead      *RightLink         // Doubly-linked list head for active right activations
	activeTail      *RightLink         // Doubly-linked list tail
	activeCount     int
	indexes         []*AlphaIndex
	unlinkableNodes []RightUnlinkable // Nodes to notify on 0 <-> 1 WME transitions
}

// NewAlphaMemory creates a new AlphaMemory.
func NewAlphaMemory() *AlphaMemory {
	return &AlphaMemory{
		items:           make(map[int64]*model.WME),
		successors:      make([]RightActivatable, 0),
		indexes:         make([]*AlphaIndex, 0),
		unlinkableNodes: make([]RightUnlinkable, 0),
	}
}

// LinkSuccessor adds a RightLink to the active doubly-linked list in O(1) time.
func (am *AlphaMemory) LinkSuccessor(link *RightLink) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if link.isLinked {
		return
	}
	link.prev = am.activeTail
	link.next = nil
	link.isLinked = true
	if am.activeTail != nil {
		am.activeTail.next = link
	} else {
		am.activeHead = link
	}
	am.activeTail = link
	am.activeCount++
}

// UnlinkSuccessor removes a RightLink from the active doubly-linked list in O(1) time.
func (am *AlphaMemory) UnlinkSuccessor(link *RightLink) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if !link.isLinked {
		return
	}
	if link.prev != nil {
		link.prev.next = link.next
	} else {
		am.activeHead = link.next
	}
	if link.next != nil {
		link.next.prev = link.prev
	} else {
		am.activeTail = link.prev
	}
	link.prev = nil
	link.next = nil
	link.isLinked = false
	am.activeCount--
}

// ActiveSuccessorCount returns the number of currently linked (active) successors in this AlphaMemory.
func (am *AlphaMemory) ActiveSuccessorCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.activeCount
}

// IsSuccessorActive returns true if the specified node is currently linked to receive right activations.
func (am *AlphaMemory) IsSuccessorActive(node RightActivatable) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	if provider, ok := node.(RightLinkProvider); ok {
		return provider.RightLink().IsLinked()
	}
	for curr := am.activeHead; curr != nil; curr = curr.next {
		if curr.target == node {
			return true
		}
	}
	return false
}

// AddSuccessor registers a beta node to receive right activations.
func (am *AlphaMemory) AddSuccessor(node RightActivatable) {
	am.mu.Lock()
	am.successors = append(am.successors, node)

	if unlinkable, ok := node.(RightUnlinkable); ok {
		am.unlinkableNodes = append(am.unlinkableNodes, unlinkable)
	}

	if provider, ok := node.(RightLinkProvider); ok {
		link := provider.RightLink()
		link.target = node
		if link.isLinked && link.prev == nil && link.next == nil && am.activeHead != link {
			link.prev = am.activeTail
			link.next = nil
			if am.activeTail != nil {
				am.activeTail.next = link
			} else {
				am.activeHead = link
			}
			am.activeTail = link
			am.activeCount++
		}
	} else {
		// Non-unlinkable successor: permanently active
		link := &RightLink{target: node, isLinked: true}
		if am.activeTail != nil {
			am.activeTail.next = link
			link.prev = am.activeTail
			am.activeTail = link
		} else {
			am.activeHead = link
			am.activeTail = link
		}
		am.activeCount++
	}
	am.mu.Unlock()
}

// RemoveSuccessor unregisters a beta node from receiving right activations.
func (am *AlphaMemory) RemoveSuccessor(node RightActivatable) {
	am.mu.Lock()
	var newSuccs []RightActivatable
	for _, s := range am.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	am.successors = newSuccs

	if unlinkable, ok := node.(RightUnlinkable); ok {
		var newUnlinkables []RightUnlinkable
		for _, u := range am.unlinkableNodes {
			if u != unlinkable {
				newUnlinkables = append(newUnlinkables, u)
			}
		}
		am.unlinkableNodes = newUnlinkables
	}

	if provider, ok := node.(RightLinkProvider); ok {
		link := provider.RightLink()
		if link.isLinked {
			if link.prev != nil {
				link.prev.next = link.next
			} else {
				am.activeHead = link.next
			}
			if link.next != nil {
				link.next.prev = link.prev
			} else {
				am.activeTail = link.prev
			}
			link.prev = nil
			link.next = nil
			link.isLinked = false
			am.activeCount--
		}
	} else {
		for curr := am.activeHead; curr != nil; curr = curr.next {
			if curr.target == node {
				if curr.prev != nil {
					curr.prev.next = curr.next
				} else {
					am.activeHead = curr.next
				}
				if curr.next != nil {
					curr.next.prev = curr.prev
				} else {
					am.activeTail = curr.prev
				}
				curr.prev = nil
				curr.next = nil
				curr.isLinked = false
				am.activeCount--
				break
			}
		}
	}
	am.mu.Unlock()
}

// GetOrCreateIndex returns an existing AlphaIndex matching specs or creates and populates a new one.
func (am *AlphaMemory) GetOrCreateIndex(specs []AlphaIndexSpec) *AlphaIndex {
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, idx := range am.indexes {
		if alphaSpecsEqual(idx.specs, specs) {
			return idx
		}
	}

	idx := NewAlphaIndex(specs)
	for _, wme := range am.items {
		idx.Add(wme)
	}
	am.indexes = append(am.indexes, idx)
	return idx
}

func alphaSpecsEqual(a, b []AlphaIndexSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Attribute != b[i].Attribute || a[i].VectorIndex != b[i].VectorIndex {
			return false
		}
	}
	return true
}

// Items returns a snapshot of all WMEs currently stored.
func (am *AlphaMemory) Items() []*model.WME {
	am.mu.RLock()
	defer am.mu.RUnlock()
	res := make([]*model.WME, 0, len(am.items))
	for _, w := range am.items {
		res = append(res, w)
	}
	return res
}

// ItemCount returns the number of WMEs currently stored in this AlphaMemory.
func (am *AlphaMemory) ItemCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.items)
}

// Activation processes an incoming WME assertion or retraction.
func (am *AlphaMemory) Activation(wme *model.WME, tag PropagationTag) {
	am.mu.Lock()
	var transition int // 1: 0 -> 1, -1: 1 -> 0

	if tag == TagAdd {
		am.items[wme.Timetag] = wme
		for _, idx := range am.indexes {
			idx.Add(wme)
		}
		if len(am.items) == 1 {
			transition = 1
		}
	} else {
		delete(am.items, wme.Timetag)
		for _, idx := range am.indexes {
			idx.Remove(wme)
		}
		if len(am.items) == 0 {
			transition = -1
		}
	}

	var notifyNodes []RightUnlinkable
	if transition != 0 {
		notifyNodes = append([]RightUnlinkable(nil), am.unlinkableNodes...)
	}
	am.mu.Unlock()

	// If transitioning 0 -> 1: Re-link downstream nodes to their parent BetaMemories
	if transition == 1 {
		for _, node := range notifyNodes {
			node.OnRightMemoryNonEmpty()
		}
	}

	// Snapshot active successors from the doubly-linked list
	am.mu.RLock()
	var succs []RightActivatable
	for curr := am.activeHead; curr != nil; curr = curr.next {
		succs = append(succs, curr.target)
	}
	am.mu.RUnlock()

	for _, s := range succs {
		s.RightActivation(wme, tag)
	}

	// If transitioning 1 -> 0: Unlink downstream nodes from their parent BetaMemories (after retraction)
	if transition == -1 {
		for _, node := range notifyNodes {
			node.OnRightMemoryEmpty()
		}
	}
}

// ConstantTestNode evaluates a constant constraint or disjunction on a single attribute.
type ConstantTestNode struct {
	Attribute   string
	Op          model.Operator
	Value       model.Value
	Disjunction []model.TestConstraint
	VectorIndex int // -1 for scalar/membership test, >= 0 for positional element test
	successors  []AlphaNode
}

// NewConstantTestNode creates a new test node.
func NewConstantTestNode(attr string, op model.Operator, val model.Value) *ConstantTestNode {
	return NewIndexedConstantTestNode(attr, op, val, -1)
}

// NewIndexedConstantTestNode creates a new test node with an explicit vector element index.
func NewIndexedConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	return &ConstantTestNode{
		Attribute:   model.NormalizeAttribute(attr),
		Op:          op,
		Value:       val,
		VectorIndex: vecIdx,
		successors:  make([]AlphaNode, 0),
	}
}

// NewDisjunctiveConstantTestNode creates a test node that tests an attribute against a disjunction << ... >>.
func NewDisjunctiveConstantTestNode(attr string, disj []model.TestConstraint) *ConstantTestNode {
	return NewIndexedDisjunctiveConstantTestNode(attr, disj, -1)
}

// NewIndexedDisjunctiveConstantTestNode creates a test node with vector element index and disjunction << ... >>.
func NewIndexedDisjunctiveConstantTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	return &ConstantTestNode{
		Attribute:   model.NormalizeAttribute(attr),
		Disjunction: disj,
		VectorIndex: vecIdx,
		successors:  make([]AlphaNode, 0),
	}
}

// AddSuccessor adds a child alpha node or alpha memory.
func (ct *ConstantTestNode) AddSuccessor(succ AlphaNode) {
	ct.successors = append(ct.successors, succ)
}

func evalOp(val model.Value, op model.Operator, target model.Value) bool {
	switch op {
	case model.OpEqual:
		return val.Equal(target)
	case model.OpNotEqual:
		return !val.Equal(target)
	case model.OpLess:
		cmp, err := val.Compare(target)
		return err == nil && cmp < 0
	case model.OpLessEqual:
		cmp, err := val.Compare(target)
		return err == nil && cmp <= 0
	case model.OpGreater:
		cmp, err := val.Compare(target)
		return err == nil && cmp > 0
	case model.OpGreaterEqual:
		cmp, err := val.Compare(target)
		return err == nil && cmp >= 0
	default:
		return false
	}
}

func matchesDisjunction(val model.Value, disj []model.TestConstraint) bool {
	for _, c := range disj {
		if evalOp(val, c.Op, c.Value) {
			return true
		}
	}
	return false
}

// Test evaluates the constraint against the WME.
func (ct *ConstantTestNode) Test(wme *model.WME) bool {
	val, ok := wme.Get(ct.Attribute)
	if !ok {
		val = model.NewSymbol("nil")
	}

	if len(ct.Disjunction) > 0 {
		if val.IsVector() {
			elems := val.VectorElements()
			if ct.VectorIndex >= 0 {
				if ct.VectorIndex < len(elems) {
					return matchesDisjunction(elems[ct.VectorIndex], ct.Disjunction)
				}
				return false
			}
			for _, elem := range elems {
				if matchesDisjunction(elem, ct.Disjunction) {
					return true
				}
			}
			return false
		}
		return matchesDisjunction(val, ct.Disjunction)
	}

	if val.IsVector() {
		elems := val.VectorElements()
		if ct.Value.IsVector() {
			return evalOp(val, ct.Op, ct.Value)
		}
		if ct.VectorIndex >= 0 {
			if ct.VectorIndex < len(elems) {
				return evalOp(elems[ct.VectorIndex], ct.Op, ct.Value)
			}
			return false
		}
		// Membership test (VectorIndex == -1):
		for _, elem := range elems {
			if evalOp(elem, ct.Op, ct.Value) {
				return true
			}
		}
		return false
	}

	return evalOp(val, ct.Op, ct.Value)
}

// Activation evaluates the test and propagates to successors if satisfied.
func (ct *ConstantTestNode) Activation(wme *model.WME, tag PropagationTag) {
	if ct.Test(wme) {
		for _, s := range ct.successors {
			s.Activation(wme, tag)
		}
	}
}

// TypeNode filters WMEs by their class name.
type TypeNode struct {
	Class      string
	successors []AlphaNode
}

// NewTypeNode creates a new TypeNode.
func NewTypeNode(class string) *TypeNode {
	return &TypeNode{
		Class:      class,
		successors: make([]AlphaNode, 0),
	}
}

// AddSuccessor adds a downstream alpha node.
func (tn *TypeNode) AddSuccessor(succ AlphaNode) {
	tn.successors = append(tn.successors, succ)
}

// Activation routes WMEs matching the class name to successors.
func (tn *TypeNode) Activation(wme *model.WME, tag PropagationTag) {
	if tn.Class == "*" || tn.Class == wme.Class {
		for _, s := range tn.successors {
			s.Activation(wme, tag)
		}
	}
}

// AlphaRootNode receives all WME events and routes them to TypeNodes.
type AlphaRootNode struct {
	mu        sync.RWMutex
	typeNodes map[string]*TypeNode
}

// NewAlphaRootNode creates a new AlphaRootNode.
func NewAlphaRootNode() *AlphaRootNode {
	return &AlphaRootNode{
		typeNodes: make(map[string]*TypeNode),
	}
}

// GetOrCreateTypeNode retrieves or instantiates a TypeNode for a given class.
func (arn *AlphaRootNode) GetOrCreateTypeNode(class string) *TypeNode {
	arn.mu.Lock()
	defer arn.mu.Unlock()

	if tn, exists := arn.typeNodes[class]; exists {
		return tn
	}
	tn := NewTypeNode(class)
	arn.typeNodes[class] = tn
	return tn
}

// Activation routes the WME event to the matching TypeNode.
func (arn *AlphaRootNode) Activation(wme *model.WME, tag PropagationTag) {
	arn.mu.RLock()
	tn, exists := arn.typeNodes[wme.Class]
	wildcard, hasWildcard := arn.typeNodes["*"]
	arn.mu.RUnlock()

	if exists {
		tn.Activation(wme, tag)
	}
	if hasWildcard {
		wildcard.Activation(wme, tag)
	}
}
