package manners

import (
	"bytes"
	"path/filepath"
	"testing"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
)

func init() {
	_ = WriteMannersFiles(".")
}

func TestManners16(t *testing.T) {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	path := filepath.Join(".", "manners16.ops")
	if err := eng.LoadFile(path); err != nil {
		t.Fatalf("failed to load %s: %v", path, err)
	}

	cycles, err := eng.Run(5000)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to halt after finding solution, cycles: %d", cycles)
	}

	t.Logf("Manners 16 solved in %d cycles. Output preview:\n%s", cycles, out.String())
}

func TestManners32(t *testing.T) {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	path := filepath.Join(".", "manners32.ops")
	if err := eng.LoadFile(path); err != nil {
		t.Fatalf("failed to load %s: %v", path, err)
	}

	cycles, err := eng.Run(10000)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to halt after finding solution, cycles: %d", cycles)
	}

	t.Logf("Manners 32 solved in %d cycles.", cycles)
}

func BenchmarkManners16(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		_ = eng.SetWatchLevel(0)
		_ = eng.LoadFile("manners16.ops")
		b.StartTimer()

		_, _ = eng.Run(5000)
	}
}

func BenchmarkManners32(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		_ = eng.SetWatchLevel(0)
		_ = eng.LoadFile("manners32.ops")
		b.StartTimer()

		_, _ = eng.Run(10000)
	}
}
