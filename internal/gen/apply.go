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
	SHA256    string   // hex hash of the bytes that tdm wrote; "" for a link
	Target    string   // for a link: where it points; "" for a file
	Key       string   // for a shared JSON file: the key that tdm owns (see jsonkey.go)
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
	Reason    string // for Skip: why; for a forced Write or Delete: where the old file went
	Old       []byte // the bytes on disk; nil when there is no file
	New       []byte // the planned bytes; nil for Delete
	Target    string // for a link: its target, in place of Old and New; "" for a file
	backup    bool   // a forced change keeps the old file in Path + backupSuffix
	relink    string // a link write that replaces tdm's link to this old target
}

// Applied is the result of Prepare: the decisions, and the state that results from them.
type Applied struct {
	Changes  []Change  // planned files in generator and plan order, then the deletes
	Owned    []Owned   // the owned files after the changes, sorted by path
	root     string    // the space root
	stale    []string  // derived files to delete, relative to root
	warnings *[]string // dropped links (Prepare) and failed deletes (when the steps run)
}

// backupSuffix marks the copy of a file that --force replaced.
const backupSuffix = ".tandem-bak"

// Prepare compares the plans of the generators that ran with the disk and the owned files. It
// only reads. plans has an entry for each generator that ran, also when its plan is empty: the
// owned files of a generator that ran and planned nothing are deleted. Owned files of other
// generators stay as they are. members are the worktree paths of the members relative to root,
// with "/" (such as "api" or "org/api"); no plan may write into them.
//
// The ownership check, for each planned file:
//
//   - No file on disk: write it.
//   - Owned, and the disk has the hash that tdm wrote: write it again (or nothing when the bytes
//     are the same).
//   - Owned, the hash differs, but the disk has the planned bytes: nothing to write; the new
//     hash goes in the state.
//   - Owned, but the hash differs: a person changed it. Skip, unless force.
//   - Not owned: someone else's file. Skip, unless force.
//
// Force keeps a copy of the old file in <path>.tandem-bak, also for a forced delete. When that
// copy exists already, the file is skipped: an older copy is never overwritten.
//
// Links follow the rules in link.go. ignored reports whether git ignores a link path (relative to
// root) in its member; with nil, no link is made.
func Prepare(root string, owned []Owned, plans map[string]Plan, members []string, force bool, ignored func(string) (bool, error)) (Applied, error) {
	root = filepath.Clean(root)
	a := Applied{root: root, warnings: new([]string)}
	byPath := map[string]Owned{}
	for _, o := range owned {
		byPath[o.Path] = o
	}

	ids := make([]string, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	// A stale path must not be a generated file: that file has its own ownership check, which a
	// stale delete would skip.
	generated := map[string]bool{}
	for _, id := range ids {
		for _, f := range plans[id].Files {
			generated[f.Path] = true
		}
	}
	for p := range byPath {
		generated[p] = true
	}

	planned := map[string]bool{}
	for _, id := range ids {
		for _, s := range plans[id].Stale {
			if err := CheckPath(root, s, members); err != nil {
				return Applied{}, fmt.Errorf("generator %s: stale %w", id, err)
			}
			if generated[s] {
				return Applied{}, fmt.Errorf("generator %s: stale %s is a generated file", id, s)
			}
		}
		for _, f := range plans[id].Files {
			if err := CheckPath(root, f.Path, members); err != nil {
				return Applied{}, fmt.Errorf("generator %s: %w", id, err)
			}
			if planned[f.Path] {
				return Applied{}, fmt.Errorf("generator %s: %s is planned twice", id, f.Path)
			}
			planned[f.Path] = true

			decideFile := decide
			if f.Key != "" {
				decideFile = decideKey
			}
			c, o, err := decideFile(root, id, f, byPath, force)
			if err != nil {
				return Applied{}, err
			}
			a.Changes = append(a.Changes, c)
			if o != nil {
				if c.Action != Skip {
					o.Stale = plans[id].Stale // a skipped file keeps the stale list it was written with
				}
				a.Owned = append(a.Owned, *o)
			}
			if c.Action == Write {
				a.stale = append(a.stale, plans[id].Stale...)
			}
		}

		for _, l := range plans[id].Links {
			if err := CheckLink(root, l, members); err != nil {
				return Applied{}, fmt.Errorf("generator %s: %w", id, err)
			}
			if planned[l.Path] {
				return Applied{}, fmt.Errorf("generator %s: %s is planned twice", id, l.Path)
			}
			planned[l.Path] = true

			c, o, err := decideLink(root, id, l, byPath, ignored)
			if err != nil {
				return Applied{}, err
			}
			a.Changes = append(a.Changes, c)
			if o != nil {
				a.Owned = append(a.Owned, *o)
			}
		}
	}

	// The state came from disk and the members may have changed since, so the owned paths go
	// through the same check before a delete can touch them.
	dropped := map[string]bool{} // owned links that broke the rules
	for _, o := range owned {
		if o.Target != "" {
			// A member that left or became a symlink is not an error: the link is not tdm's to
			// touch any more, so it leaves the state, as heal does for a file that is gone.
			if err := CheckLink(root, Link{Path: o.Path, Target: o.Target}, members); err != nil {
				*a.warnings = append(*a.warnings, fmt.Sprintf("%s: the link %s leaves the state: %v", o.Generator, o.Path, err))
				dropped[o.Path] = true
			}
			continue
		}
		for _, p := range append([]string{o.Path}, o.Stale...) {
			if err := CheckPath(root, p, members); err != nil {
				return Applied{}, fmt.Errorf("owned by %s: %w", o.Generator, err)
			}
		}
	}

	for _, o := range owned {
		if planned[o.Path] || dropped[o.Path] {
			continue
		}
		if _, ran := plans[o.Generator]; !ran {
			a.Owned = append(a.Owned, o) // a generator that did not run keeps its files
			continue
		}
		if o.Target != "" {
			a.Changes = append(a.Changes, decideLinkDelete(root, o))
			continue
		}

		decideGone := decideDelete
		if o.Key != "" {
			decideGone = decideKeyDelete
		}
		c, keep, err := decideGone(root, o, force)
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
	own := &Owned{Generator: id, Path: f.Path, SHA256: Hash(f.Content)}

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
	if isOwned && (prev.SHA256 == Hash(old) || bytes.Equal(old, f.Content)) {
		c.Action = Write
		if bytes.Equal(old, f.Content) {
			c.Action = Unchanged
		}
		return c, own, nil
	}

	var keep *Owned // a changed file stays tdm's: a later --force or a person's revert takes it back
	who := "not written by tdm"
	if isOwned {
		keep, who = &prev, "changed by a person"
	}
	bak := f.Path + backupSuffix
	switch {
	case !force:
		c.Action, c.Reason = Skip, who+"; --force replaces it and keeps a copy"
		return c, keep, nil
	case exists(filepath.Join(root, filepath.FromSlash(bak))):
		c.Action, c.Reason = Skip, who+", and "+bak+" exists; move it away to replace the file"
		return c, keep, nil
	}
	c.Action, c.Reason, c.backup = Write, who+"; the old file is in "+bak, true
	return c, own, nil
}

// decideDelete decides about an owned file that no plan wants. It returns whether the file stays
// owned (a person changed it, so it is kept).
func decideDelete(root string, o Owned, force bool) (Change, bool, error) {
	c := Change{Generator: o.Generator, Path: o.Path}

	old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(o.Path))) //nolint:gosec // G304: a path from the state, checked at load
	if errors.Is(err, fs.ErrNotExist) {
		c.Action, c.Reason = Unchanged, "already gone" // it leaves the state
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("read %s: %w", o.Path, err)
	}
	c.Old = old

	if Hash(old) == o.SHA256 {
		c.Action = Delete
		return c, false, nil
	}

	bak := o.Path + backupSuffix
	switch {
	case !force:
		c.Action, c.Reason = Skip, "changed by a person, so it is kept; --force deletes it and keeps a copy"
		return c, true, nil
	case exists(filepath.Join(root, filepath.FromSlash(bak))):
		c.Action, c.Reason = Skip, "changed by a person, and "+bak+" exists; move it away to delete the file"
		return c, true, nil
	}
	c.Action, c.Reason, c.backup = Delete, "changed by a person; the old file is in "+bak, true
	return c, false, nil
}

// CheckPath enforces the rules for a path that tdm writes or deletes: relative to the space root,
// clean, with "/", not inside a member worktree, and not through a symlink on disk. A symlink in
// the root is an always.repos entry: a path through it would change the main clone.
//
// The member check ignores case, because macOS and Windows file systems do. members are as in
// Prepare.
func CheckPath(root, p string, members []string) error {
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
			return fmt.Errorf("path %q goes through the symlink %s; tdm does not write through symlinks, so move it away", p, cur)
		}
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Hash is the ownership record of the bytes b: the hex SHA-256 that the state keeps.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Steps returns the saga steps that make the changes: writes and deletes, then the stale files,
// each once. Each step's undo puts back the bytes that were there.
//
// A delete that fails is a warning, not a step failure (generators spec, Cleanup): the file stays
// and leaves the owned list, and Warnings says so after the steps ran.
func (a Applied) Steps() []saga.Step {
	warn := func(rel string, err error) {
		*a.warnings = append(*a.warnings, fmt.Sprintf("could not delete %s, so it stays; delete it by hand: %v", rel, err))
	}

	var steps []saga.Step
	for _, c := range a.Changes {
		switch {
		case c.Action == Write && c.Target != "":
			steps = append(steps, linkStep(a.root, c))
		case c.Action == Write:
			steps = append(steps, writeStep(a.root, c))
		case c.Action == Delete && c.Target != "":
			steps = append(steps, unlinkStep(a.root, c, warn))
		case c.Action == Delete:
			steps = append(steps, deleteStep(a.root, c, warn))
		}
	}

	seen := map[string]bool{}
	for _, s := range a.stale {
		if !seen[s] {
			seen[s] = true
			steps = append(steps, staleStep(a.root, s, warn))
		}
	}
	return steps
}

// Warnings returns the owned links that left the state in Prepare, and the deletes that failed
// when the steps ran.
func (a Applied) Warnings() []string {
	if a.warnings == nil {
		return nil
	}
	return *a.warnings
}

func writeStep(root string, c Change) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(c.Path))
	var madeDirs string // the highest directory that this step made
	madeBackup := false
	mode := newFileMode // the mode of the file that this step replaces, for the file and its backup

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
			mode = modeOf(p)
			madeDirs = firstMissing(filepath.Dir(p), root)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // workspace files are for all tools that read the space
				return err
			}
			if c.backup {
				if err := writeAtomic(p+backupSuffix, c.Old, mode); err != nil {
					return errors.Join(err, dropOwn())
				}
				madeBackup = true
			}
			if err := writeAtomic(p, c.New, mode); err != nil {
				return errors.Join(err, dropOwn()) // the saga undoes only finished steps
			}
			return nil
		},
		Undo: func(context.Context) error {
			if c.Old != nil {
				if err := writeAtomic(p, c.Old, mode); err != nil {
					return err
				}
			} else if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return dropOwn()
		},
	}
}

// newFileMode is the mode of a new workspace file: tools that run as other users read them.
const newFileMode fs.FileMode = 0o644

// modeOf returns the permissions of the file at p, or newFileMode when there is none. A file that
// tdm replaces keeps its mode, so a private file does not become readable by others.
func modeOf(p string) fs.FileMode {
	if fi, err := os.Stat(p); err == nil {
		return fi.Mode().Perm()
	}
	return newFileMode
}

// writeAtomic writes b to p with mode through a temp file in the same directory and a rename, so
// a failure part way leaves the old file whole.
func writeAtomic(p string, b []byte, mode fs.FileMode) error {
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
	if err := os.Chmod(tmp.Name(), mode); err != nil { // CreateTemp makes 0600
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func deleteStep(root string, c Change, warn func(string, error)) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(c.Path))
	madeBackup, deleted := false, false
	mode := newFileMode
	return saga.Step{
		Name: "delete " + c.Path,
		Do: func(context.Context) error {
			mode = modeOf(p)
			if c.backup {
				if err := writeAtomic(p+backupSuffix, c.Old, mode); err != nil {
					return err
				}
				madeBackup = true
			}
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				if madeBackup {
					if err := os.Remove(p + backupSuffix); err != nil {
						return err
					}
					madeBackup = false
				}
				warn(c.Path, err)
				return nil
			}
			deleted = true
			removeEmptyDirs(filepath.Dir(p), root)
			return nil
		},
		Undo: func(context.Context) error {
			if !deleted {
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // as before the delete
				return err
			}
			if err := writeAtomic(p, c.Old, mode); err != nil {
				return err
			}
			if madeBackup {
				return os.Remove(p + backupSuffix)
			}
			return nil
		},
	}
}

// staleStep deletes a derived file such as go.work.sum, which the tool makes again. A directory
// is not deleted: a stale entry names files only.
func staleStep(root, rel string, warn func(string, error)) saga.Step {
	p := filepath.Join(root, filepath.FromSlash(rel))
	var old []byte // set only when the file was deleted
	mode := newFileMode
	return saga.Step{
		Name: "delete stale " + rel,
		Do: func(context.Context) error {
			fi, err := os.Lstat(p)
			if errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.Mode().IsRegular()) {
				return nil
			}
			var b []byte
			if err == nil {
				mode = fi.Mode().Perm()
				b, err = os.ReadFile(p) //nolint:gosec // G304: a path in the space root
			}
			if err == nil {
				err = os.Remove(p)
			}
			if err != nil {
				warn(rel, err)
				return nil
			}
			old = b
			return nil
		},
		Undo: func(context.Context) error {
			if old == nil {
				return nil
			}
			return writeAtomic(p, old, mode)
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
// .cargo/ that a generator made. It removes only empty directories, never a symlink, and stops at
// the first one that it cannot remove.
func removeEmptyDirs(dir, stop string) {
	for d := dir; d != stop && strings.HasPrefix(d, stop+string(filepath.Separator)); d = filepath.Dir(d) {
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() || os.Remove(d) != nil {
			return
		}
	}
}
