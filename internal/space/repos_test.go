package space

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/state"
)

// fixture is a repos root with real clones, and a Manager that uses it.
type fixture struct {
	dir   string
	repos map[string]testhelper.Repo
	m     *Manager
}

func newFixture(t *testing.T, names ...string) fixture {
	t.Helper()
	testhelper.Isolate(t)

	dir := t.TempDir()
	root := filepath.Join(dir, "repos")
	f := fixture{dir: dir, repos: map[string]testhelper.Repo{}}

	for _, n := range names {
		f.repos[n] = testhelper.NewRepo(t, root, n)
	}

	cfg := config.Config{
		Discovery: config.Discovery{RootDir: root, MaxDepth: 2},
		Spaces:    config.Spaces{RootDir: filepath.Join(dir, "spaces")},
	}
	f.m = New(cfg, git.System{}, state.NewStore(filepath.Join(dir, "state")))

	return f
}

// pushUpstream adds a commit to origin's main from another clone.
func (f fixture) pushUpstream(t *testing.T, name string) {
	t.Helper()
	other := filepath.Join(f.dir, "other-"+name)
	testhelper.Git(t, f.dir, "clone", "-q", f.repos[name].Origin, other)
	testhelper.Commit(t, other, "up.txt", "up\n", "upstream")
	testhelper.Git(t, other, "push", "-q", "origin", "main")
}

func TestRepoStatus(t *testing.T) {
	f := newFixture(t, "api", "web")
	testhelper.WriteFile(t, f.repos["web"].Path, "dirty.txt", "x\n")

	got, err := f.m.RepoStatus(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].Repo.Name != "api" || got[1].Repo.Name != "web" {
		t.Fatalf("RepoStatus() = %+v, want api and web in order", got)
	}
	if !got[0].Status.Clean() || got[0].Status.Branch != "main" || got[0].Origin != f.repos["api"].Origin {
		t.Errorf("api = %+v, want clean on main with its origin", got[0])
	}
	if got[1].Status.Clean() {
		t.Errorf("web = %+v, want one changed file", got[1])
	}
}

func TestRepoStatusBadName(t *testing.T) {
	f := newFixture(t, "api")
	if _, err := f.m.RepoStatus(context.Background(), []string{"nope"}); err == nil {
		t.Error("RepoStatus(nope) = nil error, want an error")
	}
}

func TestSyncRepos(t *testing.T) {
	f := newFixture(t, "api", "web", "dirty")
	f.pushUpstream(t, "api")
	f.pushUpstream(t, "dirty")
	testhelper.WriteFile(t, f.repos["dirty"].Path, "wip.txt", "x\n")

	got, err := f.m.SyncRepos(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]SyncOutcome{"api": SyncUpdated, "dirty": SyncSkippedDirty, "web": SyncUpToDate}
	for _, r := range got {
		if r.Outcome != want[r.Repo.Name] || r.Err != nil {
			t.Errorf("%s: %v (%v), want %v", r.Repo.Name, r.Outcome, r.Err, want[r.Repo.Name])
		}
	}

	if testhelper.Git(t, f.repos["api"].Path, "rev-parse", "HEAD") != testhelper.Git(t, f.repos["api"].Path, "rev-parse", "origin/main") {
		t.Error("api main did not move to origin/main")
	}
}

func TestSyncDiverged(t *testing.T) {
	f := newFixture(t, "api")
	testhelper.Commit(t, f.repos["api"].Path, "local.txt", "l\n", "local")
	f.pushUpstream(t, "api")

	got, err := f.m.SyncRepos(context.Background(), []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Outcome != SyncFailed || !errors.Is(got[0].Err, git.ErrNotFastForward) {
		t.Errorf("api = %v (%v), want failed with ErrNotFastForward", got[0].Outcome, got[0].Err)
	}
}
