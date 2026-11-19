# 0006. JSON command output

Status: accepted (2026-11-18)

## Context

Scripts and agents need command results without parsing terminal tables and marks.
Internal space and generator types can change independently of a public format.

## Decision

- A global `--json` flag selects one JSON result object on stdout. Objects use
  snake_case keys, two-space indentation, a final newline, and empty lists rather
  than null. The command formats are documented in `docs/json.md`.
- CLI output structs in `internal/cli/json.go` define the format. Internal space
  and generator types have no output-format tags. Conversion uses plain functions.
- Errors are JSON objects on stderr and keep a nonzero exit code. Partial results
  can accompany an error. Warnings and confirmations retain their text on stderr.
  Generator results also include their existing warnings as lists.
- Exec and the terminal UI reject JSON mode. Remove and delete keep their current
  stdin confirmations. No new confirmation flag is added.
- This format is a public API. A format change requires a CHANGELOG entry.

## Consequences

- Text output remains the default, with the same command behavior.
- CLI converters must keep empty slices non-null and preserve result details.
- Consumers must check exit codes and accept additional object keys. Stderr can
  include text warnings or confirmation prompts before a JSON error object.
