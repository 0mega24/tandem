package internal_test

import (
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
)

// topLevel is the allowlist of the "Locked project structure" in docs/DESIGN.md. Change that
// section first, then this list, in the same PR.
var topLevel = []string{
	".agents", ".claude-plugin", ".codex-plugin", ".editorconfig", ".envrc", ".github", ".gitignore",
	".golangci.yml", ".goreleaser.yaml", "AGENTS.md", "CHANGELOG.md", "CLAUDE.md", "LICENSE", "Makefile", "README.md",
	"cmd", "docs", "examples", "flake.lock", "flake.nix", "go.mod", "go.sum", "go.tool.mod",
	"go.tool.sum", "internal", "nix", "schema", "skills", "tools",
}

// genSupport are the directories under internal/gen/ that are not generators.
var genSupport = []string{"all", "external", "gentest", "urlnorm"}

// trackedFiles lists the files that git tracks or would track (git add), relative to the repo root.
// The test skips outside a git checkout, for example in the Nix build sandbox.
func trackedFiles(t *testing.T) []string {
	t.Helper()
	touchDirs(t)
	out, err := exec.Command("git", "-C", "..", "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		t.Skipf("not a git checkout (%v); the layout check runs in make check and CI", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func TestTopLevelLayout(t *testing.T) {
	for _, f := range trackedFiles(t) {
		top, _, _ := strings.Cut(f, "/")
		if !slices.Contains(topLevel, top) {
			t.Errorf("%s: top-level entry %q is not in the locked structure (docs/DESIGN.md)", f, top)
		}
	}
}

func TestGeneratorLayout(t *testing.T) {
	files := map[string]bool{}
	ids := map[string]bool{}
	for _, f := range trackedFiles(t) {
		files[f] = true
		rest, ok := strings.CutPrefix(f, "internal/gen/")
		if !ok {
			continue
		}
		id, _, isDir := strings.Cut(rest, "/")
		if isDir && !slices.Contains(genSupport, id) {
			ids[id] = true
		}
	}
	for id := range ids {
		dir := path.Join("internal/gen", id)
		for _, want := range []string{id + ".go", id + "_test.go", "doc.go"} {
			if !files[path.Join(dir, want)] {
				t.Errorf("%s: missing %s (make newgen writes it)", dir, want)
			}
		}
		if !hasPrefix(files, dir+"/testdata/") {
			t.Errorf("%s: missing testdata/", dir)
		}
	}
}

func hasPrefix(files map[string]bool, prefix string) bool {
	for f := range files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// touchDirs reads the checked directories. go test caches a result by the files and directories
// that the test itself opens, not by what the git subprocess reads, so a new entry must change a
// directory that the test opened.
func touchDirs(t *testing.T) {
	t.Helper()
	if _, err := os.ReadDir(".."); err != nil {
		t.Fatal(err)
	}
	gens, err := os.ReadDir("gen")
	if err != nil {
		return // no generators yet
	}
	for _, g := range gens {
		if g.IsDir() {
			_, _ = os.ReadDir(path.Join("gen", g.Name())) // cache key only; a read error changes nothing
		}
	}
}
