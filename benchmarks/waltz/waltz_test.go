package waltz

import (
	"bytes"
	"path/filepath"
	"testing"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
)

func TestWaltz12(t *testing.T) {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	path := filepath.Join(".", "waltz12.ops")
	if err := eng.LoadFile(path); err != nil {
		t.Fatalf("failed to load %s: %v", path, err)
	}

	cycles, err := eng.Run(5000)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to halt after labeling, cycles: %d", cycles)
	}

	t.Logf("Waltz 12 completed in %d cycles. Output preview:\n%s", cycles, out.String())
}

func TestWaltz50(t *testing.T) {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	path := filepath.Join(".", "waltz50.ops")
	if err := eng.LoadFile(path); err != nil {
		t.Fatalf("failed to load %s: %v", path, err)
	}

	cycles, err := eng.Run(10000)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to halt after labeling, cycles: %d", cycles)
	}

	t.Logf("Waltz 50 completed in %d cycles. Output preview:\n%s", cycles, out.String())
}

func BenchmarkWaltz12(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		_ = eng.SetWatchLevel(0)
		_ = eng.LoadFile("waltz12.ops")
		b.StartTimer()

		_, _ = eng.Run(5000)
	}
}

func BenchmarkWaltz50(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		_ = eng.SetWatchLevel(0)
		_ = eng.LoadFile("waltz50.ops")
		b.StartTimer()

		_, _ = eng.Run(10000)
	}
}
