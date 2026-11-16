# Contracts

Orbiter performs operations for integrations, by domain and brand, and runs them on a Captain's machine with trust. Three contracts make that possible. They are one family: each says what one party may ask of another and what the other may do, so Orbiter can hold every party to it.

| Contract | Between | Says |
|---|---|---|
| [Manifest](#1-the-manifest-contract) | Orbiter and an integration | What an integration is, what it needs to run, what it asks Orbiter to do, and what it claims. The claims are also security contracts |
| [Op](#2-the-op-contract) | Orbiter and whoever asks it for a capability | The operations Orbiter can perform (PATH, FIND, directives, prompts) and how requests and answers are shaped |
| [ABI](#3-the-abi-contract) | The host (Orbiter) and a guest (an integration's WASM) | The messages that cross the sandbox boundary: exports, host functions, input and output shapes, limits |

How each reads against the others: the manifest says what a guest may ask for, the ABI is how a guest asks, and the op contract is what Orbiter does with the request. Orbiter enforces all three at runtime. The Captain reads the manifest to decide whether to trust an integration; transparency is what makes that possible. See [security.md](security.md) for the trust model and the sandbox.

This page documents what the code does today. A **Gap** names something the contract does not say or the code does not do. A **Direction** records a decision from the Captain that the code does not yet follow; the plans (`docs/superpowers/plans/2026-10-27-*.md`) carry the work. Nothing here is a proposal.

## Platform facts

- Orbiter knows the OS for certain only because of Go.
- Orbiter knows the shell only through the shell integration, as a byproduct of setting the shell in the vessel layer.
- Paths: the internal filesystem parser holds both path types (`Path{Unix, Windows}` in `internal/integrations/types.go`) and uses the native one. A path is stored in forward-slash form so the star chart is portable, and parsed for the current host on every read.

---

## 1. The manifest contract

A manifest is `manifest.toml` in the integration's folder (`~/.orbiter/integrations/<brand>/`). Every section is optional; Orbiter applies the defaults below. An integration whose manifest brand differs from its folder name is skipped at load, so a folder cannot impersonate another brand.

The Captain's grouping of what a manifest says: metadata claims, internal resources for the runtime, external resources for operations, and health and detection. The sections below are today's layout, shown against that grouping; the layout is under review (`docs/superpowers/plans/2026-10-27-manifest-design.md`).

### `[integration]` (metadata; `binaries` is an operation request)

| Field | Meaning |
|---|---|
| `brand` | The integration's identity. Must equal the folder name |
| `name`, `description` | Human-readable |
| `roles` | Roles it serves. Orbiter infers resource versus transponder from its static role taxonomy |
| `binaries` | Binary names Orbiter finds on the integration's behalf (FIND_BINARY). Each must be a plain file name (`[A-Za-z0-9._+-]+`) |

### `[detection]` (health and detection)

| Field | Meaning |
|---|---|
| `files` | File names whose presence suggests the integration applies |
| `env` | Rules `{key, pattern}`: the variable is set and, when `pattern` is given, contains it |

With no rules the integration is always asked. Detection pre-filters before the guest's `detect` runs.

### `[dependencies]` (operations)

`resources` and `transponders` map a role to the brands the integration accepts (empty list means any). The star chart crawl reads them to decide what an integration's call can see.

### `[commands]` (operations and runtime)

| Field | Meaning |
|---|---|
| `allowed` | Entries `{cmd, description}`: every executable the guest may run through `run_command`. The description is shown to the Captain |
| `timeout_seconds` | Longest a command may run. When absent, 5 minutes |
| `env` | The environment variables a command receives, PATH included when it needs it, and nothing else. A name ending `*` claims every variable with that prefix |

Orbiter enforces these at `run_command` (section 3). A command outside `allowed` quarantines the integration.

### `[shell]` (operations: writes to the Captain's shell)

`exports` entries each declare either a `hook` (a file name in the integration folder) or `envs` (variable names the integration may set through `StateReport.Exports`). `description` is shown to the Captain. `sensitive` is meant to mark secrets.

### `[runtime]` (runtime)

| Field | Meaning | Default |
|---|---|---|
| `engine` | `interpreter` or `compiler`. Both enforce the same sandbox; they differ in when the cost is paid | `interpreter` |
| `memory_limit` | Most linear memory the guest may have, in bytes. Rounds up to a 64 KiB page | 64 MiB; at least one page, at most 1 GiB |
| `pool_size` | Instances allowed at once | 1 |
| `input_buffer_kb`, `output_buffer_kb` | Guest buffer size hints | 8 |

An unknown engine or an out-of-range `memory_limit` refuses the integration at load.

### `[find]` and `[config]`

`[find]` (`files` globs without spaces, `dirs`, `envs`) declares discovery beyond binaries. `[config]` declares config fields (`key`, `type`, `required`, `description`) for guided setup.

### What is enforced, and what is only declared

| Claim | Enforced by |
|---|---|
| brand equals folder | `LoadInstalled` (skip) |
| engine, memory | `Load` (refuse), wazero (memory ceiling) |
| `commands.allowed`, `env`, `timeout_seconds` | the `run_command` gate |
| `shell.exports` env names | `FilterExports` on every `StateReport` |
| `dependencies` | the crawl, which filters the guest's context |
| `detection` | the discovery pre-filter |
| `pool_size` | the instance slots |

**Gaps.** `sensitive` is read by nothing. `[config]` is read by nothing and no bundled manifest declares it. `[find]` is planned for but no bundled manifest declares it, and only binaries are requested. The buffer hints are read by nothing. No manifest carries a format version, so Orbiter cannot read an old manifest after the syntax changes. Nothing marks a command as the health check. Nothing declares paths for both platforms. A hook is a file name inside a list of environment exports, not a declared artifact.

**Direction.** The manifest is redesigned as a whole, with explicit, verbose claims: environment read and write claims, hook declarations with a size cap, and both-platform paths. The Captain's duty to read a manifest on clone is implicit and expected.

---

## 2. The op contract

An op is something Orbiter can do, asked for by an integration or by Orbiter itself. Ops reach the Captain's shell as **directives**, and reach the Captain as **prompts**.

### Directives

A `Directive` (`internal/commands/executor.go`) is a line Orbiter prints, one per line, that the shell hook interprets. Anything else Orbiter prints passes through as ordinary output.

| Op | Line | Effect |
|---|---|---|
| `DIR` | `DIR <path>` | change directory |
| `SET` | `SET KEY=VALUE` | export a variable (a newline in a value is escaped as `\n`) |
| `UNSET` | `UNSET KEY` | unset a variable |
| `DEPART` | `DEPART` | undo the exports of the planet being left |
| `SESSION` | `SESSION <id>` | the session this run opened; the hook calls `orbiter uplink` after answering FINDs |
| `FIND_BINARY` | `FIND_BINARY <txn> <name>` | find where a binary is |
| `FIND_FILE` | `FIND_FILE <txn> <pattern> [location]` | find files matching a glob, under a location (default: the working directory) |
| `FIND_DIR` | `FIND_DIR <txn> <path>` | does a directory exist |
| `FIND_ENV` | `FIND_ENV <txn> <name>` | read a variable |

A directive whose `SESSION` or `FIND_*` fields contain a line break is dropped, because it would smuggle in a second directive.

### FIND

FIND is how Orbiter learns where things are for the Captain's shell, and the hook contract defines how a shell answers. A FIND can come from an integration (its own binary, or a file or directory tied to it), from Orbiter, or be a raw request. The answer goes to whoever asked.

**Request.** `FindRequest{Op, Key, Value}`. Before each FIND directive Orbiter writes a transaction (`pending`) tagged with its session and arguments.

**Answer.** The shell function sends one JSON object to `orbiter relay --txn=<id>` (JSON from `--response` or stdin). Relay stores it and marks the transaction `relayed`.

```json
{ "status": "ok" | "not_found" | "error", "message": "...",
  "matches": ["/usr/bin/git", "..."], "path": "...", "exists": true, "value": "..." }
```

- `FIND_BINARY` and `FIND_FILE` answer with `matches` (every candidate, duplicates removed).
- `FIND_DIR` answers with `path` and `exists`.
- `FIND_ENV` answers with `value` (empty means unset).
- `ok` and `not_found` both succeed; `error` is an error and any other status is an error. Every path in `matches` is parsed into a `Path` and normalized; a path that does not parse is an error.

**What Orbiter does with the answer.**
- `FIND_BINARY` with exactly one match records the path in the resource's config (`binaries.<name>`) and passes it to the integration as `ResolvedContext.Binaries[name]`.
- More than one match is ambiguous and goes to the Captain (a `SELECT` prompt).
- A found path that differs from a recorded path never replaces it without the Captain's confirmation (a `CONFIRM` prompt).
- `FIND_FILE`, `FIND_DIR` and `FIND_ENV` are carried in the context (`Files`, `Dirs`, `Envs`) but the lifecycle does not yet request or persist them.
- Whether a recorded binary is still present is the integration's own report (`StateReport.Present`); Orbiter does not second-guess it.

`orbiter uplink --session=<id>` processes the relayed transactions, asks the Captain where needed, then resumes the interrupted operation.

### Prompts

Prompts are how Orbiter asks the Captain. They share the transaction mechanism.

| Op | Response |
|---|---|
| `SELECT` | `{"selected": "..."}`, or an array for multi |
| `CONFIRM` | `{"confirmed": true\|false}` |
| `TEXT` | `{"value": "..."}` (`variant`: short, long, obscured) |

In styled mode Orbiter reads the answer from stdin. In JSON mode the client answers through relay and uplink. The full I/O contract is in `docs/superpowers/plans/2026-06-25-io-contract-spec.md`.

### Commands

`run_command` (section 3) is how a guest has Orbiter run a command. It is an op too: the guest asks, Orbiter validates against the manifest, runs it and returns the output.

### The hook

The shell integration provides a hook, a text file named by its manifest. `orbiter shell init` prints it with `::ORBITER::` replaced by the path of the Orbiter binary. It wraps `orbiter`, interprets the directives above, answers FINDs, and a prompt hook calls `orbiter hook` to apply `SET` and `DEPART` as the Captain changes directory.

**Gaps.**
- The op vocabulary is defined only by `Directive.String()` and the hook scripts. There is no list of which party may send which op, and no schema for each op's reply beyond `findWire`.
- Request and answer are not checked against the requester's manifest: Orbiter filters `SET` keys against the manifest's `shell.exports`, but it does not check anything else it emits or receives.
- The FIND specs under `docs/superpowers/plans/2026-06-23-find-interface-spec.md` and `2026-06-25-relay-uplink-protocol-spec.md` show a `::ORBITER::FIND_BINARY:<txn> <args>` line format and a `--data` flag that the code does not use. The code is the contract; those specs are stale on this point.
- PATH is read by each shell's hook. There is no declared PATH contract per shell.
- Each FIND is answered by code in the hook script, which runs with the Captain's privileges and which Orbiter cannot bound, audit or check.

**Direction.**
- Orbiter runs every command; no integration has direct shell access.
- FIND and PATH are shell hook concerns. Orbiter uses the hook contract, and each shell supplies its own PATH contract. Orbiter parses the PATH content, which is OS-specific, with its filesystem parser.
- At integration init Orbiter does a PATH check and a FIND for the integration's binary, validates that the two locations agree, decides at init, and holds one config. It can then optionally run a command by the full path instead of the bare name. Today `run_command` runs the bare name against Orbiter's own PATH.
- Shell init discovers the shell through the integration, then installs its hooks (seven steps, `2026-10-27-shell-mediated-execution.md`).
- Hooks live in the shell, with a backup copy and a byte-check. A hook that fails lint is shown to the Captain, never refused by policy. A changed hook is not installed until the Captain confirms it.
- A hook inside the WASM binary is forbidden.

---

## 3. The ABI contract

The ABI is the interface between the host and a guest. A guest is a WASM module with no WASI: it has no files, no console, no clock beyond what the engine gives, and only the host functions below.

### Exports (guest provides)

| Export | Input | Output |
|---|---|---|
| `detect` | `DetectContext` | `DetectReport` |
| `initialize` | `ResolvedContext` | `StateReport` |
| `scan` | `ResolvedContext` | `StateReport` |
| `calibrate` | `ResolvedContext` | `StateReport` |

Each export takes no arguments and returns nothing. The input and output travel through the host functions. All JSON uses the field names in `internal/integrations/types.go`.

### Imports (host provides)

```text
orbiter.read_input(ptr i32, max i32) -> n i32
    Copies the call's input JSON into guest memory at ptr, at most max bytes.
    Returns the number of bytes written. Every call returns the same bytes from the start.

orbiter.write_output(ptr i32, len i32)
    Reads len bytes of guest memory at ptr and stores them as the call's output.
    A later call replaces the earlier output.

orbiter.run_command(specPtr i32, specLen i32, outPtr i32, max i32) -> n i32
    specPtr, specLen: JSON {"cmd": "git", "args": ["status"]} in guest memory.
    Writes the command's stdout and stderr, combined, at outPtr, at most max bytes.
    Returns the number of bytes written. Returns 0 when the command is denied,
    fails, times out or exceeds the output cap.

env.abort(i32, i32, i32, i32)
    A no-op. AssemblyScript modules always import it.
```

A module that imports anything else (WASI, an unknown function, or a memory) is refused at load.

### The call

1. The host takes a free slot (`pool_size`; calls wait for one).
2. It instantiates a fresh module and calls the export.
3. The guest calls `read_input`, works, may call `run_command`, then calls `write_output`.
4. The host closes the instance.

Every call is stateless: nothing one call leaves in memory reaches the next. Orbiter is the state manager; an integration keeps nothing between calls.

### `run_command` gate

1. **Quarantine.** A quarantined integration runs nothing.
2. **Allowlist.** The command name must be in `commands.allowed`. A path is never a name: a command containing a path separator or NUL is treated as undeclared. Straying quarantines the integration.
3. **Trust.** A command the Captain always-allowed runs without a prompt.
4. **Captain prompt.** Otherwise the Captain approves or denies (control characters in the prompt are escaped), and may always-allow the exact command.
5. **Execute.** With the manifest's timeout (default 5 minutes), the manifest's environment claim only, and the output capped at 4 MiB (the command is killed past it).

Every decision is written to the audit log.

### Limits

| Limit | Value |
|---|---|
| Guest compute per call | 30 seconds (time waiting for the Captain or a command does not count) |
| Command run time | manifest `timeout_seconds`, else 5 minutes |
| Command output | 4 MiB |
| Guest memory | `memory_limit`, default 64 MiB, max 1 GiB |

### Errors

| What happens | What the host does |
|---|---|
| The guest traps, runs out of memory or exceeds the compute limit | the call fails; `Init`, `Scan` and `Calibrate` return a `StateReport` with `Error` set; `Detect` returns an empty report |
| The export is missing | the call fails ("function not exported") |
| The output is not valid JSON for the report | `Error: unmarshal response`; `Detect` returns an empty report |
| A bad pointer passed to a host function | the host function does nothing and returns 0 |

`StateReport.Exports` is filtered to the variable names the manifest declares in `shell.exports`; anything else is dropped.

### Types the guest sees

`DetectContext`, `DetectReport`, `SuggestedResource`, `ResolvedContext` (with `Platform`, `Self`, `Resources`, `Transponders`, `Binaries`, `Files`, `Dirs`, `Envs`, `Responses`), `ResolvedResource`, `StateReport` (`present`, `reachable`, `binary_path`, `install_dir`, `in_path`, `manager`, `config`, `observations`, `error`, `needs_input`, `exports`) and `InputRequest` are defined in `internal/integrations/types.go`, which is the schema.

**Gaps.**
- No version on the contract, so a change to a shape cannot be told from an old guest.
- A module's exports are checked only when called, not at load.
- `run_command` returns 0 for both "denied or failed" and "ran and printed nothing", and gives no exit code or reason. Stdout and stderr are combined.
- `read_input` truncates silently when the input is larger than `max`, and the guest cannot learn the full size.
- `write_output` has no size cap beyond guest memory.
- `StateReport.NeedsInput` and `ResolvedContext.Responses` are declared but nothing reads or fills them, so a guest cannot ask the Captain for input yet.
- `docs/integrations.md` still shows `"brand": "go"` and a 64 KB payload note, and omits most of `StateReport`.

**Direction.** Integrations are function-only, with no exceptions: Orbiter hands an integration data, the integration decides and returns output, and Orbiter acts on it. Orbiter runs every command. A guest may request anything its manifest declares, Orbiter enforces the manifest at runtime, and the Captain is responsible for reading it.
