package gen

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFind(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"go.mod", "a/go.mod", "a/b/go.mod", "node_modules/x/go.mod", "gen/go.mod", "tools/x.csproj",
	} {
		writeFile(t, root, f, "")
	}
	writeFile(t, root, ".gitignore", "# out\n/gen/\n!keep\n")
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	for depth, want := range map[int][]string{
		0: {"go.mod"},
		1: {"a/go.mod", "go.mod"},
		2: {"a/b/go.mod", "a/go.mod", "go.mod"},
	} {
		got, err := Find(root, depth, "go.mod")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Find(depth %d) = %v, want %v", depth, got, want)
		}
	}

	got, err := Find(root, 4, "*.csproj", "*.fsproj")
	if err != nil || !slices.Equal(got, []string{"tools/x.csproj"}) {
		t.Errorf("Find(*.csproj) = %v, %v", got, err)
	}
}

func TestExpand(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"crates/a", "crates/b", "crates/b/inner", "packages/ui/x", "target/c", "tools"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		patterns, want []string
	}{
		{[]string{"crates/*"}, []string{"crates/a", "crates/b"}},
		{[]string{"./crates/*/", "!crates/b"}, []string{"crates/a"}},
		{[]string{"packages/**"}, []string{"packages", "packages/ui", "packages/ui/x"}},
		{[]string{"tools", "missing"}, []string{"tools"}},
		{[]string{"*/c"}, nil},
	}
	for _, tt := range tests {
		got, err := Expand(root, tt.patterns)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Expand(%v) = %v, want %v", tt.patterns, got, tt.want)
		}
	}
}
