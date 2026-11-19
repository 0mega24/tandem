package ui

import (
	"io"
	"os"
	"regexp"
	"strings"
)

// The basic ANSI colors, so the output uses the colors of the user's terminal theme.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	faint  = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// markCell finds a cell that starts with a mark: at the line start or after the two spaces
// between columns, up to the next two spaces or the line end.
var markCell = regexp.MustCompile(`(^|  )((?:[✓!-] |[↑↓]\d)(?:[^ ]| [^ ])*)`)

// Color reports whether to color output to w: w is a terminal, NO_COLOR is not set and TERM is
// not dumb. Pipes, files and test buffers get plain text.
func Color(w io.Writer) bool {
	return colorEnv() && Terminal(w)
}

// Terminal reports whether v is a file on a terminal (an *os.File that is a char device).
func Terminal(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func colorEnv() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// colorLine colors the mark cells of one aligned table line. It runs after the alignment,
// because tabwriter would count the escape codes as text.
func colorLine(line string) string {
	return markCell.ReplaceAllStringFunc(line, func(m string) string {
		cell := strings.TrimLeft(m, " ")
		pad := m[:len(m)-len(cell)]
		c := green
		switch {
		case strings.HasPrefix(cell, "!"):
			c = yellow
		case strings.HasPrefix(cell, "-"):
			c = faint
		case strings.HasPrefix(cell, "↑"), strings.HasPrefix(cell, "↓"):
			c = cyan
		}
		return pad + c + cell + reset
	})
}

// ColorDiff colors a unified diff from Diff: headers bold, hunk lines cyan, removed lines red,
// added lines green.
func ColorDiff(d string) string {
	lines := strings.SplitAfter(d, "\n")
	var b strings.Builder
	for _, l := range lines {
		text := strings.TrimSuffix(l, "\n")
		c := ""
		switch {
		case strings.HasPrefix(text, "--- "), strings.HasPrefix(text, "+++ "):
			c = bold
		case strings.HasPrefix(text, "@@"):
			c = cyan
		case strings.HasPrefix(text, "-"):
			c = red
		case strings.HasPrefix(text, "+"):
			c = green
		}
		if c == "" || text == "" {
			b.WriteString(l)
			continue
		}
		b.WriteString(c + text + reset + l[len(text):])
	}
	return b.String()
}

// Warning returns the prefix of a warning line on w, yellow on a terminal.
func Warning(w io.Writer) string { return prefix("tdm: warning:", yellow, Color(w)) }

// Error returns the prefix of an error line on w, red on a terminal.
func Error(w io.Writer) string { return prefix("tdm:", red, Color(w)) }

func prefix(s, c string, color bool) string {
	if !color {
		return s
	}
	return c + s + reset
}
