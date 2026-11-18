package gen

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// skipDirs are never scanned: VCS data, dependency and build output (generators spec, Detection
// rule 3).
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "target": true, "build": true, "bin": true, "obj": true,
	"dist": true, "vendor": true, ".venv": true,
}

// Find returns the files under root whose base name matches one of the patterns (path.Match),
// down to depth directory levels below root (0 is root only). It skips skipDirs, symlinks, and the
// directories that the root .gitignore names. The paths are relative to root, with "/", sorted.
//
// The .gitignore support is simple: one pattern per line, matched against the base name, or
// against the path when the pattern has a "/" inside; negations ("!") are ignored.
func Find(root string, depth int, patterns ...string) ([]string, error) {
	ignored, err := gitignore(root)
	if err != nil {
		return nil, err
	}

	var out []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if skipDirs[d.Name()] || ignored(rel) || strings.Count(rel, "/") >= depth {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink == 0 && matchAny(patterns, d.Name()) {
			out = append(out, rel)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

// Expand returns the directories under root that match the member globs of a manifest, such as
// Cargo [workspace].members or pnpm-workspace.yaml packages. "**" matches any number of
// directories, and a pattern that starts with "!" removes the matches before it. The paths are
// relative to root, with "/", sorted, without duplicates; skipDirs and symlinks are never
// matched.
func Expand(root string, patterns []string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || p == root {
			return nil
		}
		if skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, p)
		dirs = append(dirs, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		return nil, err
	}

	keep := map[string]bool{}
	for _, pat := range patterns {
		neg := strings.HasPrefix(pat, "!")
		pat = strings.TrimSuffix(strings.TrimPrefix(path.Clean(strings.TrimPrefix(pat, "!")), "./"), "/")
		for _, d := range dirs {
			if globMatch(strings.Split(pat, "/"), strings.Split(d, "/")) {
				keep[d] = !neg
			}
		}
	}

	var out []string
	for d, ok := range keep {
		if ok {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return out, nil
}

// globMatch matches path segments against pattern segments, where "**" is any number of them.
func globMatch(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globMatch(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], segs[0])
	return err == nil && ok && globMatch(pat[1:], segs[1:])
}

func matchAny(patterns []string, name string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool {
		ok, err := path.Match(p, name)
		return err == nil && ok
	})
}

// gitignore returns a test for the directory paths that the root .gitignore names.
func gitignore(root string) (func(rel string) bool, error) {
	f, err := os.Open(filepath.Join(root, ".gitignore")) //nolint:gosec // G304: a file in the worktree that Detect reads
	if errors.Is(err, fs.ErrNotExist) {
		return func(string) bool { return false }, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read only

	var pats []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		pats = append(pats, strings.Trim(line, "/"))
	}
	if err := s.Err(); err != nil {
		return nil, err
	}

	return func(rel string) bool {
		base := path.Base(rel)
		for _, p := range pats {
			target := base
			if strings.Contains(p, "/") {
				target = rel
			}
			if ok, err := path.Match(p, target); err == nil && ok {
				return true
			}
		}
		return false
	}, nil
}
