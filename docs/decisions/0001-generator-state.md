# 0001: Generator set and generated files in the space state

Status: accepted (2026-11-18)

## Context

A space is a folder with a git worktree for each selected repo, all on one branch.
Generators write workspace files such as go.work and Cargo.toml, or plan links for Meson.
Later commands must know which files tdm wrote. They need this record to update or remove
the files and to protect files that a person wrote or changed. A space also keeps its generator
set: `tdm new --generate` chooses it once, and `add` and `remove` use the same set. The legacy
state format used `go_workspace: true` for Go files.

## Decision

The state file gets two keys, as the generator spec says:

- `generators`: the generator IDs of the space. An empty set is stored as `["none"]`, as in the config:
  the key is `omitempty`, and a missing key means the default set.
- `generated`: one entry per file: `generator`, `path` (relative to the space root), `sha256` of
  the bytes that tdm wrote, and `stale` (derived files, such as go.work.sum, that go with it).
  A symlink that a plan makes (meson's subprojects/<dir>) has `target` and an empty `sha256`: the
  link is tdm's when it still points there.
  A JSON file that tdm shares with a person (vscode's .code-workspace) has `key`: the one
  top-level key that tdm owns. Its `sha256` is of that key's value (compact JSON), so a person may
  change the other keys.

The hash is the ownership record. A file on disk with the same hash is tdm's; a different hash
means a person changed it, and tdm skips it unless `--force`. JSON files have no comment for an
ownership header, so the hash is their only record.

`go_workspace` is read, never written. When a state file has it and no `generated` list, tdm
makes the go.work entry from the file on disk, and writes the new format at the next save.

## Consequences

- Both keys use `omitempty`. A missing `generators` key selects the defaults. An explicit
  `["none"]` keeps all generators off, so it must be stored even when no files are generated.
- State readers that reject unknown YAML keys would reject the new keys. Tandem accepts
  unknown state keys, but older readers do not know how to manage the new generated files.
- The hash covers the whole file, including any generated header. Removing the header changes
  the hash, so tdm skips the file. A header alone does not prove that tdm wrote the file.
