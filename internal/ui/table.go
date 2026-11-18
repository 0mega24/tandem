// Package ui formats terminal output for the CLI.
package ui

import (
	"io"
	"strings"
	"text/tabwriter"
)

// Table writes aligned columns. It keeps the first write error, and Flush returns it, so a
// command checks one error instead of one per line.
type Table struct {
	w   *tabwriter.Writer
	err error
}

// NewTable returns a Table with two spaces between columns.
func NewTable(w io.Writer) *Table {
	return &Table{w: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

// Row writes one line. Trailing empty cells are left out, so a short row has no trailing spaces.
func (t *Table) Row(cells ...string) {
	if t.err != nil {
		return
	}

	for len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}

	_, t.err = io.WriteString(t.w, strings.Join(cells, "\t")+"\n")
}

// Flush writes the aligned table and returns the first error.
func (t *Table) Flush() error {
	if t.err != nil {
		return t.err
	}
	return t.w.Flush()
}
