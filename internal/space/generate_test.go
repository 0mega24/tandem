package space

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	_ "github.com/0mega24/tandem/internal/gen/golang" // the go generator, the default set in these tests
	"github.com/0mega24/tandem/internal/git/testhelper"
)

func writeTestFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateMigratesGoWorkspace(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	root := filepath.Join(f.dir, "spaces", "feat")
	writeTestFile(t, filepath.Join(root, "api/go.mod"), "module example.com/api\n\ngo 1.23\n")
	writeTestFile(t, filepath.Join(root, "go.work"), "go 1.21\n\nuse ./api\n") // as wtg wrote it

	// A wtg state file: go_workspace and no generated list.
	stateFile := filepath.Join(f.dir, "state", "feat.yaml")
	b, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, stateFile, string(b)+"go_workspace: true\n")

	res, err := f.m.Generate(context.Background(), "feat", GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changes) != 1 || res.Changes[0].Action != gen.Write {
		t.Fatalf("changes = %+v, want go.work written: the migration made it tdm's", res.Changes)
	}
	sp, err := f.m.Space("feat")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sp.Generators, []string{"go"}) || len(sp.Generated) != 1 || sp.GoWorkspace {
		t.Errorf("state = generators %v, generated %+v, go_workspace %v", sp.Generators, sp.Generated, sp.GoWorkspace)
	}
}

func TestGenerateRollback(t *testing.T) {
	f := newFixture(t, "api")
	newSpaceFor(t, f, "api")
	root := filepath.Join(f.dir, "spaces", "feat")
	writeTestFile(t, filepath.Join(root, "api/go.mod"), "module example.com/api\n\ngo 1.23\n")
	writeTestFile(t, filepath.Join(root, "go.work.sum"), "sum")

	// The state save, the last step, fails: the files must be as before.
	stateDir := filepath.Join(f.dir, "state")
	if err := os.Chmod(stateDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) }) // so that TempDir can remove it

	if _, err := f.m.Generate(context.Background(), "feat", GenerateOptions{}); err == nil {
		t.Fatal("Generate = nil error, want the save to fail")
	}
	if _, err := os.Stat(filepath.Join(root, "go.work")); !os.IsNotExist(err) {
		t.Errorf("go.work = %v, want it removed by the rollback", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "go.work.sum")); err != nil || string(b) != "sum" {
		t.Errorf("go.work.sum = %q, %v; want it back", b, err)
	}
}

func TestIgnoredChecksTheMember(t *testing.T) {
	f := newFixture(t, "app")
	testhelper.Commit(t, f.repos["app"].Path, ".gitignore", "subprojects/*\n!subprojects/*.wrap\n", "ignore")
	newSpaceFor(t, f, "app")
	sp, err := f.m.Space("feat")
	if err != nil {
		t.Fatal(err)
	}

	ignored := f.m.ignored(context.Background(), sp)
	for rel, want := range map[string]bool{"app/subprojects/libfoo": true, "app/subprojects/libfoo.wrap": false, "go.work": false} {
		if got, err := ignored(rel); err != nil || got != want {
			t.Errorf("ignored(%s) = %v, %v; want %v", rel, got, err, want)
		}
	}
}
