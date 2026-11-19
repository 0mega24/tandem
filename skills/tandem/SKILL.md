---
name: tandem
description: Use tdm (Tandem) to work on one feature branch across many git repos. Use when a task needs changes in more than one repo, when you must make, change, inspect or delete a space, run a command in each repo of a space, or write workspace files such as go.work.
---

# Use tdm

A space is one directory. It holds one git worktree for each repo, all on the same feature branch.
Tandem writes the workspace files (`go.work`, Cargo patches, pnpm and others) that connect them.
The worktrees share git branches and objects with the main clones. Worktree edits stay separate.

## Rules

- Run `tdm` in a shell. Do not run `tdm ui`: it is a terminal UI for people.
- Work in the worktrees of the space, not in the main clones.
- Run `tdm status` before you remove or delete. Commit and push first when a repo has work that origin does not have.
- `tdm remove` and `tdm delete` ask before they remove changed or unpushed work. Without a
  terminal, they refuse that removal; clean worktrees can still be removed. Do not pipe `yes`.
- Never pass `--force` or `-D` unless the person asks.

## Find a space

```sh
tdm status            # all spaces; the space of the current directory is first
tdm status -l feat    # also list the changed files
tdm path feat         # print the space directory
```
Use `cd "$(tdm path feat)"` to enter it.

## Make a space

```sh
tdm repo sync                   # optional: start new branches from origin
tdm new feat api web lib        # worktree on branch feat in each repo
tdm new --branch fix/x feat api # use another branch name
tdm new --no-generate feat api  # write no workspace files
```

- A repo is its path under `discovery.root_dir` (`myorg/api`), or its last part (`api`) when that is unique.
- The branch is `git.branch_prefix` plus the space name. A new branch starts from the local default branch. An existing branch is checked out as it is.
- `tdm new` and `tdm add` need at least one repo argument.
- If `new` or `add` fails, tdm undoes its completed steps. `remove` and `delete` can leave
  partial progress; they save the state so the command can be run again.

## Change the members

```sh
tdm add feat docs           # add a repo
tdm remove feat docs        # take a repo out; -d also deletes its merged branch
tdm generate feat           # write the workspace files again
tdm generate -n feat        # dry run: show a diff, write nothing
tdm generate --list         # list the generators
```
`tdm generate` skips a file that tdm did not write or that a person changed. `--force` can
replace it and keeps `<file>.tandem-bak`. If that backup already exists, the file is skipped.
For VS Code workspace files, tdm checks and updates only the `folders` key.

## Run a command in every repo

```sh
tdm exec feat -- git status -sb
tdm exec feat -- make test
```
The command runs in each worktree, one after the other, after a `== <repo> ==` line. One failure does not stop the rest. The exit status is 1 when any repo failed.

## Finish

```sh
tdm status feat               # check changes and unpushed commits
tdm delete feat               # remove worktrees and state; keep a nonempty space directory
tdm delete -d feat            # also delete merged space branches
```
`tdm delete` keeps the branches unless you pass `-d` (merged only) or `-D` (any). It keeps the main clones and their working files.

## More

- `tdm <command> --help` shows all flags.
- `tdm repo status` and `tdm repo sync` show and update the main clones. Sync skips a clone with local changes.
- Use `--config FILE` or `TDM_CONFIG` to add a config layer after the system and user files.
  `--config` takes priority over `TDM_CONFIG`.
