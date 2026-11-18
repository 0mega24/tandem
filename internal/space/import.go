package space

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/state"
)

// ImportRow describes one space, config file, or unsupported config key.
type ImportRow struct {
	Name    string
	Skipped bool
	Reason  string
}

// ImportWTG copies the wtg records without changing worktrees or source files.
// wtgConfig is WTG_CONFIG; an empty value selects the XDG user config.
func ImportWTG(env config.Env, wtgConfig string, dryRun bool) ([]ImportRow, error) {
	source := state.NewStore(filepath.Join(filepath.Dir(filepath.Dir(state.DefaultDir(env.Home, env.XDGDataHome))), "wtg", "spaces"))
	target := state.NewStore(state.DefaultDir(env.Home, env.XDGDataHome))
	spaces, err := source.List()
	if err != nil {
		return nil, err
	}
	skippedFiles, err := importSkippedFiles(source.Dir())
	if err != nil {
		return nil, err
	}
	configHome := env.XDGConfigHome
	if !filepath.IsAbs(configHome) {
		configHome = filepath.Join(env.Home, ".config")
	}
	explicit := wtgConfig != ""
	if !explicit {
		wtgConfig = filepath.Join(configHome, "wtg", "config.toml")
	}
	if wtgConfig == "~" || strings.HasPrefix(wtgConfig, "~/") {
		wtgConfig = filepath.Join(env.Home, strings.TrimPrefix(wtgConfig[1:], "/"))
	}
	body, unknown, hasRun, err := importConfig(wtgConfig, explicit, env.Home)
	var rows []ImportRow
	var invalid invalidConfig
	switch {
	case errors.As(err, &invalid):
		// Every tdm command would fail with this config: import the spaces, not the config.
		rows = append(rows, ImportRow{Name: "config", Skipped: true, Reason: "not imported: " + err.Error()})
	case err != nil:
		return nil, err
	}
	for _, sp := range spaces {
		row := ImportRow{Name: sp.Name, Reason: sp.Path}
		// Lstat also protects malformed files and dangling links from replacement.
		_, err := os.Lstat(filepath.Join(target.Dir(), sp.Name+".yaml"))
		switch {
		case err == nil:
			row.Skipped = true
			row.Reason = "Tandem space exists"
		case !errors.Is(err, fs.ErrNotExist):
			return rows, partial(err)
		case dryRun:
			row.Reason = "would import " + sp.Path
		default:
			// A wtg file has no generated files yet, so healGenerated only migrates go_workspace
			// and does not warn: the zero Manager, which has no writer, is enough.
			m := Manager{}
			sp, _ = m.healGenerated(sp)
			if err := target.Create(sp); errors.Is(err, state.ErrExists) {
				row.Skipped = true
				row.Reason = "Tandem space exists"
			} else if err != nil {
				return rows, partial(err)
			}
		}
		rows = append(rows, row)
	}
	if body != nil {
		p := filepath.Join(configHome, "tandem", "config.toml")
		row, err := importConfigFile(p, body, dryRun)
		if err != nil {
			return rows, partial(err)
		}
		rows = append(rows, row)
		if hasRun {
			rows = append(rows, ImportRow{Name: "always.run", Reason: "imported; the hook gets TDM_* variables, not WTG_*: update the script"})
		}
	}
	rows = append(rows, skippedFiles...)
	for _, key := range unknown {
		rows = append(rows, ImportRow{Name: key, Skipped: true, Reason: "unsupported wtg key; merge manually"})
	}
	return rows, nil
}

// invalidConfig is a wtg config that decodes but that tdm cannot load.
type invalidConfig struct{ error }

// importConfig returns the tandem config to write, the wtg keys that tandem does not know, and
// whether always.run is set (the hook gets other variable names).
func importConfig(file string, explicit bool, home string) ([]byte, []string, bool, error) {
	b, err := os.ReadFile(file) //nolint:gosec // G304: the user selects the wtg config to import
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read wtg config: %w", err)
	}
	parser := toml.Parser()
	values, err := parser.Unmarshal(b)
	if err != nil {
		return nil, nil, false, fmt.Errorf("parse wtg config %s: %w", file, err)
	}
	// These are the shared wtg settings. New Tandem settings must not become wtg settings.
	shared := map[string][]string{
		"discovery": {"root_dir", "max_depth"},
		"spaces":    {"root_dir"},
		"git":       {"branch_prefix"},
		"always":    {"repos", "files", "secrets", "run"},
	}
	mapped := map[string]any{}
	var unknown []string
	for section, value := range values {
		keys, ok := shared[section]
		table, isTable := value.(map[string]any)
		if !ok {
			unknown = append(unknown, importUnknownKeys(section, value)...)
			continue
		}
		if !isTable {
			return nil, nil, false, fmt.Errorf("wtg config %s: %s must be a table", file, section)
		}
		keep := map[string]any{}
		for key, value := range table {
			if slices.Contains(keys, key) {
				keep[key] = value
			} else {
				unknown = append(unknown, importUnknownKeys(section+"."+key, value)...)
			}
		}
		if len(keep) > 0 {
			mapped[section] = keep
		}
	}
	cfg := config.Defaults()
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{TagName: "koanf", Result: &cfg})
	if err != nil {
		return nil, nil, false, err
	}
	if err := decoder.Decode(mapped); err != nil {
		return nil, nil, false, fmt.Errorf("wtg config %s: %w", file, err)
	}
	slices.Sort(unknown)
	if len(mapped) == 0 {
		return nil, unknown, false, nil // only unknown keys: nothing to write
	}
	if err := config.Check(cfg, home); err != nil {
		return nil, unknown, false, invalidConfig{fmt.Errorf("wtg config %s: %w", file, err)}
	}
	always, _ := mapped["always"].(map[string]any)
	_, hasRun := always["run"]
	b, err = parser.Marshal(mapped)
	if err != nil {
		return nil, nil, false, fmt.Errorf("encode imported config: %w", err)
	}
	return []byte(config.Template() + "\n# Settings imported from wtg. Hooks must use TDM_* environment variables.\n" + string(b)), unknown, hasRun, nil
}

func importUnknownKeys(prefix string, value any) []string {
	table, ok := value.(map[string]any)
	if !ok || len(table) == 0 {
		return []string{prefix}
	}
	var keys []string
	for key, value := range table {
		keys = append(keys, importUnknownKeys(prefix+"."+key, value)...)
	}
	return keys
}

func importConfigFile(file string, body []byte, dryRun bool) (ImportRow, error) {
	row := ImportRow{Name: "config", Reason: file}
	// tandem also reads a legacy config.yaml; a config.toml next to it would hide it.
	for _, p := range []string{file, filepath.Join(filepath.Dir(file), "config.yaml")} {
		_, err := os.Lstat(p)
		if err == nil {
			row.Skipped = true
			row.Reason = "Tandem config exists (" + filepath.Base(p) + "); merge wtg config manually"
			return row, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return row, err
		}
	}
	if dryRun {
		row.Reason = "would import " + file
		return row, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return row, err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the user's XDG config path
	if errors.Is(err, fs.ErrExist) {
		row.Skipped = true
		row.Reason = "Tandem config exists; merge wtg config manually"
		return row, nil
	}
	if err != nil {
		return row, err
	}
	_, err = f.Write(body)
	return row, errors.Join(err, f.Close())
}

// partial says that the spaces before the error are imported: a second run skips them.
func partial(err error) error {
	return fmt.Errorf("partial import (run tdm import wtg again to finish): %w", err)
}

// importSkippedFiles lists the files in the wtg state dir that are not space files, so the
// import reports them and does not skip them in silence.
func importSkippedFiles(dir string) ([]ImportRow, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []ImportRow
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if e.IsDir() || (ok && state.ValidateName(name) == nil) {
			continue
		}
		rows = append(rows, ImportRow{Name: e.Name(), Skipped: true, Reason: "not a wtg space file (<name>.yaml); import it by hand"})
	}
	return rows, nil
}
