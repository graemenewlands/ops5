package conflict

import (
	"sort"
	"strings"
	"sync"

	"ops5/pkg/model"
	"ops5/pkg/rete"
)

// Set manages the active conflict set (agenda) of candidate rule instantiations.
type Set struct {
	mu          sync.RWMutex
	strategy    StrategyType
	activations map[string]*Activation
	refracted   map[string]bool
}

// NewSet creates a new conflict set with default LEX strategy.
func NewSet() *Set {
	return &Set{
		strategy:    StrategyLEX,
		activations: make(map[string]*Activation),
		refracted:   make(map[string]bool),
	}
}

// SetStrategy updates the conflict resolution strategy (LEX or MEA).
func (cs *Set) SetStrategy(strat StrategyType) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.strategy = strat
}

// Strategy returns current strategy.
func (cs *Set) Strategy() StrategyType {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.strategy
}

// OnActivationAdd implements rete.ConflictSetListener.
func (cs *Set) OnActivationAdd(rule *model.Rule, token *rete.Token) {
	act := NewActivation(rule, token)
	key := act.Key()

	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Refraction: an instantiation that already fired will not be added to conflict set
	if cs.refracted[key] {
		return
	}

	cs.activations[key] = act
}

// OnActivationRemove implements rete.ConflictSetListener.
func (cs *Set) OnActivationRemove(rule *model.Rule, token *rete.Token) {
	act := NewActivation(rule, token)
	key := act.Key()

	cs.mu.Lock()
	defer cs.mu.Unlock()

	delete(cs.activations, key)
}

// Count returns the number of pending activations in the conflict set.
func (cs *Set) Count() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.activations)
}

// SelectDominant selects the winning activation according to the current conflict resolution strategy.
func (cs *Set) SelectDominant() (*Activation, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.activations) == 0 {
		return nil, false
	}

	var dominant *Activation
	compareFn := LexCompare
	if cs.strategy == StrategyMEA {
		compareFn = MeaCompare
	}

	for _, act := range cs.activations {
		if dominant == nil {
			dominant = act
			continue
		}

		if compareFn(act, dominant) > 0 {
			dominant = act
		}
	}

	return dominant, dominant != nil
}

// MarkFired marks an activation as fired (refracted) so it cannot fire again for the exact same WMEs.
func (cs *Set) MarkFired(act *Activation) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	key := act.Key()
	cs.refracted[key] = true
	delete(cs.activations, key)
}

// All returns all pending activations sorted by decreasing salience (dominant first).
func (cs *Set) All() []*Activation {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	list := make([]*Activation, 0, len(cs.activations))
	for _, act := range cs.activations {
		list = append(list, act)
	}

	compareFn := LexCompare
	if cs.strategy == StrategyMEA {
		compareFn = MeaCompare
	}

	sort.Slice(list, func(i, j int) bool {
		return compareFn(list[i], list[j]) > 0
	})

	return list
}

// Reset clears all activations and refracted history.
func (cs *Set) Reset() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.activations = make(map[string]*Activation)
	cs.refracted = make(map[string]bool)
}

// RemoveRule evicts all activations and refracted entries associated with the specified rule name.
// Returns the number of pending activations that were removed.
func (cs *Set) RemoveRule(ruleName string) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	count := 0
	for key, act := range cs.activations {
		if act.Rule.Name == ruleName {
			delete(cs.activations, key)
			count++
		}
	}

	for key := range cs.refracted {
		if strings.HasPrefix(key, ruleName+":") {
			delete(cs.refracted, key)
		}
	}

	return count
}

