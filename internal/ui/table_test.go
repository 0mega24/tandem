package ui

import (
	"bytes"
	"errors"
	"testing"
)

func TestTable(t *testing.T) {
	var b bytes.Buffer
	tb := NewTable(&b)
	tb.Row("api", "main", "✓ clean")
	tb.Row("myorg/web", "feature", "! 2 changed", "")
	if err := tb.Flush(); err != nil {
		t.Fatal(err)
	}

	want := "api        main     ✓ clean\nmyorg/web  feature  ! 2 changed\n"
	if b.String() != want {
		t.Errorf("table =\n%q\nwant\n%q", b.String(), want)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestTableReportsWriteError(t *testing.T) {
	tb := NewTable(failWriter{})
	tb.Row("a", "b")
	if err := tb.Flush(); err == nil {
		t.Error("Flush() = nil, want the write error")
	}
}
