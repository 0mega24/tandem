package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/space"
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
			repoArgsHelp + "\n\n" +
			"All checks run before the first change. When a step fails, the steps before it are undone.",
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "branch", Usage: "use branch `NAME` in place of git.branch_prefix + <space>"},
			&ucli.StringFlag{Name: "path", Usage: "make the space in `DIR` in place of spaces.root_dir/<space>", TakesFile: true},
		},
		Action: newSpace,
	}
}

func newSpace(ctx context.Context, cmd *ucli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return errors.New("give a space name and at least one repo: tdm new <space> <repo>")
	}

	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	res, err := m.NewSpace(ctx, space.NewOptions{
		Name:   args[0],
		Repos:  args[1:],
		Branch: cmd.String("branch"),
		Path:   cmd.String("path"),
	})
	if err != nil {
		return err
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
		secrets := ""
		if len(r.Secrets) > 0 {
			secrets = "+ " + strings.Join(r.Secrets, ", ")
		}
		t.Row("  "+r.Entry.Name, "✓ worktree", "["+res.Space.Branch+"]", how, secrets)
	}
	for _, f := range res.Files {
		t.Row("  "+f, "✓ copied")
	}
	return t.Flush()
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

func statusCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "status",
		Usage:     "Show the repos of each space: branch, changes and commits that origin does not have (↑)",
		ArgsUsage: "[space...]",
		Description: "With no space, all spaces. The space that holds the current directory comes first.\n\n" +
			"Exit status 1 when a repo cannot be read.",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "long", Aliases: []string{"l"}, Usage: "also list the changed files"},
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
