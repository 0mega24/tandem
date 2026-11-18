package config

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
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

// Env holds the process environment that config reads. Tests build it by hand.
type Env struct {
	Home          string
	XDGConfigHome string // empty means ~/.config
	XDGConfigDirs string // colon list; empty means /etc/xdg
	XDGDataHome   string // empty means ~/.local/share
	TDMConfig     string // the TDM_CONFIG variable
}

// EnvFromOS reads Env from the running process.
func EnvFromOS() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, fmt.Errorf("find home directory: %w", err)
	}
	return Env{
		Home:          home,
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		XDGConfigDirs: os.Getenv("XDG_CONFIG_DIRS"),
		XDGDataHome:   os.Getenv("XDG_DATA_HOME"),
		TDMConfig:     os.Getenv("TDM_CONFIG"),
	}, nil
}

// Loaded is the result of Load.
type Loaded struct {
	Config Config
	Files  []string // the files that were read, lowest priority first
}

// Load reads every config layer that exists, merges it over Defaults, and validates the result.
// flagPath is the --config value ("" when not given). An explicit file that does not exist is an
// error; a missing system or user file is not.
func Load(env Env, flagPath string) (Loaded, error) {
	files, err := layers(env, flagPath)
	if err != nil {
		return Loaded{}, err
	}
	k := koanf.New(".")
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304: the user chooses the config paths on purpose
		if err != nil {
			return Loaded{}, fmt.Errorf("read config: %w", err)
		}
		if err := k.Load(rawbytes.Provider(b), parserFor(f)); err != nil {
			return Loaded{}, fmt.Errorf("parse config %s: %w", f, err)
		}
	}

	cfg := Defaults()
	err = k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{DecoderConfig: &mapstructure.DecoderConfig{
		TagName:     "koanf",
		ErrorUnused: true, // a typo in a key is an error, not a silent default
		Result:      &cfg,
	}})
	if err != nil {
		return Loaded{}, fmt.Errorf("config %s: %w", strings.Join(files, ", "), err)
	}
	cfg.expand(env.Home)
	if err := cfg.validate(); err != nil {
		return Loaded{}, fmt.Errorf("config: %w", err)
	}
	return Loaded{Config: cfg, Files: files}, nil
}

// UserPath is the file that `tdm config init` writes and `tdm config path` prints: the explicit
// file (--config, then TDM_CONFIG), else the user file in $XDG_CONFIG_HOME/tandem.
func UserPath(env Env, flagPath string) string {
	if p := explicit(env, flagPath); p != "" {
		return p
	}
	dir := filepath.Join(configHome(env), "tandem")
	if p, ok := existing(dir); ok {
		return p
	}
	return filepath.Join(dir, "config.toml")
}

// layers lists the config files to read, lowest priority first.
func layers(env Env, flagPath string) ([]string, error) {
	var files []string
	dirs := strings.Split(env.XDGConfigDirs, ":")
	if env.XDGConfigDirs == "" {
		dirs = []string{"/etc/xdg"}
	}
	for _, d := range slices.Backward(dirs) {
		if !filepath.IsAbs(d) {
			continue // the XDG spec says to ignore relative entries
		}
		if p, ok := existing(filepath.Join(d, "tandem")); ok {
			files = append(files, p)
		}
	}
	if p, ok := existing(filepath.Join(configHome(env), "tandem")); ok {
		files = append(files, p)
	}
	if p := explicit(env, flagPath); p != "" {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
		files = append(files, p)
	}
	return files, nil
}

func explicit(env Env, flagPath string) string {
	if flagPath != "" {
		return expandHome(flagPath, env.Home)
	}
	return expandHome(env.TDMConfig, env.Home)
}

func configHome(env Env) string {
	if filepath.IsAbs(env.XDGConfigHome) {
		return env.XDGConfigHome
	}
	return filepath.Join(env.Home, ".config")
}

// existing returns dir/config.toml, or the legacy dir/config.yaml when only that one exists.
func existing(dir string) (string, bool) {
	for _, name := range []string{"config.toml", "config.yaml"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		} else if !errors.Is(err, fs.ErrNotExist) {
			// A file that exists but cannot be read must not be skipped: Load reports the error.
			return p, true
		}
	}
	return "", false
}

func parserFor(file string) koanf.Parser {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".yaml", ".yml":
		return yaml.Parser()
	default:
		return toml.Parser()
	}
}
