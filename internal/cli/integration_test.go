//go:build integration

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestIntegration runs testdata/integration/*.txtar: each script makes a space with tdm and then
// runs the real build tool in it (make test-integration, in the integration dev shell). A script
// skips itself when its tool is missing ([!exec:cargo] skip).
//
// The scripts get their own HOME, as TestScript does, but the tool caches stay in the real home:
// so a second run builds offline, and a script never fills a fresh cache from the network.
func TestIntegration(t *testing.T) {
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	caches := map[string]string{
		"GOMODCACHE":           filepath.Join(realHome, "go", "pkg", "mod"),
		"GOCACHE":              filepath.Join(realHome, ".cache", "go-build"),
		"CARGO_HOME":           filepath.Join(realHome, ".cargo"),
		"GRADLE_USER_HOME":     filepath.Join(realHome, ".gradle"),
		"MAVEN_OPTS":           "-Dmaven.repo.local=" + filepath.Join(realHome, ".m2", "repository"),
		"npm_config_cache":     filepath.Join(realHome, ".npm"),
		"npm_config_store_dir": filepath.Join(realHome, ".local", "share", "pnpm", "store"),
		"UV_CACHE_DIR":         filepath.Join(realHome, ".cache", "uv"),
		"CABAL_DIR":            filepath.Join(realHome, ".cabal"),
		"NUGET_PACKAGES":       filepath.Join(realHome, ".nuget", "packages"),
		"DOTNET_CLI_HOME":      filepath.Join(realHome, ".dotnet-home"),
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/integration",
		Setup: func(e *testscript.Env) error {
			if err := scriptSetup(e); err != nil {
				return err
			}
			for k, v := range caches {
				if os.Getenv(k) != "" {
					v = os.Getenv(k) // CI points a cache somewhere else
				}
				e.Setenv(k, v)
			}
			e.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
			e.Setenv("DOTNET_NOLOGO", "1")
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"mkrepo": mkrepo,
		},
	})
}
