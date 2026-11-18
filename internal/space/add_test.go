package space

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/saga"
)

func TestAddRepos(t *testing.T) {
	f := newFixture(t, "api", "web")
	ctx := context.Background()
	if _, err := f.m.NewSpace(ctx, NewOptions{Name: "feat", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}

	res, err := f.m.AddRepos(ctx, "feat", []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 || res.Repos[0].From != "main" || res.Repos[0].WasLink {
		t.Fatalf("added = %+v, want web with a new branch from main", res.Repos)
	}
	if head := testhelper.Git(t, res.Repos[0].Entry.WorktreePath, "branch", "--show-current"); head != "feat" {
		t.Errorf("web worktree is on %q, want feat", head)
	}

	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 2 || sp.Repos[1].Name != "web" {
		t.Errorf("state = %+v, %v; want api, web", sp.Repos, err)
	}
}

func TestAddReplacesLink(t *testing.T) {
	f := newFixture(t, "api", "myorg/infra")
	f.m.cfg.Always = config.Always{Repos: []string{"infra"}}
	ctx := context.Background()
	if _, err := f.m.NewSpace(ctx, NewOptions{Name: "feat", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}

	res, err := f.m.AddRepos(ctx, "feat", []string{"infra"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Repos[0].WasLink {
		t.Error("WasLink = false, want true")
	}

	wt := res.Repos[0].Entry.WorktreePath
	if fi, err := os.Lstat(wt); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s = %v, %v; want a worktree directory", wt, fi, err)
	}
	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 2 || sp.Repos[1].Symlink {
		t.Errorf("state = %+v, %v; want infra as a worktree", sp.Repos, err)
	}
}

func TestAddErrors(t *testing.T) {
	f := newFixture(t, "api", "web")
	ctx := context.Background()
	if _, err := f.m.NewSpace(ctx, NewOptions{Name: "feat", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.dir, "spaces", "feat", "web"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, space, repo, want string
	}{
		{"unknown space", "nope", "web", "space not found"},
		{"in the space", "feat", "api", "api is in space feat"},
		{"path exists", "feat", "web", "web: " + filepath.Join(f.dir, "spaces", "feat", "web") + " exists"},
		{"unknown repo", "feat", "nope", `no repo "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.m.AddRepos(ctx, tt.space, []string{tt.repo})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAddRollbackRestoresLink(t *testing.T) {
	f := newFixture(t, "api", "myorg/infra")
	f.m.cfg.Always = config.Always{Repos: []string{"infra"}}
	ctx := context.Background()
	if _, err := f.m.NewSpace(ctx, NewOptions{Name: "feat", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}
	before, err := f.m.Space("feat")
	if err != nil {
		t.Fatal(err)
	}

	m := New(f.m.cfg, failAdd{repo: f.repos["myorg/infra"].Path}, f.m.store)
	if _, err := m.AddRepos(ctx, "feat", []string{"infra"}); !saga.IsRolledBack(err) {
		t.Fatalf("err = %v, want a full rollback", err)
	}

	link := filepath.Join(f.dir, "spaces", "feat", "myorg", "infra")
	if dst, err := os.Readlink(link); err != nil || dst != f.repos["myorg/infra"].Path {
		t.Errorf("link = %q, %v; want it back", dst, err)
	}
	after, err := f.m.Space("feat")
	if err != nil || len(after.Repos) != len(before.Repos) || !after.Repos[1].Symlink {
		t.Errorf("state = %+v, %v; want it as before", after.Repos, err)
	}
}
