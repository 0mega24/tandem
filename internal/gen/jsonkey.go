package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// A File with a Key is a JSON file that tdm shares with a person: tdm owns one top-level key, and
// the other keys are theirs (the vscode generator owns "folders" of a .code-workspace file, and
// the person keeps "settings"). So the ownership check compares only that key:
//
//   - The hash in the state is of the key's value (compact JSON with sorted keys), not of the file.
//   - A write merges the planned value into the file on disk and keeps the other keys.
//   - A delete takes the key out; the file goes only when nothing else is left in it.
//
// Plan stays pure: it plans {"<Key>": value}, and apply does the merge.

// keyHash is the ownership record of one JSON value.
func keyHash(v any) string {
	b, _ := json.Marshal(v) // a decoded JSON value always encodes
	return Hash(b)
}

// mergeKey returns the JSON object obj with key set to v, as tdm writes it: tab indents, sorted
// keys, a final newline.
func mergeKey(obj map[string]any, key string, v any) []byte {
	out := map[string]any{key: v}
	for k, x := range obj {
		if k != key {
			out[k] = x
		}
	}
	return marshalObject(out)
}

func marshalObject(obj map[string]any) []byte {
	b, _ := json.MarshalIndent(obj, "", "\t") // values that came from json.Unmarshal
	return append(b, '\n')
}

// readObject reads a JSON object, also as JSONC (comments and trailing commas, which VS Code
// writes); ok is false when the file is not one.
func readObject(b []byte) (map[string]any, bool) {
	var obj map[string]any
	if json.Unmarshal(stripJSONC(b), &obj) != nil || obj == nil {
		return nil, false
	}
	return obj, true
}

// stripJSONC removes // and /* */ comments and trailing commas outside strings. Plain JSON comes
// back unchanged.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(b) && b[j] != '"' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j, len(b)-1)
			out = append(out, b[i:j+1]...)
			i = j
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			i-- // keep the newline
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := bytes.Index(b[i+2:], []byte("*/"))
			if end < 0 {
				return b // an open comment: let json report the error
			}
			i += end + 3
		case c == ']' || c == '}':
			k := len(out) - 1
			for k >= 0 && (out[k] == ' ' || out[k] == '\t' || out[k] == '\n' || out[k] == '\r') {
				k--
			}
			if k >= 0 && out[k] == ',' {
				out = append(out[:k], out[k+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// hasComments reports whether a write of b as plain JSON drops comments or trailing commas.
func hasComments(b []byte) bool { return !bytes.Equal(stripJSONC(b), b) }

// decideKey is decide for a File with a Key.
func decideKey(root, id string, f File, byPath map[string]Owned, force bool) (Change, *Owned, error) {
	planned, ok := readObject(f.Content)
	if !ok || len(planned) != 1 || planned[f.Key] == nil {
		return Change{}, nil, fmt.Errorf("generator %s: %s must be a JSON object with only the key %q", id, f.Path, f.Key)
	}
	value := planned[f.Key]
	c := Change{Generator: id, Path: f.Path, New: mergeKey(nil, f.Key, value)}
	own := &Owned{Generator: id, Path: f.Path, Key: f.Key, SHA256: keyHash(value)}

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
	disk, isObject := readObject(old)
	if isObject {
		c.New = mergeKey(disk, f.Key, value)
		onDisk := keyHash(disk[f.Key])
		switch {
		case onDisk == own.SHA256:
			c.Action = Unchanged // the key has the planned value; the other keys are not tdm's
			return c, own, nil
		case isOwned && prev.SHA256 == onDisk:
			c.Action = Write
			if hasComments(old) {
				c.Reason = "the file is written as plain JSON, so its comments go"
			}
			return c, own, nil
		}
	}

	var keep *Owned
	who := "not written by tdm"
	if isOwned {
		keep, who = &prev, "its "+f.Key+" key was changed by a person"
	}
	bak := f.Path + backupSuffix
	switch {
	case !force:
		c.Action, c.Reason = Skip, who+"; --force replaces the key and keeps a copy"
		return c, keep, nil
	case exists(filepath.Join(root, filepath.FromSlash(bak))):
		c.Action, c.Reason = Skip, who+", and "+bak+" exists; move it away to replace the key"
		return c, keep, nil
	}
	c.Action, c.Reason, c.backup = Write, who+"; the old file is in "+bak, true
	return c, own, nil
}

// decideKeyDelete is decideDelete for an owned key. It returns whether the record stays owned.
func decideKeyDelete(root string, o Owned, force bool) (Change, bool, error) {
	c := Change{Generator: o.Generator, Path: o.Path}

	old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(o.Path))) //nolint:gosec // G304: a path from the state, checked at load
	if errors.Is(err, fs.ErrNotExist) {
		c.Action, c.Reason = Unchanged, "already gone"
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("read %s: %w", o.Path, err)
	}
	c.Old = old

	disk, isObject := readObject(old)
	if !isObject {
		c.Action, c.Reason = Skip, "not a JSON object any more, so it is not tdm's"
		return c, false, nil
	}
	if keyHash(disk[o.Key]) != o.SHA256 {
		bak := o.Path + backupSuffix
		switch {
		case !force:
			c.Action, c.Reason = Skip, "its "+o.Key+" key was changed by a person, so it is kept; --force removes it"
			return c, true, nil
		case exists(filepath.Join(root, filepath.FromSlash(bak))):
			c.Action, c.Reason = Skip, "its "+o.Key+" key was changed, and "+bak+" exists; move it away"
			return c, true, nil
		}
		c.backup = true
	}

	delete(disk, o.Key)
	if len(disk) == 0 {
		c.Action = Delete
		return c, false, nil
	}
	c.Action, c.New, c.Reason = Write, marshalObject(disk), "the "+o.Key+" key is removed; the other keys stay"
	if hasComments(old) {
		c.Reason += "; comments go"
	}
	return c, false, nil
}
