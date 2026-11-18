package ts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestDetect(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files map[string]string
		depth int
		want  []string
		bad   bool
	}{
		{name: "composite", files: map[string]string{"tsconfig.json": `{"compilerOptions":{"composite":true}}`}, want: []string{"tsconfig.json"}},
		{name: "non-composite", files: map[string]string{"tsconfig.json": `{"compilerOptions":{"composite":false}}`}},
		{name: "missing-composite", files: map[string]string{"tsconfig.json": `{}`}},
		{name: "root-only", files: map[string]string{"pkg/tsconfig.json": `{"compilerOptions":{"composite":true}}`}},
		{name: "depth", depth: 1, files: map[string]string{"pkg/tsconfig.json": `{"compilerOptions":{"composite":true}}`, "pkg/deep/tsconfig.json": `{"compilerOptions":{"composite":true}}`, "node_modules/x/tsconfig.json": `invalid`, ".gitignore": "ignored/\n", "ignored/tsconfig.json": `invalid`}, want: []string{"pkg/tsconfig.json"}},
		{name: "invalid", files: map[string]string{"tsconfig.json": `{"compilerOptions":`}, bad: true},
		{name: "wrong-type", files: map[string]string{"tsconfig.json": `{"compilerOptions":{"composite":"true"}}`}, bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tt.files {
				file := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			d, err := (Generator{}).Detect(gen.Member{Name: "api", WorktreePath: root}, gen.DetectOptions{Depth: tt.depth})
			if (err != nil) != tt.bad {
				t.Fatalf("Detect error = %v, want error %v", err, tt.bad)
			}
			if tt.bad {
				return
			}
			if tt.want == nil {
				if d != nil {
					t.Fatalf("Detect = %+v, want nil", d)
				}
				return
			}
			if d == nil || !reflect.DeepEqual(d.Facts["configs"], tt.want) {
				t.Fatalf("Detect = %+v, want configs %v", d, tt.want)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	found := []gen.Detection{
		{Member: gen.Member{Name: "z"}, Facts: map[string]any{"configs": []string{"tsconfig.json"}}},
		{Member: gen.Member{Name: "org/a"}, Facts: map[string]any{"configs": []string{"pkg/tsconfig.json", "tsconfig.json"}}},
	}
	p, err := (Generator{}).Plan(gen.Space{}, found)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Files      []string `json:"files"`
		References []struct {
			Path string `json:"path"`
		} `json:"references"`
	}
	if len(p.Files) != 1 {
		t.Fatalf("files = %v, want one", p.Files)
	}
	if err := json.Unmarshal(p.Files[0].Content, &got); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, r := range got.References {
		paths = append(paths, r.Path)
	}
	want := []string{"./org/a/pkg/tsconfig.json", "./org/a/tsconfig.json", "./z/tsconfig.json"}
	if !reflect.DeepEqual(paths, want) || got.Files == nil || len(got.Files) != 0 {
		t.Fatalf("solution = %+v, want files [] and paths %v", got, want)
	}
	if found[0].Member.Name != "z" {
		t.Fatal("Plan changed its input order")
	}
}

func TestInvalidConfig(t *testing.T) {
	if _, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"mode": "unknown"}}, nil); err == nil {
		t.Fatal("Plan accepted an unknown option")
	}
}

func TestDetectSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "tsconfig.json"), []byte(`{"compilerOptions":{"composite":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "tsconfig.json"), filepath.Join(root, "tsconfig.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	d, err := (Generator{}).Detect(gen.Member{Name: "api", WorktreePath: root}, gen.DetectOptions{Depth: 2})
	if err != nil || d != nil {
		t.Fatalf("Detect = %+v, %v; want nil, nil", d, err)
	}
}

func TestMeta(t *testing.T) {
	m := (Generator{}).Meta()
	if m.ID != "ts" || m.DefaultOn || m.Release != 1 || !reflect.DeepEqual(m.Manifests, []string{"tsconfig.json"}) || !reflect.DeepEqual(m.Languages, []string{"TypeScript"}) || m.Config != (Config{}) {
		t.Fatalf("Meta = %+v, want release 1 TypeScript generator, default off, empty config", m)
	}
}
