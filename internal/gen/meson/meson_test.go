package meson

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) {
	gentest.Run(t, Generator{})
}

func TestDetectDepth(t *testing.T) {
	g := Generator{}
	root := filepath.Join("testdata", "nested", "in", "app")
	m := gen.Member{Name: "app", WorktreePath: root}
	for _, tt := range []struct {
		depth int
		want  int
	}{{0, 1}, {1, 2}} {
		d, err := g.Detect(m, gen.DetectOptions{Depth: tt.depth})
		if err != nil {
			t.Fatal(err)
		}
		if d.Manifest != "meson.build" {
			t.Errorf("Detect(depth=%d) manifest = %q, want root meson.build", tt.depth, d.Manifest)
		}
		projects := d.Facts["projects"].([]project)
		if len(projects) != tt.want {
			t.Errorf("Detect(depth=%d) found %d projects, want %d", tt.depth, len(projects), tt.want)
		}
	}
}

func TestParseWrap(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       wrap
		wantError  bool
	}{
		{"default", "# comment\n[wrap-git]\n url = https://host/foo.git\nrevision = head\n[provide]\nfoo = foo_dep\n", wrap{"foo", "https://host/foo.git"}, false},
		{"directory", "[wrap-git]\nurl = git@host:foo\ndirectory = local-foo\n", wrap{"local-foo", "git@host:foo"}, false},
		{"file", "[wrap-file]\nurl = https://host/archive\n", wrap{"foo", ""}, false},
		{"malformed", "[wrap-git]\nurl\n", wrap{}, true},
		{"escape", "[wrap-git]\nurl = https://host/foo\ndirectory = ../foo\n", wrap{}, true},
		{"absolute", "[wrap-git]\nurl = https://host/foo\ndirectory = /foo\n", wrap{}, true},
		{"backslash", "[wrap-git]\nurl = https://host/foo\ndirectory = foo\\bar\n", wrap{}, true},
		{"empty", "[wrap-git]\nurl = https://host/foo\ndirectory =\n", wrap{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWrap([]byte(tt.text), "foo")
			if got != tt.want || (err != nil) != tt.wantError {
				t.Errorf("parseWrap() = %#v, %v; want %#v, error %v", got, err, tt.want, tt.wantError)
			}
		})
	}
}

func TestUnknownConfig(t *testing.T) {
	_, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"mode": "auto"}}, nil)
	if err == nil {
		t.Fatal("Plan accepted an unknown config key")
	}
}

func TestPlanOrder(t *testing.T) {
	g := Generator{}
	root, err := filepath.Abs(filepath.Join("testdata", "transitive", "in"))
	if err != nil {
		t.Fatal(err)
	}
	s := gen.Space{Root: root}
	var found []gen.Detection
	for _, name := range []string{"foo", "bar", "app"} {
		m := gen.Member{Name: name, WorktreePath: filepath.Join(root, name), OriginURL: "https://example.org/" + name}
		s.Members = append(s.Members, m)
		d, err := g.Detect(m, gen.DetectOptions{})
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, *d)
	}
	p, err := g.Plan(s, found)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(found)
	slices.Reverse(s.Members)
	again, err := g.Plan(s, found)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Links) != 3 || !reflect.DeepEqual(p, again) {
		t.Fatalf("Plan order changed: %#v, %#v", p, again)
	}
}

func TestDetectIgnoredSubprojects(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subprojects"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"meson.build":          "project('app')\n",
		".gitignore":           "subprojects/\n",
		"subprojects/foo.wrap": "[wrap-git]\nurl = https://example.org/foo.git\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := (Generator{}).Detect(gen.Member{Name: "app", WorktreePath: root}, gen.DetectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	projects := d.Facts["projects"].([]project)
	if len(projects) != 1 || len(projects[0].wraps) != 1 {
		t.Fatalf("Detect ignored explicit wrap metadata: %#v", projects)
	}
}
