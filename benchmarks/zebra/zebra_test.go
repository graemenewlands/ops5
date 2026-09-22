package zebra

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
)

func TestZebra(t *testing.T) {
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	var out bytes.Buffer
	eng.SetOutputWriter(&out)
	_ = eng.SetWatchLevel(0)

	path := filepath.Join(".", "zebra.ops")
	if err := eng.LoadFile(path); err != nil {
		t.Fatalf("failed to load %s: %v", path, err)
	}

	cycles, err := eng.Run(1000)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if !eng.IsHalted() {
		t.Fatalf("expected engine to halt after solving zebra puzzle, cycles: %d", cycles)
	}

	outStr := out.String()
	t.Logf("Zebra puzzle solved in %d cycles. Output:\n%s", cycles, outStr)

	if !strings.Contains(outStr, "Question 1: Who drinks water? ->  norwegian") {
		t.Errorf("expected water drinker to be norwegian, got output: %s", outStr)
	}
	if !strings.Contains(outStr, "Question 2: Who owns the zebra? ->  japanese") {
		t.Errorf("expected zebra owner to be japanese, got output: %s", outStr)
	}
}

func BenchmarkZebra(b *testing.B) {
	path := filepath.Join(".", "zebra.ops")
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		eng := engine.New()
		eng.SetStrategy(conflict.StrategyLEX)
		var out bytes.Buffer
		eng.SetOutputWriter(&out)
		_ = eng.SetWatchLevel(0)

		if err := eng.LoadFile(path); err != nil {
			b.Fatalf("failed to load %s: %v", path, err)
		}
		b.StartTimer()
		if _, err := eng.Run(1000); err != nil {
			b.Fatalf("run failed: %v", err)
		}
	}
}
