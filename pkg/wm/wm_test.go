package wm

import (
	"testing"

	"ops5/pkg/model"
)

type mockListener struct {
	asserted  []*model.WME
	retracted []*model.WME
}

func (m *mockListener) OnAssert(wme *model.WME) {
	m.asserted = append(m.asserted, wme)
}

func (m *mockListener) OnRetract(wme *model.WME) {
	m.retracted = append(m.retracted, wme)
}

func TestWorkingMemoryOperations(t *testing.T) {
	wm := New()
	listener := &mockListener{}
	wm.AddListener(listener)

	// Test Make
	wme1 := wm.Make("goal", map[string]model.Value{
		"status":   model.NewSymbol("active"),
		"priority": model.NewInt(10),
	})

	if wme1.Timetag != 1 {
		t.Fatalf("expected timetag 1, got %d", wme1.Timetag)
	}
	if wm.Count() != 1 {
		t.Fatalf("expected count 1, got %d", wm.Count())
	}
	if len(listener.asserted) != 1 || listener.asserted[0].Timetag != 1 {
		t.Fatalf("listener did not record assertion of wme1")
	}

	wme2 := wm.Make("goal", map[string]model.Value{
		"status":   model.NewSymbol("pending"),
		"priority": model.NewInt(5),
	})

	if wme2.Timetag != 2 {
		t.Fatalf("expected timetag 2, got %d", wme2.Timetag)
	}
	if wm.Count() != 2 {
		t.Fatalf("expected count 2, got %d", wm.Count())
	}

	// Test Get and FindByClass
	fetched, ok := wm.Get(1)
	if !ok || fetched.Timetag != 1 {
		t.Fatalf("failed to fetch wme1")
	}

	goals := wm.FindByClass("goal")
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(goals))
	}

	// Test Modify (timetag 1 -> status: completed)
	wme3, err := wm.Modify(1, map[string]model.Value{
		"status": model.NewSymbol("completed"),
	})
	if err != nil {
		t.Fatalf("modify failed: %v", err)
	}
	if wme3.Timetag != 3 {
		t.Fatalf("expected new timetag 3, got %d", wme3.Timetag)
	}
	// Check merged attributes: priority should still be 10!
	prio, ok := wme3.Get("priority")
	if !ok || !prio.Equal(model.NewInt(10)) {
		t.Fatalf("expected preserved priority 10, got %v", prio)
	}
	status, _ := wme3.Get("status")
	if !status.Equal(model.NewSymbol("completed")) {
		t.Fatalf("expected updated status 'completed', got %v", status)
	}

	// Timetag 1 should no longer exist in wm
	if _, exists := wm.Get(1); exists {
		t.Fatalf("old WME with timetag 1 should have been retracted")
	}
	// Listener should have 1 retraction (timetag 1) and 3 assertions (1, 2, 3)
	if len(listener.retracted) != 1 || listener.retracted[0].Timetag != 1 {
		t.Fatalf("expected 1 retraction of timetag 1, got %v", listener.retracted)
	}
	if len(listener.asserted) != 3 {
		t.Fatalf("expected 3 assertions, got %d", len(listener.asserted))
	}

	// Test Remove
	removed, err := wm.Remove(2)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if removed.Timetag != 2 {
		t.Fatalf("expected removed timetag 2, got %d", removed.Timetag)
	}
	if wm.Count() != 1 {
		t.Fatalf("expected count 1, got %d", wm.Count())
	}
	if len(listener.retracted) != 2 {
		t.Fatalf("expected 2 retractions, got %d", len(listener.retracted))
	}

	// Test Remove non-existent
	if _, err := wm.Remove(999); err == nil {
		t.Fatalf("expected error when removing non-existent timetag")
	}

	// Test RemoveAll
	allRemoved := wm.RemoveAll()
	if len(allRemoved) != 1 || allRemoved[0].Timetag != 3 {
		t.Fatalf("expected timetag 3 removed by RemoveAll, got %v", allRemoved)
	}
	if wm.Count() != 0 {
		t.Fatalf("expected count 0 after RemoveAll, got %d", wm.Count())
	}
	// Assert new WME, nextTimetag should still be 4 (not reset to 1)
	newWME := wm.Make("goal", nil)
	if newWME.Timetag != 4 {
		t.Fatalf("expected next timetag 4, got %d", newWME.Timetag)
	}
}

