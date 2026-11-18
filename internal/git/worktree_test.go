package git

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0mega24/tandem/internal/git/testhelper"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /r\x00HEAD abc\x00branch refs/heads/main\x00\x00" +
		"worktree /w/a\x00HEAD def\x00detached\x00\x00" +
		"worktree /w/b\x00HEAD 123\x00branch refs/heads/me/feat\x00prunable gitdir file points to non-existent location\x00\x00"
	want := []Worktree{
		{Path: "/r", Head: "abc", Branch: "main"},
		{Path: "/w/a", Head: "def", Detached: true},
		{Path: "/w/b", Head: "123", Branch: "me/feat", Prunable: true},
	}
	if got := parseWorktrees(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktrees() = %+v, want %+v", got, want)
	}
}

// realPath resolves symlinks, because git prints resolved paths (macOS /var is /private/var).
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWorktreeLifecycle(t *testing.T) {
	testhelper.Isolate(t)
	dir := realPath(t, t.TempDir())
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}
	wt := filepath.Join(dir, "space", "api")

	if err := s.AddWorktree(ctx, r.Path, wt, "me/feat", "main"); err != nil {
		t.Fatal(err)
	}
	list, err := s.Worktrees(ctx, r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].Path != wt || list[1].Branch != "me/feat" {
		t.Fatalf("Worktrees() = %+v, want the main clone and %s on me/feat", list, wt)
	}

	// The branch is checked out now, so a second worktree on it fails.
	if err := s.AddWorktree(ctx, r.Path, filepath.Join(dir, "other"), "me/feat", ""); err == nil {
		t.Error("AddWorktree on a checked-out branch = nil, want an error")
	}

	// A change blocks a remove without force.
	testhelper.WriteFile(t, wt, "dirty.txt", "x\n")
	if err := s.RemoveWorktree(ctx, r.Path, wt, false); err == nil {
		t.Error("RemoveWorktree(dirty, no force) = nil, want an error")
	}
	if err := s.RemoveWorktree(ctx, r.Path, wt, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove: %v", err)
	}

	// The branch stays, and a new worktree can use it.
	if err := s.AddWorktree(ctx, r.Path, wt, "me/feat", ""); err != nil {
		t.Errorf("AddWorktree on the existing branch: %v", err)
	}
}

func TestPruneAndRepair(t *testing.T) {
	testhelper.Isolate(t)
	dir := realPath(t, t.TempDir())
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}

	gone := filepath.Join(dir, "gone")
	if err := s.AddWorktree(ctx, r.Path, gone, "b1", "main"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Worktrees(ctx, r.Path)
	if len(list) != 2 || !list[1].Prunable {
		t.Fatalf("after rm -rf: Worktrees() = %+v, want the entry marked prunable", list)
	}
	if err := s.PruneWorktrees(ctx, r.Path); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Worktrees(ctx, r.Path); len(list) != 1 {
		t.Errorf("after prune: Worktrees() = %+v, want only the main clone", list)
	}

	// A moved worktree: git loses it until repair.
	moved, dest := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := s.AddWorktree(ctx, r.Path, moved, "b2", "main"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, dest); err != nil {
		t.Fatal(err)
	}
	if err := s.RepairWorktree(ctx, r.Path, dest); err != nil {
		t.Fatal(err)
	}
	list, _ = s.Worktrees(ctx, r.Path)
	if len(list) != 2 || list[1].Path != dest || list[1].Prunable {
		t.Errorf("after repair: Worktrees() = %+v, want %s known and not prunable", list, dest)
	}
}
