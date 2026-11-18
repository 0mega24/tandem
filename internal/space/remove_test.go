package space

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git/testhelper"
)

func newSpaceFor(t *testing.T, f fixture, repos ...string) {
	t.Helper()
	if _, err := f.m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: repos}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepos(t *testing.T) {
	f := newFixture(t, "api", "myorg/web")
	newSpaceFor(t, f, "api", "web")

	got, err := f.m.RemoveRepos(context.Background(), "feat", []string{"web"}, RemoveOptions{DeleteBranch: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].BranchErr != nil || got[0].Relinked {
		t.Fatalf("removed = %+v", got)
	}

	if _, err := os.Stat(filepath.Join(f.dir, "spaces", "feat", "myorg")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the empty org dir is still there: %v", err)
	}
	if out := testhelper.Git(t, f.repos["myorg/web"].Path, "branch", "--list", "feat"); out != "" {
		t.Errorf("-d kept the merged branch: %q", out)
	}
	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 1 || sp.Repos[0].Name != "api" {
		t.Errorf("state = %+v, %v; want only api", sp.Repos, err)
	}
}

func TestRemoveAsksFirst(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	wt := filepath.Join(f.dir, "spaces", "feat", "api")
	testhelper.Commit(t, wt, "a.txt", "a\n", "work")
	testhelper.WriteFile(t, wt, "wip.txt", "w\n")
	ctx := context.Background()

	// No Confirm, or a no: nothing is removed.
	if _, err := f.m.RemoveRepos(ctx, "feat", []string{"api"}, RemoveOptions{}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	var asked []Risk
	no := func(r []Risk) bool { asked = r; return false }
	if _, err := f.m.RemoveRepos(ctx, "feat", []string{"api"}, RemoveOptions{Confirm: no}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	if len(asked) != 1 || asked[0].Changed != 1 || asked[0].Unpushed != 1 {
		t.Errorf("risks = %+v, want api with 1 changed and 1 unpushed", asked)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("a no removed the worktree: %v", err)
	}

	// A yes removes it; -d keeps the branch, which is not merged.
	got, err := f.m.RemoveRepos(ctx, "feat", []string{"api"}, RemoveOptions{
		DeleteBranch: true,
		Confirm:      func([]Risk) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BranchErr == nil {
		t.Error("BranchErr = nil, want the unmerged branch kept")
	}
	if _, err := os.Stat(wt); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("worktree still there: %v", err)
	}
}

func TestRemoveAlwaysRepos(t *testing.T) {
	f := newFixture(t, "api", "infra", "tools")
	f.m.cfg.Always = config.Always{Repos: []string{"infra", "tools"}}
	ctx := context.Background()
	newSpaceFor(t, f, "api", "infra") // infra: a worktree; tools: a symlink

	got, err := f.m.RemoveRepos(ctx, "feat", []string{"infra", "tools"}, RemoveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Relinked || got[1].Relinked {
		t.Errorf("removed = %+v, want infra relinked and tools dropped", got)
	}

	root := filepath.Join(f.dir, "spaces", "feat")
	if dst, err := os.Readlink(filepath.Join(root, "infra")); err != nil || dst != f.repos["infra"].Path {
		t.Errorf("infra = %q, %v; want the symlink back", dst, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "tools")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("tools link still there: %v", err)
	}

	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 2 || sp.Repos[1].Name != "infra" || !sp.Repos[1].Symlink {
		t.Errorf("state = %+v, %v; want api and the infra symlink", sp.Repos, err)
	}
}

func TestRemoveErrors(t *testing.T) {
	f := newFixture(t, "api", "web")
	newSpaceFor(t, f, "api")
	ctx := context.Background()

	if _, err := f.m.RemoveRepos(ctx, "feat", []string{"web"}, RemoveOptions{}); err == nil || !strings.Contains(err.Error(), `space feat: no repo "web"`) {
		t.Errorf("err = %v, want web not in the space", err)
	}
	if _, err := f.m.RemoveRepos(ctx, "nope", []string{"api"}, RemoveOptions{}); err == nil {
		t.Error("err = nil for an unknown space")
	}
}
