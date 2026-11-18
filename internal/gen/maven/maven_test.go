package maven

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestPlanAggregator(t *testing.T) {
	g := Generator{}
	p, err := g.Plan(gen.Space{Name: "feat"}, []gen.Detection{
		{Member: gen.Member{Name: "z"}, Facts: map[string]any{"reactors": []string{"pom.xml"}}},
		{Member: gen.Member{Name: "a&b"}, Facts: map[string]any{"reactors": []string{"pom.xml"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != "pom.xml" {
		t.Fatalf("Plan files = %v, want pom.xml", p.Files)
	}
	text := string(p.Files[0].Content)
	if !strings.Contains(text, "<module>./a&amp;b</module>\n    <module>./z</module>") {
		t.Fatalf("Plan did not sort and escape modules:\n%s", text)
	}
	if strings.Contains(text, "<parent>") {
		t.Fatal("aggregator must not set a parent")
	}
}

func TestDetectDeclaredModules(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "pom.xml", `<project><modules><module>packages/*</module></modules></project>`)
	writeFixture(t, root, "packages/lib/pom.xml", `<project/>`)
	d, err := (Generator{}).Detect(gen.Member{Name: "api", WorktreePath: root}, gen.DetectOptions{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("Detect returned no Maven member")
	}
	if d.Manifest != "pom.xml" {
		t.Errorf("Manifest = %q, want root pom.xml", d.Manifest)
	}
	if got := d.Facts["reactors"]; !reflect.DeepEqual(got, []string{"pom.xml"}) {
		t.Errorf("reactors = %v, want [pom.xml]", got)
	}
	if got := d.Facts["modules"]; !reflect.DeepEqual(got, map[string][]string{"pom.xml": {"packages/lib"}}) {
		t.Errorf("modules = %v, want expanded packages/lib", got)
	}
}

func writeFixture(t *testing.T, root, name, text string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectDepth(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "src/pom.xml", `<project/>`)
	writeFixture(t, root, "ignored/pom.xml", `invalid`)
	writeFixture(t, root, "target/pom.xml", `invalid`)
	writeFixture(t, root, "src/deeper/pom.xml", `invalid`)
	writeFixture(t, root, ".gitignore", "ignored/\n")
	if err := os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "src/pom.xml"), filepath.Join(root, "pom.xml")); err != nil {
		t.Fatal(err)
	}
	m := gen.Member{Name: "api", WorktreePath: root}
	d, err := (Generator{}).Detect(m, gen.DetectOptions{})
	if err != nil || d != nil {
		t.Fatalf("root Detect = %v, %v; want no match", d, err)
	}
	d, err = (Generator{}).Detect(m, gen.DetectOptions{Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || !reflect.DeepEqual(d.Facts["reactors"], []string{"src/pom.xml"}) {
		t.Fatalf("depth Detect = %v, want src/pom.xml only", d)
	}
	p, err := (Generator{}).Plan(gen.Space{Name: "feat"}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || !strings.Contains(string(p.Files[0].Content), "<module>./api/src</module>") {
		t.Fatalf("depth Plan = %v, want ./api/src module", p)
	}
}

func TestDetectInvalidPOM(t *testing.T) {
	for _, text := range []string{`<project>`, `<other/>`} {
		t.Run(text, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "pom.xml", text)
			_, err := (Generator{}).Detect(gen.Member{Name: "api", WorktreePath: root}, gen.DetectOptions{})
			if err == nil || !strings.Contains(err.Error(), "api/pom.xml") {
				t.Fatalf("Detect = %v, want contextual XML error", err)
			}
		})
	}
}

func TestPlanInvalidConfig(t *testing.T) {
	_, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"mode": "parent"}}, nil)
	if err == nil {
		t.Fatal("Plan accepted an unknown option")
	}
}

func TestMeta(t *testing.T) {
	meta := (Generator{}).Meta()
	if meta.ID != "maven" || !meta.DefaultOn || meta.Release != 1 || !reflect.DeepEqual(meta.Manifests, []string{"pom.xml"}) {
		t.Fatalf("Meta = %+v, want default-on release-1 Maven generator", meta)
	}
}
