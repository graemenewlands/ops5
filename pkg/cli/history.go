package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// History manages command history for the REPL with navigation and file persistence.
type History struct {
	mu       sync.Mutex
	entries  []string
	cursor   int
	filePath string
	maxSize  int
}

// NewHistory initializes a new history manager.
func NewHistory(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 1000
	}
	h := &History{
		entries: make([]string, 0),
		cursor:  0,
		maxSize: maxSize,
	}

	if home, err := os.UserHomeDir(); err == nil {
		h.filePath = filepath.Join(home, ".ops5_history")
		h.load()
	}
	return h
}

func (h *History) load() {
	if h.filePath == "" {
		return
	}
	f, err := os.Open(h.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			h.entries = append(h.entries, line)
		}
	}
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	h.cursor = len(h.entries)
}

// Save persists the current history entries to disk.
func (h *History) Save() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.filePath == "" || len(h.entries) == 0 {
		return
	}
	f, err := os.Create(h.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range h.entries {
		w.WriteString(entry + "\n")
	}
	w.Flush()
}

// Add appends a command to history if it's non-empty and not a duplicate of the last entry.
func (h *History) Add(cmd string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return
	}
	// Avoid consecutive duplicates
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == trimmed {
		h.cursor = len(h.entries)
		return
	}

	h.entries = append(h.entries, trimmed)
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[1:]
	}
	h.cursor = len(h.entries)
}

// ResetCursor resets the navigation cursor to the end of history.
func (h *History) ResetCursor() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cursor = len(h.entries)
}

// Previous moves the history cursor backward and returns the entry.
func (h *History) Previous() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) == 0 || h.cursor <= 0 {
		if len(h.entries) > 0 && h.cursor == 0 {
			return h.entries[0], true
		}
		return "", false
	}

	h.cursor--
	return h.entries[h.cursor], true
}

// Next moves the history cursor forward and returns the entry.
func (h *History) Next() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cursor >= len(h.entries)-1 {
		h.cursor = len(h.entries)
		return "", false
	}

	h.cursor++
	return h.entries[h.cursor], true
}

// Entries returns a copy of all current history entries.
func (h *History) Entries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	copied := make([]string, len(h.entries))
	copy(copied, h.entries)
	return copied
}
