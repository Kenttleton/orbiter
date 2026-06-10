# Orbiter — Scaffolding Design

**Date:** 2026-06-09
**Phase:** 1 — Scaffolding
**Status:** Draft

---

## Overview

This document describes the foundational scaffold for the Orbiter project: a cross-platform CLI/TUI for freelance and contract software engineers to navigate and orchestrate development environments across organizations, identities, credentials, and projects.

The scaffold establishes the project structure, binary separation, Star Chart schema, internal package layout, output rendering layer, and build tooling. No feature logic is implemented in this phase — only the skeleton that all subsequent phases build on.

---

## Technology Decisions

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go | Single static binaries, excellent CLI/TUI ecosystem, native cross-compilation |
| CLI framework | Cobra | Standard Go CLI framework, subcommand tree, flag inheritance |
| TUI framework | Bubble Tea + Lipgloss | Charmbracelet ecosystem, composable views, styled output |
| SQLite driver | `modernc.org/sqlite` | Pure Go, no CGo, cross-compiles cleanly to all targets |
| ID generation | Custom OrbitID | 16-char snowflake: 8 timestamp + 2 entity type + 6 random, base36, stdlib only |
| Build tooling | Just (Justfile) | Cross-platform task runner |
| Migrations | Embedded SQL via `go:embed` | Simple integer versioning, no external migration tooling |

---

## Binary Separation

Two binaries are produced from one Go module:

### `orbit` — CLI

- All actionable commands (CRUD, Six Commands)
- Source of truth for all Star Chart operations
- Supports styled output (default) and JSON output (flag or config)

### `orbiter` — TUI

- Read-only viewer for the universe and Beacons (Phase 1–4)
- CRUD and Calibration via TUI deferred to Phase 5
- Shells out to `orbit` subprocesses and parses JSON output
- **Requires `orbit` in PATH** — mirrors how git UIs wrap the `git` CLI

### Install Options

- **`orbit` only** — CLI-only install, no TUI
- **Both** — install `orbit` first, then `orbiter`
- Installing `orbiter` without `orbit` is not supported

---

## Project Structure

```text
orbiter/
├── cmd/
│   ├── orbit/
│   │   └── main.go          ← CLI entry point
│   └── orbiter/
│       └── main.go          ← TUI entry point
├── internal/
│   ├── starchart/           ← SQLite connection, migrations, transactions
│   ├── models/              ← Domain structs with ID + JSON tags
│   ├── commands/            ← Cobra command tree + DI wiring
│   ├── resolver/            ← Alias → ID middleware layer
│   ├── output/              ← Styled + JSON renderer interface and impls
│   └── tui/                 ← Bubble Tea views + orbit subprocess runner
├── migrations/              ← Embedded SQL migration files
│   └── 0001_initial.sql
├── docs/
│   └── superpowers/
│       └── specs/
├── go.mod
├── go.sum
├── justfile
├── LICENSE
└── ORBITER_CONSTITUTION.md
```

---

## Star Chart Schema

The Star Chart is a SQLite database at `~/.orbiter/starchart.db` by default.

**Location resolution order:**
1. `ORBIT_STARCHART` environment variable
2. `~/.orbiter/starchart.db`

### OrbitID Format

Every entity is assigned an **OrbitID** — a 16-character, time-sortable, self-describing identifier generated entirely from stdlib.

```text
[8 chars timestamp][2 chars entity type][6 chars random]
 0k3m2r4a           pl                   7b2c1f
```

**Structure:**

|Segment|Length|Encoding|Source|
|---|---|---|---|
|Timestamp|8 chars|base36, ms since 2025-01-01 epoch|`time.Now().UnixMilli()`|
|Entity type|2 chars|fixed prefix per type|see table below|
|Random|6 chars|base36|`math/rand/v2` (non-crypto)|

**Entity type prefixes:**

|Entity|Prefix|
|---|---|
|vessel|`vs`|
|galaxy|`gx`|
|solar_system|`sy`|
|planet|`pl`|
|callsign|`cs`|
|transponder|`tp`|
|resource|`rs`|
|default|`df`|
|beacon|`bk`|
|navigation_history|`nh`|

**Properties:**

- Lexicographically sortable by creation time
- Entity type filterable without a JOIN: `WHERE id LIKE '__________pl%'`
- `ParseID(id)` extracts entity type and timestamp from the ID directly
- Resolver can infer entity type from ID format, skipping a DB round-trip
- No external dependency — pure stdlib (`math/rand/v2`, `time`, `strconv`)
- `math/rand/v2` is appropriate: physical machine access = Star Chart access

**API in `internal/models/id.go`:**

```go
type OrbitID struct {
    Raw        string
    EntityType string
    Timestamp  time.Time
}

func NewID(entityType string) string       // generate a new OrbitID string
func ParseID(id string) (OrbitID, error)  // extract type + timestamp from ID
func IsID(s string) bool                  // true if s matches OrbitID format
```

### The Alias Table as Global ID Registry

The `aliases` table is the single registry for all entity IDs. Every entity — regardless of type — is registered here when created. This guarantees:

- No ID collisions across entity tables
- Entity type is stored once, in one place (also derivable from the ID itself)
- Cross-table references use only `entity_id` — no `entity_type` column needed elsewhere

```sql
CREATE TABLE aliases (
    id          TEXT PRIMARY KEY,   -- OrbitID: 16-char snowflake
    name        TEXT UNIQUE NOT NULL, -- defaults to id when no alias given
    entity_type TEXT NOT NULL,      -- 'vessel'|'galaxy'|'solar_system'|'planet'|
                                    -- 'callsign'|'transponder'|'resource'
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**CLI resolution:** input → check `aliases.name` → return `id` → operate on record.
**ID fallback:** if input matches an OrbitID directly, skip alias lookup.
**Alias ownership:** aliases are created, updated, and removed exclusively through CRUD operations — never by the Six Commands.

### Entity Tables

Entity tables carry no `name` column and no `entity_type` column. Identity lives in `aliases`.

#### `vessel`

Single-row. Represents the local workstation.

```sql
CREATE TABLE vessel (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

One row inserted on first run. Single-row enforced by application logic in `orbit vessel init`.

#### `galaxies`

```sql
CREATE TABLE galaxies (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `solar_systems`

```sql
CREATE TABLE solar_systems (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `planets`

```sql
CREATE TABLE planets (
    id              TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id       TEXT NOT NULL REFERENCES aliases(id),
    solar_system_id TEXT REFERENCES aliases(id),  -- optional
    repo_url        TEXT,
    repo_path       TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `callsigns`

Represents the Captain's active identity. Scoped to vessel or galaxy.

```sql
CREATE TABLE callsigns (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),  -- vessel or galaxy
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `transponders`

Pointers to credential locations and auth services. Never store secrets.
Always linked to a callsign. Think of a transponder as a named collection of
credential pointers that activates alongside a callsign for a given context.

```sql
CREATE TABLE transponders (
    id           TEXT PRIMARY KEY REFERENCES aliases(id),
    callsign_id  TEXT NOT NULL REFERENCES aliases(id),  -- required, always callsign-scoped
    entity_id    TEXT REFERENCES aliases(id),            -- optional: narrows to planet/system
    service      TEXT NOT NULL,     -- e.g. 'github' | '1password' | 'aws'
    location     TEXT NOT NULL,     -- pointer to credential (never the credential itself)
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `resources`

Tooling, runtimes, and capabilities. Scoped to any entity via `entity_id`.

```sql
CREATE TABLE resources (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    kind       TEXT NOT NULL,    -- e.g. 'node' | 'python' | 'docker'
    manager    TEXT,             -- e.g. 'nvm' | 'uv' | 'rustup'
    version    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

#### `defaults`

Configuration defaults scoped to any entity. Vessel-level defaults include output format.

```sql
CREATE TABLE defaults (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, key)
);
```

**Output format default:** stored as `key='output_format'`, `value='styled'|'json'` on the vessel record. Exact CRUD command syntax defined in Phase 2.

#### `navigation_history`

Immutable log of navigation events. Subject to retention cleanup (see below).

```sql
CREATE TABLE navigation_history (
    id               TEXT PRIMARY KEY,
    from_entity_id   TEXT REFERENCES aliases(id),  -- null on first jump
    to_entity_id     TEXT NOT NULL REFERENCES aliases(id),
    command          TEXT NOT NULL,
    occurred_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX navigation_history_occurred_idx ON navigation_history(occurred_at);
```

**Retention:** navigation history is cleaned on a configurable cycle. Default retention is 90 days. The cleanup operation is an explicit `orbit vessel history clean` command and also runs automatically during `orbit scan`. Retention period is stored as a vessel-level default (`key='history_retention_days'`).

#### `beacons`

Most recent verified observation of an entity. One beacon per entity.

```sql
CREATE TABLE beacons (
    id           TEXT PRIMARY KEY,
    entity_id    TEXT NOT NULL REFERENCES aliases(id),
    status       TEXT NOT NULL,       -- 'healthy' | 'drifted' | 'unknown'
    observations TEXT NOT NULL,       -- JSON array of observation strings
    verified_at  DATETIME NOT NULL,
    UNIQUE(entity_id)
);
```

### Schema Migrations

Migrations are plain `.sql` files embedded via `go:embed`, applied in version order on startup.

```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

## Internal Package Design

### `internal/starchart`

Hybrid approach: generic CRUD methods for simple entity operations, full transaction pipeline reserved for Six Commands and the resolver.

**Generic CRUD methods** (used by Phase 2 CRUD commands):

- `Insert(ctx, table string, record any) error` — insert a model by table name
- `Get(ctx, table, id string, dest any) error` — fetch single record by ID
- `List(ctx, table string, dest any, filters ...Filter) error` — list records with optional filters
- `Update(ctx, table, id string, fields map[string]any) error` — partial update by ID
- `Delete(ctx, table, id string) error` — delete by ID

These cover the straightforward CRUD path without hand-writing queries per entity type.

**Transaction pipeline** (used by Six Commands and resolver):

- `(*StarChart).Tx(fn func(*Tx) error) error` — full Prepare → Validate → Execute → Verify → Commit wrapper for operations that touch multiple tables or require side-effect coordination
- `(*StarChart).Resolve(input string) (Alias, error)` — optimized single query: checks `aliases.name` first, falls back to direct ID match

### `internal/models`

- One struct per entity with `ID` field and JSON tags
- `Vessel`, `Galaxy`, `SolarSystem`, `Planet`, `Callsign`, `Transponder`, `Resource`, `Alias`, `Default`, `Beacon`, `NavigationHistory`
- Pure data structures — no database logic

### `internal/resolver`

Alias → ID resolution as a standalone middleware layer, dependency-injected into all commands.

```go
type Resolver interface {
    Resolve(input string) (id string, entityType string, err error)
}
```

- Checks `aliases.name` first, falls back to direct ID match if input matches OrbitID format
- Returns a typed result so callers know what kind of entity they're operating on
- Consumed by commands via DI — no command duplicates this logic

### `internal/output`

Output renderer as a dependency-injected resource at the command interface.

```go
type Renderer interface {
    Info(msg string)
    Success(msg string)
    Warning(msg string)
    Error(msg string)
    Plan(steps []PlanStep)
    Table(headers []string, rows [][]string)
    Progress(label string) ProgressHandle
    JSON(v any) error
}
```

- `StyledRenderer` — Lipgloss-based, Terraform-inspired: green additions, red removals, yellow changes, indented plan blocks
- `JSONRenderer` — marshals structured output as JSON to stdout
- `NewRenderer(format string) Renderer` — factory based on config/flag
- Injected into commands at startup — commands never select their own renderer

**Progress indication** uses thematic sci-fi messaging via Bubble Tea spinners. Because Six Commands derive their full task list from the Star Chart before executing, all steps are known upfront. Progress is shown as a persistent list with a live fraction counter:

```text
Executing maneuver...

  [1/5] ✓ Plotting course...          Cloned acme/payment-api
  [2/5] ✓ Calibrating transponder...  GitHub credentials verified
  [3/5] ⠸ Acquiring resource...       Installing node v20.0.0 via nvm
  [4/5]   Acquiring resource...       Installing pnpm
  [5/5]   Sweeping sector...          Scanning payment-api
```

Each step shows: fraction counter + thematic label (primary) + plain operational subtitle (persistent, always visible). Completed steps show a checkmark; the active step shows a spinner; pending steps are dimmed.

|Operation|Thematic label|Plain subtitle|
|---|---|---|
|Cloning repository|`Plotting course...`|`Cloning [org/repo]`|
|Installing resource|`Acquiring resource...`|`Installing [kind] v[version] via [manager]`|
|Verifying credentials|`Calibrating transponder...`|`Verifying [service] credentials`|
|Scanning entity|`Sweeping sector...`|`Scanning [name]`|
|Connecting to service|`Establishing link...`|`Connecting to [service]`|
|Applying changes|`Executing maneuver...`|`Applying changes to [name]`|

**`--verbose` flag** (root-level, also `ORBIT_VERBOSE=1`): replaces thematic labels with plain operational text, keeps the fraction counter, and streams the full raw output of every underlying tool invocation (git, nvm, uv, etc.) inline under each step. Intended for debugging stalled operations, CI pipelines, and engineers who need to see exactly what each tool is doing and why.

### `internal/commands`

- Cobra root command and subcommand tree (empty stubs in Phase 1)
- DI wiring: `Resolver`, `Renderer`, and `StarChart` injected at root, propagated to subcommands via `cobra.Command.PersistentPreRunE`
- `--output json` flag at root level overrides vessel default
- `ORBIT_OUTPUT=json` env var also overrides

### `internal/tui`

- `Runner` — executes `orbit` subprocesses with `--output json`, parses response
- Bubble Tea model stubs for universe view and beacon view (Phase 1: empty scaffolds)

---

## Output Format

**Default:** styled (human-readable, Terraform-inspired)
**Flag override:** `--output json` per invocation
**Env override:** `ORBIT_OUTPUT=json`
**Persisted default:** vessel-level default in Star Chart (managed via Phase 2 CRUD)

---

## Star Chart Integrity

All state-changing operations follow the pipeline from the constitution:

```text
Prepare   → validate inputs, resolve aliases, check preconditions
Validate  → check Star Chart consistency (no clash, no orphan)
Execute   → perform external side effects (if any)
Verify    → confirm results
Commit    → write to Star Chart
```

Implemented as a `Tx` wrapper in `internal/starchart`. Failed operations roll back without leaving the Star Chart in an invalid state.

---

## Build Tooling (Justfile)

```just
build-orbit:
    go build -o bin/orbit ./cmd/orbit

build-orbiter:
    go build -o bin/orbiter ./cmd/orbiter

build: build-orbit build-orbiter

install-orbit:
    go install ./cmd/orbit

install-orbiter:
    go install ./cmd/orbiter

install: install-orbit install-orbiter

test:
    go test ./...

lint:
    golangci-lint run

clean:
    rm -rf bin/

# Cross-compilation target for CI release builds.
# Usage: just build-release orbit linux amd64 v1.2.3
build-release binary goos goarch version:
    #!/usr/bin/env bash
    set -euo pipefail
    EXT=""
    if [ "{{goos}}" = "windows" ]; then EXT=".exe"; fi
    mkdir -p dist
    CGO_ENABLED=0 GOOS={{goos}} GOARCH={{goarch}} go build \
        -ldflags="-s -w -X main.version={{version}}" \
        -o "dist/{{binary}}-{{goos}}-{{goarch}}${EXT}" \
        ./cmd/{{binary}}
```

### CI/CD Notes

The `build-orbit` and `build-orbiter` targets are for local development only — they output to `bin/` with no cross-compilation flags.

The `build-release` recipe is the CI/CD entrypoint. It accepts four parameters (`binary`, `goos`, `goarch`, `version`) and:

- Outputs to `dist/` with a platform-suffixed filename (e.g. `orbit-linux-amd64`, `orbiter-darwin-arm64.exe`)
- Injects the version string via `-X main.version`
- Sets `CGO_ENABLED=0` for portable static binaries
- Uses a bash shebang so the `.exe` extension logic works on any CI runner

The GitHub release workflows invoke it as:

```bash
just build-release orbit   $GOOS $GOARCH $VERSION
just build-release orbiter $GOOS $GOARCH $VERSION
```

Both `orbit` and `orbiter` are built for every target in the release matrix.

---

## Cross-Platform Notes

- Core scaffold compiles cleanly for all platforms via `GOOS`/`GOARCH`
- No CGo dependencies — `modernc.org/sqlite` is pure Go
- Platform-specific concerns (credential store access, shell env mutation) deferred to the Integrations phase, handled per-tool

---

## What This Phase Does NOT Include

- Any CRUD command logic (Phase 2)
- Any Six Command logic (Phase 3)
- Any integration with external tools (Phase 4)
- TUI management views (Phase 5)
- Actual Cobra command implementations (stubs only)
- Actual Bubble Tea views (stubs only)
