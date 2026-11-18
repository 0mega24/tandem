package space

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tree makes the directories; a path ending in "/.git" marks a repo.
func tree(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func names(repos []Repo) []string {
	out := make([]string, len(repos))
	for i, r := range repos {
		out[i] = r.Name
	}
	return out
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	tree(t, root,
		"api/.git",
		"api/vendor/lib/.git", // inside a repo: not listed
		"myorg/api/.git",
		"myorg/web/.git",
		"deep/a/b/.git", // depth 3: not listed with max depth 2
		".hidden/x/.git",
		"linked/src",
	)

	// A linked worktree has a .git file, not a directory: not listed.
	if err := os.WriteFile(filepath.Join(root, "linked/.git"), []byte("gitdir: /x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root, 2)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"api", "myorg/api", "myorg/web"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("Discover() = %v, want %v", names(got), want)
	}
	if got[1].Path != filepath.Join(root, "myorg/api") {
		t.Errorf("Path = %q, want an absolute path under root", got[1].Path)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	_, err := Discover(filepath.Join(t.TempDir(), "nope"), 2)
	if err == nil || !strings.Contains(err.Error(), "discovery.root_dir") {
		t.Errorf("Discover(missing) = %v, want an error that names discovery.root_dir", err)
	}
}

func TestResolve(t *testing.T) {
	repos := []Repo{{Name: "api"}, {Name: "infra"}, {Name: "myorg/api"}, {Name: "myorg/web"}, {Name: "other/web"}}

	tests := []struct {
		arg, want, wantErr string
	}{
		{arg: "api", want: "api"},             // exact name wins over the leaf of myorg/api
		{arg: "myorg/api", want: "myorg/api"}, // full short name
		{arg: "infra", want: "infra"},
		{arg: "web", wantErr: "ambiguous: use one of myorg/web, other/web"},
		{arg: "we", wantErr: `no repo "we"; did you mean myorg/web, other/web?`},
		{arg: "zzz", wantErr: `no repo "zzz"`},
	}
	for _, tt := range tests {
		got, err := Resolve(repos, tt.arg)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Resolve(%q) error = %v, want %q", tt.arg, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got.Name != tt.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tt.arg, got.Name, err, tt.want)
		}
	}
}

func TestResolveAll(t *testing.T) {
	repos := []Repo{{Name: "api"}, {Name: "web"}}

	got, err := ResolveAll(repos, []string{"web", "api", "web"})
	if err != nil || !reflect.DeepEqual(names(got), []string{"web", "api"}) {
		t.Errorf("ResolveAll() = %v, %v; want [web api] in argument order, no duplicate", names(got), err)
	}

	_, err = ResolveAll(repos, []string{"x", "api", "y"})
	if err == nil || !strings.Contains(err.Error(), `"x"`) || !strings.Contains(err.Error(), `"y"`) {
		t.Errorf("ResolveAll(bad names) error = %v, want both bad names", err)
	}
}
