package ui

import (
	"fmt"
	"strings"
)

// diffContext is the number of unchanged lines around each change, as in diff -u.
const diffContext = 3

// Diff returns a unified diff of the lines of a and b, or "" when they are equal. It builds the
// full longest-common-subsequence table, O(len(a)*len(b)): fine for workspace files, which have
// tens of lines, not for large files.
func Diff(name string, a, b []byte) string {
	if string(a) == string(b) {
		return ""
	}
	x, y := lines(a), lines(b)
	ops := editScript(x, y)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s (planned)\n", name, name)
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		// A hunk: the changes from i on, with context, until a run of unchanged lines that is
		// longer than two contexts.
		start := max(i-diffContext, 0)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*diffContext {
				end = min(end+diffContext, len(ops))
				break
			}
			end = run
		}
		writeHunk(&out, ops[start:end])
		i = end
	}
	return out.String()
}

type op struct {
	kind         byte   // ' ', '-' or '+'
	text         string // the line without its newline
	aLine, bLine int    // 1-based line numbers of the op in a and b
}

func lines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// editScript returns the ops that turn x into y, from the LCS table.
func editScript(x, y []string) []op {
	n, m := len(x), len(y)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			ops = append(ops, op{' ', x[i], i + 1, j + 1})
			i, j = i+1, j+1
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', x[i], i + 1, j + 1}) // deletes first, as diff -u
			i++
		default:
			ops = append(ops, op{'+', y[j], i + 1, j + 1})
			j++
		}
	}
	return ops
}

func writeHunk(out *strings.Builder, ops []op) {
	aLen, bLen := 0, 0
	for _, o := range ops {
		if o.kind != '+' {
			aLen++
		}
		if o.kind != '-' {
			bLen++
		}
	}
	aStart, bStart := ops[0].aLine, ops[0].bLine
	if aLen == 0 {
		aStart-- // diff -u names the line before an empty range
	}
	if bLen == 0 {
		bStart--
	}
	fmt.Fprintf(out, "@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
	for _, o := range ops {
		out.WriteByte(o.kind)
		out.WriteString(o.text)
		out.WriteByte('\n')
	}
}
