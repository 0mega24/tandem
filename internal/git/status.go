package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Status is the state of one worktree.
type Status struct {
	Branch   string // empty when detached
	Upstream string // empty when there is no upstream
	Ahead    int
	Behind   int
	Changed  []string // all changed paths, untracked included
}

func (s Status) Clean() bool { return len(s.Changed) == 0 }

func (s System) Status(ctx context.Context, dir string) (Status, error) {
	out, err := s.run(ctx, dir, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(string(out))
}

func parseStatus(out string) (Status, error) {
	var st Status
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case f == "":
		case strings.HasPrefix(f, "# branch.head "):
			if head := strings.TrimPrefix(f, "# branch.head "); head != "(detached)" {
				st.Branch = head
			}
		case strings.HasPrefix(f, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(f, "# branch.upstream ")
		case strings.HasPrefix(f, "# branch.ab "):
			if _, err := fmt.Sscanf(f, "# branch.ab +%d -%d", &st.Ahead, &st.Behind); err != nil {
				return Status{}, fmt.Errorf("parse git status %q: %w", f, err)
			}
		case strings.HasPrefix(f, "#"):
			// other headers (branch.oid, stash) are not used
		default:
			path, extra, err := entryPath(f)
			if err != nil {
				return Status{}, err
			}
			st.Changed = append(st.Changed, path)
			i += extra // a rename or copy has its original path in the next field
		}
	}
	return st, nil
}

// entryPath returns the path of one entry and how many extra fields the entry uses.
//
// The path is the last of a fixed number of fields, so it may hold spaces.
func entryPath(f string) (path string, extra int, err error) {
	var n int
	switch f[0] {
	case '1':
		n = 9 // 1 XY sub mH mI mW hH hI path
	case '2':
		n, extra = 10, 1 // 2 XY sub mH mI mW hH hI Xscore path, then origPath
	case 'u':
		n = 11 // u XY sub m1 m2 m3 mW h1 h2 h3 path
	case '?', '!':
		n = 2 // ? path
	default:
		return "", 0, fmt.Errorf("parse git status: unknown entry %q", f)
	}
	parts := strings.SplitN(f, " ", n)
	if len(parts) != n {
		return "", 0, fmt.Errorf("parse git status: short entry %q", f)
	}
	return parts[n-1], extra, nil
}

// Unpushed counts the commits on branch that origin does not have. It is 0 for a repo with no
// origin: there is nowhere to push, and a count of all commits would look like local work.
// An empty branch means the detached HEAD of the worktree repo: its commits that no local branch
// and no origin branch has, which a worktree removal would lose.
func (s System) Unpushed(ctx context.Context, repo, branch string) (int, error) {
	url, err := s.OriginURL(ctx, repo)
	if err != nil || url == "" {
		return 0, err
	}

	args := []string{"rev-list", "--count", "refs/heads/" + branch, "--not", "--remotes=origin"}
	if branch == "" {
		args = []string{"rev-list", "--count", "HEAD", "--not", "--branches", "--remotes=origin"}
	}
	out, err := s.line(ctx, repo, args...)
	if err != nil {
		return 0, err
	}

	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parse git rev-list count %q: %w", out, err)
	}
	return n, nil
}
