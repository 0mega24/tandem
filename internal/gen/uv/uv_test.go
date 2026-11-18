package uv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestPlanWorkspace(t *testing.T) {
	p, err := (Generator{}).Plan(gen.Space{Name: "feat"}, []gen.Detection{{
		Member: gen.Member{Name: "api"},
		Facts:  map[string]any{"packages": []project{{Name: "api", Path: "."}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 {
		t.Fatalf("Plan files = %d, want 1", len(p.Files))
	}
	for _, want := range []string{"# " + gen.Header("feat"), "[tool.uv.workspace]", `"./api"`, `"api" = { workspace = true }`} {
		if !strings.Contains(string(p.Files[0].Content), want) {
			t.Errorf("Plan content = %s, want %q", p.Files[0].Content, want)
		}
	}
}

func TestRootWorkspaceControlsDepth(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "pyproject.toml", "[tool.uv.workspace]\nmembers = [\"packages/core\"]\n")
	writeManifest(t, root, "packages/core/pyproject.toml", "[project]\nname = \"core\"\n")
	writeManifest(t, root, "packages/other/pyproject.toml", "[project]\nname = \"other\"\n")
	d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: root}, gen.DetectOptions{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("Detect = nil, want workspace")
	}
	packages := d.Facts["packages"].([]project)
	if len(packages) != 1 || packages[0].Name != "core" {
		t.Fatalf("Detect packages = %v, want only core", packages)
	}
}

func writeManifest(t *testing.T, root, file, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGolden(t *testing.T) {
	gentest.Run(t, Generator{})
}

func TestPlanConfig(t *testing.T) {
	_, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"mode": "workspace"}}, nil)
	if err == nil {
		t.Fatal("Plan accepted an unknown config key")
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		depth   int
		want    []string
		wantErr bool
	}{
		{name: "root-only", files: map[string]string{"src/pyproject.toml": "[project]\nname = \"nested\"\n"}},
		{name: "depth", depth: 1, files: map[string]string{"src/pyproject.toml": "[project]\nname = \"nested\"\n"}, want: []string{"nested"}},
		{name: "ignored", depth: 2, files: map[string]string{".gitignore": "ignored/\n", "ignored/pyproject.toml": "[project]\nname = \"ignored\"\n", ".venv/pyproject.toml": "[project]\nname = \"venv\"\n"}},
		{name: "tool-only", files: map[string]string{"pyproject.toml": "[tool.ruff]\nline-length = 100\n"}},
		{name: "malformed", files: map[string]string{"pyproject.toml": "[project\n"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for file, content := range tt.files {
				writeManifest(t, root, file, content)
			}
			d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: root}, gen.DetectOptions{Depth: tt.depth})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect error = %v, want error %v", err, tt.wantErr)
			}
			var names []string
			if d != nil {
				for _, pkg := range d.Facts["packages"].([]project) {
					names = append(names, pkg.Name)
				}
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("Detect packages = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestDetectSymlinks(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "outside/pyproject.toml", "[project]\nname = \"outside\"\n")
	worktree := filepath.Join(root, "repo")
	if err := os.Mkdir(worktree, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct{ target, name string }{
		{filepath.Join(root, "outside/pyproject.toml"), "pyproject.toml"},
		{filepath.Join(root, "outside"), "linked"},
	} {
		if err := os.Symlink(link.target, filepath.Join(worktree, link.name)); err != nil {
			t.Fatal(err)
		}
	}
	d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: worktree}, gen.DetectOptions{Depth: 2})
	if err != nil || d != nil {
		t.Fatalf("Detect symlinks = %v, %v; want nil, nil", d, err)
	}
}

func TestPlanDeterministic(t *testing.T) {
	g := Generator{}
	root := t.TempDir()
	var found []gen.Detection
	for _, name := range []string{"z", "a"} {
		writeManifest(t, root, name+"/pyproject.toml", "[project]\nname = \""+name+"\"\n")
		d, err := g.Detect(gen.Member{Name: name, WorktreePath: filepath.Join(root, name)}, gen.DetectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, *d)
	}
	space := gen.Space{Name: "feat"}
	p, err := g.Plan(space, found)
	if err != nil {
		t.Fatal(err)
	}
	found[0], found[1] = found[1], found[0]
	again, err := g.Plan(space, found)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, again) {
		t.Fatal("Plan depends on detection order")
	}
	var parsed map[string]any
	if err := toml.Unmarshal(p.Files[0].Content, &parsed); err != nil {
		t.Fatalf("Plan wrote invalid TOML: %v", err)
	}
	if !reflect.DeepEqual(p.Stale, []string{"uv.lock", ".venv"}) {
		t.Errorf("Stale = %v", p.Stale)
	}
}
