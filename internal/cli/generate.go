package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/external"
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
			"--force replaces them and keeps the old file in <file>.tandem-bak. --dry-run shows the rows\n" +
			"and a diff of each file that would change.",
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

	if err := printWarnings(cmd, res); err != nil {
		return err
	}

	if cmd.Bool("json") {
		return writeJSON(cmd.Root().Writer, jsonGenerate(res, cmd.Bool("dry-run")))
	}
	t := ui.NewTable(cmd.Root().Writer)
	generatedRows(t, res)
	if err := t.Flush(); err != nil {
		return err
	}
	if !cmd.Bool("dry-run") {
		return nil
	}
	for _, c := range res.Changes {
		var d string
		switch c.Action {
		case gen.Write:
			d = ui.Diff(c.Path, c.Old, c.New)
		case gen.Delete:
			d = ui.Diff(c.Path, c.Old, nil)
		}
		if ui.Color(cmd.Root().Writer) {
			d = ui.ColorDiff(d)
		}
		if _, err := io.WriteString(cmd.Root().Writer, d); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(cmd.Root().Writer, "dry run: nothing was written")
	return err
}

// printWarnings writes the warnings of the generators and of the failed deletes to stderr.
func printWarnings(cmd *ucli.Command, res space.GenerateResult) error {
	for _, r := range res.Runs {
		for _, w := range r.Warnings {
			if _, err := fmt.Fprintf(cmd.Root().ErrWriter, "%s %s: %s\n", ui.Warning(cmd.Root().ErrWriter), r.ID, w); err != nil {
				return err
			}
		}
	}
	for _, w := range res.Warnings {
		if _, err := fmt.Fprintf(cmd.Root().ErrWriter, "%s %s\n", ui.Warning(cmd.Root().ErrWriter), w); err != nil {
			return err
		}
	}
	return nil
}

// generatedRows adds one row for each file, in generator order, and a row for each generator that
// found members but has no files (one member: nothing to connect). A generator with no members gets no row: most generators of a
// space find nothing, and a row for each would hide the rows that matter.
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
		case len(r.Members) == 1:
			t.Row("  "+r.ID, "- nothing to connect", members)
		case len(r.Members) > 1:
			t.Row("  "+r.ID, "- nothing to write", members)
		}
	}
}

func listGenerators(cmd *ucli.Command) error {
	if _, _, err := loadGenerators(cmd); err != nil {
		return err
	}
	out := generateListOutput{Generators: []generatorOutput{}}
	t := ui.NewTable(cmd.Root().Writer)
	if !cmd.Bool("json") {
		t.Row("ID", "DEFAULT", "LANGUAGES", "MANIFESTS", "SOURCE")
	}
	for _, g := range gen.All() {
		if p, ok := g.(external.Plugin); ok {
			if err := p.Load(); err != nil {
				if _, werr := fmt.Fprintln(cmd.Root().ErrWriter, ui.Warning(cmd.Root().ErrWriter), err); werr != nil {
					return werr
				}
			}
		}
		meta := g.Meta()
		on := "off"
		if meta.DefaultOn {
			on = "on"
		}
		source := "built-in"
		if p, ok := g.(external.Plugin); ok {
			source = p.Path
		}
		if cmd.Bool("json") {
			out.Generators = append(out.Generators, generatorOutput{
				ID:        meta.ID,
				DefaultOn: meta.DefaultOn,
				Languages: append([]string{}, meta.Languages...),
				Manifests: append([]string{}, meta.Manifests...),
				Source:    source,
			})
		} else {
			t.Row(meta.ID, on, strings.Join(meta.Languages, ", "), strings.Join(meta.Manifests, ", "), source)
		}
	}
	if cmd.Bool("json") {
		return writeJSON(cmd.Root().Writer, out)
	}
	return t.Flush()
}
