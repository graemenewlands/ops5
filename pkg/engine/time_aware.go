package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/graemenewlands/ops5/pkg/model"
)

// Daemon controls a background ticking goroutine driving TimeAwareEngine execution cycles.
type Daemon struct {
	tae              *TimeAwareEngine
	interval         time.Duration
	cyclesPerTick    int
	stopOnQuiescence bool
	onTick           func(cycles int)

	done     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mu          sync.Mutex
	running     bool
	totalCycles int
	err         error
}

// ScheduleDefinition holds configuration and runtime state for a recurring scheduled event.
type ScheduleDefinition struct {
	Class         string
	StartTime     string // e.g. "06:00am"
	DurationOrEnd string // e.g. "30m" or "06:30am"
	IsDuration    bool   // true if DurationOrEnd is a relative duration, false if wall-clock end
	LastFiredDate string // "YYYY-MM-DD"
}

// TimeAwareEngine extends the core OPS5 Engine with temporal awareness,
// system now/cycle/datetime WMEs, TTL fact expiration, and recurring event schedules.
type TimeAwareEngine struct {
	*Engine
	mu                     sync.RWMutex
	clock                  func() time.Time
	simulatedTime          time.Time
	isSimulated            bool
	schedules              map[string]*ScheduleDefinition
	registeredCleanupRules map[string]bool
}

// NewTimeAwareEngine instantiates a new TimeAwareEngine wrapping a freshly initialized Engine.
func NewTimeAwareEngine() *TimeAwareEngine {
	core := New()
	tae := &TimeAwareEngine{
		Engine:                 core,
		clock:                  time.Now,
		schedules:              make(map[string]*ScheduleDefinition),
		registeredCleanupRules: make(map[string]bool),
	}

	// Register schemas for implicit system WMEs
	tae.DeclareClass("now", []string{"val"})
	tae.DeclareClass("cycle", []string{"val"})
	tae.DeclareClass("datetime", []string{"year", "month", "day", "dow", "hour", "minute", "second"})

	return tae
}

// SetClock configures a custom clock provider (defaults to time.Now).
func (tae *TimeAwareEngine) SetClock(fn func() time.Time) {
	tae.mu.Lock()
	defer tae.mu.Unlock()
	if fn == nil {
		tae.clock = time.Now
		tae.isSimulated = false
	} else {
		tae.clock = fn
		tae.isSimulated = false
	}
}

// SetCurrentTime sets a fixed clock returning the specified Unix epoch milliseconds in UTC.
func (tae *TimeAwareEngine) SetCurrentTime(epochMs int64) {
	t := time.UnixMilli(epochMs).UTC()
	tae.mu.Lock()
	defer tae.mu.Unlock()
	tae.simulatedTime = t
	tae.isSimulated = true
	tae.clock = func() time.Time {
		return tae.simulatedTime
	}
}

// CurrentTime returns the engine's current reference time (real or simulated).
func (tae *TimeAwareEngine) CurrentTime() time.Time {
	tae.mu.RLock()
	defer tae.mu.RUnlock()
	return tae.clock()
}

// AdvanceTime advances the current simulated time monotonically by delta and returns the new time.
func (tae *TimeAwareEngine) AdvanceTime(delta time.Duration) time.Time {
	tae.mu.Lock()
	defer tae.mu.Unlock()

	if !tae.isSimulated {
		tae.simulatedTime = tae.clock()
		tae.isSimulated = true
		tae.clock = func() time.Time {
			return tae.simulatedTime
		}
	}
	tae.simulatedTime = tae.simulatedTime.Add(delta)
	return tae.simulatedTime
}

// RunSimulated executes a simulated discrete-time loop without sleeping:
// on each step, it advances simulated time by timeStep, synchronizes (now) and (datetime),
// and invokes Run(cyclesPerStep).
// It halts when quiescence is reached (0 rules fired on a step) or maxSteps is reached.
func (tae *TimeAwareEngine) RunSimulated(timeStep time.Duration, cyclesPerStep int, maxSteps int) (int, time.Duration, error) {
	if timeStep <= 0 {
		return 0, 0, fmt.Errorf("timeStep must be positive, got %v", timeStep)
	}
	if cyclesPerStep <= 0 {
		cyclesPerStep = 1
	}

	tae.mu.Lock()
	if !tae.isSimulated {
		tae.simulatedTime = tae.clock()
		tae.isSimulated = true
		tae.clock = func() time.Time {
			return tae.simulatedTime
		}
	}
	startSimTime := tae.simulatedTime
	tae.mu.Unlock()

	totalCycles := 0
	steps := 0

	for {
		if maxSteps > 0 && steps >= maxSteps {
			break
		}
		steps++

		tae.AdvanceTime(timeStep)
		cycles, err := tae.Run(cyclesPerStep)
		if err != nil {
			tae.mu.RLock()
			elapsed := tae.simulatedTime.Sub(startSimTime)
			tae.mu.RUnlock()
			return totalCycles, elapsed, err
		}
		totalCycles += cycles

		if cycles == 0 {
			break
		}
	}

	tae.mu.RLock()
	simElapsed := tae.simulatedTime.Sub(startSimTime)
	tae.mu.RUnlock()

	return totalCycles, simElapsed, nil
}

// Schedule registers a recurring scheduled event.
func (tae *TimeAwareEngine) Schedule(className string, start string, durationOrEnd string, isDuration bool) error {
	tae.mu.Lock()
	defer tae.mu.Unlock()

	normClass := strings.ToLower(className)
	if normClass == "" {
		return fmt.Errorf("empty class name for schedule")
	}

	// Validate start format
	if _, err := model.ParseWallClock(start, time.Now(), time.Local); err != nil {
		return fmt.Errorf("invalid schedule start time %q: %w", start, err)
	}

	// Validate duration or end format
	if isDuration {
		if _, err := model.ParseDuration(durationOrEnd); err != nil {
			return fmt.Errorf("invalid schedule duration %q: %w", durationOrEnd, err)
		}
	} else {
		if _, err := model.ParseWallClock(durationOrEnd, time.Now(), time.Local); err != nil {
			return fmt.Errorf("invalid schedule end time %q: %w", durationOrEnd, err)
		}
	}

	tae.schedules[normClass] = &ScheduleDefinition{
		Class:         normClass,
		StartTime:     start,
		DurationOrEnd: durationOrEnd,
		IsDuration:    isDuration,
	}

	tae.ensureClassSchemaAndCleanupLocked(normClass)
	return nil
}

// Schedules returns a snapshot of all registered schedule definitions.
func (tae *TimeAwareEngine) Schedules() []*ScheduleDefinition {
	tae.mu.RLock()
	defer tae.mu.RUnlock()

	res := make([]*ScheduleDefinition, 0, len(tae.schedules))
	for _, s := range tae.schedules {
		copy := *s
		res = append(res, &copy)
	}
	return res
}

// Make overrides core Make to inject default ^ttl and ^marked-for-remove-on-cycle attributes,
// parse human-readable duration/wall-clock TTL strings into epoch milliseconds, and register cleanup rules.
func (tae *TimeAwareEngine) Make(class string, attrs map[string]model.Value) *model.WME {
	tae.mu.Lock()
	defer tae.mu.Unlock()

	normClass := strings.ToLower(class)

	// Bypass temporal injection for system WMEs
	if normClass == "now" || normClass == "cycle" || normClass == "datetime" {
		return tae.Engine.Make(class, attrs)
	}

	// Handle declarative schedule creation via Make: e.g. make breakfast ^start 06:00am ^duration 30m
	if startVal, hasStart := attrs["start"]; hasStart {
		var startStr string
		if s, ok := startVal.Text(); ok {
			startStr = s
		}
		if durVal, hasDur := attrs["duration"]; hasDur {
			if durStr, ok := durVal.Text(); ok {
				_ = tae.scheduleLocked(normClass, startStr, durStr, true)
			}
		} else if endVal, hasEnd := attrs["end"]; hasEnd {
			if endStr, ok := endVal.Text(); ok {
				_ = tae.scheduleLocked(normClass, startStr, endStr, false)
			}
		}
	}

	tae.ensureClassSchemaAndCleanupLocked(normClass)

	// Prepare resolved attributes with ttl and marked-for-remove-on-cycle defaults
	resolvedAttrs := make(map[string]model.Value, len(attrs)+2)
	for k, v := range attrs {
		resolvedAttrs[strings.ToLower(k)] = v
	}

	nowMs := tae.clock().UnixMilli()
	loc := tae.clock().Location()

	// 1. Process ^ttl
	if ttlVal, exists := resolvedAttrs["ttl"]; exists {
		parsedTTL, err := model.ParseTTL(ttlVal, nowMs, loc)
		if err == nil {
			resolvedAttrs["ttl"] = model.NewInt(parsedTTL)
		} else {
			resolvedAttrs["ttl"] = model.NewInt(0)
		}
	} else {
		resolvedAttrs["ttl"] = model.NewInt(0)
	}

	// 2. Process ^marked-for-remove-on-cycle
	if _, exists := resolvedAttrs["marked-for-remove-on-cycle"]; !exists {
		resolvedAttrs["marked-for-remove-on-cycle"] = model.NewInt(0)
	}

	return tae.Engine.Make(class, resolvedAttrs)
}

func (tae *TimeAwareEngine) scheduleLocked(className string, start string, durationOrEnd string, isDuration bool) error {
	tae.schedules[className] = &ScheduleDefinition{
		Class:         className,
		StartTime:     start,
		DurationOrEnd: durationOrEnd,
		IsDuration:    isDuration,
	}
	tae.ensureClassSchemaAndCleanupLocked(className)
	return nil
}

func (tae *TimeAwareEngine) ensureClassSchemaAndCleanupLocked(normClass string) {
	schema, exists := tae.GetSchema(normClass)
	if !exists {
		tae.DeclareClass(normClass, []string{"ttl", "marked-for-remove-on-cycle"})
	} else {
		schema.AddAttribute("ttl")
		schema.AddAttribute("marked-for-remove-on-cycle")
	}

	if !tae.registeredCleanupRules[normClass] {
		tae.registeredCleanupRules[normClass] = true

		// Create auto-generated cleanup rule:
		// (p sys-ttl-cleanup-<class> [salience -1000]
		//     <target> (<class> ^marked-for-remove-on-cycle { > 0 = <c> })
		//     (cycle ^val <c>)
		//   -->
		//     (remove <target>)
		// )
		rule := model.NewRule("sys-ttl-cleanup-" + normClass)
		rule.SetSalience(-1000)

		ce1 := model.NewPositiveCE(normClass).
			WithElementVariable("target").
			AddTest("marked-for-remove-on-cycle", model.OpGreater, model.NewInt(0)).
			AddEqualTest("marked-for-remove-on-cycle", model.NewVariable("<marked>"))

		ce2 := model.NewPositiveCE("cycle").
			AddEqualTest("val", model.NewVariable("<curr>"))

		ce3 := model.NewTestCE(model.NewEvalTest(model.EvalComparison{
			Left:     model.NewVariable("<curr>"),
			Op:       model.OpGreaterEqual,
			Right:    model.NewVariable("<marked>"),
			HasRight: true,
		}))

		rule.AddCondition(ce1)
		rule.AddCondition(ce2)
		rule.AddCondition(ce3)
		rule.AddAction(model.RemoveAction{TargetElementVar: "target"})

		tae.Engine.AddRule(rule)
	}
}

// Step runs a single Match-Resolve-Act cycle, synchronizing the (cycle ^val <N>) WME.
func (tae *TimeAwareEngine) Step() (bool, error) {
	tae.mu.Lock()
	tae.syncCycleLocked()
	tae.mu.Unlock()

	fired, err := tae.Engine.Step()
	if err != nil || !fired {
		return fired, err
	}

	tae.mu.Lock()
	tae.syncCycleLocked()
	tae.mu.Unlock()

	return fired, nil
}

// Run executes the time-aware execution loop:
// 1. Updates (now ^val <ms>) and (datetime ...)
// 2. Evaluates schedules and materializes active events
// 3. Scans active WMEs and marks expired facts for removal at cycle + 1
// 4. Synchronizes (cycle ^val <cycleCount>) and runs until quiescence, halt, breakpoint, or maxCycles
func (tae *TimeAwareEngine) Run(maxCycles int) (int, error) {
	tae.mu.Lock()
	nowTime := tae.clock()
	nowMs := nowTime.UnixMilli()
	loc := nowTime.Location()

	// 1. Sync now and datetime WMEs
	tae.syncNowAndDatetimeLocked(nowTime)

	// 2. Evaluate schedules
	todayDateStr := nowTime.Format("2006-01-02")
	for _, sched := range tae.schedules {
		startTimeToday, err := model.ParseWallClock(sched.StartTime, time.Date(nowTime.Year(), nowTime.Month(), nowTime.Day(), 0, 0, 0, 0, loc), loc)
		if err != nil {
			continue
		}
		var endTimeToday time.Time
		if sched.IsDuration {
			dur, err := model.ParseDuration(sched.DurationOrEnd)
			if err != nil {
				continue
			}
			endTimeToday = startTimeToday.Add(dur)
		} else {
			endTimeToday, err = model.ParseWallClock(sched.DurationOrEnd, startTimeToday, loc)
			if err != nil {
				continue
			}
		}

		// If current time is within [startTimeToday, endTimeToday)
		if (nowTime.Equal(startTimeToday) || nowTime.After(startTimeToday)) && nowTime.Before(endTimeToday) {
			if sched.LastFiredDate != todayDateStr {
				sched.LastFiredDate = todayDateStr
				tae.Engine.Make(sched.Class, map[string]model.Value{
					"ttl":                        model.NewInt(endTimeToday.UnixMilli()),
					"marked-for-remove-on-cycle": model.NewInt(0),
				})
			}
		}
	}

	// 3. Scan active WMEs and mark expired facts
	var expiredWMEs []*model.WME
	for _, wme := range tae.WorkingMemory().All() {
		c := strings.ToLower(wme.Class)
		if c == "now" || c == "cycle" || c == "datetime" {
			continue
		}
		ttlVal, hasTTL := wme.Get("ttl")
		markedVal, _ := wme.Get("marked-for-remove-on-cycle")

		var ttl int64
		if hasTTL && ttlVal.Type() == model.TypeInteger {
			ttl = ttlVal.Raw().(int64)
		}
		var marked int64
		if markedVal.Type() == model.TypeInteger {
			marked = markedVal.Raw().(int64)
		}

		if ttl > 0 && ttl <= nowMs && marked == 0 {
			expiredWMEs = append(expiredWMEs, wme)
		}
	}

	if len(expiredWMEs) > 0 {
		// Add 1 to current cycle as per specification
		tae.Engine.cycleCount++
		targetCycle := int64(tae.Engine.cycleCount)
		for _, wme := range expiredWMEs {
			tae.Engine.Modify(wme.Timetag, map[string]model.Value{
				"marked-for-remove-on-cycle": model.NewInt(targetCycle),
			})
		}
	}

	tae.syncCycleLocked()
	tae.mu.Unlock()

	// 4. Execution loop
	startCycle := tae.CycleCount()
	for {
		if maxCycles > 0 && (tae.CycleCount()-startCycle) >= maxCycles {
			break
		}

		fired, err := tae.Step()
		if err != nil {
			return tae.CycleCount() - startCycle, err
		}
		if !fired {
			break
		}
	}

	return tae.CycleCount() - startCycle, nil
}

func (tae *TimeAwareEngine) syncNowAndDatetimeLocked(t time.Time) {
	nowMs := t.UnixMilli()

	// Sync now
	nowWMEs := tae.FindWMEsMatching(model.NewPositiveCE("now"))
	if len(nowWMEs) == 0 {
		tae.Engine.Make("now", map[string]model.Value{"val": model.NewInt(nowMs)})
	} else {
		tae.Engine.Modify(nowWMEs[0].Timetag, map[string]model.Value{"val": model.NewInt(nowMs)})
	}

	// Sync datetime
	dowSymbol := strings.ToLower(t.Weekday().String())
	dtAttrs := map[string]model.Value{
		"year":   model.NewInt(int64(t.Year())),
		"month":  model.NewInt(int64(t.Month())),
		"day":    model.NewInt(int64(t.Day())),
		"dow":    model.NewSymbol(dowSymbol),
		"hour":   model.NewInt(int64(t.Hour())),
		"minute": model.NewInt(int64(t.Minute())),
		"second": model.NewInt(int64(t.Second())),
	}
	dtWMEs := tae.FindWMEsMatching(model.NewPositiveCE("datetime"))
	if len(dtWMEs) == 0 {
		tae.Engine.Make("datetime", dtAttrs)
	} else {
		tae.Engine.Modify(dtWMEs[0].Timetag, dtAttrs)
	}
}

func (tae *TimeAwareEngine) syncCycleLocked() {
	currCycle := int64(tae.Engine.CycleCount())
	cycleWMEs := tae.FindWMEsMatching(model.NewPositiveCE("cycle"))
	if len(cycleWMEs) == 0 {
		tae.Engine.Make("cycle", map[string]model.Value{"val": model.NewInt(currCycle)})
	} else {
		// Only modify if cycle value changed
		if val, ok := cycleWMEs[0].Get("val"); !ok || val.Raw().(int64) != currCycle {
			tae.Engine.Modify(cycleWMEs[0].Timetag, map[string]model.Value{"val": model.NewInt(currCycle)})
		}
	}
}

// StartDaemon starts a background ticker daemon that invokes Run(cyclesPerTick) on each tick.
// If stopOnQuiescence is true, the daemon automatically halts when Run() returns 0 fired rules.
func (tae *TimeAwareEngine) StartDaemon(interval time.Duration, cyclesPerTick int, stopOnQuiescence bool) (*Daemon, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("daemon interval must be positive, got %v", interval)
	}
	if cyclesPerTick <= 0 {
		cyclesPerTick = 1
	}

	d := &Daemon{
		tae:              tae,
		interval:         interval,
		cyclesPerTick:    cyclesPerTick,
		stopOnQuiescence: stopOnQuiescence,
		done:             make(chan struct{}),
		stop:             make(chan struct{}),
		running:          true,
	}

	d.wg.Add(1)
	go d.loop()

	return d, nil
}

func (d *Daemon) loop() {
	defer d.wg.Done()
	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
		close(d.done)
	}()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			cycles, err := d.tae.Run(d.cyclesPerTick)
			d.mu.Lock()
			if err != nil {
				d.err = err
				d.mu.Unlock()
				return
			}
			d.totalCycles += cycles
			hook := d.onTick
			d.mu.Unlock()

			if hook != nil {
				hook(cycles)
			}

			if d.stopOnQuiescence && cycles == 0 {
				return
			}
		}
	}
}

// Stop signals the daemon to halt and waits for any active tick cycle to conclude.
func (d *Daemon) Stop() {
	d.stopOnce.Do(func() {
		close(d.stop)
	})
	d.wg.Wait()
}

// Done returns a receive-only channel that closes when the daemon terminates (by Stop, error, or quiescence).
func (d *Daemon) Done() <-chan struct{} {
	return d.done
}

// TotalCycles returns the accumulated number of production cycles executed by the daemon.
func (d *Daemon) TotalCycles() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.totalCycles
}

// Error returns any runtime error encountered during execution ticks.
func (d *Daemon) Error() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// Running reports whether the daemon is actively ticking.
func (d *Daemon) Running() bool {
	select {
	case <-d.done:
		return false
	default:
		return true
	}
}

// SetOnTick registers an optional callback invoked after each tick execution.
func (d *Daemon) SetOnTick(fn func(cycles int)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onTick = fn
}
