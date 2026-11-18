// Package gentest runs the golden-file tests of a generator.
//
// Each directory testdata/<case>/ is one space:
//
//	in/<member>/...        one directory per member worktree, named by its short name ("--" for "/")
//	in/<name>.symlink/     a plain directory that stands for an always.repos symlink: it is not
//	                       a member; the generator sees it in Space.Symlinks
//	origins.json           optional: {"<member>": "<origin URL>"}
//	config.json            optional: the [generate.<id>] table
//	want/...               the files that Plan writes, relative to the space root
//	want.txt               optional: the warnings, stale paths and links ("warning: ...",
//	                       "stale: ...", "link: <path> -> <target>")
//
// Each case also checks the rules that apply to every plan: the files are sorted by path with no
// path twice, every path passes gen.CheckPath (gen.CheckLink for links), and a second Plan gives
// the same result.
//
// `go test ./internal/gen/<id>/ -update` writes want/ and want.txt again from the plan.
package gentest

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
)

var update = flag.Bool("update", false, "write the golden files of the generator tests again")

// SpaceName is the space name in every case, so the ownership headers are the same.
const SpaceName = "feat"

// Run runs every case in testdata/ against g.
func Run(t *testing.T, g gen.Generator) {
	t.Helper()
	cases, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			runCase(t, g, filepath.Join("testdata", c.Name()))
		})
	}
}

func runCase(t *testing.T, g gen.Generator, dir string) {
	in, err := filepath.Abs(filepath.Join(dir, "in"))
	if err != nil {
		t.Fatal(err)
	}

	space := gen.Space{Name: SpaceName, Root: in, Branch: SpaceName}
	readJSON(t, filepath.Join(dir, "config.json"), &space.Config)
	origins := map[string]string{}
	readJSON(t, filepath.Join(dir, "origins.json"), &origins)

	entries, err := os.ReadDir(in)
	if err != nil {
		t.Fatal(err)
	}
	var memberDirs []string // the worktree paths relative to the root, for gen.CheckPath
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if n, ok := strings.CutSuffix(e.Name(), ".symlink"); ok {
			p := filepath.Join(in, e.Name())
			space.Symlinks = append(space.Symlinks, gen.Member{Name: n, WorktreePath: p, RepoPath: p, OriginURL: origins[n]})
			continue
		}
		memberDirs = append(memberDirs, e.Name())
		name := strings.ReplaceAll(e.Name(), "--", "/")
		p := filepath.Join(in, e.Name())
		space.Members = append(space.Members, gen.Member{Name: name, WorktreePath: p, RepoPath: p, OriginURL: origins[name]})
	}
	slices.SortFunc(space.Members, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })

	var found []gen.Detection
	for _, m := range space.Members {
		d, err := g.Detect(m, gen.DetectOptions{Config: space.Config})
		if err != nil {
			t.Fatalf("Detect(%s): %v", m.Name, err)
		}
		if d != nil {
			found = append(found, *d)
		}
	}

	plan, err := g.Plan(space, found)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	again, err := g.Plan(space, found)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if !reflect.DeepEqual(plan, again) {
		t.Error("a second Plan gave another result: a plan must not depend on map order or time")
	}
	checkRules(t, in, plan, memberDirs)

	got := files(plan)
	notes := summary(plan)
	if *update {
		writeWant(t, dir, got, notes)
		return
	}
	compare(t, dir, got, notes)
}

// files returns the planned files by path, with the paths in the OS form.
func files(p gen.Plan) map[string][]byte {
	out := map[string][]byte{}
	for _, f := range p.Files {
		out[filepath.FromSlash(f.Path)] = f.Content
	}
	return out
}

// checkRules fails the test for a plan that Prepare would refuse or that is not sorted.
func checkRules(t *testing.T, root string, p gen.Plan, members []string) {
	t.Helper()
	for i, f := range p.Files {
		if i > 0 && p.Files[i-1].Path >= f.Path {
			t.Errorf("files are not sorted by path, or %s is there twice", f.Path)
		}
		if err := gen.CheckPath(root, f.Path, members); err != nil {
			t.Error(err)
		}
	}
	for _, s := range p.Stale {
		if err := gen.CheckPath(root, s, members); err != nil {
			t.Errorf("stale: %v", err)
		}
	}
	for _, l := range p.Links {
		if err := gen.CheckLink(root, l, members); err != nil {
			t.Errorf("link: %v", err)
		}
	}
}

func summary(p gen.Plan) string {
	var b strings.Builder
	for _, w := range p.Warnings {
		b.WriteString("warning: " + w + "\n")
	}
	for _, s := range p.Stale {
		b.WriteString("stale: " + s + "\n")
	}
	for _, l := range p.Links {
		b.WriteString("link: " + l.Path + " -> " + l.Target + "\n")
	}
	return b.String()
}

func compare(t *testing.T, dir string, got map[string][]byte, notes string) {
	t.Helper()
	want := readTree(t, filepath.Join(dir, "want"))

	for p, b := range got {
		w, ok := want[p]
		switch {
		case !ok:
			t.Errorf("plan writes %s, which want/ does not have (run with -update)", p)
		case !bytes.Equal(b, w):
			t.Errorf("%s differs from want/ (run with -update to see the diff in git)\ngot:\n%s\nwant:\n%s", p, b, w)
		}
	}
	for p := range want {
		if _, ok := got[p]; !ok {
			t.Errorf("want/ has %s, which the plan does not write", p)
		}
	}

	wantNotes, err := os.ReadFile(filepath.Join(dir, "want.txt")) //nolint:gosec // G304: a test fixture
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if notes != string(wantNotes) {
		t.Errorf("warnings and stale paths:\ngot:\n%s\nwant (want.txt):\n%s", notes, wantNotes)
	}
}

func writeWant(t *testing.T, dir string, got map[string][]byte, notes string) {
	t.Helper()
	want := filepath.Join(dir, "want")
	if err := os.RemoveAll(want); err != nil {
		t.Fatal(err)
	}
	for p, b := range got {
		dst := filepath.Join(want, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // test fixtures in the repo
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil { //nolint:gosec // test fixtures in the repo
			t.Fatal(err)
		}
	}

	notesFile := filepath.Join(dir, "want.txt")
	if notes == "" {
		if err := os.Remove(notesFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(notesFile, []byte(notes), 0o644); err != nil { //nolint:gosec // test fixtures in the repo
		t.Fatal(err)
	}
}

// readTree reads all files under dir by their path relative to dir. A missing dir is empty: the
// "no members" case writes nothing.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p) //nolint:gosec // G304: a test fixture
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		out[rel] = b
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return out
}

func readJSON(t *testing.T, file string, v any) {
	t.Helper()
	b, err := os.ReadFile(file) //nolint:gosec // G304: a test fixture
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
}
