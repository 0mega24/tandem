package maven

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes a Maven reactor aggregator.
type Generator struct{}

// Config has no options: Maven keeps each member's existing build configuration.
type Config struct{}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID:        "maven",
		Languages: []string{"Java"},
		Manifests: []string{"pom.xml"},
		DefaultOn: true,
		Release:   1,
		Config:    Config{},
	}
}

type pom struct {
	XMLName xml.Name `xml:"project"`
	Modules []string `xml:"modules>module"`
}

func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	manifests, err := gen.Find(m.WorktreePath, o.Depth, "pom.xml")
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return nil, nil
	}

	// A declared module is built by its own aggregator, so do not add it twice to the reactor.
	declared := map[string]bool{}
	modules := map[string][]string{}
	for _, manifest := range manifests {
		b, err := os.ReadFile(filepath.Join(m.WorktreePath, filepath.FromSlash(manifest)))
		if err != nil {
			return nil, err
		}
		var project pom
		if err := xml.Unmarshal(b, &project); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, manifest, err)
		}
		dir := path.Dir(manifest)
		patterns := make([]string, 0, len(project.Modules))
		for _, module := range project.Modules {
			patterns = append(patterns, path.Join(dir, strings.TrimSpace(module)))
		}
		if len(patterns) == 0 {
			continue
		}
		expanded, err := gen.Expand(m.WorktreePath, patterns)
		if err != nil {
			return nil, err
		}
		modules[manifest] = expanded
		for _, module := range expanded {
			declared[path.Join(module, "pom.xml")] = true
		}
	}

	var reactors []string
	for _, manifest := range manifests {
		if !declared[manifest] {
			reactors = append(reactors, manifest)
		}
	}

	gradle, err := gen.Find(m.WorktreePath, o.Depth, "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts")
	if err != nil {
		return nil, err
	}
	manifest := manifests[0]
	if slices.Contains(manifests, "pom.xml") {
		manifest = "pom.xml"
	}
	return &gen.Detection{
		Member: m, Manifest: manifest,
		Facts: map[string]any{"reactors": reactors, "modules": modules, "gradle": len(gradle) > 0},
	}, nil
}

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
	var modules []string
	for _, d := range found {
		reactors, _ := d.Facts["reactors"].([]string)
		reactors = slices.Clone(reactors)
		slices.Sort(reactors)
		for _, manifest := range reactors {
			modules = append(modules, "./"+path.Join(d.Member.Name, path.Dir(manifest)))
		}
		if mixed, _ := d.Facts["gradle"].(bool); mixed {
			p.Warnings = append(p.Warnings, d.Member.Name+" has both Maven and Gradle manifests; it is included in both plans")
		}
	}
	if len(modules) == 0 {
		return p, nil
	}

	var b bytes.Buffer
	b.WriteString("<!-- " + gen.Header(s.Name) + " -->\n")
	b.WriteString("<project xmlns=\"http://maven.apache.org/POM/4.0.0\">\n")
	b.WriteString("  <modelVersion>4.0.0</modelVersion>\n  <groupId>tandem.local</groupId>\n  <artifactId>")
	if err := xml.EscapeText(&b, []byte("tandem-"+s.Name)); err != nil {
		return gen.Plan{}, err
	}
	b.WriteString("</artifactId>\n  <version>0</version>\n  <packaging>pom</packaging>\n  <modules>\n")
	for _, module := range modules {
		b.WriteString("    <module>")
		if err := xml.EscapeText(&b, []byte(module)); err != nil {
			return gen.Plan{}, err
		}
		b.WriteString("</module>\n")
	}
	b.WriteString("  </modules>\n</project>\n")
	p.Files = []gen.File{{Path: "pom.xml", Content: b.Bytes()}}
	return p, nil
}
