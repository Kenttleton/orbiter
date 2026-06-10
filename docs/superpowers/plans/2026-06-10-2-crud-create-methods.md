# Phase 2: CRUD Create Methods Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `add`, `init`, and `attach` build verbs for all entities — galaxy, solar system, planet, callsign, transponder, resource — producing a working `orbit` CLI for constructing the Star Chart.

**Architecture:** Each `add` command creates an entity + Beacon(unverified) atomically in a single transaction. `init` commands (planet, resource) additionally attempt real-world provisioning by dispatching to the integration registry. The attachment graph is a first-class table; `attach` wires any two named entities together. All new starchart functions wrap the existing generic CRUD and Tx primitives.

**Tech Stack:** Go, Cobra, modernc.org/sqlite, github.com/stretchr/testify, BubbleTea (existing), existing `internal/starchart`, `internal/models`, `internal/output`, `internal/commands` packages.

---

## File Map

**Create:**
- `internal/migrations/0002_role_brand.sql` — schema migration: drop old columns, add role+brand, add attachments table
- `internal/models/attachment.go` — Attachment struct
- `internal/integrations/types.go` — Platform, DetectContext, DetectReport, ResolvedContext, StateReport, SuggestedResource, Manifest types
- `internal/integrations/registry.go` — Integration interface, Register/Get/AllForRole functions
- `internal/starchart/create.go` — CreateGalaxy, CreateSolarSystem, CreatePlanet, CreateCallsign, CreateTransponder, CreateResource (each atomic: alias + entity + beacon)
- `internal/starchart/attach.go` — Attach function with one-callsign-per-node guard
- `internal/starchart/crawl.go` — BranchCrawl and BuildResolvedContext
- `internal/starchart/init.go` — InitResource, InitTransponder (dispatch to integration registry)
- `internal/commands/galaxy.go` — `galaxy add`
- `internal/commands/system.go` — `system add`
- `internal/commands/callsign.go` — `callsign add`
- `internal/commands/planet.go` — `planet add`, `planet init`
- `internal/commands/transponder.go` — `transponder add`
- `internal/commands/resource.go` — `resource add`, `resource init`
- `internal/commands/attach.go` — `attach <from> <to>`

**Modify:**
- `internal/models/id.go` — add `EntityTypeAttachment = "at"`
- `internal/models/beacon.go` — add unverified/verified/failed/degraded/retired status constants
- `internal/models/resource.go` — replace Kind/Manager/EntityID with Role/Brand/Manages/Config; drop Version
- `internal/models/transponder.go` — replace CallsignID/EntityID/Service with Role/Brand
- `internal/models/callsign.go` — remove EntityID
- `internal/models/planet.go` — remove RepoURL/RepoPath
- `internal/commands/stubs.go` — remove add/edit/remove inline stubs, delegate to per-entity files
- `internal/commands/root.go` — add `newAttachCmd(&d)` to root command

---

## Task 1: Migration 0002

**Files:**
- Create: `internal/migrations/0002_role_brand.sql`

- [ ] **Step 1: Write the migration SQL**

Create `internal/migrations/0002_role_brand.sql`:

```sql
-- Migration 0002: role+brand model and attachments

-- planets: remove repo_url and repo_path (repo associations are managed via attachments)
ALTER TABLE planets DROP COLUMN repo_url;
ALTER TABLE planets DROP COLUMN repo_path;

-- callsigns: remove entity_id (scope determined by attachments)
ALTER TABLE callsigns DROP COLUMN entity_id;

-- transponders: remove callsign_id, entity_id, service; add role+brand
ALTER TABLE transponders DROP COLUMN callsign_id;
ALTER TABLE transponders DROP COLUMN entity_id;
ALTER TABLE transponders DROP COLUMN service;
ALTER TABLE transponders ADD COLUMN role  TEXT NOT NULL DEFAULT '';
ALTER TABLE transponders ADD COLUMN brand TEXT NOT NULL DEFAULT '';

-- resources: remove entity_id, kind, manager, version; add role+brand+manages+config
ALTER TABLE resources DROP COLUMN entity_id;
ALTER TABLE resources DROP COLUMN kind;
ALTER TABLE resources DROP COLUMN manager;
ALTER TABLE resources DROP COLUMN version;
ALTER TABLE resources ADD COLUMN role    TEXT NOT NULL DEFAULT '';
ALTER TABLE resources ADD COLUMN brand   TEXT NOT NULL DEFAULT '';
ALTER TABLE resources ADD COLUMN manages TEXT NOT NULL DEFAULT '[]';
ALTER TABLE resources ADD COLUMN config  TEXT NOT NULL DEFAULT '{}';

-- attachments: universal graph edges (not in alias registry)
CREATE TABLE IF NOT EXISTS attachments (
    id         TEXT PRIMARY KEY,
    from_id    TEXT NOT NULL REFERENCES aliases(id),
    to_id      TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_id, to_id)
);

CREATE INDEX IF NOT EXISTS attachments_from_idx ON attachments(from_id);
CREATE INDEX IF NOT EXISTS attachments_to_idx   ON attachments(to_id);

INSERT INTO schema_version (version) VALUES (2);
```

- [ ] **Step 2: Run the migration test**

Run: `go test ./internal/starchart/... -run TestMigrate -v`

Expected: PASS — the existing migrate_test.go opens a fresh DB which applies all migrations including 0002.

If the migration test doesn't cover version 2 explicitly, the next task will add a targeted test.

- [ ] **Step 3: Add migration version test**

In `internal/starchart/migrate_test.go`, add:

```go
func TestMigration0002Applied(t *testing.T) {
    sc := testDB(t)
    v, err := sc.SchemaVersion()
    require.NoError(t, err)
    require.GreaterOrEqual(t, v, 2)
}
```

Run: `go test ./internal/starchart/... -run TestMigration0002Applied -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/migrations/0002_role_brand.sql internal/starchart/migrate_test.go
git commit -m "feat: add migration 0002 — role+brand model and attachments table"
```

---

## Task 2: Update Models

**Files:**
- Modify: `internal/models/beacon.go`
- Modify: `internal/models/id.go`
- Modify: `internal/models/resource.go`
- Modify: `internal/models/transponder.go`
- Modify: `internal/models/callsign.go`
- Modify: `internal/models/planet.go`
- Create: `internal/models/attachment.go`

- [ ] **Step 1: Write failing model tests**

In `internal/models/models_test.go`, add:

```go
func TestBeaconStatusConstants(t *testing.T) {
    require.Equal(t, "unverified", models.BeaconStatusUnverified)
    require.Equal(t, "verified",   models.BeaconStatusVerified)
    require.Equal(t, "failed",     models.BeaconStatusFailed)
    require.Equal(t, "degraded",   models.BeaconStatusDegraded)
    require.Equal(t, "retired",    models.BeaconStatusRetired)
}

func TestEntityTypeAttachment(t *testing.T) {
    require.Equal(t, "at", models.EntityTypeAttachment)
    id := models.NewID(models.EntityTypeAttachment)
    parsed, err := models.ParseID(id)
    require.NoError(t, err)
    require.Equal(t, "at", parsed.EntityType)
}
```

Run: `go test ./internal/models/... -v`
Expected: FAIL — undefined constants

- [ ] **Step 2: Update beacon.go**

Replace the full contents of `internal/models/beacon.go`:

```go
package models

import "time"

const (
    BeaconStatusHealthy    = "healthy"
    BeaconStatusDrifted    = "drifted"
    BeaconStatusUnknown    = "unknown"
    BeaconStatusUnverified = "unverified"
    BeaconStatusVerified   = "verified"
    BeaconStatusFailed     = "failed"
    BeaconStatusDegraded   = "degraded"
    BeaconStatusRetired    = "retired"
)

// Beacon is the most recent verified observation of an entity.
// One beacon exists per entity. Updated by Scan and Jump.
// Observations is a JSON array of observation strings.
type Beacon struct {
    ID           string    `db:"id"           json:"id"`
    EntityID     string    `db:"entity_id"    json:"entity_id"`
    Status       string    `db:"status"       json:"status"`
    Observations string    `db:"observations" json:"observations"`
    VerifiedAt   time.Time `db:"verified_at"  json:"verified_at"`
}
```

- [ ] **Step 3: Add EntityTypeAttachment to id.go**

In `internal/models/id.go`, add to the entity type constants block:

```go
EntityTypeAttachment = "at"
```

(Add after `EntityTypeNavHistory = "nh"`)

- [ ] **Step 4: Update resource.go**

Replace the full contents of `internal/models/resource.go`:

```go
package models

import "time"

// Resource describes a tooling requirement, runtime, or capability.
// Role and Brand identify the integration. Manages is a JSON array of brands
// this resource manages (non-empty only for role=manager). Config is a JSON
// object for integration-specific configuration.
type Resource struct {
    ID        string    `db:"id"         json:"id"`
    Role      string    `db:"role"       json:"role"`
    Brand     string    `db:"brand"      json:"brand"`
    Manages   string    `db:"manages"    json:"manages"`
    Config    string    `db:"config"     json:"config"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 5: Update transponder.go**

Replace the full contents of `internal/models/transponder.go`:

```go
package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself. Role is the access mechanism
// (file, env, keychain, vault, agent) and Brand is the service it grants
// access to (github, aws, claude, etc.).
type Transponder struct {
    ID        string    `db:"id"         json:"id"`
    Role      string    `db:"role"       json:"role"`
    Brand     string    `db:"brand"      json:"brand"`
    Location  string    `db:"location"   json:"location"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 6: Update callsign.go**

Replace the full contents of `internal/models/callsign.go`:

```go
package models

import "time"

// Callsign represents the Captain's active identity (e.g. "kent-acme").
// Scope is determined by attachments to hierarchy nodes.
type Callsign struct {
    ID        string    `db:"id"         json:"id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 7: Update planet.go**

Replace the full contents of `internal/models/planet.go`:

```go
package models

import "time"

// Planet represents a project — the primary navigation target.
// Galaxy and (optional) SolarSystem establish its position in the hierarchy.
type Planet struct {
    ID            string    `db:"id"              json:"id"`
    GalaxyID      string    `db:"galaxy_id"       json:"galaxy_id"`
    SolarSystemID string    `db:"solar_system_id" json:"solar_system_id,omitempty"`
    CreatedAt     time.Time `db:"created_at"      json:"created_at"`
}
```

- [ ] **Step 8: Create attachment.go**

Create `internal/models/attachment.go`:

```go
package models

import "time"

// Attachment is a directed graph edge wiring two entities together.
// FromID is the "child" entity (resource, callsign, transponder).
// ToID is the "parent" entity (vessel, galaxy, system, planet, or callsign).
// Attachment IDs are not registered in the aliases table.
type Attachment struct {
    ID        string    `db:"id"         json:"id"`
    FromID    string    `db:"from_id"    json:"from_id"`
    ToID      string    `db:"to_id"      json:"to_id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 9: Run model tests**

Run: `go test ./internal/models/... -v`
Expected: PASS

- [ ] **Step 10: Build to confirm no compilation errors**

Run: `go build ./...`
Expected: no errors (stubs.go still compiles because it doesn't reference removed fields)

- [ ] **Step 11: Commit**

```bash
git add internal/models/
git commit -m "feat: update models for role+brand — drop old fields, add Attachment"
```

---

## Task 3: Integration Types and Registry

**Files:**
- Create: `internal/integrations/types.go`
- Create: `internal/integrations/registry.go`

- [ ] **Step 1: Write failing integration registry test**

Create `internal/integrations/registry_test.go`:

```go
package integrations_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/stretchr/testify/require"
)

type stubIntegration struct{}

func (s *stubIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: true}
}
func (s *stubIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *stubIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *stubIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}

func TestRegistryGetNotFound(t *testing.T) {
    _, ok := integrations.Get("manager", "nvm")
    require.False(t, ok)
}

func TestRegistryRegisterAndGet(t *testing.T) {
    integrations.Register("manager", "test-mgr", &stubIntegration{})
    i, ok := integrations.Get("manager", "test-mgr")
    require.True(t, ok)
    require.NotNil(t, i)
}

func TestRegistryAllForRole(t *testing.T) {
    integrations.Register("tool", "tool-a", &stubIntegration{})
    integrations.Register("tool", "tool-b", &stubIntegration{})
    all := integrations.AllForRole("tool")
    require.GreaterOrEqual(t, len(all), 2)
}
```

Run: `go test ./internal/integrations/... -v`
Expected: FAIL — package does not exist

- [ ] **Step 2: Create types.go**

Create `internal/integrations/types.go`:

```go
package integrations

import "github.com/Kenttleton/orbiter/internal/models"

// Platform describes the host operating system and architecture.
type Platform struct {
    OS   string `json:"os"`   // "darwin" | "linux" | "windows"
    Arch string `json:"arch"` // "amd64" | "arm64"
}

// DetectContext is passed to an integration's Detect method.
// Files is populated only for file-pattern integrations (runtime, manager, tool).
// Remote and filesystem integrations receive an empty Files map and use CWD directly.
type DetectContext struct {
    Platform Platform          `json:"platform"`
    CWD      string            `json:"cwd"`
    Files    map[string]string `json:"files"` // filename → contents
}

// DetectReport is returned by an integration's Detect method.
type DetectReport struct {
    Detected  bool               `json:"detected"`
    Resources []SuggestedResource `json:"resources,omitempty"`
}

// SuggestedResource is one resource suggestion returned by detection.
type SuggestedResource struct {
    Role    string         `json:"role"`
    Brand   string         `json:"brand"`
    Version string         `json:"version,omitempty"`
    Config  map[string]any `json:"config,omitempty"`
}

// ResolvedContext is the integration boundary struct passed to Init, Scan, and Calibrate.
// It is assembled from the Phase 1 branch crawl, filtered by the integration's manifest
// dependencies. All fields are JSON-serializable for Phase 3 WASM compatibility.
type ResolvedContext struct {
    Platform     Platform                       `json:"platform"`
    Resources    map[string][]ResolvedResource  `json:"resources"`    // role → matched resources
    Transponders map[string][]ResolvedTransponder `json:"transponders"` // role → matched transponders
}

// ResolvedResource is a resource from the branch, with its StateReport populated
// if it has already been initialized earlier in the execution graph.
type ResolvedResource struct {
    Resource    models.Resource `json:"resource"`
    StateReport *StateReport    `json:"state_report,omitempty"`
}

// ResolvedTransponder is a transponder from the branch.
type ResolvedTransponder struct {
    Transponder models.Transponder `json:"transponder"`
}

// StateReport is returned by Init, Scan, and Calibrate.
// Manager is always populated — every installation has a manager.
type StateReport struct {
    Present      bool           `json:"present"`
    Reachable    bool           `json:"reachable"`
    BinaryPath   string         `json:"binary_path,omitempty"`
    InstallDir   string         `json:"install_dir,omitempty"`
    InPath       bool           `json:"in_path"`
    Manager      string         `json:"manager"`
    Config       map[string]any `json:"config,omitempty"`
    Observations []string       `json:"observations,omitempty"`
    Error        string         `json:"error,omitempty"`
}

// ManifestIntegration is the [integration] section of a manifest.toml.
type ManifestIntegration struct {
    Type  string `toml:"type"`  // "resource" | "transponder"
    Role  string `toml:"role"`
    Brand string `toml:"brand"`
}

// ManifestDetection is the [detection] section of a manifest.toml.
type ManifestDetection struct {
    Files []string `toml:"files"`
}

// ManifestDependencies is the [dependencies] section of a manifest.toml.
// Keys are roles. Values are brand whitelists (empty slice means any brand).
type ManifestDependencies struct {
    Resources    map[string][]string `toml:"resources"`
    Transponders map[string][]string `toml:"transponders"`
}

// Manifest is the parsed content of an integration's manifest.toml.
type Manifest struct {
    Integration  ManifestIntegration  `toml:"integration"`
    Detection    ManifestDetection    `toml:"detection"`
    Dependencies ManifestDependencies `toml:"dependencies"`
}
```

- [ ] **Step 3: Create registry.go**

Create `internal/integrations/registry.go`:

```go
package integrations

import "strings"

// Integration is implemented by every registered integration.
type Integration interface {
    Detect(ctx DetectContext) DetectReport
    Init(ctx ResolvedContext) StateReport
    Scan(ctx ResolvedContext) StateReport
    Calibrate(ctx ResolvedContext) StateReport
}

var registry = map[string]Integration{}

// Register is called by each integration's init() function to self-register.
func Register(role, brand string, i Integration) {
    registry[role+"/"+brand] = i
}

// Get returns the integration for a given role+brand pair.
func Get(role, brand string) (Integration, bool) {
    i, ok := registry[role+"/"+brand]
    return i, ok
}

// AllForRole returns all integrations registered for a given role.
// Used for always-run detection (remote, filesystem).
func AllForRole(role string) []Integration {
    prefix := role + "/"
    var result []Integration
    for key, i := range registry {
        if strings.HasPrefix(key, prefix) {
            result = append(result, i)
        }
    }
    return result
}
```

- [ ] **Step 4: Run integration tests**

Run: `go test ./internal/integrations/... -v`
Expected: PASS

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/integrations/
git commit -m "feat: add integration types and registry"
```

---

## Task 4: StarChart Create Functions

**Files:**
- Create: `internal/starchart/create.go`
- Create: `internal/starchart/create_test.go`

Each create function inserts alias + entity + beacon atomically via `sc.Tx()`. The beacon status starts at `BeaconStatusUnverified`.

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/create_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func TestCreateGalaxy(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, err := sc.CreateGalaxy(ctx, "acme")
    require.NoError(t, err)
    require.NotEmpty(t, g.ID)

    // alias registered
    alias, err := sc.Resolve(ctx, "acme")
    require.NoError(t, err)
    require.Equal(t, g.ID, alias.ID)
    require.Equal(t, models.EntityTypeGalaxy, alias.EntityType)

    // beacon created
    beacon, err := sc.GetBeacon(ctx, g.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreateGalaxyDuplicateNameErrors(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    _, err := sc.CreateGalaxy(ctx, "acme")
    require.NoError(t, err)

    _, err = sc.CreateGalaxy(ctx, "acme")
    require.Error(t, err)
}

func TestCreateSolarSystem(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "acme")
    sys, err := sc.CreateSolarSystem(ctx, "backend", g.ID)
    require.NoError(t, err)
    require.NotEmpty(t, sys.ID)
    require.Equal(t, g.ID, sys.GalaxyID)

    beacon, err := sc.GetBeacon(ctx, sys.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreatePlanet(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    require.NoError(t, err)
    require.NotEmpty(t, p.ID)
    require.Equal(t, g.ID, p.GalaxyID)
    require.Empty(t, p.SolarSystemID)
}

func TestCreateCallsign(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    cs, err := sc.CreateCallsign(ctx, "kent-acme")
    require.NoError(t, err)
    require.NotEmpty(t, cs.ID)
}

func TestCreateTransponder(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    tp, err := sc.CreateTransponder(ctx, "acme-github", "file", "github", "~/.ssh/id_ed25519_acme")
    require.NoError(t, err)
    require.Equal(t, "file", tp.Role)
    require.Equal(t, "github", tp.Brand)
    require.Equal(t, "~/.ssh/id_ed25519_acme", tp.Location)
}

func TestCreateResource(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, err := sc.CreateResource(ctx, "nvm", "manager", "nvm", `["node"]`, `{}`)
    require.NoError(t, err)
    require.Equal(t, "manager", r.Role)
    require.Equal(t, "nvm", r.Brand)
    require.Equal(t, `["node"]`, r.Manages)
}
```

Run: `go test ./internal/starchart/... -run TestCreate -v`
Expected: FAIL — undefined methods

- [ ] **Step 2: Implement create.go**

Create `internal/starchart/create.go`:

```go
package starchart

import (
    "context"
    "time"

    "github.com/Kenttleton/orbiter/internal/models"
)

// GetBeacon returns the beacon for an entity. Returns ErrNotFound if none exists.
func (sc *StarChart) GetBeacon(ctx context.Context, entityID string) (models.Beacon, error) {
    var beacons []models.Beacon
    err := sc.List(ctx, "beacons", &beacons, Filter{Column: "entity_id", Op: "=", Value: entityID})
    if err != nil {
        return models.Beacon{}, err
    }
    if len(beacons) == 0 {
        return models.Beacon{}, ErrNotFound
    }
    return beacons[0], nil
}

// createEntity executes the alias + entity + beacon triple atomically.
// entityFn inserts the entity row within the transaction.
func (sc *StarChart) createEntity(
    ctx context.Context,
    id, name, entityType string,
    entityFn func(*Tx) error,
) error {
    now := time.Now().UTC()
    alias := models.Alias{
        ID:         id,
        Name:       name,
        EntityType: entityType,
        CreatedAt:  now,
    }
    beacon := models.Beacon{
        ID:           models.NewID(models.EntityTypeBeacon),
        EntityID:     id,
        Status:       models.BeaconStatusUnverified,
        Observations: "[]",
        VerifiedAt:   now,
    }
    return sc.Tx(ctx, func(t *Tx) error {
        if err := t.Insert(ctx, "aliases", alias); err != nil {
            return err
        }
        if err := entityFn(t); err != nil {
            return err
        }
        return t.Insert(ctx, "beacons", beacon)
    })
}

// CreateGalaxy registers a new galaxy in the Star Chart.
func (sc *StarChart) CreateGalaxy(ctx context.Context, name string) (models.Galaxy, error) {
    id := models.NewID(models.EntityTypeGalaxy)
    g := models.Galaxy{ID: id, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypeGalaxy, func(t *Tx) error {
        return t.Insert(ctx, "galaxies", g)
    })
    return g, err
}

// CreateSolarSystem registers a new solar system under a galaxy.
func (sc *StarChart) CreateSolarSystem(ctx context.Context, name, galaxyID string) (models.SolarSystem, error) {
    id := models.NewID(models.EntityTypeSolarSystem)
    sys := models.SolarSystem{ID: id, GalaxyID: galaxyID, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypeSolarSystem, func(t *Tx) error {
        return t.Insert(ctx, "solar_systems", sys)
    })
    return sys, err
}

// CreatePlanet registers a new planet under a galaxy (and optionally a solar system).
func (sc *StarChart) CreatePlanet(ctx context.Context, name, galaxyID, solarSystemID string) (models.Planet, error) {
    id := models.NewID(models.EntityTypePlanet)
    p := models.Planet{ID: id, GalaxyID: galaxyID, SolarSystemID: solarSystemID, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypePlanet, func(t *Tx) error {
        return t.Insert(ctx, "planets", p)
    })
    return p, err
}

// CreateCallsign registers a new callsign.
func (sc *StarChart) CreateCallsign(ctx context.Context, name string) (models.Callsign, error) {
    id := models.NewID(models.EntityTypeCallsign)
    cs := models.Callsign{ID: id, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypeCallsign, func(t *Tx) error {
        return t.Insert(ctx, "callsigns", cs)
    })
    return cs, err
}

// CreateTransponder registers a new transponder.
func (sc *StarChart) CreateTransponder(ctx context.Context, name, role, brand, location string) (models.Transponder, error) {
    id := models.NewID(models.EntityTypeTransponder)
    tp := models.Transponder{ID: id, Role: role, Brand: brand, Location: location, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypeTransponder, func(t *Tx) error {
        return t.Insert(ctx, "transponders", tp)
    })
    return tp, err
}

// CreateResource registers a new resource.
// manages is a JSON array (e.g. `["node"]`); config is a JSON object (e.g. `{}`).
func (sc *StarChart) CreateResource(ctx context.Context, name, role, brand, manages, config string) (models.Resource, error) {
    id := models.NewID(models.EntityTypeResource)
    r := models.Resource{ID: id, Role: role, Brand: brand, Manages: manages, Config: config, CreatedAt: time.Now().UTC()}
    err := sc.createEntity(ctx, id, name, models.EntityTypeResource, func(t *Tx) error {
        return t.Insert(ctx, "resources", r)
    })
    return r, err
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/starchart/... -run TestCreate -v`
Expected: PASS

- [ ] **Step 4: Run full test suite**

Run: `go test ./internal/starchart/... -v`
Expected: PASS — existing tests still pass

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/create.go internal/starchart/create_test.go
git commit -m "feat: implement StarChart create functions with atomic alias+entity+beacon"
```

---

## Task 5: StarChart Attach and Crawl

**Files:**
- Create: `internal/starchart/attach.go`
- Create: `internal/starchart/attach_test.go`
- Create: `internal/starchart/crawl.go`

- [ ] **Step 1: Write failing attach tests**

Create `internal/starchart/attach_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/stretchr/testify/require"
)

func TestAttach(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "acme")
    cs, _ := sc.CreateCallsign(ctx, "kent-acme")

    att, err := sc.Attach(ctx, "kent-acme", "acme")
    require.NoError(t, err)
    require.Equal(t, cs.ID, att.FromID)
    require.Equal(t, g.ID, att.ToID)
}

func TestAttachDuplicateErrors(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    sc.CreateGalaxy(ctx, "acme")
    sc.CreateCallsign(ctx, "kent-acme")

    _, err := sc.Attach(ctx, "kent-acme", "acme")
    require.NoError(t, err)

    _, err = sc.Attach(ctx, "kent-acme", "acme")
    require.Error(t, err)
}

func TestAttachOneCallsignPerNode(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    sc.CreateGalaxy(ctx, "acme")
    sc.CreateCallsign(ctx, "kent-acme")
    sc.CreateCallsign(ctx, "other-acme")

    _, err := sc.Attach(ctx, "kent-acme", "acme")
    require.NoError(t, err)

    _, err = sc.Attach(ctx, "other-acme", "acme")
    require.Error(t, err, "should reject second callsign for same node")
}

func TestAttachResourceToVessel(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, _ := sc.CreateResource(ctx, "nvm", "manager", "nvm", `["node"]`, `{}`)

    // vessel is the special built-in entity — get its ID
    vessel, err := sc.GetVessel(ctx)
    require.NoError(t, err)

    att, err := sc.Attach(ctx, "nvm", "vessel")
    require.NoError(t, err)
    require.Equal(t, r.ID, att.FromID)
    require.Equal(t, vessel.ID, att.ToID)
}
```

Run: `go test ./internal/starchart/... -run TestAttach -v`
Expected: FAIL — undefined Attach and GetVessel

- [ ] **Step 2: Implement attach.go**

Create `internal/starchart/attach.go`:

```go
package starchart

import (
    "context"
    "fmt"
    "time"

    "github.com/Kenttleton/orbiter/internal/models"
)

// GetVessel returns the vessel record. There is exactly one vessel per Star Chart.
func (sc *StarChart) GetVessel(ctx context.Context) (models.Vessel, error) {
    var vessels []models.Vessel
    if err := sc.List(ctx, "vessel", &vessels); err != nil {
        return models.Vessel{}, err
    }
    if len(vessels) == 0 {
        return models.Vessel{}, ErrNotFound
    }
    return vessels[0], nil
}

// Attach creates a directed attachment from the entity named fromName to the entity
// named toName. "vessel" resolves to the vessel entity without requiring a name lookup.
//
// One-callsign-per-node rule: if fromName resolves to a callsign, the target node must
// not already have a callsign attached.
func (sc *StarChart) Attach(ctx context.Context, fromName, toName string) (models.Attachment, error) {
    from, err := sc.Resolve(ctx, fromName)
    if err != nil {
        return models.Attachment{}, fmt.Errorf("resolve %q: %w", fromName, err)
    }

    var toID string
    if toName == "vessel" {
        v, err := sc.GetVessel(ctx)
        if err != nil {
            return models.Attachment{}, fmt.Errorf("get vessel: %w", err)
        }
        toID = v.ID
    } else {
        to, err := sc.Resolve(ctx, toName)
        if err != nil {
            return models.Attachment{}, fmt.Errorf("resolve %q: %w", toName, err)
        }
        toID = to.ID
    }

    // One-callsign-per-node: if attaching a callsign, ensure target has none.
    if from.EntityType == models.EntityTypeCallsign {
        if err := sc.guardOneCallsign(ctx, toID); err != nil {
            return models.Attachment{}, err
        }
    }

    att := models.Attachment{
        ID:        models.NewID(models.EntityTypeAttachment),
        FromID:    from.ID,
        ToID:      toID,
        CreatedAt: time.Now().UTC(),
    }
    if err := sc.Insert(ctx, "attachments", att); err != nil {
        return models.Attachment{}, fmt.Errorf("create attachment: %w", err)
    }
    return att, nil
}

// guardOneCallsign returns an error if toID already has a callsign attached.
func (sc *StarChart) guardOneCallsign(ctx context.Context, toID string) error {
    const q = `
        SELECT COUNT(*) FROM attachments a
        JOIN aliases al ON al.id = a.from_id
        WHERE a.to_id = ? AND al.entity_type = ?
    `
    row := sc.db.QueryRowContext(ctx, q, toID, models.EntityTypeCallsign)
    var count int
    if err := row.Scan(&count); err != nil {
        return fmt.Errorf("check callsign count: %w", err)
    }
    if count > 0 {
        return fmt.Errorf("node already has a callsign attached — detach it first")
    }
    return nil
}
```

Note: `sc.db` is unexported, so `guardOneCallsign` must live in the `starchart` package (not `starchart_test`). It can access `sc.db` directly.

- [ ] **Step 3: Run attach tests**

Run: `go test ./internal/starchart/... -run TestAttach -v`
Expected: PASS

- [ ] **Step 4: Create crawl.go**

The branch crawl assembles context for integration dispatch. For Phase 2, this produces the skeleton `ResolvedContext` used by `InitResource`. Full traversal is completed in Phase 3 when integration implementations exist.

Create `internal/starchart/crawl.go`:

```go
package starchart

import (
    "context"
    "fmt"
    "runtime"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/models"
)

// BranchContext is the raw result of crawling the hierarchy for an entity.
// Assembled during Phase 1 of execution; passed to BuildResolvedContext for filtering.
type BranchContext struct {
    Platform     integrations.Platform
    EntityID     string
    Resources    []models.Resource
    Transponders []models.Transponder
    Callsigns    []models.Callsign
}

// BranchCrawl traverses the attachment graph upward from entityID to the vessel,
// collecting resources, transponders, and callsigns at every level.
// The result is ordered vessel-first (FILO: outermost context first).
func (sc *StarChart) BranchCrawl(ctx context.Context, entityID string) (BranchContext, error) {
    branch := BranchContext{
        Platform: currentPlatform(),
        EntityID: entityID,
    }

    // Collect resources attached at the entity's hierarchy levels.
    resources, err := sc.collectAttachedResources(ctx, entityID)
    if err != nil {
        return BranchContext{}, fmt.Errorf("crawl resources: %w", err)
    }
    branch.Resources = resources

    // Collect transponders via callsigns at each level.
    callsigns, err := sc.collectAttachedCallsigns(ctx, entityID)
    if err != nil {
        return BranchContext{}, fmt.Errorf("crawl callsigns: %w", err)
    }
    branch.Callsigns = callsigns

    for _, cs := range callsigns {
        tps, err := sc.collectTranspondersForCallsign(ctx, cs.ID)
        if err != nil {
            return BranchContext{}, fmt.Errorf("crawl transponders: %w", err)
        }
        branch.Transponders = append(branch.Transponders, tps...)
    }

    return branch, nil
}

// BuildResolvedContext filters a BranchContext using the integration's manifest
// dependency declarations, producing the ResolvedContext passed to the integration.
func BuildResolvedContext(branch BranchContext, manifest integrations.Manifest) integrations.ResolvedContext {
    rc := integrations.ResolvedContext{
        Platform:     branch.Platform,
        Resources:    map[string][]integrations.ResolvedResource{},
        Transponders: map[string][]integrations.ResolvedTransponder{},
    }

    depResources := manifest.Dependencies.Resources
    for role, brands := range depResources {
        for _, r := range branch.Resources {
            if r.Role != role {
                continue
            }
            if !brandAccepted(r.Brand, brands) {
                continue
            }
            rc.Resources[role] = append(rc.Resources[role], integrations.ResolvedResource{Resource: r})
        }
    }

    depTransponders := manifest.Dependencies.Transponders
    for role, brands := range depTransponders {
        for _, tp := range branch.Transponders {
            if tp.Role != role {
                continue
            }
            if !brandAccepted(tp.Brand, brands) {
                continue
            }
            rc.Transponders[role] = append(rc.Transponders[role], integrations.ResolvedTransponder{Transponder: tp})
        }
    }

    return rc
}

func currentPlatform() integrations.Platform {
    arch := runtime.GOARCH
    if arch == "arm64" {
        arch = "arm64"
    } else {
        arch = "amd64"
    }
    return integrations.Platform{OS: runtime.GOOS, Arch: arch}
}

// brandAccepted returns true if brand is in the whitelist, or if the whitelist is empty
// (empty = any brand accepted).
func brandAccepted(brand string, whitelist []string) bool {
    if len(whitelist) == 0 {
        return true
    }
    for _, b := range whitelist {
        if b == brand {
            return true
        }
    }
    return false
}

// collectAttachedResources returns all resources attached at or above entityID in the hierarchy.
func (sc *StarChart) collectAttachedResources(ctx context.Context, entityID string) ([]models.Resource, error) {
    const q = `
        SELECT r.id, r.role, r.brand, r.manages, r.config, r.created_at
        FROM resources r
        JOIN attachments a ON a.from_id = r.id
        WHERE a.to_id = ?
    `
    rows, err := sc.db.QueryContext(ctx, q, entityID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var result []models.Resource
    for rows.Next() {
        var r models.Resource
        if err := rows.Scan(&r.ID, &r.Role, &r.Brand, &r.Manages, &r.Config, &r.CreatedAt); err != nil {
            return nil, err
        }
        result = append(result, r)
    }
    return result, rows.Err()
}

// collectAttachedCallsigns returns all callsigns attached at or above entityID in the hierarchy.
func (sc *StarChart) collectAttachedCallsigns(ctx context.Context, entityID string) ([]models.Callsign, error) {
    const q = `
        SELECT cs.id, cs.created_at
        FROM callsigns cs
        JOIN attachments a ON a.from_id = cs.id
        WHERE a.to_id = ?
    `
    rows, err := sc.db.QueryContext(ctx, q, entityID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var result []models.Callsign
    for rows.Next() {
        var cs models.Callsign
        if err := rows.Scan(&cs.ID, &cs.CreatedAt); err != nil {
            return nil, err
        }
        result = append(result, cs)
    }
    return result, rows.Err()
}

// collectTranspondersForCallsign returns all transponders attached to a callsign.
func (sc *StarChart) collectTranspondersForCallsign(ctx context.Context, callsignID string) ([]models.Transponder, error) {
    const q = `
        SELECT tp.id, tp.role, tp.brand, tp.location, tp.created_at
        FROM transponders tp
        JOIN attachments a ON a.from_id = tp.id
        WHERE a.to_id = ?
    `
    rows, err := sc.db.QueryContext(ctx, q, callsignID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var result []models.Transponder
    for rows.Next() {
        var tp models.Transponder
        if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Location, &tp.CreatedAt); err != nil {
            return nil, err
        }
        result = append(result, tp)
    }
    return result, rows.Err()
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/starchart/attach.go internal/starchart/attach_test.go internal/starchart/crawl.go
git commit -m "feat: implement Attach with one-callsign guard and BranchCrawl skeleton"
```

---

## Task 6: StarChart Init Functions

**Files:**
- Create: `internal/starchart/init.go`
- Create: `internal/starchart/init_test.go`

`InitResource` and `InitTransponder` look up the integration, perform a branch crawl, and update the Beacon. With no integrations registered in Phase 2, they return a clean "no integration" beacon.

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/init_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/stretchr/testify/require"
)

func TestInitResourceNoIntegration(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, err := sc.CreateResource(ctx, "nvm", "manager", "nvm", `["node"]`, `{}`)
    require.NoError(t, err)

    // No integration registered for manager/nvm — should update beacon to failed
    err = sc.InitResource(ctx, r.ID)
    require.NoError(t, err) // InitResource itself doesn't error; it records the failure in beacon

    beacon, err := sc.GetBeacon(ctx, r.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusFailed, beacon.Status)
}
```

Run: `go test ./internal/starchart/... -run TestInit -v`
Expected: FAIL — undefined InitResource

- [ ] **Step 2: Implement init.go**

Create `internal/starchart/init.go`:

```go
package starchart

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/models"
)

// InitResource provisions a resource by dispatching to its registered integration.
// If no integration is registered, the beacon is updated to BeaconStatusFailed.
// InitResource itself never returns an error from a failed provision — failures are
// recorded in the Beacon. Returns an error only for Star Chart I/O failures.
func (sc *StarChart) InitResource(ctx context.Context, resourceID string) error {
    var r models.Resource
    if err := sc.Get(ctx, "resources", resourceID, &r); err != nil {
        return fmt.Errorf("get resource: %w", err)
    }

    integration, ok := integrations.Get(r.Role, r.Brand)
    if !ok {
        return sc.updateBeacon(ctx, resourceID, models.BeaconStatusFailed, []string{
            fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand),
        })
    }

    branch, err := sc.BranchCrawl(ctx, resourceID)
    if err != nil {
        return fmt.Errorf("branch crawl: %w", err)
    }

    // Use an empty manifest for now — Phase 3 loads from embedded FS.
    manifest := integrations.Manifest{}
    resolved := BuildResolvedContext(branch, manifest)

    report := integration.Init(resolved)
    return sc.applyStateReport(ctx, resourceID, report)
}

// InitTransponder provisions a transponder by dispatching to its registered integration.
func (sc *StarChart) InitTransponder(ctx context.Context, transponderID string) error {
    var tp models.Transponder
    if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
        return fmt.Errorf("get transponder: %w", err)
    }

    integration, ok := integrations.Get(tp.Role, tp.Brand)
    if !ok {
        return sc.updateBeacon(ctx, transponderID, models.BeaconStatusFailed, []string{
            fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
        })
    }

    branch, err := sc.BranchCrawl(ctx, transponderID)
    if err != nil {
        return fmt.Errorf("branch crawl: %w", err)
    }

    manifest := integrations.Manifest{}
    resolved := BuildResolvedContext(branch, manifest)

    report := integration.Init(resolved)
    return sc.applyStateReport(ctx, transponderID, report)
}

// applyStateReport converts an integration StateReport into a Beacon update.
func (sc *StarChart) applyStateReport(ctx context.Context, entityID string, report integrations.StateReport) error {
    status := models.BeaconStatusVerified
    observations := report.Observations
    if report.Error != "" {
        status = models.BeaconStatusFailed
        observations = append(observations, report.Error)
    } else if !report.Present {
        status = models.BeaconStatusFailed
        observations = append(observations, "integration reported not present after init")
    }
    return sc.updateBeacon(ctx, entityID, status, observations)
}

// updateBeacon sets the beacon status and observations for an entity.
func (sc *StarChart) updateBeacon(ctx context.Context, entityID, status string, observations []string) error {
    obs, err := json.Marshal(observations)
    if err != nil {
        return fmt.Errorf("marshal observations: %w", err)
    }
    return sc.Update(ctx, "beacons", entityID, map[string]any{
        // beacons uses entity_id as the UNIQUE key, not the primary key
        // Use a targeted UPDATE by entity_id instead of id
    })
    // The generic Update uses id column. Beacons are keyed by entity_id for UNIQUE.
    // Use a direct ExecContext instead.
    _ = obs
    return sc.updateBeaconByEntityID(ctx, entityID, status, string(obs))
}

// updateBeaconByEntityID updates beacon status by entity_id (not beacon id).
func (sc *StarChart) updateBeaconByEntityID(ctx context.Context, entityID, status, observations string) error {
    _, err := sc.db.ExecContext(ctx,
        `UPDATE beacons SET status = ?, observations = ?, verified_at = ? WHERE entity_id = ?`,
        status, observations, time.Now().UTC(), entityID,
    )
    return err
}
```

Note: The `updateBeacon` function above has a draft error — the generic `Update` uses `id` as the WHERE key, but beacons are looked up by `entity_id`. The `updateBeaconByEntityID` method handles this directly. Remove the dead code in the `updateBeacon` function body before committing.

Clean version of `updateBeacon`:

```go
func (sc *StarChart) updateBeacon(ctx context.Context, entityID, status string, observations []string) error {
    obs, err := json.Marshal(observations)
    if err != nil {
        return fmt.Errorf("marshal observations: %w", err)
    }
    _, err = sc.db.ExecContext(ctx,
        `UPDATE beacons SET status = ?, observations = ?, verified_at = ? WHERE entity_id = ?`,
        status, string(obs), time.Now().UTC(), entityID,
    )
    return err
}
```

Use this clean version — delete the `updateBeaconByEntityID` helper.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/starchart/... -run TestInit -v`
Expected: PASS

- [ ] **Step 4: Run full suite**

Run: `go test ./internal/starchart/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/init.go internal/starchart/init_test.go
git commit -m "feat: implement InitResource and InitTransponder with integration dispatch"
```

---

## Task 7: Galaxy, System, and Callsign Add Commands

**Files:**
- Create: `internal/commands/galaxy.go`
- Create: `internal/commands/system.go`
- Create: `internal/commands/callsign.go`
- Modify: `internal/commands/stubs.go`

These three entities only support `add` in Phase 2. The command functions replace the existing inline stubs in `stubs.go`.

- [ ] **Step 1: Create galaxy.go**

Create `internal/commands/galaxy.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newGalaxyCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "galaxy",
        Short: "Manage galaxies (organizations/clients)",
    }
    cmd.AddCommand(newGalaxyAddCmd(d))
    return cmd
}

func newGalaxyAddCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "add <name>",
        Short: "Register a galaxy in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            g, err := d.sc.CreateGalaxy(cmd.Context(), args[0])
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("galaxy %q registered (%s)", args[0], g.ID))
            return nil
        },
    }
}
```

- [ ] **Step 2: Create system.go**

Create `internal/commands/system.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newSystemCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "system",
        Short: "Manage solar systems (team subdivisions)",
    }
    cmd.AddCommand(newSystemAddCmd(d))
    return cmd
}

func newSystemAddCmd(d *deps) *cobra.Command {
    var galaxy string
    cmd := &cobra.Command{
        Use:   "add <name>",
        Short: "Register a solar system in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            galAlias, err := d.sc.Resolve(cmd.Context(), galaxy)
            if err != nil {
                return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
            }
            sys, err := d.sc.CreateSolarSystem(cmd.Context(), args[0], galAlias.ID)
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("system %q registered under %q (%s)", args[0], galaxy, sys.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this system belongs to")
    _ = cmd.MarkFlagRequired("galaxy")
    return cmd
}
```

- [ ] **Step 3: Create callsign.go**

Create `internal/commands/callsign.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newCallsignCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "callsign",
        Short: "Manage callsigns (identities)",
    }
    cmd.AddCommand(newCallsignAddCmd(d))
    return cmd
}

func newCallsignAddCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "add <name>",
        Short: "Register a callsign in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            cs, err := d.sc.CreateCallsign(cmd.Context(), args[0])
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("callsign %q registered (%s)", args[0], cs.ID))
            return nil
        },
    }
}
```

- [ ] **Step 4: Update stubs.go**

Remove the `newGalaxyCmd`, `newSystemCmd`, `newCallsignCmd`, and `newTransponderCmd` functions from `internal/commands/stubs.go`. These are now defined in their own files. Keep the six lifecycle command stubs (survey, chart, jump, scan, calibrate, retro) and the vessel commands.

Also remove the edit/remove subcommand stubs inside planet and resource commands — those are handled by lifecycle commands, not build verbs.

After the edit, `stubs.go` should contain only:

```go
package commands

import (
    "github.com/spf13/cobra"
)

// --- Six Lifecycle Commands ---

func newSurveyCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "survey [target]",
        Short: "Inspect metadata — \"What is this thing?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("survey: not yet implemented")
            return nil
        },
    }
}

func newChartCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "chart [target]",
        Short: "Preview a transition — \"What would happen if I went there?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("chart: not yet implemented")
            return nil
        },
    }
}

func newJumpCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "jump [target]",
        Short: "Execute a transition — \"Take me there.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("jump: not yet implemented")
            return nil
        },
    }
}

func newScanCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "scan [target]",
        Short: "Verify reality — \"What does reality currently look like?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("scan: not yet implemented")
            return nil
        },
    }
}

func newCalibrateCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "calibrate [target]",
        Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("calibrate: not yet implemented")
            return nil
        },
    }
}

func newRetroCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "retro [target]",
        Short: "Retire obsolete entities — \"Remove what no longer belongs in the universe.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("retro: not yet implemented")
            return nil
        },
    }
}

// --- Vessel Commands ---

func newVesselCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "vessel",
        Short: "Manage the vessel (this workstation)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "survey", Short: "Show vessel configuration", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel survey: not yet implemented")
            return nil
        }},
        newVesselDefaultsCmd(d),
        newVesselHistoryCmd(d),
    )
    return cmd
}

func newVesselDefaultsCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "defaults",
        Short: "Manage vessel-level defaults",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a default", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel defaults add: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newVesselHistoryCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "history",
        Short: "Manage navigation history",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "clean", Short: "Remove history older than retention period", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel history clean: not yet implemented")
            return nil
        }},
    )
    return cmd
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Smoke test the commands**

```bash
go run ./cmd/orbit galaxy add testco
go run ./cmd/orbit system add platform --galaxy testco
go run ./cmd/orbit callsign add kent-test
```

Expected: each prints a success message with the entity name and ID.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/galaxy.go internal/commands/system.go internal/commands/callsign.go internal/commands/stubs.go
git commit -m "feat: implement galaxy add, system add, callsign add commands"
```

---

## Task 8: Planet Commands

**Files:**
- Create: `internal/commands/planet.go`

`planet add` registers metadata. `planet init` runs the detection stub (no integrations registered in Phase 2, so it always returns no suggestions) and records Beacon:unverified.

- [ ] **Step 1: Create planet.go**

Create `internal/commands/planet.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newPlanetCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "planet",
        Short: "Manage planets (projects)",
    }
    cmd.AddCommand(
        newPlanetAddCmd(d),
        newPlanetInitCmd(d),
    )
    return cmd
}

func newPlanetAddCmd(d *deps) *cobra.Command {
    var galaxy, system string
    cmd := &cobra.Command{
        Use:   "add <name>",
        Short: "Register a planet in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            galAlias, err := d.sc.Resolve(cmd.Context(), galaxy)
            if err != nil {
                return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
            }
            var sysID string
            if system != "" {
                sysAlias, err := d.sc.Resolve(cmd.Context(), system)
                if err != nil {
                    return fmt.Errorf("system %q not found: %w", system, err)
                }
                sysID = sysAlias.ID
            }
            p, err := d.sc.CreatePlanet(cmd.Context(), args[0], galAlias.ID, sysID)
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("planet %q registered under %q (%s)", args[0], galaxy, p.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this planet belongs to")
    cmd.Flags().StringVar(&system, "system", "", "solar system this planet belongs to (optional)")
    _ = cmd.MarkFlagRequired("galaxy")
    return cmd
}

func newPlanetInitCmd(d *deps) *cobra.Command {
    var galaxy, system string
    cmd := &cobra.Command{
        Use:   "init <name>",
        Short: "Register and initialize a planet from the current directory",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            galAlias, err := d.sc.Resolve(cmd.Context(), galaxy)
            if err != nil {
                return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
            }
            var sysID string
            if system != "" {
                sysAlias, err := d.sc.Resolve(cmd.Context(), system)
                if err != nil {
                    return fmt.Errorf("system %q not found: %w", system, err)
                }
                sysID = sysAlias.ID
            }
            p, err := d.sc.CreatePlanet(cmd.Context(), args[0], galAlias.ID, sysID)
            if err != nil {
                return err
            }
            // Detection: no integrations registered in Phase 2 — report none found.
            d.renderer.Info("detection: no integrations registered (Phase 2)")
            d.renderer.Success(fmt.Sprintf("planet %q initialized under %q (%s)", args[0], galaxy, p.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this planet belongs to")
    cmd.Flags().StringVar(&system, "system", "", "solar system this planet belongs to (optional)")
    _ = cmd.MarkFlagRequired("galaxy")
    return cmd
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Smoke test**

```bash
go run ./cmd/orbit galaxy add acme
go run ./cmd/orbit planet add payment-api --galaxy acme
go run ./cmd/orbit planet init website --galaxy acme
```

Expected: each prints a success message.

- [ ] **Step 4: Commit**

```bash
git add internal/commands/planet.go
git commit -m "feat: implement planet add and planet init commands"
```

---

## Task 9: Transponder and Resource Commands

**Files:**
- Create: `internal/commands/transponder.go`
- Create: `internal/commands/resource.go`

- [ ] **Step 1: Create transponder.go**

Create `internal/commands/transponder.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newTransponderCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "transponder",
        Short: "Manage transponders (credential pointers)",
    }
    cmd.AddCommand(newTransponderAddCmd(d))
    return cmd
}

func newTransponderAddCmd(d *deps) *cobra.Command {
    var role, brand, location string
    cmd := &cobra.Command{
        Use:   "add <name>",
        Short: "Register a transponder in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            tp, err := d.sc.CreateTransponder(cmd.Context(), args[0], role, brand, location)
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("transponder %q registered (%s/%s at %s) (%s)", args[0], role, brand, location, tp.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "access mechanism (file, env, keychain, vault, agent)")
    cmd.Flags().StringVar(&brand, "brand", "", "service brand (github, aws, claude, etc.)")
    cmd.Flags().StringVar(&location, "location", "", "credential location (file path, env var name, vault reference)")
    _ = cmd.MarkFlagRequired("role")
    _ = cmd.MarkFlagRequired("brand")
    _ = cmd.MarkFlagRequired("location")
    return cmd
}
```

- [ ] **Step 2: Create resource.go**

Create `internal/commands/resource.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newResourceCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "resource",
        Short: "Manage resources (tooling, runtimes, capabilities)",
    }
    cmd.AddCommand(
        newResourceAddCmd(d),
        newResourceInitCmd(d),
    )
    return cmd
}

func newResourceAddCmd(d *deps) *cobra.Command {
    var role, brand, manages, config string
    cmd := &cobra.Command{
        Use:   "add <name>",
        Short: "Register a resource in the Star Chart",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            if manages == "" {
                manages = "[]"
            }
            if config == "" {
                config = "{}"
            }
            r, err := d.sc.CreateResource(cmd.Context(), args[0], role, brand, manages, config)
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("resource %q registered (%s/%s) (%s)", args[0], role, brand, r.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "resource role (manager, runtime, tool, remote, filesystem)")
    cmd.Flags().StringVar(&brand, "brand", "", "resource brand (nvm, node, git, etc.)")
    cmd.Flags().StringVar(&manages, "manages", "", `JSON array of brands this manager controls (e.g. '["node"]')`)
    cmd.Flags().StringVar(&config, "config", "", `JSON object of resource configuration (e.g. '{"key": "value"}')`)
    _ = cmd.MarkFlagRequired("role")
    _ = cmd.MarkFlagRequired("brand")
    return cmd
}

func newResourceInitCmd(d *deps) *cobra.Command {
    var role, brand, manages, config string
    cmd := &cobra.Command{
        Use:   "init <name>",
        Short: "Register and provision a resource",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            if manages == "" {
                manages = "[]"
            }
            if config == "" {
                config = "{}"
            }
            r, err := d.sc.CreateResource(cmd.Context(), args[0], role, brand, manages, config)
            if err != nil {
                return err
            }
            // Dispatch to integration — records beacon:failed if no integration registered.
            if err := d.sc.InitResource(cmd.Context(), r.ID); err != nil {
                return err
            }
            beacon, err := d.sc.GetBeacon(cmd.Context(), r.ID)
            if err != nil {
                return err
            }
            if beacon.Status == "failed" {
                d.renderer.Warning(fmt.Sprintf("resource %q registered but provisioning failed — no integration for %s/%s", args[0], role, brand))
            } else {
                d.renderer.Success(fmt.Sprintf("resource %q provisioned (%s/%s) (%s)", args[0], role, brand, r.ID))
            }
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "resource role (manager, runtime, tool, remote, filesystem)")
    cmd.Flags().StringVar(&brand, "brand", "", "resource brand (nvm, node, git, etc.)")
    cmd.Flags().StringVar(&manages, "manages", "", `JSON array of brands this manager controls (e.g. '["node"]')`)
    cmd.Flags().StringVar(&config, "config", "", `JSON object of resource configuration`)
    _ = cmd.MarkFlagRequired("role")
    _ = cmd.MarkFlagRequired("brand")
    return cmd
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: Smoke test**

```bash
go run ./cmd/orbit transponder add acme-github --role file --brand github --location ~/.ssh/id_ed25519_acme
go run ./cmd/orbit resource add nvm --role manager --brand nvm --manages '["node"]'
go run ./cmd/orbit resource init my-nvm --role manager --brand nvm --manages '["node"]'
```

Expected: first two succeed; third registers and prints a warning about no integration for manager/nvm.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/transponder.go internal/commands/resource.go
git commit -m "feat: implement transponder add, resource add and resource init commands"
```

---

## Task 10: Attach Command and Root Update

**Files:**
- Create: `internal/commands/attach.go`
- Modify: `internal/commands/root.go`

- [ ] **Step 1: Create attach.go**

Create `internal/commands/attach.go`:

```go
package commands

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newAttachCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "attach <from> <to>",
        Short: "Wire two entities together in the graph",
        Long: `attach creates a directed edge from <from> to <to> in the Star Chart graph.

Examples:
  orbit attach acme-github kent-acme   # transponder → callsign
  orbit attach kent-acme acme          # callsign → galaxy
  orbit attach nvm vessel              # resource → vessel (global scope)
  orbit attach nvm payment-api         # resource → planet (scoped)`,
        Args: cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            att, err := d.sc.Attach(cmd.Context(), args[0], args[1])
            if err != nil {
                return err
            }
            d.renderer.Success(fmt.Sprintf("%q → %q attached (%s)", args[0], args[1], att.ID))
            return nil
        },
    }
}
```

- [ ] **Step 2: Register attach in root.go**

In `internal/commands/root.go`, add `newAttachCmd(&d)` to the `root.AddCommand(...)` call:

```go
root.AddCommand(
    newSurveyCmd(&d),
    newChartCmd(&d),
    newJumpCmd(&d),
    newScanCmd(&d),
    newCalibrateCmd(&d),
    newRetroCmd(&d),
    newGalaxyCmd(&d),
    newSystemCmd(&d),
    newPlanetCmd(&d),
    newCallsignCmd(&d),
    newTransponderCmd(&d),
    newResourceCmd(&d),
    newVesselCmd(&d),
    newAttachCmd(&d),
)
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: End-to-end smoke test**

```bash
go run ./cmd/orbit galaxy add acme
go run ./cmd/orbit callsign add kent-acme
go run ./cmd/orbit transponder add acme-github --role file --brand github --location ~/.ssh/id_ed25519_acme
go run ./cmd/orbit resource add nvm --role manager --brand nvm --manages '["node"]'
go run ./cmd/orbit attach acme-github kent-acme
go run ./cmd/orbit attach kent-acme acme
go run ./cmd/orbit attach nvm vessel
```

Expected: all succeed; each prints a confirmation. No duplicate attachment errors.

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/commands/attach.go internal/commands/root.go
git commit -m "feat: implement attach command — wire entities in the graph"
```

---

## Self-Review

### Spec Coverage

| Requirement | Covered by |
|---|---|
| Migration 0002 (role+brand schema, attachments) | Task 1 |
| Beacon status constants (unverified/verified/failed/degraded/retired) | Task 2 |
| `EntityTypeAttachment = "at"` | Task 2 |
| Role+brand models (Resource, Transponder) | Task 2 |
| Callsign without entity_id | Task 2 |
| Planet without repo_url/repo_path | Task 2 |
| Integration types (Platform, DetectContext, StateReport, etc.) | Task 3 |
| Integration registry (Register/Get/AllForRole) | Task 3 |
| Atomic alias+entity+beacon creates | Task 4 |
| Attach with one-callsign-per-node guard | Task 5 |
| BranchCrawl and BuildResolvedContext | Task 5 |
| InitResource/InitTransponder → beacon update | Task 6 |
| `orbit galaxy add` | Task 7 |
| `orbit system add` | Task 7 |
| `orbit callsign add` | Task 7 |
| `orbit planet add` | Task 8 |
| `orbit planet init` | Task 8 |
| `orbit transponder add` | Task 9 |
| `orbit resource add` | Task 9 |
| `orbit resource init` | Task 9 |
| `orbit attach <from> <to>` | Task 10 |
| Remove edit/remove stubs | Task 7 |

### Known Phase 2 Limitations

These are intentional — not gaps:

- **No integration implementations** — `resource init` registers the resource but always records `failed` because no integrations are compiled in. Phase 3 adds integration packages.
- **No manifest loading** — `BranchCrawl` passes an empty manifest to `BuildResolvedContext`. Phase 3 loads manifests from the embedded FS.
- **No context inference** — galaxy/system must be specified explicitly. Navigation state inference is Phase 3.
- **No planet repo clone** — `planet init` doesn't clone a repository. That requires the remote integration (Phase 3).
- **No detection flow** — `planet init` reports no integrations detected. Phase 3 wires detection.
