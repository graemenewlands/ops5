package engine

import (
	"fmt"
	"io"
	"os"
	"sort"
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
	schemas      map[string]*model.ClassSchema
	vectorAttrs  map[string]bool
	ruleCount    int
	cycleCount   int
	halted       bool
	outputWriter io.Writer
	traceEnabled bool
	currentCol   int
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
		schemas:      make(map[string]*model.ClassSchema),
		vectorAttrs:  make(map[string]bool),
		ruleCount:    0,
		cycleCount:   0,
		halted:       false,
		outputWriter: os.Stdout,
		traceEnabled: false,
		currentCol:   1,
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

// DeclareVectorAttribute registers an attribute name as a vector-attribute.
func (e *Engine) DeclareVectorAttribute(attr string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	norm := model.NormalizeAttribute(attr)
	if norm == "" {
		return
	}
	e.vectorAttrs[norm] = true
	for _, s := range e.schemas {
		if s.HasAttribute(norm) {
			s.SetVectorAttribute(norm, true)
		}
	}
}

// IsVectorAttribute checks if an attribute is declared as a vector-attribute.
func (e *Engine) IsVectorAttribute(attr string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.vectorAttrs[model.NormalizeAttribute(attr)]
}

// VectorAttributes returns a list of all declared vector attributes.
func (e *Engine) VectorAttributes() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	res := make([]string, 0, len(e.vectorAttrs))
	for a := range e.vectorAttrs {
		res = append(res, a)
	}
	sort.Strings(res)
	return res
}

// DeclareClass registers a schema for a class name from a literalize directive.
func (e *Engine) DeclareClass(class string, attributes []string) *model.ClassSchema {
	e.mu.Lock()
	defer e.mu.Unlock()

	schema := model.NewClassSchema(class, attributes)
	for a := range e.vectorAttrs {
		if schema.HasAttribute(a) {
			schema.SetVectorAttribute(a, true)
		}
	}
	e.schemas[schema.Class] = schema
	return schema
}

// GetSchema retrieves a class schema by class name.
func (e *Engine) GetSchema(class string) (*model.ClassSchema, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	schema, ok := e.schemas[strings.ToLower(class)]
	return schema, ok
}

// Schemas returns a slice of all registered class schemas.
func (e *Engine) Schemas() []*model.ClassSchema {
	e.mu.Lock()
	defer e.mu.Unlock()

	res := make([]*model.ClassSchema, 0, len(e.schemas))
	for _, s := range e.schemas {
		res = append(res, s)
	}
	return res
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

// resolveValue substitutes variable placeholders using token bindings.
func resolveValue(val model.Value, bindings map[string]model.Value) model.Value {
	if val.IsVariable() {
		vName := val.VariableName()
		if bound, ok := bindings[vName]; ok {
			return bound
		}
	}
	if val.IsVector() {
		elems := val.VectorElements()
		resolved := make([]model.Value, len(elems))
		for i, el := range elems {
			resolved[i] = resolveValue(el, bindings)
		}
		return model.NewVector(resolved)
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
				hasCRLF := false
				for _, arg := range act.Args {
					if arg.Type == model.WriteArgCRLF {
						hasCRLF = true
						break
					}
				}

				lastWasSpaceOrTab := (e.currentCol == 1)
				for _, arg := range act.Args {
					switch arg.Type {
					case model.WriteArgCRLF:
						fmt.Fprint(e.outputWriter, "\n")
						e.currentCol = 1
						lastWasSpaceOrTab = true

					case model.WriteArgTabTo:
						targetCol := 1
						resolvedCol := resolveValue(arg.Value, dominant.Token.Bindings)
						if resolvedCol.Type() == model.TypeInteger {
							targetCol = int(resolvedCol.Raw().(int64))
						} else if resolvedCol.Type() == model.TypeFloat {
							targetCol = int(resolvedCol.Raw().(float64))
						}
						if targetCol < 1 {
							targetCol = 1
						}

						if targetCol > e.currentCol {
							spaces := strings.Repeat(" ", targetCol-e.currentCol)
							fmt.Fprint(e.outputWriter, spaces)
							e.currentCol = targetCol
						} else {
							fmt.Fprint(e.outputWriter, " ")
							e.currentCol++
						}
						lastWasSpaceOrTab = true

					case model.WriteArgValue:
						resolved := resolveValue(arg.Value, dominant.Token.Bindings)
						var text string
						if resolved.Type() == model.TypeString {
							text = resolved.Raw().(string)
						} else {
							text = resolved.String()
						}

						if !lastWasSpaceOrTab {
							fmt.Fprint(e.outputWriter, " ")
							e.currentCol++
						}

						fmt.Fprint(e.outputWriter, text)
						for _, r := range text {
							if r == '\n' {
								e.currentCol = 1
							} else {
								e.currentCol++
							}
						}
						lastWasSpaceOrTab = false
					}
				}

				if !hasCRLF {
					fmt.Fprint(e.outputWriter, "\n")
					e.currentCol = 1
				}
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
