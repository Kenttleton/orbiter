# Phase 3: Lifecycle Commands Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the six lifecycle commands (`survey`, `chart`, `jump`, `scan`, `calibrate`, `retro`), rename the binary from `orbit` to `orbiter`, add `orbiter starchart` as the TUI entry point, and wire shell integration with autocomplete.

**Architecture:** A shared `Executor` drives all six commands through the same pipeline (resolve → crawl → dispatch → aggregate → beacon → render). Commands differ only in which integration method they call and what they do with the results. Shell integration follows the starship.rs model: scripts are embedded in the binary, `orbiter init <shell>` injects the binary path and outputs the integration function.

**Tech Stack:** Go, Cobra, Bubble Tea, Lipgloss, wazero (integration dispatch), modernc.org/sqlite, `//go:embed` for shell scripts.

---

## Integration Contract

Integrations are stateless WASM modules — the only mechanism by which Orbiter interacts with the real world. There is no fallback, no built-in behavior, and no direct interaction outside of integration dispatch. Orbiter owns all state management; integrations own all interaction with external tools, services, and the local environment. Each handler call is independently invokable, idempotent, and side-effect-free from Orbiter's perspective — mirroring the AWS Lambda model.

**Integration manifests are stored in the Star Chart.** When an integration is loaded, its manifest metadata (role, brand, declared file detection patterns, dependencies) is written to the database. This cache enables efficient pre-filtering during discovery and allows the lifecycle system to reason about registered integrations without re-reading WASM binaries on every operation. Manifests are not the integration itself — they are its declared contract with Orbiter.

Integration manifests are never touched by `retro`. The manifest cache reflects the integration registry, not the Star Chart graph.

**Phase 3 scope:** The lifecycle commands call the integration layer as registered at startup. Manifest refresh, integration scanning, and runtime integration registration are Phase 4 concerns.

### Inside-out operations (discovery)

During `planet init --discover`, Orbiter pre-filters the registered integrations using the **stored manifest file patterns** — only integrations whose declared patterns match files present in the CWD are candidates. Matched integrations then have their `detect` handler called to confirm relevance and suggest resources to attach. This avoids calling every integration on every discovery pass.

No prior Star Chart resource or transponder registration is required for discovery. Integrations can surface suggestions entirely from the environment.

### Outside-in operations (lifecycle)

Once resources and transponders are attached to the Star Chart graph, the lifecycle commands (`jump`, `scan`, `calibrate`, `chart`) dispatch through them. Each attached resource's `role + brand` identifies which integration to call. If no integration is registered for a resource or transponder's `role + brand`, Orbiter skips it and records the entity as `unknown` in its beacon.

---

## Binary Rename

`orbit` → `orbiter`. The previous `orbiter` TUI binary is replaced by `orbiter starchart`.

| Before | After |
| --- | --- |
| `orbit <command>` | `orbiter <command>` |
| `orbiter` (TUI) | `orbiter starchart` |
| `cmd/orbit/` | `cmd/orbiter/` |

All internal references, Justfile targets, README, ORBITER_CONSTITUTION, and docs update to reflect `orbiter` as the single binary. `orbit` is removed entirely.

---

## Shell Integration

`orbiter` ships shell integration scripts embedded in the binary via `//go:embed`. The scripts use a `::ORBITER::` token replaced at `init` time with the actual binary path — matching the starship.rs pattern.

### Init command

```bash
orbiter init zsh    # outputs zsh integration function
orbiter init bash   # outputs bash integration function
orbiter init fish   # outputs fish integration function
```

Users add one line to their shell profile:

```bash
# .zshrc / .bashrc
eval "$(orbiter init zsh)"

# config.fish
orbiter init fish | source
```

### Embedded script pattern

Each shell script is a passthrough function that intercepts only `jump` (which needs `eval` to apply `cd` and env exports to the current shell). Everything else calls the binary directly using the injected path to avoid infinite recursion.

**`shell/orbiter.zsh`** (embedded, `::ORBITER::` replaced at init):

```zsh
function orbiter() {
  if [ "$1" = "jump" ]; then
    eval "$(::ORBITER:: "$@")"
  else
    ::ORBITER:: "$@"
  fi
}
```

**`shell/orbiter.bash`** (same pattern, bash-compatible):

```bash
function orbiter() {
  if [ "$1" = "jump" ]; then
    eval "$(::ORBITER:: "$@")"
  else
    ::ORBITER:: "$@"
  fi
}
```

**`shell/orbiter.fish`**:

```fish
function orbiter
  if test "$argv[1]" = "jump"
    ::ORBITER:: $argv | source
  else
    ::ORBITER:: $argv
  end
end
```

### Autocomplete

```bash
orbiter completions zsh    # outputs zsh completion script
orbiter completions bash
orbiter completions fish
```

Generated by Cobra's built-in completion engine from the command tree. No manual maintenance. Users can pipe into their completions directory or source directly.

---

## CWD Resolution

When a lifecycle command is called without an explicit target, `orbiter` resolves the target from the current working directory.

**Algorithm:** exact match first, then longest prefix match (CSS selector specificity logic).

1. Query the starchart for all entities that have a stored filesystem path (planets, galaxies with a base path).
2. If any entity path exactly equals the CWD → that entity wins immediately.
3. Otherwise, for each entity path, check whether the CWD starts with that path.
4. Among all prefix matches, select the one with the longest matching path prefix.
5. If no match: `error: no target found`

**Examples:**

```text
CWD: ~/Documents/acme
  → exact match on galaxy ~/Documents/acme → galaxy wins

CWD: ~/Documents/acme/payment-api/src/handlers
  → no exact match
  → Galaxy path:  ~/Documents/acme              ← prefix match, 3 segments
  → Planet path:  ~/Documents/acme/payment-api  ← prefix match, 4 segments → winner
```

This is the same pattern used by `add`, `init`, and `attach` — the Executor calls the existing starchart resolution path.

---

## Executor

`internal/commands/executor.go` — owns the shared pipeline. Each lifecycle command is a thin Cobra handler that calls one Executor method.

```go
type Executor struct {
    sc       *starchart.StarChart
    renderer output.Renderer
}

func (e *Executor) Survey(ctx context.Context, target string) error
func (e *Executor) Chart(ctx context.Context, target string) error
func (e *Executor) Jump(ctx context.Context, target string) ([]ShellDirective, error)
func (e *Executor) Scan(ctx context.Context, target string) error
func (e *Executor) Calibrate(ctx context.Context, target string) error
func (e *Executor) Retro(ctx context.Context, target string, confirmed bool) error
```

### Shared pipeline

Every method runs the same steps, varying only in step 3:

```text
1. Resolve     explicit target  → starchart.Resolve(input)
               no target        → starchart.ResolveCWD(cwd)  [longest prefix match]
               no match         → error "no target found"

2. Crawl       BranchCrawl(entityID)
               → collects attached resources, callsigns, transponders

3. Dispatch    call integration method per resource (see command table below)
               BuildResolvedContext(branch, manifest) for each integration

4. Aggregate   collect StateReports, compute delta vs desired state

5. Beacon      write updated observations (commands that call integrations)

6. Render      lipgloss styled (default) or JSON (--output json)
```

### ShellDirective

`Jump` returns directives for the shell function to `eval`:

```go
type ShellDirective struct {
    Op    string // "cd" | "export"
    Key   string // env var name (export only)
    Value string // path or value
}
```

The binary serialises these to stdout as shell statements:

```text
cd /path/to/acme/payment-api
export GITHUB_SSH_KEY=/Users/kent/.ssh/id_ed25519_acme
```

---

## The Six Commands

### `orbiter survey [target]`

**Purpose:** "What is this thing?" — inspect desired state.

- Resolve target (explicit or CWD)
- BranchCrawl
- Read Star Chart data: entity metadata, attached resources, callsigns, transponders
- Read latest beacon for each entity
- **No integration calls**
- **No beacon writes**
- Render: desired state table + last observed beacon per entity

---

### `orbiter chart [target]`

**Purpose:** "What would happen if I jumped there?" — terraform plan.

- Resolve target (explicit or CWD)
- BranchCrawl
- Call `integration.Scan()` per resource → `StateReport`
- **Write beacons** (side effect of scan — more health checks = better data)
- Compute delta: compare StateReports against desired state
- Render plan with terraform-style markers — no mutations beyond beacon writes

**Output markers:**

```text
✓  runtime/go        present, reachable, v1.25.1
+  manager/nvm       not installed — will install
~  remote/github     repository not cloned — will clone
```

Each line represents a resource (`role/brand` display format) with the action its integration will take.

---

### `orbiter jump [target]`

**Purpose:** "Take me there." — terraform apply.

- Resolve target (explicit or CWD)
- BranchCrawl
- Call `integration.Scan()` per resource to build the delta (same as `chart`)
- **Render delta plan** (same terraform-style output)
- **Prompt confirmation:** `Execute maneuver? [y/N]`
- On confirm:
  - For each attached resource: call `integration.Init()` (unverified/failed) or `integration.Calibrate()` (drifted) — this includes `role=remote` resources such as a github integration, which handles clone idempotently like any other resource
  - Activate callsign (build env export directives)
  - Load transponders (build env export directives)
  - Write beacons
- Return `[]ShellDirective` for shell function to `eval` (`cd` + env exports)

`jump` without the shell function still executes everything — it just prints the directives as plain output rather than having them `eval`'d.

There is no special clone step. Repository cloning is owned entirely by whichever `role=remote` integration is registered for the planet's remote resource. If no such integration is registered, `jump` skips that resource.

---

### `orbiter scan [target]`

**Purpose:** "What does reality currently look like?"

- Resolve target (explicit or CWD)
- BranchCrawl
- Call `integration.Scan()` per resource → `StateReport`
- Write beacons with results
- Render observed state: what is present, reachable, healthy vs drifted per resource

---

### `orbiter calibrate [target]`

**Purpose:** "Bring reality and the Star Chart back into alignment."

- Resolve target (explicit or CWD)
- BranchCrawl
- Call `integration.Scan()` per resource to identify drift
- For each drifted/missing resource: call `integration.Calibrate()`
- Write beacons
- Render: what was already healthy, what was fixed, what failed

---

### `orbiter retro [target]`

**Purpose:** "Remove what no longer belongs."

`retro` operates exclusively on Star Chart entities and their graph relationships — galaxies, solar systems, planets, callsigns, resources, transponders, and attachments. **Integrations are not Star Chart entities and are never touched by `retro`.** The integration registry is unaffected by any retro operation.

- Resolve target (explicit or CWD)
- Deep crawl entire subtree from target node (all descendants)
- For each node: query attachment table — is it attached to any branch other than the one being retired?
  - **Unshared node** → will be retired (beacon set to `retired`)
  - **Shared node** → attachment will be broken, node survives
- Render delta before confirmation:

```text
-  planet/payment-api     will be retired
-  resource/node-20       will be retired
~  transponder/acme-ssh   shared — attachment only removed
```

- **Prompt confirmation:** `Retire N entities? [y/N]`
- On confirm: execute in a single transaction
  - Delete unshared nodes
  - Remove attachments for shared nodes
  - Set beacons to `retired` for retired nodes

---

## Output Format

All commands output **lipgloss styled** terminal output by default. Pass `--output json` (or set `ORBITER_OUTPUT=json`) for machine-readable output consumed by `orbiter starchart` and scripting.

Terraform-style status markers used across `chart`, `jump`, `calibrate`, `retro`:

| Marker | Meaning |
| --- | --- |
| `✓` | Healthy / no change needed |
| `+` | Will be installed / provisioned |
| `~` | Will be updated / reconfigured / detached |
| `-` | Will be retired / removed |
| `✗` | Failed / unreachable |

Progress during `jump` shows a live step list (existing output pattern from Phase 2):

```text
Executing maneuver...

  [1/4] ✓ Verified runtime/go          go 1.25.1
  [2/4] ✓ Verified remote/github       SSH key valid
  [3/4] ⠸ Installing manager/nvm...
  [4/4]   Cloning acme/payment-api
```

---

## `orbiter starchart`

The TUI entry point. Launches the Bubble Tea interface for visual inspection of the Star Chart.

- Registered as a Cobra subcommand on the `orbiter` root
- Calls the existing `internal/tui` package (Bubble Tea stubs from Phase 2 wiring)
- Reads Star Chart data; all mutations go through the CLI layer
- Receives data via `--output json` calls to the CLI (same pattern as before)

No new TUI functionality is in scope for Phase 3 — just wiring `orbiter starchart` as the correct entry point.

---

## Phase 3 Scope Summary

| Area | In Scope | Out of Scope |
| --- | --- | --- |
| Binary rename `orbit` → `orbiter` | ✓ | |
| `orbiter starchart` TUI entry | ✓ (wire only) | TUI feature work |
| Shell integration (`init`, `completions`) | ✓ | Windows / PowerShell / Elvish |
| CWD longest-prefix resolution | ✓ | |
| Executor + shared pipeline | ✓ | |
| `survey`, `chart`, `jump`, `scan`, `calibrate`, `retro` | ✓ | |
| Beacon writes from lifecycle commands | ✓ | |
| Retro cascade with shared node detection | ✓ | |
| `jump` confirmation prompt | ✓ | |
| `retro` confirmation prompt | ✓ | |
| `remote/github` integration (Rust, WASM) | | Phase 3.5 |
| Manifest refresh during lifecycle | | Phase 4 |
| Runtime integration scanning and registration | | Phase 4 |
| Integration instance pooling | | Phase 4 |
| Runtime plugin directories | | Phase 4 |
| 64 KB payload enforcement | | Phase 4 |
| Multi-language integration testing | | Phase 4 |
| TUI feature work | | Phase 5 |

### Phase 3.5: `remote/github` integration

Phase 3.5 adds a `role=remote, brand=github` integration written in Rust (compiled to WASM). This is the first integration required to exercise the full lifecycle — without it, `jump` cannot clone repositories and `scan` cannot verify remote reachability.

It implements all four handlers:

- `detect` — check for `.git/config` with a github.com remote URL in the project directory
- `initialize` — clone the repository to the planet's configured local path (idempotent — no-op if already cloned)
- `scan` — verify the repository exists locally and the remote is reachable
- `calibrate` — re-establish the remote if missing, update the origin URL if it has changed

Phase 3.5 also serves as the empirical test for the Rust WASM guest ABI documented in `docs/integrations.md`.
