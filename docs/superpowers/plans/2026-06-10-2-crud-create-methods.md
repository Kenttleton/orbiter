# Phase 2: CRUD Create Methods Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `add`, `init`, and `attach` build verbs for all entities — galaxy, solar system, planet, callsign, transponder, resource — producing a working `orbit` CLI for constructing the Star Chart.

**Architecture:** `add` creates an entity + Beacon(unverified) atomically. `init` on any entity creates-or-uses the entity and cascades provisioning depth-first down its children: galaxy → systems → planets → resources/transponders; callsign → transponders. Integration dispatch happens at the leaf level (resource, transponder). Roles are Orbiter-owned; brands are integration-owned — Orbiter accepts any brand string and the integration registry determines validity at init time. The `StarChart` struct consumes integrations through an interface it defines locally (idiomatic Go: interfaces at the consumer).

**Tech Stack:** Go, Cobra, modernc.org/sqlite, github.com/stretchr/testify; existing `internal/starchart`, `internal/models`, `internal/output`, `internal/commands` packages.

---

## File Map

**Create:**
- `internal/models/attachment.go` — Attachment struct
- `internal/integrations/types.go` — Platform, DetectContext, DetectReport, ResolvedContext, StateReport, SuggestedResource, Manifest types
- `internal/integrations/registry.go` — Integration interface + Registry struct with Register/Get/AllForRole; package-level Default registry
- `internal/starchart/create.go` — CreateGalaxy, CreateSolarSystem, CreatePlanet, CreateCallsign, CreateTransponder, CreateResource; GetBeacon; createEntity helper
- `internal/starchart/attach.go` — Attach function with one-callsign-per-node guard; GetVessel
- `internal/starchart/crawl.go` — BranchContext type, BranchCrawl, BuildResolvedContext
- `internal/starchart/init.go` — integrationProvider interface (defined here, at the consumer); InitEntity cascade dispatcher; InitGalaxy, InitSolarSystem, InitPlanet, InitCallsign, InitTransponder, InitResource
- `internal/commands/galaxy.go` — `galaxy add`, `galaxy init`
- `internal/commands/system.go` — `system add`, `system init`
- `internal/commands/callsign.go` — `callsign add`, `callsign init`
- `internal/commands/planet.go` — `planet add`, `planet init`
- `internal/commands/transponder.go` — `transponder add`, `transponder init`
- `internal/commands/resource.go` — `resource add`, `resource init`
- `internal/commands/attach.go` — `attach <from> <to>`

**Modify:**
- `internal/models/id.go` — add `EntityTypeAttachment = "at"`
- `internal/models/beacon.go` — add unverified/verified/failed/degraded/retired status constants
- `internal/models/resource.go` — replace Kind/Manager/EntityID/Version with Role/Brand/Manages/Config
- `internal/models/transponder.go` — replace CallsignID/EntityID/Service with Role/Brand
- `internal/models/callsign.go` — remove EntityID
- `internal/models/planet.go` — remove RepoURL/RepoPath
- `internal/commands/stubs.go` — remove entity command stubs (galaxy, system, planet, callsign, transponder, resource); keep six lifecycle stubs and vessel commands
- `internal/commands/root.go` — add `newAttachCmd(&d)` to root

---

## Task 1: Update Models

**Files:**
- Modify: `internal/models/beacon.go`
- Modify: `internal/models/id.go`
- Modify: `internal/models/resource.go`
- Modify: `internal/models/transponder.go`
- Modify: `internal/models/callsign.go`
- Modify: `internal/models/planet.go`
- Create: `internal/models/attachment.go`

The schema changes implied by these model updates belong in a migration file. Since there is no deployed database yet, that file can be written separately and is not part of this plan.

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

Replace `internal/models/beacon.go`:

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
// One beacon exists per entity. Observations is a JSON array of strings.
type Beacon struct {
    ID           string    `db:"id"           json:"id"`
    EntityID     string    `db:"entity_id"    json:"entity_id"`
    Status       string    `db:"status"       json:"status"`
    Observations string    `db:"observations" json:"observations"`
    VerifiedAt   time.Time `db:"verified_at"  json:"verified_at"`
}
```

- [ ] **Step 3: Add EntityTypeAttachment to id.go**

In `internal/models/id.go`, append to the entity type constant block (after `EntityTypeNavHistory`):

```go
EntityTypeAttachment = "at"
```

- [ ] **Step 4: Update resource.go**

Replace `internal/models/resource.go`:

```go
package models

import "time"

// Resource describes a tooling requirement, runtime, or capability.
// Role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
// Brand identifies the specific implementation and is integration-owned —
// Orbiter accepts any brand string; the integration registry determines validity.
// Manages is a JSON array of brands this resource controls (non-empty for role=manager).
// Config is a JSON object for integration-specific configuration.
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

Replace `internal/models/transponder.go`:

```go
package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself.
// Role is the access mechanism (file, env, keychain, vault, agent) and is Orbiter-owned.
// Brand is the service the credential grants access to and is integration-owned.
type Transponder struct {
    ID        string    `db:"id"         json:"id"`
    Role      string    `db:"role"       json:"role"`
    Brand     string    `db:"brand"      json:"brand"`
    Location  string    `db:"location"   json:"location"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 6: Update callsign.go**

Replace `internal/models/callsign.go`:

```go
package models

import "time"

// Callsign represents the Captain's active identity.
// Scope is determined by attachments to hierarchy nodes.
type Callsign struct {
    ID        string    `db:"id"         json:"id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 7: Update planet.go**

Replace `internal/models/planet.go`:

```go
package models

import "time"

// Planet represents a project — the primary navigation target.
// Galaxy and optional SolarSystem establish its position in the hierarchy.
// Repository association is managed via remote resource attachments.
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
// FromID is the child entity (resource, callsign, transponder).
// ToID is the parent entity (vessel, galaxy, system, planet, or callsign).
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
Expected: no errors

- [ ] **Step 11: Commit**

```bash
git add internal/models/
git commit -m "feat: update models for role+brand — drop legacy fields, add Attachment, extend Beacon statuses"
```

---

## Task 2: Integration Types and Registry

**Files:**
- Create: `internal/integrations/types.go`
- Create: `internal/integrations/registry.go`
- Create: `internal/integrations/registry_test.go`

The `Integration` interface lives here so implementors know what to satisfy. The `integrationProvider` interface consumed by `starchart` will be defined in Task 4 (at the consumer, per Go convention).

- [ ] **Step 1: Write failing tests**

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
    return integrations.StateReport{Present: true, Manager: "stub"}
}
func (s *stubIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "stub"}
}
func (s *stubIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "stub"}
}

func TestRegistryGetNotFound(t *testing.T) {
    r := integrations.NewRegistry()
    _, ok := r.Get("manager", "nvm")
    require.False(t, ok)
}

func TestRegistryRegisterAndGet(t *testing.T) {
    r := integrations.NewRegistry()
    r.Register("manager", "test-brand", &stubIntegration{})

    i, ok := r.Get("manager", "test-brand")
    require.True(t, ok)
    require.NotNil(t, i)
}

func TestRegistryAllForRole(t *testing.T) {
    r := integrations.NewRegistry()
    r.Register("tool", "brand-a", &stubIntegration{})
    r.Register("tool", "brand-b", &stubIntegration{})
    r.Register("runtime", "brand-c", &stubIntegration{})

    tools := r.AllForRole("tool")
    require.Len(t, tools, 2)

    runtimes := r.AllForRole("runtime")
    require.Len(t, runtimes, 1)
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

// DetectContext is passed to Detect. Files is populated only for file-pattern
// roles (runtime, manager, tool). Remote and filesystem integrations receive
// an empty Files map and inspect CWD directly.
type DetectContext struct {
    Platform Platform          `json:"platform"`
    CWD      string            `json:"cwd"`
    Files    map[string]string `json:"files"`
}

// DetectReport is returned by Detect.
type DetectReport struct {
    Detected  bool               `json:"detected"`
    Resources []SuggestedResource `json:"resources,omitempty"`
}

// SuggestedResource is one resource suggestion produced by detection.
// Brand is integration-owned — the integration knows what it detected.
type SuggestedResource struct {
    Role    string         `json:"role"`
    Brand   string         `json:"brand"`
    Version string         `json:"version,omitempty"`
    Config  map[string]any `json:"config,omitempty"`
}

// ResolvedContext is the boundary struct passed to Init, Scan, and Calibrate.
// Assembled by the starchart branch crawl, filtered by manifest dependencies.
// All fields are JSON-serializable for Phase 3 WASM compatibility.
type ResolvedContext struct {
    Platform     Platform                         `json:"platform"`
    Resources    map[string][]ResolvedResource    `json:"resources"`
    Transponders map[string][]ResolvedTransponder `json:"transponders"`
}

// ResolvedResource wraps a resource from the branch with its StateReport
// if it has already been initialized earlier in the execution graph.
type ResolvedResource struct {
    Resource    models.Resource `json:"resource"`
    StateReport *StateReport    `json:"state_report,omitempty"`
}

// ResolvedTransponder wraps a transponder from the branch.
type ResolvedTransponder struct {
    Transponder models.Transponder `json:"transponder"`
}

// StateReport is returned by Init, Scan, and Calibrate.
// Manager is always populated — every installation has a manager
// (nvm, homebrew, apt, the OS itself, or "source").
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

// Manifest is the parsed content of an integration's manifest.toml.
type Manifest struct {
    Integration  ManifestIntegration  `toml:"integration"`
    Detection    ManifestDetection    `toml:"detection"`
    Dependencies ManifestDependencies `toml:"dependencies"`
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
// Keys are roles. Values are brand whitelists (empty slice = any brand accepted).
type ManifestDependencies struct {
    Resources    map[string][]string `toml:"resources"`
    Transponders map[string][]string `toml:"transponders"`
}
```

- [ ] **Step 3: Create registry.go**

Create `internal/integrations/registry.go`:

```go
package integrations

import "strings"

// Integration must be implemented by every registered integration.
// The interface is defined here so implementors have a single import target.
// Consumers (starchart) define their own narrower interface for what they call.
type Integration interface {
    Detect(ctx DetectContext) DetectReport
    Init(ctx ResolvedContext) StateReport
    Scan(ctx ResolvedContext) StateReport
    Calibrate(ctx ResolvedContext) StateReport
}

// Registry holds a set of integrations keyed by "role/brand".
type Registry struct {
    entries map[string]Integration
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
    return &Registry{entries: map[string]Integration{}}
}

// Register adds an integration for the given role and brand.
// Calling Register with the same role+brand overwrites the previous entry.
func (r *Registry) Register(role, brand string, i Integration) {
    r.entries[role+"/"+brand] = i
}

// Get returns the integration for a given role+brand pair.
func (r *Registry) Get(role, brand string) (Integration, bool) {
    i, ok := r.entries[role+"/"+brand]
    return i, ok
}

// AllForRole returns all integrations registered for a given role.
// Used during detection for always-run roles (remote, filesystem).
func (r *Registry) AllForRole(role string) []Integration {
    prefix := role + "/"
    var result []Integration
    for key, i := range r.entries {
        if strings.HasPrefix(key, prefix) {
            result = append(result, i)
        }
    }
    return result
}

// Default is the process-wide registry. Integration init() functions call
// Default.Register to self-register. Wired into StarChart via Open().
var Default = NewRegistry()

// Register adds an integration to the Default registry.
// Called by integration init() functions.
func Register(role, brand string, i Integration) {
    Default.Register(role, brand, i)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/integrations/... -v`
Expected: PASS

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/integrations/
git commit -m "feat: add integration types and Registry with Default process-wide instance"
```

---

## Task 3: StarChart Create Functions

**Files:**
- Create: `internal/starchart/create.go`
- Create: `internal/starchart/create_test.go`

Each create function inserts alias + entity + beacon atomically via `sc.Tx()`. Beacon status starts at `BeaconStatusUnverified`.

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/create_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/stretchr/testify/require"
)

func TestCreateGalaxy(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, err := sc.CreateGalaxy(ctx, "stride-build")
    require.NoError(t, err)
    require.NotEmpty(t, g.ID)

    alias, err := sc.Resolve(ctx, "stride-build")
    require.NoError(t, err)
    require.Equal(t, g.ID, alias.ID)
    require.Equal(t, models.EntityTypeGalaxy, alias.EntityType)

    beacon, err := sc.GetBeacon(ctx, g.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreateGalaxyDuplicateNameErrors(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    _, err := sc.CreateGalaxy(ctx, "stride-build")
    require.NoError(t, err)
    _, err = sc.CreateGalaxy(ctx, "stride-build")
    require.Error(t, err)
}

func TestCreateSolarSystem(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "stride-build")
    sys, err := sc.CreateSolarSystem(ctx, "platform", g.ID)
    require.NoError(t, err)
    require.Equal(t, g.ID, sys.GalaxyID)

    beacon, err := sc.GetBeacon(ctx, sys.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreatePlanet(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "stride-build")
    p, err := sc.CreatePlanet(ctx, "payments-api", g.ID, "")
    require.NoError(t, err)
    require.Equal(t, g.ID, p.GalaxyID)
    require.Empty(t, p.SolarSystemID)
}

func TestCreateCallsign(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    cs, err := sc.CreateCallsign(ctx, "work-dev")
    require.NoError(t, err)
    require.NotEmpty(t, cs.ID)
}

func TestCreateTransponder(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    tp, err := sc.CreateTransponder(ctx, "work-github", "file", "github", "~/.ssh/id_ed25519_work")
    require.NoError(t, err)
    require.Equal(t, "file", tp.Role)
    require.Equal(t, "github", tp.Brand)
    require.Equal(t, "~/.ssh/id_ed25519_work", tp.Location)
}

func TestCreateResource(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, err := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
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
    if err := sc.List(ctx, "beacons", &beacons, Filter{Column: "entity_id", Op: "=", Value: entityID}); err != nil {
        return models.Beacon{}, err
    }
    if len(beacons) == 0 {
        return models.Beacon{}, ErrNotFound
    }
    return beacons[0], nil
}

// createEntity inserts alias + entity + beacon atomically.
// entityFn inserts the entity row within the transaction.
func (sc *StarChart) createEntity(ctx context.Context, id, name, entityType string, entityFn func(*Tx) error) error {
    now := time.Now().UTC()
    return sc.Tx(ctx, func(t *Tx) error {
        if err := t.Insert(ctx, "aliases", models.Alias{
            ID: id, Name: name, EntityType: entityType, CreatedAt: now,
        }); err != nil {
            return err
        }
        if err := entityFn(t); err != nil {
            return err
        }
        return t.Insert(ctx, "beacons", models.Beacon{
            ID:           models.NewID(models.EntityTypeBeacon),
            EntityID:     id,
            Status:       models.BeaconStatusUnverified,
            Observations: "[]",
            VerifiedAt:   now,
        })
    })
}

// CreateGalaxy registers a new galaxy in the Star Chart.
func (sc *StarChart) CreateGalaxy(ctx context.Context, name string) (models.Galaxy, error) {
    id := models.NewID(models.EntityTypeGalaxy)
    g := models.Galaxy{ID: id, CreatedAt: time.Now().UTC()}
    return g, sc.createEntity(ctx, id, name, models.EntityTypeGalaxy, func(t *Tx) error {
        return t.Insert(ctx, "galaxies", g)
    })
}

// CreateSolarSystem registers a new solar system under a galaxy.
func (sc *StarChart) CreateSolarSystem(ctx context.Context, name, galaxyID string) (models.SolarSystem, error) {
    id := models.NewID(models.EntityTypeSolarSystem)
    sys := models.SolarSystem{ID: id, GalaxyID: galaxyID, CreatedAt: time.Now().UTC()}
    return sys, sc.createEntity(ctx, id, name, models.EntityTypeSolarSystem, func(t *Tx) error {
        return t.Insert(ctx, "solar_systems", sys)
    })
}

// CreatePlanet registers a new planet under a galaxy and optional solar system.
func (sc *StarChart) CreatePlanet(ctx context.Context, name, galaxyID, solarSystemID string) (models.Planet, error) {
    id := models.NewID(models.EntityTypePlanet)
    p := models.Planet{ID: id, GalaxyID: galaxyID, SolarSystemID: solarSystemID, CreatedAt: time.Now().UTC()}
    return p, sc.createEntity(ctx, id, name, models.EntityTypePlanet, func(t *Tx) error {
        return t.Insert(ctx, "planets", p)
    })
}

// CreateCallsign registers a new callsign.
func (sc *StarChart) CreateCallsign(ctx context.Context, name string) (models.Callsign, error) {
    id := models.NewID(models.EntityTypeCallsign)
    cs := models.Callsign{ID: id, CreatedAt: time.Now().UTC()}
    return cs, sc.createEntity(ctx, id, name, models.EntityTypeCallsign, func(t *Tx) error {
        return t.Insert(ctx, "callsigns", cs)
    })
}

// CreateTransponder registers a new transponder.
// Role is Orbiter-owned (file, env, keychain, vault, agent).
// Brand is integration-owned — any string is accepted.
func (sc *StarChart) CreateTransponder(ctx context.Context, name, role, brand, location string) (models.Transponder, error) {
    id := models.NewID(models.EntityTypeTransponder)
    tp := models.Transponder{ID: id, Role: role, Brand: brand, Location: location, CreatedAt: time.Now().UTC()}
    return tp, sc.createEntity(ctx, id, name, models.EntityTypeTransponder, func(t *Tx) error {
        return t.Insert(ctx, "transponders", tp)
    })
}

// CreateResource registers a new resource.
// Role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
// Brand is integration-owned — any string is accepted.
// manages is a JSON array (e.g. `["node"]`); config is a JSON object.
func (sc *StarChart) CreateResource(ctx context.Context, name, role, brand, manages, config string) (models.Resource, error) {
    id := models.NewID(models.EntityTypeResource)
    r := models.Resource{ID: id, Role: role, Brand: brand, Manages: manages, Config: config, CreatedAt: time.Now().UTC()}
    return r, sc.createEntity(ctx, id, name, models.EntityTypeResource, func(t *Tx) error {
        return t.Insert(ctx, "resources", r)
    })
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/starchart/... -run TestCreate -v`
Expected: PASS

- [ ] **Step 4: Run full suite**

Run: `go test ./internal/starchart/... -v`
Expected: PASS — existing tests still pass

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/create.go internal/starchart/create_test.go
git commit -m "feat: implement StarChart create functions with atomic alias+entity+beacon"
```

---

## Task 4: StarChart Attach and Crawl

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

    "github.com/stretchr/testify/require"
)

func TestAttach(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "stride-build")
    cs, _ := sc.CreateCallsign(ctx, "work-dev")

    att, err := sc.Attach(ctx, "work-dev", "stride-build")
    require.NoError(t, err)
    require.Equal(t, cs.ID, att.FromID)
    require.Equal(t, g.ID, att.ToID)
}

func TestAttachDuplicateErrors(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    sc.CreateGalaxy(ctx, "stride-build")
    sc.CreateCallsign(ctx, "work-dev")

    _, err := sc.Attach(ctx, "work-dev", "stride-build")
    require.NoError(t, err)
    _, err = sc.Attach(ctx, "work-dev", "stride-build")
    require.Error(t, err)
}

func TestAttachOneCallsignPerNode(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    sc.CreateGalaxy(ctx, "stride-build")
    sc.CreateCallsign(ctx, "dev-a")
    sc.CreateCallsign(ctx, "dev-b")

    _, err := sc.Attach(ctx, "dev-a", "stride-build")
    require.NoError(t, err)

    _, err = sc.Attach(ctx, "dev-b", "stride-build")
    require.Error(t, err, "should reject second callsign on same node")
}

func TestAttachResourceToVessel(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
    vessel, err := sc.GetVessel(ctx)
    require.NoError(t, err)

    att, err := sc.Attach(ctx, "nvm-mgr", "vessel")
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

// GetVessel returns the single vessel record.
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

// Attach creates a directed graph edge from fromName to toName.
// "vessel" resolves to the vessel entity without requiring an alias lookup.
//
// One-callsign-per-node: if fromName resolves to a callsign, the target node
// must not already have a callsign attached.
func (sc *StarChart) Attach(ctx context.Context, fromName, toName string) (models.Attachment, error) {
    from, err := sc.Resolve(ctx, fromName)
    if err != nil {
        return models.Attachment{}, fmt.Errorf("resolve %q: %w", fromName, err)
    }

    toID, err := sc.resolveAttachTarget(ctx, toName)
    if err != nil {
        return models.Attachment{}, fmt.Errorf("resolve %q: %w", toName, err)
    }

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

// resolveAttachTarget resolves the "to" side of an attachment.
// "vessel" is a reserved name that bypasses the alias table.
func (sc *StarChart) resolveAttachTarget(ctx context.Context, name string) (string, error) {
    if name == "vessel" {
        v, err := sc.GetVessel(ctx)
        if err != nil {
            return "", err
        }
        return v.ID, nil
    }
    a, err := sc.Resolve(ctx, name)
    if err != nil {
        return "", err
    }
    return a.ID, nil
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
        return fmt.Errorf("check existing callsigns: %w", err)
    }
    if count > 0 {
        return fmt.Errorf("node already has a callsign attached — detach it first via orbit retro")
    }
    return nil
}
```

- [ ] **Step 3: Run attach tests**

Run: `go test ./internal/starchart/... -run TestAttach -v`
Expected: PASS

- [ ] **Step 4: Create crawl.go**

The branch crawl assembles context for integration dispatch. Phase 2 implements direct-attachment collection; full hierarchy traversal (vessel → galaxy → system → planet) is completed alongside integration implementations.

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

// BranchContext is the raw crawl result for an entity.
// Assembled during Phase 1 of execution; filtered by BuildResolvedContext.
type BranchContext struct {
    Platform     integrations.Platform
    EntityID     string
    Resources    []models.Resource
    Transponders []models.Transponder
    Callsigns    []models.Callsign
}

// BranchCrawl collects the resources, transponders, and callsigns reachable
// from entityID via the attachment graph.
func (sc *StarChart) BranchCrawl(ctx context.Context, entityID string) (BranchContext, error) {
    branch := BranchContext{
        Platform: currentPlatform(),
        EntityID: entityID,
    }

    resources, err := sc.resourcesAttachedTo(ctx, entityID)
    if err != nil {
        return BranchContext{}, fmt.Errorf("crawl resources for %s: %w", entityID, err)
    }
    branch.Resources = resources

    callsigns, err := sc.callsignsAttachedTo(ctx, entityID)
    if err != nil {
        return BranchContext{}, fmt.Errorf("crawl callsigns for %s: %w", entityID, err)
    }
    branch.Callsigns = callsigns

    for _, cs := range callsigns {
        tps, err := sc.transpondersAttachedTo(ctx, cs.ID)
        if err != nil {
            return BranchContext{}, fmt.Errorf("crawl transponders for callsign %s: %w", cs.ID, err)
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
        Resources:    make(map[string][]integrations.ResolvedResource),
        Transponders: make(map[string][]integrations.ResolvedTransponder),
    }
    for role, brands := range manifest.Dependencies.Resources {
        for _, r := range branch.Resources {
            if r.Role == role && brandAccepted(r.Brand, brands) {
                rc.Resources[role] = append(rc.Resources[role], integrations.ResolvedResource{Resource: r})
            }
        }
    }
    for role, brands := range manifest.Dependencies.Transponders {
        for _, tp := range branch.Transponders {
            if tp.Role == role && brandAccepted(tp.Brand, brands) {
                rc.Transponders[role] = append(rc.Transponders[role], integrations.ResolvedTransponder{Transponder: tp})
            }
        }
    }
    return rc
}

func currentPlatform() integrations.Platform {
    return integrations.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// brandAccepted returns true if brand is in the whitelist or the whitelist is empty.
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

func (sc *StarChart) resourcesAttachedTo(ctx context.Context, nodeID string) ([]models.Resource, error) {
    const q = `
        SELECT r.id, r.role, r.brand, r.manages, r.config, r.created_at
        FROM resources r
        JOIN attachments a ON a.from_id = r.id
        WHERE a.to_id = ?
    `
    rows, err := sc.db.QueryContext(ctx, q, nodeID)
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

func (sc *StarChart) callsignsAttachedTo(ctx context.Context, nodeID string) ([]models.Callsign, error) {
    const q = `
        SELECT cs.id, cs.created_at
        FROM callsigns cs
        JOIN attachments a ON a.from_id = cs.id
        WHERE a.to_id = ?
    `
    rows, err := sc.db.QueryContext(ctx, q, nodeID)
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

func (sc *StarChart) transpondersAttachedTo(ctx context.Context, callsignID string) ([]models.Transponder, error) {
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
git commit -m "feat: implement Attach with one-callsign guard and BranchCrawl"
```

---

## Task 5: StarChart Init — Cascade Dispatcher

**Files:**
- Create: `internal/starchart/init.go`
- Create: `internal/starchart/init_test.go`

`init` on any entity cascades depth-first to its children. The `integrationProvider` interface is defined here, at the consumer (idiomatic Go — not in the `integrations` package). `StarChart` accepts an `integrationProvider` at construction so tests can inject a stub.

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/init_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

// successIntegration always reports the entity as present and healthy.
type successIntegration struct{}

func (s *successIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: true}
}
func (s *successIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *successIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *successIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}

func TestInitResourceNoIntegration(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)

    require.NoError(t, sc.InitResource(ctx, r.ID))

    beacon, err := sc.GetBeacon(ctx, r.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusFailed, beacon.Status)
}

func TestInitResourceWithIntegration(t *testing.T) {
    ctx := context.Background()

    reg := integrations.NewRegistry()
    reg.Register("manager", "nvm", &successIntegration{})

    sc := testDBWithRegistry(t, reg)
    r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)

    require.NoError(t, sc.InitResource(ctx, r.ID))

    beacon, err := sc.GetBeacon(ctx, r.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusVerified, beacon.Status)
}

func TestInitGalaxyCascadesToPlanets(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    g, _ := sc.CreateGalaxy(ctx, "stride-build")
    p, _ := sc.CreatePlanet(ctx, "payments-api", g.ID, "")
    r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
    sc.Attach(ctx, "nvm-mgr", "payments-api")

    require.NoError(t, sc.InitGalaxy(ctx, g.ID))

    // Planet beacon updated
    pBeacon, err := sc.GetBeacon(ctx, p.ID)
    require.NoError(t, err)
    require.NotEqual(t, models.BeaconStatusUnverified, pBeacon.Status)

    // Resource beacon updated (failed because no integration registered)
    rBeacon, err := sc.GetBeacon(ctx, r.ID)
    require.NoError(t, err)
    require.Equal(t, models.BeaconStatusFailed, rBeacon.Status)
}
```

`testDBWithRegistry` is a new helper. Add it to `internal/starchart/crud_test.go` (alongside the existing `testDB` helper):

```go
func testDBWithRegistry(t *testing.T, reg *integrations.Registry) *starchart.StarChart {
    t.Helper()
    sc, err := starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })
    return sc
}
```

Run: `go test ./internal/starchart/... -run TestInit -v`
Expected: FAIL — undefined methods

- [ ] **Step 2: Add OpenWithRegistry to db.go**

In `internal/starchart/db.go`, add alongside `Open`:

```go
// OpenWithRegistry opens the Star Chart database and wires the given registry
// for integration dispatch. Used in tests and in main() when overriding the default.
func OpenWithRegistry(path string, reg integrationProvider) (*StarChart, error) {
    if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
        return nil, fmt.Errorf("create starchart directory: %w", err)
    }
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("open starchart: %w", err)
    }
    if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
        db.Close()
        return nil, fmt.Errorf("enable foreign keys: %w", err)
    }
    sc := &StarChart{db: db, integrations: reg}
    if err := sc.migrate(context.Background()); err != nil {
        db.Close()
        return nil, fmt.Errorf("run migrations: %w", err)
    }
    return sc, nil
}
```

Also update `Open` to wire the default registry:

```go
func Open(path string) (*StarChart, error) {
    return OpenWithRegistry(path, integrations.Default)
}
```

And add the `integrations` field to the `StarChart` struct in `db.go`:

```go
type StarChart struct {
    db           *sql.DB
    integrations integrationProvider
}
```

- [ ] **Step 3: Implement init.go**

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

// integrationProvider is the interface starchart needs from an integration registry.
// Defined here, at the consumer (idiomatic Go — not in the integrations package).
type integrationProvider interface {
    Get(role, brand string) (integrations.Integration, bool)
    AllForRole(role string) []integrations.Integration
}

// InitGalaxy cascades init to all planets in the galaxy.
// The galaxy's own beacon is updated to reflect the cascade result.
func (sc *StarChart) InitGalaxy(ctx context.Context, galaxyID string) error {
    var planets []models.Planet
    if err := sc.List(ctx, "planets", &planets, Filter{Column: "galaxy_id", Op: "=", Value: galaxyID}); err != nil {
        return fmt.Errorf("list planets for galaxy %s: %w", galaxyID, err)
    }
    var initErr error
    for _, p := range planets {
        if err := sc.InitPlanet(ctx, p.ID); err != nil {
            initErr = err
        }
    }
    status := models.BeaconStatusVerified
    if initErr != nil {
        status = models.BeaconStatusDegraded
    }
    return sc.setBeaconStatus(ctx, galaxyID, status, nil)
}

// InitSolarSystem cascades init to all planets in the system.
func (sc *StarChart) InitSolarSystem(ctx context.Context, systemID string) error {
    var planets []models.Planet
    if err := sc.List(ctx, "planets", &planets, Filter{Column: "solar_system_id", Op: "=", Value: systemID}); err != nil {
        return fmt.Errorf("list planets for system %s: %w", systemID, err)
    }
    var initErr error
    for _, p := range planets {
        if err := sc.InitPlanet(ctx, p.ID); err != nil {
            initErr = err
        }
    }
    status := models.BeaconStatusVerified
    if initErr != nil {
        status = models.BeaconStatusDegraded
    }
    return sc.setBeaconStatus(ctx, systemID, status, nil)
}

// InitPlanet cascades init to all resources attached to the planet.
func (sc *StarChart) InitPlanet(ctx context.Context, planetID string) error {
    resources, err := sc.resourcesAttachedTo(ctx, planetID)
    if err != nil {
        return fmt.Errorf("list resources for planet %s: %w", planetID, err)
    }
    callsigns, err := sc.callsignsAttachedTo(ctx, planetID)
    if err != nil {
        return fmt.Errorf("list callsigns for planet %s: %w", planetID, err)
    }

    var initErr error
    for _, r := range resources {
        if err := sc.InitResource(ctx, r.ID); err != nil {
            initErr = err
        }
    }
    for _, cs := range callsigns {
        if err := sc.InitCallsign(ctx, cs.ID); err != nil {
            initErr = err
        }
    }

    status := models.BeaconStatusVerified
    if initErr != nil {
        status = models.BeaconStatusDegraded
    }
    return sc.setBeaconStatus(ctx, planetID, status, nil)
}

// InitCallsign cascades init to all transponders attached to the callsign.
func (sc *StarChart) InitCallsign(ctx context.Context, callsignID string) error {
    transponders, err := sc.transpondersAttachedTo(ctx, callsignID)
    if err != nil {
        return fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
    }
    var initErr error
    for _, tp := range transponders {
        if err := sc.InitTransponder(ctx, tp.ID); err != nil {
            initErr = err
        }
    }
    status := models.BeaconStatusVerified
    if initErr != nil {
        status = models.BeaconStatusDegraded
    }
    return sc.setBeaconStatus(ctx, callsignID, status, nil)
}

// InitResource provisions a resource by dispatching to its registered integration.
// If no integration is registered, the beacon is updated to BeaconStatusFailed.
// Returns an error only for Star Chart I/O failures — provisioning failures are
// recorded in the Beacon, not returned as errors.
func (sc *StarChart) InitResource(ctx context.Context, resourceID string) error {
    var r models.Resource
    if err := sc.Get(ctx, "resources", resourceID, &r); err != nil {
        return fmt.Errorf("get resource %s: %w", resourceID, err)
    }

    integration, ok := sc.integrations.Get(r.Role, r.Brand)
    if !ok {
        return sc.setBeaconStatus(ctx, resourceID, models.BeaconStatusFailed, []string{
            fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand),
        })
    }

    branch, err := sc.BranchCrawl(ctx, resourceID)
    if err != nil {
        return fmt.Errorf("branch crawl for resource %s: %w", resourceID, err)
    }

    report := integration.Init(BuildResolvedContext(branch, integrations.Manifest{}))
    return sc.applyStateReport(ctx, resourceID, report)
}

// InitTransponder provisions a transponder by dispatching to its registered integration.
func (sc *StarChart) InitTransponder(ctx context.Context, transponderID string) error {
    var tp models.Transponder
    if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
        return fmt.Errorf("get transponder %s: %w", transponderID, err)
    }

    integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
    if !ok {
        return sc.setBeaconStatus(ctx, transponderID, models.BeaconStatusFailed, []string{
            fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
        })
    }

    branch, err := sc.BranchCrawl(ctx, transponderID)
    if err != nil {
        return fmt.Errorf("branch crawl for transponder %s: %w", transponderID, err)
    }

    report := integration.Init(BuildResolvedContext(branch, integrations.Manifest{}))
    return sc.applyStateReport(ctx, transponderID, report)
}

// applyStateReport converts an integration StateReport into a Beacon update.
func (sc *StarChart) applyStateReport(ctx context.Context, entityID string, report integrations.StateReport) error {
    status := models.BeaconStatusVerified
    obs := report.Observations
    if report.Error != "" {
        status = models.BeaconStatusFailed
        obs = append(obs, report.Error)
    } else if !report.Present {
        status = models.BeaconStatusFailed
        obs = append(obs, "integration reported entity not present after init")
    }
    return sc.setBeaconStatus(ctx, entityID, status, obs)
}

// setBeaconStatus updates a beacon by entity_id. The generic Update primitive
// uses the id column; beacons are addressed by entity_id for uniqueness.
func (sc *StarChart) setBeaconStatus(ctx context.Context, entityID, status string, observations []string) error {
    if observations == nil {
        observations = []string{}
    }
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

- [ ] **Step 4: Run init tests**

Run: `go test ./internal/starchart/... -run TestInit -v`
Expected: PASS

- [ ] **Step 5: Run full suite**

Run: `go test ./internal/starchart/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/starchart/init.go internal/starchart/init_test.go internal/starchart/db.go internal/starchart/crud_test.go
git commit -m "feat: implement init cascade — galaxy/system/planet/callsign/transponder/resource"
```

---

## Task 6: Entity Add+Init Commands — Galaxy, System, Callsign

**Files:**
- Create: `internal/commands/galaxy.go`
- Create: `internal/commands/system.go`
- Create: `internal/commands/callsign.go`
- Modify: `internal/commands/stubs.go`

All three entities support both `add` (register only) and `init` (register or use existing, then cascade).

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
    cmd.AddCommand(
        newGalaxyAddCmd(d),
        newGalaxyInitCmd(d),
    )
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

func newGalaxyInitCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "init <name>",
        Short: "Register and initialize a galaxy, cascading to all children",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                // Does not exist — create it first.
                g, err := d.sc.CreateGalaxy(ctx, args[0])
                if err != nil {
                    return err
                }
                alias.ID = g.ID
            }
            if err := d.sc.InitGalaxy(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("galaxy %q initialized — status: %s", args[0], beacon.Status))
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
    cmd.AddCommand(
        newSystemAddCmd(d),
        newSystemInitCmd(d),
    )
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

func newSystemInitCmd(d *deps) *cobra.Command {
    var galaxy string
    cmd := &cobra.Command{
        Use:   "init <name>",
        Short: "Register and initialize a system, cascading to all planets",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                galAlias, err := d.sc.Resolve(ctx, galaxy)
                if err != nil {
                    return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
                }
                sys, err := d.sc.CreateSolarSystem(ctx, args[0], galAlias.ID)
                if err != nil {
                    return err
                }
                alias.ID = sys.ID
            }
            if err := d.sc.InitSolarSystem(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("system %q initialized — status: %s", args[0], beacon.Status))
            return nil
        },
    }
    cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this system belongs to (required when creating)")
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
    cmd.AddCommand(
        newCallsignAddCmd(d),
        newCallsignInitCmd(d),
    )
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

func newCallsignInitCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "init <name>",
        Short: "Register and initialize a callsign, cascading to attached transponders",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                cs, err := d.sc.CreateCallsign(ctx, args[0])
                if err != nil {
                    return err
                }
                alias.ID = cs.ID
            }
            if err := d.sc.InitCallsign(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("callsign %q initialized — status: %s", args[0], beacon.Status))
            return nil
        },
    }
}
```

- [ ] **Step 4: Trim stubs.go**

Replace `internal/commands/stubs.go` with only the six lifecycle stubs and vessel commands (entity commands are now in their own files):

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
        &cobra.Command{
            Use:  "survey",
            Short: "Show vessel configuration",
            RunE: func(cmd *cobra.Command, args []string) error {
                d.renderer.Info("vessel survey: not yet implemented")
                return nil
            },
        },
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
        &cobra.Command{
            Use:   "add",
            Short: "Add a default",
            RunE: func(cmd *cobra.Command, args []string) error {
                d.renderer.Info("vessel defaults add: not yet implemented")
                return nil
            },
        },
    )
    return cmd
}

func newVesselHistoryCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "history",
        Short: "Manage navigation history",
    }
    cmd.AddCommand(
        &cobra.Command{
            Use:   "clean",
            Short: "Remove history older than retention period",
            RunE: func(cmd *cobra.Command, args []string) error {
                d.renderer.Info("vessel history clean: not yet implemented")
                return nil
            },
        },
    )
    return cmd
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Smoke test**

```bash
go run ./cmd/orbit galaxy add freelance-work
go run ./cmd/orbit galaxy init oss-contrib
go run ./cmd/orbit system add api-team --galaxy freelance-work
go run ./cmd/orbit callsign add dev-primary
go run ./cmd/orbit callsign init dev-backup
```

Expected: each succeeds. `init` variants print the resulting beacon status.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/galaxy.go internal/commands/system.go internal/commands/callsign.go internal/commands/stubs.go
git commit -m "feat: implement galaxy/system/callsign add and init commands"
```

---

## Task 7: Planet Add+Init Commands

**Files:**
- Create: `internal/commands/planet.go`

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
            ctx := cmd.Context()
            galAlias, err := d.sc.Resolve(ctx, galaxy)
            if err != nil {
                return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
            }
            var sysID string
            if system != "" {
                sysAlias, err := d.sc.Resolve(ctx, system)
                if err != nil {
                    return fmt.Errorf("system %q not found: %w", system, err)
                }
                sysID = sysAlias.ID
            }
            p, err := d.sc.CreatePlanet(ctx, args[0], galAlias.ID, sysID)
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
        Short: "Register and initialize a planet, cascading to all attached resources",
        Long: `init registers the planet (or uses an existing one) and cascades init to all
resources and callsigns attached to it.

With no integrations registered (Phase 2), attached resources record a failed beacon.
Integration implementations added in Phase 3 will provision resources on init.`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                galAlias, err := d.sc.Resolve(ctx, galaxy)
                if err != nil {
                    return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
                }
                var sysID string
                if system != "" {
                    sysAlias, err := d.sc.Resolve(ctx, system)
                    if err != nil {
                        return fmt.Errorf("system %q not found: %w", system, err)
                    }
                    sysID = sysAlias.ID
                }
                p, err := d.sc.CreatePlanet(ctx, args[0], galAlias.ID, sysID)
                if err != nil {
                    return err
                }
                alias.ID = p.ID
            }
            if err := d.sc.InitPlanet(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("planet %q initialized — status: %s", args[0], beacon.Status))
            return nil
        },
    }
    cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this planet belongs to (required when creating)")
    cmd.Flags().StringVar(&system, "system", "", "solar system this planet belongs to (optional)")
    return cmd
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Smoke test**

```bash
go run ./cmd/orbit galaxy add freelance-work
go run ./cmd/orbit planet add auth-service --galaxy freelance-work
go run ./cmd/orbit planet init payments-api --galaxy freelance-work
```

- [ ] **Step 4: Commit**

```bash
git add internal/commands/planet.go
git commit -m "feat: implement planet add and init commands"
```

---

## Task 8: Transponder and Resource Add+Init Commands

**Files:**
- Create: `internal/commands/transponder.go`
- Create: `internal/commands/resource.go`

Brands for both are accepted as any string — validation happens when the integration registry is consulted during init.

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
    cmd.AddCommand(
        newTransponderAddCmd(d),
        newTransponderInitCmd(d),
    )
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
            d.renderer.Success(fmt.Sprintf("transponder %q registered (%s/%s) (%s)", args[0], role, brand, tp.ID))
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "access mechanism: file | env | keychain | vault | agent")
    cmd.Flags().StringVar(&brand, "brand", "", "service brand (any string — validated by integration at init time)")
    cmd.Flags().StringVar(&location, "location", "", "credential location: file path, env var name, vault path, etc.")
    _ = cmd.MarkFlagRequired("role")
    _ = cmd.MarkFlagRequired("brand")
    _ = cmd.MarkFlagRequired("location")
    return cmd
}

func newTransponderInitCmd(d *deps) *cobra.Command {
    var role, brand, location string
    cmd := &cobra.Command{
        Use:   "init <name>",
        Short: "Register and provision a transponder",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                tp, err := d.sc.CreateTransponder(ctx, args[0], role, brand, location)
                if err != nil {
                    return err
                }
                alias.ID = tp.ID
            }
            if err := d.sc.InitTransponder(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("transponder %q initialized — status: %s", args[0], beacon.Status))
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "access mechanism (required when creating)")
    cmd.Flags().StringVar(&brand, "brand", "", "service brand (required when creating)")
    cmd.Flags().StringVar(&location, "location", "", "credential location (required when creating)")
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
    cmd.Flags().StringVar(&role, "role", "", "resource role: manager | runtime | tool | remote | filesystem")
    cmd.Flags().StringVar(&brand, "brand", "", "resource brand (any string — validated by integration at init time)")
    cmd.Flags().StringVar(&manages, "manages", "", `JSON array of brands this manager controls, e.g. '["node","npm"]'`)
    cmd.Flags().StringVar(&config, "config", "", `JSON object of resource configuration, e.g. '{"default_version":"lts"}'`)
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
            ctx := cmd.Context()
            if manages == "" {
                manages = "[]"
            }
            if config == "" {
                config = "{}"
            }
            alias, err := d.sc.Resolve(ctx, args[0])
            if err != nil {
                r, err := d.sc.CreateResource(ctx, args[0], role, brand, manages, config)
                if err != nil {
                    return err
                }
                alias.ID = r.ID
            }
            if err := d.sc.InitResource(ctx, alias.ID); err != nil {
                return err
            }
            beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
            d.renderer.Success(fmt.Sprintf("resource %q initialized — status: %s", args[0], beacon.Status))
            return nil
        },
    }
    cmd.Flags().StringVar(&role, "role", "", "resource role (required when creating)")
    cmd.Flags().StringVar(&brand, "brand", "", "resource brand (required when creating)")
    cmd.Flags().StringVar(&manages, "manages", "", "JSON array of brands this manager controls (required when creating a manager)")
    cmd.Flags().StringVar(&config, "config", "", "JSON object of resource configuration")
    return cmd
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: Smoke test**

```bash
go run ./cmd/orbit transponder add work-github --role file --brand github --location ~/.ssh/id_ed25519_work
go run ./cmd/orbit transponder init work-aws --role vault --brand aws --location "op://Work/AWS/access_key"
go run ./cmd/orbit resource add node-version-mgr --role manager --brand nvm --manages '["node","npm"]'
go run ./cmd/orbit resource init python-env --role manager --brand uv --manages '["python","pip"]'
```

Expected: add commands succeed; init commands succeed but print status `failed` (no integrations registered yet).

- [ ] **Step 5: Commit**

```bash
git add internal/commands/transponder.go internal/commands/resource.go
git commit -m "feat: implement transponder and resource add/init commands"
```

---

## Task 9: Attach Command and Root Update

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
        Long: `attach creates a directed edge in the Star Chart graph.

<from> is the child entity, <to> is the parent. "vessel" is a reserved
target meaning the global vessel (available everywhere, all contexts).

Examples:
  orbit attach work-github  work-dev       # transponder → callsign
  orbit attach work-dev     freelance-work  # callsign → galaxy
  orbit attach node-version-mgr  vessel    # resource → vessel (global)
  orbit attach node-version-mgr  payments-api  # resource → planet (scoped)`,
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
# Build the universe for a freelance project
go run ./cmd/orbit galaxy add freelance-work
go run ./cmd/orbit callsign add dev-primary
go run ./cmd/orbit transponder add work-github --role file --brand github --location ~/.ssh/id_ed25519_work
go run ./cmd/orbit resource add node-version-mgr --role manager --brand nvm --manages '["node","npm"]'
go run ./cmd/orbit planet add payments-api --galaxy freelance-work

# Wire the graph
go run ./cmd/orbit attach work-github     dev-primary
go run ./cmd/orbit attach dev-primary     freelance-work
go run ./cmd/orbit attach node-version-mgr  vessel
go run ./cmd/orbit attach node-version-mgr  payments-api

# Init the whole galaxy — cascades to planet, resource (fails — no integration yet)
go run ./cmd/orbit galaxy init freelance-work
```

Expected: all attach and add operations succeed. `galaxy init` cascades and prints `degraded` status (because resource init fails with no integration registered).

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/commands/attach.go internal/commands/root.go
git commit -m "feat: implement attach command and wire into root"
```

---

## Self-Review

### Spec Coverage

| Requirement | Task |
|---|---|
| Beacon status constants (unverified/verified/failed/degraded/retired) | 1 |
| `EntityTypeAttachment = "at"` | 1 |
| Role+brand models — Resource, Transponder | 1 |
| Callsign without EntityID | 1 |
| Planet without RepoURL/RepoPath | 1 |
| Integration types (Platform, DetectContext, StateReport, ResolvedContext, etc.) | 2 |
| Integration Registry with NewRegistry/Register/Get/AllForRole | 2 |
| Package-level Default registry for init() self-registration | 2 |
| integrationProvider interface defined at consumer (starchart) | 5 |
| StarChart accepts injected registry via OpenWithRegistry | 5 |
| Atomic alias+entity+beacon creates | 3 |
| GetBeacon | 3 |
| Attach with one-callsign-per-node guard | 4 |
| BranchCrawl and BuildResolvedContext | 4 |
| InitGalaxy cascades to planets | 5 |
| InitSolarSystem cascades to planets | 5 |
| InitPlanet cascades to resources and callsigns | 5 |
| InitCallsign cascades to transponders | 5 |
| InitResource dispatches to integration registry | 5 |
| InitTransponder dispatches to integration registry | 5 |
| `galaxy add` + `galaxy init` | 6 |
| `system add` + `system init` | 6 |
| `callsign add` + `callsign init` | 6 |
| Remove entity stubs from stubs.go | 6 |
| `planet add` + `planet init` | 7 |
| `transponder add` + `transponder init` | 8 |
| `resource add` + `resource init` | 8 |
| `orbit attach <from> <to>` | 9 |

### Known Phase 2 Limitations (Intentional)

- **No integration implementations** — `resource init` and `transponder init` always record `failed` because no integrations are compiled in. Phase 3 adds integration packages.
- **No manifest loading** — `InitResource` passes an empty `Manifest{}` to `BuildResolvedContext`. Phase 3 loads manifests from the embedded filesystem alongside each integration package.
- **No context inference** — `--galaxy` must be specified explicitly. Navigation state inference is a Phase 3 concern.
- **No detection flow** — `planet init` cascades to attached resources only. File-pattern and always-run detection runs in Phase 3 when integration implementations exist.
- **Shallow branch crawl** — `BranchCrawl` collects direct attachments to the entity. Full hierarchy traversal (vessel → galaxy → system → planet) runs in Phase 3.
