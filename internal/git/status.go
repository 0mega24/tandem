package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Status is the state of one worktree, from `git status --porcelain=v2 --branch -z`.
type Status struct {
	Branch   string   // empty when detached
	Upstream string   // empty when the branch has no upstream
	Ahead    int      // commits on Branch that are not on Upstream
	Behind   int      // commits on Upstream that are not on Branch
	Changed  []string // changed, staged, conflicted and untracked paths
}

// Clean reports whether the worktree has no changes and no untracked files.
func (s Status) Clean() bool { return len(s.Changed) == 0 }

// Status reads the status of the worktree at dir.
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

// entryPath returns the path of one status entry and how many extra fields the entry uses.
// The path is the last of a fixed number of space-separated fields, so it may hold spaces.
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

// Unpushed counts the commits on branch that are on no origin remote-tracking branch. A branch
// that was never pushed counts all its commits that origin does not have.
func (s System) Unpushed(ctx context.Context, repo, branch string) (int, error) {
	out, err := s.line(ctx, repo, "rev-list", "--count", "refs/heads/"+branch, "--not", "--remotes=origin")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parse git rev-list count %q: %w", out, err)
	}
	return n, nil
}
