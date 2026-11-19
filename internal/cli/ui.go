package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/tui"
)

func uiCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "ui",
		Usage: "Open the terminal UI; enter prints the path of the chosen space",
		Description: "The view draws on stderr, so stdout has only the path: cd \"$(tdm ui)\" goes to the space\n" +
			"that you choose.",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			if cmd.Bool("json") {
				return errors.New("--json is not supported by ui")
			}
			m, err := newManager(cmd)
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}

			path, err := tui.Run(ctx, uiBackend{m}, dir, cmd.Root().Reader, cmd.Root().ErrWriter)
			if err != nil || path == "" {
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, path)
			return err
		},
	}
}

// uiBackend keeps generated-file reads out of the terminal model.
type uiBackend struct{ *space.Manager }

var _ tui.Backend = uiBackend{}

func (b uiBackend) ReadGenerated(ctx context.Context, name, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sp, err := b.Space(name)
	if err != nil {
		return nil, err
	}
	for _, g := range sp.Generated {
		if g.Path == path {
			content, err := os.ReadFile(filepath.Join(sp.Path, filepath.FromSlash(path)))
			if err != nil {
				return nil, fmt.Errorf("read generated file %s: %w", path, err)
			}
			return content, nil
		}
	}
	return nil, fmt.Errorf("generated file %s is not in space %s", path, name)
}
