package gen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/saga"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if errors.Is(err, os.ErrNotExist) {
		return "<missing>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t *testing.T, a Applied) {
	t.Helper()
	if err := saga.Run(context.Background(), a.Steps()); err != nil {
		t.Fatal(err)
	}
}

func plan(files ...File) Plan { return Plan{Files: files, Stale: []string{"go.work.sum"}} }

func file(p, content string) File { return File{Path: p, Content: []byte(content)} }

func TestOwnershipTable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "same.txt", "v1")
	writeFile(t, root, "update.txt", "v1")
	writeFile(t, root, "edited.txt", "a person's edit")
	writeFile(t, root, "theirs.txt", "not ours")
	writeFile(t, root, "go.work.sum", "sum")
	owned := []Owned{
		{Generator: "go", Path: "same.txt", SHA256: hash([]byte("v1"))},
		{Generator: "go", Path: "update.txt", SHA256: hash([]byte("v1"))},
		{Generator: "go", Path: "edited.txt", SHA256: hash([]byte("v1"))},
	}
	plans := map[string]Plan{"go": plan(
		file("edited.txt", "v2"), file("new/dir/new.txt", "v2"), file("same.txt", "v1"),
		file("theirs.txt", "v2"), file("update.txt", "v2"),
	)}

	a, err := Prepare(root, owned, plans, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Action{"edited.txt": Skip, "new/dir/new.txt": Write, "same.txt": Unchanged, "theirs.txt": Skip, "update.txt": Write}
	for _, c := range a.Changes {
		if c.Action != want[c.Path] {
			t.Errorf("%s: %v (%s), want %v", c.Path, c.Action, c.Reason, want[c.Path])
		}
	}
	run(t, a)

	if got := readFile(t, root, "edited.txt"); got != "a person's edit" {
		t.Errorf("edited.txt = %q, want it kept", got)
	}
	if got := readFile(t, root, "theirs.txt"); got != "not ours" {
		t.Errorf("theirs.txt = %q, want it kept", got)
	}
	if got := readFile(t, root, "go.work.sum"); got != "<missing>" {
		t.Errorf("go.work.sum = %q, want it deleted after a change", got)
	}

	// edited.txt stays owned with its old hash; theirs.txt is not owned.
	paths := map[string]string{}
	for _, o := range a.Owned {
		paths[o.Path] = o.SHA256
	}
	if paths["edited.txt"] != hash([]byte("v1")) || paths["theirs.txt"] != "" || paths["new/dir/new.txt"] != hash([]byte("v2")) {
		t.Errorf("owned = %+v", a.Owned)
	}
}

func TestForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "edited.txt", "a person's edit")
	writeFile(t, root, "theirs.txt", "not ours")
	owned := []Owned{{Generator: "go", Path: "edited.txt", SHA256: hash([]byte("v1"))}}

	a, err := Prepare(root, owned, map[string]Plan{"go": plan(file("edited.txt", "v2"), file("theirs.txt", "v2"))}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)

	if readFile(t, root, "edited.txt") != "v2" || readFile(t, root, "theirs.txt") != "v2" {
		t.Error("--force did not write both files")
	}
	if got := readFile(t, root, "theirs.txt"+backupSuffix); got != "not ours" {
		t.Errorf("backup = %q, want the old file", got)
	}
	if len(a.Owned) != 2 {
		t.Errorf("owned = %+v, want both files", a.Owned)
	}
}

func TestDeleteWhenNoLongerPlanned(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".cargo/config.toml", "v1")
	writeFile(t, root, "edited.txt", "a person's edit")
	writeFile(t, root, "other.txt", "x")
	owned := []Owned{
		{Generator: "rust", Path: ".cargo/config.toml", SHA256: hash([]byte("v1")), Stale: []string{"Cargo.lock"}},
		{Generator: "rust", Path: "edited.txt", SHA256: hash([]byte("v1"))},
		{Generator: "js", Path: "other.txt", SHA256: hash([]byte("x"))},
	}
	writeFile(t, root, "Cargo.lock", "lock")

	// rust ran and found no members; js did not run.
	a, err := Prepare(root, owned, map[string]Plan{"rust": {}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)

	if _, err := os.Stat(filepath.Join(root, ".cargo")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".cargo/ = %v, want the empty dir removed", err)
	}
	if readFile(t, root, "Cargo.lock") != "<missing>" {
		t.Error("the stale file of a deleted file is still there")
	}
	if readFile(t, root, "edited.txt") != "a person's edit" || readFile(t, root, "other.txt") != "x" {
		t.Error("deleted a changed file or a file of a generator that did not run")
	}
	if len(a.Owned) != 2 || a.Owned[0].Path != "edited.txt" || a.Owned[1].Path != "other.txt" {
		t.Errorf("owned = %+v, want edited.txt and other.txt", a.Owned)
	}
}

func TestPrepareRejectsBadPaths(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"../x", "/etc/x", "a/../b", "", "api/go.work", "api", `a\b`} {
		_, err := Prepare(root, nil, map[string]Plan{"go": plan(file(p, "x"))}, []string{"api"}, false)
		if err == nil {
			t.Errorf("Prepare(%q) = nil, want an error", p)
		}
	}

	for _, s := range []string{"../x", "/etc/x", "api/go.mod"} {
		if _, err := Prepare(root, nil, map[string]Plan{"go": {Stale: []string{s}}}, []string{"api"}, false); err == nil {
			t.Errorf("Prepare with stale %q = nil, want an error", s)
		}
		owned := []Owned{{Generator: "go", Path: "go.work", SHA256: "x", Stale: []string{s}}}
		if _, err := Prepare(root, owned, map[string]Plan{"go": {}}, []string{"api"}, false); err == nil {
			t.Errorf("Prepare with owned stale %q = nil, want an error", s)
		}
	}

	twice := map[string]Plan{"go": plan(file("go.work", "a")), "rust": plan(file("go.work", "b"))}
	if _, err := Prepare(root, nil, twice, nil, false); err == nil || !strings.Contains(err.Error(), "planned twice") {
		t.Errorf("two generators on one path: %v", err)
	}
}

func TestStepsUndo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "update.txt", "v1")
	writeFile(t, root, "gone.txt", "g")
	writeFile(t, root, "theirs.txt", "not ours")
	writeFile(t, root, "go.work.sum", "sum")
	owned := []Owned{
		{Generator: "go", Path: "update.txt", SHA256: hash([]byte("v1"))},
		{Generator: "rust", Path: "gone.txt", SHA256: hash([]byte("g"))},
	}
	plans := map[string]Plan{
		"go":   plan(file("update.txt", "v2"), file("new/dir/new.txt", "n"), file("theirs.txt", "v2")),
		"rust": {},
	}

	a, err := Prepare(root, owned, plans, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	fail := saga.Step{Name: "fail", Do: func(context.Context) error { return errors.New("boom") }}
	if err := saga.Run(context.Background(), append(a.Steps(), fail)); !saga.IsRolledBack(err) {
		t.Fatalf("err = %v, want a full rollback", err)
	}

	for rel, want := range map[string]string{
		"update.txt": "v1", "gone.txt": "g", "theirs.txt": "not ours", "go.work.sum": "sum",
		"new/dir/new.txt": "<missing>", "theirs.txt" + backupSuffix: "<missing>",
	} {
		if got := readFile(t, root, rel); got != want {
			t.Errorf("%s = %q after the rollback, want %q", rel, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("new/ = %v, want the made dirs removed", err)
	}
}

func TestWriteStepCleansUpAFailure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "theirs.txt", "not ours")
	a, err := Prepare(root, nil, map[string]Plan{"go": plan(file("theirs.txt", "v2"))}, nil, true)
	if err != nil {
		t.Fatal(err)
	}

	// Between Prepare and the write, the file becomes a directory with content: the rename fails
	// after the backup was made.
	if err := os.Remove(filepath.Join(root, "theirs.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "theirs.txt/keep", "k")

	if err := saga.Run(context.Background(), a.Steps()); err == nil {
		t.Fatal("Steps ran, want the write to fail")
	}
	if got := readFile(t, root, "theirs.txt"+backupSuffix); got != "<missing>" {
		t.Errorf("backup = %q, want it removed with the failed step", got)
	}
	if got := readFile(t, root, "theirs.txt/keep"); got != "k" {
		t.Errorf("theirs.txt/keep = %q, want it untouched", got)
	}
}
