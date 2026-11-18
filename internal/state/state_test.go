package state

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sample(name string) Space {
	return Space{
		Name:      name,
		Path:      "/ws/" + name,
		Branch:    "me/" + name,
		CreatedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
		Repos: []RepoEntry{
			{Name: "myorg/api", RepoPath: "/repos/myorg/api", WorktreePath: "/ws/" + name + "/myorg/api"},
			{Name: "infra", RepoPath: "/repos/infra", WorktreePath: "/ws/" + name + "/infra", Symlink: true},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "spaces"))
	want := sample("feat-1")
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("feat-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestFileFormat(t *testing.T) {
	// The YAML keys are a file format: other tools and older versions read them.
	s := NewStore(t.TempDir())
	if err := s.Save(sample("f")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.Dir(), "f.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `name: f
path: /ws/f
branch: me/f
created_at: 2026-11-18T10:00:00Z
repos:
    - name: myorg/api
      repo_path: /repos/myorg/api
      worktree_path: /ws/f/myorg/api
    - name: infra
      repo_path: /repos/infra
      worktree_path: /ws/f/infra
      symlink: true
`
	if string(b) != want {
		t.Errorf("state file =\n%s\nwant\n%s", b, want)
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := NewStore(t.TempDir()).Load("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Load(missing) error = %v, want ErrNotFound", err)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	tests := []struct{ name, content, wantErr string }{
		{"not yaml", "name: [", "parse state"},
		{"missing branch", "name: x\npath: /ws/x\n", "must be set"},
		{"name differs", "name: y\npath: /ws/y\nbranch: b\n", `name is "y"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := NewStore(dir).Load("x")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestListSortedAndSkipsOtherFiles(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if err := s.Save(sample(n)); err != nil {
			t.Fatal(err)
		}
	}
	for _, junk := range []string{".alpha.123.tmp", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(s.Dir(), junk), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sp := range got {
		names = append(names, sp.Name)
	}
	if want := []string{"alpha", "mid", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("List() names = %v, want %v", names, want)
	}
}

func TestListMissingDir(t *testing.T) {
	got, err := NewStore(filepath.Join(t.TempDir(), "none")).List()
	if err != nil || len(got) != 0 {
		t.Errorf("List() on a missing dir = %v, %v; want no spaces and no error", got, err)
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	s := NewStore(t.TempDir())
	for range 2 { // the second Save replaces the file
		if err := s.Save(sample("a")); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "a.yaml" {
		t.Errorf("state dir has %v, want only a.yaml", entries)
	}
}

func TestDelete(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(sample("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load after Delete error = %v, want ErrNotFound", err)
	}
	if err := s.Delete("a"); err != nil {
		t.Errorf("Delete(missing) = %v, want nil", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"a", "feat-1", "my_feature.v2", "X9"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "a..b", "-x", ".hidden", "a/b", "a b", "x.lock", "ü"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", bad)
		}
	}
}

func TestDefaultDir(t *testing.T) {
	if got, want := DefaultDir("/h", ""), "/h/.local/share/tandem/spaces"; got != want {
		t.Errorf("DefaultDir no XDG = %q, want %q", got, want)
	}
	if got, want := DefaultDir("/h", "/data"), "/data/tandem/spaces"; got != want {
		t.Errorf("DefaultDir XDG = %q, want %q", got, want)
	}
	if got, want := DefaultDir("/h", "rel"), "/h/.local/share/tandem/spaces"; got != want {
		t.Errorf("DefaultDir relative XDG = %q, want %q", got, want)
	}
}

func TestCreateRefusesAnExistingSpace(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Create(sample("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(sample("a")); !errors.Is(err, ErrExists) {
		t.Errorf("second Create() = %v, want ErrExists", err)
	}

	entries, err := os.ReadDir(s.Dir())
	if err != nil || len(entries) != 1 {
		t.Errorf("state dir has %d entries (%v), want only a.yaml", len(entries), err)
	}
}

func TestFileFormatGenerated(t *testing.T) {
	s := NewStore(t.TempDir())
	sp := sample("g")
	sp.Repos = sp.Repos[:1]
	sp.Generators = []string{"go", "rust"}
	sp.Generated = []GeneratedFile{{Generator: "go", Path: "go.work", SHA256: "ab12", Stale: []string{"go.work.sum"}}}
	if err := s.Save(sp); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(s.Dir(), "g.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `name: g
path: /ws/g
branch: me/g
created_at: 2026-11-18T10:00:00Z
repos:
    - name: myorg/api
      repo_path: /repos/myorg/api
      worktree_path: /ws/g/myorg/api
generators:
    - go
    - rust
generated:
    - generator: go
      path: go.work
      sha256: ab12
      stale:
        - go.work.sum
`
	if string(b) != want {
		t.Errorf("state file =\n%s\nwant\n%s", b, want)
	}
}

func TestLoadWtgGoWorkspace(t *testing.T) {
	s := NewStore(t.TempDir())
	write := "name: w\npath: /ws/w\nbranch: w\ncreated_at: 2026-03-27T10:00:00Z\nrepos: []\ngo_workspace: true\n"
	if err := os.WriteFile(filepath.Join(s.Dir(), "w.yaml"), []byte(write), 0o600); err != nil {
		t.Fatal(err)
	}

	sp, err := s.Load("w")
	if err != nil || !sp.GoWorkspace {
		t.Errorf("Load = %+v, %v; want GoWorkspace from a wtg state file", sp, err)
	}
}
