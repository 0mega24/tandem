package cabal

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes cabal.project.
type Generator struct{}

// Config is empty because Cabal has no generator-specific settings.
type Config struct{}

type facts struct {
	Packages []string
	Optional []string
	Projects []string
	Stack    bool
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{ID: "cabal", Languages: []string{"Haskell"}, Manifests: []string{"*.cabal", "cabal.project"}, DefaultOn: true, Release: 2, Config: Config{}}
}

// Detect reads projects before loose package manifests so a project's package list controls
// which packages belong to it. OpenRoot confines reads through symlinks to the worktree.
func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	root, err := os.OpenRoot(m.WorktreePath)
	if err != nil {
		return nil, err
	}
	defer root.Close() //nolint:errcheck // read-only directory
	files, err := gen.Find(m.WorktreePath, max(0, o.Depth), "cabal.project", "*.cabal")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	var f facts
	covered := map[string]bool{}
	first := ""
	for _, file := range files {
		if path.Base(file) != "cabal.project" {
			continue
		}
		b, err := root.ReadFile(filepath.FromSlash(file))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		packages, optional, err := projectLocations(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, file, err)
		}
		dir := path.Dir(file)
		packages, err = expandLocations(m.WorktreePath, dir, packages)
		if err != nil {
			return nil, err
		}
		optional, err = expandLocations(m.WorktreePath, dir, optional)
		if err != nil {
			return nil, err
		}
		f.Packages = append(f.Packages, packages...)
		f.Optional = append(f.Optional, optional...)
		f.Projects = append(f.Projects, file)
		covered[dir] = true
		if first == "" {
			first = file
		}
	}
	for _, file := range files {
		if path.Ext(file) != ".cabal" || covered[path.Dir(file)] {
			continue
		}
		// Read through OpenRoot too: a file must be readable within the worktree.
		if _, err := root.ReadFile(filepath.FromSlash(file)); err != nil {
			return nil, err
		}
		f.Packages = append(f.Packages, path.Dir(file))
		if first == "" {
			first = file
		}
	}
	_, err = root.Stat("stack.yaml")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	f.Stack = err == nil
	slices.Sort(f.Packages)
	f.Packages = slices.Compact(f.Packages)
	slices.Sort(f.Optional)
	f.Optional = slices.Compact(f.Optional)
	return &gen.Detection{Member: m, Manifest: first, Facts: map[string]any{"cabal": f}}, nil
}

// expandLocations expands declared directory globs. Cabal file and archive globs remain
// locations for Cabal to resolve; gen.Expand matches directories only.
func expandLocations(root, dir string, locations []string) ([]string, error) {
	var out []string
	for _, location := range locations {
		if strings.Contains(location, "://") || path.IsAbs(location) {
			return nil, fmt.Errorf("package location %q must be relative to the worktree", location)
		}
		location = path.Join(dir, location)
		if location == ".." || strings.HasPrefix(location, "../") {
			return nil, fmt.Errorf("package location %q leaves the worktree", location)
		}
		if strings.ContainsAny(location, "*?[") && !strings.HasSuffix(location, ".cabal") && !strings.HasSuffix(location, ".tar.gz") {
			dirs, err := gen.Expand(root, []string{location})
			if err != nil {
				return nil, err
			}
			if len(dirs) > 0 {
				out = append(out, dirs...)
				continue
			}
		}
		out = append(out, location)
	}
	return out, nil
}

// projectLocations reads only top-level package fields. Other project settings belong to the
// member and are not copied to the space project.
func projectLocations(text string) ([]string, []string, error) {
	var packages, optional []string
	field := ""
	scan := bufio.NewScanner(strings.NewReader(text))
	for scan.Scan() {
		raw := scan.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if len(raw) == len(strings.TrimLeftFunc(raw, unicode.IsSpace)) {
			key, value, ok := strings.Cut(line, ":")
			field = ""
			if !ok || (key != "packages" && key != "optional-packages") {
				continue
			}
			field, line = key, value
		}
		if field == "" {
			continue
		}
		locations, err := locationTokens(line)
		if err != nil {
			return nil, nil, err
		}
		if field == "packages" {
			packages = append(packages, locations...)
		} else {
			optional = append(optional, locations...)
		}
	}
	return packages, optional, scan.Err()
}

// locationTokens keeps commas inside brace globs and spaces inside quoted paths.
func locationTokens(line string) ([]string, error) {
	var tokens []string
	start, braces := -1, 0
	quoted, escaped := false, false
	for i, r := range line {
		if !quoted && r == '-' && strings.HasPrefix(line[i:], "--") && (i == 0 || unicode.IsSpace(rune(line[i-1]))) {
			line = line[:i]
			break
		}
		if !quoted && braces == 0 && (unicode.IsSpace(r) || r == ',') {
			if start >= 0 {
				tokens = append(tokens, line[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
		}
		if !quoted && r == '{' {
			braces++
		}
		if !quoted && r == '}' {
			braces--
		}
	}
	if quoted || braces != 0 {
		return nil, fmt.Errorf("invalid package location list %q", line)
	}
	if start >= 0 {
		tokens = append(tokens, line[start:])
	}
	for i, token := range tokens {
		if strings.HasPrefix(token, "\"") {
			value, err := strconv.Unquote(token)
			if err != nil {
				return nil, fmt.Errorf("package location: %w", err)
			}
			tokens[i] = value
		}
	}
	return tokens, nil
}

// Plan uses only detection facts and sorts a copy so callers can reuse their input.
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
	var packages, optional []string
	for _, d := range found {
		f, ok := d.Facts["cabal"].(facts)
		if !ok {
			return gen.Plan{}, fmt.Errorf("%s: missing Cabal detection facts", d.Member.Name)
		}
		for _, location := range f.Packages {
			packages = append(packages, "./"+path.Join(d.Member.Name, location))
		}
		for _, location := range f.Optional {
			optional = append(optional, "./"+path.Join(d.Member.Name, location))
		}
		for _, project := range f.Projects {
			p.Warnings = append(p.Warnings, d.Member.Name+" has its own "+project+"; only its package locations are copied to the space project")
		}
		if f.Stack {
			p.Warnings = append(p.Warnings, d.Member.Name+" has stack.yaml and Cabal manifests; the space uses Cabal")
		}
	}
	if len(packages) == 0 && len(optional) == 0 {
		return gen.Plan{}, nil
	}
	var b strings.Builder
	b.WriteString("-- " + gen.Header(s.Name) + "\n\npackages:\n")
	for _, location := range packages {
		b.WriteString("  " + quoteLocation(location) + "\n")
	}
	if len(optional) > 0 {
		b.WriteString("\noptional-packages:\n")
		for _, location := range optional {
			b.WriteString("  " + quoteLocation(location) + "\n")
		}
	}
	p.Files = []gen.File{{Path: "cabal.project", Content: []byte(b.String())}}
	p.Stale = []string{"dist-newstyle"}
	return p, nil
}

func quoteLocation(location string) string {
	if strings.ContainsAny(location, ", \t\r\n\"\\") {
		return strconv.Quote(location)
	}
	return location
}
