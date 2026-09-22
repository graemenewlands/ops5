package conflict

import (
	"container/heap"
	"sort"
	"strings"
	"sync"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

type activationHeap struct {
	items     []*Activation
	compareFn func(a, b *Activation) int
}

func (h *activationHeap) Len() int {
	return len(h.items)
}

func (h *activationHeap) Less(i, j int) bool {
	return h.compareFn(h.items[i], h.items[j]) > 0
}

func (h *activationHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
	h.items[i].heapIndex = i
	h.items[j].heapIndex = j
}

func (h *activationHeap) Push(x any) {
	n := len(h.items)
	item := x.(*Activation)
	item.heapIndex = n
	h.items = append(h.items, item)
}

func (h *activationHeap) Pop() any {
	old := h.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.heapIndex = -1
	h.items = old[0 : n-1]
	return item
}

// Set manages the active conflict set (agenda) of candidate rule instantiations.
type Set struct {
	mu          sync.RWMutex
	strategy    StrategyType
	activations map[string]*Activation
	refracted   map[string]bool
	agenda      activationHeap
}

// NewSet creates a new conflict set with default LEX strategy.
func NewSet() *Set {
	cs := &Set{
		strategy:    StrategyLEX,
		activations: make(map[string]*Activation),
		refracted:   make(map[string]bool),
	}
	cs.agenda.compareFn = LexCompare
	return cs
}

func (cs *Set) getCompareFn() func(a, b *Activation) int {
	if cs.strategy == StrategyMEA {
		return MeaCompare
	}
	return LexCompare
}

// SetStrategy updates the conflict resolution strategy (LEX or MEA).
func (cs *Set) SetStrategy(strat StrategyType) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.strategy == strat {
		return
	}
	cs.strategy = strat
	if strat == StrategyMEA {
		cs.agenda.compareFn = MeaCompare
	} else {
		cs.agenda.compareFn = LexCompare
	}
	heap.Init(&cs.agenda)
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

	// Already present in conflict set: no-op (identical rule & timetags)
	if _, exists := cs.activations[key]; exists {
		return
	}

	if cs.agenda.compareFn == nil {
		cs.agenda.compareFn = cs.getCompareFn()
	}

	cs.activations[key] = act
	heap.Push(&cs.agenda, act)
}

// OnActivationRemove implements rete.ConflictSetListener.
func (cs *Set) OnActivationRemove(rule *model.Rule, token *rete.Token) {
	act := NewActivation(rule, token)
	key := act.Key()

	cs.mu.Lock()
	defer cs.mu.Unlock()

	existing, exists := cs.activations[key]
	if !exists {
		return
	}

	delete(cs.activations, key)
	if existing.heapIndex >= 0 && existing.heapIndex < len(cs.agenda.items) {
		heap.Remove(&cs.agenda, existing.heapIndex)
	}
}

// Count returns the number of pending activations in the conflict set.
func (cs *Set) Count() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.activations)
}

// SelectDominant selects the winning activation according to the current conflict resolution strategy.
// With the binary max-heap agenda, this is an O(1) operation.
func (cs *Set) SelectDominant() (*Activation, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.agenda.items) == 0 {
		return nil, false
	}

	return cs.agenda.items[0], true
}

// MarkFired marks an activation as fired (refracted) so it cannot fire again for the exact same WMEs.
func (cs *Set) MarkFired(act *Activation) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	key := act.Key()
	cs.refracted[key] = true

	existing, exists := cs.activations[key]
	if exists {
		delete(cs.activations, key)
		if existing.heapIndex >= 0 && existing.heapIndex < len(cs.agenda.items) {
			heap.Remove(&cs.agenda, existing.heapIndex)
		}
	} else if act.heapIndex >= 0 && act.heapIndex < len(cs.agenda.items) {
		heap.Remove(&cs.agenda, act.heapIndex)
	}
}

// All returns all pending activations sorted by decreasing salience (dominant first).
func (cs *Set) All() []*Activation {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	list := make([]*Activation, len(cs.agenda.items))
	copy(list, cs.agenda.items)

	compareFn := cs.agenda.compareFn
	if compareFn == nil {
		compareFn = cs.getCompareFn()
	}

	sort.Slice(list, func(i, j int) bool {
		return compareFn(list[i], list[j]) > 0
	})

	return list
}

// RuleActivations returns all pending activations for a specific rule name, sorted by decreasing salience.
func (cs *Set) RuleActivations(ruleName string) []*Activation {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var list []*Activation
	for _, act := range cs.activations {
		if act.Rule != nil && act.Rule.Name == ruleName {
			list = append(list, act)
		}
	}

	compareFn := cs.agenda.compareFn
	if compareFn == nil {
		compareFn = cs.getCompareFn()
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

	for _, act := range cs.agenda.items {
		act.heapIndex = -1
	}
	cs.activations = make(map[string]*Activation)
	cs.refracted = make(map[string]bool)
	cs.agenda.items = nil
}

// RemoveRule evicts all activations and refracted entries associated with the specified rule name.
// Returns the number of pending activations that were removed.
func (cs *Set) RemoveRule(ruleName string) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	count := 0
	for key, act := range cs.activations {
		if act.Rule != nil && act.Rule.Name == ruleName {
			delete(cs.activations, key)
			count++
		}
	}

	for key := range cs.refracted {
		if strings.HasPrefix(key, ruleName+":") {
			delete(cs.refracted, key)
		}
	}

	if count > 0 {
		newItems := make([]*Activation, 0, len(cs.activations))
		for _, act := range cs.activations {
			act.heapIndex = len(newItems)
			newItems = append(newItems, act)
		}
		cs.agenda.items = newItems
		if cs.agenda.compareFn == nil {
			cs.agenda.compareFn = cs.getCompareFn()
		}
		heap.Init(&cs.agenda)
	}

	return count
}
