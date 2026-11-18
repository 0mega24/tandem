package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"go.mod", "go.sum"} {
		b, err := os.ReadFile(filepath.Join("../..", name))
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, name, string(b))
	}
	if err := os.CopyFS(filepath.Join(root, "internal"), os.DirFS("../../internal")); err != nil {
		t.Fatal(err)
	}
	if err := run(root, "demo-kit", "demo.txt"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "internal/gen/demo-kit")
	for _, file := range []string{"doc.go", "demo-kit.go", "demo-kit_test.go"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"depends", "mixed", "nested", "no-members", "one-member", "symlink"} {
		for _, sub := range []string{"in", "want"} {
			if info, err := os.Stat(filepath.Join(dir, "testdata", name, sub)); err != nil || !info.IsDir() {
				t.Fatalf("missing %s/%s: %v", name, sub, err)
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "doc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "//tandem:generator id=demo-kit manifest=demo.txt") {
		t.Fatalf("doc.go metadata: %s", b)
	}
	command(t, root, true, "vet", "./internal/gen/demo-kit")
	write(t, root, "internal/gen/demo-kit/detect_test.go", `package demo_kit
import (
 "os"
 "path/filepath"
 "testing"
 "github.com/0mega24/tandem/internal/gen"
)
func TestScaffoldDetection(t *testing.T) {
 root := t.TempDir()
 m := gen.Member{WorktreePath: root}
 g := Generator{}
 if d, err := g.Detect(m, gen.DetectOptions{}); err != nil || d != nil {
  t.Fatalf("missing manifest: %v, %v", d, err)
 }
 if err := os.Mkdir(filepath.Join(root, "nested"), 0755); err != nil { t.Fatal(err) }
 if err := os.WriteFile(filepath.Join(root, "nested/demo.txt"), nil, 0644); err != nil { t.Fatal(err) }
 if d, err := g.Detect(m, gen.DetectOptions{Depth: 2}); err != nil || d != nil {
  t.Fatalf("nested manifest: %v, %v", d, err)
 }
 if err := os.WriteFile(filepath.Join(root, "demo.txt"), nil, 0644); err != nil { t.Fatal(err) }
 d, err := g.Detect(m, gen.DetectOptions{})
 if err != nil || d == nil || d.Member != m || d.Manifest != "demo.txt" {
  t.Fatalf("root manifest: %v, %v", d, err)
 }
 if err := os.RemoveAll(root); err != nil { t.Fatal(err) }
 if _, err := g.Detect(m, gen.DetectOptions{}); err == nil {
  t.Fatal("Detect hid a missing worktree")
 }
}
`)
	command(t, root, true, "test", "./internal/gen/demo-kit", "-run", "TestScaffoldDetection")
	out := command(t, root, false, "test", "./internal/gen/demo-kit", "-run", "TestGolden", "-v")
	if got := strings.Count(out, "Plan: not implemented: edit internal/gen/demo-kit/demo-kit.go"); got != 6 {
		t.Fatalf("got %d Plan errors, want 6:\n%s", got, out)
	}
	before, err := os.ReadFile(filepath.Join(dir, "demo-kit.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run(root, "demo-kit", "other.txt"); err == nil {
		t.Fatal("run accepted an existing generator")
	}
	after, err := os.ReadFile(filepath.Join(dir, "demo-kit.go"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("existing generator changed: %v", err)
	}
}

func TestRunRejectsInvalidInput(t *testing.T) {
	for _, id := range []string{"", "none", "Zig", "../zig", "a/b", "a_b", "1zig"} {
		t.Run("id="+id, func(t *testing.T) {
			root := t.TempDir()
			if err := run(root, id, "demo.txt"); err == nil {
				t.Fatal("run accepted an invalid ID")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid input wrote files: %v, %v", entries, err)
			}
		})
	}
	for _, manifest := range []string{"", "../demo", "/demo", "a/b", "a\\b", ".", "..", "demo\n.txt", "demo txt"} {
		t.Run("manifest="+manifest, func(t *testing.T) {
			if err := run(t.TempDir(), "demo", manifest); err == nil {
				t.Fatal("run accepted an invalid root manifest")
			}
		})
	}
}

func TestRunWithKeywordID(t *testing.T) {
	for _, id := range []string{"go", "main"} {
		root := t.TempDir()
		if err := run(root, id, "manifest.txt"); err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/gen", id, id+".go"), nil, parser.PackageClauseOnly)
		if err != nil || file.Name.Name == "main" {
			t.Fatalf("invalid importable package for %s: %v", id, err)
		}
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func command(t *testing.T, root string, success bool, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if (err == nil) != success {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRunWithGoManifest(t *testing.T) {
	if err := run(t.TempDir(), "demo", "build.go"); err != nil {
		t.Fatal(err)
	}
}
