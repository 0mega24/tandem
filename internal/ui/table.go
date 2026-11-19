// Package ui formats terminal output for the CLI.
package ui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"
)

// Table writes aligned columns. It keeps the first write error, and Flush returns it, so a
// command checks one error instead of one per line.
type Table struct {
	w   *tabwriter.Writer
	err error

	// With color, the aligned text goes to buf first and Flush colors it on the way to out.
	color bool
	buf   bytes.Buffer
	out   io.Writer
}

// NewTable returns a Table with two spaces between columns. It colors the marks when w is a
// terminal (see Color).
func NewTable(w io.Writer) *Table { return newTable(w, Color(w)) }

func newTable(w io.Writer, color bool) *Table {
	t := &Table{color: color, out: w}
	if color {
		t.w = tabwriter.NewWriter(&t.buf, 0, 0, 2, ' ', 0)
	} else {
		t.w = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	}
	return t
}

// Row writes one line. Trailing empty cells are left out, so a short row has no trailing spaces.
func (t *Table) Row(cells ...string) {
	if t.err != nil {
		return
	}

	for len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}

	for i, c := range cells {
		cells[i] = escape(c)
	}

	_, t.err = io.WriteString(t.w, strings.Join(cells, "\t")+"\n")
}

// escape writes control characters as \xNN. Cells hold text from repos and git (branch names,
// error messages), and a tab, newline or escape sequence in one would break the table or the terminal.
func escape(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}

	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			fmt.Fprintf(&b, "\\x%02x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Flush writes the aligned table and returns the first error.
func (t *Table) Flush() error {
	if t.err != nil {
		return t.err
	}
	if err := t.w.Flush(); err != nil || !t.color {
		return err
	}
	var b strings.Builder
	for line := range strings.Lines(t.buf.String()) {
		text := strings.TrimSuffix(line, "\n")
		b.WriteString(colorLine(text) + line[len(text):])
	}
	t.buf.Reset()
	_, err := io.WriteString(t.out, b.String())
	return err
}
