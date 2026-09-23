package rete

import (
	"testing"

	"github.com/graemenewlands/ops5/pkg/model"
)

func TestTokenWMEAt(t *testing.T) {
	wme1 := model.NewWME(10, "goal", map[string]model.Value{"status": model.NewSymbol("active")})
	wme2 := model.NewWME(20, "context", map[string]model.Value{"val": model.NewInt(42)})
	wme3 := model.NewWME(30, "step", map[string]model.Value{"num": model.NewInt(1)})

	// Chain: root -> t1 (wme1) -> t2 (nil WME, e.g. eval/negative) -> t3 (wme2) -> t4 (wme3)
	t0 := NewToken(nil, nil, nil)
	t1 := NewToken(t0, wme1, nil)
	t2 := NewToken(t1, nil, nil) // intermediate token without WME
	t3 := NewToken(t2, wme2, nil)
	t4 := NewToken(t3, wme3, nil)

	wmes := t4.WMEs()
	if len(wmes) != 3 {
		t.Fatalf("expected 3 non-nil WMEs, got %d", len(wmes))
	}

	// Test WMEAt for 1, 2, 3
	wAt1, ok1 := t4.WMEAt(1)
	if !ok1 || wAt1 != wme1 {
		t.Errorf("WMEAt(1) = %v, %v; want %v, true", wAt1, ok1, wme1)
	}

	wAt2, ok2 := t4.WMEAt(2)
	if !ok2 || wAt2 != wme2 {
		t.Errorf("WMEAt(2) = %v, %v; want %v, true", wAt2, ok2, wme2)
	}

	wAt3, ok3 := t4.WMEAt(3)
	if !ok3 || wAt3 != wme3 {
		t.Errorf("WMEAt(3) = %v, %v; want %v, true", wAt3, ok3, wme3)
	}

	// Test out of bounds
	if _, ok := t4.WMEAt(0); ok {
		t.Errorf("WMEAt(0) expected false")
	}
	if _, ok := t4.WMEAt(4); ok {
		t.Errorf("WMEAt(4) expected false")
	}
	if _, ok := t4.WMEAt(-1); ok {
		t.Errorf("WMEAt(-1) expected false")
	}

	// Nil token test
	var nilTok *Token
	if _, ok := nilTok.WMEAt(1); ok {
		t.Errorf("nilTok.WMEAt(1) expected false")
	}
}
