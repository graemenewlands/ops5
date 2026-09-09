package engine

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"ops5/pkg/conflict"
	"ops5/pkg/model"
	"ops5/pkg/rete"
	"ops5/pkg/wm"
)

// Engine is the central OPS5 runtime coordinator.
type Engine struct {
	mu           sync.Mutex
	wm           *wm.WorkingMemory
	network      *rete.Network
	conflictSet  *conflict.Set
	rules        []*model.Rule
	ruleCount    int
	cycleCount   int
	halted       bool
	outputWriter io.Writer
	traceEnabled bool
}

// New creates a new Engine instance.
func New() *Engine {
	mem := wm.New()
	net := rete.NewNetwork()
	cs := conflict.NewSet()

	// Connect WM events to Rete Alpha Network
	mem.AddListener(net)

	return &Engine{
		wm:           mem,
		network:      net,
		conflictSet:  cs,
		rules:        make([]*model.Rule, 0),
		ruleCount:    0,
		cycleCount:   0,
		halted:       false,
		outputWriter: os.Stdout,
		traceEnabled: false,
	}
}

// SetOutputWriter configures where WRITE actions emit output.
func (e *Engine) SetOutputWriter(w io.Writer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outputWriter = w
}

// SetTrace enables or disables cycle execution tracing.
func (e *Engine) SetTrace(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traceEnabled = enabled
}

// SetStrategy sets the conflict resolution strategy (LEX or MEA).
func (e *Engine) SetStrategy(strategy conflict.StrategyType) {
	e.conflictSet.SetStrategy(strategy)
}

// WorkingMemory returns the underlying working memory manager.
func (e *Engine) WorkingMemory() *wm.WorkingMemory {
	return e.wm
}

// ConflictSet returns the conflict set manager.
func (e *Engine) ConflictSet() *conflict.Set {
	return e.conflictSet
}

// AddRule registers a rule with the engine and compiles it into the Rete network.
func (e *Engine) AddRule(rule *model.Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ruleCount++
	rule.Index = e.ruleCount
	e.rules = append(e.rules, rule)
	existingWMEs := e.wm.All()
	e.network.AddRuleWithWMEs(rule, e.conflictSet, existingWMEs)
}

// Make asserts a new WME.
func (e *Engine) Make(class string, attrs map[string]model.Value) *model.WME {
	return e.wm.Make(class, attrs)
}

// Remove retracts a WME by timetag.
func (e *Engine) Remove(timetag int64) (*model.WME, error) {
	return e.wm.Remove(timetag)
}

// Modify updates an existing WME.
func (e *Engine) Modify(timetag int64, attrs map[string]model.Value) (*model.WME, error) {
	return e.wm.Modify(timetag, attrs)
}

// ResolveValue substitutes variable placeholders using token bindings.
func resolveValue(val model.Value, bindings map[string]model.Value) model.Value {
	if val.IsVariable() {
		vName := val.VariableName()
		if bound, ok := bindings[vName]; ok {
			return bound
		}
	}
	return val
}

// resolveTargetTimetag locates the WME timetag referenced by element variable or index.
func resolveTargetTimetag(act *conflict.Activation, elemVar string, index int) (int64, error) {
	if elemVar != "" {
		vName := strings.TrimPrefix(strings.TrimSuffix(elemVar, ">"), "<")
		if bound, ok := act.Token.Bindings[vName]; ok {
			if bound.Type() == model.TypeInteger {
				return bound.Raw().(int64), nil
			}
		}
		return 0, fmt.Errorf("element variable <%s> not bound to a valid timetag", elemVar)
	}

	wmes := act.Token.WMEs()
	if index >= 1 && index <= len(wmes) {
		return wmes[index-1].Timetag, nil
	}

	return 0, fmt.Errorf("invalid condition element index %d (token has %d WMEs)", index, len(wmes))
}

// Step runs a single Match-Resolve-Act cycle.
// Returns (true, nil) if a rule fired, (false, nil) if conflict set is empty or halted.
func (e *Engine) Step() (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.halted {
		return false, nil
	}

	dominant, ok := e.conflictSet.SelectDominant()
	if !ok {
		// Conflict set empty -> quiescence reached
		return false, nil
	}

	// Refract activation so it won't fire again for the same WMEs
	e.conflictSet.MarkFired(dominant)
	e.cycleCount++

	if e.traceEnabled && e.outputWriter != nil {
		fmt.Fprintf(e.outputWriter, "[Cycle %d] Fired rule '%s' with WMEs %v\n", e.cycleCount, dominant.Rule.Name, dominant.Timetags)
	}

	// Execute RHS actions
	for _, action := range dominant.Rule.Actions {
		switch act := action.(type) {
		case model.MakeAction:
			resolvedAttrs := make(map[string]model.Value, len(act.Attributes))
			for k, v := range act.Attributes {
				resolvedAttrs[k] = resolveValue(v, dominant.Token.Bindings)
			}
			e.wm.Make(act.Class, resolvedAttrs)

		case model.ModifyAction:
			targetTimetag, err := resolveTargetTimetag(dominant, act.TargetElementVar, act.TargetIndex)
			if err != nil {
				return true, err
			}
			resolvedAttrs := make(map[string]model.Value, len(act.Attributes))
			for k, v := range act.Attributes {
				resolvedAttrs[k] = resolveValue(v, dominant.Token.Bindings)
			}
			_, err = e.wm.Modify(targetTimetag, resolvedAttrs)
			if err != nil {
				return true, err
			}

		case model.RemoveAction:
			targetTimetag, err := resolveTargetTimetag(dominant, act.TargetElementVar, act.TargetIndex)
			if err != nil {
				return true, err
			}
			_, err = e.wm.Remove(targetTimetag)
			if err != nil {
				return true, err
			}

		case model.WriteAction:
			if e.outputWriter != nil {
				var parts []string
				for _, item := range act.Items {
					resolved := resolveValue(item, dominant.Token.Bindings)
					if resolved.Type() == model.TypeString {
						parts = append(parts, resolved.Raw().(string))
					} else {
						parts = append(parts, resolved.String())
					}
				}
				fmt.Fprintln(e.outputWriter, strings.Join(parts, " "))
			}

		case model.HaltAction:
			e.halted = true
			return true, nil

		case model.CustomAction:
			if act.Execute != nil {
				if err := act.Execute(dominant); err != nil {
					return true, err
				}
			}
		}
	}

	return true, nil
}

// Run executes cycles until quiescence, halt, or maxCycles limit.
// If maxCycles <= 0, runs until quiescence or halt.
// Returns the number of cycles executed.
func (e *Engine) Run(maxCycles int) (int, error) {
	startCycle := e.cycleCount
	for {
		if maxCycles > 0 && (e.cycleCount-startCycle) >= maxCycles {
			break
		}

		fired, err := e.Step()
		if err != nil {
			return e.cycleCount - startCycle, err
		}
		if !fired {
			break
		}
	}

	return e.cycleCount - startCycle, nil
}

// CycleCount returns total cycles executed.
func (e *Engine) CycleCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cycleCount
}

// IsHalted returns true if the engine was halted by a halt action.
func (e *Engine) IsHalted() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.halted
}
