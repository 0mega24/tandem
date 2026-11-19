package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
	"github.com/0mega24/tandem/internal/ui"
)

func newCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "new",
		Usage:     "Make a space: a worktree on one new or existing branch in each repo",
		ArgsUsage: "<space> <repo>...",
		Description: "The branch is git.branch_prefix + <space>. In a repo where it does not exist, it is made\n" +
			"from the local default branch (run tdm repo sync first to start from origin). Where it\n" +
			"exists, it is checked out as it is. A branch that another worktree has checked out is an error.\n\n" +
			"The space also gets the always.* settings: a symlink to the main clone of each always.repos\n" +
			"repo that is not named here, a copy of each always.files file in the space root, and a copy\n" +
			"of each always.secrets file that a main clone has, in its worktree.\n\n" +
			"Then the generators (generate.enabled, or --generate) write the workspace files, such as\n" +
			"go.work. The space keeps this generator set for add, remove and generate.\n\n" +
			repoNameHelp + "\n\n" +
			"All checks run before the first change. When a step fails, the steps before it are undone.",
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "branch", Usage: "use branch `NAME` in place of git.branch_prefix + <space>"},
			&ucli.StringFlag{Name: "path", Usage: "make the space in `DIR` in place of spaces.root_dir/<space>", TakesFile: true},
			&ucli.StringFlag{Name: "generate", Usage: "use the generators `IDS` (comma list, or none) in place of generate.enabled"},
			&ucli.BoolFlag{Name: "no-generate", Usage: "use no generators (the same as --generate=none)"},
		},
		Action: newSpace,
	}
}

func newSpace(ctx context.Context, cmd *ucli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return errors.New("give a space name and at least one repo: tdm new <space> <repo>")
	}

	var generators []string // nil: generate.enabled
	switch {
	case cmd.IsSet("generate") && cmd.Bool("no-generate"):
		return errors.New("--generate and --no-generate do not go together")
	case cmd.Bool("no-generate"):
		generators = []string{"none"}
	case cmd.IsSet("generate"):
		generators = []string{}
		for id := range strings.SplitSeq(cmd.String("generate"), ",") {
			if id = strings.TrimSpace(id); id != "" {
				generators = append(generators, id)
			}
		}
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	res, err := m.NewSpace(ctx, space.NewOptions{
		Name:       args[0],
		Repos:      args[1:],
		Branch:     cmd.String("branch"),
		Path:       cmd.String("path"),
		Generators: generators,
	})
	if err != nil {
		return err
	}

	return printNew(cmd, res)
}

// printNew prints what new or add made: the worktrees, links and copies.
func printNew(cmd *ucli.Command, res space.NewResult) error {
	if cmd.Bool("json") {
		if err := printWarnings(cmd, res.Generated); err != nil {
			return err
		}
		out := jsonNew(res)
		if cmd.Name == "add" {
			return writeJSON(cmd.Root().Writer, addOutput(out))
		}
		return writeJSON(cmd.Root().Writer, out)
	}
	t := ui.NewTable(cmd.Root().Writer)
	t.Row(res.Space.Name, res.Space.Path)
	for _, r := range res.Repos {
		if r.Entry.Symlink {
			t.Row("  "+r.Entry.Name, "✓ symlink", "→ main clone (always.repos)")
			continue
		}

		how := "existing branch"
		if r.From != "" {
			how = "new branch from " + r.From
		}
		if r.WasLink {
			how += ", was a symlink"
		}
		secrets := ""
		if len(r.Secrets) > 0 {
			secrets = "+ " + strings.Join(r.Secrets, ", ")
		}
		t.Row("  "+r.Entry.Name, "✓ worktree", "["+res.Space.Branch+"]", how, secrets)
	}
	for _, f := range res.Files {
		t.Row("  "+f, "✓ copied")
	}
	generatedRows(t, res.Generated)
	if err := printWarnings(cmd, res.Generated); err != nil {
		return err
	}
	return t.Flush()
}

func addCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "add",
		Usage:     "Add repos to a space: a worktree on the space branch in each",
		ArgsUsage: "<space> <repo>...",
		Description: "The branch rules and the always.secrets are the same as for tdm new. A repo that is an\n" +
			"always.repos symlink in the space gets a worktree in place of the symlink.\n\n" +
			repoNameHelp + "\n\n" +
			"All checks run before the first change. When a step fails, the steps before it are undone.",
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			if cmd.Args().Len() == 0 {
				completeSpaces(cmd)
			}
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			args := cmd.Args().Slice()
			if len(args) < 2 {
				return errors.New("give a space name and at least one repo: tdm add <space> <repo>")
			}

			m, err := newManager(cmd)
			if err != nil {
				return err
			}

			res, err := m.AddRepos(ctx, args[0], args[1:])
			if err != nil {
				return err
			}
			return printNew(cmd, res)
		},
	}
}

func pathCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "path",
		Usage:     "Print the directory of a space (the tcd shell helper uses it)",
		ArgsUsage: "<space>",
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			completeSpaces(cmd)
		},
		Action: func(_ context.Context, cmd *ucli.Command) error {
			if cmd.Args().Len() != 1 {
				return errors.New("usage: tdm path <space>")
			}

			m, err := newManager(cmd)
			if err != nil {
				return err
			}

			sp, err := m.Space(cmd.Args().First())
			if err != nil {
				return err
			}

			if cmd.Bool("json") {
				return writeJSON(cmd.Root().Writer, pathOutput{Path: sp.Path})
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, sp.Path)
			return err
		},
	}
}

// completeSpaces prints one space name per line. A completion has no place to show an error.
func completeSpaces(cmd *ucli.Command) {
	m, err := newManager(cmd)
	if err != nil {
		return
	}

	names, err := m.SpaceNames()
	if err != nil {
		return
	}
	for _, n := range names {
		_, _ = fmt.Fprintln(cmd.Root().Writer, n)
	}
}

// generatedMark says what is on disk at a generated path, with the marks of generate.
func generatedMark(g space.GeneratedStatus) string {
	switch {
	case g.Err != nil:
		return "✗ " + g.Err.Error()
	case g.State == gen.Changed:
		return "! changed by a person"
	case g.State == gen.Gone:
		return "- gone"
	}
	return "✓ generated"
}

func statusCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "status",
		Usage:     "Show the repos of each space: branch, changes and commits that origin does not have (↑)",
		ArgsUsage: "[space...]",
		Description: "With no space, all spaces. The space that holds the current directory comes first.\n\n" +
			"Exit status 1 when a repo cannot be read.",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "long", Aliases: []string{"l"}, Usage: "also list the changed files and the generated files"},
		},
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			completeSpaces(cmd)
		},
		Action: spaceStatus,
	}
}

func spaceStatus(ctx context.Context, cmd *ucli.Command) error {
	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	list, err := m.Status(ctx, cmd.Args().Slice(), dir)
	if err != nil {
		return err
	}

	if cmd.Bool("json") {
		out, resultErr := jsonStatus(list, cmd.Bool("long"))
		return errors.Join(resultErr, writeJSON(cmd.Root().Writer, out))
	}

	t := ui.NewTable(cmd.Root().Writer)
	failed := false

	for _, s := range list {
		t.Row(s.Space.Name, s.Space.Path)
		for _, r := range s.Repos {
			switch {
			case r.Err != nil:
				failed = true
				t.Row("  "+r.Entry.Name, "["+s.Space.Branch+"]", "✗ "+r.Err.Error())
			case r.Entry.Symlink:
				t.Row("  "+r.Entry.Name, "→ main clone")
			default:
				branch := r.Status.Branch
				if branch == "" {
					branch = "detached"
				}
				t.Row("  "+r.Entry.Name, "["+branch+"]", memberChanges(r))
				if cmd.Bool("long") {
					for _, f := range r.Status.Changed {
						t.Row("      " + f)
					}
				}
			}
		}
		if cmd.Bool("long") {
			for _, g := range s.Generated {
				t.Row("  "+g.File.Path, generatedMark(g), "("+g.File.Generator+")")
			}
		}
	}

	if err := t.Flush(); err != nil {
		return err
	}
	if failed {
		return errReported
	}
	return nil
}

func memberChanges(r space.MemberStatus) string {
	s := "✓ clean"
	if !r.Status.Clean() {
		s = fmt.Sprintf("! %d changed", len(r.Status.Changed))
	}

	if r.Unpushed > 0 {
		s += fmt.Sprintf("  ↑%d", r.Unpushed)
	}
	if r.Status.Behind > 0 {
		s += fmt.Sprintf("  ↓%d", r.Status.Behind)
	}

	return s
}

func removeCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "remove",
		Usage:     "Take repos out of a space",
		ArgsUsage: "<space> <repo>...",
		Description: "When a worktree has changes or commits that origin does not have, tdm lists them and asks\n" +
			"first; with no answer (for example, stdin is not a terminal) nothing is removed.\n\n" +
			"A repo in always.repos gets its symlink back. A repo that is a symlink leaves the space.\n" +
			"A repo is its name in the space, or the last part of it when that is unique.",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{
				Name:    "delete-branch",
				Aliases: []string{"d"},
				Usage:   "also delete the space branch in each repo when it is merged (git branch -d)",
			},
		},
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			if cmd.Args().Len() == 0 {
				completeSpaces(cmd)
			}
		},
		Action: removeRepos,
	}
}

func removeRepos(ctx context.Context, cmd *ucli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return errors.New("give a space name and at least one repo: tdm remove <space> <repo>")
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	removed, generated, err := m.RemoveRepos(ctx, args[0], args[1:], space.RemoveOptions{
		DeleteBranch: cmd.Bool("delete-branch"),
		Confirm:      func(risks []space.Risk) bool { return confirm(cmd, risks) },
	})

	if cmd.Bool("json") {
		if err != nil && len(removed) == 0 { // nothing was done: the error alone is the result
			return err
		}
		out := removeOutput{Space: args[0], Repos: jsonRemoved(removed, cmd.Bool("delete-branch")), Generated: jsonGenerate(generated, false)}
		return errors.Join(err, printWarnings(cmd, generated), writeJSON(cmd.Root().Writer, out))
	}

	t := ui.NewTable(cmd.Root().Writer)
	removedRows(t, removed, cmd.Bool("delete-branch"))
	generatedRows(t, generated)
	return errors.Join(err, printWarnings(cmd, generated), t.Flush())
}

// removedRows writes one row for each repo that remove or delete took out.
func removedRows(t *ui.Table, removed []space.Removed, deleteBranch bool) {
	for _, r := range removed {
		what := "✓ removed"
		switch {
		case r.Entry.Symlink:
			what = "✓ symlink removed"
		case r.Relinked:
			what = "✓ removed, symlink back (always.repos)"
		}

		branch := ""
		if r.BranchErr != nil {
			branch = "! branch kept: " + r.BranchErr.Error()
		} else if deleteBranch && !r.Entry.Symlink {
			branch = "✓ branch deleted"
		}
		t.Row(r.Entry.Name, what, branch)
	}
}

func deleteCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "delete",
		Usage:     "Delete a space: its worktrees, symlinks, always.files copies, directory and state",
		ArgsUsage: "<space>",
		Description: "When a worktree has changes or commits that origin does not have, tdm lists them and asks\n" +
			"first; with no answer nothing is deleted. Branches stay unless -d or -D is given. The space\n" +
			"directory stays when it holds other files. The main clones are not touched.",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "delete-branch", Aliases: []string{"d"}, Usage: "also delete each space branch that is merged (git branch -d)"},
			&ucli.BoolFlag{Name: "force-delete-branch", Aliases: []string{"D"}, Usage: "also delete each space branch, merged or not (git branch -D)"},
		},
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			completeSpaces(cmd)
		},
		Action: deleteSpace,
	}
}

func deleteSpace(ctx context.Context, cmd *ucli.Command) error {
	if cmd.Args().Len() != 1 {
		return errors.New("give one space name: tdm delete <space>")
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	force := cmd.Bool("force-delete-branch")
	res, err := m.DeleteSpace(ctx, cmd.Args().First(), space.DeleteOptions{
		DeleteBranch: cmd.Bool("delete-branch"),
		ForceBranch:  force,
		Confirm:      func(risks []space.Risk) bool { return confirm(cmd, risks) },
	})

	if cmd.Bool("json") {
		// Nothing was done: the error alone is the result.
		if err != nil && (res.Space.Name == "" || errors.Is(err, space.ErrDeclined)) {
			return err
		}
		out := deleteOutput{
			Space:     jsonSpace(res.Space),
			Repos:     jsonRemoved(res.Repos, force || cmd.Bool("delete-branch")),
			Files:     append([]string{}, res.Files...),
			KeptFiles: jsonKept(res.KeptFiles),
			Generated: jsonChanges(res.Generated, false),
			KeptDir:   res.KeptDir,
		}
		return errors.Join(err, writeJSON(cmd.Root().Writer, out))
	}

	t := ui.NewTable(cmd.Root().Writer)
	removedRows(t, res.Repos, force || cmd.Bool("delete-branch"))
	for _, f := range res.Files {
		t.Row(f, "✓ removed")
	}
	for _, k := range res.KeptFiles {
		t.Row(k.Name, "! kept: not the same as "+k.Source)
	}
	for _, c := range res.Generated {
		switch c.Action {
		case gen.Delete:
			t.Row(c.Path, "✓ removed")
		case gen.Skip:
			t.Row(c.Path, "! kept: "+c.Reason)
		}
	}
	if err == nil {
		if res.KeptDir {
			t.Row(res.Space.Path, "! kept: it holds other files")
		}
		t.Row(res.Space.Name, "✓ deleted")
	}
	return errors.Join(err, t.Flush())
}

// confirm prints the risks to stderr and reads a yes from stdin. Only "y" or "yes" is a yes.
func confirm(cmd *ucli.Command, risks []space.Risk) bool {
	w := cmd.Root().ErrWriter
	t := ui.NewTable(w)
	for _, r := range risks {
		var parts []string
		if r.Changed > 0 {
			parts = append(parts, fmt.Sprintf("%d changed", r.Changed))
		}
		if r.Unpushed > 0 {
			parts = append(parts, fmt.Sprintf("%d not pushed", r.Unpushed))
		}
		t.Row("  "+r.Name, strings.Join(parts, ", "))
	}

	if _, err := fmt.Fprintln(w, "These worktrees have changes that will be lost, or commits that only the branch has:"); err != nil {
		return false
	}
	if t.Flush() != nil {
		return false
	}
	if _, err := fmt.Fprint(w, "Continue? [y/N] "); err != nil {
		return false
	}

	// A read error or EOF before an answer is a no; "y" at EOF without a newline is a yes.
	line, _ := bufio.NewReader(cmd.Root().Reader).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func execCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "exec",
		Usage:     "Run a command in each worktree of a space, one after the other",
		ArgsUsage: "<space> -- <command> [args...]",
		Description: "The command runs in each worktree in turn, after a '== <repo> ==' line. It does not run in\n" +
			"always.repos symlinks: they are the main clones. A failure does not stop the other repos;\n" +
			"the failures are listed at the end, and the exit status is 1.",
		ShellComplete: func(_ context.Context, cmd *ucli.Command) {
			if cmd.Args().Len() == 0 {
				completeSpaces(cmd)
			}
		},
		Action: execSpace,
	}
}

func execSpace(ctx context.Context, cmd *ucli.Command) error {
	if cmd.Bool("json") {
		return errors.New("--json is not supported by exec")
	}
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return errors.New("give a space and a command: tdm exec <space> -- <command> [args...]")
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	out, errOut := cmd.Root().Writer, cmd.Root().ErrWriter
	results, err := m.Exec(ctx, args[0], args[1:], out, errOut, func(e state.RepoEntry) {
		_, _ = fmt.Fprintf(out, "== %s ==\n", e.Name) // a broken stdout shows in the command's own output
	})
	if err != nil {
		return err
	}

	var failed []string
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", r.Entry.Name, r.Err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("failed in %d of %d repos: %s", len(failed), len(results), strings.Join(failed, ", "))
}
