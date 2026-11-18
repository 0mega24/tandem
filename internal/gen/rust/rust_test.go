package rust

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestWorkspace(t *testing.T) {
	root := t.TempDir()
	member := gen.Member{Name: "api", WorktreePath: filepath.Join(root, "api")}
	if err := os.Mkdir(member.WorktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(member.WorktreePath, "Cargo.toml"), []byte("[package]\nname = 'api'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := (Generator{}).Detect(member, gen.DetectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("Detect did not find the root crate")
	}
	p, err := (Generator{}).Plan(gen.Space{Name: "feat", Root: root}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != "Cargo.toml" || !strings.Contains(string(p.Files[0].Content), `"./api"`) {
		t.Fatalf("Plan = %+v; want Cargo workspace with ./api", p)
	}
}

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestConfig(t *testing.T) {
	for _, config := range []map[string]any{
		{"mode": "invalid"}, {"mode": 1}, {"unknown": true}, {"registries": []string{""}}, {"registries": "company"},
	} {
		if _, err := (Generator{}).Plan(gen.Space{Config: config}, nil); err == nil {
			t.Errorf("Plan(%v) accepted invalid config", config)
		}
	}
	meta := (Generator{}).Meta()
	config, ok := meta.Config.(Config)
	if meta.ID != "rust" || !meta.DefaultOn || meta.Release != 1 || !ok || config.Mode != Auto {
		t.Fatalf("Meta = %+v", meta)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		depth       int
		crates, git int
		workspace   bool
		wantErr     bool
	}{
		{name: "root-only", files: map[string]string{"nested/Cargo.toml": "[package]\nname='nested'"}},
		{name: "depth", depth: 1, crates: 1, files: map[string]string{"nested/Cargo.toml": "[package]\nname='nested'", "nested/deep/Cargo.toml": "[package]\nname='deep'"}},
		{name: "ignored", depth: 2, files: map[string]string{".gitignore": "ignored/\n", "ignored/Cargo.toml": "bad", "target/Cargo.toml": "bad", "node_modules/Cargo.toml": "bad"}},
		{name: "malformed", files: map[string]string{"Cargo.toml": "[package"}, wantErr: true},
		{name: "missing-name", files: map[string]string{"Cargo.toml": "[package]\nversion='1'"}, wantErr: true},
		{name: "missing-member", files: map[string]string{"Cargo.toml": "[workspace]\nmembers=['crates/*']", "crates/missing/README": "no manifest"}, wantErr: true},
		{name: "workspace", depth: 2, crates: 4, workspace: true, files: map[string]string{"Cargo.toml": "[package]\nname='root'\n[workspace]\nmembers=['crates/*']\nexclude=['crates/ignored']", "crates/core/Cargo.toml": "[package]\nname='core'", "crates/ignored/Cargo.toml": "[package]\nname='ignored'", "other/Cargo.toml": "[package]\nname='other'"}},
		{name: "dependency-tables", crates: 1, workspace: true, git: 5, files: map[string]string{"Cargo.toml": `[package]
name='api'
[dependencies]
a={git='https://example.org/a'}
[dev-dependencies]
b={git='https://example.org/b'}
[build-dependencies]
c={git='https://example.org/c'}
[target.'cfg(unix)'.dependencies]
d={git='https://example.org/d'}
[workspace.dependencies]
e={git='https://example.org/e'}
`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for file, content := range tt.files {
				writeFile(t, root, file, content)
			}
			d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: root}, gen.DetectOptions{Depth: tt.depth})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect = %v; want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.crates == 0 && !tt.workspace {
				if d != nil {
					t.Fatalf("Detect = %+v; want nil", d)
				}
				return
			}
			if d == nil {
				t.Fatal("Detect = nil")
			}
			f := d.Facts["cargo"].(facts)
			if len(f.Crates) != tt.crates || len(f.Git) != tt.git || (len(f.Workspaces) > 0) != tt.workspace {
				t.Fatalf("Detect facts = %+v; want %d crates, %d git URLs, workspace %v", f, tt.crates, tt.git, tt.workspace)
			}
		})
	}
}

func writeFile(t *testing.T, root, file, content string) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "Cargo.toml", "[package]\nname='outside'\n")
	if err := os.Symlink(filepath.Join(outside, "Cargo.toml"), filepath.Join(root, "Cargo.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Generator{}).Detect(gen.Member{WorktreePath: root}, gen.DetectOptions{}); err == nil {
		t.Fatal("Detect followed a manifest outside the worktree")
	}
}

func TestPlanPure(t *testing.T) {
	root := t.TempDir()
	var found []gen.Detection
	for _, name := range []string{"zeta", "alpha"} {
		dir := filepath.Join(root, name)
		writeFile(t, dir, "Cargo.toml", "[package]\nname='"+name+"'\n")
		d, err := (Generator{}).Detect(gen.Member{Name: name, WorktreePath: dir}, gen.DetectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, *d)
	}
	for _, mode := range []string{"workspace", "patch"} {
		space := gen.Space{Name: "feat", Root: root, Config: map[string]any{"mode": mode}}
		p, err := (Generator{}).Plan(space, found)
		if err != nil {
			t.Fatal(err)
		}
		found[0], found[1] = found[1], found[0]
		again, err := (Generator{}).Plan(space, found)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p, again) {
			t.Fatal("Plan depends on detection order")
		}
	}
	// Files can disappear after Detect; Plan must still use only the captured facts.
	for _, d := range found {
		if err := os.Remove(filepath.Join(d.Member.WorktreePath, "Cargo.toml")); err != nil {
			t.Fatal(err)
		}
	}
	p, err := (Generator{}).Plan(gen.Space{Name: "feat", Root: root}, found)
	if err != nil || len(p.Files) != 1 {
		t.Fatalf("Plan after manifest removal = %+v, %v", p, err)
	}
	if found[0].Member.Name != "zeta" {
		t.Fatal("Plan changed the caller's detection order")
	}
}

func TestPlanMemberCollision(t *testing.T) {
	root := t.TempDir()
	member := gen.Member{Name: "api", WorktreePath: filepath.Join(root, "api")}
	writeFile(t, member.WorktreePath, "Cargo.toml", "[package]\nname='api'\n")
	d, err := (Generator{}).Detect(member, gen.DetectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ dir, mode string }{{".cargo", "patch"}, {"target", "workspace"}} {
		space := gen.Space{Name: "feat", Root: root, Config: map[string]any{"mode": tt.mode}, Members: []gen.Member{member, {Name: tt.dir, WorktreePath: filepath.Join(root, tt.dir)}}}
		if _, err := (Generator{}).Plan(space, []gen.Detection{*d}); err == nil {
			t.Errorf("Plan accepted a generated or stale path inside member %s", tt.dir)
		}
	}
}
