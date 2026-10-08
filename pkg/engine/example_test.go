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
