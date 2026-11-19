package rust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/urlnorm"
)

func init() { gen.Register(Generator{}) }

// Generator writes a Cargo workspace or patch configuration.
type Generator struct{}

// Mode selects how Cargo connects the space's crates.
type Mode string

const (
	Auto      Mode = "auto"
	Patch     Mode = "patch"
	Workspace Mode = "workspace"
)

// Config holds the [generate.rust] settings.
type Config struct {
	// Mode chooses auto, patch or workspace. Auto uses a workspace unless a member has one.
	// A member workspace forces patch mode, even when workspace mode is requested.
	Mode Mode `toml:"mode" jsonschema:"enum=auto,enum=patch,enum=workspace,default=auto"`

	// Registries lists extra registries to patch in addition to crates-io.
	Registries []string `toml:"registries" jsonschema:"description=Extra registries to patch besides crates-io"`
}

func defaultConfig() Config { return Config{Mode: Auto, Registries: []string{}} }

func (Generator) Meta() gen.Meta {
	return gen.Meta{ID: "rust", Languages: []string{"Rust"}, Manifests: []string{"Cargo.toml"}, DefaultOn: true, Release: 1, Config: defaultConfig()}
}

type dependencies struct {
	Normal map[string]any `toml:"dependencies"`
	Dev    map[string]any `toml:"dev-dependencies"`
	Build  map[string]any `toml:"build-dependencies"`
}

type manifest struct {
	Package *struct {
		Name string `toml:"name"`
	} `toml:"package"`
	Workspace *struct {
		Members      []string       `toml:"members"`
		Exclude      []string       `toml:"exclude"`
		Dependencies map[string]any `toml:"dependencies"`
	} `toml:"workspace"`
	dependencies
	Target map[string]dependencies `toml:"target"`
}

type (
	crate struct{ Name, Dir string }
	facts struct {
		Crates     []crate
		Git        []string
		Workspaces []string
	}
)

// Detect reads the root manifest, its declared crates, and optional depth matches. OpenRoot
// keeps manifest symlinks from reading files outside the member worktree.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	root, err := os.OpenRoot(m.WorktreePath)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // closing a read-only directory cannot change the detection result
	paths := []string{"Cargo.toml"}
	if o.Depth > 0 {
		extra, err := gen.Find(m.WorktreePath, o.Depth, "Cargo.toml")
		if err != nil {
			return nil, err
		}
		paths = append(paths, extra...)
	}
	seen := map[string]bool{}
	var f facts
	first := ""
	for i := 0; i < len(paths); i++ {
		file := path.Clean(paths[i])
		if seen[file] {
			continue
		}
		seen[file] = true
		b, err := root.ReadFile(filepath.FromSlash(file))
		if errors.Is(err, fs.ErrNotExist) && file == "Cargo.toml" {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		var doc manifest
		if err := toml.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		if first == "" {
			first = file
		}
		dir := path.Dir(file)
		if doc.Package != nil {
			if doc.Package.Name == "" {
				return nil, fmt.Errorf("%s/%s: package name is empty", m.Name, file)
			}
			f.Crates = append(f.Crates, crate{Name: doc.Package.Name, Dir: dir})
		}
		f.Git = append(f.Git, gitURLs(doc.Normal, doc.Dev, doc.Build)...)
		for _, deps := range doc.Target {
			f.Git = append(f.Git, gitURLs(deps.Normal, deps.Dev, deps.Build)...)
		}
		if doc.Workspace == nil {
			continue
		}
		f.Workspaces = append(f.Workspaces, dir)
		f.Git = append(f.Git, gitURLs(doc.Workspace.Dependencies)...)
		patterns := slices.Clone(doc.Workspace.Members)
		for _, exclude := range doc.Workspace.Exclude {
			patterns = append(patterns, "!"+exclude)
		}
		dirs, err := gen.Expand(filepath.Join(m.WorktreePath, filepath.FromSlash(dir)), patterns)
		if err != nil {
			return nil, fmt.Errorf("%s/%s members: %w", m.Name, file, err)
		}
		for _, child := range dirs {
			paths = append(paths, path.Join(dir, child, "Cargo.toml"))
		}
	}
	if first == "" {
		return nil, nil
	}
	slices.Sort(f.Git)
	f.Git = slices.Compact(f.Git)
	return &gen.Detection{Member: m, Manifest: first, Facts: map[string]any{"cargo": f}}, nil
}

func gitURLs(tables ...map[string]any) []string {
	var urls []string
	for _, table := range tables {
		for _, value := range table {
			dep, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if git, ok := dep["git"].(string); ok && git != "" {
				urls = append(urls, git)
			}
		}
	}
	return urls
}

// Plan chooses patch mode for existing workspaces so Cargo never sees nested workspace roots.
func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	cfg, err := gen.DecodeConfig(s.Config, defaultConfig())
	if err != nil {
		return gen.Plan{}, err
	}
	if cfg.Mode != Auto && cfg.Mode != Patch && cfg.Mode != Workspace {
		return gen.Plan{}, fmt.Errorf("invalid rust mode %q", cfg.Mode)
	}
	for _, registry := range cfg.Registries {
		if strings.TrimSpace(registry) == "" {
			return gen.Plan{}, errors.New("rust registry must not be empty")
		}
	}
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	var p gen.Plan
	hasWorkspace := false
	for _, d := range found {
		f, ok := d.Facts["cargo"].(facts)
		if !ok {
			return gen.Plan{}, fmt.Errorf("%s: missing Cargo detection facts", d.Member.Name)
		}
		for _, dir := range f.Workspaces {
			hasWorkspace = true
			p.Warnings = append(p.Warnings, d.Member.Name+"/"+path.Join(dir, "Cargo.toml")+" has a workspace; using patch mode")
		}
	}
	mode := cfg.Mode
	if hasWorkspace {
		mode = Patch
	} else if mode == Auto {
		mode = Workspace
	}
	if cfg.Mode == Workspace && hasWorkspace {
		p.Warnings = append(p.Warnings, "rust workspace mode requested, but nested workspaces require patch mode")
	}
	if cfg.Mode == Auto && !hasWorkspace {
		p.Warnings = append(p.Warnings, "rust auto selected workspace mode: no member has a workspace")
	}
	if mode == Workspace {
		p, err = workspacePlan(s, found, p)
	} else {
		p, err = patchPlan(s, found, cfg, p)
	}
	if err != nil {
		return gen.Plan{}, err
	}
	paths := slices.Clone(p.Stale)
	for _, file := range p.Files {
		paths = append(paths, file.Path)
	}
	for _, member := range s.Members {
		rel, err := filepath.Rel(s.Root, member.WorktreePath)
		if err != nil {
			return gen.Plan{}, err
		}
		rel = strings.ToLower(filepath.ToSlash(rel))
		for _, file := range paths {
			file = strings.ToLower(file)
			if rel == "." || file == rel || strings.HasPrefix(file, rel+"/") {
				return gen.Plan{}, fmt.Errorf("rust path %q is inside member %s", file, member.Name)
			}
		}
	}
	return p, nil
}

func cratePath(s gen.Space, d gen.Detection, c crate) (string, error) {
	rel, err := filepath.Rel(s.Root, filepath.Join(d.Member.WorktreePath, filepath.FromSlash(c.Dir)))
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s: crate is outside the space or at its root", d.Member.Name)
	}
	return "./" + rel, nil
}

func workspacePlan(s gen.Space, found []gen.Detection, p gen.Plan) (gen.Plan, error) {
	var b strings.Builder
	b.WriteString("# " + gen.Header(s.Name) + "\n\n[workspace]\nresolver = \"2\"\nmembers = [\n")
	count := 0
	for _, d := range found {
		f := d.Facts["cargo"].(facts)
		for _, c := range f.Crates {
			dir, err := cratePath(s, d, c)
			if err != nil {
				return gen.Plan{}, err
			}
			b.WriteString("  " + strconv.Quote(dir) + ",\n")
			count++
		}
	}
	if count == 0 {
		return gen.Plan{}, nil
	}
	b.WriteString("]\n")
	p.Files = []gen.File{{Path: "Cargo.toml", Content: []byte(b.String())}}
	p.Stale = []string{"Cargo.lock", "target"}
	return p, nil
}

func patchPlan(s gen.Space, found []gen.Detection, cfg Config, p gen.Plan) (gen.Plan, error) {
	type gitUse struct{ member, url string }
	gitUses := map[string][]gitUse{}
	for _, d := range found {
		for _, url := range d.Facts["cargo"].(facts).Git {
			origin := urlnorm.Normalize(url)
			gitUses[origin] = append(gitUses[origin], gitUse{member: d.Member.Name, url: url})
		}
	}
	tables := map[string]map[string]string{}
	registries := append([]string{"crates-io"}, cfg.Registries...)
	for _, registry := range registries {
		tables[registry] = map[string]string{}
	}
	for _, d := range found {
		f := d.Facts["cargo"].(facts)
		sources := slices.Clone(registries)
		for _, use := range gitUses[urlnorm.Normalize(d.Member.OriginURL)] {
			// A source member alone cannot usefully patch its own git dependencies.
			if use.member != d.Member.Name {
				sources = append(sources, use.url)
			}
		}
		for _, c := range f.Crates {
			dir, err := cratePath(s, d, c)
			if err != nil {
				return gen.Plan{}, err
			}
			for _, source := range sources {
				if tables[source] == nil {
					tables[source] = map[string]string{}
				}
				if old, ok := tables[source][c.Name]; ok && old != dir {
					return gen.Plan{}, fmt.Errorf("duplicate crate %q in patch source %q", c.Name, source)
				}
				tables[source][c.Name] = dir
			}
		}
	}
	sources := make([]string, 0, len(tables))
	for source, crates := range tables {
		if len(crates) > 0 {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return gen.Plan{}, nil
	}
	slices.Sort(sources)
	var b strings.Builder
	b.WriteString("# " + gen.Header(s.Name) + "\n")
	for _, source := range sources {
		b.WriteString("\n[patch." + strconv.Quote(source) + "]\n")
		names := make([]string, 0, len(tables[source]))
		for name := range tables[source] {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			b.WriteString(strconv.Quote(name) + " = { path = " + strconv.Quote(tables[source][name]) + " }\n")
		}
	}
	p.Files = []gen.File{{Path: ".cargo/config.toml", Content: []byte(b.String())}}
	return p, nil
}
