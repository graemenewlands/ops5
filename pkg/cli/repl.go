package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"ops5/pkg/conflict"
	"ops5/pkg/engine"
	"ops5/pkg/harness"
	"ops5/pkg/model"
	"ops5/pkg/parser"
)

// REPL provides an interactive command line interface for the OPS5 runtime.
type REPL struct {
	engine *engine.Engine
	runner *harness.Runner
	in     *bufio.Reader
	out    io.Writer
}

// NewREPL creates a new REPL instance.
func NewREPL(in io.Reader, out io.Writer) *REPL {
	eng := engine.New()
	eng.SetOutputWriter(out)
	bufIn := bufio.NewReader(in)
	eng.SetInputReader(bufIn)
	return &REPL{
		engine: eng,
		runner: harness.NewRunner(),
		in:     bufIn,
		out:    out,
	}
}

// Engine returns the underlying engine.
func (r *REPL) Engine() *engine.Engine {
	return r.engine
}

// Start launches the interactive REPL loop.
func (r *REPL) Start() {
	fmt.Fprintln(r.out, "OPS5 Interactive Runtime (type 'help' for commands, 'exit' to quit)")

	var multilineBuf strings.Builder
	openParens := 0

	for {
		if openParens == 0 {
			fmt.Fprint(r.out, "ops5> ")
		} else {
			fmt.Fprint(r.out, "...   ")
		}

		line, err := r.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(r.out, "\nGoodbye!")
				return
			}
			fmt.Fprintf(r.out, "Input error: %v\n", err)
			continue
		}

		line = strings.TrimRight(line, "\r\n")

		// Count parentheses for multi-line inputs (e.g. multi-line rule definitions)
		for _, ch := range line {
			if ch == '(' {
				openParens++
			} else if ch == ')' {
				openParens--
			}
		}

		if multilineBuf.Len() > 0 {
			multilineBuf.WriteString("\n")
		}
		multilineBuf.WriteString(line)

		if openParens > 0 {
			continue
		}

		input := strings.TrimSpace(multilineBuf.String())
		multilineBuf.Reset()
		openParens = 0

		if input == "" {
			continue
		}

		// Handle command
		if r.handleCommand(input) {
			break
		}
	}
}

// handleCommand returns true if the REPL should exit.
func (r *REPL) handleCommand(input string) bool {
	lower := strings.ToLower(input)
	if lower == "exit" || lower == "quit" || lower == "(exit)" || lower == "(quit)" {
		fmt.Fprintln(r.out, "Goodbye!")
		return true
	}

	// 1. S-expression rule definition: (p ...)
	if strings.HasPrefix(input, "(p ") || strings.HasPrefix(input, "(P ") {
		r.handleDefineRule(input)
		return false
	}

	// 2. S-expression make: (make ...)
	if strings.HasPrefix(input, "(make ") || strings.HasPrefix(input, "(MAKE ") {
		r.handleMake(input)
		return false
	}

	// 3. S-expression literalize: (literalize ...)
	if strings.HasPrefix(strings.ToLower(input), "(literalize ") {
		r.handleLiteralize(input)
		return false
	}

	// 4. S-expression vector-attribute: (vector-attribute ...)
	if strings.HasPrefix(strings.ToLower(input), "(vector-attribute ") {
		r.handleVectorAttribute(input)
		return false
	}

	// 5. S-expression openfile: (openfile ...)
	if strings.HasPrefix(strings.ToLower(input), "(openfile ") {
		r.handleOpenFile(input)
		return false
	}

	// 6. S-expression closefile: (closefile ...)
	if strings.HasPrefix(strings.ToLower(input), "(closefile ") {
		r.handleCloseFile(input)
		return false
	}

	// 7. S-expression default: (default ...)
	if strings.HasPrefix(strings.ToLower(input), "(default ") {
		r.handleDefault(input)
		return false
	}

	// 8. S-expression litval: (litval ...)
	if strings.HasPrefix(strings.ToLower(input), "(litval ") || strings.ToLower(strings.TrimSpace(input)) == "(litval)" {
		r.handleLitval(input)
		return false
	}

	// Strip outer parentheses for command convenience if present: e.g. (wm) -> wm
	cmd := input
	if strings.HasPrefix(cmd, "(") && strings.HasSuffix(cmd, ")") && !strings.Contains(cmd, "^") {
		cmd = strings.TrimSpace(cmd[1 : len(cmd)-1])
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	switch strings.ToLower(parts[0]) {
	case "help":
		r.printHelp()

	case "openfile":
		r.handleOpenFile(input)

	case "closefile":
		r.handleCloseFile(input)

	case "default":
		r.handleDefault(input)

	case "literalize":
		r.handleLiteralize("(" + input + ")")

	case "vector-attribute":
		r.handleVectorAttribute("(" + input + ")")

	case "vector-attributes":
		r.printVectorAttributes()

	case "schemas", "schema":
		classFilter := ""
		if len(parts) > 1 {
			classFilter = parts[1]
		}
		r.printSchemas(classFilter)

	case "wm":
		classFilter := ""
		if len(parts) > 1 {
			classFilter = parts[1]
		}
		r.printWorkingMemory(classFilter)

	case "cs":
		r.printConflictSet()

	case "run":
		maxCycles := 0
		if len(parts) > 1 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				maxCycles = n
			} else {
				fmt.Fprintf(r.out, "Invalid cycle limit: %s\n", parts[1])
				return false
			}
		}
		r.runCycles(maxCycles)

	case "step":
		r.stepCycle()

	case "make":
		// Handle make without parentheses: make class ^attr val
		r.handleMake("(" + input + ")")

	case "remove":
		if len(parts) < 2 {
			fmt.Fprintln(r.out, "Usage: remove <timetag>")
			return false
		}
		timetag, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			fmt.Fprintf(r.out, "Invalid timetag: %s\n", parts[1])
			return false
		}
		removed, err := r.engine.Remove(timetag)
		if err != nil {
			fmt.Fprintf(r.out, "Error: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "Removed: %s\n", removed.String())
		}

	case "strategy":
		if len(parts) == 1 {
			fmt.Fprintf(r.out, "Current conflict resolution strategy: %s\n", r.engine.ConflictSet().Strategy().String())
		} else {
			strat := strings.ToUpper(parts[1])
			if strat == "MEA" {
				r.engine.SetStrategy(conflict.StrategyMEA)
				fmt.Fprintln(r.out, "Strategy set to MEA (Means-Ends Analysis)")
			} else if strat == "LEX" {
				r.engine.SetStrategy(conflict.StrategyLEX)
				fmt.Fprintln(r.out, "Strategy set to LEX (Lexicographic)")
			} else {
				fmt.Fprintln(r.out, "Unknown strategy. Valid options: LEX, MEA")
			}
		}

	case "trace":
		if len(parts) == 1 {
			fmt.Fprintln(r.out, "Usage: trace on|off")
		} else {
			val := strings.ToLower(parts[1])
			if val == "on" || val == "true" || val == "1" {
				r.engine.SetTrace(true)
				fmt.Fprintln(r.out, "Tracing enabled")
			} else {
				r.engine.SetTrace(false)
				fmt.Fprintln(r.out, "Tracing disabled")
			}
		}

	case "load":
		if len(parts) < 2 {
			fmt.Fprintln(r.out, "Usage: load <filepath.ops>")
			return false
		}
		r.loadFile(parts[1])

	case "test":
		if len(parts) < 2 {
			fmt.Fprintln(r.out, "Usage: test <testcase.json>")
			return false
		}
		r.runTestCase(parts[1])

	case "genatom":
		fmt.Fprintln(r.out, r.engine.Genatom().String())

	case "litval":
		r.handleLitval(input)

	case "reset":
		r.engine.WorkingMemory().Reset()
		r.engine.ConflictSet().Reset()
		r.engine.ResetGenatom()
		fmt.Fprintln(r.out, "Working memory, conflict set, and genatom counter reset.")

	default:
		if strings.HasPrefix(input, "(") {
			p, err := parser.NewParser(input)
			if err == nil {
				for _, va := range r.engine.VectorAttributes() {
					p.RegisterVectorAttribute(va)
				}
				for _, s := range r.engine.Schemas() {
					p.RegisterSchema(s)
				}
				stmt, err := p.NextStatement()
				if err == nil && stmt != nil && stmt.Type == parser.StmtMake {
					r.handleMake(input)
					return false
				}
			}
		}
		fmt.Fprintf(r.out, "Unknown command: %s (type 'help' for command list)\n", parts[0])
	}

	return false
}

func (r *REPL) handleDefineRule(src string) {
	p, err := parser.NewParser(src)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range r.engine.Schemas() {
		p.RegisterSchema(s)
	}
	rule, err := p.ParseRule()
	if err != nil {
		fmt.Fprintf(r.out, "Rule syntax error: %v\n", err)
		return
	}
	r.engine.AddRule(rule)
	fmt.Fprintf(r.out, "Defined rule '%s' (conditions=%d, specificity=%d)\n", rule.Name, len(rule.Conditions), rule.Specificity())
}

func (r *REPL) handleMake(src string) {
	p, err := parser.NewParser(src)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range r.engine.Schemas() {
		p.RegisterSchema(s)
	}
	class, attrs, err := p.ParseMake()
	if err != nil {
		fmt.Fprintf(r.out, "Make syntax error: %v\n", err)
		return
	}
	for _, s := range p.Schemas() {
		if _, ok := r.engine.GetSchema(s.Class); !ok {
			r.engine.DeclareClass(s.Class, s.Attributes)
		}
	}
	wme := r.engine.Make(class, attrs)
	fmt.Fprintf(r.out, "Asserted: %s\n", wme.String())
}

func (r *REPL) handleLiteralize(src string) {
	trimmed := strings.TrimSpace(src)
	if !strings.HasPrefix(trimmed, "(") {
		trimmed = "(" + trimmed + ")"
	}
	p, err := parser.NewParser(trimmed)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	class, attrs, err := p.ParseLiteralize()
	if err != nil {
		fmt.Fprintf(r.out, "Literalize syntax error: %v\n", err)
		return
	}
	schema := r.engine.DeclareClass(class, attrs)
	vecs := schema.VectorAttributeNames()
	if len(vecs) > 0 {
		fmt.Fprintf(r.out, "Declared class schema '%s' with %d attributes: %v (vector: %v)\n",
			schema.Class, len(schema.Attributes), schema.Attributes, vecs)
	} else {
		fmt.Fprintf(r.out, "Declared class schema '%s' with %d attributes: %v\n",
			schema.Class, len(schema.Attributes), schema.Attributes)
	}
}

func (r *REPL) handleVectorAttribute(src string) {
	trimmed := strings.TrimSpace(src)
	if !strings.HasPrefix(trimmed, "(") {
		trimmed = "(" + trimmed + ")"
	}
	p, err := parser.NewParser(trimmed)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	attrs, err := p.ParseVectorAttribute()
	if err != nil {
		fmt.Fprintf(r.out, "vector-attribute syntax error: %v\n", err)
		return
	}
	for _, a := range attrs {
		r.engine.DeclareVectorAttribute(a)
	}
	fmt.Fprintf(r.out, "Declared vector attribute(s): %v\n", attrs)
}

func (r *REPL) handleLitval(input string) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		fmt.Fprintln(r.out, "Usage: (litval [<class>] <attr>)")
		return
	}
	var class, attr string
	if len(parts) == 2 {
		attr = parts[1]
	} else {
		class = parts[1]
		attr = parts[2]
	}
	idx, ok := r.engine.Litval(class, attr)
	if !ok {
		if class != "" {
			fmt.Fprintf(r.out, "Unknown attribute '%s' in class '%s'\n", attr, class)
		} else {
			fmt.Fprintf(r.out, "Unknown attribute '%s'\n", attr)
		}
		return
	}
	fmt.Fprintf(r.out, "%d\n", idx)
}

func (r *REPL) printVectorAttributes() {
	attrs := r.engine.VectorAttributes()
	if len(attrs) == 0 {
		fmt.Fprintln(r.out, "No vector attributes declared.")
		return
	}
	fmt.Fprintf(r.out, "Declared Vector Attributes (%d):\n", len(attrs))
	for _, a := range attrs {
		fmt.Fprintf(r.out, "  ^%s\n", a)
	}
}

func (r *REPL) printSchemas(classFilter string) {
	schemas := r.engine.Schemas()
	if len(schemas) == 0 {
		fmt.Fprintln(r.out, "No class schemas declared.")
		return
	}

	filter := strings.ToLower(classFilter)
	count := 0
	for _, s := range schemas {
		if filter == "" || s.Class == filter {
			if count == 0 {
				fmt.Fprintln(r.out, "Class Schemas:")
			}
			vecs := s.VectorAttributeNames()
			if len(vecs) > 0 {
				fmt.Fprintf(r.out, "  %s: %v (vector: %v)\n", s.Class, s.Attributes, vecs)
			} else {
				fmt.Fprintf(r.out, "  %s: %v\n", s.Class, s.Attributes)
			}
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(r.out, "No schema found for class '%s'.\n", classFilter)
	}
}

func (r *REPL) printWorkingMemory(classFilter string) {
	var wmes []*model.WME
	if classFilter != "" {
		wmes = r.engine.WorkingMemory().FindByClass(classFilter)
	} else {
		wmes = r.engine.WorkingMemory().All()
	}

	if len(wmes) == 0 {
		fmt.Fprintln(r.out, "Working memory is empty.")
		return
	}

	fmt.Fprintf(r.out, "Working Memory (%d elements):\n", len(wmes))
	for _, w := range wmes {
		fmt.Fprintf(r.out, "  %s\n", w.String())
	}
}

func (r *REPL) printConflictSet() {
	acts := r.engine.ConflictSet().All()
	if len(acts) == 0 {
		fmt.Fprintln(r.out, "Conflict set is empty.")
		return
	}

	dom, _ := r.engine.ConflictSet().SelectDominant()

	fmt.Fprintf(r.out, "Conflict Set (%d activations, strategy: %s):\n", len(acts), r.engine.ConflictSet().Strategy().String())
	for i, act := range acts {
		marker := "  "
		if dom != nil && act.Key() == dom.Key() {
			marker = "* " // Dominant activation
		}
		fmt.Fprintf(r.out, "%s%d. %s  WMEs: %v  (specificity: %d)\n", marker, i+1, act.Rule.Name, act.Timetags, act.Specificity())
	}
}

func (r *REPL) stepCycle() {
	dom, ok := r.engine.ConflictSet().SelectDominant()
	if !ok {
		fmt.Fprintln(r.out, "No activations in conflict set (quiescence).")
		return
	}

	ruleName := dom.Rule.Name
	timetags := dom.Timetags

	fired, err := r.engine.Step()
	if err != nil {
		fmt.Fprintf(r.out, "Error during firing: %v\n", err)
		return
	}
	if fired {
		fmt.Fprintf(r.out, "Fired: %s with WMEs %v (Cycle %d)\n", ruleName, timetags, r.engine.CycleCount())
	}
}

func (r *REPL) runCycles(maxCycles int) {
	fmt.Fprintf(r.out, "Running (max cycles: %d)...\n", maxCycles)
	cycles, err := r.engine.Run(maxCycles)
	if err != nil {
		fmt.Fprintf(r.out, "Execution error: %v\n", err)
	}
	if r.engine.IsHalted() {
		fmt.Fprintf(r.out, "Execution halted by rule action after %d cycles.\n", cycles)
	} else {
		fmt.Fprintf(r.out, "Reached quiescence after %d cycles.\n", cycles)
	}
}

// LoadFile reads and registers rules, makes, and literalize schemas from an OPS5 source file.
func (r *REPL) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", path, err)
	}

	p, err := parser.NewParser(string(data))
	if err != nil {
		return err
	}

	// Register any existing schemas and vector attributes from the engine into the parser
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range r.engine.Schemas() {
		p.RegisterSchema(s)
	}

	rulesCount := 0
	makesCount := 0
	litCount := 0
	vecCount := 0

	for {
		stmt, err := p.NextStatement()
		if err != nil {
			return err
		}
		if stmt == nil {
			break
		}

		switch stmt.Type {
		case parser.StmtRule:
			for _, s := range p.Schemas() {
				if _, ok := r.engine.GetSchema(s.Class); !ok {
					r.engine.DeclareClass(s.Class, s.Attributes)
				}
			}
			r.engine.AddRule(stmt.Rule)
			rulesCount++
		case parser.StmtMake:
			for _, s := range p.Schemas() {
				if _, ok := r.engine.GetSchema(s.Class); !ok {
					r.engine.DeclareClass(s.Class, s.Attributes)
				}
			}
			r.engine.Make(stmt.MakeClass, stmt.MakeAttributes)
			makesCount++
		case parser.StmtLiteralize:
			r.engine.DeclareClass(stmt.LiteralizeClass, stmt.LiteralizeAttrs)
			litCount++
		case parser.StmtVectorAttribute:
			for _, a := range stmt.VectorAttrs {
				r.engine.DeclareVectorAttribute(a)
			}
			vecCount++
		case parser.StmtOpenFile:
			filespec := stmt.OpenFile.Filespec.String()
			if stmt.OpenFile.Filespec.Type() == model.TypeString {
				filespec = stmt.OpenFile.Filespec.Raw().(string)
			}
			if err := r.engine.OpenFile(stmt.OpenFile.LogicalName, filespec, stmt.OpenFile.Mode); err != nil {
				return err
			}
		case parser.StmtCloseFile:
			if err := r.engine.CloseFile(stmt.CloseFile.LogicalName); err != nil {
				return err
			}
		case parser.StmtDefault:
			if err := r.engine.SetDefault(stmt.Default.LogicalName, stmt.Default.Subsystem); err != nil {
				return err
			}
		}
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("added %d rules", rulesCount))
	parts = append(parts, fmt.Sprintf("asserted %d WMEs", makesCount))
	if litCount > 0 {
		parts = append(parts, fmt.Sprintf("declared %d schemas", litCount))
	}
	if vecCount > 0 {
		parts = append(parts, fmt.Sprintf("declared %d vector-attributes", vecCount))
	}
	fmt.Fprintf(r.out, "Loaded %s: %s.\n", path, strings.Join(parts, ", "))
	return nil
}

func (r *REPL) loadFile(path string) {
	if err := r.LoadFile(path); err != nil {
		fmt.Fprintf(r.out, "Load error: %v\n", err)
	}
}

func (r *REPL) runTestCase(path string) {
	tc, err := r.runner.LoadTestCaseFromJSON(path)
	if err != nil {
		fmt.Fprintf(r.out, "Error loading test case: %v\n", err)
		return
	}

	fmt.Fprintf(r.out, "Running test case: %s\n", tc.Name)
	res := r.runner.Run(tc)
	if res.Passed {
		fmt.Fprintf(r.out, "PASS: %s (ran %d cycles)\n", tc.Name, res.CyclesRan)
		if res.Output != "" {
			fmt.Fprintf(r.out, "Output:\n%s\n", res.Output)
		}
	} else {
		fmt.Fprintf(r.out, "FAIL: %s (error: %v)\n", tc.Name, res.Error)
		if res.Output != "" {
			fmt.Fprintf(r.out, "Output:\n%s\n", res.Output)
		}
	}
}

func (r *REPL) printHelp() {
	helpText := `
Commands:
  (p <name> ...)            Define a production rule (multiline supported)
  (literalize <c> ...)      Declare an element class schema with attributes
  (vector-attribute <a...>) Declare attribute(s) as multi-valued vector attributes
  vector-attributes         Display declared vector attributes
  make <cls> [^a v]         Assert a new Working Memory Element (e.g. make goal ^status active)
  modify <tag> [^a v]       Modify attributes of an existing WME by timetag
  remove <tag>              Retract a WME by its timetag
  openfile <log> <f> <m>    Open a file stream (modes: in, out, append)
  closefile <log>           Close an open file stream
  default <log> <subsys>    Set default stream for accept, write, or trace
  genatom                   Generate a unique symbolic atom (e.g. atom1)
  litval [<cls>] <attr>     Display the numeric index assigned to an attribute
  wm [class]                Display current working memory elements
  schemas [class]           Display declared class schemas
  cs                        Display conflict set (pending instantiations in salience order)
  step                      Execute one Match-Resolve-Act cycle
  run [N]                   Run until quiescence, halt, or N cycles
  strategy [lex|mea]        View or switch conflict resolution strategy
  trace on|off              Toggle cycle execution tracing
  load <file.ops>           Load and compile rules and makes from an OPS5 source file
  test <file.json>          Execute an external test case file
  reset                     Reset working memory and conflict set
  help                      Show this help text
  exit / quit               Exit the REPL
`
	fmt.Fprint(r.out, helpText)
}

func (r *REPL) handleOpenFile(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) < 4 {
		fmt.Fprintln(r.out, "Usage: openfile <logical-name> <filespec> <in|out|append>")
		return
	}
	logicalName := tokens[1]
	filespec := tokens[2]
	mode := strings.ToLower(tokens[3])

	if err := r.engine.OpenFile(logicalName, filespec, mode); err != nil {
		fmt.Fprintf(r.out, "Error: %v\n", err)
		return
	}
	fmt.Fprintf(r.out, "Opened file '%s' as %s (%s)\n", filespec, logicalName, mode)
}

func (r *REPL) handleCloseFile(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) < 2 {
		fmt.Fprintln(r.out, "Usage: closefile <logical-name>")
		return
	}
	logicalName := tokens[1]

	if err := r.engine.CloseFile(logicalName); err != nil {
		fmt.Fprintf(r.out, "Error: %v\n", err)
		return
	}
	fmt.Fprintf(r.out, "Closed file '%s'\n", logicalName)
}

func (r *REPL) handleDefault(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) == 1 {
		fmt.Fprintf(r.out, "Default streams: accept=%s write=%s trace=%s\n",
			r.defaultStreamDisplay("accept"),
			r.defaultStreamDisplay("write"),
			r.defaultStreamDisplay("trace"))
		return
	}
	if len(tokens) == 2 {
		subsystem := strings.ToLower(tokens[1])
		if subsystem == "accept" || subsystem == "write" || subsystem == "trace" {
			fmt.Fprintf(r.out, "Default for %s is '%s'\n", subsystem, r.defaultStreamDisplay(subsystem))
			return
		}
		fmt.Fprintln(r.out, "Usage: default <logical-name> <accept|write|trace>")
		return
	}
	logicalName := tokens[1]
	subsystem := strings.ToLower(tokens[2])

	if err := r.engine.SetDefault(logicalName, subsystem); err != nil {
		fmt.Fprintf(r.out, "Error: %v\n", err)
		return
	}
	fmt.Fprintf(r.out, "Default for %s set to '%s'\n", subsystem, logicalName)
}

func (r *REPL) defaultStreamDisplay(subsystem string) string {
	d := r.engine.DefaultStream(subsystem)
	if d == "" {
		return "terminal"
	}
	return d
}

func tokenizeLine(input string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	l := parser.NewLexer(trimmed)
	var tokens []string
	for {
		tok, err := l.NextToken()
		if err != nil {
			return nil, err
		}
		if tok.Type == parser.TokenEOF {
			break
		}
		if tok.Type == parser.TokenLParen || tok.Type == parser.TokenRParen {
			continue
		}
		tokens = append(tokens, tok.Value)
	}
	return tokens, nil
}

