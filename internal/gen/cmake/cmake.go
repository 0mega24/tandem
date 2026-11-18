package cmake

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/urlnorm"
)

func init() { gen.Register(Generator{}) }

type Generator struct{}

const defaultScanDepth = 3

type Config struct {
	ScanDepth int `toml:"scan_depth" jsonschema:"minimum=0,default=3,description=Directory levels to scan for CMake dependency declarations"`
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{ID: "cmake", Languages: []string{"C", "C++"}, Manifests: []string{"CMakeLists.txt"}, DefaultOn: true, Release: 1, Config: Config{ScanDepth: defaultScanDepth}}
}

func config(raw map[string]any) (Config, error) {
	cfg, err := gen.DecodeConfig(raw, Config{ScanDepth: defaultScanDepth})
	if err != nil {
		return cfg, err
	}
	if cfg.ScanDepth < 0 {
		return cfg, fmt.Errorf("scan_depth must be nonnegative")
	}
	return cfg, nil
}

type dependency struct{ Name, URL string }

func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	cfg, err := config(o.Config)
	if err != nil {
		return nil, err
	}
	manifests, err := gen.Find(m.WorktreePath, o.Depth, "CMakeLists.txt")
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return nil, nil
	}
	files, err := gen.Find(m.WorktreePath, cfg.ScanDepth, "CMakeLists.txt", "*.cmake")
	if err != nil {
		return nil, err
	}
	var deps []dependency
	for _, file := range files {
		b, err := os.ReadFile(filepath.Join(m.WorktreePath, filepath.FromSlash(file)))
		if err != nil {
			return nil, fmt.Errorf("read %s/%s: %w", m.Name, file, err)
		}
		deps = append(deps, declarations(string(b))...)
	}
	mixed, err := gen.Find(m.WorktreePath, 0, "meson.build")
	if err != nil {
		return nil, err
	}
	return &gen.Detection{Member: m, Manifest: manifests[0], Facts: map[string]any{"dependencies": deps, "meson": len(mixed) > 0}}, nil
}

func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if _, err := config(s.Config); err != nil {
		return gen.Plan{}, err
	}
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	members := s.Members
	if len(members) == 0 {
		for _, d := range found {
			members = append(members, d.Member)
		}
	}
	origins := make(map[string][]gen.Member, len(members))
	for _, m := range members {
		if u := urlnorm.Normalize(m.OriginURL); u != "" {
			origins[u] = append(origins[u], m)
		}
	}
	var p gen.Plan
	values := map[string]string{}
	var names []string
	for _, d := range found {
		if mixed, _ := d.Facts["meson"].(bool); mixed {
			p.Warnings = append(p.Warnings, d.Member.Name+" has both CMake and Meson build files; the CMake plan uses CMakeLists.txt")
		}
		deps, _ := d.Facts["dependencies"].([]dependency)
		for _, dep := range deps {
			for _, m := range origins[urlnorm.Normalize(dep.URL)] {
				if m.Name == d.Member.Name {
					continue
				}
				source := filepath.ToSlash(m.WorktreePath)
				if !filepath.IsAbs(m.WorktreePath) {
					return gen.Plan{}, fmt.Errorf("%s: CMake source path must be absolute", m.Name)
				}
				for _, name := range []string{"FETCHCONTENT_SOURCE_DIR_" + strings.ToUpper(dep.Name), "CPM_" + dep.Name + "_SOURCE"} {
					if old, ok := values[name]; ok {
						if old != source {
							return gen.Plan{}, fmt.Errorf("CMake variable %s matches multiple source paths", name)
						}
						continue
					}
					values[name] = source
					names = append(names, name)
				}
			}
		}
	}
	var b strings.Builder
	b.WriteString("# " + gen.Header(s.Name) + "\n\n")
	for _, name := range names {
		fmt.Fprintf(&b, "set(%s %s CACHE PATH \"tandem\")\n", name, quote(values[name]))
	}
	p.Files = []gen.File{{Path: ".tandem/cmake/overrides.cmake", Content: []byte(b.String())}}
	return p, nil
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`).Replace(s) + `"`
}
