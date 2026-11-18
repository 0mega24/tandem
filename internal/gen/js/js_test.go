package js

import (
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestPlanOneMember(t *testing.T) {
	g := Generator{}
	d, err := g.Detect(gen.Member{Name: "api", WorktreePath: "testdata/one-member/in/api"}, gen.DetectOptions{})
	if err != nil || d == nil {
		t.Fatalf("Detect = %v, %v", d, err)
	}
	p, err := g.Plan(gen.Space{Name: "feat"}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != "pnpm-workspace.yaml" || !strings.Contains(string(p.Files[0].Content), "linkWorkspacePackages: true") {
		t.Fatalf("Plan = %+v", p)
	}
}

func TestDetectDepth(t *testing.T) {
	g := Generator{}
	m := gen.Member{Name: "shared", WorktreePath: "testdata/json-workspace/in/shared"}
	d, err := g.Detect(m, gen.DetectOptions{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	p, err := g.Plan(gen.Space{Name: "feat"}, []gen.Detection{*d})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(p.Files[0].Content), "./shared/packages/lib") {
		t.Fatalf("declared member listed twice: %s", p.Files[0].Content)
	}
	m.WorktreePath = "testdata/json-workspace/in"
	d, err = g.Detect(m, gen.DetectOptions{})
	if err != nil || d != nil {
		t.Fatalf("root-only Detect = %v, %v", d, err)
	}
	d, err = g.Detect(m, gen.DetectOptions{Depth: 1})
	if err != nil || d == nil || d.Manifest != "shared/package.json" {
		t.Fatalf("depth Detect = %v, %v", d, err)
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, cfg := range []map[string]any{{"manager": "unknown"}, {"unknown": true}, {"link_workspace_packages": "yes"}} {
		if _, err := (Generator{}).Plan(gen.Space{Config: cfg}, nil); err == nil {
			t.Errorf("Plan config %v succeeded", cfg)
		}
	}
}

func TestPlanOrder(t *testing.T) {
	g := Generator{}
	var found []gen.Detection
	for _, name := range []string{"lib", "api"} {
		d, err := g.Detect(gen.Member{Name: name, WorktreePath: "testdata/depends/in/" + name}, gen.DetectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, *d)
	}
	p, err := g.Plan(gen.Space{Name: "feat"}, found)
	if err != nil {
		t.Fatal(err)
	}
	content := string(p.Files[0].Content)
	if strings.Index(content, "./api") > strings.Index(content, "./lib") {
		t.Fatalf("unsorted workspaces: %s", content)
	}
	if found[0].Member.Name != "lib" {
		t.Fatal("Plan mutated detections")
	}
}

func TestDetectRootFirst(t *testing.T) {
	g := Generator{}
	d, err := g.Detect(gen.Member{Name: "shared", WorktreePath: "testdata/depth-workspace/in/shared"}, gen.DetectOptions{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if d.Manifest != "package.json" {
		t.Fatalf("Detect manifest = %q, want package.json", d.Manifest)
	}
	packages := d.Facts["packages"].([]packageFacts)
	if len(packages) != 1 {
		t.Fatalf("Detect = %+v, want root workspace only", packages)
	}
}
