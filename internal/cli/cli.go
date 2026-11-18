// Package cli holds the tdm commands. It parses flags and arguments and prints results; the
// work is done in other internal packages.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/version"
)

// Run runs tdm with the full argument list (args[0] is the program name) and returns the exit code.
// stdin is read only for a confirmation.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newRoot(stdin, stdout, stderr)
	if err := root.Run(ctx, args); err != nil {
		if !errors.Is(err, errReported) {
			// The exit code reports the failure; a broken stderr leaves nothing else to do.
			_, _ = fmt.Fprintln(stderr, "tdm:", err)
		}
		return 1
	}
	return 0
}

func newRoot(stdin io.Reader, stdout, stderr io.Writer) *ucli.Command {
	return &ucli.Command{
		Name:                  "tdm",
		Usage:                 "one branch across many repos, as git worktrees, with language workspace files",
		Reader:                stdin,
		Writer:                stdout,
		ErrWriter:             stderr,
		EnableShellCompletion: true,
		HideVersion:           true,
		// Run returns the exit code; urfave/cli must not call os.Exit itself.
		ExitErrHandler: func(context.Context, *ucli.Command, error) {},
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:      "config",
				Usage:     "config `FILE` that overrides all other layers (also: TDM_CONFIG)",
				TakesFile: true,
			},
		},
		Commands: []*ucli.Command{
			newCommand(),
			addCommand(),
			removeCommand(),
			deleteCommand(),
			pathCommand(),
			statusCommand(),
			repoCommand(),
			configCommand(),
			{
				Name:  "version",
				Usage: "Print the version",
				Action: func(_ context.Context, cmd *ucli.Command) error {
					_, err := fmt.Fprintln(cmd.Root().Writer, version.Get())
					return err
				},
			},
		},
	}
}
