// Package state stores one YAML file for each space in $XDG_DATA_HOME/tandem/spaces/.
//
// The state is a hint, not the truth: the space operations check it against the disk and git
// before they act on it. The state is not kept inside the space directory, so tdm can still
// find a space whose directory was deleted.
package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// ErrNotFound is returned by Load when no space has the name.
var ErrNotFound = errors.New("space not found")

// ErrExists is returned by Create when a space has the name.
var ErrExists = errors.New("space exists")

// Space is the stored state of one space.
type Space struct {
	Name      string      `yaml:"name"`
	Path      string      `yaml:"path"`   // absolute space root
	Branch    string      `yaml:"branch"` // the one branch of all worktrees
	CreatedAt time.Time   `yaml:"created_at"`
	Repos     []RepoEntry `yaml:"repos"`

	// Generators is the generator set of this space, chosen at tdm new. Add and remove use it.
	Generators []string `yaml:"generators,omitempty"`

	// Generated lists the files that the generators wrote, with the hash that proves tdm owns them.
	Generated []GeneratedFile `yaml:"generated,omitempty"`

	// GoWorkspace is read from wtg state files only, for the migration to Generated.
	GoWorkspace bool `yaml:"go_workspace,omitempty"`
}

// GeneratedFile is one file that a generator wrote in the space.
type GeneratedFile struct {
	Generator string   `yaml:"generator"`        // generator ID, e.g. "go"
	Path      string   `yaml:"path"`             // relative to Space.Path, with "/"
	SHA256    string   `yaml:"sha256"`           // hex hash of the bytes that tdm wrote; "" for a link
	Target    string   `yaml:"target,omitempty"` // for a symlink that tdm made: where it points
	Stale     []string `yaml:"stale,omitempty"`  // derived files deleted with this one, e.g. "go.work.sum"
}

// RepoEntry is one repo in a space: a worktree on the space branch, or a symlink to the main clone.
type RepoEntry struct {
	Name         string `yaml:"name"`          // short name, e.g. "myorg/api"
	RepoPath     string `yaml:"repo_path"`     // main clone, absolute
	WorktreePath string `yaml:"worktree_path"` // path in the space, absolute
	Symlink      bool   `yaml:"symlink,omitempty"`
}

// namePattern allows names that are safe as a file name, a directory name and a git ref part.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateName checks a space name given by the user.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) || strings.Contains(name, "..") || strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("invalid space name %q: use letters, digits, '.', '_' and '-', starting with a letter or digit", name)
	}
	return nil
}

// Store reads and writes space state files in one directory.
type Store struct {
	dir string
}

// NewStore returns a Store for dir. The directory is made on the first Save.
func NewStore(dir string) Store { return Store{dir: dir} }

// DefaultDir is $XDG_DATA_HOME/tandem/spaces, with ~/.local/share when XDG_DATA_HOME is not an
// absolute path.
func DefaultDir(home, xdgDataHome string) string {
	if !filepath.IsAbs(xdgDataHome) {
		xdgDataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdgDataHome, "tandem", "spaces")
}

// Dir is the directory of the store.
func (s Store) Dir() string { return s.dir }

func (s Store) file(name string) string { return filepath.Join(s.dir, name+".yaml") }

// Load reads the space with the name. It returns an error that wraps ErrNotFound when there is none.
func (s Store) Load(name string) (Space, error) {
	if err := ValidateName(name); err != nil {
		return Space{}, err
	}
	b, err := os.ReadFile(s.file(name))
	if errors.Is(err, fs.ErrNotExist) {
		return Space{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return Space{}, fmt.Errorf("read state: %w", err)
	}
	sp, err := decode(s.file(name), b)
	if err == nil && sp.Name != name {
		err = fmt.Errorf("parse state %s: name is %q, want %q (the file name)", s.file(name), sp.Name, name)
	}
	return sp, err
}

// List reads all spaces, sorted by name. A file that cannot be read or parsed makes List fail,
// with the file name in the error, because a skipped space would look deleted.
func (s Store) List() ([]Space, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state dir: %w", err)
	}
	var spaces []Space
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok || e.IsDir() || ValidateName(name) != nil {
			continue // not a state file, for example a temp file of an interrupted Save
		}
		sp, err := s.Load(name)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, sp)
	}
	slices.SortFunc(spaces, func(a, b Space) int { return strings.Compare(a.Name, b.Name) })
	return spaces, nil
}

// Save writes the space atomically: a reader sees the old file or the new one, never a part.
func (s Store) Save(sp Space) error {
	return s.write(sp, os.Rename)
}

// Create writes a new space like Save, but fails with ErrExists when the file is there, so two
// tdm new with one name cannot both win.
func (s Store) Create(sp Space) error {
	err := s.write(sp, os.Link)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExists, sp.Name)
	}
	return err
}

// write puts the encoded space in a temp file, then gives it its name with place.
func (s Store) write(sp Space, place func(tmp, file string) error) error {
	if err := ValidateName(sp.Name); err != nil {
		return err
	}
	b, err := yaml.Marshal(sp)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil { // per-user data, like the XDG base dirs
		return fmt.Errorf("make state dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, "."+sp.Name+".*.tmp")
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after a rename; after a link or a failure this removes it
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close() // the write error is the one to report
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := place(tmp.Name(), s.file(sp.Name)); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// Delete removes the state file of the space. A missing file is not an error.
func (s Store) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := os.Remove(s.file(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete state: %w", err)
	}
	return nil
}

func decode(file string, b []byte) (Space, error) {
	var sp Space
	if err := yaml.Unmarshal(b, &sp); err != nil {
		return Space{}, fmt.Errorf("parse state %s: %w", file, err)
	}
	if sp.Name == "" || sp.Path == "" || sp.Branch == "" {
		return Space{}, fmt.Errorf("parse state %s: name, path and branch must be set", file)
	}
	return sp, nil
}
