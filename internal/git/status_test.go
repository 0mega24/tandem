package git

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0mega24/tandem/internal/git/testhelper"
)

func TestParseStatus(t *testing.T) {
	out := "# branch.oid abc\x00# branch.head me/feat\x00# branch.upstream origin/me/feat\x00# branch.ab +2 -1\x00" +
		"1 .M N... 100644 100644 100644 aaa aaa has space.go\x00" +
		"2 R. N... 100644 100644 100644 bbb bbb R100 new.go\x00old.go\x00" +
		"u UU N... 100644 100644 100644 100644 c1 c2 c3 conflict.go\x00" +
		"? untracked.txt\x00"
	want := Status{
		Branch: "me/feat", Upstream: "origin/me/feat", Ahead: 2, Behind: 1,
		Changed: []string{"has space.go", "new.go", "conflict.go", "untracked.txt"},
	}
	got, err := parseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseStatus() = %+v, want %+v", got, want)
	}
}

func TestParseStatusDetached(t *testing.T) {
	got, err := parseStatus("# branch.oid abc\x00# branch.head (detached)\x00")
	if err != nil || got.Branch != "" || !got.Clean() {
		t.Errorf("parseStatus(detached) = %+v, %v; want no branch, clean", got, err)
	}
}

func TestParseStatusBadEntry(t *testing.T) {
	if _, err := parseStatus("X what\x00"); err == nil {
		t.Error("parseStatus(unknown entry) = nil error, want an error")
	}
}

func TestStatusAndUnpushed(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}
	wt := filepath.Join(dir, "space", "api")
	if err := s.AddWorktree(ctx, r.Path, wt, "me/feat", "main"); err != nil {
		t.Fatal(err)
	}

	st, err := s.Status(ctx, wt)
	if err != nil || !st.Clean() || st.Branch != "me/feat" || st.Upstream != "" {
		t.Errorf("Status(new worktree) = %+v, %v; want clean on me/feat with no upstream", st, err)
	}
	if n, err := s.Unpushed(ctx, r.Path, "me/feat"); err != nil || n != 0 {
		t.Errorf("Unpushed(new branch) = %d, %v; want 0", n, err)
	}

	testhelper.Commit(t, wt, "a.txt", "a\n", "one")
	testhelper.Commit(t, wt, "b.txt", "b\n", "two")
	testhelper.WriteFile(t, wt, "new dir/c.txt", "c\n")
	st, err = s.Status(ctx, wt)
	if err != nil || !reflect.DeepEqual(st.Changed, []string{"new dir/c.txt"}) {
		t.Errorf("Status() = %+v, %v; want one untracked file", st, err)
	}
	if n, err := s.Unpushed(ctx, r.Path, "me/feat"); err != nil || n != 2 {
		t.Errorf("Unpushed() = %d, %v; want 2", n, err)
	}

	testhelper.Git(t, wt, "push", "-q", "-u", "origin", "me/feat")
	testhelper.Commit(t, wt, "d.txt", "d\n", "three")
	st, err = s.Status(ctx, wt)
	if err != nil || st.Upstream != "origin/me/feat" || st.Ahead != 1 || st.Behind != 0 {
		t.Errorf("Status() after push = %+v, %v; want upstream set and ahead 1", st, err)
	}
	if n, err := s.Unpushed(ctx, r.Path, "me/feat"); err != nil || n != 1 {
		t.Errorf("Unpushed() after push = %d, %v; want 1", n, err)
	}
}

func TestUnpushedWithoutOrigin(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	testhelper.Git(t, dir, "init", "-q")
	testhelper.Commit(t, dir, "a.txt", "a\n", "one")

	if n, err := (System{}).Unpushed(context.Background(), dir, "main"); err != nil || n != 0 {
		t.Errorf("Unpushed() = %d, %v; want 0 for a repo with no origin", n, err)
	}
}

func TestCheckIgnore(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	testhelper.Git(t, dir, "init", "-q")
	testhelper.Commit(t, dir, ".gitignore", "subprojects/*\n!subprojects/*.wrap\n", "ignore")

	ctx := context.Background()
	for p, want := range map[string]bool{"subprojects/libfoo": true, "subprojects/libfoo.wrap": false, "src/x": false} {
		if got, err := (System{}).CheckIgnore(ctx, dir, p); err != nil || got != want {
			t.Errorf("CheckIgnore(%s) = %v, %v; want %v", p, got, err, want)
		}
	}
	if _, err := (System{}).CheckIgnore(ctx, t.TempDir(), "x"); err == nil {
		t.Error("CheckIgnore outside a repo = nil error")
	}
}
