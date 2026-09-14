package engine

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"ops5/pkg/conflict"
	"ops5/pkg/model"
	"ops5/pkg/parser"
	"ops5/pkg/rete"
	"ops5/pkg/wm"
)

type openFileEntry struct {
	file   *os.File
	reader *bufio.Reader
	writer io.Writer
	mode   string // "in", "out", "append"
}

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
	outputWriter        io.Writer
	traceWriter         io.Writer
	watchLevel          int
	traceEnabled        bool
	currentCol          int
	lastAddedTimetag    int64
	genatomCounter      int64
	attrIndices         map[string]int
	currentActivation   *conflict.Activation

	// File I/O subsystem
	openFiles           map[string]*openFileEntry
	defaultAcceptStream string // logical name or "" for stdin
	defaultWriteStream  string // logical name or "" for outputWriter
	defaultTraceStream  string // logical name or "" for outputWriter
	inputReader         io.Reader
	stdinReader         *bufio.Reader
}

// New creates a new Engine instance.
func New() *Engine {
	mem := wm.New()
	net := rete.NewNetwork()
	cs := conflict.NewSet()

	// Connect WM events to Rete Alpha Network
	mem.AddListener(net)

	return &Engine{
		wm:                  mem,
		network:             net,
		conflictSet:         cs,
		rules:               make([]*model.Rule, 0),
		schemas:             make(map[string]*model.ClassSchema),
		vectorAttrs:         make(map[string]bool),
		ruleCount:           0,
		cycleCount:          0,
		halted:              false,
		outputWriter:        os.Stdout,
		traceWriter:         os.Stdout,
		watchLevel:          1,
		traceEnabled:        true,
		currentCol:          1,
		lastAddedTimetag:    0,
		genatomCounter:      0,
		attrIndices:         make(map[string]int),
		openFiles:           make(map[string]*openFileEntry),
		defaultAcceptStream: "",
		defaultWriteStream:  "",
		defaultTraceStream:  "",
		inputReader:         os.Stdin,
		stdinReader:         bufio.NewReader(os.Stdin),
	}
}

// SetOutputWriter configures where WRITE actions emit output.
func (e *Engine) SetOutputWriter(w io.Writer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outputWriter = w
}

// SetTraceWriter configures where trace messages (watch firings and WM changes) emit output.
func (e *Engine) SetTraceWriter(w io.Writer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traceWriter = w
}

// SetTrace enables or disables cycle execution tracing.
func (e *Engine) SetTrace(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traceEnabled = enabled
	if enabled {
		if e.watchLevel == 0 {
			e.watchLevel = 1
		}
	} else {
		e.watchLevel = 0
	}
}

// WatchLevel returns the current watch trace level (0, 1, or 2).
func (e *Engine) WatchLevel() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.watchLevel
}

// SetWatchLevel sets the watch trace level:
// 0 = no report of firings or changes to working memory
// 1 = report rule name and time tags for each instantiation fired
// 2 = report watch 1 info + report each change to working memory
func (e *Engine) SetWatchLevel(level int) error {
	if level < 0 || level > 2 {
		return fmt.Errorf("invalid watch level %d (expected 0, 1, or 2)", level)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.watchLevel = level
	e.traceEnabled = (level > 0)
	return nil
}

// TraceWriter returns the current output destination for trace messages.
func (e *Engine) TraceWriter() io.Writer {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.traceWriterLocked()
}

func (e *Engine) traceWriterLocked() io.Writer {
	if e.defaultTraceStream != "" {
		if entry, ok := e.openFiles[e.defaultTraceStream]; ok && entry.writer != nil {
			return entry.writer
		}
	}
	if e.traceWriter != nil {
		return e.traceWriter
	}
	if e.outputWriter != nil {
		return e.outputWriter
	}
	return os.Stdout
}

func (e *Engine) logWMAssertLocked(wme *model.WME) {
	if e.watchLevel >= 2 {
		tw := e.traceWriterLocked()
		if tw != nil {
			fmt.Fprintf(tw, "=>WM: %s\n", wme.String())
		}
	}
}

func (e *Engine) logWMRetractLocked(wme *model.WME) {
	if e.watchLevel >= 2 {
		tw := e.traceWriterLocked()
		if tw != nil {
			fmt.Fprintf(tw, "<=WM: %s\n", wme.String())
		}
	}
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
// If a rule with the same name already exists, the previous definition is excised first.
func (e *Engine) AddRule(rule *model.Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If rule already exists, replace previous definition
	for i, r := range e.rules {
		if r.Name == rule.Name {
			e.network.RemoveRule(rule.Name)
			e.conflictSet.RemoveRule(rule.Name)
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			break
		}
	}

	e.ruleCount++
	rule.Index = e.ruleCount
	e.rules = append(e.rules, rule)
	existingWMEs := e.wm.All()
	e.network.AddRuleWithWMEs(rule, e.conflictSet, existingWMEs)
}

// ExciseRule evicts a production rule by name from production memory,
// detaches it from the Rete network, and purges any pending activations from the conflict set.
// Returns true if the rule was found and excised, false otherwise.
func (e *Engine) ExciseRule(ruleName string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	found := false
	var newRules []*model.Rule
	for _, r := range e.rules {
		if r.Name == ruleName {
			found = true
		} else {
			newRules = append(newRules, r)
		}
	}
	if !found {
		return false
	}
	e.rules = newRules

	// Detach terminal node from Rete network
	e.network.RemoveRule(ruleName)

	// Purge pending activations from conflict set
	e.conflictSet.RemoveRule(ruleName)

	return true
}

// ExciseRules evicts multiple production rules by name.
// Returns the list of successfully excised rule names.
func (e *Engine) ExciseRules(names ...string) []string {
	var excised []string
	for _, name := range names {
		if e.ExciseRule(name) {
			excised = append(excised, name)
		}
	}
	return excised
}

// Rules returns a slice of all registered production rules in definition order.
func (e *Engine) Rules() []*model.Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	res := make([]*model.Rule, len(e.rules))
	copy(res, e.rules)
	return res
}

// Rule returns the registered production rule with the given name, or nil if not found.
func (e *Engine) Rule(name string) *model.Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.rules {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// PrintRule returns the pretty-printed OPS5 source text of a production rule by name.
// Returns (text, true) if the rule exists, or ("", false) if not found.
func (e *Engine) PrintRule(name string) (string, bool) {
	r := e.Rule(name)
	if r == nil {
		return "", false
	}
	return r.String(), true
}

// PrintRules returns the pretty-printed OPS5 source text of the specified rules.
// If names is empty or contains "*", all production rules currently in production memory are returned.
func (e *Engine) PrintRules(names ...string) []string {
	if len(names) == 0 || (len(names) == 1 && names[0] == "*") {
		rules := e.Rules()
		res := make([]string, 0, len(rules))
		for _, r := range rules {
			res = append(res, r.String())
		}
		return res
	}

	var res []string
	for _, name := range names {
		if text, ok := e.PrintRule(name); ok {
			res = append(res, text)
		}
	}
	return res
}

// FindWMEsMatching returns all active WMEs matching the given condition element pattern, ordered by timetag.
func (e *Engine) FindWMEsMatching(pattern *model.ConditionElement) []*model.WME {
	allWMEs := e.wm.All()
	if pattern == nil {
		return allWMEs
	}
	var res []*model.WME
	for _, w := range allWMEs {
		if pattern.Matches(w) {
			res = append(res, w)
		}
	}
	return res
}

// PrintPPWM returns string representations of all active WMEs matching the pattern.
func (e *Engine) PrintPPWM(pattern *model.ConditionElement) []string {
	wmes := e.FindWMEsMatching(pattern)
	res := make([]string, len(wmes))
	for i, w := range wmes {
		res[i] = w.String()
	}
	return res
}

// PPWM parses a ppwm command or pattern string, validates it against OPS5 restrictions,
// and returns all matching active working memory elements.
func (e *Engine) PPWM(patternSrc string) ([]*model.WME, error) {
	trimmed := strings.TrimSpace(patternSrc)
	if strings.HasPrefix(strings.ToLower(trimmed), "(ppwm") {
		// already has (ppwm
	} else if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = "(ppwm " + trimmed[1:]
	} else if strings.HasPrefix(strings.ToLower(trimmed), "ppwm ") || strings.ToLower(trimmed) == "ppwm" || strings.ToLower(trimmed) == "ppwm*" {
		trimmed = "(" + trimmed + ")"
	} else {
		trimmed = "(ppwm " + trimmed + ")"
	}

	p, err := parser.NewParser(trimmed)
	if err != nil {
		return nil, err
	}
	for _, va := range e.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range e.Schemas() {
		p.RegisterSchema(s)
	}
	pattern, err := p.ParsePPWM()
	if err != nil {
		return nil, err
	}
	return e.FindWMEsMatching(pattern), nil
}

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
	for i, a := range schema.Attributes {
		e.attrIndices[a] = i + 2
	}
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

// Litval returns the 1-based WME element index of the given attribute (or attribute in class).
func (e *Engine) Litval(class, attr string) (int, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.litvalLocked(class, attr)
}

func (e *Engine) litvalLocked(class, attr string) (int, bool) {
	normAttr := model.NormalizeAttribute(attr)
	if normAttr == "" {
		return 0, false
	}

	// 1. If class is specified, check that class's schema
	if class != "" {
		normClass := strings.ToLower(class)
		if schema, ok := e.schemas[normClass]; ok {
			if idx, found := schema.IndexOf(normAttr); found {
				return idx + 2, true
			}
		}
		return 0, false
	}

	// 2. If class is empty, check registered schemas in deterministic order
	var schemaKeys []string
	for k := range e.schemas {
		schemaKeys = append(schemaKeys, k)
	}
	sort.Strings(schemaKeys)
	for _, k := range schemaKeys {
		schema := e.schemas[k]
		if idx, found := schema.IndexOf(normAttr); found {
			return idx + 2, true
		}
	}

	// 3. Check global attribute indices map
	if idx, ok := e.attrIndices[normAttr]; ok {
		return idx, true
	}

	return 0, false
}

// Make asserts a new WME, resolving any RHS value functions (compute, accept, genatom, litval).
func (e *Engine) Make(class string, attrs map[string]model.Value) *model.WME {
	e.mu.Lock()
	defer e.mu.Unlock()

	normClass := strings.ToLower(class)
	if schema, ok := e.schemas[normClass]; ok {
		orderedKeys := e.getOrderedAttributeKeys(class, attrs)
		for _, k := range orderedKeys {
			schema.AddAttribute(k)
			if idx, ok := schema.IndexOf(k); ok {
				if _, exists := e.attrIndices[k]; !exists {
					e.attrIndices[k] = idx + 2
				}
			}
		}
	} else if len(attrs) > 0 {
		orderedKeys := e.getOrderedAttributeKeys(class, attrs)
		schema := model.NewClassSchema(class, orderedKeys)
		for a := range e.vectorAttrs {
			if schema.HasAttribute(a) {
				schema.SetVectorAttribute(a, true)
			}
		}
		e.schemas[normClass] = schema
		for i, k := range orderedKeys {
			if _, exists := e.attrIndices[k]; !exists {
				e.attrIndices[k] = i + 2
			}
		}
	}

	resolvedAttrs := make(map[string]model.Value, len(attrs))
	orderedKeys := e.getOrderedAttributeKeys(class, attrs)
	for _, k := range orderedKeys {
		resolvedAttrs[k] = e.resolveValue(attrs[k], nil)
	}
	wme := e.wm.Make(class, resolvedAttrs)
	e.lastAddedTimetag = wme.Timetag
	e.logWMAssertLocked(wme)
	return wme
}

// Remove retracts a WME by timetag.
func (e *Engine) Remove(timetag int64) (*model.WME, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	wme, err := e.wm.Remove(timetag)
	if err != nil {
		return nil, err
	}
	if e.lastAddedTimetag == timetag {
		e.lastAddedTimetag = 0
	}
	e.logWMRetractLocked(wme)
	return wme, nil
}

// RemoveAll retracts all active WMEs from working memory and returns them.
func (e *Engine) RemoveAll() []*model.WME {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastAddedTimetag = 0
	wmes := e.wm.RemoveAll()
	for _, w := range wmes {
		e.logWMRetractLocked(w)
	}
	return wmes
}

// Modify updates an existing WME, resolving any RHS value functions.
func (e *Engine) Modify(timetag int64, attrs map[string]model.Value) (*model.WME, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var clsName string
	oldWme, _ := e.wm.Get(timetag)
	if oldWme != nil {
		clsName = oldWme.Class
	}
	resolvedAttrs := make(map[string]model.Value, len(attrs))
	orderedKeys := e.getOrderedAttributeKeys(clsName, attrs)
	for _, k := range orderedKeys {
		resolvedAttrs[k] = e.resolveValue(attrs[k], nil)
	}

	wme, err := e.wm.Modify(timetag, resolvedAttrs)
	if err == nil && wme != nil {
		e.lastAddedTimetag = wme.Timetag
		if oldWme != nil {
			e.logWMRetractLocked(oldWme)
		}
		e.logWMAssertLocked(wme)
	}
	return wme, err
}

// LastAddedTimetag returns the timetag of the last WME added to working memory by make, modify, or call.
func (e *Engine) LastAddedTimetag() int64 {
	return e.lastAddedTimetag
}

// SetInputReader configures where ACCEPT actions read input from when using default stdin.
func (e *Engine) SetInputReader(r io.Reader) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inputReader = r
	if br, ok := r.(*bufio.Reader); ok {
		e.stdinReader = br
	} else {
		e.stdinReader = bufio.NewReader(r)
	}
}

// OpenFile opens a file and binds it to a logical name.
// mode can be "in" (read), "out" (truncate write), or "append".
func (e *Engine) OpenFile(logicalName, filespec, mode string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.openFileLocked(logicalName, filespec, mode)
}

func (e *Engine) openFileLocked(logicalName, filespec, mode string) error {
	normLog := strings.ToLower(logicalName)
	normMode := strings.ToLower(mode)

	var f *os.File
	var err error

	switch normMode {
	case "in":
		f, err = os.OpenFile(filespec, os.O_RDONLY, 0)
	case "out":
		f, err = os.OpenFile(filespec, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	case "append":
		f, err = os.OpenFile(filespec, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	default:
		return fmt.Errorf("invalid mode %q in openfile (valid: in, out, append)", mode)
	}

	if err != nil {
		return fmt.Errorf("openfile %s (%s, %s) failed: %w", logicalName, filespec, mode, err)
	}

	// Close previously open file with same logical name if any
	if prev, exists := e.openFiles[normLog]; exists && prev.file != nil {
		_ = prev.file.Close()
	}

	entry := &openFileEntry{
		file: f,
		mode: normMode,
	}
	if normMode == "in" {
		entry.reader = bufio.NewReader(f)
	} else {
		entry.writer = f
	}

	e.openFiles[normLog] = entry
	return nil
}

// CloseFile closes a file associated with a logical name.
func (e *Engine) CloseFile(logicalName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closeFileLocked(logicalName)
}

func (e *Engine) closeFileLocked(logicalName string) error {
	normLog := strings.ToLower(logicalName)
	entry, exists := e.openFiles[normLog]
	if !exists {
		return fmt.Errorf("file %s is not open", logicalName)
	}

	if e.defaultAcceptStream == normLog {
		e.defaultAcceptStream = ""
	}
	if e.defaultWriteStream == normLog {
		e.defaultWriteStream = ""
	}
	if e.defaultTraceStream == normLog {
		e.defaultTraceStream = ""
	}

	delete(e.openFiles, normLog)
	if entry.file != nil {
		return entry.file.Close()
	}
	return nil
}

// CloseAllFiles closes all currently open files.
func (e *Engine) CloseAllFiles() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeAllFilesLocked()
}

func (e *Engine) closeAllFilesLocked() {
	for k, entry := range e.openFiles {
		if entry.file != nil {
			_ = entry.file.Close()
		}
		delete(e.openFiles, k)
	}
	e.defaultAcceptStream = ""
	e.defaultWriteStream = ""
	e.defaultTraceStream = ""
}

// SetDefault redirects default I/O stream for accept, write, or trace.
func (e *Engine) SetDefault(logicalName, subsystem string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setDefaultLocked(logicalName, subsystem)
}

func (e *Engine) setDefaultLocked(logicalName, subsystem string) error {
	normLog := strings.ToLower(logicalName)
	normSub := strings.ToLower(subsystem)

	isRestore := normLog == "" || normLog == "nil" || normLog == "terminal" || normLog == "t" || normLog == "stdin" || normLog == "stdout"

	if !isRestore {
		entry, exists := e.openFiles[normLog]
		if !exists {
			return fmt.Errorf("file %s is not open", logicalName)
		}
		if normSub == "accept" && entry.mode != "in" {
			return fmt.Errorf("file %s is not open for input (mode=%s)", logicalName, entry.mode)
		}
		if (normSub == "write" || normSub == "trace") && entry.mode == "in" {
			return fmt.Errorf("file %s is not open for output (mode=%s)", logicalName, entry.mode)
		}
	}

	switch normSub {
	case "accept":
		if isRestore {
			e.defaultAcceptStream = ""
		} else {
			e.defaultAcceptStream = normLog
		}
	case "write":
		if isRestore {
			e.defaultWriteStream = ""
		} else {
			e.defaultWriteStream = normLog
		}
	case "trace":
		if isRestore {
			e.defaultTraceStream = ""
		} else {
			e.defaultTraceStream = normLog
		}
	default:
		return fmt.Errorf("unknown subsystem %q in default (expected accept, write, or trace)", subsystem)
	}
	return nil
}

// DefaultStream returns the logical stream name for a subsystem, or "" if standard.
func (e *Engine) DefaultStream(subsystem string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.defaultStreamLocked(subsystem)
}

func (e *Engine) defaultStreamLocked(subsystem string) string {
	switch strings.ToLower(subsystem) {
	case "accept":
		return e.defaultAcceptStream
	case "write":
		return e.defaultWriteStream
	case "trace":
		return e.defaultTraceStream
	default:
		return ""
	}
}

// IsFileOpen returns true if logicalName is currently open.
func (e *Engine) IsFileOpen(logicalName string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isFileOpenLocked(logicalName)
}

func (e *Engine) isFileOpenLocked(logicalName string) bool {
	_, exists := e.openFiles[strings.ToLower(logicalName)]
	return exists
}

// ReadAccept reads the next atom from a logical stream or standard input.
func (e *Engine) ReadAccept(logicalName string) (model.Value, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.readAcceptLocked(logicalName)
}

func (e *Engine) readAcceptLocked(logicalName string) (model.Value, error) {
	var reader *bufio.Reader
	normLog := strings.ToLower(logicalName)
	if normLog == "" {
		normLog = e.defaultAcceptStream
	}

	if normLog != "" {
		entry, exists := e.openFiles[normLog]
		if !exists {
			return model.NewSymbol("nil"), fmt.Errorf("file %s is not open for accept", normLog)
		}
		if entry.reader == nil {
			return model.NewSymbol("nil"), fmt.Errorf("file %s has no input reader", normLog)
		}
		reader = entry.reader
	} else {
		if e.stdinReader == nil {
			if e.inputReader == nil {
				e.inputReader = os.Stdin
			}
			e.stdinReader = bufio.NewReader(e.inputReader)
		}
		reader = e.stdinReader
	}

	var b strings.Builder
	// Skip leading whitespace
	for {
		ch, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				if b.Len() > 0 {
					break
				}
				return model.NewSymbol("end-of-file"), nil
			}
			return model.NewSymbol("nil"), err
		}
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
			break
		}
	}

	// Read word characters until whitespace
	for {
		ch, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return model.NewSymbol("nil"), err
		}
		if unicode.IsSpace(ch) {
			break
		}
		b.WriteRune(ch)
	}

	word := b.String()
	if word == "" {
		return model.NewSymbol("end-of-file"), nil
	}

	return model.AutoValue(word), nil
}

// ReadAcceptLine reads an entire line and returns atoms/vector.
func (e *Engine) ReadAcceptLine(logicalName string) (model.Value, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.readAcceptLineLocked(logicalName)
}

func (e *Engine) readAcceptLineLocked(logicalName string) (model.Value, error) {
	var reader *bufio.Reader
	normLog := strings.ToLower(logicalName)
	if normLog == "" {
		normLog = e.defaultAcceptStream
	}

	if normLog != "" {
		entry, exists := e.openFiles[normLog]
		if !exists {
			return model.NewSymbol("nil"), fmt.Errorf("file %s is not open for acceptline", normLog)
		}
		if entry.reader == nil {
			return model.NewSymbol("nil"), fmt.Errorf("file %s has no input reader", normLog)
		}
		reader = entry.reader
	} else {
		if e.stdinReader == nil {
			if e.inputReader == nil {
				e.inputReader = os.Stdin
			}
			e.stdinReader = bufio.NewReader(e.inputReader)
		}
		reader = e.stdinReader
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return model.NewSymbol("nil"), err
	}
	if err == io.EOF && len(line) == 0 {
		return model.NewSymbol("end-of-file"), nil
	}

	line = strings.TrimRight(line, "\r\n")
	words := strings.Fields(line)
	if len(words) == 0 {
		return model.NewSymbol("nil"), nil
	}
	if len(words) == 1 {
		return model.AutoValue(words[0]), nil
	}

	vals := make([]model.Value, len(words))
	for i, w := range words {
		vals[i] = model.AutoValue(w)
	}
	return model.NewVector(vals), nil
}

// applyArithmeticOp applies an arithmetic operator to two numeric values.
func applyArithmeticOp(a model.Value, op model.ComputeOp, b model.Value) (model.Value, error) {
	// If both are integers
	if a.Type() == model.TypeInteger && b.Type() == model.TypeInteger {
		i1 := a.Raw().(int64)
		i2 := b.Raw().(int64)
		switch op {
		case model.ComputeOpAdd:
			return model.NewInt(i1 + i2), nil
		case model.ComputeOpSub:
			return model.NewInt(i1 - i2), nil
		case model.ComputeOpMul:
			return model.NewInt(i1 * i2), nil
		case model.ComputeOpDiv:
			if i2 == 0 {
				return model.NewInt(0), fmt.Errorf("division by zero in compute")
			}
			return model.NewInt(i1 / i2), nil
		case model.ComputeOpMod:
			if i2 == 0 {
				return model.NewInt(0), fmt.Errorf("division by zero in compute modulo")
			}
			return model.NewInt(i1 % i2), nil
		}
	}

	// Floating point arithmetic if either operand is Float (or numeric string)
	var f1, f2 float64
	if a.Type() == model.TypeInteger {
		f1 = float64(a.Raw().(int64))
	} else if a.Type() == model.TypeFloat {
		f1 = a.Raw().(float64)
	} else if aStr, ok := a.Raw().(string); ok {
		if val, err := strconv.ParseFloat(aStr, 64); err == nil {
			f1 = val
		} else {
			return model.NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", a)
		}
	} else {
		return model.NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", a)
	}

	if b.Type() == model.TypeInteger {
		f2 = float64(b.Raw().(int64))
	} else if b.Type() == model.TypeFloat {
		f2 = b.Raw().(float64)
	} else if bStr, ok := b.Raw().(string); ok {
		if val, err := strconv.ParseFloat(bStr, 64); err == nil {
			f2 = val
		} else {
			return model.NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", b)
		}
	} else {
		return model.NewInt(0), fmt.Errorf("non-numeric operand in compute: %v", b)
	}

	switch op {
	case model.ComputeOpAdd:
		return model.NewFloat(f1 + f2), nil
	case model.ComputeOpSub:
		return model.NewFloat(f1 - f2), nil
	case model.ComputeOpMul:
		return model.NewFloat(f1 * f2), nil
	case model.ComputeOpDiv:
		if f2 == 0 {
			return model.NewInt(0), fmt.Errorf("division by zero in compute")
		}
		return model.NewFloat(f1 / f2), nil
	case model.ComputeOpMod:
		if f2 == 0 {
			return model.NewInt(0), fmt.Errorf("division by zero in compute modulo")
		}
		return model.NewFloat(math.Mod(f1, f2)), nil
	default:
		return model.NewInt(0), fmt.Errorf("unknown compute operator: %v", op)
	}
}

// evaluateCompute evaluates a (compute ...) expression using the provided variable bindings.
func (e *Engine) evaluateCompute(expr *model.ComputeExpr, bindings map[string]model.Value) (model.Value, error) {
	if len(expr.Operands) == 0 {
		return model.NewInt(0), nil
	}

	current := e.resolveValue(expr.Operands[0], bindings)
	if current.IsCompute() {
		var err error
		current, err = e.evaluateCompute(current.ComputeExpr(), bindings)
		if err != nil {
			return model.NewInt(0), err
		}
	}

	for i, op := range expr.Operators {
		if i+1 >= len(expr.Operands) {
			break
		}
		next := e.resolveValue(expr.Operands[i+1], bindings)
		if next.IsCompute() {
			var err error
			next, err = e.evaluateCompute(next.ComputeExpr(), bindings)
			if err != nil {
				return model.NewInt(0), err
			}
		}

		res, err := applyArithmeticOp(current, op, next)
		if err != nil {
			return model.NewInt(0), err
		}
		current = res
	}

	return current, nil
}

// resolveValue substitutes variable placeholders and evaluates compute and accept expressions.
func (e *Engine) resolveValue(val model.Value, bindings map[string]model.Value) model.Value {
	if val.IsVariable() {
		vName := val.VariableName()
		if bound, ok := bindings[vName]; ok {
			return bound
		}
	}
	if val.IsVector() {
		elems := val.VectorElements()
		resolved := make([]model.Value, 0, len(elems))
		for _, el := range elems {
			r := e.resolveValue(el, bindings)
			if r.IsVector() {
				resolved = append(resolved, r.VectorElements()...)
			} else {
				resolved = append(resolved, r)
			}
		}
		return model.NewVector(resolved)
	}
	if val.IsCompute() {
		res, err := e.evaluateCompute(val.ComputeExpr(), bindings)
		if err == nil {
			return res
		}
	}
	if val.IsAccept() {
		ae := val.AcceptExpr()
		var res model.Value
		var err error
		if ae.IsLine {
			res, err = e.readAcceptLineLocked(ae.LogicalFile)
		} else {
			res, err = e.readAcceptLocked(ae.LogicalFile)
		}
		if err == nil {
			return res
		}
		return model.NewSymbol("nil")
	}
	if val.IsGenatom() {
		return e.genatomLocked()
	}
	if val.IsLitval() {
		le := val.LitvalExpr()
		class := le.Class
		if strings.HasPrefix(class, "<") && strings.HasSuffix(class, ">") {
			classVal := e.resolveValue(model.NewVariable(class), bindings)
			if classVal.Type() == model.TypeSymbol || classVal.Type() == model.TypeString {
				class = classVal.Raw().(string)
			}
		}
		attrVal := e.resolveValue(le.Attribute, bindings)
		attrName := attrVal.String()
		if attrVal.Type() == model.TypeSymbol || attrVal.Type() == model.TypeString {
			attrName = attrVal.Raw().(string)
		}
		if idx, ok := e.litvalLocked(class, attrName); ok {
			return model.NewInt(int64(idx))
		}
		return model.NewSymbol("nil")
	}
	if val.IsSubstr() {
		return e.evaluateSubstrLocked(val.SubstrExpr(), bindings)
	}
	return val
}

// resolveTargetTimetag locates the WME timetag referenced by element variable or index.
func resolveTargetTimetag(act *conflict.Activation, elemVar string, index int, bindings map[string]model.Value) (int64, error) {
	if elemVar != "" {
		vName := strings.TrimPrefix(strings.TrimSuffix(elemVar, ">"), "<")
		if bound, ok := bindings[vName]; ok {
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

	e.currentActivation = dominant
	defer func() {
		e.currentActivation = nil
	}()

	// Refract activation so it won't fire again for the same WMEs
	e.conflictSet.MarkFired(dominant)
	e.cycleCount++

	if e.watchLevel >= 1 {
		tw := e.traceWriterLocked()
		if tw != nil {
			fmt.Fprintf(tw, "[Cycle %d] Fired rule '%s' with WMEs %v\n", e.cycleCount, dominant.Rule.Name, dominant.Timetags)
		}
	}

	// Local bindings for this rule firing, initialized with token bindings
	localBindings := make(map[string]model.Value, len(dominant.Token.Bindings))
	for k, v := range dominant.Token.Bindings {
		localBindings[k] = v
	}

	// Execute RHS actions
	for _, action := range dominant.Rule.Actions {
		switch act := action.(type) {
		case model.WatchAction:
			if act.Level == nil {
				tw := e.traceWriterLocked()
				if tw != nil {
					fmt.Fprintf(tw, "Current watch level: %d\n", e.watchLevel)
				}
			} else {
				e.watchLevel = *act.Level
				e.traceEnabled = (*act.Level > 0)
			}

		case model.BindAction:
			resolved := e.resolveValue(act.Value, localBindings)
			varName := strings.TrimPrefix(strings.TrimSuffix(act.Variable, ">"), "<")
			localBindings[varName] = resolved

		case model.CBindAction:
			if e.lastAddedTimetag == 0 {
				return true, fmt.Errorf("cbind: no element has been added to working memory")
			}
			varName := strings.TrimPrefix(strings.TrimSuffix(act.Variable, ">"), "<")
			localBindings[varName] = model.NewInt(e.lastAddedTimetag)

		case model.MakeAction:
			resolvedAttrs := make(map[string]model.Value, len(act.Attributes))
			orderedKeys := e.getOrderedAttributeKeys(act.Class, act.Attributes)
			for _, k := range orderedKeys {
				resolvedAttrs[k] = e.resolveValue(act.Attributes[k], localBindings)
			}
			wme := e.wm.Make(act.Class, resolvedAttrs)
			e.lastAddedTimetag = wme.Timetag
			e.logWMAssertLocked(wme)

		case model.ModifyAction:
			targetTimetag, err := resolveTargetTimetag(dominant, act.TargetElementVar, act.TargetIndex, localBindings)
			if err != nil {
				return true, err
			}
			var clsName string
			oldWme, _ := e.wm.Get(targetTimetag)
			if oldWme != nil {
				clsName = oldWme.Class
			}
			resolvedAttrs := make(map[string]model.Value, len(act.Attributes))
			orderedKeys := e.getOrderedAttributeKeys(clsName, act.Attributes)
			for _, k := range orderedKeys {
				resolvedAttrs[k] = e.resolveValue(act.Attributes[k], localBindings)
			}
			newWme, err := e.wm.Modify(targetTimetag, resolvedAttrs)
			if err != nil {
				return true, err
			}
			e.lastAddedTimetag = newWme.Timetag
			if act.TargetElementVar != "" {
				varName := strings.TrimPrefix(strings.TrimSuffix(act.TargetElementVar, ">"), "<")
				localBindings[varName] = model.NewInt(newWme.Timetag)
			}
			if oldWme != nil {
				e.logWMRetractLocked(oldWme)
			}
			e.logWMAssertLocked(newWme)

		case model.RemoveAction:
			if act.Wildcard {
				removed := e.wm.RemoveAll()
				e.lastAddedTimetag = 0
				for _, w := range removed {
					e.logWMRetractLocked(w)
				}
				continue
			}
			targetTimetag, err := resolveTargetTimetag(dominant, act.TargetElementVar, act.TargetIndex, localBindings)
			if err != nil {
				return true, err
			}
			oldWme, err := e.wm.Remove(targetTimetag)
			if err != nil {
				return true, err
			}
			if oldWme != nil {
				e.logWMRetractLocked(oldWme)
			}

		case model.OpenFileAction:
			filespec := act.Filespec.String()
			if act.Filespec.Type() == model.TypeString {
				filespec = act.Filespec.Raw().(string)
			} else if act.Filespec.Type() == model.TypeVariable {
				resolved := e.resolveValue(act.Filespec, localBindings)
				if resolved.Type() == model.TypeString {
					filespec = resolved.Raw().(string)
				} else {
					filespec = resolved.String()
				}
			}
			if err := e.openFileLocked(act.LogicalName, filespec, act.Mode); err != nil {
				return true, err
			}

		case model.CloseFileAction:
			if err := e.closeFileLocked(act.LogicalName); err != nil {
				return true, err
			}

		case model.DefaultAction:
			if err := e.setDefaultLocked(act.LogicalName, act.Subsystem); err != nil {
				return true, err
			}

		case model.WriteAction:
			ww := e.outputWriter
			if e.defaultWriteStream != "" {
				if entry, ok := e.openFiles[e.defaultWriteStream]; ok && entry.writer != nil {
					ww = entry.writer
				}
			}
			if ww != nil {
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
						fmt.Fprint(ww, "\n")
						e.currentCol = 1
						lastWasSpaceOrTab = true

					case model.WriteArgTabTo:
						targetCol := 1
						resolvedCol := e.resolveValue(arg.Value, localBindings)
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
							fmt.Fprint(ww, spaces)
							e.currentCol = targetCol
						} else {
							fmt.Fprint(ww, " ")
							e.currentCol++
						}
						lastWasSpaceOrTab = true

					case model.WriteArgValue:
						resolved := e.resolveValue(arg.Value, localBindings)
						var text string
						if resolved.Type() == model.TypeString {
							text = resolved.Raw().(string)
						} else {
							text = resolved.String()
						}

						if !lastWasSpaceOrTab {
							fmt.Fprint(ww, " ")
							e.currentCol++
						}

						fmt.Fprint(ww, text)
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
					fmt.Fprint(ww, "\n")
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

func (e *Engine) getOrderedAttributeKeys(class string, attrs map[string]model.Value) []string {
	var keys []string
	seen := make(map[string]bool, len(attrs))

	if schema, ok := e.schemas[strings.ToLower(class)]; ok {
		for _, attr := range schema.Attributes {
			norm := model.NormalizeAttribute(attr)
			if _, exists := attrs[norm]; exists && !seen[norm] {
				keys = append(keys, norm)
				seen[norm] = true
			}
		}
	}

	var remaining []string
	for k := range attrs {
		if !seen[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	keys = append(keys, remaining...)
	return keys
}

// Genatom generates a new unique symbolic atom (atom1, atom2, ...).
func (e *Engine) Genatom() model.Value {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.genatomLocked()
}

func (e *Engine) genatomLocked() model.Value {
	e.genatomCounter++
	return model.NewSymbol(fmt.Sprintf("atom%d", e.genatomCounter))
}

// ResetGenatom resets the genatom sequential counter to 0.
func (e *Engine) ResetGenatom() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.genatomCounter = 0
}

// CurrentCol returns the current output column position (1-indexed, where 1 means at start of line).
func (e *Engine) CurrentCol() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.currentCol
}

// EnsureNewline outputs a newline to the current output stream if the column cursor is not at 1.
func (e *Engine) EnsureNewline() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.currentCol > 1 {
		ww := e.outputWriter
		if e.defaultWriteStream != "" {
			if entry, ok := e.openFiles[e.defaultWriteStream]; ok && entry.writer != nil {
				ww = entry.writer
			}
		}
		if ww != nil {
			fmt.Fprint(ww, "\n")
		}
		e.currentCol = 1
	}
}

// EvaluateSubstr evaluates a substr expression on working memory.
func (e *Engine) EvaluateSubstr(se *model.SubstrExpr, bindings map[string]model.Value) model.Value {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.evaluateSubstrLocked(se, bindings)
}

func (e *Engine) evaluateSubstrLocked(se *model.SubstrExpr, bindings map[string]model.Value) model.Value {
	if se == nil {
		return model.NewSymbol("nil")
	}

	var targetWME *model.WME

	// 1. Resolve ElementRef
	// Can be an element variable (e.g. <str>) or an integer condition element index / timetag (e.g. 1)
	if se.ElementRef.IsVariable() {
		vName := strings.TrimPrefix(strings.TrimSuffix(se.ElementRef.VariableName(), ">"), "<")
		if bound, ok := bindings[vName]; ok {
			if bound.Type() == model.TypeInteger {
				tag := bound.Raw().(int64)
				if w, ok := e.wm.Get(tag); ok {
					targetWME = w
				} else if e.currentActivation != nil {
					for _, w := range e.currentActivation.Token.WMEs() {
						if w.Timetag == tag {
							targetWME = w
							break
						}
					}
					if targetWME == nil && tag >= 1 && int(tag) <= len(e.currentActivation.Token.WMEs()) {
						targetWME = e.currentActivation.Token.WMEs()[tag-1]
					}
				}
			}
		}
	} else {
		refVal := e.resolveValue(se.ElementRef, bindings)
		if refVal.Type() == model.TypeInteger {
			idx := refVal.Raw().(int64)
			if e.currentActivation != nil && idx >= 1 && int(idx) <= len(e.currentActivation.Token.WMEs()) {
				targetWME = e.currentActivation.Token.WMEs()[idx-1]
			} else if w, ok := e.wm.Get(idx); ok {
				targetWME = w
			}
		}
	}

	if targetWME == nil {
		return model.NewSymbol("nil")
	}

	normClass := strings.ToLower(targetWME.Class)
	schema := e.schemas[normClass]

	var orderedAttrs []string
	seenAttrs := make(map[string]bool)

	if schema != nil {
		for _, a := range schema.Attributes {
			norm := model.NormalizeAttribute(a)
			if !seenAttrs[norm] {
				orderedAttrs = append(orderedAttrs, norm)
				seenAttrs[norm] = true
			}
		}
	}
	var extraAttrs []string
	for a := range targetWME.Attributes {
		norm := model.NormalizeAttribute(a)
		if !seenAttrs[norm] {
			extraAttrs = append(extraAttrs, norm)
			seenAttrs[norm] = true
		}
	}
	sort.Strings(extraAttrs)
	orderedAttrs = append(orderedAttrs, extraAttrs...)

	attrStartPos := make(map[string]int)
	attrEndPos := make(map[string]int)

	// Position 1 is class name
	posList := []model.Value{model.NewSymbol(targetWME.Class)}

	for _, attr := range orderedAttrs {
		isVec := e.vectorAttrs[attr] || (schema != nil && schema.IsVectorAttribute(attr))
		startPos := len(posList) + 1
		attrStartPos[attr] = startPos

		val, exists := targetWME.Attributes[attr]
		if !exists {
			if isVec {
				attrEndPos[attr] = startPos - 1
			} else {
				posList = append(posList, model.NewSymbol("nil"))
				attrEndPos[attr] = len(posList)
			}
		} else {
			if isVec {
				if val.IsVector() {
					elems := val.VectorElements()
					for _, el := range elems {
						posList = append(posList, el)
					}
					attrEndPos[attr] = len(posList)
				} else if val.Type() == model.TypeSymbol && strings.EqualFold(val.Raw().(string), "nil") {
					attrEndPos[attr] = startPos - 1
				} else {
					posList = append(posList, val)
					attrEndPos[attr] = len(posList)
				}
			} else {
				posList = append(posList, val)
				attrEndPos[attr] = len(posList)
			}
		}
	}

	resolveAttrOrIndex := func(val model.Value) (int, bool, bool) {
		r := e.resolveValue(val, bindings)
		if r.Type() == model.TypeInteger {
			return int(r.Raw().(int64)), false, true
		}
		if r.Type() == model.TypeFloat {
			return int(r.Raw().(float64)), false, true
		}
		strVal := ""
		if r.Type() == model.TypeSymbol || r.Type() == model.TypeString {
			strVal = r.Raw().(string)
		} else if r.Type() == model.TypeVariable {
			strVal = r.VariableName()
		}
		if strings.EqualFold(strVal, "inf") {
			return 0, true, true
		}
		norm := model.NormalizeAttribute(strVal)
		if pos, ok := attrStartPos[norm]; ok {
			return pos, false, true
		}
		if lit, ok := e.litvalLocked(targetWME.Class, norm); ok {
			return lit, false, true
		}
		if num, err := strconv.Atoi(strVal); err == nil {
			return num, false, true
		}
		return 0, false, false
	}

	startIdx, _, startOk := resolveAttrOrIndex(se.Start)
	if !startOk {
		return model.NewSymbol("nil")
	}

	endIdx, endIsInf, endOk := resolveAttrOrIndex(se.End)
	if !endOk {
		return model.NewSymbol("nil")
	}

	if endIsInf {
		foundVec := false
		for _, attr := range orderedAttrs {
			isVec := e.vectorAttrs[attr] || (schema != nil && schema.IsVectorAttribute(attr))
			if isVec {
				sPos := attrStartPos[attr]
				ePos := attrEndPos[attr]
				if startIdx >= sPos && startIdx <= ePos {
					endIdx = ePos
					foundVec = true
					break
				}
			}
		}
		if !foundVec {
			for _, attr := range orderedAttrs {
				isVec := e.vectorAttrs[attr] || (schema != nil && schema.IsVectorAttribute(attr))
				if isVec {
					sPos := attrStartPos[attr]
					if startIdx >= sPos {
						endIdx = attrEndPos[attr]
						foundVec = true
						break
					}
				}
			}
		}
		if !foundVec {
			for _, attr := range orderedAttrs {
				isVec := e.vectorAttrs[attr] || (schema != nil && schema.IsVectorAttribute(attr))
				if isVec {
					endIdx = attrEndPos[attr]
					foundVec = true
					break
				}
			}
		}
		if !foundVec {
			endIdx = len(posList)
		}
	}

	if !endIsInf && startIdx == endIdx {
		// Single element access: returns the scalar element
		if startIdx >= 1 && startIdx <= len(posList) {
			return posList[startIdx-1]
		}
		return model.NewSymbol("nil")
	}

	if startIdx > endIdx || startIdx > len(posList) || endIdx < 1 {
		return model.NewVector([]model.Value{})
	}

	clampedStart := startIdx
	if clampedStart < 1 {
		clampedStart = 1
	}
	clampedEnd := endIdx
	if clampedEnd > len(posList) {
		clampedEnd = len(posList)
	}

	if clampedStart > clampedEnd {
		return model.NewVector([]model.Value{})
	}

	var res []model.Value
	for i := clampedStart; i <= clampedEnd; i++ {
		res = append(res, posList[i-1])
	}
	return model.NewVector(res)
}



