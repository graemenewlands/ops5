package benchmarks

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
)

type benchmarkResult struct {
	Name          string
	Cycles        int
	WMEsAsserted  int64
	Duration      time.Duration
	CyclesPerSec  float64
	WMEsPerSec    float64
	PeakHeapBytes uint64
}

func runBenchmarkProblem(t *testing.T, name, opsPath string, maxCycles int) benchmarkResult {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	if err := eng.LoadFile(opsPath); err != nil {
		t.Fatalf("[%s] failed to load %s: %v", name, opsPath, err)
	}

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()
	cycles, err := eng.Run(maxCycles)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("[%s] engine execution error: %v", name, err)
	}
	if !eng.IsHalted() {
		t.Fatalf("[%s] engine did not reach quiescence/halt within %d cycles", name, maxCycles)
	}

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	totalAsserted := eng.WorkingMemory().TotalAsserted()
	sec := elapsed.Seconds()
	if sec == 0 {
		sec = 1e-9
	}

	heapDelta := uint64(0)
	if memAfter.TotalAlloc > memBefore.TotalAlloc {
		heapDelta = memAfter.TotalAlloc - memBefore.TotalAlloc
	}

	return benchmarkResult{
		Name:          name,
		Cycles:        cycles,
		WMEsAsserted:  totalAsserted,
		Duration:      elapsed,
		CyclesPerSec:  float64(cycles) / sec,
		WMEsPerSec:    float64(totalAsserted) / sec,
		PeakHeapBytes: heapDelta,
	}
}

func TestBenchmarkSuite(t *testing.T) {
	benchmarks := []struct {
		name      string
		relPath   string
		maxCycles int
	}{
		{"Manners-16", filepath.Join("manners", "manners16.ops"), 5000},
		{"Manners-32", filepath.Join("manners", "manners32.ops"), 10000},
		{"Manners-64", filepath.Join("manners", "manners64.ops"), 20000},
		{"Waltz-12", filepath.Join("waltz", "waltz12.ops"), 5000},
		{"Waltz-50", filepath.Join("waltz", "waltz50.ops"), 10000},
		{"Zebra-5", filepath.Join("zebra", "zebra.ops"), 1000},
	}

	var results []benchmarkResult
	for _, b := range benchmarks {
		if testing.Short() && b.name == "Manners-64" {
			t.Logf("Skipping %s in -short mode", b.name)
			continue
		}
		res := runBenchmarkProblem(t, b.name, b.relPath, b.maxCycles)
		results = append(results, res)
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("========================================================================================================================\n")
	sb.WriteString(fmt.Sprintf("%-12s | %8s | %10s | %12s | %14s | %14s | %12s\n",
		"Benchmark", "Cycles", "WMEs Assert", "Quiescence", "Cycles/sec", "WMEs/sec", "Heap Alloc"))
	sb.WriteString("------------------------------------------------------------------------------------------------------------------------\n")

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("%-12s | %8d | %10d | %12s | %14.1f | %14.1f | %10.2f KB\n",
			r.Name,
			r.Cycles,
			r.WMEsAsserted,
			r.Duration.Round(time.Millisecond),
			r.CyclesPerSec,
			r.WMEsPerSec,
			float64(r.PeakHeapBytes)/1024.0,
		))
	}
	sb.WriteString("========================================================================================================================\n")

	t.Log(sb.String())
}

func benchmarkRunner(b *testing.B, opsPath string, maxCycles int) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		var out bytes.Buffer
		eng.SetOutputWriter(&out)
		_ = eng.SetWatchLevel(0)

		if err := eng.LoadFile(opsPath); err != nil {
			b.Fatalf("failed to load %s: %v", opsPath, err)
		}

		b.StartTimer()
		t0 := time.Now()
		cycles, err := eng.Run(maxCycles)
		dur := time.Since(t0)

		if err != nil {
			b.Fatalf("run failed: %v", err)
		}
		if !eng.IsHalted() {
			b.Fatalf("expected halt")
		}

		sec := dur.Seconds()
		if sec > 0 {
			b.ReportMetric(float64(cycles)/sec, "cycles/s")
			b.ReportMetric(float64(eng.WorkingMemory().TotalAsserted())/sec, "wmes/s")
		}
	}
}

func BenchmarkSuiteManners16(b *testing.B) {
	benchmarkRunner(b, filepath.Join("manners", "manners16.ops"), 5000)
}

func BenchmarkSuiteManners32(b *testing.B) {
	benchmarkRunner(b, filepath.Join("manners", "manners32.ops"), 10000)
}

func BenchmarkSuiteWaltz12(b *testing.B) {
	benchmarkRunner(b, filepath.Join("waltz", "waltz12.ops"), 5000)
}

func BenchmarkSuiteWaltz50(b *testing.B) {
	benchmarkRunner(b, filepath.Join("waltz", "waltz50.ops"), 10000)
}

func BenchmarkSuiteZebra(b *testing.B) {
	benchmarkRunner(b, filepath.Join("zebra", "zebra.ops"), 1000)
}
