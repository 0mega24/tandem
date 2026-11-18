package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain lets the test binary act as tdm, so the scripts run the real CLI in a child process.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"tdm": func() { os.Exit(Run(context.Background(), os.Args, os.Stdin, os.Stdout, os.Stderr)) },
	})
}

// TestScript runs testdata/script/*.txtar. Each script has its own $WORK, and HOME and the XDG
// variables point into it, so a script never reads the real config or state.
func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   "testdata/script",
		Setup: scriptSetup,
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"mkrepo": mkrepo,
		},
	})
}

// scriptSetup gives a script its own HOME, XDG dirs and git config.
func scriptSetup(e *testscript.Env) error {
	examples, err := filepath.Abs("../../examples/plugins")
	if err != nil {
		return err
	}
	e.Setenv("PLUGIN_EXAMPLES", examples)
	home := filepath.Join(e.WorkDir, "home")
	e.Setenv("HOME", home)
	e.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	e.Setenv("XDG_CONFIG_DIRS", filepath.Join(e.WorkDir, "etc/xdg"))
	e.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local/share"))
	e.Setenv("TDM_CONFIG", "")

	// git must not read the user's config, and commits need a fixed author.
	gitconfig := filepath.Join(e.WorkDir, ".gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		return err
	}
	e.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	e.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		e.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		e.Setenv(k, "test@example.invalid")
	}

	return nil
}

// mkrepo <name>... makes $HOME/repos/<name>, a clone with one commit on main, and its bare
// origin $WORK/origins/<name>.git.
func mkrepo(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: mkrepo name...")
	}

	// ts.Exec runs with the script's environment, so the git isolation above applies.
	git := func(dir string, args ...string) {
		ts.Check(ts.Exec("git", append([]string{"-C", dir}, args...)...))
	}

	for _, name := range args {
		origin := ts.MkAbs(filepath.Join("origins", name+".git"))
		seed := ts.MkAbs(filepath.Join("seeds", name))
		clone := filepath.Join(ts.Getenv("HOME"), "repos", name)

		ts.Check(os.MkdirAll(seed, 0o755))
		git(seed, "init", "-q", "--bare", origin)
		git(seed, "init", "-q")
		ts.Check(os.WriteFile(filepath.Join(seed, "README.md"), []byte("# "+name+"\n"), 0o644))
		git(seed, "add", ".")
		git(seed, "commit", "-q", "-m", "initial")
		git(seed, "push", "-q", origin, "main")
		git(origin, "symbolic-ref", "HEAD", "refs/heads/main")
		git(seed, "clone", "-q", origin, clone)
	}
}
