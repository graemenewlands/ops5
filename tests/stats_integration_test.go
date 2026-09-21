package tests

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"ops5/pkg/engine"
	"ops5/pkg/model"
	"ops5/pkg/parser"
)

// helper to execute OPS5 script statements (rules and top-level makes)
func runScript(t *testing.T, eng *engine.Engine, script string) {
	p, err := parser.NewParser(script)
	if err != nil {
		t.Fatalf("parser init error: %v", err)
	}

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			t.Fatalf("syntax error: %v", err)
		}
		if stmt == nil {
			break
		}

		switch stmt.Type {
		case parser.StmtRule:
			eng.AddRule(stmt.Rule)
		case parser.StmtMake:
			eng.Make(stmt.MakeClass, stmt.MakeAttributes)
		}
	}
}

func TestStatsAccumulationSingleRule(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// In OPS5: Create the rule and working memory elements via (make item ^value ...)
	script := `
(p compute-item-stats
	(control ^action calculate)
	(accumulate (item ^value <v>) :sum <v> <sum>)
	(accumulate (item ^value <v>) :max <v> <max>)
	(accumulate (item ^value <v>) :avg <v> <avg>)
	(accumulate (item ^value <v>) :count <cnt>)
-->
	(make stats ^sum <sum> ^max <max> ^avg <avg> ^count <cnt>)
	(write "Results: sum=" <sum> "max=" <max> "avg=" <avg> "count=" <cnt> (crlf))
)

(make control ^action calculate)
(make item ^value 10)
(make item ^value 25)
(make item ^value 42)
(make item ^value 8)
(make item ^value 15)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule firing, got %d", fired)
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Results: sum= 100 max= 42 avg= 20 count= 5") {
		t.Errorf("unexpected output: %s", output)
	}

	stats := eng.WorkingMemory().FindByClass("stats")
	if len(stats) != 1 {
		t.Fatalf("expected 1 stats WME, got %d", len(stats))
	}
	sumVal, _ := stats[0].Get("sum")
	maxVal, _ := stats[0].Get("max")
	avgVal, _ := stats[0].Get("avg")
	cntVal, _ := stats[0].Get("count")

	if !sumVal.Equal(model.NewInt(100)) {
		t.Errorf("expected sum 100, got %v", sumVal)
	}
	if !maxVal.Equal(model.NewInt(42)) {
		t.Errorf("expected max 42, got %v", maxVal)
	}
	if !avgVal.Equal(model.NewFloat(20.0)) {
		t.Errorf("expected avg 20, got %v", avgVal)
	}
	if !cntVal.Equal(model.NewInt(5)) {
		t.Errorf("expected count 5, got %v", cntVal)
	}
}

// TestStatsAccumulationWithRandomValues verifies that an arbitrary set of random numbers
// decided at test generation time accurately accumulates sum, max, average, and count.
func TestStatsAccumulationWithRandomValues(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// Build a script with a rule to compute stats
	ruleSrc := `
(p compute-stats
	(control ^action calculate)
	(accumulate (item ^value <v>) :sum <v> <sum>)
	(accumulate (item ^value <v>) :max <v> <max>)
	(accumulate (item ^value <v>) :avg <v> <avg>)
	(accumulate (item ^value <v>) :count <cnt>)
-->
	(make stats ^sum <sum> ^max <max> ^avg <avg> ^count <cnt>)
	(write "Sum:" <sum> "Max:" <max> "Avg:" <avg> "Count:" <cnt> (crlf))
)

(make control ^action calculate)
`
	// Generate random numbers using a deterministic seed
	rng := rand.New(rand.NewSource(2026))
	numItems := 10
	values := make([]int64, numItems)

	var sb strings.Builder
	sb.WriteString(ruleSrc)

	var expectedSum int64
	var expectedMax int64
	for i := 0; i < numItems; i++ {
		// Random integer between 1 and 1000
		val := int64(rng.Intn(1000) + 1)
		values[i] = val
		expectedSum += val
		if i == 0 || val > expectedMax {
			expectedMax = val
		}
		sb.WriteString(fmt.Sprintf("(make item ^value %d)\n", val))
	}
	expectedAvg := float64(expectedSum) / float64(numItems)
	expectedCount := int64(numItems)

	runScript(t, eng, sb.String())

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 rule to fire, got %d", fired)
	}

	stats := eng.WorkingMemory().FindByClass("stats")
	if len(stats) != 1 {
		t.Fatalf("expected 1 stats WME, got %d", len(stats))
	}
	sumVal, _ := stats[0].Get("sum")
	maxVal, _ := stats[0].Get("max")
	avgVal, _ := stats[0].Get("avg")
	cntVal, _ := stats[0].Get("count")

	t.Logf("Generated values: %v", values)
	t.Logf("Expected: sum=%d, max=%d, avg=%v, count=%d", expectedSum, expectedMax, expectedAvg, expectedCount)
	t.Logf("Computed: sum=%v, max=%v, avg=%v, count=%v", sumVal, maxVal, avgVal, cntVal)

	if !sumVal.Equal(model.NewInt(expectedSum)) {
		t.Errorf("sum mismatch: expected %d, got %v", expectedSum, sumVal)
	}
	if !maxVal.Equal(model.NewInt(expectedMax)) {
		t.Errorf("max mismatch: expected %d, got %v", expectedMax, maxVal)
	}
	if !avgVal.Equal(model.NewFloat(expectedAvg)) {
		t.Errorf("avg mismatch: expected %v, got %v", expectedAvg, avgVal)
	}
	if !cntVal.Equal(model.NewInt(expectedCount)) {
		t.Errorf("count mismatch: expected %d, got %v", expectedCount, cntVal)
	}
}

// TestStatsAccumulationDynamicUpdates verifies that adding and removing item WMEs
// dynamically updates the accumulated sum, max, average, and count across engine cycles.
func TestStatsAccumulationDynamicUpdates(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p monitor-stats
	(control ^mode active)
	(accumulate (item ^value <v>) :sum <v> <sum>)
	(accumulate (item ^value <v>) :max <v> <max>)
	(accumulate (item ^value <v>) :avg <v> <avg>)
	(accumulate (item ^value <v>) :count <cnt>)
-->
	(write "UPDATE: sum=" <sum> "max=" <max> "avg=" <avg> "count=" <cnt> (crlf))
)

(make control ^mode active)
`
	runScript(t, eng, ruleSrc)

	// Phase 1: Add 3 items: 10, 20, 30
	// Expected: sum=60, max=30, avg=20, count=3
	w1 := eng.Make("item", map[string]model.Value{"value": model.NewInt(10)})
	eng.Make("item", map[string]model.Value{"value": model.NewInt(20)})
	eng.Make("item", map[string]model.Value{"value": model.NewInt(30)})

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing in phase 1, got %d", fired)
	}
	if !strings.Contains(out.String(), "UPDATE: sum= 60 max= 30 avg= 20 count= 3") {
		t.Errorf("unexpected output phase 1: %s", out.String())
	}

	out.Reset()

	// Phase 2: Add 1 item: 40
	// Expected: sum=100, max=40, avg=25, count=4
	eng.Make("item", map[string]model.Value{"value": model.NewInt(40)})

	fired, err = eng.Run(-1)
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing in phase 2, got %d", fired)
	}
	if !strings.Contains(out.String(), "UPDATE: sum= 100 max= 40 avg= 25 count= 4") {
		t.Errorf("unexpected output phase 2: %s", out.String())
	}

	out.Reset()

	// Phase 3: Remove w1 (value 10)
	// Remaining: 20, 30, 40 -> sum=90, max=40, avg=30, count=3
	eng.Remove(w1.Timetag)

	fired, err = eng.Run(-1)
	if err != nil {
		t.Fatalf("run 3 failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing in phase 3, got %d", fired)
	}
	if !strings.Contains(out.String(), "UPDATE: sum= 90 max= 40 avg= 30 count= 3") {
		t.Errorf("unexpected output phase 3: %s", out.String())
	}
}
