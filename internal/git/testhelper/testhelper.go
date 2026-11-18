// Package testhelper makes real git repos in t.TempDir() for tests.
package testhelper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Isolate makes git in this test ignore the user's and the system's git config, and sets a fixed
// author. It uses t.Setenv, so the test must not call t.Parallel.
func Isolate(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, []byte("[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

// Git runs git in dir and returns the trimmed stdout. It fails the test on an error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Repo is a main clone with a bare origin.
type Repo struct {
	Path   string // the main clone
	Origin string // the bare repo that is the clone's origin
}

// NewRepo makes a bare origin with one commit on main, and a clone of it at dir/name.
// Call Isolate first.
func NewRepo(t *testing.T, dir, name string) Repo {
	t.Helper()
	origin := filepath.Join(dir, "origins", name+".git")
	seed := filepath.Join(dir, "seeds", name)
	Git(t, dir, "init", "-q", "--bare", origin)
	Git(t, dir, "init", "-q", seed)
	WriteFile(t, seed, "README.md", "# "+name+"\n")
	Git(t, seed, "add", ".")
	Git(t, seed, "commit", "-q", "-m", "initial")
	Git(t, seed, "push", "-q", origin, "main")
	Git(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")

	clone := filepath.Join(dir, name)
	Git(t, dir, "clone", "-q", origin, clone)
	return Repo{Path: clone, Origin: origin}
}

// WriteFile writes content to dir/rel and makes the parent directories.
func WriteFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Commit writes rel and commits it in dir.
func Commit(t *testing.T, dir, rel, content, msg string) {
	t.Helper()
	WriteFile(t, dir, rel, content)
	Git(t, dir, "add", rel)
	Git(t, dir, "commit", "-q", "-m", msg)
}
