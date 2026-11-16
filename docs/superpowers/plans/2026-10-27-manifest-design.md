# Manifest Design: Review

**Status:** inventory and framing only. No syntax is proposed here; the design is the next step.

**Why now:** the Captain wants the manifest syntax looked at as a whole instead of patched section by section. Recent changes (runtime engine, memory, environment claims, hook artifacts, per-integration limits) each added fields to the existing sections, and several decisions in flight depend on where those fields live: the environment read and write claims, artifact declarations, and the shell-mediated execution plan (`2026-10-27-shell-mediated-execution.md`).

## The framing

A manifest makes claims, and Orbiter enforces them at runtime while the Captain reads them to decide on trust. The Captain's grouping of what a manifest says:

1. **Metadata claims**: what the integration is (brand, name, description, roles, what it manages).
2. **Internal resources for the runtime**: what the integration needs in order to run as a function (engine, memory, concurrency, buffers, timeouts), and the environment variables it reads.
3. **External resources for operations**: what it asks Orbiter to do on its behalf (commands it may run and with what environment, files, directories and variables it may look for, variables it may write to the shell, artifacts it provides).
4. **Health and detection**: how Orbiter learns whether the integration applies here and whether its tool is healthy (detection rules, the health command).

The manifest is one of three contracts that belong together: the manifest, the op protocol (PATH, FIND and the other hooks) and the wasm interface (see `2026-10-27-shell-mediated-execution.md`, section 0). Runtime claims in the manifest are also security contracts. The manifest also holds file paths for both platforms (Windows and POSIX), so an integration that does not support the current OS can still take part.

The inventory below shows the current sections against that grouping.

## Inventory (28 bundled manifests)

| Section | Fields | Used by | Read by | Group |
|---|---|---|---|---|
| `[integration]` | `brand`, `name`, `description`, `roles` (28); `binaries` (24) | all | loader, catalog, discovery, FIND planner (`binaries`) | metadata (and `binaries` is an external-resource request) |
| `[detection]` | `files` (14 of 16 manifests that have it), `env` rules (shells no longer use them) | 16 | discovery pre-filter (`MatchesAny`) | health and detection |
| `[dependencies]` | `resources`, `transponders` (role to allowed brands) | 5 (2 of them fill the `resources` / `transponders` sub-tables) | the star chart crawl (`internal/starchart/crawl.go`) | external resources |
| `[commands]` | `allowed` (26), `timeout_seconds` (26), `env` (25) | 26 | the `run_command` gate | external resources; `timeout_seconds` and `env` are also runtime |
| `[shell]` | `exports` (7): hook files and env variables the integration may write | 7 | export filter, shell init (`HookFile`) | external resources (writes) |
| `[runtime]` | `engine`, `memory_limit`, `pool_size`, `input_buffer_kb`, `output_buffer_kb` | all (pool and buffers); engine and memory are new and unset in every bundled manifest | loader (`engine`, `memory_limit`, `pool_size`); **nothing reads the buffer hints** | runtime |
| `[find]` | `files`, `dirs`, `envs` | **0 manifests** | FIND planner | external resources |
| `[config]` | `fields` (key, type, required, description) | **0 manifests** | **nothing** | metadata (a config schema) |

## What the inventory shows

- **Overlap in the environment.** Three sections touch environment variables: `[commands] env` (what a command sees), `[find] envs` (variables Orbiter looks up for the integration) and `[shell] exports[].envs` (variables the integration may set in the shell, with the unused `sensitive` flag). Read and write are not distinguished by structure.
- **`[commands]` mixes two concerns.** `allowed` and `env` are an external-operation claim; `timeout_seconds` is a runtime limit.
- **Hooks are declared as a file name inside a list of env exports** (`[shell] exports = [{ hook = "..." }]`), so a hook is not an artifact in any structural sense.
- **Dead or unfinished sections.** `[find]` and `[config]` appear in no bundled manifest, and the `[runtime]` buffer hints are read by nothing. `[detection] env` rules were removed from every shell and are read only for non-shell integrations that have not used them.
- **Health has no home.** The shells declare their health check as an ordinary `[commands] allowed` entry; nothing marks a command as the health check.
- **Where to look for a declaration is implicit.** An integration's binaries are in `[integration]`, its other finds in `[find]`, its commands in `[commands]`; there is no single place that lists "what this integration asks Orbiter for".

## What the design has to settle

These are the questions the redesign answers; this document proposes nothing yet.

1. The top-level grouping of sections, and whether it follows the Captain's four groups.
2. One place for environment read and write claims, and how `sensitive` attaches to them.
3. How hooks are declared (a plain text file passed by file name, which the Captain and Orbiter both read), including a size cap, and what Orbiter records for the byte-check.
3a. How file paths for both platforms are declared.
4. How the health check is declared.
5. How a manifest is versioned, so Orbiter can read an old one after the syntax changes, and how the 28 bundled manifests migrate.
6. Whether unused sections (`[find]`, `[config]`, the buffer hints) are kept, wired up or removed.
7. How the manifest reads to the Captain: verbose and explicit, with a description beside every claim, because reading it is how trust is decided.
