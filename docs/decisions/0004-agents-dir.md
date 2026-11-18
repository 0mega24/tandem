# 0004. The .agents directory for the Codex marketplace

Status: accepted (2026-11-18)

## Context

The repo ships agent skills as a plugin. Codex reads a repo marketplace only from
`.agents/plugins/marketplace.json`. Claude Code reads `.claude-plugin/marketplace.json`. The
locked tree had no place for the Codex file.

## Decision

- Add `.agents/` to the locked tree. It holds only `plugins/marketplace.json`, which lists the
  `tandem` plugin with the repo root as its source.
- The Codex manifest stays in `.codex-plugin/plugin.json`, which Codex still supports.

## Consequences

- `codex plugin marketplace add 0mega24/tandem` installs the skills.
- Codex can move to a root `plugin.json`. Then change `.codex-plugin/` and this record.
- Nothing else goes in `.agents/`.
