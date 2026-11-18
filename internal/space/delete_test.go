package space

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/state"
)

func TestDeleteSpace(t *testing.T) {
	f := newFixture(t, "api", "myorg/web", "infra")
	testhelper.WriteFile(t, f.dir, "notes/AGENTS.md", "x\n")
	f.m.cfg.Always = config.Always{Repos: []string{"infra"}, Files: []string{filepath.Join(f.dir, "notes/AGENTS.md")}}
	newSpaceFor(t, f, "api", "web")
	ctx := context.Background()

	got, err := f.m.DeleteSpace(ctx, "feat", DeleteOptions{DeleteBranch: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != 3 || len(got.Files) != 1 || got.KeptDir {
		t.Errorf("deleted = %+v, want 3 repos, 1 file, no dir kept", got)
	}

	if _, err := os.Stat(filepath.Join(f.dir, "spaces", "feat")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("space dir still there: %v", err)
	}
	if _, err := f.m.Space("feat"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("state still there: %v", err)
	}
	for _, r := range []string{"api", "myorg/web"} {
		if out := testhelper.Git(t, f.repos[r].Path, "branch", "--list", "feat"); out != "" {
			t.Errorf("%s kept the merged branch", r)
		}
	}
	if _, err := os.Stat(f.repos["infra"].Path); err != nil {
		t.Errorf("the main clone of the symlinked repo is gone: %v", err)
	}
}

func TestDeleteSpaceKeepsOtherFiles(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	testhelper.WriteFile(t, filepath.Join(f.dir, "spaces", "feat"), "notes.txt", "mine\n")

	got, err := f.m.DeleteSpace(context.Background(), "feat", DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.KeptDir {
		t.Error("KeptDir = false, want the dir kept for notes.txt")
	}
	if out := testhelper.Git(t, f.repos["api"].Path, "branch", "--list", "feat"); out == "" {
		t.Error("the branch is gone without -d")
	}
}

func TestDeleteSpaceForceBranch(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	testhelper.Commit(t, filepath.Join(f.dir, "spaces", "feat", "api"), "a.txt", "a\n", "work")
	ctx := context.Background()

	if _, err := f.m.DeleteSpace(ctx, "feat", DeleteOptions{ForceBranch: true}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined for the unpushed commit", err)
	}
	if _, err := f.m.Space("feat"); err != nil {
		t.Fatalf("a no deleted the space: %v", err)
	}

	got, err := f.m.DeleteSpace(ctx, "feat", DeleteOptions{ForceBranch: true, Confirm: func([]Risk) bool { return true }})
	if err != nil || got.Repos[0].BranchErr != nil {
		t.Fatalf("DeleteSpace = %+v, %v", got, err)
	}
	if out := testhelper.Git(t, f.repos["api"].Path, "branch", "--list", "feat"); out != "" {
		t.Error("-D kept the unmerged branch")
	}
}
