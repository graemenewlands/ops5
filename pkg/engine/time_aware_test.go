package engine

import (
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
)

func TestTimeAwareEngine_SystemWMEs(t *testing.T) {
	tae := NewTimeAwareEngine()

	// Fixed time: Wednesday 2026-10-28 14:30:00 UTC
	fixedTime := time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC)
	tae.SetClock(func() time.Time {
		return fixedTime
	})

	cycles, err := tae.Run(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = cycles

	// Verify now WME
	nowWMEs := tae.FindWMEsMatching(model.NewPositiveCE("now"))
	if len(nowWMEs) != 1 {
		t.Fatalf("expected 1 'now' WME, got %d", len(nowWMEs))
	}
	nowVal, ok := nowWMEs[0].Get("val")
	if !ok || nowVal.Raw().(int64) != fixedTime.UnixMilli() {
		t.Errorf("expected now.val %d, got %v", fixedTime.UnixMilli(), nowVal)
	}

	// Verify datetime WME
	dtWMEs := tae.FindWMEsMatching(model.NewPositiveCE("datetime"))
	if len(dtWMEs) != 1 {
		t.Fatalf("expected 1 'datetime' WME, got %d", len(dtWMEs))
	}
	dt := dtWMEs[0]
	if y, _ := dt.Get("year"); y.Raw().(int64) != 2026 {
		t.Errorf("expected year 2026, got %v", y)
	}
	if m, _ := dt.Get("month"); m.Raw().(int64) != 10 {
		t.Errorf("expected month 10, got %v", m)
	}
	if d, _ := dt.Get("day"); d.Raw().(int64) != 28 {
		t.Errorf("expected day 28, got %v", d)
	}
	if dow, _ := dt.Get("dow"); dow.Raw().(string) != "wednesday" {
		t.Errorf("expected dow wednesday, got %v", dow)
	}
	if hr, _ := dt.Get("hour"); hr.Raw().(int64) != 14 {
		t.Errorf("expected hour 14, got %v", hr)
	}
	if min, _ := dt.Get("minute"); min.Raw().(int64) != 30 {
		t.Errorf("expected minute 30, got %v", min)
	}

	// Verify cycle WME
	cycleWMEs := tae.FindWMEsMatching(model.NewPositiveCE("cycle"))
	if len(cycleWMEs) != 1 {
		t.Fatalf("expected 1 'cycle' WME, got %d", len(cycleWMEs))
	}
}

func TestTimeAwareEngine_TTLExpirationAndInterception(t *testing.T) {
	tae := NewTimeAwareEngine()

	// Initial time
	currentTime := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	tae.SetClock(func() time.Time {
		return currentTime
	})

	// Make an item with 100ms TTL
	wme := tae.Make("item", map[string]model.Value{
		"id":  model.NewSymbol("item-1"),
		"ttl": model.NewString("100ms"),
	})
	if wme == nil {
		t.Fatalf("failed to make item")
	}

	ttlVal, _ := wme.Get("ttl")
	expectedTTL := currentTime.UnixMilli() + 100
	if ttlVal.Raw().(int64) != expectedTTL {
		t.Errorf("expected ttl %d, got %v", expectedTTL, ttlVal)
	}

	// Interceptor rule: logs and records when an item is marked for removal
	var interceptedItem string
	var interceptedCycle int64

	interceptorRule := model.NewRule("log-item-expiration")
	interceptorRule.SetSalience(10) // higher than system cleanup (-1000)
	ce := model.NewPositiveCE("item").
		WithElementVariable("it").
		AddEqualTest("id", model.NewVariable("<id>")).
		AddTest("marked-for-remove-on-cycle", model.OpGreater, model.NewInt(0)).
		AddEqualTest("marked-for-remove-on-cycle", model.NewVariable("<c>"))
	interceptorRule.AddCondition(ce)
	interceptorRule.AddAction(model.CustomAction{
		Name: "log-expired",
		Execute: func(ctx any) error {
			if act, ok := ctx.(*conflict.Activation); ok {
				idVal, _ := act.Token.GetBinding("id")
				cVal, _ := act.Token.GetBinding("c")
				interceptedItem = idVal.Raw().(string)
				interceptedCycle = cVal.Raw().(int64)
			}
			return nil
		},
	})
	tae.AddRule(interceptorRule)

	// Step 1: Run while item is still valid (no expiration)
	tae.Run(5)
	if interceptedItem != "" {
		t.Errorf("expected no interception before TTL, got %s", interceptedItem)
	}
	items := tae.FindWMEsMatching(model.NewPositiveCE("item"))
	if len(items) != 1 {
		t.Fatalf("expected item still present, got %d", len(items))
	}

	// Step 2: Advance time past TTL (by 150ms)
	currentTime = currentTime.Add(150 * time.Millisecond)

	// Step 3: Run again -> marks for removal -> interceptor fires on cycle C -> system cleanup fires on cycle C+1
	cycles, err := tae.Run(10)
	if err != nil {
		t.Fatalf("unexpected error during run: %v", err)
	}
	if cycles < 2 {
		t.Errorf("expected at least 2 cycles (interceptor + cleanup), got %d", cycles)
	}

	// Verify interceptor fired
	if interceptedItem != "item-1" {
		t.Errorf("expected interceptedItem 'item-1', got %q", interceptedItem)
	}
	if interceptedCycle <= 0 {
		t.Errorf("expected positive interceptedCycle, got %d", interceptedCycle)
	}

	// Verify item was removed by cleanup rule
	remainingItems := tae.FindWMEsMatching(model.NewPositiveCE("item"))
	if len(remainingItems) != 0 {
		t.Errorf("expected 0 items remaining after cleanup, got %d: %v", len(remainingItems), remainingItems)
	}
}

func TestTimeAwareEngine_ScheduledRecurringEvent(t *testing.T) {
	tae := NewTimeAwareEngine()

	// Initial time: Day 1, 05:50am UTC
	loc := time.UTC
	day1 := time.Date(2026, 10, 28, 5, 50, 0, 0, loc)
	current := day1
	tae.SetClock(func() time.Time {
		return current
	})

	// Schedule breakfast from 06:00am for 30m duration
	err := tae.Schedule("breakfast", "06:00am", "30m", true)
	if err != nil {
		t.Fatalf("failed to schedule breakfast: %v", err)
	}

	// 1. Run at 05:50am: breakfast should NOT be materialized
	tae.Run(5)
	if len(tae.FindWMEsMatching(model.NewPositiveCE("breakfast"))) != 0 {
		t.Fatalf("expected no breakfast before 06:00am")
	}

	// 2. Advance clock to 06:05am: within [06:00, 06:30)
	current = time.Date(2026, 10, 28, 6, 5, 0, 0, loc)
	tae.Run(5)
	bfastWMEs := tae.FindWMEsMatching(model.NewPositiveCE("breakfast"))
	if len(bfastWMEs) != 1 {
		t.Fatalf("expected breakfast materialized at 06:05am, got %d", len(bfastWMEs))
	}

	// Verify breakfast TTL is 06:30am
	expectedEnd := time.Date(2026, 10, 28, 6, 30, 0, 0, loc).UnixMilli()
	ttlVal, ok := bfastWMEs[0].Get("ttl")
	if !ok || ttlVal.Raw().(int64) != expectedEnd {
		t.Errorf("expected breakfast TTL %d, got %v", expectedEnd, ttlVal)
	}

	// 3. Advance clock to 06:35am: breakfast expires and is cleaned up
	current = time.Date(2026, 10, 28, 6, 35, 0, 0, loc)
	tae.Run(5)
	if len(tae.FindWMEsMatching(model.NewPositiveCE("breakfast"))) != 0 {
		t.Fatalf("expected breakfast removed after 06:30am expiration")
	}

	// 4. Advance clock to Day 2, 05:50am: before 06:00am on day 2
	current = time.Date(2026, 10, 29, 5, 50, 0, 0, loc)
	tae.Run(5)
	if len(tae.FindWMEsMatching(model.NewPositiveCE("breakfast"))) != 0 {
		t.Fatalf("expected no breakfast before 06:00am on Day 2")
	}

	// 5. Advance clock to Day 2, 06:10am: should reoccur!
	current = time.Date(2026, 10, 29, 6, 10, 0, 0, loc)
	tae.Run(5)
	bfastDay2 := tae.FindWMEsMatching(model.NewPositiveCE("breakfast"))
	if len(bfastDay2) != 1 {
		t.Fatalf("expected breakfast to reoccur on Day 2, got %d", len(bfastDay2))
	}
	expectedEndDay2 := time.Date(2026, 10, 29, 6, 30, 0, 0, loc).UnixMilli()
	ttlValDay2, _ := bfastDay2[0].Get("ttl")
	if ttlValDay2.Raw().(int64) != expectedEndDay2 {
		t.Errorf("expected day 2 breakfast TTL %d, got %v", expectedEndDay2, ttlValDay2)
	}
}

func TestTimeAwareEngine_DecoupledFromCore(t *testing.T) {
	// Standard core engine should not have now/cycle/datetime WMEs or auto-injected attributes
	core := New()
	wme := core.Make("product", map[string]model.Value{
		"sku": model.NewSymbol("WIDGET-1"),
	})
	if wme.Has("ttl") {
		t.Errorf("core engine WME should not have ttl attribute")
	}
	if wme.Has("marked-for-remove-on-cycle") {
		t.Errorf("core engine WME should not have marked-for-remove-on-cycle attribute")
	}
	if len(core.FindWMEsMatching(model.NewPositiveCE("now"))) != 0 {
		t.Errorf("core engine should not have 'now' WME")
	}
}

func TestTimeAwareEngine_MakeWithScheduleAttributes(t *testing.T) {
	tae := NewTimeAwareEngine()

	// Make breakfast using start and duration attributes
	w := tae.Make("breakfast", map[string]model.Value{
		"start":    model.NewString("06:00am"),
		"duration": model.NewString("30m"),
	})
	if w == nil {
		t.Fatalf("expected WME created")
	}

	// Verify schedule is registered
	scheds := tae.Schedules()
	if len(scheds) != 1 {
		t.Fatalf("expected 1 schedule registered, got %d", len(scheds))
	}
	if scheds[0].Class != "breakfast" || scheds[0].StartTime != "06:00am" || scheds[0].DurationOrEnd != "30m" || !scheds[0].IsDuration {
		t.Errorf("unexpected schedule: %+v", scheds[0])
	}
}

func TestTimeAwareEngine_DatetimeMatching(t *testing.T) {
	tae := NewTimeAwareEngine()

	// Fixed time: Wednesday 2026-10-28 10:00:00 UTC
	loc := time.UTC
	wednesdayTime := time.Date(2026, 10, 28, 10, 0, 0, 0, loc)
	tae.SetClock(func() time.Time {
		return wednesdayTime
	})

	firedWednesday := false
	ruleWed := model.NewRule("wednesday-special")
	ceWed := model.NewPositiveCE("datetime").
		AddEqualTest("dow", model.NewSymbol("wednesday"))
	ruleWed.AddCondition(ceWed)
	ruleWed.AddAction(model.CustomAction{
		Name: "wed-fired",
		Execute: func(ctx any) error {
			firedWednesday = true
			return nil
		},
	})
	tae.AddRule(ruleWed)

	tae.Run(5)
	if !firedWednesday {
		t.Errorf("expected wednesday rule to fire")
	}

	// Change to Monday: 2026-10-26
	tae2 := NewTimeAwareEngine()
	mondayTime := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)
	tae2.SetClock(func() time.Time {
		return mondayTime
	})
	firedWednesday2 := false
	ruleWed2 := model.NewRule("wednesday-special")
	ruleWed2.AddCondition(ceWed)
	ruleWed2.AddAction(model.CustomAction{
		Name: "wed-fired-2",
		Execute: func(ctx any) error {
			firedWednesday2 = true
			return nil
		},
	})
	tae2.AddRule(ruleWed2)

	tae2.Run(5)
	if firedWednesday2 {
		t.Errorf("did not expect wednesday rule to fire on monday")
	}
}

