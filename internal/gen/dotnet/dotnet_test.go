package dotnet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) {
	gentest.Run(t, Generator{})
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		depth    int
		projects []string
		manifest string
		bad      bool
	}{
		{name: "scan", files: map[string]string{"src/App/App.csproj": "", "root.fsproj": "", "obj/Skip.csproj": "", "ignored/Skip.csproj": "", ".gitignore": "ignored/\n", "a/b/c/d/e/TooDeep.csproj": ""}, projects: []string{"root.fsproj", "src/App/App.csproj"}, manifest: "root.fsproj"},
		{name: "slnx", files: map[string]string{"Repo.slnx": `<Solution><Folder Name="/src/"><Project Path="src\App.csproj" /></Folder></Solution>`, "Unlisted.csproj": ""}, projects: []string{"src/App.csproj"}, manifest: "Repo.slnx"},
		{name: "sln", files: map[string]string{"Repo.sln": "Project(\"{TYPE}\") = \"App\", \"src\\App.csproj\", \"{ID}\"\nEndProject\nProject(\"{TYPE}\") = \"folder\", \"folder\", \"{ID}\"\nEndProject\n"}, projects: []string{"src/App.csproj"}, manifest: "Repo.sln"},
		{name: "depth", files: map[string]string{"src/Repo.slnx": `<Solution><Project Path="App.vbproj" /></Solution>`, "src/Extra.csproj": ""}, depth: 1, projects: []string{"src/App.vbproj"}, manifest: "src/Repo.slnx"},
		{name: "bad XML", files: map[string]string{"Repo.slnx": `<Solution>`}, bad: true},
		{name: "escape", files: map[string]string{"Repo.slnx": `<Solution><Project Path="../outside.csproj" /></Solution>`}, bad: true},
		{name: "no match", files: map[string]string{"README": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tt.files {
				p := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: root}, gen.DetectOptions{Depth: tt.depth})
			if (err != nil) != tt.bad {
				t.Fatalf("Detect() error = %v, want error %v", err, tt.bad)
			}
			if tt.bad {
				return
			}
			if tt.manifest == "" {
				if d != nil {
					t.Fatalf("Detect() = %v, want nil", d)
				}
				return
			}
			if d == nil || d.Manifest != tt.manifest || !reflect.DeepEqual(d.Facts["projects"], tt.projects) {
				t.Fatalf("Detect() = %#v, want %s and %v", d, tt.manifest, tt.projects)
			}
		})
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, cfg := range []map[string]any{{"format": "other"}, {"unknown": true}} {
		if _, err := (Generator{}).Plan(gen.Space{Config: cfg}, nil); err == nil {
			t.Errorf("Plan(%v) succeeded, want config error", cfg)
		}
	}
}

func TestPlanOrder(t *testing.T) {
	found := []gen.Detection{
		{Member: gen.Member{Name: "z"}, Facts: map[string]any{"projects": []string{"Z.csproj"}}},
		{Member: gen.Member{Name: "a"}, Facts: map[string]any{"projects": []string{"A.csproj"}}},
	}
	s := gen.Space{Name: "feat"}
	p, err := (Generator{}).Plan(s, found)
	if err != nil {
		t.Fatal(err)
	}
	found[0], found[1] = found[1], found[0]
	q, err := (Generator{}).Plan(s, found)
	if err != nil || !reflect.DeepEqual(p, q) {
		t.Fatalf("Plan order changes output: %v", err)
	}
	if len(p.Files) != 1 || !strings.Contains(string(p.Files[0].Content), `Path="./a/A.csproj"`) {
		t.Fatalf("Plan() = %#v, want member-relative project paths", p)
	}
}

func TestLegacySolutionFolder(t *testing.T) {
	p, err := (Generator{}).Plan(gen.Space{Name: "feat", Config: map[string]any{"format": "sln"}}, []gen.Detection{
		{Member: gen.Member{Name: "api"}, Facts: map[string]any{"projects": []string{"Api.csproj"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(p.Files[0].Content)
	if !strings.Contains(content, `Project("{2150E333-8FDC-42A3-9474-1A3956D46DE8}") = "api"`) {
		t.Error("legacy solution is missing the solution folder project type")
	}
	if !strings.Contains(content, "{E52C4E19-5437-59F9-AB92-F2C01A680BE6}") {
		t.Error("legacy solution project GUID does not match UUIDv5(DNS, ./api/Api.csproj)")
	}
}
