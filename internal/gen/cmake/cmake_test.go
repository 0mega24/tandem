package cmake

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestDeclarations(t *testing.T) {
	input := `# FetchContent_Declare(fake GIT_REPOSITORY https://host/fake)
#[=[ CPMAddPackage(NAME fake GIT_REPOSITORY https://host/fake) ]=]
set(text "FetchContent_Declare(fake GIT_REPOSITORY https://host/fake)")
fEtChCoNtEnT_dEcLaRe(Shared
 GIT_REPOSITORY "https://host/shared.git" GIT_TAG main)
CPMAddPackage(GIT_REPOSITORY [=[git@host:org/lib.git]=] NAME "libAlias")
FetchContent_Declare(dynamic GIT_REPOSITORY ${url})
find_package(Ignored REQUIRED)
`
	want := []dependency{{Name: "Shared", URL: "https://host/shared.git"}, {Name: "libAlias", URL: "git@host:org/lib.git"}}
	if got := declarations(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("declarations = %#v, want %#v", got, want)
	}
}

func TestPlan(t *testing.T) {
	dep := gen.Member{Name: "org/lib", WorktreePath: `/space/a "quoted" $lib`, OriginURL: "git@host:org/lib.git"}
	consumer := gen.Member{Name: "api", WorktreePath: "/space/api"}
	found := []gen.Detection{{Member: consumer, Facts: map[string]any{"dependencies": []dependency{{Name: "consumerAlias", URL: "https://host/org/lib/"}}}}, {Member: dep}}
	s := gen.Space{Name: "feat", Members: []gen.Member{dep, consumer}}
	p, err := (Generator{}).Plan(s, found)
	if err != nil {
		t.Fatal(err)
	}
	want := "# " + gen.Header("feat") + "\n\n" + `set(FETCHCONTENT_SOURCE_DIR_CONSUMERALIAS "/space/a \"quoted\" \$lib" CACHE PATH "tandem")` + "\n" + `set(CPM_consumerAlias_SOURCE "/space/a \"quoted\" \$lib" CACHE PATH "tandem")` + "\n"
	if len(p.Files) != 1 || p.Files[0].Path != ".tandem/cmake/overrides.cmake" || string(p.Files[0].Content) != want {
		t.Fatalf("Plan = %#v, want %q", p, want)
	}
	reversed := []gen.Detection{found[1], found[0]}
	again, err := (Generator{}).Plan(s, reversed)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatalf("reordered Plan = %#v, %v", again, err)
	}
}

func TestDetectDepth(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".gitignore":                "ignored/\n",
		"ignored/CMakeLists.txt":    "FetchContent_Declare(ignored GIT_REPOSITORY https://host/ignored)",
		"sub/CMakeLists.txt":        "project(api)",
		"sub/cmake/deps.cmake":      "FetchContent_Declare(lib GIT_REPOSITORY https://host/lib)",
		"build/CMakeLists.txt":      "project(ignored)",
		"sub/cmake/deep/more.cmake": "FetchContent_Declare(deep GIT_REPOSITORY https://host/deep)",
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := gen.Member{Name: "api", WorktreePath: root}
	d, err := (Generator{}).Detect(m, gen.DetectOptions{})
	if err != nil || d != nil {
		t.Fatalf("root-only Detect = %#v, %v", d, err)
	}
	d, err = (Generator{}).Detect(m, gen.DetectOptions{Depth: 1, Config: map[string]any{"scan_depth": 2}})
	if err != nil || d == nil {
		t.Fatalf("depth Detect = %#v, %v", d, err)
	}
	want := []dependency{{Name: "lib", URL: "https://host/lib"}}
	if d.Manifest != "sub/CMakeLists.txt" || !reflect.DeepEqual(d.Facts["dependencies"], want) {
		t.Fatalf("Detect = %#v", d)
	}
}

func TestConfig(t *testing.T) {
	for _, cfg := range []map[string]any{{"scan_depth": -1}, {"scan_depth": "bad"}, {"unknown": 1}} {
		if _, err := (Generator{}).Plan(gen.Space{Config: cfg}, nil); err == nil {
			t.Errorf("Plan config %v: want error", cfg)
		}
	}
}

// Golden paths use a fixed root because CMake requires absolute source paths.
type goldenGenerator struct{ Generator }

func (g goldenGenerator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	p, err := g.Generator.Plan(s, found)
	for i := range p.Files {
		p.Files[i].Content = []byte(strings.ReplaceAll(string(p.Files[i].Content), filepath.ToSlash(s.Root), "/space"))
	}
	return p, err
}
func TestGolden(t *testing.T) { gentest.Run(t, goldenGenerator{}) }
