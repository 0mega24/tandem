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
		{Generator: "go", Path: "same.txt", SHA256: Hash([]byte("v1"))},
		{Generator: "go", Path: "update.txt", SHA256: Hash([]byte("v1"))},
		{Generator: "go", Path: "edited.txt", SHA256: Hash([]byte("v1"))},
	}
	plans := map[string]Plan{"go": plan(
		file("edited.txt", "v2"), file("new/dir/new.txt", "v2"), file("same.txt", "v1"),
		file("theirs.txt", "v2"), file("update.txt", "v2"),
	)}

	a, err := Prepare(root, owned, plans, nil, false, nil)
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
	if paths["edited.txt"] != Hash([]byte("v1")) || paths["theirs.txt"] != "" || paths["new/dir/new.txt"] != Hash([]byte("v2")) {
		t.Errorf("owned = %+v", a.Owned)
	}
}

func TestForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "edited.txt", "a person's edit")
	writeFile(t, root, "theirs.txt", "not ours")
	owned := []Owned{{Generator: "go", Path: "edited.txt", SHA256: Hash([]byte("v1"))}}

	a, err := Prepare(root, owned, map[string]Plan{"go": plan(file("edited.txt", "v2"), file("theirs.txt", "v2"))}, nil, true, nil)
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
	if got := readFile(t, root, "edited.txt"+backupSuffix); got != "a person's edit" {
		t.Errorf("backup of the changed owned file = %q, want the edit", got)
	}
	if len(a.Owned) != 2 {
		t.Errorf("owned = %+v, want both files", a.Owned)
	}
}

func TestForceKeepsAnOlderBackup(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "theirs.txt", "not ours")
	writeFile(t, root, "theirs.txt"+backupSuffix, "an older backup")

	a, err := Prepare(root, nil, map[string]Plan{"go": plan(file("theirs.txt", "v2"))}, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Changes[0].Action != Skip {
		t.Fatalf("action = %v, want skip while a backup exists", a.Changes[0].Action)
	}
	run(t, a)
	if readFile(t, root, "theirs.txt") != "not ours" || readFile(t, root, "theirs.txt"+backupSuffix) != "an older backup" {
		t.Error("--force changed a file whose backup slot was taken")
	}
}

func TestDeleteWhenNoLongerPlanned(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".cargo/config.toml", "v1")
	writeFile(t, root, "edited.txt", "a person's edit")
	writeFile(t, root, "other.txt", "x")
	owned := []Owned{
		{Generator: "rust", Path: ".cargo/config.toml", SHA256: Hash([]byte("v1")), Stale: []string{"Cargo.lock"}},
		{Generator: "rust", Path: "edited.txt", SHA256: Hash([]byte("v1"))},
		{Generator: "js", Path: "other.txt", SHA256: Hash([]byte("x"))},
	}
	writeFile(t, root, "Cargo.lock", "lock")

	// rust ran and found no members; js did not run.
	a, err := Prepare(root, owned, map[string]Plan{"rust": {}}, nil, false, nil)
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
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "infra")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "real/x", "x")
	if err := os.Symlink(filepath.Join(root, "real/x"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../x", "/etc/x", "a/../b", "", "api/go.work", "api", `a\b`, "API/x", "infra/go.work", "link.txt"} {
		_, err := Prepare(root, nil, map[string]Plan{"go": plan(file(p, "x"))}, []string{"api"}, false, nil)
		if err == nil {
			t.Errorf("Prepare(%q) = nil, want an error", p)
		}
	}

	for _, s := range []string{"../x", "/etc/x", "api/go.mod"} {
		if _, err := Prepare(root, nil, map[string]Plan{"go": {Stale: []string{s}}}, []string{"api"}, false, nil); err == nil {
			t.Errorf("Prepare with stale %q = nil, want an error", s)
		}
		owned := []Owned{{Generator: "go", Path: "go.work", SHA256: "x", Stale: []string{s}}}
		if _, err := Prepare(root, owned, map[string]Plan{"go": {}}, []string{"api"}, false, nil); err == nil {
			t.Errorf("Prepare with owned stale %q = nil, want an error", s)
		}
	}

	twice := map[string]Plan{"go": plan(file("go.work", "a")), "rust": plan(file("go.work", "b"))}
	if _, err := Prepare(root, nil, twice, nil, false, nil); err == nil || !strings.Contains(err.Error(), "planned twice") {
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
		{Generator: "go", Path: "update.txt", SHA256: Hash([]byte("v1"))},
		{Generator: "rust", Path: "gone.txt", SHA256: Hash([]byte("g"))},
	}
	plans := map[string]Plan{
		"go":   plan(file("update.txt", "v2"), file("new/dir/new.txt", "n"), file("theirs.txt", "v2")),
		"rust": {},
	}

	a, err := Prepare(root, owned, plans, nil, true, nil)
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
	a, err := Prepare(root, nil, map[string]Plan{"go": plan(file("theirs.txt", "v2"))}, nil, true, nil)
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

func TestForceDeleteKeepsACopy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "edited.txt", "a person's edit")
	owned := []Owned{{Generator: "go", Path: "edited.txt", SHA256: Hash([]byte("v1"))}}

	a, err := Prepare(root, owned, map[string]Plan{"go": {}}, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)
	if readFile(t, root, "edited.txt") != "<missing>" || readFile(t, root, "edited.txt"+backupSuffix) != "a person's edit" {
		t.Error("a forced delete of a changed file did not keep a copy")
	}
}

func TestOwnedWithPlannedBytes(t *testing.T) {
	// A person made the file equal to the new plan: nothing to write, and the state gets the new
	// hash. A skipped file keeps the stale list that it was written with.
	root := t.TempDir()
	writeFile(t, root, "go.work", "v2")
	writeFile(t, root, "edited.txt", "a person's edit")
	owned := []Owned{
		{Generator: "go", Path: "go.work", SHA256: Hash([]byte("v1"))},
		{Generator: "go", Path: "edited.txt", SHA256: Hash([]byte("v1")), Stale: []string{"old.sum"}},
	}

	a, err := Prepare(root, owned, map[string]Plan{"go": plan(file("edited.txt", "v2"), file("go.work", "v2"))}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Changes[1].Action != Unchanged {
		t.Errorf("go.work: %v, want unchanged", a.Changes[1].Action)
	}
	if a.Owned[1].SHA256 != Hash([]byte("v2")) {
		t.Errorf("go.work hash = %s, want the hash of v2", a.Owned[1].SHA256)
	}
	if got := a.Owned[0].Stale; len(got) != 1 || got[0] != "old.sum" {
		t.Errorf("edited.txt stale = %v, want [old.sum]", got)
	}
}

func TestStaleStepsOnce(t *testing.T) {
	root := t.TempDir()
	a, err := Prepare(root, nil, map[string]Plan{"go": plan(file("a", "a"), file("b", "b"))}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, st := range a.Steps() {
		if strings.HasPrefix(st.Name, "delete stale ") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d stale steps for go.work.sum, want 1", n)
	}
}

func TestRemoveEmptyDirsKeepsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	removeEmptyDirs(filepath.Join(root, "link"), root)
	if _, err := os.Lstat(filepath.Join(root, "link")); err != nil {
		t.Errorf("link = %v, want the symlink kept", err)
	}
}

func TestFailedDeleteIsAWarning(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "locked/go.work", "v1")
	writeFile(t, root, "locked/go.work.sum", "sum")
	owned := []Owned{{Generator: "go", Path: "locked/go.work", SHA256: Hash([]byte("v1")), Stale: []string{"locked/go.work.sum"}}}
	a, err := Prepare(root, owned, map[string]Plan{"go": {}}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A directory without write permission: the files cannot be deleted.
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // so that TempDir can remove it

	run(t, a)
	if w := a.Warnings(); len(w) != 2 || !strings.Contains(w[0], "could not delete locked/go.work") {
		t.Errorf("warnings = %q, want one for go.work and one for go.work.sum", w)
	}
	if readFile(t, root, "locked/go.work") != "v1" || readFile(t, root, "locked/go.work.sum") != "sum" {
		t.Error("the files are gone")
	}
}

func TestLinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/subprojects/libfoo.wrap", "[wrap-git]\n")
	writeFile(t, root, "libfoo/meson.build", "")
	writeFile(t, root, "app/subprojects/taken/x", "")
	members := []string{"app", "libfoo"}
	yes := func(string) (bool, error) { return true, nil }
	links := func(ls ...Link) map[string]Plan { return map[string]Plan{"meson": {Links: ls}} }
	foo := Link{Path: "app/subprojects/libfoo", Target: "../../libfoo"}

	a, err := Prepare(root, nil, links(foo, Link{Path: "app/subprojects/taken", Target: "../../libfoo"}), members, true, yes)
	if err != nil {
		t.Fatal(err)
	}
	if a.Changes[0].Action != Write || a.Changes[1].Action != Skip {
		t.Fatalf("changes = %+v, want the link made and the taken path skipped, also with force", a.Changes)
	}
	run(t, a)
	if got, _ := os.Readlink(filepath.Join(root, foo.Path)); got != foo.Target {
		t.Fatalf("link = %q, want %q", got, foo.Target)
	}
	if len(a.Owned) != 1 || a.Owned[0].Target != foo.Target || a.Owned[0].SHA256 != "" {
		t.Errorf("owned = %+v, want the link with its target", a.Owned)
	}

	// The same plan again: unchanged. Not ignored by git: skipped.
	if a, _ := Prepare(root, a.Owned, links(foo), members, false, yes); a.Changes[0].Action != Unchanged {
		t.Errorf("second run: %+v, want unchanged", a.Changes[0])
	}
	bar := Link{Path: "app/subprojects/bar", Target: "../../libfoo"}
	if a, _ := Prepare(root, nil, links(bar), members, false, nil); a.Changes[0].Action != Skip || !strings.Contains(a.Changes[0].Reason, "does not ignore") {
		t.Errorf("not ignored: %+v, want a skip", a.Changes[0])
	}

	// No longer planned: the link goes, and only the link.
	d, err := Prepare(root, a.Owned, links(), members, false, yes)
	if err != nil {
		t.Fatal(err)
	}
	run(t, d)
	if _, err := os.Lstat(filepath.Join(root, foo.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("link = %v, want it deleted", err)
	}
	if readFile(t, root, "libfoo/meson.build") != "" {
		t.Error("the target is gone")
	}
}

func TestCheckLink(t *testing.T) {
	root := t.TempDir()
	members := []string{"app", "libfoo"}
	for _, l := range []Link{
		{Path: "subprojects/x", Target: "../libfoo"},        // not in a member
		{Path: "app/subprojects/x", Target: "/abs/libfoo"},  // absolute target
		{Path: "app/subprojects/x", Target: "../../../out"}, // target outside the root
		{Path: "app/../x", Target: "../libfoo"},
	} {
		if err := CheckLink(root, l, members); err == nil {
			t.Errorf("CheckLink(%+v) = nil, want an error", l)
		}
	}
	if err := CheckLink(root, Link{Path: "app/subprojects/x", Target: "../../libfoo"}, members); err != nil {
		t.Errorf("CheckLink(valid) = %v", err)
	}
}

func TestLinkOwnership(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/subprojects/x.wrap", "")
	writeFile(t, root, "libfoo/meson.build", "")
	writeFile(t, root, "libbar/meson.build", "")
	members := []string{"app", "libbar", "libfoo"}
	yes := func(string) (bool, error) { return true, nil }
	p := filepath.Join(root, "app/subprojects/dep")

	// A person's link with the planned target stays theirs.
	if err := os.Symlink("../../libfoo", p); err != nil {
		t.Fatal(err)
	}
	foo := Link{Path: "app/subprojects/dep", Target: "../../libfoo"}
	a, err := Prepare(root, nil, map[string]Plan{"meson": {Links: []Link{foo}}}, members, false, yes)
	if err != nil {
		t.Fatal(err)
	}
	if a.Changes[0].Action != Skip || len(a.Owned) != 0 {
		t.Errorf("a person's link: %+v, owned %+v; want a skip, not owned", a.Changes[0], a.Owned)
	}

	// tdm's link and a new target: the link is replaced.
	owned := []Owned{{Generator: "meson", Path: foo.Path, Target: foo.Target}}
	bar := Link{Path: foo.Path, Target: "../../libbar"}
	a, err = Prepare(root, owned, map[string]Plan{"meson": {Links: []Link{bar}}}, members, false, yes)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)
	if got, _ := os.Readlink(p); got != bar.Target || a.Owned[0].Target != bar.Target {
		t.Errorf("link = %q, owned %+v; want the new target", got, a.Owned)
	}

	// An owned link whose member left: dropped with a warning, not an error.
	gone := []Owned{{Generator: "meson", Path: "old/subprojects/dep", Target: "../../libfoo"}}
	a, err = Prepare(root, gone, map[string]Plan{"meson": {}}, members, false, yes)
	if err != nil || len(a.Owned) != 0 || len(a.Warnings()) != 1 {
		t.Errorf("owned link of a member that left: err %v, owned %+v, warnings %q", err, a.Owned, a.Warnings())
	}
}

// A stale path is a derived file such as go.work.sum. It must not name a file that a generator
// plans or owns: that file has its own ownership check, which a stale delete would skip.
func TestPrepareRejectsStaleGeneratedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.work", "edited by a person")
	owned := []Owned{{Generator: "go", Path: "go.work", SHA256: "x"}}
	plans := map[string]Plan{
		"go":  {Files: []File{file("go.work", "v2")}},
		"zig": {Files: []File{file(".tandem/zig/forks.args", "x")}, Stale: []string{"go.work"}},
	}
	if _, err := Prepare(root, owned, plans, nil, false, nil); err == nil {
		t.Error("Prepare = nil error, want: stale go.work is a generated file")
	}
}

// A forced replace keeps the mode of the old file, in the file and in its backup: a private file
// must not become readable by others.
func TestForceKeepsMode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "secret.txt", "private")
	if err := os.Chmod(filepath.Join(root, "secret.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Prepare(root, nil, map[string]Plan{"go": {Files: []File{file("secret.txt", "v2")}}}, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	run(t, a)
	for _, p := range []string{"secret.txt", "secret.txt" + backupSuffix} {
		fi, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", p, fi.Mode().Perm())
		}
	}
}
