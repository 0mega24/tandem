# 0003. Generator plugin protocol 1

Status: accepted (2026-11-18)

## Context

The spec (project.md, Plugins) lets a person add a generator without a Go build: an executable
`tdm-gen-<id>`. The protocol is a public contract. A change breaks the plugins that people wrote,
so it gets a version and a record.

## Decision

- Discovery: `[generate] plugin_dirs` in order, then `$XDG_DATA_HOME/tandem/plugins`, then `PATH`.
  The first executable for an ID wins; an ID of a built-in generator is an error.
- `tdm-gen-<id> meta` and `tdm-gen-<id> plan`: one JSON object on stdout. `plan` gets the space
  on stdin. The answer has `protocol: 1`; an unknown version is an error that names the known
  versions. Unknown and missing fields are errors.
- A plugin returns a plan; tdm checks the paths and links and is the only writer, as for the
  built-in generators.
- Limits: 60 s per call and 16 MiB of stdout. Each stderr line gets the prefix `<id>: `.
- `tdm plugin check <id>` runs `meta`, then `plan` on a temporary space.

The schemas are in docs/plugins.md.

## Consequences

- A new field or rule that an old plugin cannot satisfy needs protocol 2; tdm then accepts both.
- A plugin runs with the rights of the person. tdm trusts it as it trusts any program on `PATH`.
