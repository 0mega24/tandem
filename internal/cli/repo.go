package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/space"
	"github.com/0mega24/tandem/internal/state"
	"github.com/0mega24/tandem/internal/ui"
)

// errReported means the command already printed the details; Run only sets the exit code.
var errReported = errors.New("")

// newManager loads the config and builds the space operations for one command.
func newManager(cmd *ucli.Command) (*space.Manager, error) {
	loaded, env, err := loadGenerators(cmd)
	if err != nil {
		return nil, err
	}

	store := state.NewStore(state.DefaultDir(env.Home, env.XDGDataHome))
	m := space.New(loaded.Config, git.System{}, store)

	// stderr, so that the output of tdm stays clean for scripts.
	m.SetOutput(cmd.Root().ErrWriter)
	return m, nil
}

// repoArgsHelp is shared because all repo commands resolve names the same way.
const repoArgsHelp = "With no repo, all main clones under discovery.root_dir (symlinks are not followed).\n" +
	"A repo is its path under the root (myorg/api), or its last part (api) when that is unique."

func repoCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "repo",
		Usage: "Work on the main clones under discovery.root_dir",
		Commands: []*ucli.Command{
			{
				Name:        "status",
				Usage:       "Show branch, changes and ahead/behind of each main clone",
				Description: repoArgsHelp + "\n\nExit status 1 when a repo cannot be read.",
				ArgsUsage:   "[repo...]",
				Flags: []ucli.Flag{
					&ucli.BoolFlag{Name: "long", Aliases: []string{"l"}, Usage: "also show the origin URL and the path"},
				},
				Action: repoStatus,
			},
			{
				Name:  "sync",
				Usage: "Fetch and fast-forward the default branch of each main clone (worktrees are not touched)",
				Description: repoArgsHelp + "\n\n" +
					"A clone with local changes is skipped, and so is a default branch that is checked out\n" +
					"in a worktree. A skip is not an error: exit status 1 only when a repo fails, for example\n" +
					"when its branch and origin have diverged.",
				ArgsUsage: "[repo...]",
				Action:    repoSync,
			},
		},
	}
}

func repoStatus(ctx context.Context, cmd *ucli.Command) error {
	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	list, err := m.RepoStatus(ctx, cmd.Args().Slice())
	if err != nil {
		return err
	}

	t := ui.NewTable(cmd.Root().Writer)
	failed := false

	for _, r := range list {
		if r.Err != nil {
			failed = true
			t.Row(r.Repo.Name, "", "✗ "+r.Err.Error())
			continue
		}

		if cmd.Bool("long") {
			t.Row(r.Repo.Name, r.Status.Branch, changes(r.Status), r.Origin, r.Repo.Path)
		} else {
			t.Row(r.Repo.Name, r.Status.Branch, changes(r.Status))
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

// changes is the status column: clean or the change count, then ahead and behind.
func changes(st git.Status) string {
	s := "✓ clean"
	if !st.Clean() {
		s = fmt.Sprintf("! %d changed", len(st.Changed))
	}

	if st.Ahead > 0 {
		s += fmt.Sprintf("  ↑%d", st.Ahead)
	}
	if st.Behind > 0 {
		s += fmt.Sprintf("  ↓%d", st.Behind)
	}

	return s
}

func repoSync(ctx context.Context, cmd *ucli.Command) error {
	m, err := newManager(cmd)
	if err != nil {
		return err
	}

	results, err := m.SyncRepos(ctx, cmd.Args().Slice())
	if err != nil {
		return err
	}

	return printSync(cmd.Root().Writer, results)
}

func printSync(out io.Writer, results []space.SyncResult) error {
	t := ui.NewTable(out)
	failed := false

	for _, r := range results {
		mark := "✓"
		switch r.Outcome {
		case space.SyncSkippedDirty, space.SyncSkippedCheckedOut:
			mark = "!"
		case space.SyncFailed:
			mark, failed = "✗", true
		}

		result := mark + " " + r.Outcome.String()
		if r.Err != nil {
			result += ": " + r.Err.Error()
		}
		t.Row(r.Repo.Name, r.Branch, result)
	}

	if err := t.Flush(); err != nil {
		return err
	}
	if failed {
		return errReported
	}
	return nil
}
