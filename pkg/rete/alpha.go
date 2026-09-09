package rete

import (
	"sync"

	"ops5/pkg/model"
)

// AlphaNode represents any node in the alpha network capable of processing a WME.
type AlphaNode interface {
	Activation(wme *model.WME, tag PropagationTag)
}

// RightActivatable represents a beta node that receives right activations from an AlphaMemory.
type RightActivatable interface {
	RightActivation(wme *model.WME, tag PropagationTag)
}

// AlphaMemory stores WMEs that satisfy all intra-element condition tests for a pattern.
type AlphaMemory struct {
	mu         sync.RWMutex
	items      map[int64]*model.WME
	successors []RightActivatable
}

// NewAlphaMemory creates a new AlphaMemory.
func NewAlphaMemory() *AlphaMemory {
	return &AlphaMemory{
		items:      make(map[int64]*model.WME),
		successors: make([]RightActivatable, 0),
	}
}

// AddSuccessor registers a beta node to receive right activations.
func (am *AlphaMemory) AddSuccessor(node RightActivatable) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.successors = append(am.successors, node)
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

// Activation processes an incoming WME assertion or retraction.
func (am *AlphaMemory) Activation(wme *model.WME, tag PropagationTag) {
	am.mu.Lock()
	if tag == TagAdd {
		am.items[wme.Timetag] = wme
	} else {
		delete(am.items, wme.Timetag)
	}
	succs := append([]RightActivatable(nil), am.successors...)
	am.mu.Unlock()

	for _, s := range succs {
		s.RightActivation(wme, tag)
	}
}

// ConstantTestNode evaluates a constant constraint on a single attribute.
type ConstantTestNode struct {
	Attribute  string
	Op         model.Operator
	Value      model.Value
	successors []AlphaNode
}

// NewConstantTestNode creates a new test node.
func NewConstantTestNode(attr string, op model.Operator, val model.Value) *ConstantTestNode {
	return &ConstantTestNode{
		Attribute:  model.NormalizeAttribute(attr),
		Op:         op,
		Value:      val,
		successors: make([]AlphaNode, 0),
	}
}

// AddSuccessor adds a child alpha node or alpha memory.
func (ct *ConstantTestNode) AddSuccessor(succ AlphaNode) {
	ct.successors = append(ct.successors, succ)
}

// Test evaluates the constraint against the WME.
func (ct *ConstantTestNode) Test(wme *model.WME) bool {
	val, ok := wme.Get(ct.Attribute)
	if !ok {
		// If attribute is missing, it cannot satisfy equality or ordering tests with non-nil
		if ct.Op == model.OpNotEqual {
			return true
		}
		return false
	}

	switch ct.Op {
	case model.OpEqual:
		return val.Equal(ct.Value)
	case model.OpNotEqual:
		return !val.Equal(ct.Value)
	case model.OpLess:
		cmp, err := val.Compare(ct.Value)
		return err == nil && cmp < 0
	case model.OpLessEqual:
		cmp, err := val.Compare(ct.Value)
		return err == nil && cmp <= 0
	case model.OpGreater:
		cmp, err := val.Compare(ct.Value)
		return err == nil && cmp > 0
	case model.OpGreaterEqual:
		cmp, err := val.Compare(ct.Value)
		return err == nil && cmp >= 0
	default:
		return false
	}
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
