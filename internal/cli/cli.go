// Package cli holds the tdm commands. It parses flags and arguments and prints results; the
// work is done in other internal packages.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/gen"
	_ "github.com/0mega24/tandem/internal/gen/all" // the built-in generators register themselves
	"github.com/0mega24/tandem/internal/gen/external"
	"github.com/0mega24/tandem/internal/ui"
	"github.com/0mega24/tandem/internal/version"
)

// runMu serializes Run; see Run.
var runMu sync.Mutex

// Run runs tdm with the full argument list (args[0] is the program name) and returns the exit code.
// stdin is read only for a confirmation.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// The generator registry is global, and the discovered plugins belong to this run, so the
	// runs of one process take turns. A real tdm process has one run; this guards embedded use.
	runMu.Lock()
	defer runMu.Unlock()
	defer func() {
		for _, g := range gen.All() {
			if p, ok := g.(external.Plugin); ok {
				gen.Unregister(p.ID())
			}
		}
	}()
	root := newRoot(stdin, stdout, stderr)
	if err := root.Run(ctx, args); err != nil {
		if root.Bool("json") {
			// The exit code still reports failure when stderr cannot be written.
			_ = writeJSON(stderr, errorOutput{Error: err.Error()})
		} else if !errors.Is(err, errReported) {
			// The exit code reports the failure; a broken stderr leaves nothing else to do.
			_, _ = fmt.Fprintln(stderr, ui.Error(stderr), err)
		}
		return 1
	}
	return 0
}

func newRoot(stdin io.Reader, stdout, stderr io.Writer) *ucli.Command {
	root := &ucli.Command{
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
			&ucli.BoolFlag{Name: "json", Usage: "print command results as JSON"},
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
			execCommand(),
			generateCommand(),
			uiCommand(),
			pluginCommand(),
			pathCommand(),
			statusCommand(),
			repoCommand(),
			configCommand(),
			importCommand(),
			{
				Name:  "version",
				Usage: "Print the version",
				Action: func(_ context.Context, cmd *ucli.Command) error {
					info := version.Get()
					if cmd.Bool("json") {
						return writeJSON(cmd.Root().Writer, versionOutput{Version: info.Version, Commit: info.Commit, Date: info.Date})
					}
					_, err := fmt.Fprintln(cmd.Root().Writer, info)
					return err
				},
			},
		},
	}
	jsonUsageErrors(root)
	return root
}

// jsonUsageErrors keeps parser help out of JSON errors. Text mode keeps urfave's usage output.
func jsonUsageErrors(cmd *ucli.Command) {
	cmd.OnUsageError = func(_ context.Context, current *ucli.Command, err error, _ bool) error {
		if current.Bool("json") {
			return err
		}
		if _, writeErr := fmt.Fprintf(current.Root().ErrWriter, "Incorrect Usage: %s\n\n", err); writeErr != nil {
			return errors.Join(err, writeErr)
		}
		if current == current.Root() {
			return errors.Join(err, ucli.ShowRootCommandHelp(current))
		}
		return errors.Join(err, ucli.ShowSubcommandHelp(current))
	}
	for _, child := range cmd.Commands {
		jsonUsageErrors(child)
	}
}
