package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ops5/pkg/engine"
)

// Completer generates context-aware autocompletions for the OPS5 REPL.
type Completer struct {
	engine *engine.Engine
}

var baseCommands = []string{
	"make", "modify", "remove", "run", "step", "wm", "cs", "schemas",
	"ppwm", "strategy", "watch", "trace", "openfile", "closefile",
	"default", "genatom", "litval", "substr", "excise", "pm",
	"matches", "pbreak", "unpbreak", "unbreak",
	"load", "test", "reset", "status", "clear", "help", "exit", "quit",
}

// NewCompleter creates a Completer backed by the OPS5 engine.
func NewCompleter(eng *engine.Engine) *Completer {
	return &Completer{engine: eng}
}

// Complete returns candidate completions and the prefix being completed.
func (c *Completer) Complete(line string) (candidates []string, prefix string) {
	trimmed := strings.TrimLeft(line, " \t")
	hasParen := strings.HasPrefix(trimmed, "(")
	if hasParen {
		trimmed = strings.TrimPrefix(trimmed, "(")
	}

	parts := strings.Fields(trimmed)
	endsWithSpace := strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t")

	// 1. Completing the top-level command
	if len(parts) == 0 || (len(parts) == 1 && !endsWithSpace) {
		currentWord := ""
		if len(parts) == 1 {
			currentWord = parts[0]
		}
		for _, cmd := range baseCommands {
			if strings.HasPrefix(strings.ToLower(cmd), strings.ToLower(currentWord)) {
				candidates = append(candidates, cmd)
			}
		}
		sort.Strings(candidates)
		return candidates, currentWord
	}

	verb := strings.ToLower(parts[0])

	// Current word being typed
	currentWord := ""
	if !endsWithSpace {
		currentWord = parts[len(parts)-1]
	}

	// 2. Completing attributes starting with '^'
	if strings.HasPrefix(currentWord, "^") {
		attrPrefix := strings.TrimPrefix(currentWord, "^")
		attrCandidates := c.completeAttributes(parts, attrPrefix)
		for _, a := range attrCandidates {
			candidates = append(candidates, "^"+a)
		}
		sort.Strings(candidates)
		return candidates, currentWord
	}

	// 3. Command-specific completions
	switch verb {
	case "make", "wm", "ppwm", "schemas", "schema":
		if (len(parts) == 1 && endsWithSpace) || (len(parts) == 2 && !endsWithSpace) {
			candidates = c.completeClasses(currentWord)
			return candidates, currentWord
		}

	case "strategy":
		for _, opt := range []string{"lex", "mea"} {
			if strings.HasPrefix(opt, strings.ToLower(currentWord)) {
				candidates = append(candidates, opt)
			}
		}
		return candidates, currentWord

	case "watch":
		for _, opt := range []string{"0", "1", "2"} {
			if strings.HasPrefix(opt, currentWord) {
				candidates = append(candidates, opt)
			}
		}
		return candidates, currentWord

	case "trace":
		for _, opt := range []string{"on", "off"} {
			if strings.HasPrefix(opt, strings.ToLower(currentWord)) {
				candidates = append(candidates, opt)
			}
		}
		return candidates, currentWord

	case "excise", "pm", "matches", "pbreak", "unpbreak", "unbreak":
		if c.engine != nil {
			for _, r := range c.engine.Rules() {
				if strings.HasPrefix(strings.ToLower(r.Name), strings.ToLower(currentWord)) {
					candidates = append(candidates, r.Name)
				}
			}
		}
		sort.Strings(candidates)
		return candidates, currentWord

	case "load":
		candidates = completeFilePaths(currentWord, []string{".ops", ".ops5"})
		return candidates, currentWord

	case "test":
		candidates = completeFilePaths(currentWord, []string{".json"})
		return candidates, currentWord
	}

	return nil, ""
}

func (c *Completer) completeClasses(prefix string) []string {
	seen := make(map[string]bool)
	var res []string

	if c.engine != nil {
		for _, s := range c.engine.Schemas() {
			if !seen[s.Class] && strings.HasPrefix(strings.ToLower(s.Class), strings.ToLower(prefix)) {
				res = append(res, s.Class)
				seen[s.Class] = true
			}
		}
		for _, w := range c.engine.WorkingMemory().All() {
			if !seen[w.Class] && strings.HasPrefix(strings.ToLower(w.Class), strings.ToLower(prefix)) {
				res = append(res, w.Class)
				seen[w.Class] = true
			}
		}
	}
	sort.Strings(res)
	return res
}

func (c *Completer) completeAttributes(parts []string, prefix string) []string {
	seen := make(map[string]bool)
	var res []string

	var targetClass string
	if len(parts) >= 2 {
		targetClass = strings.ToLower(parts[1])
	}

	if c.engine != nil {
		for _, s := range c.engine.Schemas() {
			if targetClass == "" || strings.EqualFold(s.Class, targetClass) {
				for _, a := range s.Attributes {
					if !seen[a] && strings.HasPrefix(strings.ToLower(a), strings.ToLower(prefix)) {
						res = append(res, a)
						seen[a] = true
					}
				}
			}
		}
	}
	sort.Strings(res)
	return res
}

func completeFilePaths(prefix string, extensions []string) []string {
	dir := "."
	filePrefix := prefix
	if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
		dir = prefix[:idx]
		filePrefix = prefix[idx+1:]
		if dir == "" {
			dir = "/"
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var res []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(filePrefix)) {
			full := filepath.Join(dir, name)
			if dir == "." && !strings.HasPrefix(prefix, "./") {
				full = name
			}
			if entry.IsDir() {
				res = append(res, full+"/")
			} else {
				for _, ext := range extensions {
					if strings.HasSuffix(strings.ToLower(name), ext) {
						res = append(res, full)
						break
					}
				}
			}
		}
	}
	sort.Strings(res)
	return res
}
