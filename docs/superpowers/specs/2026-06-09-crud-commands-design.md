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

The build layer has two sub-phases:

- **Create** — `add` and `init` register standalone entities
- **Combine** — `attach` wires entities together into the graph

---

## Universe Model

### Hierarchy Containers

Galaxies, solar systems, and planets are **pure hierarchy containers**. Their rows exist only to anchor the tree. All operational properties (paths, URLs, tooling, credentials) come from entities attached to them — not from columns on the table.

```text
Vessel  (workstation — unique, single row)
  └── Galaxy  (org/client alias)
        ├── Solar System  (optional team alias)
        │     └── Planet  (project alias — orchestrator)
        └── Planet
```

```sql
galaxies      (id, created_at)
solar_systems (id, galaxy_id, created_at)
planets       (id, galaxy_id, solar_system_id, created_at)
```

### Resources — The Universal Building Block

Resources are standalone typed entities describing anything operational: filesystem paths, runtimes, tools, APIs, package registries, version managers, git repositories, S3 buckets, or any other capability.

```sql
resources (id, kind, manages, version, config, created_at)
```

| Field | Purpose |
| --- | --- |
| `kind` | Drives behavior and init dispatch: `manager`, `runtime/node`, `runtime/python`, `tool/git`, `filesystem`, `api`, `vcs/git`, ... |
| `manages` | JSON array of kinds this resource manages. Non-null only when `kind="manager"`. e.g. `["runtime/node", "runtime/python"]`. Specific integrations (nvm → runtime/node) are wired in implementation, not defined here. |
| `version` | Required or desired version — optional |
| `config` | JSON blob for kind-specific fields (url, path, endpoint, org, region, etc.) |

Resources are **standalone definitions**. They are not inherently scoped to any entity. Scope is determined entirely by attachments.

Resources that are managers (`kind="manager"`) reign over the resource kinds listed in their `manages` array. The branch crawl uses this to match managers to the runtimes and tools they install.

### Transponders — The Security Boundary

Transponders are semantically separate from resources. This separation is intentional: credential pointers must never be conflated with general operational resources.

```sql
transponders (id, service, location, created_at)
```

Transponders point to credential locations and authentication services. They never store secrets. They attach to callsigns via the attachments table.

### Callsigns — Identity Context

A callsign packages a set of transponders for a specific identity context. **Each hierarchy node holds at most one callsign.** This constraint is enforced at `attach` time.

```sql
callsigns (id, created_at)
```

Transponders attach to callsigns. Callsigns attach to hierarchy nodes (vessel, galaxy, system, planet). The branch crawl collects callsigns and their transponders as it descends.

### Attachments — The Graph Edges

All relationships between entities are a single attachment. The graph structure alone carries all semantic meaning — no relationship type column, no extra fields.

```sql
attachments (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    target_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, target_id)
)
```

One callsign per hierarchy node is enforced at the application layer on `attach`.

---

## Branch Crawl and Override Resolution

The branch crawl is the core resolution algorithm. It runs top-down from vessel to the target node and produces the effective set for initialization.

```text
Vessel → Galaxy → Solar System → Planet
```

**Lowest level wins per kind** — for any given kind, the attachment closest to the target node in the branch overrides anything at a higher level.

### Resource Resolution

```text
Vessel:   nvm    (kind=manager, manages=["runtime/node"])
Planet:   native (kind=manager, manages=["runtime/node"])  ← overrides nvm
Planet:   node@20 (kind=runtime/node)

Effective manager for runtime/node at this planet: native
Init: native installs node@20
```

### Transponder Resolution

Callsigns are collected as the crawl descends. Transponders are collected from each callsign. Lower-level transponder wins per `service` kind:

```text
Vessel → kent-personal
  figma-personal-key (service=figma)
  github-personal    (service=github)
  claude-personal    (service=claude)

Galaxy: acme → kent-acme
  github-acme-key    (service=github)  ← overrides github-personal
  claude-acme-api    (service=claude)  ← overrides claude-personal

Effective transponders for acme/payment-api:
  figma-personal-key  (fell through from vessel — acceptable)
  github-acme-key     (galaxy override)
  claude-acme-api     (galaxy override)
```

Fallthrough to a higher-level transponder when no lower-level override exists is **intentional and acceptable**. It means the Captain forgot to add one, but a sensible default exists.

### Missing Resource or Transponder Handling

| Scenario | Behavior |
| --- | --- |
| Manager found for kind | Use it to install |
| No manager, kind supports bare install | Proceed bare, warn |
| No manager, kind requires one | Fail: "no manager for X — attach one at vessel or planet" |
| Transponder found for service | Use it |
| No transponder for required service | Fail: "no transponder of service X in branch" |

Specific knowledge of which kinds require managers and which support bare install lives in the init handler per kind — not in the data model.

---

## Beacon Lifecycle

Every entity creation writes a Beacon. Beacons bridge desired state (Star Chart) and observed reality. They gate the operate layer.

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
- `unverified` and `failed` are candidates for `calibrate`
- `retro` short-circuits from any state

### Beacon Granularity

One Beacon per entity. No rollups. Planets and callsigns are orchestrators — their `init` fires `init` on all attached children independently. Each child writes its own Beacon. Path-aware Beacon evaluation (assembling Beacons for a `chart`/`jump` path) is deferred to the lifecycle command phase.

---

## Command Surface

### Create Commands

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

### Combine Command

```text
orbit attach <entity-alias> <target-alias>
```

Wires any entity to any target. Enforces one-callsign-per-node at runtime.

```bash
orbit attach nvm vessel                  # resource → vessel
orbit attach native payment-api          # resource → planet (overrides vessel nvm for this planet)
orbit attach node@20 payment-api         # resource → planet
orbit attach kent-acme acme              # callsign → galaxy (one per node)
orbit attach github-acme-key kent-acme   # transponder → callsign
orbit attach figma-app vessel            # resource → vessel (available everywhere)
```

### `edit` and `remove` Are Not Build Commands

- `orbit calibrate <alias>` — reconcile and update
- `orbit retro <alias>` — retire and remove

---

## `planet init` Argument Resolution

```text
orbit planet init
  → CWD matches known planet? init that planet
  → else: prompt for alias + galaxy

orbit planet init <alias>
  → alias found in Star Chart? run branch crawl, init all attached resources
  → alias not found + CWD has detectable project? register planet, discover resources, prompt to attach + init
  → alias not found + nothing detectable? prompt all with alias pre-filled

orbit planet init <url-or-path>
  → prompt for alias + galaxy, then provision

orbit planet init <alias> <url-or-path>
  → satisfied — run, prompt only for Orbiter decisions
```

**Provisioning flow:**

```text
1. Resolve alias + galaxy (from args, CWD, or prompt)
2. CreatePlanet if not exists → Beacon: unverified

3. Provision repo:
   a. URL provided   → git clone (or protocol-appropriate clone) into dir
   b. CWD has .git   → adopt in place
   c. Neither        → git init new local repo

4. Scan project files (go.mod, package.json, .nvmrc, Cargo.toml, pyproject.toml, etc.)
   → infer resource kinds and versions

5. Run branch crawl → get effective managers + transponders

6. For each discovered resource:
   a. Matching manager in effective set → attach resource, queue for init
   b. No manager, kind supports bare → attach, warn, queue
   c. No manager, kind requires one → prompt: attach a manager or skip
   d. Conflict with inherited resource of same kind → prompt (current branch wins by default)

7. InitResource for each attached resource (depth-first via manages graph)
   → each resource: Beacon verified or failed

8. Update planet Beacon: verified if all required resources verified, else failed
```

---

## Context Inference

CWD is the compass for context. Navigation history is for lifecycle commands only.

**Resolution order:**

1. Explicit flag (`--galaxy acme`) — always wins
2. CWD matches a `vcs/git` or `filesystem` resource config path attached to a planet → infer planet → galaxy → system
3. CWD is under a `filesystem` resource attached to a galaxy → infer galaxy
4. No match → prompt explicitly

Specifying a parent flag does not cascade downward assumptions.

---

## Architecture

```text
internal/
  entity/         ← NEW: FieldDef schemas per entity type
  prompt/         ← NEW: Ask, Choose, Confirm helpers
  starchart/      ← EXTENDED: Create*, Init*, Attach, BranchCrawl
  commands/       ← EXTENDED: add/init/attach handlers
```

### `internal/entity` — Declarative Field Schemas

Pure data. Same `FieldDef` slice drives Cobra flag registration and prompt labels.

```go
type FieldDef struct {
    Column   string
    Label    string
    Flag     string
    Required bool
    Suggest  func(ctx context.Context) string
}
```

### `internal/prompt` — Thin Input Helpers

Never called in JSON mode.

```go
func Ask(label, suggestion string) string
func Choose(label string, opts []string) string
func Confirm(label string, defaultYes bool) bool
```

### `internal/starchart` — Core Functions

```go
// Create* registers entity + Beacon: unverified atomically
func (sc *StarChart) CreateGalaxy(ctx, in GalaxyInput) (models.Galaxy, error)
func (sc *StarChart) CreatePlanet(ctx, in PlanetInput) (models.Planet, error)
func (sc *StarChart) CreateCallsign(ctx, in CallsignInput) (models.Callsign, error)
func (sc *StarChart) CreateTransponder(ctx, in TransponderInput) (models.Transponder, error)
func (sc *StarChart) CreateResource(ctx, in ResourceInput) (models.Resource, error)

// Init* provisions in the real world + updates Beacon
func (sc *StarChart) InitGalaxy(ctx, id string) error
func (sc *StarChart) InitSolarSystem(ctx, id string) error
func (sc *StarChart) InitPlanet(ctx, id string) error       // orchestrator
func (sc *StarChart) InitCallsign(ctx, id string) error     // orchestrator
func (sc *StarChart) InitTransponder(ctx, id string) error
func (sc *StarChart) InitResource(ctx, id string) error     // kind-dispatched

// Graph
func (sc *StarChart) Attach(ctx, entityID, targetID string) error
func (sc *StarChart) BranchCrawl(ctx, targetID string) (BranchContext, error)
```

`BranchContext` carries the fully resolved effective set — resources, managers, callsigns, transponders — for a given position in the hierarchy. All `Init*` functions call `BranchCrawl` first.

---

## Output and JSON/TUI Interface

All output through `output.Renderer`. No `fmt.Println` in handlers.

JSON mode is non-interactive. Every promptable field is expressible as a flag (same `FieldDef` drives both). Missing required field in JSON mode → structured error, no prompt.

```bash
orbit resource add nvm --kind manager --manages '["runtime/node"]' --output json
# {"id": "...", "name": "nvm", "kind": "manager", "manages": ["runtime/node"], "beacon": "unverified"}

orbit attach nvm vessel --output json
# {"entity": "nvm-id", "target": "vessel-id"}

orbit resource add --output json
# {"error": "missing required field: alias"}
```

The TUI (`orbiter`) constructs fully-specified commands and parses JSON. It never drives interactive prompts.

---

## Star Chart Integrity

All `Create*`, `Init*`, and `Attach` operations follow:

```text
Prepare  → validate inputs, check for conflicts, resolve IDs
Validate → no duplicate aliases, one callsign per node, valid attachment pairs
Execute  → real-world action (mkdir, clone, install, credential check)
Verify   → confirm action succeeded in reality
Commit   → write to Star Chart + update Beacon
```

Failed operations at any step do not modify the Star Chart.

---

## Schema Changes (migration 0002)

**Remove from existing tables:**

- `planets.repo_url`, `planets.repo_path` → become `config` on a `vcs/git` resource
- `callsigns.entity_id` → replaced by attachments
- `transponders.callsign_id`, `transponders.entity_id` → replaced by attachments
- `resources.entity_id`, `resources.manager` → replaced by attachments + `manages` column

**Modify `resources`:**

- Add `manages TEXT` — JSON array of resource kinds; non-null only for `kind="manager"`
- Add `config TEXT` — JSON blob for kind-specific fields
- `kind` and `version` stay as top-level columns for queryability

**Add:**

- `attachments` table (see above)

---

## Out of Scope for This Phase

- Specific manager-to-kind integrations (nvm → runtime/node, etc.) — init handler implementations
- Path-aware Beacon evaluation for `chart`/`jump`/`scan`
- Vessel seeding at first run (vessel creation is not user-facing)
- Navigation history (read-only log)
- Direct Beacon creation commands (Beacons are side effects of `add`/`init`)
- Standalone alias management
