package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
	"ops5/pkg/model"
	"ops5/pkg/parser"
)

// WMEAssertion defines a working memory element to assert or verify in a test case.
type WMEAssertion struct {
	Class      string            `json:"class"`
	Attributes map[string]string `json:"attributes"`
}

// TestCase represents a self-contained OPS5 test scenario.
type TestCase struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	Strategy        string         `json:"strategy,omitempty"` // "LEX" or "MEA"
	Source          string         `json:"source,omitempty"`   // OPS5 rule definitions
	SourceFile      string         `json:"source_file,omitempty"`
	InitialWM       []WMEAssertion `json:"initial_wm,omitempty"`
	MaxCycles       int            `json:"max_cycles,omitempty"`
	ExpectedCycles  int            `json:"expected_cycles,omitempty"`
	ExpectedWM      []WMEAssertion `json:"expected_wm,omitempty"`
	ForbiddenWM     []WMEAssertion `json:"forbidden_wm,omitempty"`
	ExpectedOutputs []string       `json:"expected_outputs,omitempty"`
}

// Result records the outcome of executing a TestCase.
type Result struct {
	TestCaseName string
	Passed       bool
	CyclesRan    int
	Error        error
	Output       string
}

// Runner executes test cases against the OPS5 engine.
type Runner struct{}

// NewRunner creates a new test runner.
func NewRunner() *Runner {
	return &Runner{}
}

// LoadTestCaseFromJSON loads a test case from a JSON file.
func (r *Runner) LoadTestCaseFromJSON(filePath string) (*TestCase, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test file %s: %w", filePath, err)
	}

	var tc TestCase
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON test case %s: %w", filePath, err)
	}

	// Resolve SourceFile path relative to the test JSON file if needed
	if tc.SourceFile != "" {
		if _, err := os.Stat(tc.SourceFile); err != nil {
			baseDir := filepath.Dir(filePath)
			candidate1 := filepath.Join(baseDir, tc.SourceFile)
			if _, err1 := os.Stat(candidate1); err1 == nil {
				tc.SourceFile = candidate1
			} else {
				candidate2 := filepath.Join(baseDir, filepath.Base(tc.SourceFile))
				if _, err2 := os.Stat(candidate2); err2 == nil {
					tc.SourceFile = candidate2
				}
			}
		}
	}

	return &tc, nil
}

// Run executes a single TestCase.
func (r *Runner) Run(tc *TestCase) *Result {
	eng := engine.New()
	var outBuf bytes.Buffer
	eng.SetOutputWriter(&outBuf)

	// Set strategy
	if strings.ToUpper(tc.Strategy) == "MEA" {
		eng.SetStrategy(conflict.StrategyMEA)
	} else {
		eng.SetStrategy(conflict.StrategyLEX)
	}

	// Load source code
	source := tc.Source
	if tc.SourceFile != "" {
		fileBytes, err := os.ReadFile(tc.SourceFile)
		if err != nil {
			return &Result{
				TestCaseName: tc.Name,
				Passed:       false,
				Error:        fmt.Errorf("error reading source file %s: %w", tc.SourceFile, err),
			}
		}
		source = string(fileBytes)
	}

	// Parse and register rules and inline makes from source
	if strings.TrimSpace(source) != "" {
		p, err := parser.NewParser(source)
		if err != nil {
			return &Result{
				TestCaseName: tc.Name,
				Passed:       false,
				Error:        fmt.Errorf("syntax/parser initialization error: %w", err),
			}
		}

		// Parse rules and top-level makes
		for {
			rule, err := p.ParseRule()
			if err == nil && rule != nil {
				eng.AddRule(rule)
				continue
			}

			// If not a rule, attempt make statement
			class, attrs, makeErr := p.ParseMake()
			if makeErr == nil && class != "" {
				eng.Make(class, attrs)
				continue
			}

			// Reached end or cannot parse further
			break
		}
	}

	// Assert initial WM
	for _, w := range tc.InitialWM {
		attrs := make(map[string]model.Value, len(w.Attributes))
		for k, v := range w.Attributes {
			attrs[k] = model.AutoValue(v)
		}
		eng.Make(w.Class, attrs)
	}

	// Run engine
	maxCycles := tc.MaxCycles
	if maxCycles <= 0 {
		maxCycles = 1000
	}

	cycles, err := eng.Run(maxCycles)
	outStr := outBuf.String()
	if err != nil {
		return &Result{
			TestCaseName: tc.Name,
			Passed:       false,
			CyclesRan:    cycles,
			Output:       outStr,
			Error:        fmt.Errorf("runtime execution error: %w", err),
		}
	}

	// Verify cycle count if expected
	if tc.ExpectedCycles > 0 && cycles != tc.ExpectedCycles {
		return &Result{
			TestCaseName: tc.Name,
			Passed:       false,
			CyclesRan:    cycles,
			Output:       outStr,
			Error:        fmt.Errorf("expected %d cycles, got %d", tc.ExpectedCycles, cycles),
		}
	}

	// Verify expected WMEs exist
	allWmes := eng.WorkingMemory().All()
	for _, exp := range tc.ExpectedWM {
		found := false
		for _, w := range allWmes {
			if w.Class != exp.Class {
				continue
			}
			match := true
			for k, expectedValStr := range exp.Attributes {
				actualVal, ok := w.Get(k)
				if !ok || !actualVal.Equal(model.AutoValue(expectedValStr)) {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
		if !found {
			return &Result{
				TestCaseName: tc.Name,
				Passed:       false,
				CyclesRan:    cycles,
				Output:       outStr,
				Error:        fmt.Errorf("expected WME missing: class=%s attrs=%v", exp.Class, exp.Attributes),
			}
		}
	}

	// Verify forbidden WMEs do NOT exist
	for _, forb := range tc.ForbiddenWM {
		for _, w := range allWmes {
			if w.Class != forb.Class {
				continue
			}
			match := true
			for k, expectedValStr := range forb.Attributes {
				actualVal, ok := w.Get(k)
				if !ok || !actualVal.Equal(model.AutoValue(expectedValStr)) {
					match = false
					break
				}
			}
			if match {
				return &Result{
					TestCaseName: tc.Name,
					Passed:       false,
					CyclesRan:    cycles,
					Output:       outStr,
					Error:        fmt.Errorf("forbidden WME found in working memory: class=%s attrs=%v", forb.Class, forb.Attributes),
				}
			}
		}
	}

	// Verify expected outputs
	for _, expectedText := range tc.ExpectedOutputs {
		if !strings.Contains(outStr, expectedText) {
			return &Result{
				TestCaseName: tc.Name,
				Passed:       false,
				CyclesRan:    cycles,
				Output:       outStr,
				Error:        fmt.Errorf("expected output substring %q not found in stdout: %q", expectedText, outStr),
			}
		}
	}

	return &Result{
		TestCaseName: tc.Name,
		Passed:       true,
		CyclesRan:    cycles,
		Output:       outStr,
		Error:        nil,
	}
}
