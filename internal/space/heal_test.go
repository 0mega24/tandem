package space

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/git/testhelper"
)

func TestHealDropsDeletedWorktrees(t *testing.T) {
	f := newFixture(t, "api", "web", "lib")
	newSpaceFor(t, f, "api", "web", "lib")
	var out bytes.Buffer
	f.m.SetOutput(&out)
	ctx := context.Background()

	// web: the user deleted the directory. lib: also pruned in git already.
	root := filepath.Join(f.dir, "spaces", "feat")
	for _, r := range []string{"web", "lib"} {
		if err := os.RemoveAll(filepath.Join(root, r)); err != nil {
			t.Fatal(err)
		}
	}
	testhelper.Git(t, f.repos["lib"].Path, "worktree", "prune")

	got, err := f.m.Status(ctx, []string{"feat"}, f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Repos) != 1 || got[0].Repos[0].Entry.Name != "api" {
		t.Fatalf("status repos = %+v, want only api", got[0].Repos)
	}
	if w := out.String(); strings.Count(w, "is gone") != 2 || !strings.Contains(w, "still on branch feat") {
		t.Errorf("warnings = %q, want one for web and one for lib", w)
	}

	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 1 {
		t.Errorf("state = %+v, %v; want the healed state saved", sp.Repos, err)
	}
	if trees, _ := (git.System{}).Worktrees(ctx, f.repos["web"].Path); len(trees) != 1 {
		t.Errorf("web has %d worktrees, want the stale one pruned", len(trees))
	}
	if out := testhelper.Git(t, f.repos["web"].Path, "branch", "--list", "feat"); out == "" {
		t.Error("heal deleted the branch")
	}
}

// unknownTrees is git that does not list the space worktrees, as when a main clone moved and
// came back. repairErr is what RepairWorktree returns.
type unknownTrees struct {
	git.System
	repaired  *[]string
	repairErr error
}

func (u unknownTrees) Worktrees(context.Context, string) ([]git.Worktree, error) { return nil, nil }

func (u unknownTrees) RepairWorktree(_ context.Context, _, path string) error {
	*u.repaired = append(*u.repaired, path)
	return u.repairErr
}

func TestHealRepairs(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	var repaired []string
	m := New(f.m.cfg, unknownTrees{repaired: &repaired}, f.m.store)

	if _, err := m.Status(context.Background(), []string{"feat"}, f.dir); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(f.dir, "spaces", "feat", "api"); len(repaired) != 1 || repaired[0] != want {
		t.Errorf("repaired %v, want %s", repaired, want)
	}

	m = New(f.m.cfg, unknownTrees{repaired: &repaired, repairErr: git.ErrRepairUnsupported}, f.m.store)
	_, err := m.Status(context.Background(), []string{"feat"}, f.dir)
	if !errors.Is(err, git.ErrRepairUnsupported) || !strings.Contains(err.Error(), "tdm add feat api") {
		t.Errorf("err = %v, want ErrRepairUnsupported with the way out", err)
	}
}
