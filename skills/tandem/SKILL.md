---
name: tandem
description: Use tdm (Tandem) to work on one feature branch across many git repos. Use when a task needs changes in more than one repo, when you must make, change, inspect or delete a space, run a command in each repo of a space, or write workspace files such as go.work.
---

# Use tdm

A space is one directory. It holds one git worktree for each repo, all on the same feature branch.
Tandem writes the workspace files (`go.work`, Cargo patches, pnpm and others) that connect them.
The main clones are not changed.

## Rules
- Run `tdm` in a shell. Do not run `tdm ui`: it is a terminal UI for people.
- Work in the worktrees of the space, not in the main clones.
- Run `tdm status` before you remove or delete. Commit and push first when a repo has work that origin does not have.
- `tdm remove` and `tdm delete` ask before they drop work. With no terminal they drop nothing. Do not pipe `yes` to them.
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
- With no repo, `tdm new` and `tdm add` use all main clones.
- When a step fails, tdm undoes the steps before it.

## Change the members
```sh
tdm add feat docs           # add a repo
tdm remove feat docs        # take a repo out; -d also deletes its merged branch
tdm generate feat           # write the workspace files again
tdm generate -n feat        # dry run: show a diff, write nothing
tdm generate --list         # list the generators
```
`tdm generate` skips a file that tdm did not write or that a person changed. `--force` replaces it and keeps `<file>.tandem-bak`.

## Run a command in every repo
```sh
tdm exec feat -- git status -sb
tdm exec feat -- make test
```
The command runs in each worktree, one after the other, after a `== <repo> ==` line. One failure does not stop the rest. The exit status is 1 when any repo failed.

## Finish
```sh
tdm status feat               # check for changes and for commits that origin does not have (↑)
tdm delete feat               # delete worktrees, symlinks, directory and state
tdm delete -d feat            # also delete merged space branches
```
`tdm delete` keeps the branches unless you pass `-d` (merged only) or `-D` (any). It does not touch the main clones.

## More
- `tdm <command> --help` shows all flags.
- `tdm repo status` and `tdm repo sync` show and update the main clones. Sync skips a clone with local changes.
- Use `--config FILE` or `TDM_CONFIG` to read one config file in place of the others.
