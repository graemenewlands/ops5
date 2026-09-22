package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"ops5/pkg/model"
	"golang.org/x/term"
)

// ANSI escape code constants
const (
	ansiReset        = "\033[0m"
	ansiBold         = "\033[1m"
	ansiDim          = "\033[2m"
	ansiUnderline    = "\033[4m"
	ansiRed          = "\033[31m"
	ansiGreen        = "\033[32m"
	ansiYellow       = "\033[33m"
	ansiBlue         = "\033[34m"
	ansiMagenta      = "\033[35m"
	ansiCyan         = "\033[36m"
	ansiWhite        = "\033[37m"
	ansiGray         = "\033[90m"
	ansiBrightRed    = "\033[91m"
	ansiBrightGreen  = "\033[92m"
	ansiBrightYellow = "\033[93m"
	ansiBrightBlue   = "\033[94m"
	ansiBrightMagenta= "\033[95m"
	ansiBrightCyan   = "\033[96m"
	ansiBrightWhite  = "\033[97m"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes all ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// VisualWidth returns the printable character width of a string.
func VisualWidth(s string) int {
	return len([]rune(StripANSI(s)))
}

// Styler provides formatting and syntax-highlighting routines with color toggle.
type Styler struct {
	Enabled bool
}

// NewStyler creates a new Styler, automatically detecting terminal capability if out is a file descriptor.
func NewStyler(out io.Writer) *Styler {
	enabled := false
	if f, ok := out.(*os.File); ok {
		enabled = term.IsTerminal(int(f.Fd()))
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		enabled = false
	}
	return &Styler{Enabled: enabled}
}

// Wrap applies an ANSI code if styling is enabled.
func (s *Styler) Wrap(code, text string) string {
	if !s.Enabled || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (s *Styler) Bold(text string) string         { return s.Wrap(ansiBold, text) }
func (s *Styler) Dim(text string) string          { return s.Wrap(ansiDim, text) }
func (s *Styler) Red(text string) string          { return s.Wrap(ansiRed, text) }
func (s *Styler) Green(text string) string        { return s.Wrap(ansiGreen, text) }
func (s *Styler) Yellow(text string) string       { return s.Wrap(ansiYellow, text) }
func (s *Styler) Blue(text string) string         { return s.Wrap(ansiBlue, text) }
func (s *Styler) Magenta(text string) string      { return s.Wrap(ansiMagenta, text) }
func (s *Styler) Cyan(text string) string         { return s.Wrap(ansiCyan, text) }
func (s *Styler) Gray(text string) string         { return s.Wrap(ansiGray, text) }
func (s *Styler) BrightRed(text string) string    { return s.Wrap(ansiBrightRed, text) }
func (s *Styler) BrightGreen(text string) string  { return s.Wrap(ansiBrightGreen, text) }
func (s *Styler) BrightYellow(text string) string  { return s.Wrap(ansiBrightYellow, text) }
func (s *Styler) BrightMagenta(text string) string { return s.Wrap(ansiBrightMagenta, text) }
func (s *Styler) BrightCyan(text string) string    { return s.Wrap(ansiBrightCyan, text) }

func (s *Styler) Prompt() string {
	if !s.Enabled {
		return "ops5> "
	}
	return ansiBold + ansiBrightCyan + "ops5" + ansiReset + ansiBrightYellow + "> " + ansiReset
}

func (s *Styler) ContinuationPrompt() string {
	if !s.Enabled {
		return "...   "
	}
	return ansiDim + "...   " + ansiReset
}

func (s *Styler) Success(text string) string {
	return s.Wrap(ansiBrightGreen, text)
}

func (s *Styler) Error(text string) string {
	return s.Wrap(ansiBrightRed, text)
}

func (s *Styler) Warning(text string) string {
	return s.Wrap(ansiBrightYellow, text)
}

func (s *Styler) Header(text string) string {
	return s.Wrap(ansiBold+ansiBrightWhite, text)
}

// FormatValue highlights an individual model.Value according to its type.
func (s *Styler) FormatValue(v model.Value) string {
	if !s.Enabled {
		return v.String()
	}

	switch v.Type() {
	case model.TypeInteger, model.TypeFloat:
		return s.Wrap(ansiBrightYellow, v.String())
	case model.TypeString:
		return s.Wrap(ansiBrightGreen, v.String())
	case model.TypeBoolean:
		return s.Wrap(ansiBrightMagenta, v.String())
	case model.TypeVariable:
		return s.Wrap(ansiBrightBlue, v.String())
	case model.TypeSymbol:
		if strings.EqualFold(v.String(), "nil") {
			return s.Wrap(ansiDim+ansiGray, "nil")
		}
		return s.Wrap(ansiBrightWhite, v.String())
	case model.TypeVector:
		elems := v.VectorElements()
		parts := make([]string, len(elems))
		for i, el := range elems {
			parts[i] = s.FormatValue(el)
		}
		return strings.Join(parts, " ")
	default:
		return v.String()
	}
}

// FormatWME highlights a Working Memory Element: (tag: class ^attr1 val1 ...).
func (s *Styler) FormatWME(w *model.WME) string {
	if !s.Enabled || w == nil {
		return w.String()
	}

	var keys []string
	for k := range w.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(s.Dim("("))
	b.WriteString(s.Wrap(ansiBold+ansiBrightYellow, fmt.Sprintf("%d:", w.Timetag)))
	b.WriteString(" ")
	b.WriteString(s.Wrap(ansiBold+ansiBrightMagenta, w.Class))

	for _, k := range keys {
		b.WriteString(" ")
		b.WriteString(s.Wrap(ansiBrightCyan, "^"+k))
		b.WriteString(" ")
		b.WriteString(s.FormatValue(w.Attributes[k]))
	}

	b.WriteString(s.Dim(")"))
	return b.String()
}
