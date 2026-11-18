package space

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/0mega24/tandem/internal/git/testhelper"
	"github.com/0mega24/tandem/internal/state"
)

func TestStatus(t *testing.T) {
	f := newFixture(t, "api", "web")
	ctx := context.Background()
	for _, n := range []string{"a", "b"} {
		if _, _, err := f.m.NewSpace(ctx, NewOptions{Name: n, Repos: []string{"api", "web"}}); err != nil {
			t.Fatal(err)
		}
	}

	bAPI := filepath.Join(f.dir, "spaces", "b", "api")
	testhelper.Commit(t, bAPI, "new.txt", "n\n", "work")
	testhelper.WriteFile(t, bAPI, "wip.txt", "w\n")

	// Run from inside space b: it comes first.
	got, err := f.m.Status(ctx, nil, filepath.Join(bAPI, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Space.Name != "b" || got[1].Space.Name != "a" {
		t.Fatalf("order = %v, want b first", spaceNames(got))
	}

	r := got[0].Repos[0]
	if r.Err != nil || r.Entry.Name != "api" || r.Unpushed != 1 || len(r.Status.Changed) != 1 {
		t.Errorf("b/api = %+v, want 1 unpushed and 1 changed", r)
	}
	if w := got[0].Repos[1]; w.Err != nil || !w.Status.Clean() || w.Unpushed != 0 {
		t.Errorf("b/web = %+v, want clean", w)
	}
}

func TestStatusShowsTheCheckedOutBranch(t *testing.T) {
	f := newFixture(t, "api")
	ctx := context.Background()
	if _, _, err := f.m.NewSpace(ctx, NewOptions{Name: "a", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(f.dir, "spaces", "a", "api")
	testhelper.Git(t, wt, "switch", "-q", "-c", "other")
	testhelper.Commit(t, wt, "o.txt", "o\n", "on other")

	got, err := f.m.Status(ctx, []string{"a"}, f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := got[0].Repos[0]; r.Status.Branch != "other" || r.Unpushed != 1 {
		t.Errorf("api = branch %q, %d unpushed; want other with 1", r.Status.Branch, r.Unpushed)
	}
}

func TestStatusNames(t *testing.T) {
	f := newFixture(t, "api")
	ctx := context.Background()
	if _, _, err := f.m.NewSpace(ctx, NewOptions{Name: "a", Repos: []string{"api"}}); err != nil {
		t.Fatal(err)
	}

	got, err := f.m.Status(ctx, []string{"a", "a"}, f.dir)
	if err != nil || len(got) != 1 {
		t.Fatalf("Status(a, a) = %v, %v; want one space", spaceNames(got), err)
	}

	if _, err := f.m.Status(ctx, []string{"nope"}, f.dir); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("Status(nope) err = %v, want ErrNotFound", err)
	}
}

func spaceNames(list []SpaceStatus) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Space.Name
	}
	return out
}
