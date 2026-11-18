package gradle

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

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		depth    int
		manifest string
		includes []string
	}{
		{name: "settings-groovy", files: []string{"settings.gradle"}, manifest: "settings.gradle", includes: []string{"./repo"}},
		{name: "settings-kotlin", files: []string{"settings.gradle.kts"}, manifest: "settings.gradle.kts", includes: []string{"./repo"}},
		{name: "build-groovy", files: []string{"build.gradle"}, manifest: "build.gradle", includes: []string{"./repo"}},
		{name: "build-kotlin", files: []string{"build.gradle.kts"}, manifest: "build.gradle.kts", includes: []string{"./repo"}},
		{name: "root-only", files: []string{"app/build.gradle.kts"}},
		{name: "depth", files: []string{"app/settings.gradle.kts", "lib/build.gradle", "deep/sub/build.gradle", "build/build.gradle", "ignored/build.gradle"}, depth: 1, manifest: "app/settings.gradle.kts", includes: []string{"./repo/app", "./repo/lib"}},
		{name: "root-wins", files: []string{"settings.gradle", "app/build.gradle"}, depth: 2, manifest: "settings.gradle", includes: []string{"./repo"}},
		{name: "maven-only", files: []string{"pom.xml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range append(tt.files, ".gitignore") {
				dst := filepath.Join(root, file)
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dst, []byte("ignored/\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			d, err := (Generator{}).Detect(gen.Member{Name: "repo", WorktreePath: root}, gen.DetectOptions{Depth: tt.depth})
			if err != nil {
				t.Fatal(err)
			}
			if tt.manifest == "" {
				if d != nil {
					t.Fatalf("Detect = %+v, want nil", d)
				}
				return
			}
			if d == nil || d.Manifest != tt.manifest {
				t.Fatalf("Detect = %+v, want manifest %s", d, tt.manifest)
			}
			p, err := (Generator{}).Plan(gen.Space{Name: "feat"}, []gen.Detection{*d})
			if err != nil {
				t.Fatal(err)
			}
			for _, include := range tt.includes {
				if !strings.Contains(string(p.Files[0].Content), `includeBuild("`+include+`")`) {
					t.Errorf("missing include %s: %s", include, p.Files[0].Content)
				}
			}
			if got := strings.Count(string(p.Files[0].Content), "includeBuild("); got != len(tt.includes) {
				t.Errorf("include count = %d, want %d", got, len(tt.includes))
			}
		})
	}
}

func TestPlanConfig(t *testing.T) {
	_, err := (Generator{}).Plan(gen.Space{Config: map[string]any{"unknown": true}}, nil)
	if err == nil {
		t.Fatal("Plan accepted an unknown config key")
	}
}

func TestPlanOrderAndEscaping(t *testing.T) {
	found := []gen.Detection{
		{Member: gen.Member{Name: "z$lib"}, Facts: map[string]any{"builds": []build{{dir: ".", settings: true}}}},
		{Member: gen.Member{Name: "a\"lib"}, Facts: map[string]any{"builds": []build{{dir: ".", settings: true}}}},
	}
	s := gen.Space{Name: "feat$branch"}
	p, err := (Generator{}).Plan(s, found)
	if err != nil {
		t.Fatal(err)
	}
	q, err := (Generator{}).Plan(s, []gen.Detection{found[1], found[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, q) {
		t.Fatal("Plan depends on detection order")
	}
	content := string(p.Files[0].Content)
	for _, want := range []string{`rootProject.name = "tandem-feat\$branch"`, `includeBuild("./a\"lib")`, `includeBuild("./z\$lib")`} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %s", want, content)
		}
	}
}
