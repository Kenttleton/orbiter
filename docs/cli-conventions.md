# CLI Conventions

How Orbiter commands are shaped. Follow this when adding a command or a flag so the
CLI stays predictable for people and for the clients (TUI, desktop app, third parties)
that drive it.

## Command shape

- **Operate commands** are verb-first with an optional `[target]`: `survey`, `chart`,
  `jump`, `scan`, `calibrate`, `retro`. The target is an alias name, or the entity for
  the current directory when omitted.
- **Build commands** are noun-first: `galaxy add`, `planet init`, `attach`.
- Explicit flags override inference, and flags cascade the hierarchy (the constitution's
  rule). Resolution order for anything configurable is flag, then environment variable,
  then default.

## Global flags: three independent axes

Each global flag answers one question. Never overload one flag to answer two.

| Flag | Env | Axis | Meaning |
| ---- | --- | ---- | ------- |
| `-v`, `--verbose` | `ORBITER_VERBOSE=1` | **Data volume** | Show more detail. Read commands add the bulky fields a table would otherwise hide (config, observations, timestamps). The meaning is contextual per command; a command's help says what `-v` adds. |
| `-t`, `--trace` | `ORBITER_TRACE=1` | **Process transparency** | Show how the work is done: plain operational labels instead of themed ones, and the underlying tools' raw output (git, nvm, uv, ...) streamed inline. Only meaningful for commands that run tools. For CI and debugging stalled steps. |
| `--output styled\|json` | `ORBITER_OUTPUT` | **Format** | Styled terminal output (default) or machine-readable JSON. |

**Reserved: `-i`, `--interactive`.** The third transparency axis: pass the Captain's
input through to the underlying tools (their own prompts). It is not registered yet
because no command needs it. Add it, with this table row, when one does. Do not use
`-i` or `-t` for anything else.

`-y`, `--yes` skips confirmation prompts on commands that have them.

## Scope flags

- `-a`, `--all` widens a command's scope to every descendant instead of the target and
  its direct children (the `ls -a`, `docker ps -a` sense). Scope is separate from detail:
  `-a` says how much of the tree, `-v` says how much per entity.

## Survey: the enumeration command

`survey` is how you enumerate. It never calls integrations, writes beacons, or computes
deltas (the constitution's contract).

```bash
orbiter survey vessel          # the vessel and its galaxies
orbiter survey acme            # a galaxy: its systems and the planets directly in it
orbiter survey vessel -a       # the whole hierarchy, every descendant
orbiter survey api -v          # adds config, beacon observations and timestamps
orbiter survey vessel -a -v    # everything, with detail
```

Hierarchy children: vessel to galaxies; galaxy to systems and the planets directly in it;
system to planets; planets have none. Attached resources, transponders and callsigns are
listed for each entity.

## JSON output

- Every command that supports `--output json` emits **event lines** (one JSON object per
  line: `type`, `txn`, `data`), never free-form text on stdout.
- A read command emits one `result` event whose `data` is `{success, message, data}`.
  `survey`'s `data` is a `SurveyNode` tree (`internal/commands/survey.go`).
- JSON **always carries full detail**, so `-v` does not change it. Scope flags (`-a`) do,
  because a client has to be able to ask for the whole subtree.
- `-t` and `-v` never add free-form text to JSON stdout.

## Adding to this document

New global flag, new short flag, or a flag whose meaning differs by command: add it here
in the same change. The point of this file is that the conventions are not rediscovered
by reading code.
