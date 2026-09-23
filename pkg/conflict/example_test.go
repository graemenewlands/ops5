package conflict_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

func ExampleSet() {
	cs := conflict.NewSet()
	cs.SetStrategy(conflict.StrategyLEX)

	// Create rules: normal rule vs urgent rule with salience
	normalRule := model.NewRule("normal-task").SetSalience(0)
	urgentRule := model.NewRule("urgent-alert").SetSalience(100)

	// Create dummy facts and tokens
	wme1 := model.NewWME(1, "event", nil)
	wme2 := model.NewWME(2, "event", nil)

	token1 := rete.NewToken(rete.DummyRootToken(), wme1, nil)
	token2 := rete.NewToken(rete.DummyRootToken(), wme2, nil)

	// Add activations
	cs.OnActivationAdd(normalRule, token2) // timetag 2, salience 0
	cs.OnActivationAdd(urgentRule, token1) // timetag 1, salience 100

	// Dominant selection: urgentRule wins due to higher salience despite lower timetag
	dom, ok := cs.SelectDominant()
	if ok {
		fmt.Printf("Dominant rule: %s (salience %d)\n", dom.Rule.Name, dom.Salience())
	}

	// Output:
	// Dominant rule: urgent-alert (salience 100)
}
