package rete_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/rete"
)

// activationRecorder implements rete.ConflictSetListener.
type activationRecorder struct{}

func (ar *activationRecorder) OnActivationAdd(rule *model.Rule, token *rete.Token) {
	fmt.Printf("+Activation: rule=%s, timetags=%v\n", rule.Name, token.Timetags())
}

func (ar *activationRecorder) OnActivationRemove(rule *model.Rule, token *rete.Token) {
	fmt.Printf("-Activation: rule=%s, timetags=%v\n", rule.Name, token.Timetags())
}

func ExampleNetwork() {
	net := rete.NewNetwork()
	listener := &activationRecorder{}

	// Construct rule matching active sensors with temp > 100
	rule := model.NewRule("sensor-alert")
	ce := model.NewPositiveCE("sensor").
		AddEqualTest("status", model.NewSymbol("active")).
		AddTest("temp", model.OpGreater, model.NewInt(100))
	rule.AddCondition(ce)

	net.AddRule(rule, listener)

	// Assert WME that does NOT match (temp 90)
	wme1 := model.NewWME(1, "sensor", map[string]model.Value{
		"status": model.NewSymbol("active"),
		"temp":   model.NewInt(90),
	})
	net.OnAssert(wme1)

	// Assert WME that matches (temp 110) -> produces activation
	wme2 := model.NewWME(2, "sensor", map[string]model.Value{
		"status": model.NewSymbol("active"),
		"temp":   model.NewInt(110),
	})
	net.OnAssert(wme2)

	// Retract matching WME -> removes activation
	net.OnRetract(wme2)

	// Output:
	// +Activation: rule=sensor-alert, timetags=[2]
	// -Activation: rule=sensor-alert, timetags=[2]
}
