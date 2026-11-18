package space

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/state"
)

func TestExec(t *testing.T) {
	f := newFixture(t, "api", "web", "infra")
	f.m.cfg.Always = config.Always{Repos: []string{"infra"}}
	newSpaceFor(t, f, "api", "web")
	testhelper.WriteFile(t, filepath.Join(f.dir, "spaces", "feat", "api"), "fail", "x\n")

	var out bytes.Buffer
	var heads []string
	got, err := f.m.Exec(context.Background(), "feat", []string{"sh", "-c", "pwd; test ! -e fail"}, &out, &out,
		func(e state.RepoEntry) { heads = append(heads, e.Name) })
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(heads, ",") != "api,web" || len(got) != 2 {
		t.Fatalf("ran in %v, want api and web (not the infra symlink)", heads)
	}
	if got[0].Err == nil || got[1].Err != nil {
		t.Errorf("errors = %v, %v; want api to fail and web to pass", got[0].Err, got[1].Err)
	}
	if !strings.Contains(out.String(), filepath.Join("feat", "web")+"\n") {
		t.Errorf("output %q does not show the web worktree", out.String())
	}
}
