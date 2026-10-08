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

func ExampleValue_temporal() {
	d := model.NewDate(20260929)
	dt := model.NewDateTime("2026-09-29T14:30:00")
	utc := model.NewDateUTCDirect("2026-09-29T21:30:00Z")

	fmt.Printf("Date: %s (%s)\n", d, d.Type())
	fmt.Printf("DateTime: %s (%s)\n", dt, dt.Type())
	fmt.Printf("DateUTCTime: %s (%s)\n", utc, utc.Type())

	// Output:
	// Date: 20260929 (date)
	// DateTime: 2026-09-29T14:30:00 (datetime)
	// DateUTCTime: 2026-09-29T21:30:00Z (dateutctime)
}

func ExampleValue_temporalArithmetic() {
	d := model.NewDate(20260131)
	// MonthAdd applies end-of-month clamping (Jan 31 + 1 month -> Feb 28)
	nextMonth, _ := d.MonthAdd(1)
	nextWeek, _ := d.DayAdd(7)

	t1 := model.NewDateTime("2026-09-29T10:00:00")
	t2 := model.NewDateTime("2026-10-01T14:30:00")
	diffSec, _ := t2.DateDiff(t1) // 189000 seconds
	diffDays, _ := diffSec.Days()
	diffHours, _ := diffSec.Hours()

	fmt.Printf("Next Month: %s\n", nextMonth)
	fmt.Printf("Next Week: %s\n", nextWeek)
	fmt.Printf("Diff Days: %s, Diff Hours: %s\n", diffDays, diffHours)

	// Output:
	// Next Month: 20260228
	// Next Week: 20260207
	// Diff Days: 2, Diff Hours: 52
}

type OrderExample struct {
	Timetag  int64    `ops5:",timetag"`
	ID       int64    `ops5:"order_id"`
	Customer string   `ops5:"customer"`
	Status   string   `ops5:"status,symbol"`
	Total    float64  `ops5:"total"`
	Tags     []string `ops5:"tags,vector"`
}

func (OrderExample) OPS5ClassName() string {
	return "order"
}

func ExampleClassSchemaFromStruct() {
	schema, err := model.ClassSchemaFromStruct(OrderExample{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Class: %s\n", schema.Class)
	fmt.Printf("Attributes: %v\n", schema.Attributes)
	fmt.Printf("Is Vector: %v\n", schema.IsVectorAttribute("tags"))
	fmt.Printf("Has Fingerprint: %v\n", schema.Fingerprint != "")

	// Output:
	// Class: order
	// Attributes: [order_id customer status total tags]
	// Is Vector: true
	// Has Fingerprint: true
}

func ExampleMarshalWME() {
	order := OrderExample{
		ID:       101,
		Customer: "Acme Corp",
		Status:   "pending",
		Total:    250.00,
		Tags:     []string{"priority", "b2b"},
	}

	className, attrs, err := model.MarshalWME(order)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Class: %s\n", className)
	fmt.Printf("Order ID: %s\n", attrs["order_id"])
	fmt.Printf("Status: %s (%s)\n", attrs["status"], attrs["status"].Type())
	fmt.Printf("Tags: %s (%s)\n", attrs["tags"], attrs["tags"].Type())

	// Output:
	// Class: order
	// Order ID: 101
	// Status: pending (symbol)
	// Tags: "priority" "b2b" (vector)
}

func ExampleUnmarshalWME() {
	wme := model.NewWME(42, "order", map[string]model.Value{
		"order_id": model.NewInt(101),
		"customer": model.NewString("Acme Corp"),
		"status":   model.NewSymbol("shipped"),
		"total":    model.NewFloat(250.00),
		"tags":     model.NewVector([]model.Value{model.NewString("priority"), model.NewString("b2b")}),
	})

	var order OrderExample
	if err := wme.Unmarshal(&order); err != nil {
		panic(err)
	}

	fmt.Printf("Timetag: %d\n", order.Timetag)
	fmt.Printf("Order ID: %d\n", order.ID)
	fmt.Printf("Customer: %s\n", order.Customer)
	fmt.Printf("Status: %s\n", order.Status)
	fmt.Printf("Tags: %v\n", order.Tags)

	// Output:
	// Timetag: 42
	// Order ID: 101
	// Customer: Acme Corp
	// Status: shipped
	// Tags: [priority b2b]
}


