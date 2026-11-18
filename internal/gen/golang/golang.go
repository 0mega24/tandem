package golang

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes go.work.
type Generator struct{}

// fallbackGo is the go line when no member go.mod has one: the oldest version with go.work
// features that the use lines need.
const fallbackGo = "1.21"

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID:        "go",
		Languages: []string{"Go"},
		Manifests: []string{"go.mod"},
		DefaultOn: true,
		Release:   1,
	}
}

// Detect finds a go.mod at the worktree root. Detection is root only: a repo with more
// modules gets one use line, for its root module.
func (Generator) Detect(m gen.Member, _ gen.DetectOptions) (*gen.Detection, error) {
	b, err := os.ReadFile(filepath.Join(m.WorktreePath, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	goVersion, err := goDirective(b)
	if err != nil {
		return nil, fmt.Errorf("%s/go.mod: %w", m.Name, err)
	}

	_, err = os.Stat(filepath.Join(m.WorktreePath, "go.work"))
	hasWork := err == nil

	return &gen.Detection{
		Member:   m,
		Manifest: "go.mod",
		Facts:    map[string]any{"go": goVersion, "go.work": hasWork},
	}, nil
}

// goDirective returns the version of the go line, or "" when there is none. The line is
// "go 1.22" or "go 1.22.3", once, outside any block, with an optional // comment.
func goDirective(modFile []byte) (string, error) {
	s := bufio.NewScanner(bytes.NewReader(modFile))
	for s.Scan() {
		line, _, _ := strings.Cut(s.Text(), "//")
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			if !version.IsValid("go" + fields[1]) {
				return "", fmt.Errorf("invalid go version %q", fields[1])
			}
			return fields[1], nil
		}
	}
	return "", s.Err()
}

// Plan writes go.work: the highest go line of the members, and a use line for each member.
// go.work.sum is stale when go.work changes: the go command writes it again.
func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if len(found) == 0 {
		return gen.Plan{}, nil
	}

	var p gen.Plan
	goLine := ""
	var versions []string
	for _, d := range found {
		v, _ := d.Facts["go"].(string) // "" when the go.mod has no go line
		if v != "" && !slices.Contains(versions, v) {
			versions = append(versions, v)
		}
		if v != "" && (goLine == "" || version.Compare("go"+v, "go"+goLine) > 0) {
			goLine = v
		}
		if hasWork, _ := d.Facts["go.work"].(bool); hasWork {
			p.Warnings = append(p.Warnings, d.Member.Name+" has its own go.work; in the space, the go.work in the space root is used")
		}
	}
	if goLine == "" {
		goLine = fallbackGo
	}
	if len(versions) > 1 {
		slices.SortFunc(versions, func(a, b string) int { return version.Compare("go"+a, "go"+b) })
		p.Warnings = append(p.Warnings, fmt.Sprintf("the members use go %s; go.work uses go %s, the highest",
			strings.Join(versions, ", "), goLine))
	}

	var b strings.Builder
	b.WriteString("// " + gen.Header(s.Name) + "\n\n")
	b.WriteString("go " + goLine + "\n\n")
	b.WriteString("use (\n")
	for _, d := range found {
		b.WriteString("\t./" + d.Member.Name + "\n")
	}
	b.WriteString(")\n")

	p.Files = []gen.File{{Path: "go.work", Content: []byte(b.String())}}
	p.Stale = []string{"go.work.sum"}
	return p, nil
}
