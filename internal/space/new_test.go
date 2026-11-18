package space

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

func TestNewSpace(t *testing.T) {
	f := newFixture(t, "api", "myorg/web")
	web := f.repos["myorg/web"].Path
	testhelper.Git(t, web, "branch", "feat")

	sp, repos, err := f.m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api", "web"}})
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(f.dir, "spaces", "feat")
	if sp.Path != root || sp.Branch != "feat" || len(repos) != 2 {
		t.Fatalf("space = %+v, %d repos", sp, len(repos))
	}

	wantFrom := map[string]string{"api": "main", "myorg/web": ""}
	for _, r := range repos {
		if r.From != wantFrom[r.Entry.Name] {
			t.Errorf("%s From = %q, want %q", r.Entry.Name, r.From, wantFrom[r.Entry.Name])
		}
		if head := testhelper.Git(t, r.Entry.WorktreePath, "branch", "--show-current"); head != "feat" {
			t.Errorf("%s worktree is on %q, want feat", r.Entry.Name, head)
		}
	}
	if repos[1].Entry.WorktreePath != filepath.Join(root, "myorg", "web") {
		t.Errorf("nested worktree path = %s", repos[1].Entry.WorktreePath)
	}

	saved, err := f.m.Space("feat")
	if err != nil || len(saved.Repos) != 2 || saved.Path != root {
		t.Errorf("state = %+v, %v", saved, err)
	}
}

func TestNewSpaceBranchPrefixAndOptions(t *testing.T) {
	f := newFixture(t, "api")
	f.m.cfg.Git.BranchPrefix = "me/"

	sp, _, err := f.m.NewSpace(context.Background(), NewOptions{Name: "x", Repos: []string{"api"}})
	if err != nil || sp.Branch != "me/x" {
		t.Fatalf("branch = %q, %v; want me/x", sp.Branch, err)
	}

	dir := filepath.Join(f.dir, "elsewhere")
	sp, _, err = f.m.NewSpace(context.Background(), NewOptions{Name: "y", Repos: []string{"api"}, Branch: "other", Path: dir})
	if err != nil || sp.Branch != "other" || sp.Path != dir {
		t.Fatalf("space = %+v, %v; want branch other in %s", sp, err, dir)
	}
}

func TestNewSpacePreflight(t *testing.T) {
	f := newFixture(t, "api", "web")
	ctx := context.Background()
	if _, _, err := f.m.NewSpace(ctx, NewOptions{Name: "taken", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}

	full := filepath.Join(f.dir, "full")
	testhelper.WriteFile(t, full, "keep.txt", "x\n")

	// web is on branch "busy" in its main clone, so a new worktree cannot have it.
	testhelper.Git(t, f.repos["web"].Path, "switch", "-q", "-c", "busy")

	tests := []struct {
		name string
		opts NewOptions
		want string
	}{
		{"bad name", NewOptions{Name: "-x", Repos: []string{"api"}}, "invalid space name"},
		{"no repos", NewOptions{Name: "a"}, "at least one repo"},
		{"exists", NewOptions{Name: "taken", Repos: []string{"web"}}, `space "taken" exists`},
		{"unknown repo", NewOptions{Name: "b", Repos: []string{"nope"}}, `no repo "nope"`},
		{"dir not empty", NewOptions{Name: "c", Repos: []string{"web"}, Path: full}, "is not empty"},
		{"branch checked out", NewOptions{Name: "d", Repos: []string{"api", "web"}, Branch: "busy"}, "web: branch busy is checked out in"},
		{"bad branch", NewOptions{Name: "e", Repos: []string{"api"}, Branch: "a b"}, `"a b" is not a valid branch name`},
		{"inside repo", NewOptions{Name: "g", Repos: []string{"api"}, Path: filepath.Join(f.repos["web"].Path, "sub")}, "is inside repo web"},
		{"inside space", NewOptions{Name: "h", Repos: []string{"web"}, Path: filepath.Join(f.dir, "spaces", "taken", "x")}, "is inside space taken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := f.m.NewSpace(ctx, tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if saga.IsRolledBack(err) {
				t.Error("pre-flight error went through the saga")
			}
		})
	}

	// Pre-flight made no change: api got no branch "busy", and no space directory exists.
	if ok, _ := (git.System{}).BranchExists(ctx, f.repos["api"].Path, "busy"); ok {
		t.Error("pre-flight made a branch in api")
	}
	for _, n := range []string{"a", "b", "d", "e"} {
		if _, err := os.Stat(filepath.Join(f.dir, "spaces", n)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("space dir %s exists after a failed pre-flight", n)
		}
	}
}

// failAdd is git that fails to add a worktree in one repo.
type failAdd struct {
	git.System
	repo string
}

func (f failAdd) AddWorktree(ctx context.Context, repo, path, branch, from string) error {
	if repo == f.repo {
		return errors.New("disk full")
	}
	return f.System.AddWorktree(ctx, repo, path, branch, from)
}

func TestNewSpaceRollback(t *testing.T) {
	f := newFixture(t, "myorg/api", "web")
	m := New(f.m.cfg, failAdd{repo: f.repos["web"].Path}, f.m.store)

	// The parents of the space directory are missing too; the rollback removes them.
	dir := filepath.Join(f.dir, "a", "b", "feat")
	_, _, err := m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api", "web"}, Path: dir})
	if !saga.IsRolledBack(err) {
		t.Fatalf("err = %v, want a full rollback", err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rollback kept the parents of the space dir: %v", err)
	}

	api := f.repos["myorg/api"].Path
	if out := testhelper.Git(t, api, "branch", "--list", "feat"); out != "" {
		t.Errorf("rollback kept the new branch in api: %q", out)
	}
	if _, err := m.Space("feat"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("rollback kept the state: %v", err)
	}
	if trees, _ := (git.System{}).Worktrees(context.Background(), api); len(trees) != 1 {
		t.Errorf("api has %d worktrees after rollback, want 1", len(trees))
	}
}

func TestNewSpaceKeepsExistingBranchOnRollback(t *testing.T) {
	f := newFixture(t, "api", "web")
	api := f.repos["api"].Path
	testhelper.Git(t, api, "branch", "feat")
	m := New(f.m.cfg, failAdd{repo: f.repos["web"].Path}, f.m.store)

	if _, _, err := m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api", "web"}}); err == nil {
		t.Fatal("NewSpace() = nil, want an error")
	}
	if out := testhelper.Git(t, api, "branch", "--list", "feat"); out == "" {
		t.Error("rollback deleted a branch that existed before")
	}
}

// stopAfterAdd is git that adds the worktree and then fails, as when Ctrl-C stops git late.
type stopAfterAdd struct{ git.System }

func (s stopAfterAdd) AddWorktree(ctx context.Context, repo, path, branch, from string) error {
	if err := s.System.AddWorktree(ctx, repo, path, branch, from); err != nil {
		return err
	}
	return context.Canceled
}

func TestNewSpaceCleansUpAFailedStep(t *testing.T) {
	f := newFixture(t, "myorg/api")
	m := New(f.m.cfg, stopAfterAdd{}, f.m.store)

	_, _, err := m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	api := f.repos["myorg/api"].Path
	if out := testhelper.Git(t, api, "branch", "--list", "feat"); out != "" {
		t.Errorf("the failed step kept the new branch: %q", out)
	}
	if trees, _ := (git.System{}).Worktrees(context.Background(), api); len(trees) != 1 {
		t.Errorf("api has %d worktrees, want 1", len(trees))
	}
	if _, err := os.Stat(filepath.Join(f.dir, "spaces", "feat")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the space dir is still there: %v", err)
	}
}
