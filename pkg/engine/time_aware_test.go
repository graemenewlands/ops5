package engine

import (
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
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

type Counter struct {
	Val int
}

func TestTimeAwareEngine_Daemon_CounterQuiescence(t *testing.T) {
	tae := NewTimeAwareEngine()
	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}

	rules, err := parser.ParseRules(`
		(p count-up
		   <c> (counter ^val <v> { < 5 })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		)
	`)
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	tae.AddRule(rules[0])

	// Interval of 5ms with quiescence stop
	daemon, err := tae.StartDaemon(5*time.Millisecond, 1, true)
	if err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	select {
	case <-daemon.Done():
		// Clean exit on quiescence
	case <-time.After(2 * time.Second):
		daemon.Stop()
		t.Fatalf("daemon timed out waiting for quiescence")
	}

	if daemon.Running() {
		t.Errorf("expected daemon to not be running")
	}
	if daemon.TotalCycles() != 5 {
		t.Errorf("expected 5 cycles, got %d", daemon.TotalCycles())
	}
	if daemon.Error() != nil {
		t.Errorf("unexpected daemon error: %v", daemon.Error())
	}

	// Verify counter WME final value
	wmes := tae.FindWMEsMatching(model.NewPositiveCE("counter"))
	if len(wmes) != 1 {
		t.Fatalf("expected 1 counter WME, got %d", len(wmes))
	}
	val, _ := wmes[0].Get("val")
	if val.Raw().(int64) != 5 {
		t.Errorf("expected final counter 5, got %v", val)
	}
}

func TestTimeAwareEngine_Daemon_StopInterruption(t *testing.T) {
	tae := NewTimeAwareEngine()
	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}

	rules, err := parser.ParseRules(`
		(p count-up-large
		   <c> (counter ^val <v> { < 1000 })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		)
	`)
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	tae.AddRule(rules[0])

	daemon, err := tae.StartDaemon(5*time.Millisecond, 1, true)
	if err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	// Let it run a few ticks then manually stop
	time.Sleep(30 * time.Millisecond)
	daemon.Stop()

	select {
	case <-daemon.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("daemon Done channel was not closed after Stop()")
	}

	if daemon.Running() {
		t.Errorf("expected daemon to report not running")
	}
	if daemon.TotalCycles() == 0 || daemon.TotalCycles() >= 1000 {
		t.Errorf("expected partial cycles (0 < cycles < 1000), got %d", daemon.TotalCycles())
	}
}

func TestTimeAwareEngine_Daemon_InvalidInterval(t *testing.T) {
	tae := NewTimeAwareEngine()
	_, err := tae.StartDaemon(0, 1, true)
	if err == nil {
		t.Errorf("expected error for non-positive interval")
	}
}

func TestTimeAwareEngine_RunSimulated(t *testing.T) {
	tae := NewTimeAwareEngine()
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}

	rules, err := parser.ParseRules(`
		(p count-up
		   <c> (counter ^val <v> { < 10 })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		)
	`)
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	tae.AddRule(rules[0])

	startReal := time.Now()
	cycles, simElapsed, err := tae.RunSimulated(1*time.Second, 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	realElapsed := time.Since(startReal)

	if cycles != 10 {
		t.Errorf("expected 10 cycles, got %d", cycles)
	}
	// 10 cycles + 1 quiescence check step = 11s of simulated time
	if simElapsed != 11*time.Second {
		t.Errorf("expected 11s simulated time, got %v", simElapsed)
	}
	// Entire run should execute in < 50ms of real CPU time
	if realElapsed > 100*time.Millisecond {
		t.Errorf("simulated run took too long in real time: %v", realElapsed)
	}

	// Verify counter WME final value
	wmes := tae.FindWMEsMatching(model.NewPositiveCE("counter"))
	if len(wmes) != 1 {
		t.Fatalf("expected 1 counter WME, got %d", len(wmes))
	}
	val, _ := wmes[0].Get("val")
	if val.Raw().(int64) != 10 {
		t.Errorf("expected final counter 10, got %v", val)
	}
}

func TestTimeAwareEngine_ErgonomicMakeHelpers(t *testing.T) {
	tae := NewTimeAwareEngine()

	// 1. MakeString with (counter ^val 0)
	wme1, err := tae.MakeString("(counter ^val 0)")
	if err != nil {
		t.Fatalf("MakeString error: %v", err)
	}
	if wme1.Class != "counter" {
		t.Errorf("expected class counter, got %s", wme1.Class)
	}
	val1, _ := wme1.Get("val")
	if val1.Raw().(int64) != 0 {
		t.Errorf("expected val 0, got %v", val1)
	}

	// 2. MakeKV with variadic args
	wme2, err := tae.MakeKV("score", "points", 100, "player", "alice")
	if err != nil {
		t.Fatalf("MakeKV error: %v", err)
	}
	pts, _ := wme2.Get("points")
	if pts.Raw().(int64) != 100 {
		t.Errorf("expected points 100, got %v", pts)
	}

	// 3. MakeMap with native Go map
	wme3, err := tae.MakeMap("status", map[string]any{"active": true, "code": 200})
	if err != nil {
		t.Fatalf("MakeMap error: %v", err)
	}
	act, _ := wme3.Get("active")
	if act.Raw().(bool) != true {
		t.Errorf("expected active true, got %v", act)
	}
}

// TrafficLight models a traffic signal state machine fact
type TrafficLight struct {
	Color string `ops5:",symbol"`
	TTL   string
}

// Order models a workflow order fact
type Order struct {
	ID     int
	Status string `ops5:",symbol"`
}

func TestTimeAwareEngine_SimRun_Way1_TTL(t *testing.T) {
	tae := NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	// Initial State: Green light lasting 45 seconds
	if _, err := tae.MakeFromStruct(TrafficLight{Color: "green", TTL: "45s"}); err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}

	rules, err := parser.ParseRules(`
		(p green-to-yellow
		   <light> (trafficlight ^color green ^marked-for-remove-on-cycle { > 0 })
		   -->
		   (make trafficlight ^color yellow ^ttl "5s")
		)

		(p yellow-to-red
		   <light> (trafficlight ^color yellow ^marked-for-remove-on-cycle { > 0 })
		   -->
		   (make trafficlight ^color red ^ttl "30s")
		)

		(p red-to-green
		   <light> (trafficlight ^color red ^marked-for-remove-on-cycle { > 0 })
		   -->
		   (make trafficlight ^color green ^ttl "45s")
		)
	`)
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	for _, r := range rules {
		tae.AddRule(r)
	}

	var jumps []time.Duration
	tae.SetOnTimeJump(func(from time.Time, to time.Time, reason string) {
		jumps = append(jumps, to.Sub(baseTime))
	})

	// Leap through states across 2 minutes (120 seconds) of simulated time
	res, err := tae.SimRunUntil(120 * time.Second)
	if err != nil {
		t.Fatalf("SimRunUntil error: %v", err)
	}

	if res.EventsJumped != 3 {
		t.Errorf("expected 3 jumps (45s, 50s, 80s), got %d (jumps: %v)", res.EventsJumped, jumps)
	}
	if res.Duration != 120*time.Second {
		t.Errorf("expected duration 120s, got %v", res.Duration)
	}
	if res.WallTime > 500*time.Millisecond {
		t.Errorf("expected simulation to finish in < 500ms wall time, got %v", res.WallTime)
	}
}

func TestTimeAwareEngine_SimRun_Way2_ScheduleFactAfter(t *testing.T) {
	tae := NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	if _, err := tae.MakeFromStruct(Order{ID: 101, Status: "placed"}); err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}

	rules, err := parser.ParseRules(`
		(p process-order
		   <o> (order ^id <id> ^status placed)
		   -->
		   (modify <o> ^status preparing)
		)

		(p fulfill-delivery
		   <d> (delivery ^order-id <id>)
		   <o> (order ^id <id> ^status preparing)
		   -->
		   (modify <o> ^status completed)
		)
	`)
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	for _, r := range rules {
		tae.AddRule(r)
	}

	// Schedule delivery fact for 30 minutes in the future
	if err := tae.ScheduleFactAfter("delivery", map[string]any{"order-id": 101}, 30*time.Minute); err != nil {
		t.Fatalf("ScheduleFactAfter error: %v", err)
	}

	var jumpTarget time.Time
	tae.SetOnTimeJump(func(from time.Time, to time.Time, reason string) {
		jumpTarget = to
	})

	// Run to natural event exhaustion
	res, err := tae.SimRun(time.Time{})
	if err != nil {
		t.Fatalf("SimRun error: %v", err)
	}

	if !res.Quiesced {
		t.Errorf("expected natural quiescence")
	}
	if res.EventsJumped != 1 {
		t.Errorf("expected 1 jump, got %d", res.EventsJumped)
	}
	if res.Duration != 30*time.Minute {
		t.Errorf("expected duration 30m, got %v", res.Duration)
	}
	if jumpTarget != baseTime.Add(30*time.Minute) {
		t.Errorf("expected jump target %v, got %v", baseTime.Add(30*time.Minute), jumpTarget)
	}

	// Verify order status is completed
	orders := tae.FindWMEsMatching(model.NewPositiveCE("order"))
	if len(orders) != 1 {
		t.Fatalf("expected 1 order fact, got %d", len(orders))
	}
	status, _ := orders[0].Get("status")
	if status.Raw().(string) != "completed" {
		t.Errorf("expected status 'completed', got %v", status)
	}
}

func TestTimeAwareEngine_SimRun_Way3_Schedule(t *testing.T) {
	tae := NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	// Wednesday 09:00am UTC
	start := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(start.UnixMilli())

	// Daily maintenance window: 02:00am - 03:00am (1 hour duration)
	if err := tae.Schedule("maintenance-window", "02:00am", "1h", true); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	maintenanceRuns := 0
	rule := model.NewRule("run-maintenance")
	rule.AddCondition(model.NewPositiveCE("maintenance-window").AddEqualTest("marked-for-remove-on-cycle", model.NewInt(0)))
	rule.AddAction(model.CustomAction{
		Name: "count-maintenance",
		Execute: func(ctx any) error {
			maintenanceRuns++
			return nil
		},
	})
	tae.AddRule(rule)

	res, err := tae.SimRunUntil(48 * time.Hour)
	if err != nil {
		t.Fatalf("SimRunUntil error: %v", err)
	}

	if res.Duration != 48*time.Hour {
		t.Errorf("expected 48 hours simulated duration, got %v", res.Duration)
	}
	if maintenanceRuns != 2 {
		t.Errorf("expected 2 nightly maintenance runs across 48h, got %d", maintenanceRuns)
	}
	if res.WallTime > 500*time.Millisecond {
		t.Errorf("expected 48h simulation to complete in < 500ms wall time, got %v", res.WallTime)
	}
}

func TestTimeAwareEngine_SimRun_UntilBoundary(t *testing.T) {
	tae := NewTimeAwareEngine()
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	// Schedule fact 10 minutes in future, but set until to 5 minutes
	_ = tae.ScheduleFactAfter("ping", map[string]any{"seq": 1}, 10*time.Minute)

	until := baseTime.Add(5 * time.Minute)
	res, err := tae.SimRun(until)
	if err != nil {
		t.Fatalf("SimRun error: %v", err)
	}

	if res.EndTime != until {
		t.Errorf("expected end time %v, got %v", until, res.EndTime)
	}
	if res.Duration != 5*time.Minute {
		t.Errorf("expected duration 5m, got %v", res.Duration)
	}
	if res.Quiesced {
		t.Errorf("expected Quiesced=false since future event at 10m is still pending")
	}
}

