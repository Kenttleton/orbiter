# Orbiter — CRUD Commands Design

**Date:** 2026-06-09
**Phase:** 2 — Entity Build Commands
**Status:** Draft

---

## Overview

This document specifies the CRUD command layer for Orbiter — the "build" operations that populate the Star Chart with entities. These commands are intentionally separate from the six lifecycle commands that operate the universe.

The six lifecycle commands (`survey`, `chart`, `jump`, `scan`, `calibrate`, `retro`) assume a populated Star Chart. The build commands (`add`, `init`) create the universe those commands operate on.

---

## Two-Layer Model

Orbiter has two distinct command layers with different philosophies:

| Layer | Commands | Style | Purpose |
| --- | --- | --- | --- |
| **Build** | `galaxy add`, `planet init`, etc. | Noun-first, entity-specific | Populate the Star Chart |
| **Operate** | `survey`, `chart`, `jump`, `scan`, `calibrate`, `retro` | Verb-first, lifecycle | Run the universe |

This distinction is intentional. The Captain is always either expanding their universe (build) or navigating and maintaining it (operate). The two layers never blur.

---

## Command Surface

### The Build Commands

All entities support both `add` (metadata registration) and `init` (provisioning), except callsign which is purely metadata.

```text
orbit galaxy add [<alias>]
orbit galaxy init [<alias>]

orbit system add [<alias>]
orbit system init [<alias>]

orbit planet add [<alias>]
orbit planet init [<alias>] [<url>]

orbit callsign add [<alias>]
orbit callsign init [<alias>]     ← orchestrates init on all children

orbit transponder add [<alias>]
orbit transponder init [<alias>]

orbit resource add [<alias>]
orbit resource init [<alias>]

orbit vessel defaults add [<key>]
```

### `edit` and `remove` Are Removed

`edit` and `remove` subcommands are removed from all entity commands. Their responsibilities belong to the lifecycle commands:

- `orbit calibrate <alias>` — reconcile and update entity state
- `orbit retro <alias>` — retire and remove entities

This keeps the build layer focused on creation only.

### `planet init` Argument Resolution

`planet init` has the most complex resolution logic due to the git provisioning step:

```text
orbit planet init
  → CWD has .git? adopt in place (prompt for alias + galaxy)
  → else: prompt everything

orbit planet init <alias>
  → alias found in Star Chart? proceed with gap-filling for missing fields
  → alias not found + CWD has .git? adopt existing repo with that alias
  → alias not found + no .git? prompt all with alias pre-filled

orbit planet init <url>
  → prompt for alias + all other decisions, then clone

orbit planet init <alias> <url>
  → all required info present, prompt only for Orbiter decisions
    (galaxy if not inferable, resource conflicts, manager choices)
```

---

## Beacon Lifecycle

Every entity creation writes a Beacon. Beacons are the bridge between desired state (Star Chart) and observed state (reality). They are also the signal that gates the operate layer.

| Event | Beacon Status |
| --- | --- |
| `add` | `unverified` |
| `init` success | `verified` |
| `init` failure | `failed` (with observations) |
| `scan` pass | `healthy` |
| `scan` detects drift | `degraded` |
| `retro` | `retired` |

### Build → Built → Operate Flow

```text
add ──→ [unverified]
             │
           init ──→ [verified] ──→ scan ──→ [healthy]
             │                               │
           [failed]                       [degraded]
             │                               │
             └──────── calibrate ────────────┘
                           │
                       [verified/healthy]

retro available from any state ──→ [retired]
```

**Operational gates:**

- `unverified` and `failed` block `jump`
- `unverified` and `failed` are highlighted in `chart` and `survey`
- `unverified` and `failed` are explicit candidates for `calibrate`
- `retro` is always available as a short-circuit regardless of state

### Beacon Granularity

Beacons are written at the individual entity level — one Beacon per entity. There are no rollup Beacons. A callsign `init` fires init on each child (transponders and resources where `entity_id` = callsign ID) independently; each child writes its own Beacon. The callsign's own Beacon reflects only its own metadata validity.

Path-aware Beacon evaluation (assembling relevant Beacons for a specific `chart`/`jump` path) is out of scope for this phase and will be designed with the lifecycle command implementations.

---

## Context Inference

Commands use the current working directory to infer entity context rather than tracking navigation state separately. Navigation history is reserved for the lifecycle commands (`chart`, `jump`).

**Resolution order for parent context (e.g. galaxy when adding a planet):**

1. Explicit flag (`--galaxy acme`) — always wins
2. CWD matches a known `repo_path` — infer planet → galaxy → solar system
3. CWD is under a known galaxy `dir_path` — infer galaxy (and solar system if applicable)
4. No match found — Captain is outside the Star Chart, prompt explicitly

**Specifying a parent flag cascades cleanly:** providing `--galaxy` does not assume a current solar system. The hierarchy only cascades downward from what is explicitly provided or unambiguously inferred.

---

## Architecture

Four packages are involved. Two are new; two are extended.

```text
internal/
  entity/         ← NEW: FieldDef schemas per entity type
  prompt/         ← NEW: Ask, Choose, Confirm helpers
  starchart/      ← EXTENDED: Create* and Init* functions per entity type
  commands/       ← EXTENDED: add/init handlers wired with entity + prompt
```

### `internal/entity` — Declarative Field Schemas

Pure data. No execution logic, no walkers. Each entity type declares its fields with labels, required status, and optional context-aware suggestions.

```go
type FieldDef struct {
    Column   string
    Label    string
    Flag     string                           // CLI flag name (e.g. "dir-path")
    Required bool
    Suggest  func(ctx context.Context) string // optional context hint
}

// Example
var GalaxyFields = []FieldDef{
    {Column: "name",     Flag: "name",     Label: "Galaxy alias",      Required: true},
    {Column: "dir_path", Flag: "dir-path", Label: "Default repo path", Required: false,
        Suggest: func(ctx context.Context) string { return inferDirFromContext(ctx) }},
}
```

The same `FieldDef` slice drives both the Cobra flag registration and the prompt labels. No duplication between flags and prompts.

### `internal/prompt` — Thin Input Helpers

No entity knowledge. Wraps stdin interaction for styled mode. In JSON mode these functions are never called.

```go
func Ask(label, suggestion string) string
func Choose(label string, opts []string) string
func Confirm(label string, defaultYes bool) bool
```

### `internal/starchart` — Create\* and Init\* Functions

Each entity type gets two functions. Both follow the constitutional integrity rule:
**Prepare → Validate → Execute → Verify → Commit.**

```go
// Create* registers the entity and writes Beacon: unverified atomically
func (sc *StarChart) CreateGalaxy(ctx context.Context, in GalaxyInput) (models.Galaxy, error)
func (sc *StarChart) CreateSolarSystem(ctx context.Context, in SolarSystemInput) (models.SolarSystem, error)
func (sc *StarChart) CreatePlanet(ctx context.Context, in PlanetInput) (models.Planet, error)
func (sc *StarChart) CreateCallsign(ctx context.Context, in CallsignInput) (models.Callsign, error)
func (sc *StarChart) CreateTransponder(ctx context.Context, in TransponderInput) (models.Transponder, error)
func (sc *StarChart) CreateResource(ctx context.Context, in ResourceInput) (models.Resource, error)

// Init* performs real-world provisioning and updates the Beacon to verified or failed
func (sc *StarChart) InitGalaxy(ctx context.Context, id string) error      // mkdir if not exist
func (sc *StarChart) InitSolarSystem(ctx context.Context, id string) error // mkdir if not exist
func (sc *StarChart) InitPlanet(ctx context.Context, in PlanetInitInput) error // clone/adopt/init + scan
func (sc *StarChart) InitCallsign(ctx context.Context, id string) error    // fires Init* on all transponders and resources where entity_id = callsign ID
func (sc *StarChart) InitTransponder(ctx context.Context, id string) error // attempt login/verify reachability
func (sc *StarChart) InitResource(ctx context.Context, id string) error    // install/verify
```

Every `Create*` call writes the alias + entity + Beacon in a single transaction. Failed transactions leave the Star Chart unchanged.

### `internal/commands` — Explicit Handlers

Handlers are readable, entity-specific, and thin. They orchestrate inputs and delegate to starchart — no business logic lives here.

```go
func galaxyAddCmd(d *deps) *cobra.Command {
    var dirPath string
    cmd := &cobra.Command{
        Use:   "add [alias]",
        Short: "Register a galaxy",
        RunE: func(cmd *cobra.Command, args []string) error {
            alias := resolveOrAsk(args, 0, entity.GalaxyFields[0], d.renderer)
            path  := resolveOrAsk(nil, -1, entity.GalaxyFields[1], d.renderer)
            g, err := d.sc.CreateGalaxy(cmd.Context(), starchart.GalaxyInput{
                Name: alias, DirPath: path,
            })
            if err != nil {
                return err
            }
            d.renderer.Success("Galaxy registered", g)
            return nil
        },
    }
    cmd.Flags().StringVar(&dirPath, "dir-path", "", "default repo path for this galaxy")
    return cmd
}
```

---

## `planet init` Provisioning Flow

Once inputs are resolved, all four entry paths converge on this flow:

```text
1. Resolve inputs
   → alias (from args, Star Chart lookup, or prompt)
   → url (from args, existing planet record, or prompt)
   → galaxy (from CWD inference, flag, or prompt)
   → solar system (from CWD inference, flag, or skip)

2. CreatePlanet → Star Chart (Beacon: unverified)
   → if planet already exists: skip, proceed to provision

3. Provision — one of three paths:
   a. URL present and not yet cloned  → git clone into resolved dir_path/<alias>
   b. CWD has .git                    → adopt repo in place, record repo_path
   c. Neither                         → git init new local repo at dir_path/<alias>

4. Scan project files
   → go.mod, package.json, .nvmrc, Cargo.toml, pyproject.toml, global.json, etc.
   → infer: language, runtime, version manager, version

5. Resolve discovered resources against defaults
   For each discovered resource:
     a. Conflict with vessel/galaxy default:
        → prompt: "Repo specifies X, vessel default is Y. Use [X]?" (repo wins by default)
     b. No default configured and no repo specification:
        → prompt: choose manager, set as default (vessel/galaxy/planet), or bare install
     c. Match with existing default:
        → accept silently, no prompt

6. Register resources (each with Beacon: unverified)

7. Run InitResource for each registered resource
   → Beacon per resource: verified or failed with observations

8. Update planet Beacon
   → all resources verified → planet Beacon: verified
   → any resource failed    → planet Beacon: failed (observations list failures)
```

### Example Conflict Prompts

```text
Found: Python project (pyproject.toml)
Vessel default: uv
Repo specifies: pip

Use repo manager (pip) or vessel default (uv)? [pip]
```

```text
Found: Python project (pyproject.toml)
No version manager configured.

Options:
  1. Set uv as vessel default (applies to all Python planets)
  2. Use uv for this planet only
  3. Bare install (no manager)
```

---

## Output and JSON/TUI Interface

All output routes through the existing `output.Renderer` interface. No `fmt.Println` in command handlers.

| Mode | Behavior |
| --- | --- |
| Styled (default) | Lipgloss-rendered prompts, progress, Beacon status updates |
| JSON | Machine-readable, non-interactive |

### JSON Mode Rules

JSON mode is non-interactive. Every field that can be prompted must also be expressible as a flag. If a required field is missing in JSON mode, the command returns a structured error immediately rather than prompting.

```bash
# Fully specified — no prompts, clean JSON output
orbit galaxy add acme --dir-path ~/projects/acme --output json
# {"id": "0k3m2r4agx7b2c1f", "name": "acme", "dir_path": "~/projects/acme", "beacon": "unverified"}

# Missing required field — structured error
orbit galaxy add --output json
# {"error": "missing required field: alias"}
```

The TUI (`orbiter`) constructs fully-specified commands and parses JSON responses. This makes `orbit` a clean subprocess interface — the TUI never needs to parse styled output or drive interactive prompts.

### Flag Completeness Requirement

Every `FieldDef` in `internal/entity` that has `Required: true` or that can be prompted must have a corresponding `Flag` name. Cobra flag registration is driven from the same `FieldDef` slice as prompt labels. The two modes are always in sync.

---

## Star Chart Integrity

All `Create*` and `Init*` operations follow the constitutional integrity rule:

```text
Prepare  → validate inputs, check for conflicts, resolve parent IDs
Validate → verify no duplicate aliases, referential integrity
Execute  → perform real-world action (mkdir, git clone, install, login)
Verify   → confirm the action succeeded in reality
Commit   → write to Star Chart + update Beacon
```

Failed operations at any step do not modify the Star Chart. If `Execute` or `Verify` fails, the entity is either not written (for `Create*`) or the Beacon is updated to `failed` with observations (for `Init*`).

---

## Entities Omitted From This Phase

- **Vessel `add`/`init`**: The vessel is a single-row entity seeded at first run. `vessel defaults add` is in scope; vessel creation itself is not a user-facing command.
- **Navigation History**: Read-only log, no build commands.
- **Beacons**: Written as a side effect of `add`/`init`. No direct Beacon creation commands.
- **Alias management**: Aliases are created atomically with entities. No standalone alias commands in this phase.
