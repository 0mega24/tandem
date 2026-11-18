package gen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/saga"
)

// Owned is a file that tdm wrote, as the space state records it.
type Owned struct {
	Generator string   // generator ID
	Path      string   // relative to the space root, with "/"
	SHA256    string   // hex hash of the bytes that tdm wrote
	Stale     []string // derived files deleted with this one
}

// Action is what Apply does with one file.
type Action int

const (
	Write     Action = iota // new content, or a new file
	Unchanged               // the file has the planned bytes already
	Skip                    // not tdm's file, or a person changed it; Reason says which
	Delete                  // tdm's file that no plan wants any more
)

func (a Action) String() string {
	switch a {
	case Write:
		return "write"
	case Unchanged:
		return "unchanged"
	case Skip:
		return "skip"
	case Delete:
		return "delete"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// Change is the decision for one file.
type Change struct {
	Generator string // generator ID
	Path      string // relative to the space root, with "/"
	Action    Action // what Apply does
	Reason    string // for Skip: why; for a forced Write: where the old file went
	Old       []byte // the bytes on disk; nil when there is no file
	New       []byte // the planned bytes; nil for Delete
	backup    bool   // a forced write keeps the old file in Path + backupSuffix
}

// Applied is the result of Prepare: the decisions, and the state that results from them.
type Applied struct {
	Changes []Change // planned files in generator and plan order, then the deletes
	Owned   []Owned  // the owned files after the changes, sorted by path
	root    string   // the space root
	stale   []string // derived files to delete, relative to root
}

// backupSuffix marks the copy of a file that --force replaced.
const backupSuffix = ".tandem-bak"

// Prepare compares the plans of the generators that ran with the disk and the owned files. It
// only reads. plans has an entry for each generator that ran, also when its plan is empty: the
// owned files of a generator that ran and planned nothing are deleted. Owned files of other
// generators stay as they are. members are the short names of the member worktrees, which no
// plan may write into.
//
// The ownership check, for each planned file:
//
//   - No file on disk: write it.
//   - Owned, and the disk has the hash that tdm wrote: write it again (or nothing when the bytes
//     are the same).
//   - Owned, but the hash differs: a person changed it. Skip, unless force.
//   - Not owned: someone else's file. Skip, unless force; force keeps a copy in <path>.tandem-bak.
func Prepare(root string, owned []Owned, plans map[string]Plan, members []string, force bool) (Applied, error) {
	a := Applied{root: root}
	byPath := map[string]Owned{}
	for _, o := range owned {
		byPath[o.Path] = o
	}

	ids := make([]string, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	planned := map[string]bool{}
	for _, id := range ids {
		if len(plans[id].Links) > 0 {
			return Applied{}, fmt.Errorf("generator %s: links are not supported yet", id) // they come with meson
		}
		for _, s := range plans[id].Stale {
			if err := checkPath(root, s, members); err != nil {
				return Applied{}, fmt.Errorf("generator %s: stale %w", id, err)
			}
		}
		for _, f := range plans[id].Files {
			if err := checkPath(root, f.Path, members); err != nil {
				return Applied{}, fmt.Errorf("generator %s: %w", id, err)
			}
			if planned[f.Path] {
				return Applied{}, fmt.Errorf("generator %s: %s is planned twice", id, f.Path)
			}
			planned[f.Path] = true

			c, o, err := decide(root, id, f, byPath, force)
			if err != nil {
				return Applied{}, err
			}
			a.Changes = append(a.Changes, c)
			if o != nil {
				o.Stale = plans[id].Stale
				a.Owned = append(a.Owned, *o)
			}
			if c.Action == Write {
				a.stale = append(a.stale, plans[id].Stale...)
			}
		}
	}

	// The state came from disk and the members may have changed since, so the owned paths go
	// through the same check before a delete can touch them.
	for _, o := range owned {
		for _, p := range append([]string{o.Path}, o.Stale...) {
			if err := checkPath(root, p, members); err != nil {
				return Applied{}, fmt.Errorf("owned by %s: %w", o.Generator, err)
			}
		}
	}

	for _, o := range owned {
		if planned[o.Path] {
			continue
		}
		if _, ran := plans[o.Generator]; !ran {
			a.Owned = append(a.Owned, o) // a generator that did not run keeps its files
			continue
		}

		c, keep, err := decideDelete(root, o, force)
		if err != nil {
			return Applied{}, err
		}
		a.Changes = append(a.Changes, c)
		if keep {
			a.Owned = append(a.Owned, o)
		}
		if c.Action == Delete {
			a.stale = append(a.stale, o.Stale...)
		}
	}

	slices.SortFunc(a.Owned, func(x, y Owned) int { return strings.Compare(x.Path, y.Path) })
	return a, nil
}

// decide applies the ownership check to one planned file. It returns the owned record to keep,
// or nil when the file is not tdm's after the change.
func decide(root, id string, f File, byPath map[string]Owned, force bool) (Change, *Owned, error) {
	c := Change{Generator: id, Path: f.Path, New: f.Content}
	own := &Owned{Generator: id, Path: f.Path, SHA256: hash(f.Content)}

	old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path))) //nolint:gosec // G304: a checked path in the space root
	if errors.Is(err, fs.ErrNotExist) {
		c.Action = Write
		return c, own, nil
	}
	if err != nil {
		return c, nil, fmt.Errorf("read %s: %w", f.Path, err)
	}
	c.Old = old

	prev, isOwned := byPath[f.Path]
	switch {
	case isOwned && prev.SHA256 == hash(old):
		c.Action = Write
		if bytes.Equal(old, f.Content) {
			c.Action = Unchanged
		}
	case force && isOwned:
		c.Action, c.Reason = Write, "changed by a person; replaced (--force)"
	case force:
		c.Action, c.Reason, c.backup = Write, "not written by tdm; the old file is in "+f.Path+backupSuffix, true
	case isOwned:
		c.Action, c.Reason = Skip, "changed by a person; --force replaces it"
		return c, &prev, nil // still tdm's file: a later --force or a person's revert can take it back
	default:
		c.Action, c.Reason = Skip, "not written by tdm; --force replaces it and keeps a copy"
		return c, nil, nil
	}
	return c, own, nil
}

// decideDelete decides about an owned file that no plan wants. It returns whether the file stays
// owned (a person changed it, so it is kept).
func decideDelete(root string, o Owned, force bool) (Change, bool, error) {
	c := Change{Generator: o.Generator, Path: o.Path}

	old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(o.Path))) //nolint:gosec // G304: a path from the state, checked at load
	if errors.Is(err, fs.ErrNotExist) {
		c.Action = Unchanged // already gone; it leaves the state
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("read %s: %w", o.Path, err)
	}
	c.Old = old

	if hash(old) != o.SHA256 && !force {
		c.Action, c.Reason = Skip, "changed by a person, so it is kept; --force deletes it"
		return c, true, nil
	}
	c.Action = Delete
	return c, false, nil
}

// checkPath enforces the rules for a path that tdm writes or deletes: relative to the space root,
// clean, with "/", not inside a member worktree, and not through a symlink on disk. A symlink in
// the root is an always.repos entry: a path through it would change the main clone.
//
// The member check ignores case, because macOS and Windows file systems do.
func checkPath(root, p string, members []string) error {
	if p == "" || path.IsAbs(p) || path.Clean(p) != p || strings.HasPrefix(p, "../") || p == ".." || strings.Contains(p, `\`) {
		return fmt.Errorf("path %q must be clean, relative and inside the space root", p)
	}
	for _, m := range members {
		if strings.EqualFold(p, m) || (len(p) > len(m) && strings.EqualFold(p[:len(m)+1], m+"/")) {
			return fmt.Errorf("path %q is inside member %s", p, m)
		}
	}

	cur := root
	for part := range strings.SplitSeq(p, "/") {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil // the rest is new
		}
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("path %q goes through the symlink %s", p, cur)
		}
	}
	return nil
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Steps returns the saga steps that make the changes: writes and deletes, then the stale files.
// Each step's undo puts back the bytes that were there.
func (a Applied) Steps() []saga.Step {
	var steps []saga.Step
	for _, c := range a.Changes {
		switch c.Action {
		case Write:
			steps = append(steps, writeStep(a.root, c))
		case Delete:
			steps = append(steps, deleteStep(a.root, c.Path, c.Old))
		}
	}

	for _, s := range a.stale {
		steps = append(steps, staleStep(a.root, s))
	}
	return steps
}

func writeStep(root string, c Change) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(c.Path))
	var madeDirs string // the highest directory that this step made
	madeBackup := false

	// dropOwn removes what this step made besides the file: the backup and the new directories.
	dropOwn := func() error {
		if madeBackup {
			if err := os.Remove(p + backupSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if madeDirs != "" {
			removeEmptyDirs(filepath.Dir(p), filepath.Dir(madeDirs))
		}
		return nil
	}

	return saga.Step{
		Name: "write " + c.Path,
		Do: func(context.Context) error {
			madeDirs = firstMissing(filepath.Dir(p), root)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // workspace files are for all tools that read the space
				return err
			}
			if c.backup {
				if err := writeAtomic(p+backupSuffix, c.Old); err != nil {
					return errors.Join(err, dropOwn())
				}
				madeBackup = true
			}
			if err := writeAtomic(p, c.New); err != nil {
				return errors.Join(err, dropOwn()) // the saga undoes only finished steps
			}
			return nil
		},
		Undo: func(context.Context) error {
			if c.Old != nil {
				if err := writeAtomic(p, c.Old); err != nil {
					return err
				}
			} else if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return dropOwn()
		},
	}
}

// writeAtomic writes b to p through a temp file in the same directory and a rename, so a
// failure part way leaves the old file whole.
func writeAtomic(p string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename; this only cleans up a failure

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close() // the write error is the one to report
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes 0600; workspace files are read by tools that may run as other users.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // G302: see above
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func deleteStep(root, rel string, old []byte) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(rel))
	return saga.Step{
		Name: "delete " + rel,
		Do: func(context.Context) error {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			removeEmptyDirs(filepath.Dir(p), root)
			return nil
		},
		Undo: func(context.Context) error {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // as before the delete
				return err
			}
			return writeAtomic(p, old)
		},
	}
}

// staleStep deletes a derived file such as go.work.sum, which the tool makes again. A directory
// is not deleted: a stale entry names files only.
func staleStep(root, rel string) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(rel))
	var old []byte
	return saga.Step{
		Name: "delete stale " + rel,
		Do: func(context.Context) error {
			fi, err := os.Lstat(p)
			if errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.Mode().IsRegular()) {
				return nil
			}
			if err != nil {
				return err
			}
			if old, err = os.ReadFile(p); err != nil { //nolint:gosec // G304: a path in the space root
				return err
			}
			return os.Remove(p)
		},
		Undo: func(context.Context) error {
			if old == nil {
				return nil
			}
			return writeAtomic(p, old)
		},
	}
}

// firstMissing returns the highest missing directory from dir up to root (not included), or "".
func firstMissing(dir, root string) string {
	top := ""
	for d := dir; d != root && strings.HasPrefix(d, root+string(filepath.Separator)); d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		}
		top = d
	}
	return top
}

// removeEmptyDirs removes dir and its empty parents up to stop (not included), such as the
// .cargo/ that a generator made. It stops at the first directory that is not empty.
func removeEmptyDirs(dir, stop string) {
	for d := dir; d != stop && strings.HasPrefix(d, stop+string(filepath.Separator)); d = filepath.Dir(d) {
		if os.Remove(d) != nil {
			return
		}
	}
}
