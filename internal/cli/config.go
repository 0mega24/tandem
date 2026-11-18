package cli

import (
	"context"
	"fmt"
	"os"

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
				Name:  "path",
				Usage: "Print the path of the user config file",
				Action: func(_ context.Context, cmd *ucli.Command) error {
					env, err := config.EnvFromOS()
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(cmd.Root().Writer, config.UserPath(env, cmd.String("config")))
					return err
				},
			},
		},
	}
}
