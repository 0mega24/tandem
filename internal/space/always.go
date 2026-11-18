package space

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/0mega24/tandem/internal/saga"
	"github.com/0mega24/tandem/internal/state"
)

// alwaysLinks returns an entry for each always.repos repo that the command line did not name.
// Such a repo is a symlink to its main clone: no worktree and no branch, and its edits are
// shared by all spaces. A repo on the command line gets a worktree as usual.
func (m *Manager) alwaysLinks(all, named []Repo, root string) ([]state.RepoEntry, error) {
	if len(m.cfg.Always.Repos) == 0 {
		return nil, nil
	}

	repos, err := ResolveAll(all, m.cfg.Always.Repos)
	if err != nil {
		return nil, fmt.Errorf("always.repos: %w", err)
	}

	var out []state.RepoEntry
	for _, r := range repos {
		if slices.Contains(named, r) {
			continue
		}
		out = append(out, state.RepoEntry{
			Name:         r.Name,
			RepoPath:     r.Path,
			WorktreePath: filepath.Join(root, filepath.FromSlash(r.Name)),
			Symlink:      true,
		})
	}
	return out, nil
}

// checkFiles checks that each always.files entry is a file, and that its copy in the space root
// does not take the place of a repo.
func checkFiles(files []string, entries []state.RepoEntry) error {
	var errs []error
	for _, f := range files {
		fi, err := os.Stat(f)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("always.files: %w", err))
		case !fi.Mode().IsRegular():
			errs = append(errs, fmt.Errorf("always.files: %s is not a file", f))
		}

		base := filepath.Base(f)
		for _, e := range entries {
			if topDir(e.Name) == base {
				errs = append(errs, fmt.Errorf("always.files: %s has the name of repo %s", f, e.Name))
			}
		}
	}
	return errors.Join(errs...)
}

// topDir is the first part of a short name: the entry in the space root that holds the repo.
func topDir(name string) string {
	for i := range len(name) {
		if name[i] == '/' {
			return name[:i]
		}
	}
	return name
}

// secretsOf returns the always.secrets that the main clone has. A missing one is skipped: not
// every repo has every secret. A directory is an error, because only files are copied.
func secretsOf(repo string, secrets []string) ([]string, error) {
	var out []string
	for _, s := range secrets {
		fi, err := os.Stat(filepath.Join(repo, s))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("always.secrets: %w", err)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("always.secrets: %s is a directory in %s", s, repo)
		}
		out = append(out, s)
	}
	return out, nil
}

// secretsStep copies the secrets from the main clone into its new worktree, over a file that
// the branch has. It has no undo: the undo of the worktree step removes the worktree with them.
func secretsStep(p planned) saga.Step {
	e := p.entry
	return saga.Step{
		Name: "secrets " + e.Name,
		Do: func(context.Context) error {
			for _, s := range p.secrets {
				if err := copyFile(filepath.Join(e.RepoPath, s), filepath.Join(e.WorktreePath, s)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func linkStep(root string, e state.RepoEntry) saga.Step {
	return saga.Step{
		Name: "link " + e.Name,
		Do: func(context.Context) error {
			if err := os.MkdirAll(filepath.Dir(e.WorktreePath), 0o755); err != nil { //nolint:gosec // like the space root
				return err
			}
			return os.Symlink(e.RepoPath, e.WorktreePath)
		},
		Undo: func(context.Context) error {
			if err := os.Remove(e.WorktreePath); err != nil {
				return err
			}
			removeEmptyParents(filepath.Dir(e.WorktreePath), root)
			return nil
		},
	}
}

func fileStep(root, src string) saga.Step {
	dst := filepath.Join(root, filepath.Base(src))
	return saga.Step{
		Name: "copy " + src,
		Do:   func(context.Context) error { return copyFile(src, dst) },
		Undo: func(context.Context) error { return os.Remove(dst) },
	}
}

// copyFile copies src to dst with the permissions of src, so a secret that only its owner may
// read stays that way. It makes the parent directories of dst.
func copyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(src) //nolint:gosec // G304: the path comes from the user's config
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // like the worktree
		return err
	}
	if err := os.WriteFile(dst, b, fi.Mode().Perm()); err != nil { //nolint:gosec // G703: config validation keeps secrets inside the repo; files use the base name
		return err
	}

	// WriteFile keeps the mode of a file that was there.
	return os.Chmod(dst, fi.Mode().Perm())
}
