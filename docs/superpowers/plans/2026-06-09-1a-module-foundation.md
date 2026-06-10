# Phase 1A: Module Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Initialize the Go module, create all directories, define domain model structs with db/json tags, write the initial SQLite migration, wire empty binary entry points, and produce a Justfile — resulting in a repo where both binaries compile cleanly with no logic yet.

**Architecture:** Single Go module (`github.com/Kenttleton/orbiter`) with two binaries under `cmd/`. Shared internal packages. Domain models are pure structs — no DB logic. Migrations live in `internal/migrations/` so `go:embed` can reference them without `../` traversal.

**Tech Stack:** Go 1.22+, `github.com/stretchr/testify`, Just (task runner)

---

## File Map

| File | Purpose |
|---|---|
| `go.mod` | Module declaration and dependencies |
| `go.sum` | Dependency checksums (generated) |
| `justfile` | Build, install, test, lint targets |
| `internal/migrations/migrations.go` | Embeds migration SQL files via go:embed |
| `internal/migrations/0001_initial.sql` | Full initial schema |
| `internal/models/id.go` | OrbitID generation, parsing, and validation helpers |
| `internal/models/alias.go` | Alias struct + EntityType constants |
| `internal/models/vessel.go` | Vessel struct |
| `internal/models/galaxy.go` | Galaxy struct |
| `internal/models/solar_system.go` | SolarSystem struct |
| `internal/models/planet.go` | Planet struct |
| `internal/models/callsign.go` | Callsign struct |
| `internal/models/transponder.go` | Transponder struct |
| `internal/models/resource.go` | Resource struct |
| `internal/models/default.go` | Default struct + well-known key constants |
| `internal/models/beacon.go` | Beacon struct + Status constants |
| `internal/models/navigation_history.go` | NavigationHistory struct |
| `cmd/orbit/main.go` | CLI entry point (empty) |
| `cmd/orbiter/main.go` | TUI entry point (empty) |

---

### Task 1: Initialize Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize the module**

```bash
go mod init github.com/Kenttleton/orbiter
```

Expected output: creates `go.mod` with `module github.com/Kenttleton/orbiter` and current Go version.

- [ ] **Step 2: Add runtime dependencies**

```bash
go get github.com/stretchr/testify@latest
```

- [ ] **Step 3: Verify go.mod looks correct**

```bash
cat go.mod
```

Expected: module declaration, Go version line, and `require` entry for testify.

- [ ] **Step 4: Commit**

```
git add go.mod go.sum
git commit -m "chore: initialize go module with testify dependency"
```

---

### Task 2: Create directory structure

**Files:**
- Create all directories listed below

- [ ] **Step 1: Create all package directories**

```bash
mkdir -p cmd/orbit \
         cmd/orbiter \
         internal/migrations \
         internal/models \
         internal/starchart \
         internal/resolver \
         internal/output \
         internal/commands \
         internal/tui \
         bin
```

- [ ] **Step 2: Add a .gitkeep to bin so it's tracked**

```bash
touch bin/.gitkeep
echo 'bin/*' >> .gitignore
echo '!bin/.gitkeep' >> .gitignore
```

- [ ] **Step 3: Commit**

```
git add .
git commit -m "chore: scaffold directory structure"
```

---

### Task 3: Write Justfile

**Files:**
- Create: `justfile`

- [ ] **Step 1: Write the Justfile**

Create `justfile` with this content:

```just
# Orbiter build tasks

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

test-verbose:
    go test -v ./...

lint:
    golangci-lint run

clean:
    rm -f bin/orbit bin/orbiter

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

- [ ] **Step 2: Verify Just is installed**

```bash
just --version
```

Expected: `just 1.x.x` or similar. If not installed: `brew install just` (macOS/Linux) or see https://github.com/casey/just.

- [ ] **Step 3: Commit**

```
git add justfile
git commit -m "chore: add justfile with build, install, test, and lint targets"
```

---

### Task 4: Write initial SQL migration

**Files:**
- Create: `internal/migrations/migrations.go`
- Create: `internal/migrations/0001_initial.sql`

- [ ] **Step 1: Write the embed package**

Create `internal/migrations/migrations.go`:

```go
package migrations

import "embed"

// FS contains all migration SQL files, embedded at compile time.
//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Write the initial schema migration**

Create `internal/migrations/0001_initial.sql`:

```sql
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Global OrbitID registry and alias table.
-- Every entity is registered here when created.
-- name defaults to id when no human-readable alias is given.
-- Uniqueness of name is enforced globally across all entity types.
CREATE TABLE IF NOT EXISTS aliases (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    entity_type TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Single-row table representing the local workstation.
-- Single-row constraint enforced by application logic.
CREATE TABLE IF NOT EXISTS vessel (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS galaxies (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS solar_systems (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS planets (
    id              TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id       TEXT NOT NULL REFERENCES aliases(id),
    solar_system_id TEXT REFERENCES aliases(id),
    repo_url        TEXT,
    repo_path       TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Callsigns represent the Captain's active identity.
-- Scoped to a vessel or galaxy via entity_id.
CREATE TABLE IF NOT EXISTS callsigns (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Transponders are pointers to credential locations — never the credentials themselves.
-- Always linked to a callsign. Optionally narrowed to a specific entity.
CREATE TABLE IF NOT EXISTS transponders (
    id           TEXT PRIMARY KEY REFERENCES aliases(id),
    callsign_id  TEXT NOT NULL REFERENCES aliases(id),
    entity_id    TEXT REFERENCES aliases(id),
    service      TEXT NOT NULL,
    location     TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Resources represent tooling, runtimes, and capabilities.
-- Scoped to any entity via entity_id.
CREATE TABLE IF NOT EXISTS resources (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    kind       TEXT NOT NULL,
    manager    TEXT,
    version    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Defaults store configuration key/value pairs scoped to any entity.
-- Vessel-level defaults include output_format and history_retention_days.
CREATE TABLE IF NOT EXISTS defaults (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, key)
);

-- Immutable log of navigation events.
-- IDs here are NOT in the alias registry — they are log record IDs only.
CREATE TABLE IF NOT EXISTS navigation_history (
    id             TEXT PRIMARY KEY,
    from_entity_id TEXT REFERENCES aliases(id),
    to_entity_id   TEXT NOT NULL REFERENCES aliases(id),
    command        TEXT NOT NULL,
    occurred_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS navigation_history_occurred_idx
    ON navigation_history(occurred_at);

-- Most recent verified observation of an entity. One beacon per entity.
-- IDs here are NOT in the alias registry — they are observation record IDs only.
CREATE TABLE IF NOT EXISTS beacons (
    id           TEXT PRIMARY KEY,
    entity_id    TEXT NOT NULL REFERENCES aliases(id),
    status       TEXT NOT NULL,
    observations TEXT NOT NULL,
    verified_at  DATETIME NOT NULL,
    UNIQUE(entity_id)
);

INSERT INTO schema_version (version) VALUES (1);
```

- [ ] **Step 3: Write a test that the SQL file is parseable (basic sanity)**

Create `internal/migrations/migrations_test.go`:

```go
package migrations_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/migrations"
    "github.com/stretchr/testify/require"
)

func TestFSContainsMigrations(t *testing.T) {
    entries, err := migrations.FS.ReadDir(".")
    require.NoError(t, err)
    require.NotEmpty(t, entries, "migrations directory should contain at least one SQL file")
}

func TestInitialMigrationReadable(t *testing.T) {
    data, err := migrations.FS.ReadFile("0001_initial.sql")
    require.NoError(t, err)
    require.Contains(t, string(data), "schema_version")
    require.Contains(t, string(data), "aliases")
    require.Contains(t, string(data), "vessel")
}
```

- [ ] **Step 4: Run the test**

```bash
go test ./internal/migrations/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```
git add internal/migrations/
git commit -m "feat: add initial SQLite migration with full Star Chart schema"
```

---

### Task 5: Write domain model structs

**Files:**

- Create: `internal/models/id.go`
- Create: `internal/models/alias.go`
- Create: `internal/models/vessel.go`
- Create: `internal/models/galaxy.go`
- Create: `internal/models/solar_system.go`
- Create: `internal/models/planet.go`
- Create: `internal/models/callsign.go`
- Create: `internal/models/transponder.go`
- Create: `internal/models/resource.go`
- Create: `internal/models/default.go`
- Create: `internal/models/beacon.go`
- Create: `internal/models/navigation_history.go`

- [ ] **Step 1: Write OrbitID helpers**

Create `internal/models/id.go`:

```go
package models

import (
    "fmt"
    "math/rand/v2"
    "strconv"
    "strings"
    "time"
)

const epochMs = int64(1735689600000) // 2025-01-01 00:00:00 UTC

// Entity type prefix constants — 2-char codes embedded in every OrbitID.
const (
    EntityTypeVessel      = "vs"
    EntityTypeGalaxy      = "gx"
    EntityTypeSolarSystem = "sy"
    EntityTypePlanet      = "pl"
    EntityTypeCallsign    = "cs"
    EntityTypeTransponder = "tp"
    EntityTypeResource    = "rs"
    EntityTypeDefault     = "df"
    EntityTypeBeacon      = "bk"
    EntityTypeNavHistory  = "nh"
)

// OrbitID is the parsed form of a 16-character OrbitID string.
type OrbitID struct {
    Raw        string
    EntityType string
    Timestamp  time.Time
}

// NewID generates a new 16-character OrbitID for the given entity type prefix.
// Format: [8 base36 timestamp][2 entity type][6 base36 random]
func NewID(entityType string) string {
    ts := uint64(time.Now().UnixMilli() - epochMs)
    rnd := rand.Uint32()

    tsPart := strconv.FormatUint(ts, 36)
    tsPart = strings.Repeat("0", max(0, 8-len(tsPart))) + tsPart

    rndPart := strconv.FormatUint(uint64(rnd), 36)
    rndPart = strings.Repeat("0", max(0, 6-len(rndPart))) + rndPart

    return tsPart + entityType + rndPart
}

// ParseID extracts entity type and timestamp from an OrbitID string.
func ParseID(id string) (OrbitID, error) {
    if len(id) != 16 {
        return OrbitID{}, fmt.Errorf("invalid orbit id length: %d", len(id))
    }
    tsPart := id[:8]
    entityType := id[8:10]
    tsVal, err := strconv.ParseUint(tsPart, 36, 64)
    if err != nil {
        return OrbitID{}, fmt.Errorf("invalid orbit id timestamp: %w", err)
    }
    ts := time.UnixMilli(int64(tsVal) + epochMs)
    return OrbitID{Raw: id, EntityType: entityType, Timestamp: ts}, nil
}

// IsID reports whether s matches the OrbitID format (16 chars, valid base36 timestamp).
func IsID(s string) bool {
    if len(s) != 16 {
        return false
    }
    _, err := ParseID(s)
    return err == nil
}
```

- [ ] **Step 2: Write Alias model**

Create `internal/models/alias.go`:

```go
package models

import "time"

// Alias is a row in the global OrbitID registry.
// Every entity has exactly one alias. Name defaults to ID when no
// human-readable name is provided.
// EntityType constants are defined in id.go.
type Alias struct {
    ID         string    `db:"id"          json:"id"`
    Name       string    `db:"name"        json:"name"`
    EntityType string    `db:"entity_type" json:"entity_type"`
    CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}
```

- [ ] **Step 3: Write remaining entity models**

Create `internal/models/vessel.go`:

```go
package models

import "time"

// Vessel represents the local workstation — the Orbiter itself.
// Only one row exists per Star Chart.
type Vessel struct {
    ID        string    `db:"id"         json:"id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/galaxy.go`:

```go
package models

import "time"

// Galaxy represents an organization or client (e.g. "acme", "personal").
type Galaxy struct {
    ID        string    `db:"id"         json:"id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/solar_system.go`:

```go
package models

import "time"

// SolarSystem is an optional organizational subdivision within a Galaxy
// (e.g. "platform", "mobile").
type SolarSystem struct {
    ID        string    `db:"id"         json:"id"`
    GalaxyID  string    `db:"galaxy_id"  json:"galaxy_id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/planet.go`:

```go
package models

import "time"

// Planet represents a project — the primary navigation target.
type Planet struct {
    ID             string    `db:"id"              json:"id"`
    GalaxyID       string    `db:"galaxy_id"       json:"galaxy_id"`
    SolarSystemID  string    `db:"solar_system_id" json:"solar_system_id,omitempty"`
    RepoURL        string    `db:"repo_url"        json:"repo_url,omitempty"`
    RepoPath       string    `db:"repo_path"       json:"repo_path,omitempty"`
    CreatedAt      time.Time `db:"created_at"      json:"created_at"`
}
```

Create `internal/models/callsign.go`:

```go
package models

import "time"

// Callsign represents the Captain's active identity (e.g. "kent-acme").
// Scoped to a vessel or galaxy via EntityID.
type Callsign struct {
    ID        string    `db:"id"         json:"id"`
    EntityID  string    `db:"entity_id"  json:"entity_id"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/transponder.go`:

```go
package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself.
// Always linked to a Callsign. Optionally narrowed to a specific entity.
type Transponder struct {
    ID          string    `db:"id"           json:"id"`
    CallsignID  string    `db:"callsign_id"  json:"callsign_id"`
    EntityID    string    `db:"entity_id"    json:"entity_id,omitempty"`
    Service     string    `db:"service"      json:"service"`
    Location    string    `db:"location"     json:"location"`
    CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}
```

Create `internal/models/resource.go`:

```go
package models

import "time"

// Resource describes a tooling requirement, runtime, or capability
// scoped to any entity.
type Resource struct {
    ID        string    `db:"id"         json:"id"`
    EntityID  string    `db:"entity_id"  json:"entity_id"`
    Kind      string    `db:"kind"       json:"kind"`
    Manager   string    `db:"manager"    json:"manager,omitempty"`
    Version   string    `db:"version"    json:"version,omitempty"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/default.go`:

```go
package models

import "time"

// Well-known default keys stored at the vessel level.
const (
    DefaultKeyOutputFormat      = "output_format"
    DefaultKeyHistoryRetention  = "history_retention_days"
)

// Default is a key/value configuration entry scoped to any entity.
type Default struct {
    ID        string    `db:"id"         json:"id"`
    EntityID  string    `db:"entity_id"  json:"entity_id"`
    Key       string    `db:"key"        json:"key"`
    Value     string    `db:"value"      json:"value"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}
```

Create `internal/models/beacon.go`:

```go
package models

import "time"

// Beacon status values.
const (
    BeaconStatusHealthy = "healthy"
    BeaconStatusDrifted = "drifted"
    BeaconStatusUnknown = "unknown"
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

Create `internal/models/navigation_history.go`:

```go
package models

import "time"

// NavigationHistory is an immutable log entry recording a navigation event.
type NavigationHistory struct {
    ID             string    `db:"id"             json:"id"`
    FromEntityID   string    `db:"from_entity_id" json:"from_entity_id,omitempty"`
    ToEntityID     string    `db:"to_entity_id"   json:"to_entity_id"`
    Command        string    `db:"command"        json:"command"`
    OccurredAt     time.Time `db:"occurred_at"    json:"occurred_at"`
}
```

- [ ] **Step 4: Write model tests**

Create `internal/models/models_test.go`:

```go
package models_test

import (
    "encoding/json"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
    a := models.NewID(models.EntityTypePlanet)
    b := models.NewID(models.EntityTypePlanet)
    require.Len(t, a, 16)
    require.Len(t, b, 16)
    require.NotEqual(t, a, b, "successive IDs must differ")
}

func TestNewIDEmbeddsEntityType(t *testing.T) {
    id := models.NewID(models.EntityTypePlanet)
    require.Equal(t, models.EntityTypePlanet, id[8:10])
}

func TestIsID(t *testing.T) {
    valid := models.NewID(models.EntityTypeGalaxy)
    require.True(t, models.IsID(valid))
    require.False(t, models.IsID("payment-api"))
    require.False(t, models.IsID(""))
    require.False(t, models.IsID("tooshort"))
}

func TestParseID(t *testing.T) {
    id := models.NewID(models.EntityTypePlanet)
    parsed, err := models.ParseID(id)
    require.NoError(t, err)
    require.Equal(t, id, parsed.Raw)
    require.Equal(t, models.EntityTypePlanet, parsed.EntityType)
    require.False(t, parsed.Timestamp.IsZero())
}

func TestAliasJSONRoundtrip(t *testing.T) {
    a := models.Alias{
        ID:         models.NewID(models.EntityTypePlanet),
        Name:       "payment-api",
        EntityType: models.EntityTypePlanet,
    }
    data, err := json.Marshal(a)
    require.NoError(t, err)

    var got models.Alias
    require.NoError(t, json.Unmarshal(data, &got))
    require.Equal(t, a.ID, got.ID)
    require.Equal(t, a.Name, got.Name)
    require.Equal(t, a.EntityType, got.EntityType)
}

func TestEntityTypeConstants(t *testing.T) {
    types := []string{
        models.EntityTypeVessel,
        models.EntityTypeGalaxy,
        models.EntityTypeSolarSystem,
        models.EntityTypePlanet,
        models.EntityTypeCallsign,
        models.EntityTypeTransponder,
        models.EntityTypeResource,
        models.EntityTypeDefault,
        models.EntityTypeBeacon,
        models.EntityTypeNavHistory,
    }
    for _, et := range types {
        require.Len(t, et, 2, "entity type prefix must be 2 chars: %q", et)
    }
}
```

- [ ] **Step 5: Run the model tests**

```bash
go test ./internal/models/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```
git add internal/models/
git commit -m "feat: add domain model structs with db/json tags and OrbitID helpers"
```

---

### Task 6: Write empty binary entry points

**Files:**
- Create: `cmd/orbit/main.go`
- Create: `cmd/orbiter/main.go`

- [ ] **Step 1: Write the orbit entry point stub**

Create `cmd/orbit/main.go`:

```go
package main

import "fmt"

func main() {
    fmt.Println("orbit: not yet implemented")
}
```

- [ ] **Step 2: Write the orbiter entry point stub**

Create `cmd/orbiter/main.go`:

```go
package main

import "fmt"

func main() {
    fmt.Println("orbiter: not yet implemented")
}
```

- [ ] **Step 3: Verify both binaries compile**

```bash
just build
```

Expected:

```
go build -o bin/orbit ./cmd/orbit
go build -o bin/orbiter ./cmd/orbiter
```

No errors.

- [ ] **Step 4: Smoke-test both binaries run**

```bash
./bin/orbit
./bin/orbiter
```

Expected:

```
orbit: not yet implemented
orbiter: not yet implemented
```

- [ ] **Step 5: Commit**

```
git add cmd/
git commit -m "chore: add empty binary entry points for orbit and orbiter"
```

---

### Task 7: Verify full module state

- [ ] **Step 1: Run all tests**

```bash
just test
```

Expected: all tests pass (`models`, `migrations`).

- [ ] **Step 2: Verify go.mod is tidy**

```bash
go mod tidy
git diff go.mod go.sum
```

Expected: no unexpected changes. If there are changes, stage and commit them:

```
git add go.mod go.sum
git commit -m "chore: tidy go module"
```

- [ ] **Step 3: Verify build targets all work**

```bash
just build
just test
```

Expected: both pass with no errors.
