# Orbiter — CRUD Commands Design

**Date:** 2026-06-09
**Revised:** 2026-06-10
**Phase:** 2 — Entity Build Commands
**Status:** Draft

---

## Overview

This document specifies the CRUD command layer for Orbiter — the "build" operations that populate the Star Chart with entities. These commands are intentionally separate from the six lifecycle commands that operate the universe.

The six lifecycle commands (`survey`, `chart`, `jump`, `scan`, `calibrate`, `retro`) assume a populated Star Chart. The build commands (`add`, `init`, `attach`) create and wire together the universe those commands operate on.

---

## Two-Layer Model

Orbiter has two distinct command layers with different philosophies:

| Layer | Commands | Style | Purpose |
| --- | --- | --- | --- |
| **Build** | `add`, `init`, `attach` | Noun-first, entity-specific | Populate and wire the Star Chart |
| **Operate** | `survey`, `chart`, `jump`, `scan`, `calibrate`, `retro` | Verb-first, lifecycle | Run the universe |

The build layer itself has two sub-phases:

- **Create** — `add` and `init` register standalone entities
- **Combine** — `attach` wires entities together into projects

---

## Universe Model (Revised)

### Hierarchy Containers

Galaxies, solar systems, and planets are **pure hierarchy containers**. Their rows exist only to anchor the entity tree. All operational properties (filesystem paths, repository URLs, tooling) are resources attached to them — not columns on the table.

```text
Vessel (workstation — unique, single row)
  └── Galaxy (org/client alias)
        ├── Solar System (optional team alias)
        │     └── Planet (project alias — orchestrator)
        └── Planet
```

Hierarchy tables carry only their `id` and structural foreign keys:

```sql
galaxies      (id, created_at)
solar_systems (id, galaxy_id, created_at)
planets       (id, galaxy_id, solar_system_id, created_at)
```

### Resources — The Universal Building Block

Resources are standalone, typed entities that describe anything operational: filesystem paths, runtimes, tools, APIs, package registries, credential stores, git repositories, S3 buckets, or any other capability.

```sql
resources (id, kind, manager, version, config, created_at)
```

| Field | Purpose |
| --- | --- |
| `kind` | Drives behavior: `filesystem`, `git`, `s3`, `runtime`, `tool`, `api`, `credential-store`, `config`, ... |
| `manager` | How to install or activate (nvm, uv, brew, rustup, etc.) — optional |
| `version` | Required or desired version — optional |
| `config` | JSON blob for kind-specific fields (url, path, endpoint, region, etc.) |

**Resources are not inherently scoped.** They exist as standalone definitions. What scopes them is the attachment.

**Resources can depend on other resources.** `node@20-nvm` depends on `nvm`. `private-npm-registry` depends on an npm credential. These dependencies are attachments — the same model as everything else.

**The `source` concept dissolves into resource kinds.** There is no separate source entity. A git repository, an S3 asset bucket, a Perforce depot, a local project directory — all are resources with the appropriate `kind` and `config`. This makes the model usable by any professional, not just software engineers.

### Transponders — The Security Boundary

Transponders are **semantically separate from resources** even though they are technically similar in structure. This separation is intentional and enforced by the schema.

```sql
transponders (id, service, location, created_at)
```

Transponders point to credential locations and authentication services. They never store secrets. The separation from resources ensures security semantics never bleed into general resource management: resources that require credentials explicitly depend on transponders, and that dependency is visible and queryable.

Resources can be "locked" behind a callsign's transponders. A private git repo resource requires a specific transponder to be active. Callsign context determines which transponders are live and therefore which locked resources are accessible.

### Callsigns — Identity and Auth Isolation

A callsign packages a set of transponders into an active identity context. Only one callsign is active per navigation context. Callsign activation is the mechanism that enforces credential isolation between clients.

```sql
callsigns (id, created_at)
```

Transponders and resources attach to callsigns via the attachments table. When a callsign is active, its transponders are available and its resources apply.

### Attachments — The Shared Join Model

All relationships between entities are expressed through a single attachments table. This replaces the scattered `entity_id` and `callsign_id` foreign keys that previously baked ownership into entity rows.

```sql
attachments (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),  -- what is being attached
    target_id  TEXT NOT NULL REFERENCES aliases(id),  -- what it is attached to
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, target_id)
)
```

Entities are reusable across any number of targets. A `node@20-nvm` resource is defined once and attached to a vessel, a galaxy, and three planets — four rows in `attachments`, one row in `resources`.

---

## Inheritance and Override Resolution

The hierarchy provides precedence for resolving the effective set of resources and callsigns for any entity.

**Resolution order (lowest wins):**

```text
Vessel → Galaxy → Solar System → Planet → (active Callsign)
```

For resources of the same `kind`, the attachment at the most specific level in the path wins. Higher-level attachments fill in any kinds not overridden below.

### Example: Credential Isolation

```text
Vessel callsign: kent-personal
  Transponder: claude-personal-api   ← my subscription tokens

Galaxy: acme
  Callsign: kent-acme                ← overrides vessel callsign in acme context
    Transponder: claude-acme-api     ← client's tokens

Effective when navigating to acme/payment-api:
  Active callsign:  kent-acme
  Active API key:   claude-acme-api  ← client pays, not me
```

The same override model applies to resources:

```text
Vessel:               node@20-nvm, git@2.x, docker@latest
Galaxy (acme):        node@18-nvm           ← overrides node kind for all acme planets
Planet (payment-api): node@20-nvm           ← overrides node kind for this planet only

Effective for payment-api: node@20-nvm, git@2.x, docker@latest
```

### Resource Dependency Resolution

Resources can depend on other resources. `planet init` resolves the full dependency graph before executing, depth-first:

```text
node@20-nvm
  └── requires: nvm@latest
        └── requires: filesystem:/usr/local
```

All dependencies are verified or installed before the dependent resource is initialized.

---

## Beacon Lifecycle

Every entity creation writes a Beacon. Beacons are the bridge between desired state (Star Chart) and observed state (reality). They gate the operate layer.

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

Beacons are written at the individual entity level — one Beacon per entity. Planets and callsigns are orchestrators: their `init` fires `init` on all attached children independently. Each child writes its own Beacon. Path-aware Beacon evaluation (assembling relevant Beacons for a specific `chart`/`jump` path) is out of scope for this phase.

---

## Command Surface

### Create Commands (`add` / `init`)

```text
orbit galaxy add [<alias>]
orbit galaxy init [<alias>]

orbit system add [<alias>]
orbit system init [<alias>]

orbit planet add [<alias>]
orbit planet init [<alias>] [<url-or-path>]

orbit callsign add [<alias>]
orbit callsign init [<alias>]     ← orchestrates init on all attached transponders + resources

orbit transponder add [<alias>]
orbit transponder init [<alias>]

orbit resource add [<alias>]
orbit resource init [<alias>]

orbit vessel defaults add [<key>]
```

### Combine Commands (`attach`)

```text
orbit attach <entity-alias> <target-alias>
```

Attaches any entity to any target. The relationship type is inferred from the entity kinds involved. Attachments are directional: `entity` is attached to `target`.

Examples:

```bash
orbit attach node@20-nvm payment-api     # resource → planet
orbit attach node@20-nvm acme            # resource → galaxy (applies to all acme planets)
orbit attach nvm node@20-nvm             # resource → resource (dependency)
orbit attach kent-acme acme              # callsign → galaxy
orbit attach github-acme-key kent-acme   # transponder → callsign
orbit attach private-repo payment-api    # git resource → planet
```

### `edit` and `remove` Are Not Build Commands

`edit` and `remove` belong to the lifecycle layer:

- `orbit calibrate <alias>` — reconcile and update entity state
- `orbit retro <alias>` — retire and remove entities

---

## `planet init` Argument Resolution

`planet init` is the most complex command because a planet is an orchestrator — its init fires init on all attached resources.

```text
orbit planet init
  → CWD matches known planet? init that planet
  → else: prompt for alias + context

orbit planet init <alias>
  → alias found in Star Chart? init all attached resources
  → alias not found + CWD has .git or other detectable project?
      → register planet with alias, infer attachable resources, prompt to attach + init
  → alias not found + no detectable project?
      → prompt all with alias pre-filled

orbit planet init <url-or-path>
  → prompt for alias + context, then provision

orbit planet init <alias> <url-or-path>
  → all required info present, prompt only for Orbiter decisions
```

**The provisioning flow once inputs are resolved:**

```text
1. Resolve inputs (alias, galaxy from CWD or prompt)
2. CreatePlanet if not exists → Beacon: unverified

3. Detect project kind from directory or url:
   → .git present / git url → create git resource if not attached
   → other detectable kinds (s3 url, etc.) → create appropriate resource

4. Scan project files for resource hints
   → go.mod, package.json, .nvmrc, Cargo.toml, pyproject.toml, global.json, etc.
   → infer: runtime kind, version, manager

5. Resolve each discovered resource against inherited defaults
   → Conflict (repo says X, inherited says Y): prompt (repo wins by default)
   → No default, no repo spec: prompt to choose manager / set default / bare install
   → Match: accept silently

6. Attach resolved resources to planet
   → CreateResource if not exists → attach to planet

7. Resolve dependency graph for all attached resources
   → CreateResource for any missing dependencies → attach to parent resource

8. InitResource depth-first for each resource in dependency order
   → Each resource: Beacon verified or failed

9. Update planet Beacon
   → all required resources verified → verified
   → any required resource failed    → failed with observations
```

### Conflict Prompt Examples

```text
Found: Python project (pyproject.toml)
Vessel default: uv
Project specifies: pip

Use project manager (pip) or vessel default (uv)? [pip]
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

## Context Inference

Commands use the current working directory to infer entity context. Navigation history is reserved for the lifecycle commands.

**Resolution order for parent context:**

1. Explicit flag (`--galaxy acme`) — always wins
2. CWD matches a known `repo_path` in a git resource attached to a planet → infer planet → galaxy → solar system
3. CWD is under a filesystem resource attached to a galaxy → infer galaxy (and solar system if applicable)
4. No match found — Captain is outside the Star Chart, prompt explicitly

Specifying a parent flag does not cascade assumptions downward. `--galaxy acme` does not assume a solar system.

---

## Architecture

Four packages are involved. Two are new; two are extended.

```text
internal/
  entity/         ← NEW: FieldDef schemas per entity type
  prompt/         ← NEW: Ask, Choose, Confirm helpers
  starchart/      ← EXTENDED: Create*, Init*, Attach functions
  commands/       ← EXTENDED: add/init/attach handlers
```

### `internal/entity` — Declarative Field Schemas

Pure data. No execution logic. Each entity type declares its fields with labels, required status, and optional context-aware suggestions. The same `FieldDef` slice drives Cobra flag registration and prompt labels — no duplication.

```go
type FieldDef struct {
    Column   string
    Label    string
    Flag     string                           // CLI flag name
    Required bool
    Suggest  func(ctx context.Context) string // optional context hint
}
```

### `internal/prompt` — Thin Input Helpers

No entity knowledge. Wraps stdin interaction for styled mode only — never called in JSON mode.

```go
func Ask(label, suggestion string) string
func Choose(label string, opts []string) string
func Confirm(label string, defaultYes bool) bool
```

### `internal/starchart` — Create\*, Init\*, and Attach

All functions follow Prepare → Validate → Execute → Verify → Commit.

```go
// Create* registers the entity + writes Beacon: unverified atomically
func (sc *StarChart) CreateGalaxy(ctx context.Context, in GalaxyInput) (models.Galaxy, error)
func (sc *StarChart) CreatePlanet(ctx context.Context, in PlanetInput) (models.Planet, error)
func (sc *StarChart) CreateCallsign(ctx context.Context, in CallsignInput) (models.Callsign, error)
func (sc *StarChart) CreateTransponder(ctx context.Context, in TransponderInput) (models.Transponder, error)
func (sc *StarChart) CreateResource(ctx context.Context, in ResourceInput) (models.Resource, error)

// Init* provisions in the real world + updates Beacon
func (sc *StarChart) InitGalaxy(ctx context.Context, id string) error       // init attached filesystem resources
func (sc *StarChart) InitSolarSystem(ctx context.Context, id string) error  // init attached filesystem resources
func (sc *StarChart) InitPlanet(ctx context.Context, id string) error       // init all attached resources (orchestrator)
func (sc *StarChart) InitCallsign(ctx context.Context, id string) error     // init all attached transponders + resources
func (sc *StarChart) InitTransponder(ctx context.Context, id string) error  // verify credential reachability
func (sc *StarChart) InitResource(ctx context.Context, id string) error     // install/verify (kind-specific handler)

// Attach creates a relationship between two entities
func (sc *StarChart) Attach(ctx context.Context, entityID, targetID string) error

// ResolveEffective walks the hierarchy and returns the effective resource set for a target
func (sc *StarChart) ResolveEffective(ctx context.Context, targetID string) ([]models.Resource, error)
```

---

## Output and JSON/TUI Interface

All output routes through the existing `output.Renderer`. No `fmt.Println` in command handlers.

| Mode | Behavior |
| --- | --- |
| Styled (default) | Lipgloss-rendered prompts, progress, Beacon status updates |
| JSON | Machine-readable, non-interactive |

JSON mode is non-interactive. Missing required fields return a structured error rather than prompting. Every promptable field must be expressible as a flag — same `FieldDef` drives both.

```bash
orbit galaxy add acme --output json
# {"id": "0k3m2r4agx7b2c1f", "name": "acme", "beacon": "unverified"}

orbit galaxy add --output json
# {"error": "missing required field: alias"}

orbit attach node@20-nvm payment-api --output json
# {"entity": "node@20-nvm-id", "target": "payment-api-id", "created_at": "..."}
```

The TUI (`orbiter`) constructs fully-specified commands and parses JSON responses. `orbit` is a clean subprocess interface — the TUI never drives interactive prompts.

---

## Star Chart Integrity

All `Create*`, `Init*`, and `Attach` operations follow:

```text
Prepare  → validate inputs, check for conflicts, resolve IDs
Validate → no duplicate aliases, referential integrity, valid attachment pairs
Execute  → real-world action (mkdir, git clone, install, credential check)
Verify   → confirm the action succeeded in reality
Commit   → write to Star Chart + update Beacon
```

Failed operations at any step do not modify the Star Chart.

---

## Schema Changes Required

The following changes to `0001_initial.sql` are needed before implementation:

**Remove from existing tables:**

- `planets.repo_url`, `planets.repo_path` → become `config` on a `git` resource
- `callsigns.entity_id` → replaced by attachments
- `transponders.callsign_id`, `transponders.entity_id` → replaced by attachments
- `resources.entity_id` → replaced by attachments

**Modify `resources`:**

- Add `config TEXT` for kind-specific JSON data
- `kind`, `manager`, `version` stay as top-level fields for queryability

**Add:**

- `attachments` table (see Universe Model section above)

These changes will be delivered as migration `0002_attachments.sql`.

---

## Entities Omitted From This Phase

- **Vessel `add`/`init`**: Vessel is seeded at first run. `vessel defaults add` is in scope; vessel creation is not user-facing.
- **Navigation History**: Read-only log, no build commands.
- **Beacons**: Written as a side effect of `add`/`init`. No direct Beacon creation commands.
- **Alias management**: Aliases are created atomically with entities. No standalone alias commands in this phase.
- **Path-aware Beacon evaluation**: How `chart`/`jump`/`scan` assemble relevant Beacons for a path is deferred to the lifecycle command phase.
