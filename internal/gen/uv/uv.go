package uv

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
)

func init() { gen.Register(Generator{}) }

// Generator writes a virtual uv workspace.
type Generator struct{}

// Config has no options: uv always writes a virtual workspace.
type Config struct{}

type project struct {
	Name string
	Path string
}

type manifest struct {
	Project struct {
		Name string `toml:"name"`
	} `toml:"project"`
	Tool struct {
		UV struct {
			Workspace *struct {
				Members []string `toml:"members"`
				Exclude []string `toml:"exclude"`
			} `toml:"workspace"`
		} `toml:"uv"`
	} `toml:"tool"`
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID: "uv", Languages: []string{"Python"}, Manifests: []string{"pyproject.toml"},
		DefaultOn: false, Release: 1, Config: Config{},
	}
}

func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	files, err := gen.Find(m.WorktreePath, o.Depth, "pyproject.toml")
	if err != nil {
		return nil, err
	}
	// Read parents before children so declared workspace lists take precedence.
	slices.SortFunc(files, func(a, b string) int {
		if depth := strings.Count(a, "/") - strings.Count(b, "/"); depth != 0 {
			return depth
		}
		return strings.Compare(a, b)
	})
	var packages []project
	var members, excludes, workspaces []string
	seen := map[string]bool{}
	for _, file := range files {
		if seen[file] {
			continue
		}
		mod, err := readManifest(m.WorktreePath, file)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		dir := path.Dir(file)
		if mod.Project.Name != "" {
			packages = append(packages, project{Name: mod.Project.Name, Path: dir})
			members = append(members, dir)
		}
		ws := mod.Tool.UV.Workspace
		if ws == nil {
			continue
		}
		workspaces = append(workspaces, dir)
		for _, glob := range ws.Members {
			members = append(members, path.Join(dir, glob))
		}
		patterns := slices.Clone(ws.Members)
		for _, glob := range ws.Exclude {
			excludes = append(excludes, path.Join(dir, glob))
			patterns = append(patterns, "!"+glob)
		}
		dirs, err := gen.Expand(filepath.Join(m.WorktreePath, filepath.FromSlash(dir)), patterns)
		if err != nil {
			return nil, err
		}
		for _, child := range dirs {
			file := path.Join(dir, child, "pyproject.toml")
			seen[file] = true
			pkg, err := readManifest(m.WorktreePath, file)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
			}
			if pkg.Project.Name != "" {
				packages = append(packages, project{Name: pkg.Project.Name, Path: path.Join(dir, child)})
			}
		}
		// A workspace's own list is authoritative, even when detect_depth is set.
		if dir == "." {
			break
		}
	}
	if len(packages) == 0 && len(workspaces) == 0 {
		return nil, nil
	}
	return &gen.Detection{
		Member: m, Manifest: files[0],
		Facts: map[string]any{"packages": packages, "members": members, "exclude": excludes, "workspaces": workspaces},
	}, nil
}

func readManifest(root, file string) (manifest, error) {
	var mod manifest
	p := filepath.Join(root, filepath.FromSlash(file))
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return mod, nil
	}
	if err != nil {
		return mod, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return mod, nil
	}
	b, err := os.ReadFile(p) //nolint:gosec // G304: Detect reads a manifest within the worktree
	if err != nil {
		return mod, err
	}
	if err := toml.Unmarshal(b, &mod); err != nil {
		return mod, err
	}
	return mod, nil
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
	var members, excludes, names []string
	matched := map[string]bool{}
	for _, d := range found {
		matched[d.Member.Name] = true
		packages, _ := d.Facts["packages"].([]project)
		paths, _ := d.Facts["members"].([]string)
		for _, pkg := range packages {
			names = append(names, pkg.Name)
		}
		// Package paths also support callers that supply only package facts.
		if len(paths) == 0 {
			for _, pkg := range packages {
				paths = append(paths, pkg.Path)
			}
		}
		for _, dir := range paths {
			members = append(members, "./"+path.Join(d.Member.Name, dir))
		}
		excluded, _ := d.Facts["exclude"].([]string)
		for _, dir := range excluded {
			excludes = append(excludes, "./"+path.Join(d.Member.Name, dir))
		}
		workspaces, _ := d.Facts["workspaces"].([]string)
		for _, dir := range workspaces {
			p.Warnings = append(p.Warnings, path.Join(d.Member.Name, dir)+" has its own uv workspace; its members are included in the space workspace")
		}
	}
	other := slices.Clone(s.Members)
	slices.SortFunc(other, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })
	for _, m := range other {
		if !matched[m.Name] {
			p.Warnings = append(p.Warnings, m.Name+" has no Python project; omitted from the uv workspace (mixed build systems)")
		}
	}
	slices.Sort(members)
	slices.Sort(excludes)
	slices.Sort(names)
	var b strings.Builder
	b.WriteString("# " + gen.Header(s.Name) + "\n\n[tool.uv.workspace]\nmembers = [\n")
	for _, dir := range slices.Compact(members) {
		b.WriteString("  " + strconv.Quote(dir) + ",\n")
	}
	b.WriteString("]\n")
	if len(excludes) > 0 {
		b.WriteString("exclude = [\n")
		for _, dir := range slices.Compact(excludes) {
			b.WriteString("  " + strconv.Quote(dir) + ",\n")
		}
		b.WriteString("]\n")
	}
	if len(names) > 0 {
		b.WriteString("\n[tool.uv.sources]\n")
		for _, name := range slices.Compact(names) {
			b.WriteString(strconv.Quote(name) + " = { workspace = true }\n")
		}
	}
	p.Files = []gen.File{{Path: "pyproject.toml", Content: []byte(b.String())}}
	p.Stale = []string{"uv.lock", ".venv"}
	return p, nil
}
