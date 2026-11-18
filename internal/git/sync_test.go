package git

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/0mega24/tandem/internal/git/testhelper"
)

// pushUpstream adds a commit to origin's main from a second clone.
func pushUpstream(t *testing.T, dir string, r testhelper.Repo, file string) {
	t.Helper()
	other := filepath.Join(dir, "other-"+file)
	testhelper.Git(t, dir, "clone", "-q", r.Origin, other)
	testhelper.Commit(t, other, file, file+"\n", "upstream "+file)
	testhelper.Git(t, other, "push", "-q", "origin", "main")
}

func TestFastForwardCheckedOut(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}

	pushUpstream(t, dir, r, "up.txt")
	if err := s.Fetch(ctx, r.Path); err != nil {
		t.Fatal(err)
	}
	if moved, err := s.FastForward(ctx, r.Path, "main"); err != nil || !moved {
		t.Fatalf("FastForward() = %v, %v; want moved", moved, err)
	}
	if got, want := testhelper.Git(t, r.Path, "rev-parse", "HEAD"), testhelper.Git(t, r.Path, "rev-parse", "origin/main"); got != want {
		t.Errorf("HEAD = %s, want origin/main %s", got, want)
	}
	if st, _ := s.Status(ctx, r.Path); !st.Clean() {
		t.Errorf("working tree not clean after the fast-forward: %v", st.Changed)
	}
	if moved, err := s.FastForward(ctx, r.Path, "main"); err != nil || moved {
		t.Errorf("FastForward when up to date = %v, %v; want not moved, no error", moved, err)
	}
}

func TestFastForwardNotCheckedOut(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}

	testhelper.Git(t, r.Path, "switch", "-q", "-c", "work")
	pushUpstream(t, dir, r, "up.txt")
	if err := s.Fetch(ctx, r.Path); err != nil {
		t.Fatal(err)
	}
	if moved, err := s.FastForward(ctx, r.Path, "main"); err != nil || !moved {
		t.Fatalf("FastForward() = %v, %v; want moved", moved, err)
	}
	if got, want := testhelper.Git(t, r.Path, "rev-parse", "main"), testhelper.Git(t, r.Path, "rev-parse", "origin/main"); got != want {
		t.Errorf("main = %s, want origin/main %s", got, want)
	}
	if head := testhelper.Git(t, r.Path, "symbolic-ref", "--short", "HEAD"); head != "work" {
		t.Errorf("HEAD = %s, want work (unchanged)", head)
	}
}

func TestFastForwardDiverged(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}

	testhelper.Commit(t, r.Path, "local.txt", "l\n", "local only")
	pushUpstream(t, dir, r, "up.txt")
	if err := s.Fetch(ctx, r.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FastForward(ctx, r.Path, "main"); !errors.Is(err, ErrNotFastForward) {
		t.Errorf("FastForward(diverged) = %v, want ErrNotFastForward", err)
	}
}
