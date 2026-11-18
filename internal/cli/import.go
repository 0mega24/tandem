package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/ui"
)

func importCommand() *ucli.Command {
	return &ucli.Command{
		Name: "import", Usage: "Import spaces and config from another tool",
		Commands: []*ucli.Command{{
			Name: "wtg", Usage: "Import wtg state and config without changing worktrees",
			Flags: []ucli.Flag{&ucli.BoolFlag{Name: "dry-run", Usage: "show what would be imported without writing files"}},
			Action: func(_ context.Context, cmd *ucli.Command) error {
				if cmd.Args().Len() != 0 {
					return errors.New("tdm import wtg takes no arguments")
				}
				env, err := config.EnvFromOS()
				if err != nil {
					return err
				}
				rows, err := space.ImportWTG(env, os.Getenv("WTG_CONFIG"), cmd.Bool("dry-run"))
				t := ui.NewTable(cmd.Root().Writer)
				for _, row := range rows {
					status := "✓ imported"
					if row.Skipped {
						status = "- skipped"
					} else if cmd.Bool("dry-run") {
						status = "- planned"
					}
					t.Row(row.Name, status, row.Reason)
				}
				if len(rows) == 0 && err == nil {
					_, err = fmt.Fprintln(cmd.Root().Writer, "No wtg spaces or config to import.")
				}
				return errors.Join(err, t.Flush())
			},
		}},
	}
}
