package rete

import (
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
)

// EvalNode is a single-input beta node that filters incoming tokens
// by evaluating expressions/predicates against the token's variable bindings.
type EvalNode struct {
	mu         sync.RWMutex
	test       *model.EvalTest
	predicate  func(bindings map[string]model.Value) bool
	successors []LeftActivatable
}

// NewEvalNode creates a new EvalNode with the specified model.EvalTest.
func NewEvalNode(test *model.EvalTest) *EvalNode {
	return &EvalNode{
		test:       test,
		successors: make([]LeftActivatable, 0),
	}
}

// NewEvalPredicateNode creates a new EvalNode with a custom Go predicate function.
func NewEvalPredicateNode(fn func(bindings map[string]model.Value) bool) *EvalNode {
	return &EvalNode{
		predicate:  fn,
		successors: make([]LeftActivatable, 0),
	}
}

// Test returns the underlying EvalTest.
func (en *EvalNode) Test() *model.EvalTest {
	return en.test
}

// AddSuccessor registers a downstream beta node.
func (en *EvalNode) AddSuccessor(node LeftActivatable) {
	en.mu.Lock()
	defer en.mu.Unlock()
	en.successors = append(en.successors, node)
}

// RemoveSuccessor unregisters a downstream beta node.
func (en *EvalNode) RemoveSuccessor(node LeftActivatable) {
	en.mu.Lock()
	defer en.mu.Unlock()
	var newSuccs []LeftActivatable
	for _, s := range en.successors {
		if s != node {
			newSuccs = append(newSuccs, s)
		}
	}
	en.successors = newSuccs
}

// LeftActivation handles an incoming token from the parent BetaMemory.
func (en *EvalNode) LeftActivation(token *Token, tag PropagationTag) {
	if en.predicate != nil {
		if !en.predicate(token.Bindings) {
			return
		}
	}

	if en.test != nil {
		ok, err := en.test.Evaluate(token.Bindings)
		if err != nil || !ok {
			return
		}
	}

	en.mu.RLock()
	succs := append([]LeftActivatable(nil), en.successors...)
	en.mu.RUnlock()

	for _, s := range succs {
		s.LeftActivation(token, tag)
	}
}
