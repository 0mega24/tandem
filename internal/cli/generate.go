package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/ui"
)

func generateCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "generate",
		Usage:     "Write the workspace files of a space again for its current members",
		ArgsUsage: "<space>",
		Description: "Each generator of the space (chosen at tdm new) looks for its manifests in the worktrees and\n" +
			"plans its files, such as go.work. tdm writes a file only when it wrote the file before and\n" +
			"nobody changed it since, or when the file is new. Other files are skipped with a reason;\n" +
			"--force replaces them and keeps the old file in <file>.tandem-bak.",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "show what would change, and write nothing"},
			&ucli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "replace files that tdm did not write or that a person changed"},
			&ucli.BoolFlag{Name: "list", Usage: "list the generators and exit"},
		},
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			completeSpaces(cmd)
		},
		Action: generate,
	}
}

func generate(ctx context.Context, cmd *ucli.Command) error {
	if cmd.Bool("list") {
		if cmd.Args().Len() > 0 {
			return errors.New("--list takes no space")
		}
		return listGenerators(cmd)
	}
	if cmd.Args().Len() != 1 {
		return errors.New("usage: tdm generate <space>")
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}
	res, err := m.Generate(ctx, cmd.Args().First(), space.GenerateOptions{DryRun: cmd.Bool("dry-run"), Force: cmd.Bool("force")})
	if err != nil {
		return err
	}

	for _, r := range res.Runs {
		for _, w := range r.Warnings {
			if _, err := fmt.Fprintf(cmd.Root().ErrWriter, "tdm: warning: %s: %s\n", r.ID, w); err != nil {
				return err
			}
		}
	}

	t := ui.NewTable(cmd.Root().Writer)
	generatedRows(t, res)
	if err := t.Flush(); err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		_, err := fmt.Fprintln(cmd.Root().Writer, "dry run: nothing was written")
		return err
	}
	return nil
}

// generatedRows adds one row for each file, in generator order, and a row for each generator that
// found no members and has no files.
func generatedRows(t *ui.Table, res space.GenerateResult) {
	for _, r := range res.Runs {
		members := ""
		if len(r.Members) > 0 {
			members = "(" + strings.Join(r.Members, ", ") + ")"
		}

		rows := 0
		for _, c := range res.Changes {
			if c.Generator != r.ID {
				continue
			}
			rows++
			switch c.Action {
			case gen.Write:
				t.Row("  "+r.ID, "✓ "+c.Path, members, c.Reason)
			case gen.Unchanged:
				t.Row("  "+r.ID, "= "+c.Path, "unchanged", c.Reason)
			case gen.Skip:
				t.Row("  "+r.ID, "! "+c.Path, "skipped: "+c.Reason)
			case gen.Delete:
				t.Row("  "+r.ID, "- "+c.Path, "deleted", c.Reason)
			}
		}
		switch {
		case rows > 0:
		case len(r.Members) > 0:
			t.Row("  "+r.ID, "- nothing to write", members)
		default:
			t.Row("  "+r.ID, "- no members")
		}
	}
}

func listGenerators(cmd *ucli.Command) error {
	t := ui.NewTable(cmd.Root().Writer)
	t.Row("ID", "DEFAULT", "LANGUAGES", "MANIFESTS")
	for _, g := range gen.All() {
		meta := g.Meta()
		on := "off"
		if meta.DefaultOn {
			on = "on"
		}
		t.Row(meta.ID, on, strings.Join(meta.Languages, ", "), strings.Join(meta.Manifests, ", "))
	}
	return t.Flush()
}
