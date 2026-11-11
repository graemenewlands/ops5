package engine_test

import (
	"context"
	"fmt"
	"time"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

// Task defines the working memory element structure mapped directly to an OPS5 element class.
type Task struct {
	ID     int64  `ops5:"id"`
	Status string `ops5:"status,symbol"`
}

// Example demonstrates standard single-engine rule registration, struct-based schema
// declaration and WME assertion, and execution until quiescence.
func Example() {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	eng.SetWatchLevel(0) // quiet output

	// Automatically register schema from Go struct definition
	if _, err := eng.RegisterStruct(Task{}); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p complete-task
		   <t> (task ^id <tid> ^status pending)
		   -->
		   (modify <t> ^status completed)
		   (write "Task" <tid> "is completed" (crlf))
		   (halt)
		)
	`)
	if err != nil {
		panic(err)
	}
	for _, r := range rules {
		eng.AddRule(r)
	}

	// Assert WME directly from Go struct
	if _, err := eng.MakeFromStruct(Task{
		ID:     101,
		Status: "pending",
	}); err != nil {
		panic(err)
	}

	cycles, err := eng.Run(100)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Cycles: %d, Halted: %v\n", cycles, eng.IsHalted())

	// Output:
	// Task 101 is completed
	// Cycles: 1, Halted: true
}

// WorkerTask defines a task routed to specific worker partitions.
type WorkerTask struct {
	ID           int64  `ops5:"id"`
	TargetWorker string `ops5:"target_worker,symbol"`
	Status       string `ops5:"status,symbol"`
}

func (WorkerTask) OPS5ClassName() string {
	return "task"
}

// ExamplePartitionedEngine demonstrates ParaOPS5 partitioned multi-core concurrency
// where facts are routed dynamically between isolated Rete partitions using Go struct mappings.
func ExamplePartitionedEngine() {
	// Router directs tasks with target_worker attribute to specific partition inboxes
	router := func(origin, class string, attrs map[string]model.Value) []string {
		if dest, ok := attrs["target_worker"]; ok {
			return []string{dest.String()}
		}
		return nil
	}

	pe := engine.NewPartitionedEngine(router)

	// Declare schema across partitions via Go struct
	if _, err := pe.RegisterStruct(WorkerTask{}); err != nil {
		panic(err)
	}

	p1, _ := pe.AddPartition("worker-1")
	p2, _ := pe.AddPartition("worker-2")
	p1.Engine.SetWatchLevel(0)
	p2.Engine.SetWatchLevel(0)

	// Add rule to worker-2 to process tasks targeted to it
	rules, err := parser.ParseRules(`
		(p handle-worker2-task
		   <t> (task ^id <tid> ^target_worker worker-2 ^status pending)
		   -->
		   (modify <t> ^status processed)
		   (write "Worker-2 handled task" <tid> (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	pe.AddRuleToPartition("worker-2", rules[0])

	// Assert task on worker-1 intended for worker-2 from struct
	if _, err := p1.Engine.MakeFromStruct(WorkerTask{
		ID:           99,
		TargetWorker: "worker-2",
		Status:       "pending",
	}); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cycleMap, _ := pe.RunParallel(ctx, 100)
	totalCycles := 0
	for _, c := range cycleMap {
		totalCycles += c
	}
	fmt.Printf("Parallel execution finished with %d cycle(s)\n", totalCycles)

	// Output:
	// Worker-2 handled task 99
	// Parallel execution finished with 1 cycle(s)
}

// ExampleEngine_MakeBatch demonstrates parallel batch ingestion of working memory facts.
func ExampleEngine_MakeBatch() {
	eng := engine.New()
	eng.SetWatchLevel(0)
	eng.SetAlphaWorkers(4)

	rules, err := parser.ParseRules(`
		(p count-items
		   (item ^id <id>)
		   -->
		   (write "Found item" <id> (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	eng.AddRule(rules[0])

	// Assert batch of items
	wmes := eng.MakeBatch([]engine.MakeRequest{
		{Class: "item", Attributes: map[string]model.Value{"id": model.NewInt(1)}},
		{Class: "item", Attributes: map[string]model.Value{"id": model.NewInt(2)}},
	})

	fmt.Printf("Asserted %d items in batch\n", len(wmes))

	// Output:
	// Asserted 2 items in batch
}

// ExampleTimeAwareEngine demonstrates scheduled recurring events, temporal state,
// and automatic fact expiration with TimeAwareEngine.
func ExampleTimeAwareEngine() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	// Set clock to Wednesday 06:15am UTC
	refTime := time.Date(2026, 10, 28, 6, 15, 0, 0, time.UTC)
	tae.SetClock(func() time.Time {
		return refTime
	})

	// Schedule breakfast daily between 06:00am and 06:30am
	if err := tae.Schedule("breakfast", "06:00am", "30m", true); err != nil {
		panic(err)
	}

	// Register a rule that fires when breakfast is active on a weekday
	rules, err := parser.ParseRules(`
		(p serve-breakfast
		   (datetime ^dow << monday tuesday wednesday thursday friday >>)
		   (breakfast)
		   -->
		   (write "Serving fresh breakfast!" (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	tae.AddRule(rules[0])

	// Run time-aware execution
	_, _ = tae.Run(10)

	// Output:
	// Serving fresh breakfast!
}

// Counter models an OPS5 working memory element for counting tasks.
type Counter struct {
	Val int
}

// ExampleTimeAwareEngine_StartDaemon demonstrates pacing time-disassociated rules
// using a background ticker daemon that halts on quiescence.
func ExampleTimeAwareEngine_StartDaemon() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	// Assert initial fact using a typed Go struct
	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p count-up
		   <c> (counter ^val <v> { < 3 })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		   (write "Counter:" (compute <v> + 1) (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	tae.AddRule(rules[0])

	// Start daemon with 5ms interval, 1 cycle per tick, stopOnQuiescence = true
	daemon, err := tae.StartDaemon(5*time.Millisecond, 1, true)
	if err != nil {
		panic(err)
	}

	<-daemon.Done()

	// Output:
	// Counter: 1
	// Counter: 2
	// Counter: 3
}

// ExampleTimeAwareEngine_RunSimulated demonstrates running time-disassociated rules
// with simulated monotonic virtual time at maximum CPU speed without real-time delay.
func ExampleTimeAwareEngine_RunSimulated() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	// Assert initial fact using a typed Go struct
	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p count-up
		   <c> (counter ^val <v> { < 3 })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		   (write "Counter:" (compute <v> + 1) (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	tae.AddRule(rules[0])

	// Advance virtual time by 1s per step until quiescence (runs in < 1ms of CPU time)
	_, _, err = tae.RunSimulated(1*time.Second, 1, 0)
	if err != nil {
		panic(err)
	}

	// Output:
	// Counter: 1
	// Counter: 2
	// Counter: 3
}

// TrafficSignal models a traffic light state machine fact
type TrafficSignal struct {
	Color string `ops5:",symbol"`
	TTL   string
}

// WorkflowOrder models a workflow order fact
type WorkflowOrder struct {
	ID     int
	Status string `ops5:",symbol"`
}

// ExampleTimeAwareEngine_SimRun_Way1_TTL demonstrates SimPy-style discrete-event time leaps
// driven by WME TTL expirations in a traffic light state machine.
func ExampleTimeAwareEngine_SimRun_Way1_TTL() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	if _, err := tae.MakeFromStruct(TrafficSignal{Color: "green", TTL: "45s"}); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p green-to-yellow
		   <light> (trafficsignal ^color green ^marked-for-remove-on-cycle { > 0 })
		   -->
		   (make trafficsignal ^color yellow ^ttl "5s")
		   (write "Signal transitioned to YELLOW" (crlf))
		)

		(p yellow-to-red
		   <light> (trafficsignal ^color yellow ^marked-for-remove-on-cycle { > 0 })
		   -->
		   (make trafficsignal ^color red ^ttl "30s")
		   (write "Signal transitioned to RED" (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	for _, r := range rules {
		tae.AddRule(r)
	}

	// Run discrete-event simulation across 60 seconds
	res, err := tae.SimRunUntil(60 * time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Events jumped: %d. Simulated duration: %v\n", res.EventsJumped, res.Duration)

	// Output:
	// Signal transitioned to YELLOW
	// Signal transitioned to RED
	// Events jumped: 2. Simulated duration: 1m0s
}

// ExampleTimeAwareEngine_SimRun_Way2_ScheduleFactAfter demonstrates scheduling future fact
// arrivals in discrete-event simulation (e.g. order delivery workflow).
func ExampleTimeAwareEngine_SimRun_Way2_ScheduleFactAfter() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(baseTime.UnixMilli())

	if _, err := tae.MakeFromStruct(WorkflowOrder{ID: 101, Status: "placed"}); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p process-order
		   <o> (workfloworder ^id <id> ^status placed)
		   -->
		   (modify <o> ^status preparing)
		   (write "Order " <id> " preparing." (crlf))
		)

		(p fulfill-delivery
		   <d> (delivery ^order-id <id>)
		   <o> (workfloworder ^id <id> ^status preparing)
		   -->
		   (modify <o> ^status completed)
		   (write "Delivery arrived for order " <id> "! Order completed." (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	for _, r := range rules {
		tae.AddRule(r)
	}

	// Schedule delivery fact for 30 minutes in the future
	if err := tae.ScheduleFactAfter("delivery", map[string]any{"order-id": 101}, 30*time.Minute); err != nil {
		panic(err)
	}

	// Run until event queue is exhausted
	res, err := tae.SimRun(time.Time{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Events jumped: %d. Simulated duration: %v\n", res.EventsJumped, res.Duration)

	// Output:
	// Order  101  preparing.
	// Delivery arrived for order  101 ! Order completed.
	// Events jumped: 1. Simulated duration: 30m0s
}

// ExampleTimeAwareEngine_SimRun_Way3_Schedule demonstrates multi-hour calendar leaps
// across daily recurring schedules.
func ExampleTimeAwareEngine_SimRun_Way3_Schedule() {
	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0)

	// Wednesday 09:00am UTC
	start := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	tae.SetCurrentTime(start.UnixMilli())

	// Daily maintenance window: 02:00am - 03:00am
	if err := tae.Schedule("maintenance-window", "02:00am", "1h", true); err != nil {
		panic(err)
	}

	rules, err := parser.ParseRules(`
		(p run-nightly-maintenance
		   (datetime ^dow <day>)
		   (maintenance-window ^marked-for-remove-on-cycle 0)
		   -->
		   (write "Running maintenance on " <day> (crlf))
		)
	`)
	if err != nil {
		panic(err)
	}
	tae.AddRule(rules[0])

	// Leap across 48 hours of calendar time (jumping 17h, 1h, 23h, 1h in microseconds)
	res, err := tae.SimRunUntil(48 * time.Hour)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Events jumped: %d. Simulated duration: %v\n", res.EventsJumped, res.Duration)

	// Output:
	// Running maintenance on  thursday
	// Running maintenance on  friday
	// Events jumped: 4. Simulated duration: 48h0m0s
}

