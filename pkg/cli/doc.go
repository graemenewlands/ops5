// Package cli provides the interactive terminal REPL, line editor, auto-completion, and diagnostic tools for OPS5.
//
// The CLI package delivers a complete interactive developer environment for inspecting, running, and debugging
// OPS5 production rule systems directly from a terminal emulator.
//
// # Major Components
//
//   - REPL: The interactive Read-Eval-Print Loop. Manages command parsing, cycle stepping, output formatting,
//     and integration with the underlying engine.Engine instance.
//
//   - LineEditor: Terminal line editor operating in raw mode, offering rich readline capabilities including
//     cursor motion, line kill/yank, history navigation, and tab auto-completion without external C dependencies.
//
//   - Completer: Context-aware tab completer that suggests REPL commands, registered rule names, declared class schemas,
//     attribute names, conflict resolution strategies, watch levels, and local filesystem paths.
//
//   - Table: Formats working memory elements, conflict sets, and element class schemas into clean, aligned
//     Unicode or ASCII boxed tables.
//
//   - Styler: Terminal ANSI color styler supporting 'auto', 'always', and 'never' modes with smart TTY detection.
//
// # Supported Interactive Commands
//
//   - Execution: run [N], step [N], halt
//   - Working Memory: wm, ppwm [pattern], make ..., modify <id> ..., remove <id>, clear
//   - Rule Inspection: pm [rule], matches [rule], excise <rule>, dot [file]
//   - Breakpoints: pbreak [rule], pbreak -d <rule>, pbreak -c
//   - Conflict Set: cs, strategy [lex|mea]
//   - Tracing: watch [0|1|2], trace [on|off]
//   - Schemas: schema [class]
//
// # Example Usage
//
//	// Create and launch an interactive REPL connected to standard I/O:
//	repl := cli.NewREPL(os.Stdin, os.Stdout)
//	if err := repl.Run(); err != nil {
//	    log.Fatalf("REPL terminated with error: %v", err)
//	}
package cli
