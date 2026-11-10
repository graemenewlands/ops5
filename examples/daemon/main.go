package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

// Counter models an OPS5 working memory element for counting.
type Counter struct {
	Val int
}

type counterListener struct {
	mu          sync.Mutex
	startTime   time.Time
	lastTime    time.Time
	isSimulated bool
	engine      *engine.TimeAwareEngine
}

func (l *counterListener) OnAssert(wme *model.WME) {
	if strings.ToLower(wme.Class) != "counter" {
		return
	}
	val, ok := wme.Get("val")
	if !ok {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isSimulated {
		currTime := l.engine.CurrentTime().UTC()
		simElapsed := currTime.Sub(l.startTime)
		fmt.Printf("[Virtual T+%.3fs] [%s] Counter: %v\n", simElapsed.Seconds(), currTime.Format("2006-01-02 15:04:05.000 UTC"), val)
	} else {
		now := time.Now()
		delta := now.Sub(l.lastTime)
		l.lastTime = now
		fmt.Printf("[+%.3fs] [%s] Counter: %v\n", delta.Seconds(), now.Format("15:04:05.000"), val)
	}
}

func (l *counterListener) OnRetract(wme *model.WME) {
	// No-op for retractions
}

func main() {
	rateFlag := flag.String("rate", "1s", "Ticker interval / virtual time step (e.g. 1s, 500ms, 2s)")
	targetFlag := flag.Int("target", 10, "Target count before reaching quiescence")
	simulatedFlag := flag.Bool("simulated", false, "Run in simulated virtual time at maximum CPU speed without physical delay")
	flag.Parse()

	interval, err := time.ParseDuration(*rateFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid rate duration %q: %v\n", *rateFlag, err)
		os.Exit(1)
	}
	if interval <= 0 {
		fmt.Fprintf(os.Stderr, "Error: rate duration must be positive\n")
		os.Exit(1)
	}
	if *targetFlag <= 0 {
		fmt.Fprintf(os.Stderr, "Error: target must be greater than 0\n")
		os.Exit(1)
	}

	tae := engine.NewTimeAwareEngine()
	tae.SetWatchLevel(0) // Disable raw engine trace to keep output formatted cleanly

	var baseTime time.Time
	if *simulatedFlag {
		baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		tae.SetCurrentTime(baseTime.UnixMilli())
	}

	// Assert initial fact using a typed Go struct: (counter ^val 0)
	if _, err := tae.MakeFromStruct(Counter{Val: 0}); err != nil {
		fmt.Fprintf(os.Stderr, "Error asserting counter fact: %v\n", err)
		os.Exit(1)
	}

	// Pure OPS5 rule: completely disassociated from physical time.
	// Matches whenever counter is below target, increments by 1.
	ruleSrc := fmt.Sprintf(`
		(p count-up
		   <c> (counter ^val <v> { < %d })
		   -->
		   (modify <c> ^val (compute <v> + 1))
		)
	`, *targetFlag)

	rules, err := parser.ParseRules(ruleSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing rule: %v\n", err)
		os.Exit(1)
	}
	tae.AddRule(rules[0])

	startTime := time.Now()
	listener := &counterListener{
		startTime:   startTime,
		lastTime:    startTime,
		isSimulated: *simulatedFlag,
		engine:      tae,
	}
	if *simulatedFlag {
		listener.startTime = baseTime
		listener.lastTime = baseTime
	}
	tae.WorkingMemory().AddListener(listener)

	if *simulatedFlag {
		fmt.Printf("[SIMULATED] Starting virtual time run (step: %v, target: %d, base: %s)...\n",
			interval, *targetFlag, baseTime.Format("2006-01-02 15:04:05 UTC"))

		realStart := time.Now()
		cycles, simElapsed, err := tae.RunSimulated(interval, 1, 0)
		realElapsed := time.Since(realStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Simulation error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[SIMULATED] Quiescence reached (agenda empty).\n")
		fmt.Printf("Completed %d cycles. Simulated time: %v. Real CPU time: %v.\n",
			cycles, simElapsed, realElapsed.Round(time.Microsecond))
		return
	}

	// Physical ticker daemon mode:
	fmt.Printf("[%s] Starting TimeAwareEngine Daemon (rate: %v, target: %d)...\n",
		startTime.Format("15:04:05.000"), interval, *targetFlag)

	// Start daemon with interval, 1 cycle per tick, and stopOnQuiescence = true
	daemon, err := tae.StartDaemon(interval, 1, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	// Block until daemon terminates on quiescence
	<-daemon.Done()

	elapsed := time.Since(startTime)
	if err := daemon.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "Daemon stopped with error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[%s] Quiescence reached (agenda empty). Daemon completed %d cycles in %v.\n",
		time.Now().Format("15:04:05.000"), daemon.TotalCycles(), elapsed.Round(time.Millisecond))
}
