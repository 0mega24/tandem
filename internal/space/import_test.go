package space

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/state"
)

func importWrite(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestImportWTG(t *testing.T) {
	home := t.TempDir()
	env := config.Env{Home: home, XDGDataHome: filepath.Join(home, "data"), XDGConfigHome: filepath.Join(home, "config")}
	root := filepath.Join(home, "work")
	work := "go 1.27\nuse ./api\n"
	importWrite(t, filepath.Join(root, "go.work"), work)
	source := filepath.Join(env.XDGDataHome, "wtg", "spaces", "feat.yaml")
	text := "name: feat\npath: " + root + "\nbranch: me/feat\ncreated_at: 2026-03-27T10:00:00Z\ngo_workspace: true\nrepos:\n  - name: org/api\n    repo_path: /repos/api\n    worktree_path: " + root + "/org/api\n    symlink: true\n"
	importWrite(t, source, text)
	importWrite(t, filepath.Join(env.XDGConfigHome, "wtg", "config.toml"), "[git]\nbranch_prefix = 'me/'\nold = true\n[always]\nrepos = ['org/api']\n[legacy]\nfoo = 1\n")
	rows, err := ImportWTG(env, "", true)
	if err != nil || len(rows) != 4 {
		t.Fatalf("dry run = %+v, %v", rows, err)
	}
	store := state.NewStore(state.DefaultDir(home, env.XDGDataHome))
	if _, err := os.Stat(store.Dir()); !os.IsNotExist(err) {
		t.Fatalf("dry run made state dir: %v", err)
	}
	target := filepath.Join(env.XDGConfigHome, "tandem", "config.toml")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote config: %v", err)
	}
	rows, err = ImportWTG(env, "", false)
	if err != nil || len(rows) != 4 {
		t.Fatalf("import = %+v, %v", rows, err)
	}
	sp, err := store.Load("feat")
	if err != nil {
		t.Fatal(err)
	}
	if sp.Path != root || sp.Branch != "me/feat" || sp.GoWorkspace || len(sp.Repos) != 1 || !sp.Repos[0].Symlink || sp.Repos[0].WorktreePath != root+"/org/api" || sp.CreatedAt.IsZero() {
		t.Fatalf("imported state = %+v", sp)
	}
	if len(sp.Generated) != 1 || sp.Generated[0].SHA256 != gen.Hash([]byte(work)) || !reflect.DeepEqual(sp.Generated[0].Stale, []string{"go.work.sum"}) || len(sp.Generators) == 0 {
		t.Fatalf("migration = %+v", sp)
	}
	loaded, err := config.Load(env, "")
	if err != nil || loaded.Config.Git.BranchPrefix != "me/" || !reflect.DeepEqual(loaded.Config.Always.Repos, []string{"org/api"}) {
		t.Fatalf("config = %+v, %v", loaded, err)
	}
	before, err := os.ReadFile(filepath.Join(store.Dir(), "feat.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfgBefore, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ImportWTG(env, "", false)
	if err != nil || !rows[0].Skipped || !strings.Contains(rows[1].Reason, "merge") {
		t.Fatalf("repeat = %+v, %v", rows, err)
	}
	for p, want := range map[string]string{source: text, filepath.Join(root, "go.work"): work, filepath.Join(store.Dir(), "feat.yaml"): string(before), target: string(cfgBefore)} {
		got, err := os.ReadFile(p)
		if err != nil || string(got) != want {
			t.Errorf("%s changed: %q, %v", p, got, err)
		}
	}
}

func TestImportWTGAbsentAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, file, body string
		explicit         bool
	}{
		{name: "absent"},
		{name: "bad state", file: "data/wtg/spaces/feat.yaml", body: "name: ["},
		{name: "wrong name", file: "data/wtg/spaces/feat.yaml", body: "name: other\npath: /work\nbranch: feat\n"},
		{name: "wrong type", file: "config/wtg/config.toml", body: "[git]\nbranch_prefix = 42\n"},
		{name: "bad config", file: "config/wtg/config.toml", body: "[git"},
		{name: "explicit missing", explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			env := config.Env{Home: home, XDGDataHome: filepath.Join(home, "data"), XDGConfigHome: filepath.Join(home, "config")}
			if tc.file != "" {
				importWrite(t, filepath.Join(home, tc.file), tc.body)
			}
			override := ""
			if tc.explicit {
				override = filepath.Join(home, "missing.toml")
			}
			rows, err := ImportWTG(env, override, false)
			if tc.name == "absent" {
				if err != nil || len(rows) != 0 {
					t.Fatalf("absent = %+v, %v", rows, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

// Review 0016, finding 1: a wtg config that tandem cannot load must not be written, as every
// tdm command would fail. The spaces still import.
func TestImportWTGInvalidConfig(t *testing.T) {
	for _, body := range []string{"[discovery]\nmax_depth = 0\n", "[always]\nfiles = ['CLAUDE.md']\n"} {
		home := t.TempDir()
		env := config.Env{Home: home, XDGDataHome: filepath.Join(home, "data"), XDGConfigHome: filepath.Join(home, "config")}
		importWrite(t, filepath.Join(env.XDGDataHome, "wtg", "spaces", "feat.yaml"), "name: feat\npath: "+home+"/w\nbranch: feat\ncreated_at: 2026-03-27T10:00:00Z\nrepos: []\n")
		importWrite(t, filepath.Join(env.XDGConfigHome, "wtg", "config.toml"), body)
		rows, err := ImportWTG(env, "", false)
		if err != nil {
			t.Fatalf("%s: import = %v", body, err)
		}
		if _, err := os.Stat(filepath.Join(env.XDGConfigHome, "tandem", "config.toml")); !os.IsNotExist(err) {
			t.Fatalf("%s: wrote a config that tandem cannot load", body)
		}
		if _, err := state.NewStore(state.DefaultDir(home, env.XDGDataHome)).Load("feat"); err != nil {
			t.Fatalf("%s: space not imported: %v", body, err)
		}
		if !slices.ContainsFunc(rows, func(r ImportRow) bool { return r.Name == "config" && r.Skipped }) {
			t.Fatalf("%s: rows = %+v, want a skipped config row", body, rows)
		}
	}
}

// Review 0016, finding 2: tandem still reads a legacy config.yaml. A config.toml next to it would
// win and hide it, so an existing config.yaml also means "merge manually".
func TestImportWTGKeepsLegacyYAMLConfig(t *testing.T) {
	home := t.TempDir()
	env := config.Env{Home: home, XDGDataHome: filepath.Join(home, "data"), XDGConfigHome: filepath.Join(home, "config")}
	importWrite(t, filepath.Join(env.XDGConfigHome, "wtg", "config.toml"), "[git]\nbranch_prefix = 'me/'\n")
	importWrite(t, filepath.Join(env.XDGConfigHome, "tandem", "config.yaml"), "git:\n  branch_prefix: you/\n")
	rows, err := ImportWTG(env, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.XDGConfigHome, "tandem", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("wrote config.toml next to config.yaml")
	}
	if !slices.ContainsFunc(rows, func(r ImportRow) bool { return r.Name == "config" && r.Skipped }) {
		t.Fatalf("rows = %+v, want a skipped config row", rows)
	}
}

// Review 0016, findings 3, 4 and 6: always.run gets a row about the variable names; files that
// are not space files get a row; a config with only unknown keys writes nothing.
func TestImportWTGRows(t *testing.T) {
	home := t.TempDir()
	env := config.Env{Home: home, XDGDataHome: filepath.Join(home, "data"), XDGConfigHome: filepath.Join(home, "config")}
	dir := filepath.Join(env.XDGDataHome, "wtg", "spaces")
	importWrite(t, filepath.Join(dir, "old.yml"), "name: old\n")
	importWrite(t, filepath.Join(dir, "my feature.yaml"), "name: x\n")
	importWrite(t, filepath.Join(env.XDGConfigHome, "wtg", "config.toml"), "[always]\nrun = '~/hook.sh'\n")
	rows, err := ImportWTG(env, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"always.run", "old.yml", "my feature.yaml"} {
		if !slices.ContainsFunc(rows, func(r ImportRow) bool { return r.Name == name }) {
			t.Errorf("rows = %+v, want a row for %s", rows, name)
		}
	}

	only := t.TempDir()
	env = config.Env{Home: only, XDGDataHome: filepath.Join(only, "data"), XDGConfigHome: filepath.Join(only, "config")}
	importWrite(t, filepath.Join(env.XDGConfigHome, "wtg", "config.toml"), "[legacy]\nfoo = 1\n")
	if _, err := ImportWTG(env, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.XDGConfigHome, "tandem", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("wrote a config with no imported key")
	}
}
