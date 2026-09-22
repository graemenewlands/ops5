package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
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
	eng.SetTraceWriter(&outBuf)

	defer eng.CloseAllFiles()

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

		// Parse rules, top-level makes, and literalize directives
		for {
			stmt, err := p.NextStatement()
			if err != nil {
				return &Result{
					TestCaseName: tc.Name,
					Passed:       false,
					Error:        fmt.Errorf("syntax error in source: %w", err),
				}
			}
			if stmt == nil {
				break
			}

			switch stmt.Type {
			case parser.StmtRule:
				eng.AddRule(stmt.Rule)
			case parser.StmtMake:
				if stmt.Schema != nil {
					if _, ok := eng.GetSchema(stmt.Schema.Class); !ok {
						eng.DeclareClass(stmt.Schema.Class, stmt.Schema.Attributes)
					}
				}
				eng.Make(stmt.MakeClass, stmt.MakeAttributes)
			case parser.StmtLiteralize:
				eng.DeclareClass(stmt.LiteralizeClass, stmt.LiteralizeAttrs)
			case parser.StmtVectorAttribute:
				for _, attr := range stmt.VectorAttrs {
					eng.DeclareVectorAttribute(attr)
				}
			case parser.StmtOpenFile:
				filespec := stmt.OpenFile.Filespec.String()
				if stmt.OpenFile.Filespec.Type() == model.TypeString {
					filespec = stmt.OpenFile.Filespec.Raw().(string)
				}
				if err := eng.OpenFile(stmt.OpenFile.LogicalName, filespec, stmt.OpenFile.Mode); err != nil {
					return &Result{
						TestCaseName: tc.Name,
						Passed:       false,
						Error:        err,
					}
				}
			case parser.StmtCloseFile:
				if err := eng.CloseFile(stmt.CloseFile.LogicalName); err != nil {
					return &Result{
						TestCaseName: tc.Name,
						Passed:       false,
						Error:        err,
					}
				}
			case parser.StmtDefault:
				if err := eng.SetDefault(stmt.Default.LogicalName, stmt.Default.Subsystem); err != nil {
					return &Result{
						TestCaseName: tc.Name,
						Passed:       false,
						Error:        err,
					}
				}
			case parser.StmtExcise:
				for _, name := range stmt.ExciseRules {
					eng.ExciseRule(name)
				}
			case parser.StmtPM:
				if len(stmt.PMRules) == 0 || (len(stmt.PMRules) == 1 && stmt.PMRules[0] == "*") {
					for _, rule := range eng.Rules() {
						fmt.Fprintln(&outBuf, rule.String())
					}
				} else {
					for _, name := range stmt.PMRules {
						if rule := eng.Rule(name); rule != nil {
							fmt.Fprintln(&outBuf, rule.String())
						} else {
							fmt.Fprintf(&outBuf, "Rule '%s' not found\n", name)
						}
					}
				}
			case parser.StmtRemove:
				if stmt.RemoveWildcard {
					eng.RemoveAll()
				} else {
					for _, tag := range stmt.RemoveTimetags {
						eng.Remove(tag)
					}
				}
			case parser.StmtWatch:
				if stmt.WatchLevel == nil {
					fmt.Fprintf(eng.TraceWriter(), "Current watch level: %d\n", eng.WatchLevel())
				} else {
					_ = eng.SetWatchLevel(*stmt.WatchLevel)
				}
			case parser.StmtPPWM:
				matches := eng.FindWMEsMatching(stmt.PPWMPattern)
				for _, w := range matches {
					fmt.Fprintln(&outBuf, w.String())
				}
			case parser.StmtStrategy:
				if strings.ToUpper(stmt.Strategy) == "MEA" {
					eng.SetStrategy(conflict.StrategyMEA)
				} else if strings.ToUpper(stmt.Strategy) == "LEX" {
					eng.SetStrategy(conflict.StrategyLEX)
				}
			case parser.StmtSubstr:
				val := eng.EvaluateSubstr(stmt.Substr, nil)
				fmt.Fprintln(&outBuf, val.String())
			case parser.StmtMatches:
				fmt.Fprint(&outBuf, eng.FormatMatches(stmt.MatchesRules...))
			case parser.StmtPBreak:
				for _, name := range stmt.PBreakRules {
					eng.SetBreakpoint(name)
				}
			case parser.StmtUnpbreak:
				if len(stmt.UnpbreakRules) == 0 || stmt.UnpbreakRules[0] == "*" || strings.ToLower(stmt.UnpbreakRules[0]) == "nil" {
					eng.ClearBreakpoints()
				} else {
					for _, name := range stmt.UnpbreakRules {
						eng.RemoveBreakpoint(name)
					}
				}
			case parser.StmtDOT:
				if stmt.DOTFile == "" {
					_ = eng.ExportDOT(&outBuf)
				} else {
					f, err := os.Create(stmt.DOTFile)
					if err == nil {
						_ = eng.ExportDOT(f)
						f.Close()
					}
				}
			}
		}
	}

	// Assert initial WM
	for _, w := range tc.InitialWM {
		attrs := make(map[string]model.Value, len(w.Attributes))
		for k, v := range w.Attributes {
			normK := model.NormalizeAttribute(k)
			vals := parseVectorString(v)
			if eng.IsVectorAttribute(normK) || len(vals) > 1 {
				attrs[normK] = model.NewVector(vals)
			} else if len(vals) == 1 {
				attrs[normK] = vals[0]
			} else {
				attrs[normK] = model.AutoValue(v)
			}
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
				if !ok || !matchAttributeValue(actualVal, expectedValStr) {
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
				if !ok || !matchAttributeValue(actualVal, expectedValStr) {
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

func parseVectorString(s string) []model.Value {
	l := parser.NewLexer(s)
	var vals []model.Value
	for {
		tok, err := l.NextToken()
		if err != nil || tok.Type == parser.TokenEOF {
			break
		}
		vals = append(vals, parser.TokenToValue(tok))
	}
	return vals
}

func matchAttributeValue(actual model.Value, expectedStr string) bool {
	if actual.Equal(model.AutoValue(expectedStr)) {
		return true
	}
	if actual.IsVector() {
		if actual.String() == expectedStr {
			return true
		}
		expectedElems := parseVectorString(expectedStr)
		elems := actual.VectorElements()
		if len(expectedElems) == len(elems) {
			match := true
			for i := range expectedElems {
				if !elems[i].Equal(expectedElems[i]) {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
