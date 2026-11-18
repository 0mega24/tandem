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
		"tdm": func() { os.Exit(Run(context.Background(), os.Args, os.Stdout, os.Stderr)) },
	})
}

// TestScript runs testdata/script/*.txtar. Each script has its own $WORK, and HOME and the XDG
// variables point into it, so a script never reads the real config or state.
func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(e *testscript.Env) error {
			home := filepath.Join(e.WorkDir, "home")
			e.Setenv("HOME", home)
			e.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			e.Setenv("XDG_CONFIG_DIRS", filepath.Join(e.WorkDir, "etc/xdg"))
			e.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local/share"))
			e.Setenv("TDM_CONFIG", "")
			return nil
		},
	})
}
