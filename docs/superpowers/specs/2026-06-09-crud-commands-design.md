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

## Orbiter Scope

Orbiter manages **environment configuration state**. It does not manage data state.

**Drift in Orbiter** means: divergence between what the Star Chart declares as desired environment state and the actual installed, configured, or authenticated state of tools, runtimes, credentials, and connections.

**Not drift:** file contents, sync state, upstream changes, runtime behavior, data in databases.

A `remote + dropbox` resource means: the Dropbox client is installed, authenticated, and configured to the right sync folder. Whether files inside that folder are current is Dropbox's domain, not Orbiter's.

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

Galaxies, solar systems, and planets are **pure hierarchy containers**. Their rows exist only to anchor the tree. All operational properties come from entities attached to them — not from columns on the table.

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

Resources are standalone typed entities. The combination of `role` and `brand` is what drives behavior — `role` classifies the resource, `brand` identifies the specific implementation.

```sql
resources (id, role, brand, manages, version, config, created_at)
```

| Field | Purpose |
| --- | --- |
| `role` | Classification — what the resource *is* |
| `brand` | Specific implementation — which one |
| `manages` | JSON array of brands this resource manages. Non-null only when `role=manager` |
| `version` | Required or desired version — optional |
| `config` | JSON blob for role+brand-specific fields (url, path, endpoint, etc.) |

**Resource roles:**

| Role | Represents | Example brands |
| --- | --- | --- |
| `manager` | Installs and manages other resources | nvm, volta, homebrew, apt, uv, conda |
| `runtime` | Application that runs on the machine | node, python, ruby, postgres, figma |
| `tool` | CLI tool or command-line application | git, docker, kubectl, make |
| `remote` | Remote sync target accessed via protocol | github, dropbox, s3, onedrive, gdrive |
| `filesystem` | Local path or directory | (config-driven) |

Examples of role+brand combinations:

```text
manager  + nvm      = nvm version manager
manager  + homebrew = Homebrew package manager
runtime  + node     = Node.js runtime
runtime  + figma    = Figma desktop application
runtime  + postgres = PostgreSQL database process
tool     + git      = git CLI
tool     + aws      = AWS CLI
remote   + github   = a GitHub repository
remote   + dropbox  = a Dropbox sync folder
remote   + figma    = Figma file sync target
filesystem + —      = a local path or directory
```

`runtime + figma` (the running desktop application) and `remote + figma` (a sync target for Figma design files) are distinct resources and valid simultaneously.

Resources are **standalone definitions**. Scope is determined entirely by attachments.

### Transponders — The Security Boundary

Transponders point to credential locations. They never store secrets. The `role` describes the access mechanism; the `brand` identifies the service.

```sql
transponders (id, role, brand, location, created_at)
```

| Field | Purpose |
| --- | --- |
| `role` | Access mechanism — where/how the credential is stored |
| `brand` | Service this credential grants access to |
| `location` | The specific pointer (file path, env var name, keychain item, vault path) |

**Transponder roles:**

| Role | Represents |
| --- | --- |
| `file` | Path to a key or cert file on disk |
| `env` | Environment variable name |
| `keychain` | OS keychain or credential store reference |
| `vault` | External secret manager (1Password, Doppler, HashiCorp Vault, AWS Secrets Manager) |
| `agent` | Auth agent socket (SSH agent, GPG agent) |

A transponder never contains the credential itself — only where to find it and how to set it up.

### Callsigns — Identity Context

A callsign packages a set of transponders for a specific identity context. **Each hierarchy node holds at most one callsign.** This constraint is enforced at `attach` time.

```sql
callsigns (id, created_at)
```

Transponders attach to callsigns. Callsigns attach to hierarchy nodes. The branch crawl collects callsigns and their transponders as it descends.

### Attachments — The Graph Edges

All relationships between entities are a single attachment table. The graph structure alone carries all semantic meaning.

```sql
attachments (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    target_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, target_id)
)
```

---

## Branch Crawl and Override Resolution

The branch crawl is the core resolution algorithm. It is a two-phase operation: resolve everything first, then execute integrations against the fully-resolved state.

### Phase 1 — Resolve

Walk top-down: vessel → galaxy → solar system → planet.

Maintain a current effective map: `{ role+brand → transponder }`. At each level:

1. Push new transponders from that level's callsign — updates the effective map going forward
2. Collect resources attached at this level — each resource is **snapshotted with the current effective map** at the moment of collection

This is FILO pairing: lower levels update the effective transponder state, but resources already collected at higher levels keep their pairing. A galaxy resource uses the galaxy-effective transponder. A planet resource uses the planet-effective transponder. The wrong transponder is never applied to a resource from a different level.

**Why this matters:**

```text
Vessel callsign:  file+github → personal-key
Galaxy callsign:  file+github → acme-key       ← overrides personal going forward
Planet (payment-api):
  resource: remote+github (acme repo)  → paired with acme-key       ✓
  resource: remote+github (personal lib) needs planet-level transponder
    ↓
  Planet callsign: file+github → personal-key  ← overrides acme going forward
  resource: remote+github (personal lib) → paired with personal-key ✓
```

Resources at different levels get the transponder that was active when they were collected.

Phase 1 output: a set of `(resource, transponder-pairings)` tuples organized into **dependency graphs** — managers before what they manage, independent resources identified for parallel execution.

### Phase 2 — Execute

Execute dependency graphs from Phase 1:

- **Independent graphs run in parallel**
- **Within each graph**, managers execute before what they manage
- Each integration's `StateReport` is fed into the `ResolvedContext` for subsequent calls in that graph

No integration is called during Phase 1. The full effective set is resolved before any integration runs. This prevents race conditions (gh CLI authenticated multiple times, mid-crawl partial state, etc.).

### Resource Resolution

```text
Vessel:   nvm    (role=manager, brand=nvm, manages=["node"])
Planet:   native (role=manager, brand=native, manages=["node"])  ← overrides nvm

Effective manager for node at this planet: native
node@20.Init receives native's StateReport in ResolvedContext
```

### Transponder Resolution

```text
Vessel callsign → kent:
  file+github    → personal key
  oauth+figma    → personal figma

Galaxy callsign → kent-acme:
  api-key+claude → acme-billed key    ← overrides personal claude
  (no figma override)

Effective transponders working in acme/payment-api:
  api-key+claude  → acme-billed-key   (galaxy override — client pays)
  oauth+figma     → personal figma    (fallthrough — acceptable, Captain's responsibility)
```

Fallthrough to a higher-level transponder when no lower-level override exists is **intentional and acceptable**. Multi-auth scope issues are the Captain's responsibility via Star Chart organization.

### Missing Resource or Transponder

| Scenario | Behavior |
| --- | --- |
| Manager found for brand | Use it |
| No manager in branch; integration has default | Use integration default |
| No manager in branch; integration has no default | Fail — report why; Captain resolves |
| Transponder found for role+brand | Use it |
| No transponder for required role+brand | Fail — report why |

---

## Beacon Lifecycle

Every entity creation writes a Beacon. Beacons bridge desired state (Star Chart) and observed reality.

| Event | Beacon Status |
| --- | --- |
| `add` | `unverified` |
| `init` success | `verified` |
| `init` failure | `failed` |
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

### Scan and Calibrate Behavior

`scan` always re-derives state by calling each integration's `Scan` method against current reality. Orbiter never uses cached state for a scan — the integration reports what exists now. Orbiter computes drift by comparing the live `StateReport` to the expected config in the Star Chart.

`calibrate` calls each integration's `Calibrate` method with the desired state from the Star Chart in `ResolvedContext`. The integration brings reality into alignment and reports the resulting state. Calibrate is the correct mechanism for both detected drift and intentional reconfiguration. The path is always: update Star Chart → calibrate → environment aligns.

### Beacon Granularity

One Beacon per entity. Planets and callsigns are orchestrators — their `init` fires `init` on all attached children independently. Each child writes its own Beacon.

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
orbit callsign init [<alias>]

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
orbit attach nvm vessel
orbit attach native payment-api
orbit attach node@20 payment-api
orbit attach kent-acme acme
orbit attach github-acme-key kent-acme
orbit attach figma-app vessel
```

### `edit` and `remove` Are Not Build Commands

- `orbit calibrate <alias>` — reconcile and update
- `orbit retro <alias>` — retire and remove

---

## `planet init` Argument Resolution

```text
orbit planet init
  → CWD matches known planet? init that planet
  → else: run detection, prompt for alias + galaxy

orbit planet init <alias>
  → alias found in Star Chart? run branch crawl, init all attached resources
  → alias not found + CWD has detectable project? register planet, run detection, prompt
  → alias not found + nothing detectable? prompt all with alias pre-filled

orbit planet init <url-or-path>
  → prompt for alias + galaxy, then provision

orbit planet init <alias> <url-or-path>
  → satisfied — provision, run detection, prompt only for Orbiter decisions
```

**Provisioning flow:**

```text
1. Resolve alias + galaxy (from args, CWD, or prompt)
2. CreatePlanet if not exists → Beacon: unverified

3. Provision repo:
   a. URL provided   → git clone (or protocol-appropriate clone) into dir
   b. CWD has .git   → adopt in place
   c. Neither        → git init new local repo

4. Run integration detection against planet directory (see Detection)
   → collect SuggestedResources from all integrations

5. Run branch crawl → get effective managers + transponders

6. For each suggested resource:
   a. Matching manager in effective set → attach resource, queue for init
   b. No manager, integration has default → attach, queue, warn
   c. No manager, no default → prompt: attach a manager or skip
   d. Conflict with inherited resource of same role+brand → prompt (lower level default)

7. Phase 2 execute — init all attached resources in parallel dependency graphs
   → each integration: Beacon verified or failed

8. Update planet Beacon: verified if all attached resources verified, else failed
```

---

## Context Inference

CWD is the compass for context. Navigation history is for lifecycle commands only.

**Resolution order:**

1. Explicit flag (`--galaxy acme`) — always wins
2. CWD matches a `remote` or `filesystem` resource config path attached to a planet → infer planet
3. CWD is under a `filesystem` resource attached to a galaxy → infer galaxy
4. No match → prompt explicitly

---

## Detection

Detection discovers what resources should be attached to a planet or entity. It runs during `planet init` and `resource init`. Integrations own all detection knowledge — Orbiter core has no awareness of specific file types, binary names, or sync folder conventions.

### Detection Strategy by Role

Detection strategy is determined by integration role:

| Role | Strategy |
| --- | --- |
| `runtime`, `manager`, `tool` | **File-pattern** — Detect runs only when manifest `files` patterns match in CWD |
| `remote`, `filesystem` | **Always-run** — Detect runs for all registered integrations of these roles; integration checks CWD against its own client config (e.g., Dropbox reads `~/.dropbox/info.json`) |

Remote integrations are a small fixed set. Running all of them is not a concern.

### DetectContext and DetectReport

```go
type DetectContext struct {
    Platform Platform
    CWD      string
    Files    map[string]string   // filename → contents; populated for file-pattern integrations
}

type DetectReport struct {
    Detected  bool
    Resources []SuggestedResource
}

type SuggestedResource struct {
    Role    string
    Brand   string
    Version string
    Config  map[string]any
}
```

### Detection Flow

```text
1. Read all integration manifests
2. Scan CWD for manifest file pattern matches
3. Invoke Detect for:
   a. runtime/manager/tool integrations with matched files
   b. ALL remote and filesystem integrations (always-run)
4. Aggregate DetectReports → SuggestedResources for the Captain
```

### Detection Scope

Detection runs against the **local filesystem only**. Detection never inspects the contents of sync folders or remote locations. Detecting that Dropbox is configured (the client is installed, the sync root is at `~/Dropbox`) is valid. Scanning what is inside `~/Dropbox` for config files is not.

---

## Integration Registry

Integrations are the behavior layer for Orbiter. Every resource and transponder role+brand pair that Orbiter can manage has a corresponding integration. Integrations are stateless — they receive context, interact with the real world, and report current state. Orbiter owns all state management, Beacon lifecycle, and drift detection.

### Integration Interface

```go
type Integration interface {
    Detect(ctx DetectContext) DetectReport
    Init(ctx ResolvedContext) StateReport
    Scan(ctx ResolvedContext) StateReport
    Calibrate(ctx ResolvedContext) StateReport
}
```

`Scan` always re-derives state from reality — never from cached values. `Calibrate` receives the desired state from the Star Chart in `ResolvedContext` and brings reality into alignment.

### ResolvedContext

`ResolvedContext` is the integration boundary struct. It is assembled from the Phase 1 branch crawl result, filtered by the integration's manifest dependencies, and enriched with `StateReport` results from already-executed dependencies.

```go
type ResolvedContext struct {
    Platform     Platform
    Resources    map[string][]ResolvedResource    // role → matched resources + StateReports
    Transponders map[string][]ResolvedTransponder // role → matched transponders
}

type ResolvedResource struct {
    Resource    models.Resource
    StateReport *StateReport    // populated after integration executes; nil if not yet run
}

type Platform struct {
    OS   string   // "darwin", "linux", "windows"
    Arch string   // "amd64", "arm64"
}
```

`ResolvedContext` is a plain serializable struct throughout — JSON tags on all fields, no unexported types, no interface values. This is required for Phase 3 WASM compatibility (serialized across the WASM memory boundary).

### StateReport

Integrations return `StateReport` from all three operational methods. The report describes current observed state — not analysis, not drift, not desired state. Orbiter computes drift by comparing the report to the Star Chart.

```go
type StateReport struct {
    Present      bool
    Reachable    bool
    BinaryPath   string          // absolute path to binary
    InstallDir   string          // base installation directory
    InPath       bool            // whether binary is on system PATH
    Manager      string          // brand of manager used (always populated)
    Config       map[string]any  // observed values: version, path, endpoint, etc.
    Observations []string
    Error        string
}
```

`Manager` is always populated. Every installation has a manager — nvm, homebrew, apt, winget, the OS itself, or compiled from source. There is no blank manager.

When `InPath` is false, dependent integrations use `BinaryPath` or `InstallDir` directly. For example, if nvm is not on PATH, node's Init calls `~/.nvm/nvm.sh install 20` using `InstallDir` rather than assuming `nvm` is available as a command.

### Manifest Structure

Each integration is a Go package with a `manifest.toml` for static metadata. The manifest declares what the integration IS and what it NEEDS. Orbiter reads manifests before invoking any WASM or calling any function.

```toml
[integration]
type  = "resource"    # "resource" | "transponder"
role  = "runtime"
brand = "node"

[detection]
files = [".nvmrc", ".node-version", "package.json"]

[dependencies.resources]
manager = ["nvm", "volta", "native"]   # brands; "native" = bare install acceptable

[dependencies.transponders]
# none required
```

```toml
[integration]
type  = "resource"
role  = "manager"
brand = "nvm"

[detection]
files = [".nvmrc", ".node-version"]

[dependencies.resources]
manages = ["node"]    # required and non-empty when role=manager

[dependencies.transponders]
# none required
```

```toml
[integration]
type  = "resource"
role  = "remote"
brand = "dropbox"
# remote role — Detect runs always, no file patterns needed

[detection]
files = []

[dependencies.transponders]
oauth = ["dropbox"]
```

```toml
[integration]
type  = "transponder"
role  = "file"
brand = "github"

[detection]
files = []

[dependencies.resources]
tool = ["git"]

[dependencies.transponders]
agent = []       # SSH agent satisfies, or file-based
file  = ["github"]
```

### Manifest-Driven Resolution

Orbiter uses the manifest `[dependencies]` to filter `BranchContext` before assembling `ResolvedContext` for each integration call. The WASM module receives only what it declared. Orbiter never needs to know what an integration needs at compile time — the manifest is the query spec.

When `role = manager`, `dependencies.resources.manages` declares the brands this manager controls. Orbiter validates this field is present and non-empty for all manager integrations.

### Package Structure

Each integration lives in its own package:

```text
internal/integrations/<brand>/
  manifest.toml     ← static metadata (type, role, brand, detection, dependencies)
  integration.go    ← implements Integration interface; init() self-registers
```

### Compile-Time Discovery

`go:generate` scans `internal/integrations/*/` and produces `internal/integrations/register.go` — a generated file that imports all discovered integration packages. Adding a new integration requires only dropping a new package directory and running `just build`.

No manual registry maintenance. No `all.go` file to edit.

### Phase 3 Path — WASM

Phase 3 adds a WASM loader alongside the compiled integration registry. The interface, manifest structure, and `ResolvedContext` are identical. The loader scans `<binary-dir>/integrations/<brand>/` and `~/.orbiter/integrations/<brand>/` at startup, wraps each WASM module in the `Integration` interface, and merges into the same registry map.

`ResolvedContext` is JSON-serialized across the WASM memory boundary. `StateReport` is JSON-deserialized back. The dispatch layer (`InitResource`, `ScanResource`) calls the `Integration` interface and never knows which backend it has.

Go native plugins are not viable for this purpose (Linux/macOS only, strict version coupling). WASM via wazero (pure Go, no CGo, cross-platform) is the Phase 3 target.

---

## Architecture

```text
internal/
  entity/          ← FieldDef schemas per entity type
  prompt/          ← Ask, Choose, Confirm helpers
  starchart/       ← Create*, Init*, Attach, BranchCrawl, ResolvedContext assembly
  commands/        ← add/init/attach handlers
  integrations/    ← per-brand packages + generated register.go
```

### `internal/entity` — Declarative Field Schemas

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
func (sc *StarChart) InitResource(ctx, id string) error     // dispatches to integration

// Graph
func (sc *StarChart) Attach(ctx, entityID, targetID string) error
func (sc *StarChart) BranchCrawl(ctx, targetID string) (BranchContext, error)
func (sc *StarChart) BuildResolvedContext(branch BranchContext, manifest Manifest) ResolvedContext
```

`BranchContext` is the internal crawl artifact. `ResolvedContext` is the filtered, serializable struct assembled from it for a specific integration's manifest. `BranchContext` never crosses the integration boundary.

### `internal/integrations` — Registry

```go
type Registry struct {
    integrations map[string]Integration   // "role/brand" → Integration
}

func Register(role, brand string, i Integration)
func Get(role, brand string) (Integration, bool)
func AllForRole(role string) []Integration
```

---

## Output and JSON/TUI Interface

All output through `output.Renderer`. No `fmt.Println` in handlers.

JSON mode is non-interactive. Every promptable field is expressible as a flag. Missing required field in JSON mode → structured error, no prompt.

```bash
orbit resource add nvm --role manager --brand nvm --manages '["node"]' --output json
# {"id":"...","alias":"nvm","role":"manager","brand":"nvm","manages":["node"],"beacon":"unverified"}

orbit attach nvm vessel --output json
# {"entity":"nvm-id","target":"vessel-id"}

orbit resource add --output json
# {"error":"missing required field: alias"}
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

- `planets.repo_url`, `planets.repo_path` → become `config` on a `remote + git` resource
- `callsigns.entity_id` → replaced by attachments
- `transponders.callsign_id`, `transponders.entity_id`, `transponders.service` → replaced by attachments + `role` + `brand` columns
- `resources.entity_id`, `resources.manager`, `resources.kind` → replaced by attachments + `role` + `brand` + `manages` columns

**Modify `resources`:**

- Replace `kind TEXT` with `role TEXT` and `brand TEXT`
- Add `manages TEXT` — JSON array of brands; non-null only for `role=manager`
- Add `config TEXT` — JSON blob for role+brand-specific fields
- `version` stays as a top-level column for queryability

**Modify `transponders`:**

- Replace `service TEXT` with `role TEXT` and `brand TEXT`
- `location TEXT` stays (was implicit in prior model, now explicit)

**Add:**

- `attachments` table (see above)

---

## Out of Scope for This Phase

- Specific integration implementations (nvm, node, git, dropbox, etc.)
- Phase 3 WASM loader and runtime integration loading
- Path-aware Beacon evaluation for `chart`/`jump`/`scan`
- Vessel seeding at first run
- Navigation history (read-only log)
- Direct Beacon creation commands (Beacons are side effects of `add`/`init`)
- Standalone alias management
