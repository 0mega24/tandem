# JSON command output

Use `tdm --json <command>` to read results in scripts. The flag also works after
subcommands, for example `tdm status --json -l feat`.

Each result is one JSON object on stdout, with two-space indentation and a final
newline. Keys use snake_case. Empty lists are `[]`, never `null`. Strings contain
plain values, without terminal marks or colors. Consumers should accept new keys.
The format is a public API. Changes require a CHANGELOG entry.

Help (`--help`, `tdm help`, or a command group with no subcommand) is always text.

Errors give a nonzero exit code and one object on stderr:

```json
{
  "error": "space not found: missing"
}
```

Some operations can report partial results on stdout before an error. Status and
sync results include an `error` string on each failed repo. Check the exit code.
Warnings remain text on stderr. Generator results also include their warnings in
JSON. Manager warnings, such as state repairs, remain text only. Confirmations
for remove and delete still print to stderr and read stdin. Thus stderr can have
text before an error object when warnings or confirmations occur.

`space` objects have `name`, `path` (absolute directory), and `branch`. Repository
paths are absolute. Generated-file paths are relative to the space, with `/`.
Optional error strings and link targets are absent when not applicable.

## version

Keys: `version`, `commit`, `date`. Unavailable build fields are empty strings.

```json
{
  "version": "0.1.0",
  "commit": "abc123",
  "date": "2026-11-18T12:00:00Z"
}
```

## path

Key: `path`, the space directory.

```json
{
  "path": "/home/me/workspaces/feat"
}
```

## status

Key: `spaces`, in the same order as text output. Each item has `name`, `path`,
`branch`, and `repos`. Each repo has `name`, `repo_path`, `worktree_path`,
`symlink`, `branch`, `changed` (file paths), `ahead`, and `behind`. `ahead` counts
commits on the checked-out branch that origin does not have. A symlink has an
empty branch and change list, and zero counts. An unreadable repo has `error`.
A detached worktree has an empty branch.

With `-l` or `--long`, each space also has `generated`, a list with `path`,
`generator`, and `state` (`intact`, `changed`, or `gone`). A failed file check
also has `error`. State repairs can remove missing files before this list is
built. Without `-l`, `generated` is absent. Changed repo files are always listed.

```json
{
  "spaces": [
    {
      "name": "feat",
      "path": "/home/me/workspaces/feat",
      "branch": "feat",
      "repos": [
        {
          "name": "api",
          "repo_path": "/home/me/repos/api",
          "worktree_path": "/home/me/workspaces/feat/api",
          "symlink": false,
          "branch": "feat",
          "changed": ["a.go"],
          "ahead": 2,
          "behind": 0
        }
      ],
      "generated": [
        {
          "path": "go.work",
          "generator": "go",
          "state": "intact"
        }
      ]
    }
  ]
}
```

## repo status

Key: `repos`. Each item has `name`, `path`, `origin`, `branch`, `changed`, `ahead`,
`behind`, and optional `error`. The ahead and behind counts refer to the upstream
branch. Paths and origin URLs are included with or without `-l`.

```json
{
  "repos": [
    {
      "name": "api",
      "path": "/home/me/repos/api",
      "origin": "https://example.com/api.git",
      "branch": "main",
      "changed": [],
      "ahead": 0,
      "behind": 0
    }
  ]
}
```

## repo sync

Key: `repos`. Each item has `name`, `path`, `branch`, `outcome`, and optional
`error`. Outcomes are `up_to_date`, `updated`, `skipped_dirty`,
`skipped_checked_out`, or `failed`. A skip does not fail the command. A
`skipped_checked_out` row can have an error string that explains the skip.

```json
{
  "repos": [
    {
      "name": "api",
      "path": "/home/me/repos/api",
      "branch": "main",
      "outcome": "up_to_date"
    }
  ]
}
```

## new

Keys: `space`, `repos`, `files`, `generated`. The repos list reports the entries
made: `name`, `repo_path`, `worktree_path`, `symlink`, `from` (branch start point,
or empty for an existing branch), `secrets` (copied file names), and `was_link`.
`files` lists copies in the space root. `generated` has the generate shape below.

```json
{
  "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
  "repos": [
    {
      "name": "api",
      "repo_path": "/home/me/repos/api",
      "worktree_path": "/home/me/workspaces/feat/api",
      "symlink": false,
      "from": "main",
      "secrets": [],
      "was_link": false
    }
  ],
  "files": [],
  "generated": {
    "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
    "dry_run": false,
    "runs": [],
    "changes": [],
    "warnings": []
  }
}
```

## add

The keys and repo fields match `new`. `repos` contains only entries added or
replaced by this command. `generated` describes the resulting workspace files.

```json
{
  "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
  "repos": [],
  "files": [],
  "generated": {
    "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
    "dry_run": false,
    "runs": [],
    "changes": [],
    "warnings": []
  }
}
```

## remove

Keys: `space` (the requested name), `repos`, `generated`. Each removed repo has
`name`, `repo_path`, `worktree_path`, `symlink`, `relinked` (the always.repos link
was restored), `branch_deleted`, and optional `branch_error`. A branch deletion
failure leaves the branch and sets `branch_error`. `generated` has the generate
shape. If generation did not run, its space fields are empty strings.

```json
{
  "space": "feat",
  "repos": [
    {
      "name": "web",
      "repo_path": "/home/me/repos/web",
      "worktree_path": "/home/me/workspaces/feat/web",
      "symlink": false,
      "relinked": false,
      "branch_deleted": false
    }
  ],
  "generated": {
    "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
    "dry_run": false,
    "runs": [],
    "changes": [],
    "warnings": []
  }
}
```

## delete

Keys: `space`, `repos` (the removed repo shape above), `files` (removed root
copies), `kept_files` (files with an always.files name that differ from the
source, so they stay: `name`, `source`), `generated` (file changes), and
`kept_dir` (other files kept the space directory on disk). Generated changes have the fields described under generate.

```json
{
  "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
  "repos": [],
  "files": [],
  "kept_files": [],
  "generated": [],
  "kept_dir": false
}
```

## generate

Keys: `space`, `dry_run`, `runs`, `changes`, `warnings`.
Each run has `id`, `members` (detected member names), and `warnings`.
Each change has `generator`, `path`, `action` (`write`, `unchanged`, `skip`, or
`delete`), `reason`, and optional `target` for a link. `warnings` at the top
contains result warnings plus each run warning prefixed with its generator ID.

```json
{
  "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
  "dry_run": false,
  "runs": [{"id": "go", "members": ["api"], "warnings": []}],
  "changes": [{"generator": "go", "path": "go.work", "action": "write", "reason": ""}],
  "warnings": []
}
```

## generate --dry-run

The generate keys apply, with `dry_run: true`. Each change also has `diff`, the
plain unified diff text for a write or delete, or an empty string when there is
no content diff. No terminal colors are included and no files are written.

```json
{
  "space": {"name": "feat", "path": "/home/me/workspaces/feat", "branch": "feat"},
  "dry_run": true,
  "runs": [{"id": "go", "members": ["api"], "warnings": []}],
  "changes": [
    {
      "generator": "go",
      "path": "go.work",
      "action": "unchanged",
      "reason": "",
      "diff": ""
    }
  ],
  "warnings": []
}
```

## generate --list

Key: `generators`. Each item has `id`, `default_on`, `languages`, `manifests`,
and `source` (`built-in` or the plugin executable path).

```json
{
  "generators": [
    {
      "id": "go",
      "default_on": true,
      "languages": ["Go"],
      "manifests": ["go.mod"],
      "source": "built-in"
    }
  ]
}
```

## config

Keys: `layers`, in load order from lowest to highest priority, and `user_path`.
Each layer has `path` and `content` (the file's original text). This reports the
files read, rather than a merged config. With no config file, `layers` is empty.

```json
{
  "layers": [
    {"path": "/home/me/.config/tandem/config.toml", "content": "git.branch_prefix = \"me/\"\n"}
  ],
  "user_path": "/home/me/.config/tandem/config.toml"
}
```

## config path

Key: `path`, the user config file path, with the usual config flag and environment
precedence.

```json
{
  "path": "/home/me/.config/tandem/config.toml"
}
```

## config init

Key: `path`, the newly created config file. An existing file is an error.

```json
{
  "path": "/home/me/.config/tandem/config.toml"
}
```

## import wtg

Keys: `dry_run`, `rows`. Each row has `name`, `skipped`, and `reason`. A row can
refer to a space or the config. An empty source gives `rows: []`. In a dry run,
rows describe the proposed import and no files are written.

```json
{
  "dry_run": true,
  "rows": [{"name": "feat", "skipped": false, "reason": "would import /home/me/workspaces/feat"}]
}
```

## plugin check

Keys: `id`, `protocol`, `meta_passed`, `plan_passed`. A successful check sets both
booleans to true. Failed checks return a JSON error on stderr.

```json
{
  "id": "demo",
  "protocol": 1,
  "meta_passed": true,
  "plan_passed": true
}
```

## exec

JSON mode returns a nonzero exit code and this object on stderr, before running
any child command:

```json
{
  "error": "--json is not supported by exec"
}
```

## ui

JSON mode returns a nonzero exit code and this object on stderr, before opening
the terminal UI:

```json
{
  "error": "--json is not supported by ui"
}
```
