package gen

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FileState is what is on disk at an owned path, for status.
type FileState int

const (
	Intact  FileState = iota // what tdm wrote: the same hash, key value or link target
	Changed                  // a person changed it, so generate skips it without --force
	Gone                     // nothing is there
)

// Check compares an owned file, key or link with what is on disk, by the same rules as Prepare.
func Check(root string, o Owned) (FileState, error) {
	full := filepath.Join(root, filepath.FromSlash(o.Path))
	if o.Target != "" {
		fi, err := os.Lstat(full)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return Gone, nil
		case err != nil:
			return Changed, err
		case fi.Mode()&fs.ModeSymlink != 0 && readlink(full) == o.Target:
			return Intact, nil
		}
		return Changed, nil
	}

	b, err := os.ReadFile(full) //nolint:gosec // G304: a path from the state, checked at load
	if errors.Is(err, fs.ErrNotExist) {
		return Gone, nil
	}
	if err != nil {
		return Changed, err
	}
	if o.Key != "" {
		if obj, ok := readObject(b); ok && keyHash(obj[o.Key]) == o.SHA256 {
			return Intact, nil
		}
		return Changed, nil
	}
	if Hash(b) == o.SHA256 {
		return Intact, nil
	}
	return Changed, nil
}
