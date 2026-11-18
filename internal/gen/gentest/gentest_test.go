package gentest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
)

// lines is a generator for the harness test: a member with lines.txt is found, and the plan
// lists the members in all.txt.
type lines struct{}

func (lines) Meta() gen.Meta { return gen.Meta{ID: "lines"} }

func (lines) Detect(m gen.Member, _ gen.DetectOptions) (*gen.Detection, error) {
	if _, err := os.Stat(filepath.Join(m.WorktreePath, "lines.txt")); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return &gen.Detection{Member: m, Manifest: "lines.txt"}, nil
}

func (lines) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if len(found) == 0 {
		return gen.Plan{}, nil
	}

	var b strings.Builder
	b.WriteString("# " + gen.Header(s.Name) + "\n")
	for _, d := range found {
		b.WriteString(strings.TrimSpace(d.Member.Name+" "+d.Member.OriginURL) + "\n")
	}
	p := gen.Plan{Files: []gen.File{{Path: "all.txt", Content: []byte(b.String())}}, Stale: []string{"all.sum"}}
	if s.Config["warn"] == true {
		p.Warnings = append(p.Warnings, "asked to warn")
	}
	return p, nil
}

func TestRun(t *testing.T) {
	Run(t, lines{})
}
