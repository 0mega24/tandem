package js

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Manager selects the root workspace format.
type Manager string

const (
	Auto Manager = "auto"
	PNPM Manager = "pnpm"
	NPM  Manager = "npm"
	Yarn Manager = "yarn"
	Bun  Manager = "bun"
)

// Config contains the generate.js settings.
type Config struct {
	Manager               Manager `toml:"manager" jsonschema:"enum=auto,enum=pnpm,enum=npm,enum=yarn,enum=bun,description=Package manager for the space"`
	LinkWorkspacePackages bool    `toml:"link_workspace_packages" jsonschema:"description=Link local pnpm packages for registry version dependencies"`
}

func defaultConfig() Config { return Config{Manager: Auto, LinkWorkspacePackages: true} }

// Generator plans JavaScript and TypeScript workspace manifests.
type Generator struct{}

func (Generator) Meta() gen.Meta {
	return gen.Meta{ID: "js", Languages: []string{"JavaScript", "TypeScript"}, Manifests: []string{"package.json"}, DefaultOn: true, Release: 1, Config: defaultConfig()}
}

type manifest struct {
	PackageManager string          `json:"packageManager"`
	Workspaces     json.RawMessage `json:"workspaces"`
}

type packageFacts struct {
	Directory  string
	Manager    Manager
	Workspaces []string
	Workspace  bool
	Warnings   []string
}

func validManager(m Manager) bool { return m == PNPM || m == NPM || m == Yarn || m == Bun }

// Detect reads root manifests and declared members; deeper independent packages require Depth.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	manifests, err := gen.Find(m.WorktreePath, o.Depth, "package.json")
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return nil, nil
	}
	// Visit workspace roots before their declared children, even when a child sorts first.
	slices.SortFunc(manifests, func(a, b string) int {
		if diff := strings.Count(a, "/") - strings.Count(b, "/"); diff != 0 {
			return diff
		}
		return strings.Compare(a, b)
	})
	root, err := os.OpenRoot(m.WorktreePath)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // read only
	var packages []packageFacts
	covered := map[string]bool{}
	for _, file := range manifests {
		dir := path.Dir(file)
		if covered[dir] {
			continue
		}
		facts, err := readPackage(root, dir)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		if facts.Workspace {
			dirs, err := gen.Expand(filepath.Join(m.WorktreePath, filepath.FromSlash(dir)), facts.Workspaces)
			if err != nil {
				return nil, err
			}
			for _, child := range dirs {
				rel := path.Join(dir, child)
				// Read declared package manifests now; Plan must not access the filesystem.
				b, err := root.ReadFile(path.Join(rel, "package.json"))
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return nil, err
				}
				var pkg manifest
				if err := json.Unmarshal(b, &pkg); err != nil {
					return nil, fmt.Errorf("%s/%s/package.json: %w", m.Name, rel, err)
				}
				covered[rel] = true
			}
		}
		packages = append(packages, facts)
	}
	return &gen.Detection{Member: m, Manifest: manifests[0], Facts: map[string]any{"packages": packages}}, nil
}

func readPackage(root *os.Root, dir string) (packageFacts, error) {
	facts := packageFacts{Directory: dir}
	b, err := root.ReadFile(path.Join(dir, "package.json"))
	if err != nil {
		return facts, err
	}
	var pkg manifest
	if err := json.Unmarshal(b, &pkg); err != nil {
		return facts, err
	}
	if len(pkg.Workspaces) > 0 && string(pkg.Workspaces) != "null" {
		facts.Workspace = true
		if err := json.Unmarshal(pkg.Workspaces, &facts.Workspaces); err != nil {
			var object struct {
				Packages []string `json:"packages"`
			}
			if err := json.Unmarshal(pkg.Workspaces, &object); err != nil {
				return facts, fmt.Errorf("workspaces: %w", err)
			}
			facts.Workspaces = object.Packages
		}
	}
	b, err = root.ReadFile(path.Join(dir, "pnpm-workspace.yaml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return facts, err
	}
	if err == nil {
		var workspace struct {
			Packages []string `yaml:"packages"`
		}
		if err := yaml.Unmarshal(b, &workspace); err != nil {
			return facts, fmt.Errorf("pnpm-workspace.yaml: %w", err)
		}
		facts.Workspace, facts.Workspaces = true, workspace.Packages
	}
	if pkg.PackageManager != "" {
		manager, _, _ := strings.Cut(pkg.PackageManager, "@")
		facts.Manager = Manager(manager)
		if !validManager(facts.Manager) {
			return facts, fmt.Errorf("unsupported packageManager %q", pkg.PackageManager)
		}
		return facts, nil
	}
	locks := []struct {
		file    string
		manager Manager
	}{
		{"pnpm-lock.yaml", PNPM}, {"yarn.lock", Yarn}, {"bun.lock", Bun}, {"bun.lockb", Bun}, {"package-lock.json", NPM},
	}
	var managers []Manager
	for _, lock := range locks {
		_, err := root.Stat(path.Join(dir, lock.file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return facts, err
		}
		if !slices.Contains(managers, lock.manager) {
			managers = append(managers, lock.manager)
		}
	}
	facts.Manager = NPM
	if len(managers) > 0 {
		facts.Manager = managers[0]
	}
	if len(managers) > 1 {
		facts.Warnings = append(facts.Warnings, fmt.Sprintf("%s has lockfiles for %s; using %s", dir, managerNames(managers), facts.Manager))
	}
	return facts, nil
}

func managerNames(managers []Manager) string {
	names := make([]string, len(managers))
	for i, m := range managers {
		names[i] = string(m)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	cfg, err := gen.DecodeConfig(s.Config, defaultConfig())
	if err != nil {
		return gen.Plan{}, err
	}
	if cfg.Manager != Auto && !validManager(cfg.Manager) {
		return gen.Plan{}, fmt.Errorf("invalid js manager %q", cfg.Manager)
	}
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	var p gen.Plan
	var managers []Manager
	var workspaces []string
	for _, d := range found {
		packages, _ := d.Facts["packages"].([]packageFacts)
		for _, pkg := range packages {
			if !slices.Contains(managers, pkg.Manager) {
				managers = append(managers, pkg.Manager)
			}
			for _, warning := range pkg.Warnings {
				p.Warnings = append(p.Warnings, d.Member.Name+": "+warning)
			}
			base := path.Join(d.Member.Name, pkg.Directory)
			if !pkg.Workspace {
				workspaces = append(workspaces, "./"+base)
				continue
			}
			for _, glob := range pkg.Workspaces {
				negative := strings.HasPrefix(glob, "!")
				glob = strings.TrimPrefix(glob, "!")
				if path.IsAbs(glob) {
					return gen.Plan{}, fmt.Errorf("%s: absolute workspace glob %q", d.Member.Name, glob)
				}
				rel := path.Join(pkg.Directory, glob)
				if rel == ".." || strings.HasPrefix(rel, "../") {
					return gen.Plan{}, fmt.Errorf("%s: workspace glob escapes worktree: %q", d.Member.Name, glob)
				}
				entry := "./" + path.Join(d.Member.Name, rel)
				if negative {
					entry = "!" + entry
				}
				workspaces = append(workspaces, entry)
			}
		}
	}
	manager := cfg.Manager
	if manager == Auto {
		if len(managers) > 1 {
			p.Warnings = append(p.Warnings, "members use different package managers: "+managerNames(managers)+"; set generate.js.manager to select one")
			return p, nil
		}
		if len(managers) == 0 {
			return p, nil
		}
		manager = managers[0]
	}
	// Preserve declared glob order: exclusions remove earlier matches.
	if workspaces == nil {
		workspaces = []string{}
	}
	if manager == PNPM {
		b, err := yaml.Marshal(struct {
			Packages              []string `yaml:"packages"`
			LinkWorkspacePackages bool     `yaml:"linkWorkspacePackages"`
		}{workspaces, cfg.LinkWorkspacePackages})
		if err != nil {
			return gen.Plan{}, err
		}
		p.Files = []gen.File{{Path: "pnpm-workspace.yaml", Content: append([]byte("# "+gen.Header(s.Name)+"\n"), b...)}}
	} else {
		b, err := json.MarshalIndent(struct {
			Name       string   `json:"name"`
			Private    bool     `json:"private"`
			Workspaces []string `json:"workspaces"`
		}{"tandem-" + s.Name, true, workspaces}, "", "  ")
		if err != nil {
			return gen.Plan{}, err
		}
		p.Files = []gen.File{{Path: "package.json", Content: append(b, '\n')}}
	}
	lock := map[Manager]string{PNPM: "pnpm-lock.yaml", NPM: "package-lock.json", Yarn: "yarn.lock", Bun: "bun.lock"}[manager]
	p.Stale = []string{lock, "node_modules"}
	return p, nil
}
