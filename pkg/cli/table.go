package cli

import (
	"strings"
)

// Table renders formatted ASCII or Unicode boxed tables with ANSI-aware column width calculation.
type Table struct {
	Headers []string
	Rows    [][]string
	styler  *Styler
	Unicode bool
}

// NewTable creates a new Table.
func NewTable(styler *Styler) *Table {
	unicode := true
	if styler != nil && !styler.Enabled {
		unicode = false
	}
	return &Table{
		styler:  styler,
		Unicode: unicode,
	}
}

// SetHeaders sets table column headers.
func (t *Table) SetHeaders(headers ...string) {
	t.Headers = headers
}

// AddRow adds a row to the table.
func (t *Table) AddRow(cols ...string) {
	t.Rows = append(t.Rows, cols)
}

// Render returns the formatted string representation of the table.
func (t *Table) Render() string {
	numCols := len(t.Headers)
	for _, row := range t.Rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return ""
	}

	colWidths := make([]int, numCols)
	for i, h := range t.Headers {
		w := VisualWidth(h)
		if w > colWidths[i] {
			colWidths[i] = w
		}
	}
	for _, row := range t.Rows {
		for i, col := range row {
			w := VisualWidth(col)
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	// Add 1 char padding on both sides
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	var b strings.Builder

	// Box drawing characters
	var tl, tm, tr, bl, bm, br, ml, mm, mr, hLine, vLine string
	if t.Unicode {
		tl, tm, tr = "┌", "┬", "┐"
		bl, bm, br = "└", "┴", "┘"
		ml, mm, mr = "├", "┼", "┤"
		hLine, vLine = "─", "│"
	} else {
		tl, tm, tr = "+", "+", "+"
		bl, bm, br = "+", "+", "+"
		ml, mm, mr = "+", "+", "+"
		hLine, vLine = "-", "|"
	}

	dim := func(s string) string {
		if t.styler != nil {
			return t.styler.Dim(s)
		}
		return s
	}
	headerStyle := func(s string) string {
		if t.styler != nil {
			return t.styler.Header(s)
		}
		return s
	}

	// Top border
	b.WriteString(dim(tl))
	for i, w := range colWidths {
		b.WriteString(dim(strings.Repeat(hLine, w+2)))
		if i < numCols-1 {
			b.WriteString(dim(tm))
		}
	}
	b.WriteString(dim(tr) + "\n")

	// Headers
	if len(t.Headers) > 0 {
		b.WriteString(dim(vLine))
		for i := 0; i < numCols; i++ {
			text := ""
			if i < len(t.Headers) {
				text = t.Headers[i]
			}
			pad := colWidths[i] - VisualWidth(text)
			b.WriteString(" " + headerStyle(text) + strings.Repeat(" ", pad+1))
			b.WriteString(dim(vLine))
		}
		b.WriteString("\n")

		// Middle separator
		b.WriteString(dim(ml))
		for i, w := range colWidths {
			b.WriteString(dim(strings.Repeat(hLine, w+2)))
			if i < numCols-1 {
				b.WriteString(dim(mm))
			}
		}
		b.WriteString(dim(mr) + "\n")
	}

	// Data rows
	for _, row := range t.Rows {
		b.WriteString(dim(vLine))
		for i := 0; i < numCols; i++ {
			text := ""
			if i < len(row) {
				text = row[i]
			}
			pad := colWidths[i] - VisualWidth(text)
			if pad < 0 {
				pad = 0
			}
			b.WriteString(" " + text + strings.Repeat(" ", pad+1))
			b.WriteString(dim(vLine))
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString(dim(bl))
	for i, w := range colWidths {
		b.WriteString(dim(strings.Repeat(hLine, w+2)))
		if i < numCols-1 {
			b.WriteString(dim(bm))
		}
	}
	b.WriteString(dim(br) + "\n")

	return b.String()
}
