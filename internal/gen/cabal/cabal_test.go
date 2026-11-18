package cabal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestRootPackage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.cabal"), []byte("name: app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := (Generator{}).Detect(gen.Member{Name: "app", WorktreePath: root}, gen.DetectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("Detect(root *.cabal) = nil; want a package")
	}
	p, err := (Generator{}).Plan(gen.Space{Name: "feat"}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	want := "-- " + gen.Header("feat") + "\n\npackages:\n  ./app\n"
	if len(p.Files) != 1 || string(p.Files[0].Content) != want {
		t.Fatalf("Plan = %#v; want cabal.project containing %q", p, want)
	}
	if !reflect.DeepEqual(p.Stale, []string{"dist-newstyle"}) {
		t.Fatalf("Stale = %v; want dist-newstyle/", p.Stale)
	}
}

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestConfig(t *testing.T) {
	_, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"mode": "unknown"}}, nil)
	if err == nil {
		t.Fatal("Plan(unknown config key) succeeded")
	}
}

func TestMeta(t *testing.T) {
	m := (Generator{}).Meta()
	if m.ID != "cabal" || !m.DefaultOn || m.Release != 2 || !reflect.DeepEqual(m.Manifests, []string{"*.cabal", "cabal.project"}) {
		t.Fatalf("Meta = %#v", m)
	}
}

func TestDepth(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.cabal"), []byte("name: app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := gen.Member{Name: "org/app", WorktreePath: root}
	d, err := (Generator{}).Detect(m, gen.DetectOptions{})
	if err != nil || d != nil {
		t.Fatalf("root Detect = %v, %v; want nil, nil", d, err)
	}
	d, err = (Generator{}).Detect(m, gen.DetectOptions{Depth: 1})
	if err != nil || d == nil {
		t.Fatalf("depth Detect = %v, %v; want a package", d, err)
	}
	p, err := (Generator{}).Plan(gen.Space{}, []gen.Detection{*d})
	if err != nil || len(p.Files) != 1 || !strings.Contains(string(p.Files[0].Content), "./org/app/src\n") {
		t.Fatalf("depth Plan = %#v, %v", p, err)
	}
}

func TestProjectLocations(t *testing.T) {
	tests := []struct {
		name, text         string
		packages, optional []string
		bad                bool
	}{
		{name: "multiline", text: "packages: . , lib/*.cabal -- local\n  tools/{one,two} \"space dir\"\nconstraints: base >= 4\n  ignored\noptional-packages: extras/*\n", packages: []string{".", "lib/*.cabal", "tools/{one,two}", "space dir"}, optional: []string{"extras/*"}},
		{name: "nested setting", text: "package app\n  packages: not-a-project-field\n"},
		{name: "quoted comment", text: "packages: \"a -- b\"\n", packages: []string{"a -- b"}},
		{name: "unterminated quote", text: "packages: \"bad\n", bad: true},
		{name: "unterminated glob", text: "packages: lib/{a,b\n", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, optional, err := projectLocations(tt.text)
			if (err != nil) != tt.bad || !reflect.DeepEqual(packages, tt.packages) || !reflect.DeepEqual(optional, tt.optional) {
				t.Fatalf("projectLocations(%q) = %v, %v, %v; want %v, %v, error %v", tt.text, packages, optional, err, tt.packages, tt.optional, tt.bad)
			}
		})
	}
}

func TestQuotedMember(t *testing.T) {
	d := gen.Detection{Member: gen.Member{Name: "org/a,b"}, Facts: map[string]any{"cabal": facts{Packages: []string{"."}}}}
	p, err := (Generator{}).Plan(gen.Space{}, []gen.Detection{d})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Files[0].Content), "\"./org/a,b\"") {
		t.Fatalf("Plan did not quote a comma in a path: %s", p.Files[0].Content)
	}
}

func TestUnsafeLocations(t *testing.T) {
	for _, location := range []string{"../outside", "/outside", "https://example.com/pkg.tar.gz"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "cabal.project"), []byte("packages: "+location+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := (Generator{}).Detect(gen.Member{Name: "app", WorktreePath: root}, gen.DetectOptions{})
			if err == nil {
				t.Fatalf("Detect(%q) succeeded; want error", location)
			}
		})
	}
}

func TestSymlinkManifest(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.cabal"), []byte("name: secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.cabal"), filepath.Join(root, "secret.cabal")); err != nil {
		t.Fatal(err)
	}
	d, err := (Generator{}).Detect(gen.Member{Name: "app", WorktreePath: root}, gen.DetectOptions{})
	if err != nil || d != nil {
		t.Fatalf("Detect(symlink) = %v, %v; want nil, nil", d, err)
	}
}

func TestPlanOrder(t *testing.T) {
	found := []gen.Detection{
		{Member: gen.Member{Name: "z"}, Facts: map[string]any{"cabal": facts{Packages: []string{"."}}}},
		{Member: gen.Member{Name: "a"}, Facts: map[string]any{"cabal": facts{Packages: []string{"."}}}},
	}
	p, err := (Generator{}).Plan(gen.Space{}, found)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Files[0].Content), "  ./a\n  ./z\n") || found[0].Member.Name != "z" {
		t.Fatalf("Plan = %s; input = %v", p.Files[0].Content, found)
	}
}
