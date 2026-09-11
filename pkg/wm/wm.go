package wm

import (
	"fmt"
	"sort"
	"sync"

	"ops5/pkg/model"
)

// Listener is notified whenever a WME is asserted or retracted.
type Listener interface {
	OnAssert(wme *model.WME)
	OnRetract(wme *model.WME)
}

// WorkingMemory manages the active set of working memory elements (WMEs)
// and handles timetag generation and dynamic modifications.
type WorkingMemory struct {
	mu          sync.RWMutex
	nextTimetag int64
	wmes        map[int64]*model.WME
	listeners   []Listener
}

// New creates a new WorkingMemory instance.
func New() *WorkingMemory {
	return &WorkingMemory{
		nextTimetag: 1,
		wmes:        make(map[int64]*model.WME),
		listeners:   make([]Listener, 0),
	}
}

// AddListener registers a listener for WME assertion and retraction events.
func (wm *WorkingMemory) AddListener(l Listener) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.listeners = append(wm.listeners, l)
}

// Make creates and asserts a new WME into working memory.
func (wm *WorkingMemory) Make(class string, attrs map[string]model.Value) *model.WME {
	wm.mu.Lock()
	timetag := wm.nextTimetag
	wm.nextTimetag++

	wme := model.NewWME(timetag, class, attrs)
	wm.wmes[timetag] = wme
	listeners := append([]Listener(nil), wm.listeners...)
	wm.mu.Unlock()

	for _, l := range listeners {
		l.OnAssert(wme)
	}

	return wme
}

// Remove retracts a WME by its timetag.
func (wm *WorkingMemory) Remove(timetag int64) (*model.WME, error) {
	wm.mu.Lock()
	wme, exists := wm.wmes[timetag]
	if !exists {
		wm.mu.Unlock()
		return nil, fmt.Errorf("WME with timetag %d not found", timetag)
	}
	delete(wm.wmes, timetag)
	listeners := append([]Listener(nil), wm.listeners...)
	wm.mu.Unlock()

	for _, l := range listeners {
		l.OnRetract(wme)
	}

	return wme, nil
}

// Modify updates attributes of an existing WME.
// In OPS5 semantics:
// 1. The old WME is retracted.
// 2. A new WME is asserted with a new timetag, preserving unmodified attributes.
func (wm *WorkingMemory) Modify(timetag int64, updatedAttrs map[string]model.Value) (*model.WME, error) {
	wm.mu.Lock()
	oldWme, exists := wm.wmes[timetag]
	if !exists {
		wm.mu.Unlock()
		return nil, fmt.Errorf("WME with timetag %d not found for modify", timetag)
	}

	// Remove old WME
	delete(wm.wmes, timetag)

	// Build merged attribute set
	newAttrs := make(map[string]model.Value, len(oldWme.Attributes)+len(updatedAttrs))
	for k, v := range oldWme.Attributes {
		newAttrs[k] = v
	}
	for k, v := range updatedAttrs {
		newAttrs[model.NormalizeAttribute(k)] = v
	}

	newTimetag := wm.nextTimetag
	wm.nextTimetag++
	newWme := model.NewWME(newTimetag, oldWme.Class, newAttrs)
	wm.wmes[newTimetag] = newWme

	listeners := append([]Listener(nil), wm.listeners...)
	wm.mu.Unlock()

	// Notify listeners: retraction then assertion
	for _, l := range listeners {
		l.OnRetract(oldWme)
	}
	for _, l := range listeners {
		l.OnAssert(newWme)
	}

	return newWme, nil
}

// Get retrieves a WME by timetag.
func (wm *WorkingMemory) Get(timetag int64) (*model.WME, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	wme, ok := wm.wmes[timetag]
	return wme, ok
}

// Count returns the number of active WMEs.
func (wm *WorkingMemory) Count() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.wmes)
}

// All returns all active WMEs ordered by timetag.
func (wm *WorkingMemory) All() []*model.WME {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	res := make([]*model.WME, 0, len(wm.wmes))
	for _, w := range wm.wmes {
		res = append(res, w)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timetag < res[j].Timetag
	})
	return res
}

// FindByClass returns all WMEs matching the specified class name.
func (wm *WorkingMemory) FindByClass(class string) []*model.WME {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	var res []*model.WME
	for _, w := range wm.wmes {
		if w.Class == class {
			res = append(res, w)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timetag < res[j].Timetag
	})
	return res
}

// RemoveAll removes all WMEs from working memory, notifying listeners of retractions,
// while preserving the monotonically increasing nextTimetag counter.
// Returns the slice of removed WMEs sorted by timetag.
func (wm *WorkingMemory) RemoveAll() []*model.WME {
	wm.mu.Lock()
	oldWmes := make([]*model.WME, 0, len(wm.wmes))
	for _, w := range wm.wmes {
		oldWmes = append(oldWmes, w)
	}
	sort.Slice(oldWmes, func(i, j int) bool {
		return oldWmes[i].Timetag < oldWmes[j].Timetag
	})
	wm.wmes = make(map[int64]*model.WME)
	listeners := append([]Listener(nil), wm.listeners...)
	wm.mu.Unlock()

	for _, w := range oldWmes {
		for _, l := range listeners {
			l.OnRetract(w)
		}
	}
	return oldWmes
}

// Reset clears all WMEs from working memory and optionally notifies listeners of retraction.
func (wm *WorkingMemory) Reset() {
	wm.mu.Lock()
	oldWmes := make([]*model.WME, 0, len(wm.wmes))
	for _, w := range wm.wmes {
		oldWmes = append(oldWmes, w)
	}
	wm.wmes = make(map[int64]*model.WME)
	wm.nextTimetag = 1
	listeners := append([]Listener(nil), wm.listeners...)
	wm.mu.Unlock()

	for _, w := range oldWmes {
		for _, l := range listeners {
			l.OnRetract(w)
		}
	}
}
