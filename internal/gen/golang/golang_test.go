package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) {
	gentest.Run(t, Generator{})
}

func TestGoDirective(t *testing.T) {
	tests := []struct {
		in, want string
		err      bool
	}{
		{"module x\n\ngo 1.22.3 // pinned\n", "1.22.3", false},
		{"module x\n", "", false},
		{"module x\ngo banana\n", "", true},
		{"module x\n// go 1.30\ngo 1.21\n", "1.21", false},
	}
	for _, tt := range tests {
		got, err := goDirective([]byte(tt.in))
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("goDirective(%q) = %q, %v; want %q (error %v)", tt.in, got, err, tt.want, tt.err)
		}
	}
}

// A monorepo with Go modules below its root (client/go.mod next to server/Cargo.toml) is a Go
// member when detect_depth reaches the modules: go.work gets one use line per module.
func TestDetectDepth(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "mono")
	for file, content := range map[string]string{
		"client/go.mod":     "module example.org/client\n\ngo 1.22\n",
		"tools/gen/go.mod":  "module example.org/gen\n\ngo 1.23\n",
		"server/Cargo.toml": "[package]\nname = \"server\"\n",
		"vendor/x/go.mod":   "module ignored\n",
		"deep/a/b/c/go.mod": "module too-deep\n",
	} {
		p := filepath.Join(repo, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := gen.Member{Name: "mono", WorktreePath: repo}

	if d, err := (Generator{}).Detect(m, gen.DetectOptions{}); err != nil || d != nil {
		t.Fatalf("Detect at depth 0 = %+v, %v; want nil: the root has no go.mod", d, err)
	}
	d, err := (Generator{}).Detect(m, gen.DetectOptions{Depth: 2})
	if err != nil || d == nil {
		t.Fatalf("Detect at depth 2 = %+v, %v", d, err)
	}
	p, err := (Generator{}).Plan(gen.Space{Name: "feat", Root: root}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	got := string(p.Files[0].Content)
	if !strings.Contains(got, "go 1.23\n") || !strings.Contains(got, "\t./mono/client\n\t./mono/tools/gen\n)") {
		t.Fatalf("go.work =\n%s\nwant go 1.23 and the two modules", got)
	}
}
