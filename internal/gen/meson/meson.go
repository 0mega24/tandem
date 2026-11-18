package meson

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/urlnorm"
)

func init() { gen.Register(Generator{}) }

// Generator plans Meson subproject links.
type Generator struct{}

// Config is empty because Meson has no generator-specific settings.
type Config struct{}

type project struct {
	dir   string
	wraps []wrap
	mixed bool
}

type wrap struct {
	directory string
	url       string
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID: "meson", Languages: []string{"C", "C++"},
		Manifests: []string{"meson.build", "subprojects/*.wrap"},
		DefaultOn: true, Release: 1, Config: Config{},
	}
}

// Detect reads wrap metadata without evaluating Meson build code.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	manifests, err := gen.Find(m.WorktreePath, max(0, o.Depth), "meson.build")
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return nil, nil
	}
	if i := slices.Index(manifests, "meson.build"); i > 0 {
		manifests[0], manifests[i] = manifests[i], manifests[0]
		slices.Sort(manifests[1:])
	}
	var projects []project
	for _, manifest := range manifests {
		dir := path.Dir(manifest)
		root := filepath.Join(m.WorktreePath, filepath.FromSlash(dir))
		files, err := gen.Find(root, 0, "CMakeLists.txt")
		if err != nil {
			return nil, err
		}
		// Wraps are explicit inputs even when the subprojects directory is git-ignored.
		subprojects := filepath.Join(root, "subprojects")
		info, err := os.Lstat(subprojects)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err == nil && info.IsDir() {
			wraps, err := gen.Find(subprojects, 0, "*.wrap")
			if err != nil {
				return nil, err
			}
			for _, file := range wraps {
				files = append(files, "subprojects/"+file)
			}
		}
		p := project{dir: dir}
		for _, file := range files {
			if file == "CMakeLists.txt" {
				p.mixed = true
			}
			if path.Dir(file) != "subprojects" || !strings.HasSuffix(file, ".wrap") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) //nolint:gosec // G304: wrap metadata inside the member worktree
			if err != nil {
				return nil, err
			}
			w, err := parseWrap(b, strings.TrimSuffix(path.Base(file), ".wrap"))
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", m.Name, path.Join(dir, file), err)
			}
			if w.url != "" {
				p.wraps = append(p.wraps, w)
			}
		}
		projects = append(projects, p)
	}
	return &gen.Detection{Member: m, Manifest: manifests[0], Facts: map[string]any{"projects": projects}}, nil
}

func parseWrap(b []byte, fallback string) (wrap, error) {
	w := wrap{directory: fallback}
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "wrap-git" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return wrap{}, fmt.Errorf("invalid wrap-git entry %q", line)
		}
		switch strings.TrimSpace(key) {
		case "url":
			w.url = strings.TrimSpace(value)
		case "directory":
			w.directory = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return wrap{}, err
	}
	if w.url != "" && (w.directory == "" || w.directory == "." || w.directory == ".." || strings.ContainsAny(w.directory, `/\`)) {
		return wrap{}, fmt.Errorf("invalid subproject directory %q", w.directory)
	}
	return w, nil
}

// Plan links every consumer's wraps, including those in dependency members.
func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if _, err := gen.DecodeConfig(s.Config, Config{}); err != nil {
		return gen.Plan{}, err
	}
	var p gen.Plan
	members := slices.Clone(s.Members)
	slices.SortFunc(members, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })
	origins := make(map[string]gen.Member, len(members))
	for _, m := range members {
		if strings.TrimSpace(m.OriginURL) == "" {
			continue
		}
		key := urlnorm.Normalize(m.OriginURL)
		if _, exists := origins[key]; !exists {
			origins[key] = m
		}
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	links := map[string]string{}
	for _, d := range found {
		projects, _ := d.Facts["projects"].([]project)
		for _, project := range projects {
			if project.mixed {
				p.Warnings = append(p.Warnings, d.Member.Name+"/"+path.Join(project.dir, "meson.build")+" also has CMakeLists.txt; Meson subproject links use wrap-git metadata")
			}
			for _, w := range project.wraps {
				dependency, ok := origins[urlnorm.Normalize(w.url)]
				if !ok || dependency.Name == d.Member.Name {
					continue
				}
				linkDir := filepath.Join(d.Member.WorktreePath, filepath.FromSlash(project.dir), "subprojects")
				linkPath, err := filepath.Rel(s.Root, filepath.Join(linkDir, w.directory))
				if err != nil {
					return gen.Plan{}, err
				}
				target, err := filepath.Rel(linkDir, dependency.WorktreePath)
				if err != nil {
					return gen.Plan{}, err
				}
				linkPath, target = filepath.ToSlash(linkPath), filepath.ToSlash(target)
				if previous, exists := links[linkPath]; exists && previous != target {
					return gen.Plan{}, fmt.Errorf("conflicting wrap-git targets for %s", linkPath)
				}
				links[linkPath] = target
			}
		}
	}
	for linkPath, target := range links {
		p.Links = append(p.Links, gen.Link{Path: linkPath, Target: target})
	}
	slices.SortFunc(p.Links, func(a, b gen.Link) int { return strings.Compare(a.Path, b.Path) })
	return p, nil
}
