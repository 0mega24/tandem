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
	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

func TestNewSpaceAlways(t *testing.T) {
	f := newFixture(t, "api", "web", "myorg/infra")
	api := f.repos["api"].Path
	testhelper.WriteFile(t, api, "config/dev.json", "{}\n")
	if err := os.WriteFile(filepath.Join(api, ".env"), []byte("KEY=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testhelper.WriteFile(t, f.dir, "notes/AGENTS.md", "# notes\n")

	f.m.cfg.Always = alwaysFor(f, []string{"infra", "web"})

	// web is on the command line, so it gets a worktree and no symlink.
	res, err := f.m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api", "web"}})
	if err != nil {
		t.Fatal(err)
	}
	root := res.Space.Path

	if got := res.Repos[0].Secrets; strings.Join(got, ",") != ".env,config/dev.json" {
		t.Errorf("api secrets = %v, want .env and config/dev.json", got)
	}
	if fi, err := os.Stat(filepath.Join(root, "api", ".env")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("api/.env = %v, %v; want mode 0600", fi, err)
	}
	if len(res.Repos[1].Secrets) != 0 {
		t.Errorf("web secrets = %v, want none (web has none)", res.Repos[1].Secrets)
	}

	link := filepath.Join(root, "myorg", "infra")
	if dst, err := os.Readlink(link); err != nil || dst != f.repos["myorg/infra"].Path {
		t.Errorf("infra link = %q, %v; want the main clone", dst, err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err != nil || string(b) != "# notes\n" {
		t.Errorf("AGENTS.md = %q, %v", b, err)
	}

	sp, err := f.m.Space("feat")
	if err != nil || len(sp.Repos) != 3 || !sp.Repos[2].Symlink || sp.Repos[2].Name != "myorg/infra" {
		t.Errorf("state repos = %+v, %v; want api, web, then the infra symlink", sp.Repos, err)
	}
}

func TestNewSpaceAlwaysPreflight(t *testing.T) {
	f := newFixture(t, "api", "infra")
	testhelper.WriteFile(t, f.dir, "notes/AGENTS.md", "x\n")
	testhelper.WriteFile(t, f.dir, "notes/api", "x\n")
	testhelper.WriteFile(t, f.repos["api"].Path, ".env/x", "x\n")

	tests := []struct {
		name   string
		always func(*config.Always)
		want   string
	}{
		{"unknown always repo", func(a *config.Always) { a.Repos = []string{"nope"} }, `always.repos: no repo "nope"`},
		{"missing file", func(a *config.Always) { a.Files = []string{filepath.Join(f.dir, "gone.md")} }, "always.files:"},
		{"file is a dir", func(a *config.Always) { a.Files = []string{filepath.Join(f.dir, "notes")} }, "is not a file"},
		{"file has a repo name", func(a *config.Always) { a.Files = []string{filepath.Join(f.dir, "notes/api")} }, "has the name of repo api"},
		{"secret is a dir", func(a *config.Always) { a.Secrets = []string{".env"} }, "api: always.secrets: .env is a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := config.Always{}
			tt.always(&a)
			f.m.cfg.Always = a

			_, err := f.m.NewSpace(context.Background(), NewOptions{Name: "x", Repos: []string{"api"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(f.dir, "spaces", "x")); !errors.Is(err, os.ErrNotExist) {
				t.Error("pre-flight made the space dir")
			}
		})
	}
}

func TestNewSpaceAlwaysRollback(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads a file with mode 000")
	}
	f := newFixture(t, "api", "infra")
	testhelper.WriteFile(t, f.repos["api"].Path, ".env", "KEY=1\n")
	testhelper.WriteFile(t, f.dir, "notes/AGENTS.md", "x\n")
	f.m.cfg.Always = alwaysFor(f, []string{"infra"})

	// Pre-flight only checks that the file is there; the copy, the last step before the state,
	// cannot read it.
	if err := os.Chmod(filepath.Join(f.dir, "notes/AGENTS.md"), 0); err != nil {
		t.Fatal(err)
	}

	_, err := f.m.NewSpace(context.Background(), NewOptions{Name: "feat", Repos: []string{"api"}})
	if !saga.IsRolledBack(err) {
		t.Fatalf("err = %v, want a full rollback", err)
	}
	if _, err := os.Lstat(filepath.Join(f.dir, "spaces", "feat")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rollback kept the space dir: %v", err)
	}
	if _, err := f.m.Space("feat"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("rollback kept the state: %v", err)
	}
}

func alwaysFor(f fixture, repos []string) config.Always {
	return config.Always{
		Repos:   repos,
		Files:   []string{filepath.Join(f.dir, "notes", "AGENTS.md")},
		Secrets: []string{".env", "config/dev.json", "missing.txt"},
	}
}
