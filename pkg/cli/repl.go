package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/harness"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

// REPL provides an interactive command line interface for the OPS5 runtime.
type REPL struct {
	engine     *engine.Engine
	runner     *harness.Runner
	in         *bufio.Reader
	out        io.Writer
	styler     *Styler
	history    *History
	completer  *Completer
	lineEditor *LineEditor
	tableMode  bool
}

// NewREPL creates a new REPL instance.
func NewREPL(in io.Reader, out io.Writer) *REPL {
	eng := engine.New()
	eng.SetOutputWriter(out)
	eng.SetTraceWriter(out)
	bufIn := bufio.NewReader(in)
	eng.SetInputReader(bufIn)

	styler := NewStyler(out)
	history := NewHistory(1000)
	completer := NewCompleter(eng)
	editor := NewLineEditor(in, out, styler, history, completer)
	editor.SetBufReader(bufIn)

	return &REPL{
		engine:     eng,
		runner:     harness.NewRunner(),
		in:         bufIn,
		out:        out,
		styler:     styler,
		history:    history,
		completer:  completer,
		lineEditor: editor,
		tableMode:  false,
	}
}

// Engine returns the underlying engine.
func (r *REPL) Engine() *engine.Engine {
	return r.engine
}

// SetColor enables or disables ANSI color styling.
func (r *REPL) SetColor(enabled bool) {
	r.styler.Enabled = enabled
}

// SetTableMode enables or disables tabular display mode for wm, cs, and schemas.
func (r *REPL) SetTableMode(enabled bool) {
	r.tableMode = enabled
}

// Styler returns the REPL's styler.
func (r *REPL) Styler() *Styler {
	return r.styler
}

// History returns the REPL's history manager.
func (r *REPL) History() *History {
	return r.history
}

// Completer returns the REPL's autocompletion engine.
func (r *REPL) Completer() *Completer {
	return r.completer
}

func (r *REPL) printWelcomeBanner() {
	if r.styler.Enabled {
		banner := r.styler.Bold(r.styler.BrightCyan("OPS5 Interactive Production System Runtime")) + "\n" +
			r.styler.Dim("Type ") + r.styler.Bold("help") + r.styler.Dim(" for command list, ") +
			r.styler.Bold("exit") + r.styler.Dim(" or Ctrl-D to quit. Tab for autocompletion.")
		fmt.Fprintln(r.out, banner)
	} else {
		fmt.Fprintln(r.out, "OPS5 Interactive Runtime (type 'help' for commands, 'exit' to quit)")
	}
}

// Start launches the interactive REPL loop.
func (r *REPL) Start() {
	r.printWelcomeBanner()
	defer r.history.Save()

	var multilineBuf strings.Builder
	openParens := 0

	for {
		var prompt string
		if openParens == 0 {
			prompt = r.styler.Prompt()
		} else {
			prompt = r.styler.ContinuationPrompt()
		}

		line, err := r.lineEditor.ReadLine(prompt)
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

		if !r.lineEditor.IsTerminal() {
			r.history.Add(input)
		}

		// Handle command
		if r.handleCommand(input) {
			break
		}
	}
}

// handleCommand returns true if the REPL should exit.
func (r *REPL) handleCommand(input string) bool {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, ";") {
		return false
	}

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

	// 9. S-expression excise: (excise ...)
	if strings.HasPrefix(strings.ToLower(input), "(excise ") || strings.ToLower(strings.TrimSpace(input)) == "(excise)" {
		r.handleExcise(input)
		return false
	}

	// 10. S-expression pm: (pm ...)
	if strings.HasPrefix(strings.ToLower(input), "(pm ") || strings.ToLower(strings.TrimSpace(input)) == "(pm)" || strings.ToLower(strings.TrimSpace(input)) == "(pm*)" || strings.HasPrefix(strings.ToLower(input), "(pm*") {
		r.handlePM(input)
		return false
	}

	// 11. S-expression ppwm: (ppwm ...)
	if strings.HasPrefix(strings.ToLower(input), "(ppwm ") || strings.ToLower(strings.TrimSpace(input)) == "(ppwm)" || strings.ToLower(strings.TrimSpace(input)) == "(ppwm*)" || strings.HasPrefix(strings.ToLower(input), "(ppwm*") {
		r.handlePPWM(input)
		return false
	}

	// 11. S-expression remove: (remove ...)
	if strings.HasPrefix(strings.ToLower(input), "(remove ") || strings.ToLower(strings.TrimSpace(input)) == "(remove)" || strings.ToLower(strings.TrimSpace(input)) == "(remove*)" || strings.HasPrefix(strings.ToLower(input), "(remove*") {
		r.handleRemove(input)
		return false
	}

	// 12. S-expression watch: (watch ...)
	if strings.HasPrefix(strings.ToLower(input), "(watch ") || strings.ToLower(strings.TrimSpace(input)) == "(watch)" {
		r.handleWatch(input)
		return false
	}

	// 13. S-expression substr: (substr ...)
	if strings.HasPrefix(strings.ToLower(input), "(substr ") || strings.ToLower(strings.TrimSpace(input)) == "(substr)" {
		r.handleSubstr(input)
		return false
	}

	// 14. S-expression matches: (matches ...)
	if strings.HasPrefix(strings.ToLower(input), "(matches ") || strings.ToLower(strings.TrimSpace(input)) == "(matches)" || strings.ToLower(strings.TrimSpace(input)) == "(matches*)" || strings.HasPrefix(strings.ToLower(input), "(matches*") {
		r.handleMatches(input)
		return false
	}

	// 15. S-expression pbreak: (pbreak ...)
	if strings.HasPrefix(strings.ToLower(input), "(pbreak ") || strings.ToLower(strings.TrimSpace(input)) == "(pbreak)" {
		r.handlePBreak(input)
		return false
	}

	// 16. S-expression unpbreak / unbreak: (unpbreak ...) or (unbreak ...)
	if strings.HasPrefix(strings.ToLower(input), "(unpbreak ") || strings.ToLower(strings.TrimSpace(input)) == "(unpbreak)" ||
		strings.HasPrefix(strings.ToLower(input), "(unbreak ") || strings.ToLower(strings.TrimSpace(input)) == "(unbreak)" {
		r.handleUnpbreak(input)
		return false
	}

	// 17. S-expression dot: (dot ...)
	if strings.HasPrefix(strings.ToLower(input), "(dot ") || strings.ToLower(strings.TrimSpace(input)) == "(dot)" {
		r.handleDOT(input)
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
		r.printSchemas(parts[1:]...)

	case "wm":
		r.printWorkingMemory(parts[1:]...)

	case "cs":
		r.printConflictSet(parts[1:]...)

	case "status", "info":
		r.printStatus()

	case "clear", "cls":
		fmt.Fprint(r.out, "\033[H\033[2J")

	case "table":
		if len(parts) > 1 {
			arg := strings.ToLower(parts[1])
			if arg == "on" || arg == "true" || arg == "1" {
				r.tableMode = true
				fmt.Fprintln(r.out, "Table mode enabled")
			} else if arg == "off" || arg == "false" || arg == "0" {
				r.tableMode = false
				fmt.Fprintln(r.out, "Table mode disabled")
			} else {
				fmt.Fprintln(r.out, "Usage: table [on|off]")
			}
		} else {
			if r.tableMode {
				fmt.Fprintln(r.out, "Table mode is on")
			} else {
				fmt.Fprintln(r.out, "Table mode is off")
			}
		}

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
		r.handleRemove(input)

	case "excise":
		r.handleExcise(input)

	case "pm":
		r.handlePM(input)

	case "ppwm":
		r.handlePPWM(input)

	case "matches":
		r.handleMatches(input)

	case "pbreak":
		r.handlePBreak(input)

	case "unpbreak", "unbreak":
		r.handleUnpbreak(input)

	case "dot":
		r.handleDOT(input)

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

	case "watch":
		r.handleWatch(input)

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

	case "substr":
		r.handleSubstr(input)

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
	fmt.Fprintf(r.out, "Asserted: %s\n", r.styler.FormatWME(wme))
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

func (r *REPL) printSchemas(args ...string) {
	schemas := r.engine.Schemas()
	if len(schemas) == 0 {
		fmt.Fprintln(r.out, "No class schemas declared.")
		return
	}

	useTable := r.tableMode
	classFilter := ""
	for _, arg := range args {
		if arg == "--table" || arg == "-t" {
			useTable = true
		} else if !strings.HasPrefix(arg, "-") && classFilter == "" {
			classFilter = arg
		}
	}

	filter := strings.ToLower(classFilter)
	count := 0
	var matched []*model.ClassSchema
	for _, s := range schemas {
		if filter == "" || s.Class == filter {
			matched = append(matched, s)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(r.out, "No schema found for class '%s'.\n", classFilter)
		return
	}

	if useTable {
		tbl := NewTable(r.styler)
		tbl.SetHeaders("Class", "Attributes", "Vector Attributes")
		for _, s := range matched {
			attrsStr := strings.Join(s.Attributes, ", ")
			if attrsStr == "" {
				attrsStr = r.styler.Dim("(none)")
			}
			vecs := s.VectorAttributeNames()
			vecsStr := strings.Join(vecs, ", ")
			if vecsStr == "" {
				vecsStr = r.styler.Dim("(none)")
			} else {
				vecsStr = r.styler.Wrap(ansiBrightCyan, vecsStr)
			}
			tbl.AddRow(r.styler.Wrap(ansiBold+ansiBrightMagenta, s.Class), attrsStr, vecsStr)
		}
		fmt.Fprintln(r.out, "Class Schemas:")
		fmt.Fprint(r.out, tbl.Render())
		return
	}

	fmt.Fprintln(r.out, "Class Schemas:")
	for _, s := range matched {
		vecs := s.VectorAttributeNames()
		if len(vecs) > 0 {
			fmt.Fprintf(r.out, "  %s: %v (vector: %v)\n", r.styler.Wrap(ansiBold+ansiBrightMagenta, s.Class), s.Attributes, vecs)
		} else {
			fmt.Fprintf(r.out, "  %s: %v\n", r.styler.Wrap(ansiBold+ansiBrightMagenta, s.Class), s.Attributes)
		}
	}
}

func (r *REPL) printWorkingMemory(args ...string) {
	useTable := r.tableMode
	classFilter := ""
	for _, arg := range args {
		if arg == "--table" || arg == "-t" {
			useTable = true
		} else if !strings.HasPrefix(arg, "-") && classFilter == "" {
			classFilter = arg
		}
	}

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

	if useTable {
		fmt.Fprintf(r.out, "Working Memory (%d elements):\n", len(wmes))
		r.printWorkingMemoryTable(wmes)
		return
	}

	fmt.Fprintf(r.out, "Working Memory (%d elements):\n", len(wmes))
	for _, w := range wmes {
		fmt.Fprintf(r.out, "  %s\n", r.styler.FormatWME(w))
	}
}

func (r *REPL) printWorkingMemoryTable(wmes []*model.WME) {
	tbl := NewTable(r.styler)
	tbl.SetHeaders("Timetag", "Class", "Attributes")

	for _, w := range wmes {
		var keys []string
		for k := range w.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var attrPairs []string
		for _, k := range keys {
			attrPairs = append(attrPairs, fmt.Sprintf("%s %s", r.styler.Wrap(ansiBrightCyan, "^"+k), r.styler.FormatValue(w.Attributes[k])))
		}
		attrStr := strings.Join(attrPairs, " ")
		if attrStr == "" {
			attrStr = r.styler.Dim("(none)")
		}

		tbl.AddRow(
			r.styler.Wrap(ansiBrightYellow, fmt.Sprintf("%d", w.Timetag)),
			r.styler.Wrap(ansiBold+ansiBrightMagenta, w.Class),
			attrStr,
		)
	}

	fmt.Fprint(r.out, tbl.Render())
}

func (r *REPL) printConflictSet(args ...string) {
	acts := r.engine.ConflictSet().All()
	if len(acts) == 0 {
		fmt.Fprintln(r.out, "Conflict set is empty.")
		return
	}

	useTable := r.tableMode
	for _, arg := range args {
		if arg == "--table" || arg == "-t" {
			useTable = true
		}
	}

	dom, _ := r.engine.ConflictSet().SelectDominant()

	fmt.Fprintf(r.out, "Conflict Set (%d activations, strategy: %s):\n", len(acts), r.engine.ConflictSet().Strategy().String())
	if useTable {
		r.printConflictSetTable(acts, dom)
		return
	}

	hasSalience := false
	for _, act := range acts {
		if act.Salience() != 0 {
			hasSalience = true
			break
		}
	}

	for i, act := range acts {
		marker := "  "
		if dom != nil && act.Key() == dom.Key() {
			marker = "* " // Dominant activation
		}
		salienceStr := ""
		if hasSalience {
			salienceStr = fmt.Sprintf(" [salience: %d]", act.Salience())
		}
		fmt.Fprintf(r.out, "%s%d. %s%s  WMEs: %v  (specificity: %d)\n", marker, i+1, r.styler.Bold(act.Rule.Name), salienceStr, act.Timetags, act.Specificity())
	}
}

func (r *REPL) printConflictSetTable(acts []*conflict.Activation, dom *conflict.Activation) {
	tbl := NewTable(r.styler)

	hasSalience := false
	for _, act := range acts {
		if act.Salience() != 0 {
			hasSalience = true
			break
		}
	}

	if hasSalience {
		tbl.SetHeaders("#", "Sel", "Rule", "Salience", "Timetags", "Specificity")
	} else {
		tbl.SetHeaders("#", "Sel", "Rule", "Timetags", "Specificity")
	}

	for i, act := range acts {
		sel := " "
		ruleName := act.Rule.Name
		if dom != nil && act.Key() == dom.Key() {
			sel = r.styler.BrightYellow("*")
			ruleName = r.styler.Bold(r.styler.BrightYellow(ruleName))
		}
		timetagsStr := fmt.Sprintf("%v", act.Timetags)
		if hasSalience {
			salienceStr := fmt.Sprintf("%d", act.Salience())
			if act.Salience() > 0 {
				salienceStr = r.styler.BrightCyan(salienceStr)
			} else if act.Salience() < 0 {
				salienceStr = r.styler.BrightMagenta(salienceStr)
			}
			tbl.AddRow(
				fmt.Sprintf("%d", i+1),
				sel,
				ruleName,
				salienceStr,
				timetagsStr,
				fmt.Sprintf("%d", act.Specificity()),
			)
		} else {
			tbl.AddRow(
				fmt.Sprintf("%d", i+1),
				sel,
				ruleName,
				timetagsStr,
				fmt.Sprintf("%d", act.Specificity()),
			)
		}
	}

	fmt.Fprint(r.out, tbl.Render())
}

func (r *REPL) printStatus() {
	header := r.styler.Header("OPS5 Runtime Status")
	fmt.Fprintln(r.out, header)
	fmt.Fprintf(r.out, "  Strategy:          %s\n", r.styler.Bold(r.engine.ConflictSet().Strategy().String()))
	fmt.Fprintf(r.out, "  Watch Level:       %d\n", r.engine.WatchLevel())
	fmt.Fprintf(r.out, "  Production Rules:  %d\n", len(r.engine.Rules()))
	fmt.Fprintf(r.out, "  Working Memory:    %d WMEs\n", len(r.engine.WorkingMemory().All()))
	fmt.Fprintf(r.out, "  Conflict Set:      %d activations\n", len(r.engine.ConflictSet().All()))
	if dom, ok := r.engine.ConflictSet().SelectDominant(); ok {
		fmt.Fprintf(r.out, "  Dominant Rule:     %s (timetags: %v)\n", r.styler.BrightYellow(dom.Rule.Name), dom.Timetags)
	} else {
		fmt.Fprintf(r.out, "  Dominant Rule:     %s\n", r.styler.Dim("none (quiescence)"))
	}
	fmt.Fprintf(r.out, "  Class Schemas:     %d\n", len(r.engine.Schemas()))
	fmt.Fprintf(r.out, "  Vector Attributes: %d\n", len(r.engine.VectorAttributes()))
	fmt.Fprintf(r.out, "  Table Mode:        %t\n", r.tableMode)
	fmt.Fprintf(r.out, "  Color Enabled:     %t\n", r.styler.Enabled)
}

func (r *REPL) stepCycle() {
	_, ok := r.engine.ConflictSet().SelectDominant()
	if !ok {
		fmt.Fprintln(r.out, "No activations in conflict set (quiescence).")
		return
	}

	fired, err := r.engine.Step()
	if err != nil {
		fmt.Fprintf(r.out, "Error during firing: %v\n", err)
		return
	}
	if fired {
		r.engine.EnsureNewline()
	}
}

func (r *REPL) runCycles(maxCycles int) {
	fmt.Fprintf(r.out, "Running (max cycles: %d)...\n", maxCycles)
	cycles, err := r.engine.Run(maxCycles)
	if err != nil {
		fmt.Fprintf(r.out, "Execution error: %v\n", err)
	}
	r.engine.EnsureNewline()
	if r.engine.IsHalted() {
		fmt.Fprintf(r.out, "Execution halted by rule action after %d cycles.\n", cycles)
	} else if r.engine.HitBreakpoint() != "" {
		fmt.Fprintf(r.out, "** Break on rule '%s' after %d cycles **\n", r.engine.HitBreakpoint(), cycles)
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
		case parser.StmtExcise:
			for _, name := range stmt.ExciseRules {
				r.engine.ExciseRule(name)
			}
		case parser.StmtPM:
			if len(stmt.PMRules) == 0 || (len(stmt.PMRules) == 1 && stmt.PMRules[0] == "*") {
				for _, rule := range r.engine.Rules() {
					fmt.Fprintln(r.out, rule.String())
				}
			} else {
				for _, name := range stmt.PMRules {
					if rule := r.engine.Rule(name); rule != nil {
						fmt.Fprintln(r.out, rule.String())
					} else {
						fmt.Fprintf(r.out, "Rule '%s' not found\n", name)
					}
				}
			}
		case parser.StmtRemove:
			if stmt.RemoveWildcard {
				r.engine.RemoveAll()
			} else {
				for _, tag := range stmt.RemoveTimetags {
					r.engine.Remove(tag)
				}
			}
		case parser.StmtWatch:
			if stmt.WatchLevel == nil {
				fmt.Fprintf(r.out, "Current watch level: %d\n", r.engine.WatchLevel())
			} else {
				_ = r.engine.SetWatchLevel(*stmt.WatchLevel)
			}
		case parser.StmtPPWM:
			matches := r.engine.FindWMEsMatching(stmt.PPWMPattern)
			for _, w := range matches {
				fmt.Fprintln(r.out, r.styler.FormatWME(w))
			}
		case parser.StmtStrategy:
			if strings.ToUpper(stmt.Strategy) == "MEA" {
				r.engine.ConflictSet().SetStrategy(conflict.StrategyMEA)
			} else if strings.ToUpper(stmt.Strategy) == "LEX" {
				r.engine.ConflictSet().SetStrategy(conflict.StrategyLEX)
			}
		case parser.StmtSubstr:
			val := r.engine.EvaluateSubstr(stmt.Substr, nil)
			fmt.Fprintln(r.out, val.String())
		case parser.StmtMatches:
			fmt.Fprint(r.out, r.engine.FormatMatches(stmt.MatchesRules...))
		case parser.StmtPBreak:
			if len(stmt.PBreakRules) == 0 {
				r.printBreakpoints()
			} else {
				for _, name := range stmt.PBreakRules {
					r.engine.SetBreakpoint(name)
					fmt.Fprintf(r.out, "Breakpoint set on rule '%s'\n", name)
				}
			}
		case parser.StmtUnpbreak:
			if len(stmt.UnpbreakRules) == 0 || stmt.UnpbreakRules[0] == "*" || strings.ToLower(stmt.UnpbreakRules[0]) == "nil" {
				r.engine.ClearBreakpoints()
				fmt.Fprintln(r.out, "All rule breakpoints cleared.")
			} else {
				for _, name := range stmt.UnpbreakRules {
					if r.engine.RemoveBreakpoint(name) {
						fmt.Fprintf(r.out, "Breakpoint removed for rule '%s'\n", name)
					} else {
						fmt.Fprintf(r.out, "No breakpoint was set for rule '%s'\n", name)
					}
				}
			}
		case parser.StmtDOT:
			if stmt.DOTFile == "" {
				if err := r.engine.ExportDOT(r.out); err != nil {
					fmt.Fprintf(r.out, "Error exporting DOT: %v\n", err)
				}
			} else {
				f, err := os.Create(stmt.DOTFile)
				if err != nil {
					fmt.Fprintf(r.out, "Failed to create DOT file %s: %v\n", stmt.DOTFile, err)
				} else {
					if err := r.engine.ExportDOT(f); err != nil {
						fmt.Fprintf(r.out, "Error exporting DOT to %s: %v\n", stmt.DOTFile, err)
					} else {
						fmt.Fprintf(r.out, "Exported Rete network graph to %s\n", stmt.DOTFile)
					}
					f.Close()
				}
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
	if r.styler.Enabled {
		h := func(s string) string { return r.styler.Header(s) }
		c := func(s string) string { return r.styler.BrightCyan(s) }
		d := func(s string) string { return r.styler.Dim(s) }

		var b strings.Builder
		b.WriteString("\n" + h("OPS5 Production System - Interactive Commands") + "\n\n")

		b.WriteString(h("Production Rules & Schemas:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("(p <name> ...)"), "Define a production rule (multiline supported)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("(literalize <c> ...)"), "Declare an element class schema with attributes"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("(vector-attribute <a...>)"), "Declare attribute(s) as multi-valued vector attributes"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("vector-attributes"), "Display declared vector attributes"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("schemas [class] [--table]"), "Display declared class schemas"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("pm [<rule...> | *]"), "Print production rules in memory"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("excise <rule...>"), "Evict production rules from memory and conflict set"))

		b.WriteString("\n" + h("Working Memory:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("make <class> [^a v]"), "Assert a new Working Memory Element"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("wm [class] [--table]"), "Display current working memory elements"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("ppwm [<pattern>]"), "Print WMEs matching pattern (e.g. ppwm City ^state PA)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("remove <tag...> | *"), "Retract WME(s) by timetag or all WMEs (*)"))

		b.WriteString("\n" + h("Execution & Conflict Resolution:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("step"), "Execute one Match-Resolve-Act cycle"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("run [N]"), "Run until quiescence, halt, or N cycles"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("cs [--table]"), "Display conflict set in salience order"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("strategy [lex|mea]"), "View or switch conflict resolution strategy"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("watch [0|1|2]"), "Display or set watch trace level (0=none, 1=firings, 2=firings+WM)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("trace on|off"), "Toggle cycle execution tracing"))

		b.WriteString("\n" + h("Diagnostic & Breakpoint Tools:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("matches [<rule...> | *]"), "Display partial Rete matches and activations for rule(s)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("pbreak [<rule...>]"), "Set breakpoint on rule(s) or list active breakpoints"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("unpbreak [<rule...> | *]"), "Remove breakpoint on rule(s) or clear all breakpoints"))

		b.WriteString("\n" + h("REPL & GUI Enhancements:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("status / info"), "Display runtime status overview"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("table [on|off]"), "Toggle or view boxed table formatting mode"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("clear / cls"), "Clear the terminal screen (or Ctrl-L)"))

		b.WriteString("\n" + h("I/O & Expressions:") + "\n")
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("openfile <log> <f> <m>"), "Open a file stream (modes: in, out, append)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("closefile <log>"), "Close an open file stream"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("default <log> <subsys>"), "Set default stream for accept, write, or trace"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("genatom"), "Generate a unique symbolic atom (e.g. atom1)"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("litval [<cls>] <attr>"), "Display numeric index assigned to attribute"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("substr <elem> <start> <end>"), "Extract subsequence from WME vector"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("load <file.ops>"), "Load rules and makes from an OPS5 source file"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("test <file.json>"), "Execute an external test case file"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("reset"), "Reset working memory, conflict set, and genatom"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("help"), "Show this help text"))
		b.WriteString(fmt.Sprintf("  %-32s %s\n", c("exit / quit"), "Exit the REPL (or Ctrl-D)"))
		b.WriteString("\n" + d("Keyboard: Tab=autocomplete, Up/Down=history, Ctrl-A/E=Home/End, Ctrl-K=kill to EOL") + "\n")

		fmt.Fprint(r.out, b.String())
		return
	}

	helpText := `
Commands:
  (p <name> ...)            Define a production rule (multiline supported)
  (literalize <c> ...)      Declare an element class schema with attributes
  (vector-attribute <a...>) Declare attribute(s) as multi-valued vector attributes
  vector-attributes         Display declared vector attributes
  make <cls> [^a v]         Assert a new Working Memory Element (e.g. make goal ^status active)
  modify <tag> [^a v]       Modify attributes of an existing WME by timetag
  remove <tag...> | *       Retract WME(s) by timetag or all WMEs (*)
  excise <rule...>          Evict production rules from memory and conflict set
  pm [<rule...> | *]        Print production rules in memory
  ppwm [<pattern>]          Print working memory elements matching pattern (e.g. ppwm City ^state PA)
  matches [<rule...> | *]   Display partial Rete matches and activations for rule(s)
  pbreak [<rule...>]        Set breakpoint on rule(s) or list active breakpoints
  unpbreak [<rule...> | *]  Remove breakpoint on rule(s) or clear all breakpoints
  openfile <log> <f> <m>    Open a file stream (modes: in, out, append)
  closefile <log>           Close an open file stream
  default <log> <subsys>    Set default stream for accept, write, or trace
  genatom                   Generate a unique symbolic atom (e.g. atom1)
  litval [<cls>] <attr>     Display the numeric index assigned to an attribute
  substr <elem> <start> <end> Extract a subsequence from a working memory element
  wm [class] [--table]      Display current working memory elements
  schemas [class] [--table] Display declared class schemas
  cs [--table]              Display conflict set (pending instantiations in salience order)
  status / info             Display system status overview
  table [on|off]            Toggle or view boxed table formatting mode
  clear / cls               Clear the terminal screen
  step                      Execute one Match-Resolve-Act cycle
  run [N]                   Run until quiescence, halt, or N cycles
  strategy [lex|mea]        View or switch conflict resolution strategy
  watch [0|1|2]             Display or set watch trace level (0=none, 1=firings, 2=firings+WM)
  trace on|off              Toggle cycle execution tracing
  load <file.ops>           Load and compile rules and makes from an OPS5 source file
  test <file.json>          Execute an external test case file
  reset                     Reset working memory and conflict set
  dot [<filepath>]          Export compiled Rete network in Graphviz .dot format
  help                      Show this help text
  exit / quit               Exit the REPL
`
	fmt.Fprint(r.out, helpText)
}

func (r *REPL) handleRemove(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) < 2 {
		fmt.Fprintln(r.out, "Usage: remove <timetag...> | remove *")
		return
	}

	if tokens[1] == "*" {
		removed := r.engine.RemoveAll()
		if len(removed) == 0 {
			fmt.Fprintln(r.out, "Working memory is already empty.")
		} else {
			fmt.Fprintf(r.out, "Removed all %d WMEs from working memory.\n", len(removed))
		}
		return
	}

	for _, arg := range tokens[1:] {
		timetag, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			fmt.Fprintf(r.out, "Invalid timetag: %s\n", arg)
			continue
		}
		removed, err := r.engine.Remove(timetag)
		if err != nil {
			fmt.Fprintf(r.out, "Error: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "Removed: %s\n", r.styler.FormatWME(removed))
		}
	}
}

func (r *REPL) handleExcise(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) < 2 {
		fmt.Fprintln(r.out, "Usage: excise <rule-name> [rule-name2 ...]")
		return
	}

	for _, name := range tokens[1:] {
		if r.engine.ExciseRule(name) {
			fmt.Fprintf(r.out, "Excised rule '%s'\n", name)
		} else {
			fmt.Fprintf(r.out, "Rule '%s' not found\n", name)
		}
	}
}

func (r *REPL) handlePM(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	ruleNames := tokens[1:]
	if len(ruleNames) == 0 || (len(ruleNames) == 1 && ruleNames[0] == "*") {
		rules := r.engine.Rules()
		if len(rules) == 0 {
			fmt.Fprintln(r.out, "No production rules in memory.")
			return
		}
		for _, rule := range rules {
			fmt.Fprintln(r.out, rule.String())
		}
		return
	}

	for _, name := range ruleNames {
		if rule := r.engine.Rule(name); rule != nil {
			fmt.Fprintln(r.out, rule.String())
		} else {
			fmt.Fprintf(r.out, "Rule '%s' not found\n", name)
		}
	}
}

func (r *REPL) handlePPWM(input string) {
	trimmed := strings.TrimSpace(input)
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
		fmt.Fprintf(r.out, "ppwm error: %v\n", err)
		return
	}
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range r.engine.Schemas() {
		p.RegisterSchema(s)
	}

	pattern, err := p.ParsePPWM()
	if err != nil {
		fmt.Fprintf(r.out, "ppwm error: %v\n", err)
		return
	}

	matches := r.engine.FindWMEsMatching(pattern)
	for _, w := range matches {
		fmt.Fprintln(r.out, r.styler.FormatWME(w))
	}
}

func (r *REPL) handleSubstr(input string) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(strings.ToLower(trimmed), "(substr") {
		// already has (substr
	} else if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = "(substr " + trimmed[1:]
	} else if strings.HasPrefix(strings.ToLower(trimmed), "substr ") || strings.ToLower(trimmed) == "substr" {
		trimmed = "(" + trimmed + ")"
	} else {
		trimmed = "(substr " + trimmed + ")"
	}

	p, err := parser.NewParser(trimmed)
	if err != nil {
		fmt.Fprintf(r.out, "substr error: %v\n", err)
		return
	}
	for _, va := range r.engine.VectorAttributes() {
		p.RegisterVectorAttribute(va)
	}
	for _, s := range r.engine.Schemas() {
		p.RegisterSchema(s)
	}

	val, err := p.ParseSubstr()
	if err != nil {
		fmt.Fprintf(r.out, "substr error: %v\n", err)
		return
	}

	res := r.engine.EvaluateSubstr(val.SubstrExpr(), nil)
	fmt.Fprintln(r.out, res.String())
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

func (r *REPL) handleWatch(input string) {
	norm := strings.TrimSpace(input)
	if strings.HasPrefix(norm, "(") && strings.HasSuffix(norm, ")") {
		norm = strings.TrimSpace(norm[1 : len(norm)-1])
	}
	parts := strings.Fields(norm)
	if len(parts) == 1 {
		fmt.Fprintf(r.out, "Current watch level: %d\n", r.engine.WatchLevel())
		return
	}
	if len(parts) == 2 {
		lvl, err := strconv.Atoi(parts[1])
		if err != nil || lvl < 0 || lvl > 2 {
			fmt.Fprintf(r.out, "Invalid watch level: %s (expected 0, 1, or 2)\n", parts[1])
			return
		}
		_ = r.engine.SetWatchLevel(lvl)
		fmt.Fprintf(r.out, "Watch level set to %d\n", lvl)
		return
	}
	fmt.Fprintln(r.out, "Usage: watch [0|1|2]")
}

func (r *REPL) handleMatches(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	ruleNames := tokens[1:]
	if len(ruleNames) == 0 || (len(ruleNames) == 1 && ruleNames[0] == "*") {
		reports := r.engine.Matches("*")
		if len(reports) == 0 {
			fmt.Fprintln(r.out, "No production rules in memory.")
			return
		}
		for i, rep := range reports {
			if i > 0 {
				fmt.Fprintln(r.out)
			}
			r.printRuleMatchReport(rep)
		}
		return
	}

	for i, name := range ruleNames {
		rep, ok := r.engine.RuleMatches(name)
		if !ok {
			fmt.Fprintf(r.out, "Rule '%s' not found\n", name)
			continue
		}
		if i > 0 {
			fmt.Fprintln(r.out)
		}
		r.printRuleMatchReport(rep)
	}
}

func (r *REPL) printRuleMatchReport(rep *engine.RuleMatchReport) {
	if rep == nil {
		return
	}
	header := fmt.Sprintf("** Matches for rule '%s' **", rep.RuleName)
	if r.styler.Enabled {
		fmt.Fprintln(r.out, r.styler.Header(header))
	} else {
		fmt.Fprintln(r.out, header)
	}

	for _, cond := range rep.Conditions {
		fmt.Fprintf(r.out, "Matches for Condition %d: %s\n", cond.Index, cond.Condition)
		if cond.IsTest {
			if r.styler.Enabled {
				fmt.Fprintf(r.out, "  %s\n", r.styler.Cyan("[Predicate Test]"))
			} else {
				fmt.Fprintln(r.out, "  [Predicate Test]")
			}
		} else if cond.IsNCC {
			if r.styler.Enabled {
				fmt.Fprintf(r.out, "  %s\n", r.styler.Yellow("[Negated Conjunction]"))
			} else {
				fmt.Fprintln(r.out, "  [Negated Conjunction]")
			}
		} else if cond.IsNegative {
			if len(cond.WMEs) == 0 {
				if r.styler.Enabled {
					fmt.Fprintf(r.out, "  %s\n", r.styler.Dim("None (no blocking WMEs)"))
				} else {
					fmt.Fprintln(r.out, "  None (no blocking WMEs)")
				}
			} else {
				for _, w := range cond.WMEs {
					if r.styler.Enabled {
						fmt.Fprintf(r.out, "  [%d] %s %s\n", w.Timetag, r.styler.FormatWME(w), r.styler.Red("(blocking WME)"))
					} else {
						fmt.Fprintf(r.out, "  [%d] %s (blocking WME)\n", w.Timetag, w.String())
					}
				}
			}
		} else {
			if len(cond.WMEs) == 0 {
				if r.styler.Enabled {
					fmt.Fprintf(r.out, "  %s\n", r.styler.Dim("None"))
				} else {
					fmt.Fprintln(r.out, "  None")
				}
			} else {
				for _, w := range cond.WMEs {
					if r.styler.Enabled {
						fmt.Fprintf(r.out, "  [%d] %s\n", w.Timetag, r.styler.FormatWME(w))
					} else {
						fmt.Fprintf(r.out, "  [%d] %s\n", w.Timetag, w.String())
					}
				}
			}
		}
	}

	if len(rep.PartialMatches) > 0 {
		if r.styler.Enabled {
			fmt.Fprintln(r.out, r.styler.Dim("--------------------------------------------------"))
		} else {
			fmt.Fprintln(r.out, "--------------------------------------------------")
		}
		for _, pm := range rep.PartialMatches {
			fmt.Fprintf(r.out, "Partial matches for CEs %s:\n", pm.CESpan)
			if len(pm.Timetags) == 0 {
				if r.styler.Enabled {
					fmt.Fprintf(r.out, "  %s\n", r.styler.Dim("None"))
				} else {
					fmt.Fprintln(r.out, "  None")
				}
			} else {
				for _, tags := range pm.Timetags {
					if r.styler.Enabled {
						fmt.Fprintf(r.out, "  %s\n", r.styler.BrightYellow(fmt.Sprintf("%v", tags)))
					} else {
						fmt.Fprintf(r.out, "  %v\n", tags)
					}
				}
			}
		}
	}

	if r.styler.Enabled {
		fmt.Fprintln(r.out, r.styler.Dim("--------------------------------------------------"))
	} else {
		fmt.Fprintln(r.out, "--------------------------------------------------")
	}
	fmt.Fprintln(r.out, "Activations:")
	if len(rep.Activations) == 0 {
		if r.styler.Enabled {
			fmt.Fprintf(r.out, "  %s\n", r.styler.Dim("None"))
		} else {
			fmt.Fprintln(r.out, "  None")
		}
	} else {
		for _, act := range rep.Activations {
			if r.styler.Enabled {
				fmt.Fprintf(r.out, "  %s\n", r.styler.BrightGreen(fmt.Sprintf("%v", act)))
			} else {
				fmt.Fprintf(r.out, "  %v\n", act)
			}
		}
	}
}

func (r *REPL) handlePBreak(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	ruleNames := tokens[1:]
	if len(ruleNames) == 0 {
		r.printBreakpoints()
		return
	}

	for _, name := range ruleNames {
		if r.engine.Rule(name) == nil {
			fmt.Fprintf(r.out, "Warning: rule '%s' not found in production memory\n", name)
		}
		r.engine.SetBreakpoint(name)
		fmt.Fprintf(r.out, "Breakpoint set on rule '%s'\n", name)
	}
}

func (r *REPL) handleUnpbreak(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	ruleNames := tokens[1:]
	if len(ruleNames) == 0 || (len(ruleNames) == 1 && (ruleNames[0] == "*" || strings.ToLower(ruleNames[0]) == "nil")) {
		r.engine.ClearBreakpoints()
		fmt.Fprintln(r.out, "All rule breakpoints cleared.")
		return
	}

	for _, name := range ruleNames {
		r.engine.RemoveBreakpoint(name)
		fmt.Fprintf(r.out, "Breakpoint removed from rule '%s'\n", name)
	}
}

func (r *REPL) printBreakpoints() {
	bps := r.engine.Breakpoints()
	if len(bps) == 0 {
		fmt.Fprintln(r.out, "No breakpoints set.")
		return
	}
	fmt.Fprintf(r.out, "Breakpoints (%d):\n", len(bps))
	for _, bp := range bps {
		if r.styler.Enabled {
			fmt.Fprintf(r.out, "  %s\n", r.styler.Bold(bp))
		} else {
			fmt.Fprintf(r.out, "  %s\n", bp)
		}
	}
}

func (r *REPL) handleDOT(input string) {
	tokens, err := tokenizeLine(input)
	if err != nil {
		fmt.Fprintf(r.out, "Parse error: %v\n", err)
		return
	}
	if len(tokens) <= 1 {
		if err := r.engine.ExportDOT(r.out); err != nil {
			fmt.Fprintf(r.out, "Error exporting DOT: %v\n", err)
		}
		return
	}

	filePath := tokens[1]
	f, err := os.Create(filePath)
	if err != nil {
		fmt.Fprintf(r.out, "Failed to create file %s: %v\n", filePath, err)
		return
	}
	defer f.Close()

	if err := r.engine.ExportDOT(f); err != nil {
		fmt.Fprintf(r.out, "Error exporting DOT to %s: %v\n", filePath, err)
		return
	}
	fmt.Fprintf(r.out, "Exported Rete network graph to %s\n", filePath)
}
