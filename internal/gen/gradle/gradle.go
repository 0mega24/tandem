package gradle

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes a Gradle composite build settings file.
type Generator struct{}

// Config has no settings because Gradle composites need no generator options.
type Config struct{}

type build struct {
	dir      string
	manifest string
	settings bool
	maven    bool
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID:        "gradle",
		Languages: []string{"Java", "Kotlin", "Groovy", "Scala"},
		Manifests: []string{"settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts"},
		DefaultOn: true,
		Release:   1,
		Config:    Config{},
	}
}

// Detect prefers a containing build so that its subprojects are not included a second time.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	files, err := gen.Find(m.WorktreePath, o.Depth, "settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "pom.xml")
	if err != nil {
		return nil, fmt.Errorf("%s: find Gradle manifests: %w", m.Name, err)
	}
	dirs := map[string]build{}
	for _, file := range files {
		dir := path.Dir(file)
		b := dirs[dir]
		b.dir = dir
		switch path.Base(file) {
		case "pom.xml":
			b.maven = true
		case "settings.gradle", "settings.gradle.kts":
			b.settings = true
			b.manifest = file
		default:
			if b.manifest == "" {
				b.manifest = file
			}
		}
		dirs[dir] = b
	}
	names := make([]string, 0, len(dirs))
	for dir, b := range dirs {
		if b.manifest != "" {
			names = append(names, dir)
		}
	}
	slices.Sort(names)
	var builds []build
	for _, dir := range names {
		nested := false
		for parent := path.Dir(dir); dir != "."; parent = path.Dir(parent) {
			if dirs[parent].manifest != "" {
				nested = true
				break
			}
			if parent == "." {
				break
			}
		}
		if !nested {
			builds = append(builds, dirs[dir])
		}
	}
	if len(builds) == 0 {
		return nil, nil
	}
	return &gen.Detection{Member: m, Manifest: builds[0].manifest, Facts: map[string]any{"builds": builds}}, nil
}

// Plan includes Gradle builds only. Gradle recreates the stale directories when it runs.
func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if _, err := gen.DecodeConfig(s.Config, Config{}); err != nil {
		return gen.Plan{}, err
	}
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	var p gen.Plan
	var out strings.Builder
	out.WriteString("// " + gen.Header(s.Name) + "\n")
	out.WriteString("rootProject.name = " + kotlinString("tandem-"+s.Name) + "\n\n")
	for _, d := range found {
		builds, _ := d.Facts["builds"].([]build)
		for _, b := range builds {
			dir := path.Join(d.Member.Name, b.dir)
			out.WriteString("includeBuild(" + kotlinString("./"+dir) + ")\n")
			if !b.settings {
				p.Warnings = append(p.Warnings, dir+" has no Gradle settings file; add settings.gradle or settings.gradle.kts to the repo")
			}
			if b.maven {
				p.Warnings = append(p.Warnings, dir+" has both Gradle and Maven manifests; it is included in both generator plans")
			}
		}
	}
	p.Files = []gen.File{{Path: "settings.gradle.kts", Content: []byte(out.String())}}
	p.Stale = []string{".gradle", "build"}
	return p, nil
}

// Kotlin interpolates dollar signs even in quoted strings.
func kotlinString(s string) string {
	return strings.ReplaceAll(strconv.Quote(s), "$", `\$`)
}
