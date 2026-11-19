package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/config"
)

func configCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "config",
		Usage: "Print the config files that tdm reads, in order (a later file overrides an earlier one)",
		Action: func(_ context.Context, cmd *ucli.Command) error {
			env, err := config.EnvFromOS()
			if err != nil {
				return err
			}
			loaded, err := config.Load(env, cmd.String("config"))
			if err != nil {
				return err
			}
			w := cmd.Root().Writer
			if cmd.Bool("json") {
				out := configOutput{Layers: make([]configLayerOutput, len(loaded.Files)), UserPath: config.UserPath(env, cmd.String("config"))}
				for i, f := range loaded.Files {
					b, err := os.ReadFile(f) //nolint:gosec // G304: Load already read this file
					if err != nil {
						return err
					}
					out.Layers[i] = configLayerOutput{Path: f, Content: string(b)}
				}
				return writeJSON(w, out)
			}
			if len(loaded.Files) == 0 {
				_, err := fmt.Fprintf(w, "# no config file; defaults apply. User file: %s\n", config.UserPath(env, cmd.String("config")))
				return err
			}
			for _, f := range loaded.Files {
				b, err := os.ReadFile(f) //nolint:gosec // G304: Load already read this file
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(w, "# %s\n%s\n", f, b); err != nil {
					return err
				}
			}
			return nil
		},
		Commands: []*ucli.Command{
			{
				Name:  "init",
				Usage: "Write the user config file with the generator settings as comments",
				Description: "The file starts with a #:schema line, so editors with Taplo (Even Better TOML) complete and\n" +
					"check the keys. An existing file is not changed.",
				Action: func(_ context.Context, cmd *ucli.Command) error {
					env, err := config.EnvFromOS()
					if err != nil {
						return err
					}
					p := config.UserPath(env, cmd.String("config"))
					if filepath.Ext(p) != ".toml" {
						return fmt.Errorf("%s: tdm config init writes TOML only", p)
					}
					if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // G301: the user's config directory
						return err
					}
					f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G302,G304: the user's own config file
					if errors.Is(err, fs.ErrExist) {
						return fmt.Errorf("%s exists; tdm config init does not change it", p)
					}
					if err != nil {
						return err
					}
					if _, err := f.WriteString(config.Template()); err != nil {
						return errors.Join(err, f.Close())
					}
					if err := f.Close(); err != nil {
						return err
					}
					if cmd.Bool("json") {
						return writeJSON(cmd.Root().Writer, configInitOutput{Path: p})
					}
					_, err = fmt.Fprintln(cmd.Root().Writer, p)
					return err
				},
			},
			{
				Name:  "path",
				Usage: "Print the path of the user config file",
				Action: func(_ context.Context, cmd *ucli.Command) error {
					env, err := config.EnvFromOS()
					if err != nil {
						return err
					}
					if cmd.Bool("json") {
						return writeJSON(cmd.Root().Writer, configPathOutput{Path: config.UserPath(env, cmd.String("config"))})
					}
					_, err = fmt.Fprintln(cmd.Root().Writer, config.UserPath(env, cmd.String("config")))
					return err
				},
			},
		},
	}
}
