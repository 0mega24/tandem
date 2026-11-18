package space

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Repo is a main clone under discovery.root_dir.
type Repo struct {
	Name string // path relative to the root, with "/", e.g. "myorg/api"
	Path string // absolute
}

// Discover finds the git repos under root, at most maxDepth levels down. It does not look inside
// a repo, so submodules and nested clones are not listed. Hidden directories are skipped.
func Discover(root string, maxDepth int) ([]Repo, error) {
	var repos []Repo
	if err := discover(root, "", maxDepth, &repos); err != nil {
		return nil, err
	}

	slices.SortFunc(repos, func(a, b Repo) int { return strings.Compare(a.Name, b.Name) })
	return repos, nil
}

func discover(dir, rel string, depth int, repos *[]Repo) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if rel == "" && errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("discovery.root_dir %s does not exist", dir)
		}
		return fmt.Errorf("discover repos: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		p := filepath.Join(dir, e.Name())
		name := path.Join(rel, e.Name())

		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			*repos = append(*repos, Repo{Name: name, Path: p})
			continue
		}

		if depth > 1 {
			if err := discover(p, name, depth-1, repos); err != nil {
				return err
			}
		}
	}

	return nil
}

// Resolve finds the repo for a name given by the user: the full short name ("myorg/api"), or
// the last part alone ("api") when only one repo has it.
func Resolve(repos []Repo, arg string) (Repo, error) {
	var leaf []Repo
	for _, r := range repos {
		if r.Name == arg {
			return r, nil
		}
		if path.Base(r.Name) == arg {
			leaf = append(leaf, r)
		}
	}

	switch len(leaf) {
	case 1:
		return leaf[0], nil
	case 0:
		return Repo{}, fmt.Errorf("no repo %q%s", arg, suggest(repos, arg))
	default:
		names := make([]string, len(leaf))
		for i, r := range leaf {
			names[i] = r.Name
		}
		return Repo{}, fmt.Errorf("repo %q is ambiguous: use one of %s", arg, strings.Join(names, ", "))
	}
}

// ResolveAll resolves each name, and reports every bad name at once. A repo named twice is
// returned once.
func ResolveAll(repos []Repo, args []string) ([]Repo, error) {
	var out []Repo
	var errs []error
	for _, a := range args {
		r, err := Resolve(repos, a)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}

	return out, errors.Join(errs...)
}

// suggest lists up to three repos whose name contains arg, for a "did you mean" hint.
func suggest(repos []Repo, arg string) string {
	var hits []string
	for _, r := range repos {
		if strings.Contains(strings.ToLower(r.Name), strings.ToLower(arg)) {
			hits = append(hits, r.Name)
		}
		if len(hits) == 3 {
			break
		}
	}

	if len(hits) == 0 {
		return ""
	}
	return "; did you mean " + strings.Join(hits, ", ") + "?"
}
