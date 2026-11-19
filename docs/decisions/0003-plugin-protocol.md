# 0003. Generator plugin protocol 1

Status: accepted (2026-11-18)

## Context

The plugin section of `docs/spec/project.md` lets a person add a generator without rebuilding
Tandem. The generator is an executable named `tdm-gen-<id>`. The protocol is a public contract.
A change can break existing plugins, so the protocol has a version and a decision record.

## Decision

- Discovery: `[generate] plugin_dirs` in order, then `$XDG_DATA_HOME/tandem/plugins`, then `PATH`.
  The first executable for an ID wins. An ID that matches a built-in generator causes a warning;
  Tandem keeps the built-in generator.
- `tdm-gen-<id> meta` and `tdm-gen-<id> plan`: one JSON object on stdout. `plan` gets the space
  on stdin. The `meta` answer has `protocol: 1`; an unknown version is an error that lists the
  supported versions. The plan request and answer have no version field. Unknown fields and
  missing required fields are errors.
- A plugin returns a plan; tdm checks the paths and links and is the only writer, as for the
  built-in generators.
- Limits: 60 s per call and 16 MiB of stdout. Each stderr line gets the prefix `<id>: `.
- A space is a folder with a worktree for each selected repo, all on one branch.
  `tdm plugin check <id>` runs `meta`, then `plan` on a temporary space with one empty member.
  It checks the protocol and paths, not whether the plugin builds a project.

The schemas are in docs/plugins.md.

## Consequences

- A new field or rule that an old plugin cannot satisfy needs protocol 2; tdm then accepts both.
- A plugin runs with the rights of the person. tdm trusts it as it trusts any program on `PATH`.
