package model_test

import (
	"fmt"

	"github.com/graemenewlands/ops5/pkg/model"
)

func ExampleNewRule() {
	rule := model.NewRule("apply-discount")
	rule.SetSalience(10)
	rule.SetDocstring("Applies a discount to pending orders")

	// LHS: positive condition matching customer tier and pending order
	ce := model.NewPositiveCE("order").
		AddEqualTest("status", model.NewSymbol("pending")).
		AddEqualTest("total", model.NewVariable("<t>"))
	rule.AddCondition(ce)

	// RHS: modify status of the first condition element
	rule.AddAction(&model.ModifyAction{
		TargetIndex: 1,
		Attributes: map[string]model.Value{
			"status": model.NewSymbol("discounted"),
		},
	})

	fmt.Printf("Rule %s has salience %d, specificity %d, %d condition(s)\n",
		rule.Name, rule.Salience, rule.Specificity(), len(rule.Conditions))

	// Output:
	// Rule apply-discount has salience 10, specificity 3, 1 condition(s)
}

func ExampleValue() {
	vInt := model.NewInt(42)
	vSym := model.NewSymbol("active")
	vStr := model.NewString("OPS5 Production System")

	fmt.Printf("Int: %s (%s), Symbol: %s (%s), String: %s (%s)\n",
		vInt, vInt.Type(), vSym, vSym.Type(), vStr, vStr.Type())

	// Output:
	// Int: 42 (integer), Symbol: active (symbol), String: "OPS5 Production System" (string)
}
