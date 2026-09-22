package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"ops5/pkg/cli"
	"ops5/pkg/conflict"
	"ops5/pkg/harness"
)

func main() {
	strategyFlag := flag.String("strategy", "lex", "Conflict resolution strategy: 'lex' or 'mea'")
	watchFlag := flag.Int("watch", 1, "Watch trace level: 0 (none), 1 (firings), 2 (firings+WM)")
	traceFlag := flag.Bool("trace", false, "Enable cycle tracing (sets watch=1)")
	interactiveFlag := flag.Bool("i", false, "Drop into interactive REPL after loading file")
	maxCyclesFlag := flag.Int("max-cycles", 1000, "Maximum number of cycles for batch run")
	colorFlag := flag.String("color", "auto", "Terminal color output: 'auto', 'always', or 'never'")
	tableFlag := flag.Bool("table", false, "Display working memory, conflict set, and schemas in boxed tables")
	dotFlag := flag.String("dot", "", "Export compiled Rete network to Graphviz .dot file")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "OPS5 Production Rule System (Go)\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  ops5 [flags]                 Launch interactive CLI REPL\n")
		fmt.Fprintf(os.Stderr, "  ops5 [flags] <file.ops>      Load and execute an OPS5 source file\n")
		fmt.Fprintf(os.Stderr, "  ops5 test <file.json>        Run an external test case file\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	args := flag.Args()

	// Subcommand: test <file.json>
	if len(args) >= 2 && args[0] == "test" {
		runner := harness.NewRunner()
		for _, testPath := range args[1:] {
			tc, err := runner.LoadTestCaseFromJSON(testPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", testPath, err)
				os.Exit(1)
			}
			fmt.Printf("Running test: %s\n", tc.Name)
			res := runner.Run(tc)
			if res.Passed {
				fmt.Printf("PASS: %s (cycles: %d)\n", tc.Name, res.CyclesRan)
				if res.Output != "" {
					fmt.Print(res.Output)
				}
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", tc.Name, res.Error)
				if res.Output != "" {
					fmt.Print(res.Output)
				}
				os.Exit(1)
			}
		}
		return
	}

	repl := cli.NewREPL(os.Stdin, os.Stdout)
	switch strings.ToLower(*colorFlag) {
	case "always", "true", "1", "yes":
		repl.SetColor(true)
	case "never", "false", "0", "no":
		repl.SetColor(false)
	default: // "auto"
		// Autodetected based on terminal capability
	}
	if *tableFlag {
		repl.SetTableMode(true)
	}

	if strings.ToLower(*strategyFlag) == "mea" {
		repl.Engine().SetStrategy(conflict.StrategyMEA)
	} else {
		repl.Engine().SetStrategy(conflict.StrategyLEX)
	}
	_ = repl.Engine().SetWatchLevel(*watchFlag)
	if *traceFlag {
		_ = repl.Engine().SetWatchLevel(1)
	}

	exportDOT := func() {
		if *dotFlag != "" {
			f, err := os.Create(*dotFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create DOT file %s: %v\n", *dotFlag, err)
				os.Exit(1)
			}
			defer f.Close()
			if err := repl.Engine().ExportDOT(f); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to export DOT graph: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// If a file is provided as argument
	if len(args) >= 1 && args[0] != "test" {
		filePath := args[0]
		if err := repl.LoadFile(filePath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load %s: %v\n", filePath, err)
			os.Exit(1)
		}

		exportDOT()

		// If not interactive, run to completion and exit
		if !*interactiveFlag {
			cycles, err := repl.Engine().Run(*maxCyclesFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
				os.Exit(1)
			}
			repl.Engine().EnsureNewline()
			if repl.Engine().IsHalted() {
				fmt.Printf("Halted after %d cycles.\n", cycles)
			} else {
				fmt.Printf("Quiescence reached after %d cycles.\n", cycles)
			}
			return
		}
	} else if *dotFlag != "" {
		exportDOT()
		if !*interactiveFlag {
			return
		}
	}

	// Launch interactive REPL
	repl.Start()
}
