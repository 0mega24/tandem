package cli

import (
	"context"
	"fmt"
	"os"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/tui"
)

func uiCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "ui",
		Usage: "Open the terminal UI; enter prints the path of the chosen space",
		Description: "The view draws on stderr, so stdout has only the path: cd \"$(tdm ui)\" goes to the space\n" +
			"that you choose.",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			m, err := newManager(cmd)
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}

			path, err := tui.Run(ctx, m, dir, cmd.Root().Reader, cmd.Root().ErrWriter)
			if err != nil || path == "" {
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, path)
			return err
		},
	}
}
