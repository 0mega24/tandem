package gen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/0mega24/tandem/internal/saga"
)

// Links are the one exception to the rule that a plan writes nothing inside a member: meson needs
// subprojects/<dir> in the consumer worktree. So the rules differ from files (generators spec,
// Meson rule 5 and "Changes"):
//
//   - A link is inside a member, and its target stays inside the space root.
//   - tdm makes a link only where nothing is, and only where git ignores the path in that member,
//     so the link never shows as a change in the repo. --force does not change this.
//   - The owned record is the target. tdm deletes a path only while it is a symlink to that target.
//     A link that tdm did not make is a person's, also with the same target: it is skipped and
//     never owned, so a later run cannot delete it.
//   - A link of a removed worktree goes with the worktree: git worktree remove deletes ignored
//     files and does not follow symlinks (Meson rule 7).
//   - A failed delete is a warning, as for files: the link stays and leaves the state.

// CheckLink enforces the rules for a link path and target. members are as in Prepare.
func CheckLink(root string, l Link, members []string) error {
	if !cleanRel(l.Path) {
		return fmt.Errorf("link %q must be clean, relative and inside the space root", l.Path)
	}
	inMember := false
	for _, m := range members {
		if len(l.Path) > len(m) && strings.EqualFold(l.Path[:len(m)+1], m+"/") {
			inMember = true
		}
	}
	if !inMember {
		return fmt.Errorf("link %q must be inside a member worktree", l.Path)
	}
	if to := path.Join(path.Dir(l.Path), l.Target); l.Target == "" || path.IsAbs(l.Target) || !cleanRel(to) {
		return fmt.Errorf("link %q: target %q must be relative and stay inside the space root", l.Path, l.Target)
	}

	// The link itself may be a symlink; the directories above it must not be.
	cur := root
	parts := strings.Split(l.Path, "/")
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("link %q goes through the symlink %s", l.Path, cur)
		}
	}
	return nil
}

// cleanRel reports whether p is a clean relative path with "/" that stays below the root.
func cleanRel(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !strings.Contains(p, `\`)
}

// decideLink decides about one planned link. It returns the owned record, or nil.
func decideLink(root, id string, l Link, byPath map[string]Owned, ignored func(string) (bool, error)) (Change, *Owned, error) {
	c := Change{Generator: id, Path: l.Path, Target: l.Target}
	own := &Owned{Generator: id, Path: l.Path, Target: l.Target}
	full := filepath.Join(root, filepath.FromSlash(l.Path))

	if fi, err := os.Lstat(filepath.Dir(full)); err != nil || !fi.IsDir() {
		c.Action, c.Reason = Skip, "the member has no "+path.Dir(l.Path)+" directory"
		return c, nil, nil
	}

	prev, isOwned := byPath[l.Path]
	isOwned = isOwned && prev.Target != ""
	fi, err := os.Lstat(full)
	isLink := err == nil && fi.Mode()&fs.ModeSymlink != 0
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return c, nil, err
	case isLink && isOwned && readlink(full) == l.Target && prev.Target == l.Target:
		c.Action = Unchanged
		return c, own, nil
	case isLink && isOwned && readlink(full) == prev.Target:
		c.Action, c.Reason, c.relink = Write, "the target changed from "+prev.Target, prev.Target
		return c, own, nil // still where git ignores it: the check passed when tdm made it
	default:
		c.Action, c.Reason = Skip, "something that tdm did not make is there; delete it so that tdm can link it"
		return c, nil, nil
	}

	ok := false
	if ignored != nil {
		if ok, err = ignored(l.Path); err != nil {
			return c, nil, fmt.Errorf("git check-ignore %s: %w", l.Path, err)
		}
	}
	if !ok {
		c.Action, c.Reason = Skip, "git does not ignore it; add it to the .gitignore of the member so that tdm can link it"
		return c, nil, nil
	}
	c.Action = Write
	return c, own, nil
}

// decideLinkDelete decides about an owned link that no plan wants. A path that is not the link
// any more leaves the state, as it is not tdm's.
func decideLinkDelete(root string, o Owned) Change {
	c := Change{Generator: o.Generator, Path: o.Path, Target: o.Target}
	full := filepath.Join(root, filepath.FromSlash(o.Path))

	fi, err := os.Lstat(full)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c.Action, c.Reason = Unchanged, "already gone"
	case err == nil && fi.Mode()&fs.ModeSymlink != 0 && readlink(full) == o.Target:
		c.Action = Delete
	default:
		c.Action, c.Reason = Skip, "not tdm's link any more, so it is kept"
	}
	return c
}

func readlink(p string) string {
	t, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	return t
}

func linkStep(root string, c Change) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(c.Path))
	return saga.Step{
		Name: "link " + c.Path,
		Do: func(context.Context) error {
			if c.relink != "" {
				if readlink(p) != c.relink {
					return fmt.Errorf("%s changed since the plan", c.Path)
				}
				if err := os.Remove(p); err != nil {
					return err
				}
			}
			if err := os.Symlink(c.Target, p); err != nil {
				if c.relink != "" {
					return errors.Join(err, os.Symlink(c.relink, p)) // the saga undoes only finished steps
				}
				return err
			}
			return nil
		},
		Undo: func(context.Context) error {
			if readlink(p) != c.Target {
				return nil // someone changed it since: not ours to delete
			}
			if err := os.Remove(p); err != nil {
				return err
			}
			if c.relink != "" {
				return os.Symlink(c.relink, p)
			}
			return nil
		},
	}
}

func unlinkStep(root string, c Change, warn func(string, error)) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(c.Path))
	deleted := false
	return saga.Step{
		Name: "unlink " + c.Path,
		Do: func(context.Context) error {
			if readlink(p) != c.Target {
				return nil // changed since Prepare: not ours any more
			}
			if err := os.Remove(p); err != nil {
				warn(c.Path, err)
				return nil
			}
			deleted = true
			return nil
		},
		Undo: func(context.Context) error {
			if !deleted {
				return nil
			}
			return os.Symlink(c.Target, p)
		},
	}
}
