package ts

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes a TypeScript solution with references to composite projects.
type Generator struct{}

// Config has no options: TypeScript project references have one output format.
type Config struct{}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID:        "ts",
		Languages: []string{"TypeScript"},
		Manifests: []string{"tsconfig.json"},
		DefaultOn: false,
		Release:   1,
		Config:    Config{},
	}
}

// Detect reads root configs by default. A positive depth also includes nested projects.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	manifests, err := gen.Find(m.WorktreePath, o.Depth, "tsconfig.json")
	if err != nil {
		return nil, fmt.Errorf("%s: find tsconfig.json: %w", m.Name, err)
	}
	var configs []string
	for _, manifest := range manifests {
		b, err := os.ReadFile(filepath.Join(m.WorktreePath, filepath.FromSlash(manifest)))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, manifest, err)
		}
		var cfg struct {
			CompilerOptions struct {
				Composite bool `json:"composite"`
			} `json:"compilerOptions"`
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, manifest, err)
		}
		if cfg.CompilerOptions.Composite {
			configs = append(configs, manifest)
		}
	}
	if len(configs) == 0 {
		return nil, nil
	}
	manifest := configs[0]
	if slices.Contains(configs, "tsconfig.json") {
		manifest = "tsconfig.json"
	}
	return &gen.Detection{Member: m, Manifest: manifest, Facts: map[string]any{"configs": configs}}, nil
}

// Plan emits JSON without a header, as JSON has no comment syntax.
func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if _, err := gen.DecodeConfig(s.Config, Config{}); err != nil {
		return gen.Plan{}, err
	}
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	type reference struct {
		Path string `json:"path"`
	}
	solution := struct {
		Files      []string    `json:"files"`
		References []reference `json:"references"`
	}{Files: []string{}, References: []reference{}}
	for _, d := range found {
		configs, _ := d.Facts["configs"].([]string)
		configs = slices.Clone(configs)
		slices.Sort(configs)
		for _, manifest := range configs {
			solution.References = append(solution.References, reference{Path: "./" + path.Join(d.Member.Name, manifest)})
		}
	}
	b, err := json.MarshalIndent(solution, "", "  ")
	if err != nil {
		return gen.Plan{}, fmt.Errorf("encode TypeScript solution: %w", err)
	}
	return gen.Plan{Files: []gen.File{{Path: "tsconfig.json", Content: append(b, '\n')}}}, nil
}
