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

// Successors returns all downstream AlphaNodes.
func (ct *ConstantTestNode) Successors() []AlphaNode {
	return ct.successors
}

// GetOrCreateSwitchNode retrieves an existing child switch node or registers a new one.
func (ct *ConstantTestNode) GetOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	for _, s := range ct.successors {
		if sw, ok := s.(*AlphaSwitchNode); ok {
			if sw.Attribute == attr && sw.VectorIndex == vecIdx {
				return sw
			}
		}
	}
	sw := NewIndexedAlphaSwitchNode(attr, vecIdx)
	ct.AddSuccessor(sw)
	return sw
}

// GetOrCreateConstantTestNode retrieves an existing child constant test node or registers a new one.
func (ct *ConstantTestNode) GetOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	for _, s := range ct.successors {
		if child, ok := s.(*ConstantTestNode); ok {
			if child.Attribute == attr && child.Op == op && child.Value.Equal(val) && child.VectorIndex == vecIdx && len(child.Disjunction) == 0 {
				return child
			}
		}
	}
	child := NewIndexedConstantTestNode(attr, op, val, vecIdx)
	ct.AddSuccessor(child)
	return child
}

// GetOrCreateDisjunctiveTestNode retrieves an existing child disjunctive test node or registers a new one.
func (ct *ConstantTestNode) GetOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	for _, s := range ct.successors {
		if child, ok := s.(*ConstantTestNode); ok {
			if child.Attribute == attr && child.VectorIndex == vecIdx && testConstraintsEqual(child.Disjunction, disj) {
				return child
			}
		}
	}
	child := NewIndexedDisjunctiveConstantTestNode(attr, disj, vecIdx)
	ct.AddSuccessor(child)
	return child
}

// AlphaSwitchNode partitions WME activations by attribute value using O(1) hash table lookup.
type AlphaSwitchNode struct {
	mu          sync.RWMutex
	Attribute   string
	VectorIndex int // -1 for scalar/membership test, >= 0 for positional element test
	cases       map[string][]AlphaNode
}

// NewAlphaSwitchNode creates a new switch node for a scalar attribute.
func NewAlphaSwitchNode(attr string) *AlphaSwitchNode {
	return NewIndexedAlphaSwitchNode(attr, -1)
}

// NewIndexedAlphaSwitchNode creates a new switch node for an attribute at a positional element or scalar.
func NewIndexedAlphaSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	return &AlphaSwitchNode{
		Attribute:   model.NormalizeAttribute(attr),
		VectorIndex: vecIdx,
		cases:       make(map[string][]AlphaNode),
	}
}

// BranchCount returns the number of distinct constant value branches registered on this switch node.
func (asn *AlphaSwitchNode) BranchCount() int {
	asn.mu.RLock()
	defer asn.mu.RUnlock()
	return len(asn.cases)
}

// CaseKeys returns all registered case keys in this switch node.
func (asn *AlphaSwitchNode) CaseKeys() []string {
	asn.mu.RLock()
	defer asn.mu.RUnlock()
	keys := make([]string, 0, len(asn.cases))
	for k := range asn.cases {
		keys = append(keys, k)
	}
	return keys
}

// SuccessorsForCase returns the AlphaNodes associated with a specific canonical key.
func (asn *AlphaSwitchNode) SuccessorsForCase(key string) []AlphaNode {
	asn.mu.RLock()
	defer asn.mu.RUnlock()
	return append([]AlphaNode(nil), asn.cases[key]...)
}

// AddSuccessor registers a child AlphaNode under the specified constant value branch.
func (asn *AlphaSwitchNode) AddSuccessor(key string, succ AlphaNode) {
	asn.mu.Lock()
	defer asn.mu.Unlock()
	for _, s := range asn.cases[key] {
		if s == succ {
			return
		}
	}
	asn.cases[key] = append(asn.cases[key], succ)
}

// GetOrCreateSwitchNode retrieves an existing child switch node under a branch or registers a new one.
func (asn *AlphaSwitchNode) GetOrCreateSwitchNode(key string, attr string, vecIdx int) *AlphaSwitchNode {
	asn.mu.Lock()
	defer asn.mu.Unlock()
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range asn.cases[key] {
		if sw, ok := s.(*AlphaSwitchNode); ok {
			if sw.Attribute == normAttr && sw.VectorIndex == vecIdx {
				return sw
			}
		}
	}
	sw := NewIndexedAlphaSwitchNode(normAttr, vecIdx)
	asn.cases[key] = append(asn.cases[key], sw)
	return sw
}

// GetOrCreateConstantTestNode retrieves an existing child constant test node under a branch or registers a new one.
func (asn *AlphaSwitchNode) GetOrCreateConstantTestNode(key string, attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	asn.mu.Lock()
	defer asn.mu.Unlock()
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range asn.cases[key] {
		if ct, ok := s.(*ConstantTestNode); ok {
			if ct.Attribute == normAttr && ct.Op == op && ct.Value.Equal(val) && ct.VectorIndex == vecIdx && len(ct.Disjunction) == 0 {
				return ct
			}
		}
	}
	ct := NewIndexedConstantTestNode(normAttr, op, val, vecIdx)
	asn.cases[key] = append(asn.cases[key], ct)
	return ct
}

// GetOrCreateDisjunctiveTestNode retrieves an existing child disjunctive test node under a branch or registers a new one.
func (asn *AlphaSwitchNode) GetOrCreateDisjunctiveTestNode(key string, attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	asn.mu.Lock()
	defer asn.mu.Unlock()
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range asn.cases[key] {
		if ct, ok := s.(*ConstantTestNode); ok {
			if ct.Attribute == normAttr && ct.VectorIndex == vecIdx && testConstraintsEqual(ct.Disjunction, disj) {
				return ct
			}
		}
	}
	ct := NewIndexedDisjunctiveConstantTestNode(normAttr, disj, vecIdx)
	asn.cases[key] = append(asn.cases[key], ct)
	return ct
}

// Activation extracts the attribute value and routes the WME event to matching branches in O(1).
func (asn *AlphaSwitchNode) Activation(wme *model.WME, tag PropagationTag) {
	val, ok := wme.Get(asn.Attribute)
	if !ok {
		val = model.NewSymbol("nil")
	}

	if val.IsVector() {
		elems := val.VectorElements()
		if asn.VectorIndex >= 0 {
			if asn.VectorIndex < len(elems) {
				k := CanonicalValueKey(elems[asn.VectorIndex])
				asn.mu.RLock()
				succs := asn.cases[k]
				asn.mu.RUnlock()
				for _, s := range succs {
					s.Activation(wme, tag)
				}
			}
			return
		}
		// Membership test (VectorIndex == -1)
		if len(elems) == 1 {
			k := CanonicalValueKey(elems[0])
			asn.mu.RLock()
			succs := asn.cases[k]
			asn.mu.RUnlock()
			for _, s := range succs {
				s.Activation(wme, tag)
			}
			return
		}
		var seen [8]string
		var seenHeap map[string]bool
		seenCount := 0
		for _, elem := range elems {
			k := CanonicalValueKey(elem)
			duplicate := false
			if seenCount < len(seen) {
				for i := 0; i < seenCount; i++ {
					if seen[i] == k {
						duplicate = true
						break
					}
				}
				if !duplicate {
					seen[seenCount] = k
					seenCount++
				}
			} else {
				if seenHeap == nil {
					seenHeap = make(map[string]bool, len(elems))
					for i := 0; i < seenCount; i++ {
						seenHeap[seen[i]] = true
					}
				}
				if seenHeap[k] {
					duplicate = true
				} else {
					seenHeap[k] = true
				}
			}
			if !duplicate {
				asn.mu.RLock()
				succs := asn.cases[k]
				asn.mu.RUnlock()
				for _, s := range succs {
					s.Activation(wme, tag)
				}
			}
		}
		return
	}

	// Non-vector value
	if asn.VectorIndex > 0 {
		return
	}
	k := CanonicalValueKey(val)
	asn.mu.RLock()
	succs := asn.cases[k]
	asn.mu.RUnlock()
	for _, s := range succs {
		s.Activation(wme, tag)
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
	for _, s := range tn.successors {
		if s == succ {
			return
		}
	}
	tn.successors = append(tn.successors, succ)
}

// Successors returns all downstream AlphaNodes.
func (tn *TypeNode) Successors() []AlphaNode {
	return tn.successors
}

// GetOrCreateSwitchNode retrieves an existing child switch node or registers a new one.
func (tn *TypeNode) GetOrCreateSwitchNode(attr string, vecIdx int) *AlphaSwitchNode {
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range tn.successors {
		if sw, ok := s.(*AlphaSwitchNode); ok {
			if sw.Attribute == normAttr && sw.VectorIndex == vecIdx {
				return sw
			}
		}
	}
	sw := NewIndexedAlphaSwitchNode(normAttr, vecIdx)
	tn.AddSuccessor(sw)
	return sw
}

// GetOrCreateConstantTestNode retrieves an existing child constant test node or registers a new one.
func (tn *TypeNode) GetOrCreateConstantTestNode(attr string, op model.Operator, val model.Value, vecIdx int) *ConstantTestNode {
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range tn.successors {
		if ct, ok := s.(*ConstantTestNode); ok {
			if ct.Attribute == normAttr && ct.Op == op && ct.Value.Equal(val) && ct.VectorIndex == vecIdx && len(ct.Disjunction) == 0 {
				return ct
			}
		}
	}
	ct := NewIndexedConstantTestNode(normAttr, op, val, vecIdx)
	tn.AddSuccessor(ct)
	return ct
}

// GetOrCreateDisjunctiveTestNode retrieves an existing child disjunctive test node or registers a new one.
func (tn *TypeNode) GetOrCreateDisjunctiveTestNode(attr string, disj []model.TestConstraint, vecIdx int) *ConstantTestNode {
	normAttr := model.NormalizeAttribute(attr)
	for _, s := range tn.successors {
		if ct, ok := s.(*ConstantTestNode); ok {
			if ct.Attribute == normAttr && ct.VectorIndex == vecIdx && testConstraintsEqual(ct.Disjunction, disj) {
				return ct
			}
		}
	}
	ct := NewIndexedDisjunctiveConstantTestNode(normAttr, disj, vecIdx)
	tn.AddSuccessor(ct)
	return ct
}

func testConstraintsEqual(a, b []model.TestConstraint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Op != b[i].Op || !a[i].Value.Equal(b[i].Value) {
			return false
		}
	}
	return true
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
