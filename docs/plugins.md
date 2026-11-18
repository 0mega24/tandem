# External generator plugins

A plugin is an executable named `tdm-gen-<id>`. IDs match `[a-z][a-z0-9-]*`, except
`none`. An ID must not match a built-in generator. Run `tdm generate --list` to see
all generators, their defaults, and their sources.

## Install

Put the executable in one of these places, in search order:

1. A directory in `[generate] plugin_dirs`, in list order.
2. `$XDG_DATA_HOME/tandem/plugins` (default: `~/.local/share/tandem/plugins`).
3. A directory on `PATH`, in path order.

The first executable for each ID wins. Non-executable files and empty `PATH`
entries are ignored. If the first match has the ID of a built-in generator,
Tandem prints a warning and uses the built-in. If the first match gives a bad
`meta` answer, a command that uses the plugin fails. Tandem does not try later
matches. `plugin_dirs` entries must be absolute paths or start with `~/`.

```toml
[generate]
plugin_dirs = ["~/my-plugins"]
enabled = ["go", "zig"]

[generate.zig]
# Plugin settings are sent as the request's config object.
```

Copy `examples/plugins/tdm-gen-zig` or `examples/plugins/tdm-gen-bundler` into a
plugin directory and set the executable bit. Both examples need Python 3. The Zig
entry point is a POSIX shell script; it uses Python's standard JSON codec for
paths with quotes, backslashes, or non-ASCII text. The Bundler example uses only
Python's standard library.

Run `tdm plugin check zig` after installation. This runs `meta` once, then `plan`
on a temporary space with one empty member directory. It reports each answer
that passes the schema and path checks, then removes the test space. It checks
the protocol, not whether the plugin can build a real project.

## Protocol 1

Tandem invokes the executable with one argument, `meta` or `plan`. Stdout must
contain exactly one JSON object. No banners, logging, or trailing objects are
allowed. Unknown fields, missing required fields, null values where the schemas
require another type, and unknown protocol versions are errors. Errors name the
supported versions: `1`.

`meta` receives no input. Tandem calls it at most once per command, and only when the
command needs the plugin: `generate --list`, `plugin check`, a plan, or the default
generator set. `status`, `path` and `exec` run no plugin. A failed `meta` makes the
plugin off by default, and a command that uses it fails with that error.
It describes the generator; `default_on` controls selection when
`generate.enabled` is unset. `config_schema` describes settings for plugin
consumers. The plugin must validate its own settings; Tandem does not interpret
arbitrary configuration schemas.

`plan` receives the request below on stdin. All member worktree paths are
absolute. `origin` is an empty string when no origin is set. `config` is the
plugin's `[generate.<id>]` table, or an empty object. All members are sent; the
plugin decides which manifests apply.

File `content` is a UTF-8 JSON string, not base64. The example Zig plugin writes
`.tandem/zig/forks.args`, with one `--fork=<relative-worktree>` line for each
member that has `build.zig.zon`. The Bundler plugin writes
`.tandem/bundler/env`, with shell-quoted `BUNDLE_LOCAL__<GEM>` exports for member
gemspecs. Source that file before running Bundler. Bundler's branch checks still
apply.

## Limits and path rules

- Each process has a 60-second limit and a 16-MiB stdout limit. Tandem terminates
  the process group of the plugin when it exceeds a limit, so the programs that a
  script starts stop too. A descendant cannot hold output pipes open
  indefinitely after the process ends.
- Stderr is streamed to the user. Each line starts with `<id>: `, including a
  final line without a newline. Stderr does not count toward the stdout limit.
- A nonzero exit, invalid JSON, or an invalid plan fails generation. In a space
  operation, the saga rolls back preceding steps.
- A plugin may read member files. It must not write files. Tandem applies plans
  through its ownership and rollback rules. Plugins are trusted executables;
  this protocol does not provide a filesystem sandbox.
- File and stale paths must be clean paths relative to the space root, use `/`,
  contain no `..`, and stay outside member worktrees. They must not pass through
  symlinks. Files and links must have distinct destination paths.
- A link may only be a direct `<member>/subprojects/<name>` entry whose target is
  another member worktree. The target is relative to the directory of the link, as
  `ln -s` takes it: `../../lib` for `app/subprojects/lib`. This is the Meson
  exception to the member-write rule, with the same rules as the built-in `meson`
  generator: tdm makes the link only where nothing is and only where git ignores
  the path in that member.

## JSON Schemas

These Draft 2020-12 schemas define the three message shapes. Additional path
checks above are required because JSON Schema cannot inspect the filesystem.
The meta ID must also match the executable suffix.

### Meta answer

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["protocol", "id", "languages", "manifests", "default_on", "config_schema"],
  "properties": {
    "protocol": {"const": 1, "type": "integer"},
    "id": {"type": "string", "pattern": "^[a-z][a-z0-9-]*$", "not": {"const": "none"}},
    "languages": {"type": "array", "items": {"type": "string"}},
    "manifests": {"type": "array", "items": {"type": "string"}},
    "default_on": {"type": "boolean"},
    "config_schema": {"type": "object"}
  }
}
```

### Plan request

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["space", "members", "config"],
  "properties": {
    "space": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "root", "branch"],
      "properties": {
        "name": {"type": "string"},
        "root": {"type": "string"},
        "branch": {"type": "string"}
      }
    },
    "members": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["name", "worktree", "origin"],
        "properties": {
          "name": {"type": "string"},
          "worktree": {"type": "string"},
          "origin": {"type": "string"}
        }
      }
    },
    "config": {"type": "object"}
  }
}
```

### Plan answer

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["files", "stale", "links", "warnings"],
  "properties": {
    "files": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["path", "content"],
        "properties": {
          "path": {"type": "string", "minLength": 1},
          "content": {"type": "string"}
        }
      }
    },
    "stale": {"type": "array", "items": {"type": "string", "minLength": 1}},
    "links": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["path", "target"],
        "properties": {
          "path": {"type": "string", "minLength": 1},
          "target": {"type": "string", "minLength": 1}
        }
      }
    },
    "warnings": {"type": "array", "items": {"type": "string"}}
  }
}
```
