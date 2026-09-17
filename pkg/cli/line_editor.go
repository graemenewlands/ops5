package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// LineEditor handles interactive terminal input with raw mode, readline shortcuts, history, and tab completion.
type LineEditor struct {
	in        io.Reader
	out       io.Writer
	styler    *Styler
	history   *History
	completer *Completer
	bufReader *bufio.Reader
}

// NewLineEditor creates a LineEditor.
func NewLineEditor(in io.Reader, out io.Writer, styler *Styler, history *History, completer *Completer) *LineEditor {
	return &LineEditor{
		in:        in,
		out:       out,
		styler:    styler,
		history:   history,
		completer: completer,
		bufReader: bufio.NewReader(in),
	}
}

// SetBufReader overrides the internal buffered reader (e.g. to share with engine).
func (le *LineEditor) SetBufReader(br *bufio.Reader) {
	if br != nil {
		le.bufReader = br
	}
}

// IsTerminal returns true if the editor input and output are interactive terminals.
func (le *LineEditor) IsTerminal() bool {
	f, ok := le.in.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ReadLine reads a line with interactive editing if terminal, or plain line reading if not.
func (le *LineEditor) ReadLine(prompt string) (string, error) {
	if !le.IsTerminal() {
		fmt.Fprint(le.out, prompt)
		line, err := le.bufReader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	f := le.in.(*os.File)
	fd := int(f.Fd())

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprint(le.out, prompt)
		line, err := le.bufReader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	defer term.Restore(fd, oldState)

	fmt.Fprint(le.out, prompt)

	var runes []rune
	cursor := 0
	promptWidth := VisualWidth(prompt)

	redraw := func() {
		// Return to column 0, redraw prompt and input, erase trailing characters
		fmt.Fprintf(le.out, "\r%s%s\033[K", prompt, string(runes))
		// Position cursor
		targetCol := promptWidth + cursor
		fmt.Fprint(le.out, "\r")
		if targetCol > 0 {
			fmt.Fprintf(le.out, "\033[%dC", targetCol)
		}
	}

	buf := make([]byte, 16)
	le.history.ResetCursor()

	for {
		n, err := f.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}

		// Check for escape sequences
		if buf[0] == '\x1b' {
			if n >= 3 && buf[1] == '[' {
				switch buf[2] {
				case 'A': // Up arrow: previous history
					if prev, ok := le.history.Previous(); ok {
						runes = []rune(prev)
						cursor = len(runes)
						redraw()
					}
				case 'B': // Down arrow: next history
					if next, ok := le.history.Next(); ok {
						runes = []rune(next)
						cursor = len(runes)
						redraw()
					} else {
						runes = nil
						cursor = 0
						redraw()
					}
				case 'C': // Right arrow
					if cursor < len(runes) {
						cursor++
						redraw()
					}
				case 'D': // Left arrow
					if cursor > 0 {
						cursor--
						redraw()
					}
				case 'H': // Home
					cursor = 0
					redraw()
				case 'F': // End
					cursor = len(runes)
					redraw()
				case '3': // Delete
					if n >= 4 && buf[3] == '~' && cursor < len(runes) {
						runes = append(runes[:cursor], runes[cursor+1:]...)
						redraw()
					}
				case '1': // Home on some terminals
					if n >= 4 && buf[3] == '~' {
						cursor = 0
						redraw()
					}
				case '4': // End on some terminals
					if n >= 4 && buf[3] == '~' {
						cursor = len(runes)
						redraw()
					}
				}
			}
			continue
		}

		b := buf[0]

		switch b {
		case '\r', '\n': // Enter
			fmt.Fprint(le.out, "\r\n")
			result := string(runes)
			if strings.TrimSpace(result) != "" {
				le.history.Add(result)
			}
			return result, nil

		case '\x03': // Ctrl-C
			fmt.Fprint(le.out, "^C\r\n")
			return "", nil

		case '\x04': // Ctrl-D (EOF)
			if len(runes) == 0 {
				fmt.Fprint(le.out, "\r\n")
				return "", io.EOF
			}

		case '\x7f', '\x08': // Backspace
			if cursor > 0 {
				runes = append(runes[:cursor-1], runes[cursor:]...)
				cursor--
				redraw()
			}

		case '\x01': // Ctrl-A (Home)
			cursor = 0
			redraw()

		case '\x05': // Ctrl-E (End)
			cursor = len(runes)
			redraw()

		case '\x0b': // Ctrl-K (Kill to end of line)
			runes = runes[:cursor]
			redraw()

		case '\x15': // Ctrl-U (Clear line)
			runes = nil
			cursor = 0
			redraw()

		case '\x0c': // Ctrl-L (Clear screen)
			fmt.Fprint(le.out, "\033[H\033[2J")
			redraw()

		case '\t': // Tab completion
			if le.completer != nil {
				currentLine := string(runes[:cursor])
				candidates, prefix := le.completer.Complete(currentLine)
				if len(candidates) == 1 {
					// Insert remaining suffix of match
					match := candidates[0]
					suffix := match[len(prefix):]
					for _, r := range suffix {
						runes = append(runes[:cursor], append([]rune{r}, runes[cursor:]...)...)
						cursor++
					}
					// Add trailing space for complete command
					runes = append(runes[:cursor], append([]rune{' '}, runes[cursor:]...)...)
					cursor++
					redraw()
				} else if len(candidates) > 1 {
					// Display candidate options neatly below
					fmt.Fprint(le.out, "\r\n")
					var formatted []string
					for _, c := range candidates {
						if le.styler != nil {
							formatted = append(formatted, le.styler.Cyan(c))
						} else {
							formatted = append(formatted, c)
						}
					}
					fmt.Fprintf(le.out, "  %s\r\n", strings.Join(formatted, "   "))
					redraw()
				}
			}

		default:
			// Insert character
			if b >= 32 {
				// Decode UTF-8 sequence from buf
				r := rune(b)
				runes = append(runes[:cursor], append([]rune{r}, runes[cursor:]...)...)
				cursor++
				redraw()
			}
		}
	}
}
