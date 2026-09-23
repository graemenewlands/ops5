package wm_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/wm"
)

// traceListener logs WME lifecycle events.
type traceListener struct{}

func (t *traceListener) OnAssert(wme *model.WME) {
	fmt.Printf("+Assert timetag=%d class=%s\n", wme.Timetag, wme.Class)
}

func (t *traceListener) OnRetract(wme *model.WME) {
	fmt.Printf("-Retract timetag=%d class=%s\n", wme.Timetag, wme.Class)
}

func Example() {
	memory := wm.New()
	memory.AddListener(&traceListener{})

	// 1. Assert initial WME (timetag 1)
	wme1 := memory.Make("inventory", map[string]model.Value{
		"item":  model.NewSymbol("widgets"),
		"count": model.NewInt(100),
	})

	// 2. Modify WME (in OPS5: retracts old WME, asserts new WME with updated timetag 2)
	wme2, _ := memory.Modify(wme1.Timetag, map[string]model.Value{
		"count": model.NewInt(95),
	})

	// 3. Retract WME
	_, _ = memory.Remove(wme2.Timetag)

	// Output:
	// +Assert timetag=1 class=inventory
	// -Retract timetag=1 class=inventory
	// +Assert timetag=2 class=inventory
	// -Retract timetag=2 class=inventory
}
