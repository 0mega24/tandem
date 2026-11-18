package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// write makes a file with its parent directories under root and returns its path.
func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// testEnv is an Env with every directory inside one temp dir, so the real /etc/xdg is never read.
func testEnv(t *testing.T) (Env, string) {
	t.Helper()
	root := t.TempDir()
	return Env{
		Home:          filepath.Join(root, "home"),
		XDGConfigHome: filepath.Join(root, "home/.config"),
		XDGConfigDirs: filepath.Join(root, "sys1") + ":" + filepath.Join(root, "sys2"),
	}, root
}

func TestLoadDefaults(t *testing.T) {
	env, _ := testEnv(t)
	got, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Discovery: Discovery{RootDir: filepath.Join(env.Home, "repos"), MaxDepth: 2},
		Spaces:    Spaces{RootDir: filepath.Join(env.Home, "workspaces")},
	}
	if !reflect.DeepEqual(got.Config, want) || len(got.Files) != 0 {
		t.Errorf("Load() = %+v, want %+v and no files", got, want)
	}
}

func TestLoadLayerOrder(t *testing.T) {
	env, root := testEnv(t)
	sys2 := write(t, root, "sys2/tandem/config.toml", "git.branch_prefix = \"sys2/\"\n[discovery]\nmax_depth = 5\n")
	sys1 := write(t, root, "sys1/tandem/config.toml", "git.branch_prefix = \"sys1/\"\n")
	user := write(t, root, "home/.config/tandem/config.toml", "spaces.root_dir = \"~/ws\"\n")

	got, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{sys2, sys1, user}; !reflect.DeepEqual(got.Files, want) {
		t.Errorf("Files = %v, want %v (last XDG dir first, user last)", got.Files, want)
	}
	c := got.Config
	if c.Git.BranchPrefix != "sys1/" {
		t.Errorf("branch_prefix = %q, want %q: the first XDG_CONFIG_DIRS entry wins", c.Git.BranchPrefix, "sys1/")
	}
	if c.Discovery.MaxDepth != 5 {
		t.Errorf("max_depth = %d, want 5 from sys2: keys that a later file does not set stay", c.Discovery.MaxDepth)
	}
	if want := filepath.Join(env.Home, "ws"); c.Spaces.RootDir != want {
		t.Errorf("spaces.root_dir = %q, want %q", c.Spaces.RootDir, want)
	}
}

func TestLoadExplicitWins(t *testing.T) {
	env, root := testEnv(t)
	write(t, root, "home/.config/tandem/config.toml", "git.branch_prefix = \"user/\"\n")
	env.TDMConfig = write(t, root, "env.toml", "git.branch_prefix = \"env/\"\n")
	flag := write(t, root, "flag.yaml", "git:\n  branch_prefix: flag/\n")

	tests := []struct {
		name, flag, want string
	}{
		{"TDM_CONFIG over user", "", "env/"},
		{"--config over TDM_CONFIG", flag, "flag/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env, tt.flag)
			if err != nil {
				t.Fatal(err)
			}
			if got.Config.Git.BranchPrefix != tt.want {
				t.Errorf("branch_prefix = %q, want %q", got.Config.Git.BranchPrefix, tt.want)
			}
		})
	}
}

func TestLoadLegacyYAML(t *testing.T) {
	env, root := testEnv(t)
	write(t, root, "home/.config/tandem/config.yaml", "always:\n  repos: [infra]\n  files: [~/notes/AGENTS.md]\n  run: ~/bin/hook\n")
	got, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	want := Always{
		Repos: []string{"infra"},
		Files: []string{filepath.Join(env.Home, "notes/AGENTS.md")},
		Run:   filepath.Join(env.Home, "bin/hook"),
	}
	if !reflect.DeepEqual(got.Config.Always, want) {
		t.Errorf("Always = %+v, want %+v", got.Config.Always, want)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, content, wantErr string
	}{
		{"unknown key", "[git]\nbranch_prefx = \"x\"\n", "branch_prefx"},
		{"bad type", "discovery.max_depth = \"two\"\n", "max_depth"},
		{"bad depth", "discovery.max_depth = 0\n", "max_depth = 0"},
		{"empty root", "discovery.root_dir = \"\"\n", "discovery.root_dir must not be empty"},
		{"absolute secret", "always.secrets = [\"/etc/passwd\"]\n", "always.secrets"},
		{"dotdot secret", "always.secrets = [\"a/../../b\"]\n", "always.secrets"},
		{"relative hook", "always.run = \"bin/hook\"\n", "always.run"},
		{"relative file", "always.files = [\"notes/AGENTS.md\"]\n", "must be an absolute path"},
		{"same base name", "always.files = [\"/a/AGENTS.md\", \"~/b/AGENTS.md\"]\n", "same base name"},
		{"bad toml", "[git\n", "parse config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, root := testEnv(t)
			write(t, root, "home/.config/tandem/config.toml", tt.content)
			_, err := Load(env, "")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingExplicit(t *testing.T) {
	env, root := testEnv(t)
	if _, err := Load(env, filepath.Join(root, "nope.toml")); err == nil {
		t.Error("Load(missing --config) = nil error, want an error")
	}
}

func TestSecretsKeepTilde(t *testing.T) {
	env, root := testEnv(t)
	write(t, root, "home/.config/tandem/config.toml", "always.secrets = [\".env\", \"config/dev.json\"]\n")
	got, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".env", "config/dev.json"}; !reflect.DeepEqual(got.Config.Always.Secrets, want) {
		t.Errorf("Secrets = %v, want %v", got.Config.Always.Secrets, want)
	}
}

func TestUserPath(t *testing.T) {
	env, root := testEnv(t)
	dir := filepath.Join(env.XDGConfigHome, "tandem")
	if got, want := UserPath(env, ""), filepath.Join(dir, "config.toml"); got != want {
		t.Errorf("UserPath() no files = %q, want %q", got, want)
	}
	yml := write(t, root, "home/.config/tandem/config.yaml", "")
	if got := UserPath(env, ""); got != yml {
		t.Errorf("UserPath() legacy only = %q, want %q", got, yml)
	}
	env.TDMConfig = "~/x.toml"
	if got, want := UserPath(env, ""), filepath.Join(env.Home, "x.toml"); got != want {
		t.Errorf("UserPath() TDM_CONFIG = %q, want %q", got, want)
	}
	if got := UserPath(env, "/f.toml"); got != "/f.toml" {
		t.Errorf("UserPath(--config) = %q, want /f.toml", got)
	}
}

func TestDefaultXDGDirs(t *testing.T) {
	// Without XDG_CONFIG_DIRS the system layer is /etc/xdg; it must not break a load when absent.
	env := Env{Home: t.TempDir()}
	files, err := layers(env, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !strings.HasPrefix(f, "/etc/xdg/tandem/") {
			t.Errorf("layers() = %v, want only /etc/xdg/tandem files", files)
		}
	}
}

func TestLoadGenerate(t *testing.T) {
	env, root := testEnv(t)
	write(t, root, "home/.config/tandem/config.toml", "[generate]\nenabled = [\"go\"]\ndetect_depth = 2\n\n[generate.rust]\nmode = \"patch\"\n")

	got, err := Load(env, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Config.Generate.Enabled, []string{"go"}) {
		t.Errorf("enabled = %v, want [go]", got.Config.Generate.Enabled)
	}
	if rust := got.Config.Generate.Table("rust"); rust["mode"] != "patch" {
		t.Errorf("generate.rust = %v, want mode patch", rust)
	}
	if got.Config.Generate.DetectDepth != 2 {
		t.Errorf("detect_depth = %d, want 2", got.Config.Generate.DetectDepth)
	}
	if got.Config.Generate.Table("go") != nil {
		t.Error("generate.go table, want none")
	}

	write(t, root, "home/.config/tandem/config.toml", "[generate]\nrust = 1\n")
	if _, err := Load(env, ""); err == nil || !strings.Contains(err.Error(), "generate.rust must be a table") {
		t.Errorf("err = %v, want a table error", err)
	}
}

func TestPluginDirs(t *testing.T) {
	env, root := testEnv(t)
	file := write(t, root, "plugins.toml", "[generate]\nplugin_dirs = [\"~/plugins\", \"/opt/plugins\"]\n")
	got, err := Load(env, file)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(env.Home, "plugins"), "/opt/plugins"}; !slices.Equal(got.Config.Generate.PluginDirs, want) {
		t.Fatalf("plugin_dirs = %v, want %v", got.Config.Generate.PluginDirs, want)
	}
	write(t, root, "plugins.toml", "[generate]\nplugin_dirs = [\"relative\"]\n")
	if _, err := Load(env, file); err == nil || !strings.Contains(err.Error(), "generate.plugin_dirs") {
		t.Fatalf("Load error = %v, want plugin_dirs error", err)
	}
}
