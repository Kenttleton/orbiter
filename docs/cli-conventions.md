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

## JSON output (`--output json`)

JSON mode is the contract for every client: the TUI, the desktop app, and third parties.
stdout carries **event lines only** (one JSON object per line: `type`, `txn`, `data`),
never free-form text. A command ends in exactly one terminal event:

| Terminal event | When |
| -------------- | ---- |
| `result` | The command finished. `data` is `{success, message, data}`; the inner `data` is the command's own shape (table below). |
| `plan` | `chart` (always, an empty list when there is nothing to chart). |
| `error` | The command failed. `data` is `{message}`. The process exits 1 and stderr stays empty. |
| `captain_prompt` | A command needs the Captain's answer (async): relay the response, then run `uplink`. |

Detail flags: JSON **always carries full detail**, so `-v` does not change it. Scope flags
(`-a`) do, because a client has to be able to ask for the whole subtree. `-t` never adds
free-form text to JSON stdout.

### Result shapes

Types live in `internal/commands/results.go` and `internal/commands/survey.go`.

| Command | Result `data` |
| ------- | ------------- |
| `galaxy/system/planet/callsign/transponder/resource add` | `EntityResult`: `type, id, name`, plus `galaxy`, `system`, `role`, `brand` where they apply |
| `... init` | `EntityResult` with `status` (beacon); `planet init --discover` adds `discovered: [{role, brand, outcome, error?}]` |
| `attach` | `AttachResult`: `id, from, to` |
| `survey` | `SurveyNode` tree (see below) |
| `scan` | `ScanResult`: `target, target_id, items: [{kind, id, role, brand, status, report}]` where `report` is the integration's full `StateReport` |
| `calibrate` | `CalibrateResult`: `target, target_id, items: [{kind, id, role, brand, action, before?, after}]` |
| `chart` | `plan` event: `[{action, entity_type, name, description}]` |
| `retro` | `RetroResult`: `target, retired, nodes: [{name, action}]`. Pass `-y`: a JSON client cannot answer the terminal prompt |
| `quarantine`, `unquarantine`, `vessel unquarantine` | `QuarantineResult`: `brand, quarantined, changed` |
| `vessel inspect`, `survey <brand>` | `InspectResult`: `brand, status (quarantined\|active\|not_installed), quarantined, reason?, since?` |
| `vessel init -y` | `VesselInitResult`: `installed, removed` |

### Survey

`survey`'s `data` is a `SurveyNode` tree: `type, id, name, status, role, brand, manages,
config, observations, verified_at`, the entities attached directly (`resources`,
`transponders`, `callsigns`) and `children` (one level deep, or the whole subtree with `-a`).

### Discovery for clients that are not a shell

When `calibrate` (or the `uplink` that continues it) cannot proceed until something is
found, it ends with a `result` whose data is a `PendingDiscovery`:
`{status: "needs_discovery", session, requests: [{txn, op, key, value?}]}`. The client
answers each request itself (`op` is `FIND_BINARY`, `FIND_FILE`, `FIND_DIR` or `FIND_ENV`;
the reply schema is the FIND spec's section 2) with
`orbiter relay --txn=<txn> --response=<json>`, then runs `orbiter uplink --session=<session>`.
A shell wrapper does the same through the `FIND_*` functions in its hook. A prompt the
Captain must answer arrives as a `captain_prompt` event and is answered the same way.

### Exempt commands

These emit scripts or the shell line protocol by design, and the shell executes the output:
`completions`, `shell init`, `hook`, `jump`. `init` (the terminal checklist) needs a
terminal; use `vessel init -y`. `relay` prints nothing: success is exit 0. `uplink` emits
the events above. `starchart` is the TUI entry point.

## Adding to this document

New global flag, new short flag, or a flag whose meaning differs by command: add it here
in the same change. The point of this file is that the conventions are not rediscovered
by reading code.
