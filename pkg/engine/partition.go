package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/wm"
)

// PartitionRouter determines which partition(s) should receive an asserted WME.
// Returns a slice of target partition IDs, or []string{"*"} for all partitions.
// If empty or nil, the WME remains purely local to the originating partition.
type PartitionRouter func(originPartition string, class string, attrs map[string]model.Value) []string

// PartitionedEngine coordinates multiple independent Rete execution partitions (ParaOPS5).
type PartitionedEngine struct {
	mu               sync.RWMutex
	partitions       map[string]*Partition
	router           PartitionRouter
	globalRules      []*model.Rule
	globalSchemas    map[string]*model.ClassSchema
	activeWorkers    atomic.Int32
	inFlightMessages atomic.Int64
	running          atomic.Bool
}

// Partition represents an isolated Rete execution partition with its own
// WorkingMemory, Rete network, conflict agenda, and execution loop.
type Partition struct {
	ID          string
	Engine      *Engine
	pe          *PartitionedEngine
	inbox       chan partitionEvent
	cycleCount  atomic.Int64
	halted      atomic.Bool
	isImporting atomic.Bool
}

type partitionEvent struct {
	originPartition string
	isAssert        bool
	class           string
	attrs           map[string]model.Value
}

// NewPartitionedEngine creates a new ParaOPS5 partitioned engine coordinator.
func NewPartitionedEngine(router PartitionRouter) *PartitionedEngine {
	return &PartitionedEngine{
		partitions:    make(map[string]*Partition),
		router:        router,
		globalRules:   make([]*model.Rule, 0),
		globalSchemas: make(map[string]*model.ClassSchema),
	}
}

// SetRouter updates the cross-partition WME message router.
func (pe *PartitionedEngine) SetRouter(router PartitionRouter) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.router = router
}

// AddPartition creates and registers a new isolated partition with its own Engine.
func (pe *PartitionedEngine) AddPartition(id string) (*Partition, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, exists := pe.partitions[id]; exists {
		return nil, fmt.Errorf("partition %q already exists", id)
	}

	eng := New()
	// Copy global schemas
	for _, schema := range pe.globalSchemas {
		eng.DeclareClass(schema.Class, schema.Attributes)
	}
	// Copy global rules
	for _, rule := range pe.globalRules {
		eng.AddRule(rule)
	}

	part := &Partition{
		ID:     id,
		Engine: eng,
		pe:     pe,
		inbox:  make(chan partitionEvent, 2048),
	}

	// Register partitionListener to intercept assertions and route them
	eng.WorkingMemory().AddListener(&partitionListener{partition: part})

	pe.partitions[id] = part
	return part, nil
}

// GetPartition returns the partition with the specified ID.
func (pe *PartitionedEngine) GetPartition(id string) (*Partition, bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	p, ok := pe.partitions[id]
	return p, ok
}

// Partitions returns a slice of all registered partitions.
func (pe *PartitionedEngine) Partitions() []*Partition {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	res := make([]*Partition, 0, len(pe.partitions))
	for _, p := range pe.partitions {
		res = append(res, p)
	}
	return res
}

// DeclareClass registers an element class schema across all current and future partitions.
func (pe *PartitionedEngine) DeclareClass(name string, attrs []string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	schema := model.NewClassSchema(name, attrs)
	pe.globalSchemas[name] = schema
	for _, p := range pe.partitions {
		p.Engine.DeclareClass(name, attrs)
	}
}

// AddRule registers a global production rule to all current and future partitions.
func (pe *PartitionedEngine) AddRule(rule *model.Rule) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.globalRules = append(pe.globalRules, rule)
	for _, p := range pe.partitions {
		p.Engine.AddRule(rule)
	}
}

// AddRuleToPartition registers a production rule to a specific partition.
func (pe *PartitionedEngine) AddRuleToPartition(partitionID string, rule *model.Rule) error {
	pe.mu.RLock()
	p, ok := pe.partitions[partitionID]
	pe.mu.RUnlock()

	if !ok {
		return fmt.Errorf("partition %q not found", partitionID)
	}
	p.Engine.AddRule(rule)
	return nil
}

// MakeInPartition asserts a WME directly into a specific partition.
func (pe *PartitionedEngine) MakeInPartition(partitionID string, class string, attrs map[string]model.Value) (*model.WME, error) {
	pe.mu.RLock()
	p, ok := pe.partitions[partitionID]
	pe.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("partition %q not found", partitionID)
	}
	return p.Engine.Make(class, attrs), nil
}

// Make routes a WME assertion using the configured PartitionRouter.
func (pe *PartitionedEngine) Make(originPartition string, class string, attrs map[string]model.Value) ([]*model.WME, error) {
	pe.mu.RLock()
	router := pe.router
	pe.mu.RUnlock()

	if router == nil {
		if originPartition != "" {
			w, err := pe.MakeInPartition(originPartition, class, attrs)
			if err != nil {
				return nil, err
			}
			return []*model.WME{w}, nil
		}
		return nil, fmt.Errorf("no router configured and no origin partition specified")
	}

	targets := router(originPartition, class, attrs)
	if len(targets) == 0 {
		if originPartition != "" {
			w, err := pe.MakeInPartition(originPartition, class, attrs)
			if err != nil {
				return nil, err
			}
			return []*model.WME{w}, nil
		}
		return nil, nil
	}

	var asserted []*model.WME
	for _, target := range targets {
		if target == "*" {
			pe.mu.RLock()
			for _, p := range pe.partitions {
				w := p.Engine.Make(class, attrs)
				asserted = append(asserted, w)
			}
			pe.mu.RUnlock()
		} else {
			w, err := pe.MakeInPartition(target, class, attrs)
			if err != nil {
				return nil, err
			}
			asserted = append(asserted, w)
		}
	}
	return asserted, nil
}

// StepAll executes a single Match-Resolve-Act cycle across all partitions concurrently.
// Returns a map indicating which partitions fired a rule.
func (pe *PartitionedEngine) StepAll() (map[string]bool, error) {
	pe.mu.RLock()
	parts := make([]*Partition, 0, len(pe.partitions))
	for _, p := range pe.partitions {
		parts = append(parts, p)
	}
	pe.mu.RUnlock()

	type stepResult struct {
		id    string
		fired bool
		err   error
	}

	resCh := make(chan stepResult, len(parts))
	var wg sync.WaitGroup

	for _, p := range parts {
		wg.Add(1)
		go func(part *Partition) {
			defer wg.Done()
			part.drainInbox()
			fired, err := part.Engine.Step()
			if fired {
				part.cycleCount.Add(1)
			}
			resCh <- stepResult{id: part.ID, fired: fired, err: err}
		}(p)
	}

	wg.Wait()
	close(resCh)

	firedMap := make(map[string]bool)
	for res := range resCh {
		if res.err != nil {
			return nil, res.err
		}
		firedMap[res.id] = res.fired
	}
	return firedMap, nil
}

// RunParallel executes all partitions concurrently using worker goroutines until
// global quiescence is reached across all partitions or maxCycles is hit.
func (pe *PartitionedEngine) RunParallel(ctx context.Context, maxCyclesPerPartition int) (map[string]int, error) {
	pe.mu.RLock()
	parts := make([]*Partition, 0, len(pe.partitions))
	for _, p := range pe.partitions {
		parts = append(parts, p)
	}
	pe.mu.RUnlock()

	if len(parts) == 0 {
		return map[string]int{}, nil
	}

	pe.running.Store(true)
	defer pe.running.Store(false)

	var wg sync.WaitGroup
	errCh := make(chan error, len(parts))
	cycles := sync.Map{}

	for _, p := range parts {
		wg.Add(1)
		go func(part *Partition) {
			defer wg.Done()
			c, err := part.runLoop(ctx, maxCyclesPerPartition)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
			cycles.Store(part.ID, c)
		}(p)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	result := make(map[string]int)
	cycles.Range(func(key, value any) bool {
		result[key.(string)] = value.(int)
		return true
	})

	return result, nil
}

// IsQuiescent reports whether all partitions are idle and all in-flight messages are processed.
func (pe *PartitionedEngine) IsQuiescent() bool {
	if pe.activeWorkers.Load() > 0 {
		return false
	}
	if pe.inFlightMessages.Load() > 0 {
		return false
	}
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.partitions {
		if len(p.inbox) > 0 {
			return false
		}
		if p.Engine.ConflictSet().Count() > 0 {
			return false
		}
	}
	return true
}

// Halt signals all partitions to stop execution.
func (pe *PartitionedEngine) Halt() {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.partitions {
		p.Halt()
	}
}

// LoadScript parses and compiles rules from an OPS5 source string into all partitions.
func (pe *PartitionedEngine) LoadScript(script string) error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.partitions {
		if err := p.Engine.LoadScript(script); err != nil {
			return err
		}
	}
	return nil
}

// LoadScriptToPartition parses and compiles rules into a specific partition.
func (pe *PartitionedEngine) LoadScriptToPartition(partitionID string, script string) error {
	pe.mu.RLock()
	part, ok := pe.partitions[partitionID]
	pe.mu.RUnlock()
	if !ok {
		return fmt.Errorf("partition %q not found", partitionID)
	}
	return part.Engine.LoadScript(script)
}

// CycleCount returns the number of rule firings executed in this partition.
func (p *Partition) CycleCount() int64 {
	return p.cycleCount.Load()
}

// IsHalted returns true if this partition has halted.
func (p *Partition) IsHalted() bool {
	return p.halted.Load() || p.Engine.IsHalted()
}

// Halt signals this partition to halt execution.
func (p *Partition) Halt() {
	p.halted.Store(true)
	p.Engine.Halt()
}

// WorkingMemory returns the working memory store of this partition.
func (p *Partition) WorkingMemory() *wm.WorkingMemory {
	return p.Engine.WorkingMemory()
}

// ConflictSet returns the conflict agenda of this partition.
func (p *Partition) ConflictSet() *conflict.Set {
	return p.Engine.ConflictSet()
}

func (p *Partition) sendEvent(evt partitionEvent) {
	p.inbox <- evt
}

func (p *Partition) drainInbox() int {
	drained := 0
	for {
		select {
		case evt := <-p.inbox:
			p.isImporting.Store(true)
			if evt.isAssert {
				p.Engine.Make(evt.class, evt.attrs)
			}
			p.isImporting.Store(false)
			p.pe.inFlightMessages.Add(-1)
			drained++
		default:
			return drained
		}
	}
}

func (p *Partition) runLoop(ctx context.Context, maxCycles int) (int, error) {
	quiescentChecks := 0
	for {
		select {
		case <-ctx.Done():
			return int(p.cycleCount.Load()), ctx.Err()
		default:
		}

		if p.IsHalted() {
			return int(p.cycleCount.Load()), nil
		}

		if maxCycles > 0 && int(p.cycleCount.Load()) >= maxCycles {
			return int(p.cycleCount.Load()), nil
		}

		drained := p.drainInbox()

		p.pe.activeWorkers.Add(1)
		fired, err := p.Engine.Step()
		p.pe.activeWorkers.Add(-1)

		if err != nil {
			return int(p.cycleCount.Load()), err
		}

		if fired {
			p.cycleCount.Add(1)
			quiescentChecks = 0
			continue
		}

		if drained > 0 {
			quiescentChecks = 0
			continue
		}

		// Idle: check global quiescence
		if p.pe.IsQuiescent() {
			quiescentChecks++
			if quiescentChecks >= 3 {
				// Stable global quiescence confirmed across all partitions
				return int(p.cycleCount.Load()), nil
			}
			time.Sleep(50 * time.Microsecond)
		} else {
			quiescentChecks = 0
			time.Sleep(20 * time.Microsecond)
		}
	}
}

// partitionListener intercepts working memory assertions in a partition and routes
// cross-partition events to target partitions according to the PartitionRouter.
type partitionListener struct {
	partition *Partition
}

func (pl *partitionListener) OnAssert(wme *model.WME) {
	p := pl.partition
	if p.isImporting.Load() {
		return
	}
	pe := p.pe
	if pe == nil {
		return
	}

	pe.mu.RLock()
	router := pe.router
	pe.mu.RUnlock()

	if router == nil {
		return
	}

	targets := router(p.ID, wme.Class, wme.Attributes)
	if len(targets) == 0 {
		return
	}

	for _, target := range targets {
		if target == "*" {
			// Broadcast to all other partitions
			pe.mu.RLock()
			for id, other := range pe.partitions {
				if id != p.ID {
					pe.inFlightMessages.Add(1)
					other.sendEvent(partitionEvent{
						originPartition: p.ID,
						isAssert:        true,
						class:           wme.Class,
						attrs:           wme.Attributes,
					})
				}
			}
			pe.mu.RUnlock()
		} else if target != p.ID {
			pe.mu.RLock()
			dest, ok := pe.partitions[target]
			pe.mu.RUnlock()
			if ok {
				pe.inFlightMessages.Add(1)
				dest.sendEvent(partitionEvent{
					originPartition: p.ID,
					isAssert:        true,
					class:           wme.Class,
					attrs:           wme.Attributes,
				})
			}
		}
	}
}

func (pl *partitionListener) OnRetract(wme *model.WME) {
	// Retraction routing
}
