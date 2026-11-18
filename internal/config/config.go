// Package config loads the Tandem configuration.
//
// Files are layered; a later file overrides the keys of an earlier one:
//
//  1. tandem/config.toml in each directory of $XDG_CONFIG_DIRS (default /etc/xdg), the last
//     directory first, because the first directory has the highest priority in the XDG spec;
//  2. $XDG_CONFIG_HOME/tandem/config.toml (default ~/.config);
//  3. the file in $TDM_CONFIG, or the --config flag (the flag wins).
//
// A file ending in .yaml or .yml is read as YAML; all others as TOML. In each config directory,
// config.toml is used, or the legacy config.yaml when there is no config.toml.
package config

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Config is the merged configuration.
type Config struct {
	Discovery Discovery `koanf:"discovery"`
	Spaces    Spaces    `koanf:"spaces"`
	Git       Git       `koanf:"git"`
	Always    Always    `koanf:"always"`
	Generate  Generate  `koanf:"generate"`
}

// Discovery says where tdm finds the main clones of the repos.
type Discovery struct {
	// RootDir is scanned for git repos. A repo's short name is its path relative to RootDir.
	RootDir string `koanf:"root_dir"`

	// MaxDepth is how many directory levels below RootDir the scan goes (1 = direct children).
	MaxDepth int `koanf:"max_depth"`
}

// Spaces says where tdm makes spaces.
type Spaces struct {
	RootDir string `koanf:"root_dir"` // a space is <RootDir>/<name> unless --path is given
}

// Git holds the branch settings.
type Git struct {
	BranchPrefix string `koanf:"branch_prefix"` // the branch of a space is <BranchPrefix><name>
}

// Always lists what every new space gets in addition to the repos on the command line.
type Always struct {
	// Repos are short names of repos that are symlinked into each space (no worktree).
	Repos []string `koanf:"repos"`

	// Files are copied into the space root by base name.
	Files []string `koanf:"files"`

	// Secrets are repo-relative paths that are copied from each main clone into its worktree.
	Secrets []string `koanf:"secrets"`

	// Run is an executable that tdm runs after create, add, remove and delete.
	Run string `koanf:"run"`
}

// Generate selects the generators and holds their settings. The generator IDs are checked by the
// caller, which knows the registry (gen.Select).
type Generate struct {
	// Enabled lists the generator IDs. Not set: the defaults. Empty or ["none"]: none.
	Enabled []string `koanf:"enabled"`

	// Tables holds the [generate.<id>] tables by ID. Each generator decodes its own table.
	Tables map[string]any `koanf:",remain"`
}

// Table returns the [generate.<id>] table, or nil when there is none.
func (g Generate) Table(id string) map[string]any {
	t, _ := g.Tables[id].(map[string]any) // validate checked the type; a missing table is nil
	return t
}

// Defaults returns the configuration that applies when no file sets a key. Paths keep their ~;
// Load expands them.
func Defaults() Config {
	return Config{
		Discovery: Discovery{RootDir: "~/repos", MaxDepth: 2},
		Spaces:    Spaces{RootDir: "~/workspaces"},
	}
}

// validate checks the values that the code after Load trusts.
func (c Config) validate() error {
	var errs []error
	if c.Discovery.RootDir == "" {
		errs = append(errs, errors.New("discovery.root_dir must not be empty"))
	}
	if c.Discovery.MaxDepth < 1 {
		errs = append(errs, fmt.Errorf("discovery.max_depth = %d, must be 1 or more", c.Discovery.MaxDepth))
	}
	if c.Spaces.RootDir == "" {
		errs = append(errs, errors.New("spaces.root_dir must not be empty"))
	}
	// A relative path would depend on the directory that tdm runs in. Two files with one base
	// name would be copied to the same place in the space root.
	bases := map[string]string{}
	for _, f := range c.Always.Files {
		if !filepath.IsAbs(f) {
			errs = append(errs, fmt.Errorf("always.files: %q must be an absolute path or start with ~/", f))
			continue
		}
		if other, ok := bases[filepath.Base(f)]; ok {
			errs = append(errs, fmt.Errorf("always.files: %q and %q have the same base name", other, f))
		}
		bases[filepath.Base(f)] = f
	}
	// A relative path with a "/" would depend on the directory that the hook runs in.
	if r := c.Always.Run; r != "" && !filepath.IsAbs(r) && strings.ContainsRune(r, filepath.Separator) {
		errs = append(errs, fmt.Errorf("always.run: %q must be an absolute path, a path that starts with ~/, or a name on PATH", r))
	}
	for id, t := range c.Generate.Tables {
		if _, ok := t.(map[string]any); !ok {
			errs = append(errs, fmt.Errorf("generate.%s must be a table of settings", id))
		}
	}
	for _, s := range c.Always.Secrets {
		if s == "" || filepath.IsAbs(s) || strings.HasPrefix(s, "~") || hasDotDot(s) {
			errs = append(errs, fmt.Errorf("always.secrets: %q must be relative to the repo root and must not contain \"..\"", s))
		}
	}
	return errors.Join(errs...)
}

func hasDotDot(p string) bool {
	return slices.Contains(strings.Split(path.Clean(filepath.ToSlash(p)), "/"), "..")
}

// expand replaces a leading ~ with home in the keys that are paths on this machine.
// always.secrets are repo-relative, so they are not expanded.
func (c *Config) expand(home string) {
	c.Discovery.RootDir = expandHome(c.Discovery.RootDir, home)
	c.Spaces.RootDir = expandHome(c.Spaces.RootDir, home)
	c.Always.Run = expandHome(c.Always.Run, home)
	for i, f := range c.Always.Files {
		c.Always.Files[i] = expandHome(f, home)
	}
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}
