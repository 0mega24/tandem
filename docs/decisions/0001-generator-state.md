# 0001: Generator set and generated files in the space state

Status: accepted (2026-11-18)

## Context

The generators write workspace files (go.work, Cargo.toml, ...) in the space root. Later commands
must know which files tdm wrote, to write them again, to clean them up, and to not overwrite a
file that a person wrote or changed. A space also keeps its generator set: `tdm new --generate`
chooses it once, and `add` and `remove` use the same set. wtg kept only `go_workspace: true`.

## Decision

The state file gets two keys, as the generator spec says:

- `generators`: the generator IDs of the space. An empty set is `[none]`, as in the config:
  the key is `omitempty`, and a missing key means the default set.
- `generated`: one entry per file: `generator`, `path` (relative to the space root), `sha256` of
  the bytes that tdm wrote, and `stale` (derived files, such as go.work.sum, that go with it).
  A symlink that a plan makes (meson's subprojects/<dir>) has `target` in place of `sha256`: the
  link is tdm's when it still points there.

The hash is the ownership record. A file on disk with the same hash is tdm's; a different hash
means a person changed it, and tdm skips it unless `--force`. JSON files have no comment for an
ownership header, so the hash is their only record.

`go_workspace` is read, never written. When a state file has it and no `generated` list, tdm
makes the go.work entry from the file on disk, and writes the new format at the next save.

## Consequences

- Both keys are `omitempty`: a space with no generators has the same file as before.
- Older tdm versions with strict YAML decoding would reject the new keys. There is no release yet,
  so no installed version is affected.
- A hand edit of a generated file is found by its hash, not by its header, so a header that a
  person removed does not change ownership.
