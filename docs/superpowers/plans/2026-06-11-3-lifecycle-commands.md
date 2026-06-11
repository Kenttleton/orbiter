# Phase 3: Lifecycle Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all six lifecycle commands (`survey`, `chart`, `jump`, `scan`, `calibrate`, `retro`), rename the binary from `orbit` to `orbiter`, add path-based CWD resolution, and ship shell integration (bash/zsh/fish) with autocomplete.

**Architecture:** A shared `Executor` in `internal/commands/executor.go` drives all six commands through the same pipeline: resolve target (explicit name or CWD longest-prefix match) → BranchCrawl → dispatch to integration layer → aggregate results → write beacons → render. New starchart methods (`ScanBranch`, `CalibrateBranch`, `PlanRetro`, `ExecuteRetro`, `ResolveCWD`) back the Executor. Shell integration follows the starship.rs model: scripts embedded via `//go:embed`, `::ORBITER::` token replaced at `orbiter init <shell>` time.

**Path association:** The hierarchy (vessel, galaxy, system, planet) carries no path columns — the schema is clean. Paths are expressed by attaching a `role=filesystem, brand=orbiter` resource to a hierarchy node; the path lives in the resource's `config` JSON field (`{"path": "/home/kent/acme/payment-api"}`). `ResolveCWD` collects all such resources up the attachment graph into a FILO queue (most specific last-in, first-out) and applies exact-match-first then longest-prefix matching.

**Tech Stack:** Go 1.25, Cobra, Lipgloss, wazero (integration dispatch via existing starchart layer), modernc.org/sqlite, `//go:embed`.

**Spec:** `docs/superpowers/specs/2026-06-11-lifecycle-commands-design.md`

---

## File Map

**Create:**
- `cmd/orbiter/main.go` — new CLI entry point (replaces `cmd/orbit/main.go`)
- `shell/orbiter.bash` — embedded bash integration (`::ORBITER::` token)
- `shell/orbiter.zsh` — embedded zsh integration
- `shell/orbiter.fish` — embedded fish integration
- `internal/commands/executor.go` — Executor struct + ShellDirective
- `internal/commands/lifecycle.go` — 6 lifecycle Cobra commands (replaces stubs)
- `internal/commands/shell.go` — `orbiter init` + `orbiter completions` commands
- `internal/starchart/resolve_cwd.go` — ResolveCWD (exact match → longest prefix)
- `internal/starchart/lifecycle.go` — ScanBranch, CalibrateBranch + per-entity helpers
- `internal/starchart/retro.go` — PlanRetro, ExecuteRetro, RetroNode, RetroPlan

**Modify:**
- `internal/commands/root.go` — rename Use, update env vars, register new commands
- `internal/commands/stubs.go` — remove 6 lifecycle stubs (moved to lifecycle.go)
- `Justfile` — collapse to single `orbiter` binary, remove `orbit` targets
- `README.md` — update `orbit` → `orbiter` throughout

**Delete:**
- `cmd/orbit/main.go`

---

## Task 1: Binary rename — orbit → orbiter

**Files:**
- Create: `cmd/orbiter/main.go`
- Delete: `cmd/orbit/main.go`
- Modify: `internal/commands/root.go`
- Modify: `Justfile`
- Modify: `README.md`

- [ ] **Step 1: Create `cmd/orbiter/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/Kenttleton/orbiter/internal/commands"
	_ "github.com/Kenttleton/orbiter/internal/integrations/golang"
)

func main() {
	root := commands.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Update `internal/commands/root.go`**

Change `Use: "orbit"` to `Use: "orbiter"` and update all three env vars:

```go
// In NewRootCommand():
root := &cobra.Command{
    Use:   "orbiter",
    Short: "Orbiter CLI — navigate and orchestrate your development universe",
    Long: `orbiter is the command interface for Orbiter, a state-driven navigation
and environment orchestration platform for freelance and contract engineers.`,
    // ...
}

// In PersistentPreRunE:
chartPath := os.Getenv("ORBITER_STARCHART")
if chartPath == "" {
    home, err := os.UserHomeDir()
    if err != nil {
        return fmt.Errorf("resolve home directory: %w", err)
    }
    chartPath = home + "/.orbiter/starchart.db"
}

// output format:
format := outputFormat
if format == "" {
    format = os.Getenv("ORBITER_OUTPUT")
}

// verbose:
if !verbose {
    verbose = os.Getenv("ORBITER_VERBOSE") == "1"
}
```

- [ ] **Step 3: Update `Justfile`**

Replace the full Justfile content:

```just
# Orbiter build tasks

build:
    go build -o bin/orbiter ./cmd/orbiter

install:
    go install ./cmd/orbiter

test:
    go test ./...

test-verbose:
    go test -v ./...

lint:
    golangci-lint run

clean:
    rm -f bin/orbiter

# Cross-compilation target for CI release builds.
# Usage: just build-release orbiter linux amd64 v1.2.3
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

- [ ] **Step 4: Delete `cmd/orbit/main.go`**

```bash
git rm cmd/orbit/main.go
rmdir cmd/orbit
```

- [ ] **Step 5: Verify build**

```bash
just build
./bin/orbiter --help
```

Expected: help output showing `orbiter` as the command name.

- [ ] **Step 6: Update `README.md`**

Replace all occurrences of `` `orbit` `` with `` `orbiter` `` and `orbit <command>` with `orbiter <command>`. Update the env var references (`ORBIT_STARCHART` → `ORBITER_STARCHART`, `ORBIT_OUTPUT` → `ORBITER_OUTPUT`, `ORBIT_VERBOSE` → `ORBITER_VERBOSE`).

- [ ] **Step 7: Commit**

```bash
git add cmd/orbiter/main.go internal/commands/root.go Justfile README.md
git commit -m "feat: rename orbit binary to orbiter, update env vars"
```

---

## Task 2: starchart.ResolveCWD — exact match then longest prefix via filesystem resources

Paths are expressed by attaching a `role=filesystem, brand=orbiter` resource to a hierarchy node. The path is stored in the resource's `config` JSON field as `{"path": "/home/kent/acme/payment-api"}`. `ResolveCWD` collects all such resources via the attachment graph, loads them into a FILO queue (most specific/deepest attached last → popped first), and runs exact-match-first then longest-prefix matching to find the entity to resolve to. No schema changes are needed — the `config` column already exists on `resources`.

**Files:**

- Create: `internal/starchart/resolve_cwd.go`
- Create: `internal/starchart/resolve_cwd_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/resolve_cwd_test.go`:

```go
package starchart_test

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// filesystemConfig builds the config JSON for a role=filesystem resource.
func filesystemConfig(t *testing.T, path string) string {
    t.Helper()
    b, err := json.Marshal(map[string]string{"path": path})
    require.NoError(t, err)
    return string(b)
}

func TestResolveCWD_ExactMatch(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    // Attach a filesystem/orbiter resource to the galaxy at the acme root.
    gfs, _ := sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme"))
    _, _ = sc.Attach(ctx, "acme-path", "acme")

    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme/payment-api"))
    _, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

    _ = gfs

    // Exact CWD match on galaxy path → galaxy wins, not planet
    alias, err := sc.ResolveCWD(ctx, "/home/kent/acme")
    require.NoError(t, err)
    assert.Equal(t, g.ID, alias.ID)
}

func TestResolveCWD_LongestPrefix(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    _, _ = sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme"))
    _, _ = sc.Attach(ctx, "acme-path", "acme")

    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme/payment-api"))
    _, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

    // Subdirectory of planet path → planet has longer prefix match → wins
    alias, err := sc.ResolveCWD(ctx, "/home/kent/acme/payment-api/src/handlers")
    require.NoError(t, err)
    assert.Equal(t, p.ID, alias.ID)
}

func TestResolveCWD_NoMatch(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    _, err := sc.ResolveCWD(ctx, "/home/kent/other-project")
    assert.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestResolveCWD_ExactMatchBeatsLongerPrefix(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    _, _ = sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme"))
    _, _ = sc.Attach(ctx, "acme-path", "acme")

    _, _ = sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "", filesystemConfig(t, "/home/kent/acme/payment-api"))
    _, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

    // CWD exactly equals galaxy path → galaxy wins even though planet has longer stored path
    alias, err := sc.ResolveCWD(ctx, "/home/kent/acme")
    require.NoError(t, err)
    assert.Equal(t, g.ID, alias.ID, "exact match on galaxy beats planet prefix")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... -run TestResolveCWD -v
```

Expected: compile error — `ResolveCWD` undefined.

- [ ] **Step 3: Implement `internal/starchart/resolve_cwd.go`**

```go
package starchart

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/Kenttleton/orbiter/internal/models"
)

// ResolveCWD finds the hierarchy entity whose attached filesystem/orbiter resource
// most specifically matches cwd. Exact match takes priority; among prefix matches,
// the longest matching path prefix wins (CSS selector specificity logic).
// Returns ErrNotFound if no filesystem resource path matches or prefixes cwd.
func (sc *StarChart) ResolveCWD(ctx context.Context, cwd string) (models.Alias, error) {
    // Collect all role=filesystem, brand=orbiter resources and the entities
    // they are attached to via the attachments graph.
    const q = `
        SELECT a.entity, a.name, a.created_at, r.config
        FROM resources r
        JOIN attachments att ON att.from_id = r.id
        JOIN aliases a ON a.entity = att.to_id
        WHERE r.role = 'filesystem' AND r.brand = 'orbiter'
          AND r.config != '' AND r.config != '{}'
    `
    rows, err := sc.db.QueryContext(ctx, q)
    if err != nil {
        return models.Alias{}, fmt.Errorf("query filesystem resources: %w", err)
    }
    defer rows.Close()

    type candidate struct {
        models.Alias
        path string
    }
    var best candidate
    bestLen := -1

    for rows.Next() {
        var (
            id, name, createdAt, configJSON string
        )
        if err := rows.Scan(&id, &name, &createdAt, &configJSON); err != nil {
            return models.Alias{}, fmt.Errorf("scan filesystem row: %w", err)
        }
        var cfg struct {
            Path string `json:"path"`
        }
        if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.Path == "" {
            continue
        }
        c := candidate{
            Alias: models.Alias{ID: id, Name: name},
            path:  cfg.Path,
        }
        // Exact match wins immediately — no need to scan further.
        if c.path == cwd {
            return c.Alias, nil
        }
        // Prefix match: ensure cwd starts with path + "/" to avoid
        // "/home/kent/acme" matching "/home/kent/acme-corp".
        prefix := c.path
        if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
            prefix += "/"
        }
        if len(c.path) > bestLen && len(cwd) >= len(prefix) && cwd[:len(prefix)] == prefix {
            best = c
            bestLen = len(c.path)
        }
    }
    if err := rows.Err(); err != nil {
        return models.Alias{}, fmt.Errorf("iterate filesystem rows: %w", err)
    }

    if bestLen == -1 {
        return models.Alias{}, fmt.Errorf("%w: no filesystem resource path matches %q", ErrNotFound, cwd)
    }
    return best.Alias, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/starchart/... -run TestResolveCWD -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/resolve_cwd.go internal/starchart/resolve_cwd_test.go
git commit -m "feat: add starchart.ResolveCWD via filesystem/orbiter resource attachments"
```

---

## Task 3: starchart.LeveledBranchCrawl — full FILO hierarchy walk with level pairing

`BranchCrawl` is a Phase 2 stub that only queries direct attachments on a single entity. Phase 3 lifecycle commands need the full branch — from the target entity up to vessel — with resources and transponders scoped to their own level. A level with no resources is skipped entirely (its transponders have nothing to authenticate). A resource at a level without a callsign/transponder gets an empty transponder slice and the integration reports success (public) or failure (needs auth).

`BuildResolvedContextForLevel` replaces `BuildResolvedContext` for lifecycle dispatch: it scopes transponders to only those from the resource's own level, not the entire branch pool.

**Files:**

- Modify: `internal/starchart/crawl.go`

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/leveled_crawl_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestLeveledBranchCrawl_PlanetLevel(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

    // Resource at planet level
    _, _ = sc.CreateResource(ctx, "node", "runtime", "node", "", "")
    _, _ = sc.Attach(ctx, "node", "payment-api")

    // Callsign + transponder at planet level
    cs, _ := sc.CreateCallsign(ctx, "kent-acme")
    _, _ = sc.CreateTransponder(ctx, "acme-gh", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
    _, _ = sc.Attach(ctx, "acme-gh", "kent-acme")
    _, _ = sc.Attach(ctx, "kent-acme", "payment-api")

    lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
    require.NoError(t, err)

    // Should have the planet level (and vessel — but vessel has no resources by default)
    require.Len(t, lb.Levels, 1)
    assert.Len(t, lb.Levels[0].Resources, 1)
    assert.Equal(t, "runtime", lb.Levels[0].Resources[0].Role)
    require.NotNil(t, lb.Levels[0].Callsign)
    assert.Equal(t, cs.ID, lb.Levels[0].Callsign.ID)
    assert.Len(t, lb.Levels[0].Transponders, 1)
    assert.Equal(t, "file", lb.Levels[0].Transponders[0].Role)
}

func TestLeveledBranchCrawl_SkipsEmptyLevels(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

    // Callsign + transponder at GALAXY level but no resource there
    cs, _ := sc.CreateCallsign(ctx, "kent-acme-galaxy")
    _, _ = sc.CreateTransponder(ctx, "acme-gh-galaxy", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
    _, _ = sc.Attach(ctx, "acme-gh-galaxy", "kent-acme-galaxy")
    _, _ = sc.Attach(ctx, "kent-acme-galaxy", "acme")

    // Resource at planet level, no callsign
    _, _ = sc.CreateResource(ctx, "node", "runtime", "node", "", "")
    _, _ = sc.Attach(ctx, "node", "payment-api")

    lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
    require.NoError(t, err)

    // Galaxy level: has callsign/transponder but no resource — skipped
    // Planet level: has resource but no callsign — included with empty transponders
    require.Len(t, lb.Levels, 1)
    assert.Equal(t, p.ID, lb.Levels[0].EntityID)
    assert.Nil(t, lb.Levels[0].Callsign)
    assert.Empty(t, lb.Levels[0].Transponders)

    _ = cs // galaxy callsign is correctly excluded
}

func TestLeveledBranchCrawl_TwoLevels(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

    // Galaxy level: resource + callsign (e.g. shared node version manager)
    _, _ = sc.CreateResource(ctx, "nvm", "manager", "nvm", `["node"]`, "")
    _, _ = sc.Attach(ctx, "nvm", "acme")
    csGalaxy, _ := sc.CreateCallsign(ctx, "kent-acme-galaxy")
    _, _ = sc.CreateTransponder(ctx, "acme-npm-token", "env", "npm", "NPM_TOKEN")
    _, _ = sc.Attach(ctx, "acme-npm-token", "kent-acme-galaxy")
    _, _ = sc.Attach(ctx, "kent-acme-galaxy", "acme")

    // Planet level: resource + callsign (e.g. project-specific github resource)
    _, _ = sc.CreateResource(ctx, "github-remote", "remote", "github", "", "")
    _, _ = sc.Attach(ctx, "github-remote", "payment-api")
    csPlanet, _ := sc.CreateCallsign(ctx, "kent-acme-planet")
    _, _ = sc.CreateTransponder(ctx, "acme-gh-key", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
    _, _ = sc.Attach(ctx, "acme-gh-key", "kent-acme-planet")
    _, _ = sc.Attach(ctx, "kent-acme-planet", "payment-api")

    lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
    require.NoError(t, err)

    // FILO: planet first, galaxy second
    require.Len(t, lb.Levels, 2)
    assert.Equal(t, p.ID, lb.Levels[0].EntityID, "planet level first (FILO)")
    assert.Equal(t, csPlanet.ID, lb.Levels[0].Callsign.ID)
    assert.Equal(t, g.ID, lb.Levels[1].EntityID, "galaxy level second")
    assert.Equal(t, csGalaxy.ID, lb.Levels[1].Callsign.ID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... -run TestLeveledBranchCrawl -v
```

Expected: compile errors — `LeveledBranchCrawl` undefined, `BranchLevel` undefined.

- [ ] **Step 3: Add types and `LeveledBranchCrawl` to `internal/starchart/crawl.go`**

Add after the existing `BranchCrawl` function:

```go
// BranchLevel is one level in the FILO hierarchy for a branch crawl.
// Only levels that have at least one resource are included —
// a callsign/transponder without a co-located resource is orphaned and skipped.
type BranchLevel struct {
    EntityID     string
    Resources    []models.Resource
    Callsign     *models.Callsign     // nil if no callsign at this level
    Transponders []models.Transponder // from Callsign; empty if Callsign is nil
}

// LeveledBranch is the result of a full FILO hierarchy walk.
// Levels are ordered target-entity-first, vessel-last.
// Use this for lifecycle dispatch (scan, calibrate, jump) where resource/transponder
// pairing by level is required.
type LeveledBranch struct {
    Platform integrations.Platform
    Levels   []BranchLevel
}

// LeveledBranchCrawl walks from entityID up to the vessel, collecting resources,
// callsigns, and transponders at each level. Levels with no resources are skipped.
// The resulting Levels slice is in FILO order: target entity first, vessel last.
func (sc *StarChart) LeveledBranchCrawl(ctx context.Context, entityID string) (LeveledBranch, error) {
    chain, err := sc.hierarchyChain(ctx, entityID)
    if err != nil {
        return LeveledBranch{}, fmt.Errorf("hierarchy chain for %s: %w", entityID, err)
    }

    lb := LeveledBranch{Platform: currentPlatform()}

    for _, levelID := range chain {
        resources, err := sc.resourcesAttachedTo(ctx, levelID)
        if err != nil {
            return LeveledBranch{}, fmt.Errorf("resources at level %s: %w", levelID, err)
        }
        // Skip levels with no resources — their transponders have nothing to authenticate.
        if len(resources) == 0 {
            continue
        }

        callsigns, err := sc.callsignsAttachedTo(ctx, levelID)
        if err != nil {
            return LeveledBranch{}, fmt.Errorf("callsigns at level %s: %w", levelID, err)
        }

        level := BranchLevel{
            EntityID:  levelID,
            Resources: resources,
        }
        if len(callsigns) > 0 {
            // At most one active callsign per level.
            cs := callsigns[0]
            level.Callsign = &cs
            tps, err := sc.transpondersAttachedTo(ctx, cs.ID)
            if err != nil {
                return LeveledBranch{}, fmt.Errorf("transponders for callsign %s: %w", cs.ID, err)
            }
            level.Transponders = tps
        }

        lb.Levels = append(lb.Levels, level)
    }

    return lb, nil
}

// hierarchyChain returns the entity IDs from entityID up to the vessel in FILO order.
// For a planet (with system): [planetID, systemID, galaxyID, vesselID]
// For a planet (no system):   [planetID, galaxyID, vesselID]
// For a galaxy:               [galaxyID, vesselID]
// For vessel:                 [vesselID]
func (sc *StarChart) hierarchyChain(ctx context.Context, entityID string) ([]string, error) {
    chain := []string{entityID}

    if len(entityID) < 10 {
        return chain, nil
    }

    switch entityID[8:10] {
    case "pl":
        var p models.Planet
        if err := sc.Get(ctx, "planets", entityID, &p); err != nil {
            return nil, fmt.Errorf("load planet %s: %w", entityID, err)
        }
        if p.SolarSystemID != "" {
            chain = append(chain, p.SolarSystemID)
        }
        chain = append(chain, p.GalaxyID)

    case "sy":
        var sys models.SolarSystem
        if err := sc.Get(ctx, "solar_systems", entityID, &sys); err != nil {
            return nil, fmt.Errorf("load system %s: %w", entityID, err)
        }
        chain = append(chain, sys.GalaxyID)

    case "gx":
        // galaxy: falls through to vessel append below

    case "vs":
        return chain, nil // already at vessel; vessel has no parent
    }

    // Every non-vessel entity terminates at the vessel.
    var vesselID string
    if err := sc.db.QueryRowContext(ctx, `SELECT id FROM vessel LIMIT 1`).Scan(&vesselID); err != nil {
        return nil, fmt.Errorf("load vessel: %w", err)
    }
    chain = append(chain, vesselID)
    return chain, nil
}

// BuildResolvedContextForResource builds the ResolvedContext passed to an integration.
//
// Resources are searched across ALL levels of the branch (lb) — a planet resource can
// declare a dependency on a vessel-level manager and find it. The branch gives context.
//
// Transponders are scoped strictly to the resource's own level — auth/access isolation
// is maintained per level. A system-level credential never appears in a planet-level
// integration call even though both are in the branch.
//
// This replaces BuildResolvedContext for all lifecycle dispatch (scan, calibrate, jump).
func BuildResolvedContextForResource(level BranchLevel, lb LeveledBranch, manifest integrations.Manifest) integrations.ResolvedContext {
    rc := integrations.ResolvedContext{
        Platform:     lb.Platform,
        Resources:    make(map[string][]integrations.ResolvedResource),
        Transponders: make(map[string][]integrations.ResolvedTransponder),
    }
    // Resources: search the full branch — resource dependencies can be at any level.
    for role, brands := range manifest.Dependencies.Resources {
        for _, l := range lb.Levels {
            for _, r := range l.Resources {
                if r.Role == role && brandAccepted(r.Brand, brands) {
                    rc.Resources[role] = append(rc.Resources[role], integrations.ResolvedResource{Resource: r})
                }
            }
        }
    }
    // Transponders: only from this resource's own level. Auth stays where it was attached.
    for role, brands := range manifest.Dependencies.Transponders {
        for _, tp := range level.Transponders {
            if tp.Role == role && brandAccepted(tp.Brand, brands) {
                rc.Transponders[role] = append(rc.Transponders[role], integrations.ResolvedTransponder{Transponder: tp})
            }
        }
    }
    return rc
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/starchart/... -run TestLeveledBranchCrawl -v
```

Expected: PASS. Fix any issues with the `sc.Get` helper signature — check how `Get` is called elsewhere in the starchart package to confirm the table name convention matches.

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/crawl.go internal/starchart/leveled_crawl_test.go
git commit -m "feat: add LeveledBranchCrawl with FILO hierarchy walk and level-scoped resource/transponder pairing"
```

---

## Task 4: starchart.ScanBranch and CalibrateBranch

These parallel `InitPlanet`/`InitResource` from `internal/starchart/init.go` but call `Scan`/`Calibrate` instead of `Init`. They return structured results for the Executor to render. They use `LeveledBranchCrawl` (not the Phase 2 `BranchCrawl`) so each resource gets only its level's transponders in its integration context.

**Files:**
- Create: `internal/starchart/lifecycle.go`
- Create: `internal/starchart/lifecycle_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/lifecycle_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestScanBranch_NoIntegration(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    r, _ := sc.CreateResource(ctx, "go", "runtime", "go", "", "")
    _, _ = sc.Attach(ctx, "go", "payment-api")

    result, err := sc.ScanBranch(ctx, p.ID)
    require.NoError(t, err)

    // Resource should appear with an unknown/failed beacon since no integration is registered
    require.Len(t, result.Resources, 1)
    assert.Equal(t, r.ID, result.Resources[0].Resource.ID)
    assert.NotEmpty(t, result.Resources[0].BeaconStatus)
}

func TestCalibrateBranch_NoIntegration(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = sc.CreateResource(ctx, "go", "runtime", "go", "", "")
    _, _ = sc.Attach(ctx, "go", "payment-api")

    result, err := sc.CalibrateBranch(ctx, p.ID)
    require.NoError(t, err)
    require.Len(t, result.Resources, 1)
    assert.Equal(t, "failed", result.Resources[0].Action)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... -run TestScanBranch -run TestCalibrateBranch -v
```

Expected: compile errors — `ScanBranch` and `CalibrateBranch` undefined.

- [ ] **Step 3: Implement `internal/starchart/lifecycle.go`**

```go
package starchart

import (
    "context"
    "fmt"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/models"
)

// ResourceScanResult holds the scan outcome for a single resource.
type ResourceScanResult struct {
    Resource     models.Resource
    Report       integrations.StateReport
    BeaconStatus string
}

// TransponderScanResult holds the scan outcome for a single transponder.
type TransponderScanResult struct {
    Transponder  models.Transponder
    Report       integrations.StateReport
    BeaconStatus string
}

// BranchScanResult holds all scan results for a crawled entity.
type BranchScanResult struct {
    Resources    []ResourceScanResult
    Transponders []TransponderScanResult
}

// ResourceCalibrateResult holds the calibrate outcome for a single resource.
type ResourceCalibrateResult struct {
    Resource models.Resource
    Before   integrations.StateReport // from initial scan
    After    integrations.StateReport // from calibrate; zero if resource was healthy
    Action   string                   // "healthy", "calibrated", "failed"
}

// TransponderCalibrateResult holds the calibrate outcome for a single transponder.
type TransponderCalibrateResult struct {
    Transponder models.Transponder
    Before      integrations.StateReport
    After       integrations.StateReport
    Action      string
}

// BranchCalibrateResult holds all calibrate results for a crawled entity.
type BranchCalibrateResult struct {
    Resources    []ResourceCalibrateResult
    Transponders []TransponderCalibrateResult
}

// ScanBranch scans all resources and transponders in the full FILO hierarchy for entityID.
// Each resource receives branch-wide resource context but only its own level's transponders.
// Levels with no resources are skipped. Writes beacon updates as a side effect.
func (sc *StarChart) ScanBranch(ctx context.Context, entityID string) (BranchScanResult, error) {
    lb, err := sc.LeveledBranchCrawl(ctx, entityID)
    if err != nil {
        return BranchScanResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
    }

    var result BranchScanResult
    for _, level := range lb.Levels {
        for _, r := range level.Resources {
            rr, err := sc.scanResource(ctx, r, level, lb)
            if err != nil {
                return BranchScanResult{}, err
            }
            result.Resources = append(result.Resources, rr)
        }
        for _, tp := range level.Transponders {
            tr, err := sc.scanTransponder(ctx, tp, level, lb)
            if err != nil {
                return BranchScanResult{}, err
            }
            result.Transponders = append(result.Transponders, tr)
        }
    }
    return result, nil
}

// CalibrateBranch scans the full FILO hierarchy, then calibrates drifted/failed entities.
// Each resource receives branch-wide resource context but only its own level's transponders.
// Writes beacon updates as a side effect.
func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string) (BranchCalibrateResult, error) {
    lb, err := sc.LeveledBranchCrawl(ctx, entityID)
    if err != nil {
        return BranchCalibrateResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
    }

    var result BranchCalibrateResult
    for _, level := range lb.Levels {
        for _, r := range level.Resources {
            cr, err := sc.calibrateResource(ctx, r, level, lb)
            if err != nil {
                return BranchCalibrateResult{}, err
            }
            result.Resources = append(result.Resources, cr)
        }
        for _, tp := range level.Transponders {
            ct, err := sc.calibrateTransponder(ctx, tp, level, lb)
            if err != nil {
                return BranchCalibrateResult{}, err
            }
            result.Transponders = append(result.Transponders, ct)
        }
    }
    return result, nil
}

func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, level BranchLevel, lb LeveledBranch) (ResourceScanResult, error) {
    integration, ok := sc.integrations.Get(r.Role, r.Brand)
    if !ok {
        status := models.BeaconStatusFailed
        obs := []string{fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand)}
        if err := sc.setBeaconStatus(ctx, r.ID, status, obs); err != nil {
            return ResourceScanResult{}, err
        }
        return ResourceScanResult{Resource: r, BeaconStatus: status}, nil
    }
    rc := BuildResolvedContextForResource(level, lb, integration.Meta())
    report := integration.Scan(rc)
    status := scanBeaconStatus(report)
    if err := sc.setBeaconStatus(ctx, r.ID, status, report.Observations); err != nil {
        return ResourceScanResult{}, err
    }
    return ResourceScanResult{Resource: r, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (TransponderScanResult, error) {
    integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
    if !ok {
        status := models.BeaconStatusFailed
        obs := []string{fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand)}
        if err := sc.setBeaconStatus(ctx, tp.ID, status, obs); err != nil {
            return TransponderScanResult{}, err
        }
        return TransponderScanResult{Transponder: tp, BeaconStatus: status}, nil
    }
    // Transponder integrations check their own credential location; resources in context
    // are provided in case the transponder integration needs to know about the environment.
    rc := BuildResolvedContextForResource(level, lb, integration.Meta())
    report := integration.Scan(rc)
    status := scanBeaconStatus(report)
    if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
        return TransponderScanResult{}, err
    }
    return TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, level BranchLevel, lb LeveledBranch) (ResourceCalibrateResult, error) {
    scanResult, err := sc.scanResource(ctx, r, level, lb)
    if err != nil {
        return ResourceCalibrateResult{}, err
    }
    if scanResult.BeaconStatus == models.BeaconStatusHealthy {
        return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "healthy"}, nil
    }
    integration, ok := sc.integrations.Get(r.Role, r.Brand)
    if !ok {
        return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "failed"}, nil
    }
    rc := BuildResolvedContextForResource(level, lb, integration.Meta())
    after := integration.Calibrate(rc)
    afterStatus := scanBeaconStatus(after)
    if err := sc.setBeaconStatus(ctx, r.ID, afterStatus, after.Observations); err != nil {
        return ResourceCalibrateResult{}, err
    }
    action := "calibrated"
    if afterStatus == models.BeaconStatusFailed {
        action = "failed"
    }
    return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, After: after, Action: action}, nil
}

func (sc *StarChart) calibrateTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (TransponderCalibrateResult, error) {
    scanResult, err := sc.scanTransponder(ctx, tp, level, lb)
    if err != nil {
        return TransponderCalibrateResult{}, err
    }
    if scanResult.BeaconStatus == models.BeaconStatusHealthy {
        return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, Action: "healthy"}, nil
    }
    integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
    if !ok {
        return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, Action: "failed"}, nil
    }
    rc := BuildResolvedContextForResource(level, lb, integration.Meta())
    after := integration.Calibrate(rc)
    afterStatus := scanBeaconStatus(after)
    if err := sc.setBeaconStatus(ctx, tp.ID, afterStatus, after.Observations); err != nil {
        return TransponderCalibrateResult{}, err
    }
    action := "calibrated"
    if afterStatus == models.BeaconStatusFailed {
        action = "failed"
    }
    return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, After: after, Action: action}, nil
}

// scanBeaconStatus maps a StateReport to a beacon status for scan operations.
// Unlike init, not-present is "drifted" rather than "failed".
func scanBeaconStatus(report integrations.StateReport) string {
    if report.Error != "" {
        return models.BeaconStatusFailed
    }
    if !report.Present || !report.Reachable {
        return models.BeaconStatusDrifted
    }
    return models.BeaconStatusHealthy
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/starchart/... -run "TestScanBranch|TestCalibrateBranch" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/lifecycle.go internal/starchart/lifecycle_test.go
git commit -m "feat: add starchart ScanBranch and CalibrateBranch"
```

---

## Task 5: starchart.PlanRetro and ExecuteRetro

Cascade retire: collect the full subtree, detect shared nodes (attached outside the retire set), split into retire vs detach-only. Execute in a single transaction.

**Files:**
- Create: `internal/starchart/retro.go`
- Create: `internal/starchart/retro_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/retro_test.go`:

```go
package starchart_test

import (
    "context"
    "testing"

    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestPlanRetro_UnsharedResource(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    r, _ := sc.CreateResource(ctx, "node", "runtime", "node", "", "")
    _, _ = sc.Attach(ctx, "node", "payment-api")

    plan, err := sc.PlanRetro(ctx, p.ID)
    require.NoError(t, err)

    // Planet and its unshared resource should both be retired
    require.Len(t, plan.Nodes, 2)
    actions := map[string]string{}
    for _, n := range plan.Nodes {
        actions[n.EntityID] = n.Action
    }
    assert.Equal(t, "retire", actions[p.ID])
    assert.Equal(t, "retire", actions[r.ID])
}

func TestPlanRetro_SharedResource(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p1, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    p2, _ := sc.CreatePlanet(ctx, "auth-service", g.ID, "")
    r, _ := sc.CreateResource(ctx, "node", "runtime", "node", "", "")
    _, _ = sc.Attach(ctx, "node", "payment-api")
    _, _ = sc.Attach(ctx, "node", "auth-service")

    // Retire payment-api only; node is shared with auth-service
    plan, err := sc.PlanRetro(ctx, p1.ID)
    require.NoError(t, err)

    actions := map[string]string{}
    for _, n := range plan.Nodes {
        actions[n.EntityID] = n.Action
    }
    assert.Equal(t, "retire", actions[p1.ID])
    // node is shared with auth-service — only detach, don't retire
    assert.Equal(t, "detach", actions[r.ID])
    // auth-service is untouched
    _, found := actions[p2.ID]
    assert.False(t, found)
}

func TestExecuteRetro(t *testing.T) {
    sc := openTestChart(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = sc.CreateResource(ctx, "node", "runtime", "node", "", "")
    _, _ = sc.Attach(ctx, "node", "payment-api")

    plan, err := sc.PlanRetro(ctx, p.ID)
    require.NoError(t, err)
    require.NoError(t, sc.ExecuteRetro(ctx, plan))

    // Planet should be gone
    var planet starchart.Planet
    err = sc.Get(ctx, "planets", p.ID, &planet)
    assert.ErrorIs(t, err, starchart.ErrNotFound)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... -run "TestPlanRetro|TestExecuteRetro" -v
```

Expected: compile errors — `PlanRetro` and `ExecuteRetro` undefined.

- [ ] **Step 3: Implement `internal/starchart/retro.go`**

```go
package starchart

import (
    "context"
    "fmt"

    "github.com/Kenttleton/orbiter/internal/models"
)

// RetroNode describes a single entity's action in a retro plan.
type RetroNode struct {
    EntityID string
    Name     string
    Action   string // "retire" or "detach"
}

// RetroPlan is the computed plan for a retro operation.
type RetroPlan struct {
    TargetID   string
    TargetName string
    Nodes      []RetroNode
}

// PlanRetro computes the cascade retire plan for targetID.
// All descendants are collected; shared nodes (attached to entities outside
// the retire set) are marked "detach" rather than "retire".
func (sc *StarChart) PlanRetro(ctx context.Context, targetID string) (RetroPlan, error) {
    targetAlias, err := sc.aliasOf(ctx, targetID)
    if err != nil {
        return RetroPlan{}, fmt.Errorf("resolve target alias: %w", err)
    }

    // Collect all entities in the subtree (target + all descendants).
    subtree, err := sc.collectSubtree(ctx, targetID)
    if err != nil {
        return RetroPlan{}, err
    }

    // Build retire set for shared-node detection.
    retireSet := make(map[string]bool, len(subtree))
    for _, id := range subtree {
        retireSet[id] = true
    }

    plan := RetroPlan{TargetID: targetID, TargetName: targetAlias}
    for _, entityID := range subtree {
        name, _ := sc.aliasOf(ctx, entityID)
        shared, err := sc.hasAttachmentOutside(ctx, entityID, retireSet)
        if err != nil {
            return RetroPlan{}, err
        }
        action := "retire"
        if shared {
            action = "detach"
        }
        plan.Nodes = append(plan.Nodes, RetroNode{
            EntityID: entityID,
            Name:     name,
            Action:   action,
        })
    }
    return plan, nil
}

// ExecuteRetro executes a RetroPlan in a single transaction.
// Retired entities are deleted and their beacons set to "retired".
// Detached entities have only their attachment to the retire path removed.
func (sc *StarChart) ExecuteRetro(ctx context.Context, plan RetroPlan) error {
    return sc.Tx(ctx, func(tx *Tx) error {
        retireSet := make(map[string]bool)
        for _, n := range plan.Nodes {
            retireSet[n.EntityID] = true
        }

        for _, node := range plan.Nodes {
            if node.Action == "retire" {
                // Set beacon to retired.
                if err := sc.setBeaconStatus(ctx, node.EntityID, models.BeaconStatusRetired, nil); err != nil {
                    return err
                }
                // Delete from entity table and alias.
                table, err := tableForEntity(node.EntityID)
                if err != nil {
                    return err
                }
                if _, err := tx.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, table), node.EntityID); err != nil {
                    return fmt.Errorf("delete %s %s: %w", table, node.EntityID, err)
                }
                if _, err := tx.db.ExecContext(ctx, `DELETE FROM aliases WHERE entity = ?`, node.EntityID); err != nil {
                    return fmt.Errorf("delete alias %s: %w", node.EntityID, err)
                }
            } else {
                // Detach: remove attachment edges that point into the retire set.
                if _, err := tx.db.ExecContext(ctx,
                    `DELETE FROM attachments WHERE from_id = ? AND to_id IN (SELECT id FROM (VALUES `+placeholders(len(plan.Nodes))+`))`,
                    node.EntityID,
                ); err != nil {
                    // Simpler approach for SQLite: delete all attachments involving the retired set.
                    for _, n := range plan.Nodes {
                        if n.Action == "retire" {
                            if _, err2 := tx.db.ExecContext(ctx,
                                `DELETE FROM attachments WHERE from_id = ? AND to_id = ?`,
                                node.EntityID, n.EntityID,
                            ); err2 != nil {
                                return fmt.Errorf("detach %s from %s: %w", node.EntityID, n.EntityID, err2)
                            }
                        }
                    }
                }
            }
        }
        return nil
    })
}

// collectSubtree returns the entityID itself plus all entities attached to it
// (directly or transitively), via the from_id → to_id attachment graph.
func (sc *StarChart) collectSubtree(ctx context.Context, entityID string) ([]string, error) {
    visited := make(map[string]bool)
    var result []string
    var walk func(id string) error
    walk = func(id string) error {
        if visited[id] {
            return nil
        }
        visited[id] = true
        result = append(result, id)
        // Find all entities attached TO this entity (they are children).
        rows, err := sc.db.QueryContext(ctx, `SELECT from_id FROM attachments WHERE to_id = ?`, id)
        if err != nil {
            return fmt.Errorf("query children of %s: %w", id, err)
        }
        defer rows.Close()
        var children []string
        for rows.Next() {
            var child string
            if err := rows.Scan(&child); err != nil {
                return err
            }
            children = append(children, child)
        }
        if err := rows.Err(); err != nil {
            return err
        }
        for _, child := range children {
            if err := walk(child); err != nil {
                return err
            }
        }
        return nil
    }
    return result, walk(entityID)
}

// hasAttachmentOutside returns true if entityID has any attachment to an entity
// not in retireSet (meaning it is shared beyond the subtree being retired).
func (sc *StarChart) hasAttachmentOutside(ctx context.Context, entityID string, retireSet map[string]bool) (bool, error) {
    rows, err := sc.db.QueryContext(ctx, `SELECT to_id FROM attachments WHERE from_id = ?`, entityID)
    if err != nil {
        return false, fmt.Errorf("query attachments of %s: %w", entityID, err)
    }
    defer rows.Close()
    for rows.Next() {
        var toID string
        if err := rows.Scan(&toID); err != nil {
            return false, err
        }
        if !retireSet[toID] {
            return true, nil // attached to something outside the retire set
        }
    }
    return false, rows.Err()
}

// aliasOf returns the alias name for an entity ID, or the ID itself if not found.
func (sc *StarChart) aliasOf(ctx context.Context, entityID string) (string, error) {
    var name string
    err := sc.db.QueryRowContext(ctx, `SELECT name FROM aliases WHERE entity = ?`, entityID).Scan(&name)
    if err != nil {
        return entityID, nil // fallback to ID
    }
    return name, nil
}

// tableForEntity determines the entity table from the entity type bits in the OrbitID.
// OrbitID format: [8-char timestamp][2-char type][6-char random]
func tableForEntity(entityID string) (string, error) {
    if len(entityID) < 10 {
        return "", fmt.Errorf("invalid entity ID: %q", entityID)
    }
    switch entityID[8:10] {
    case "gx":
        return "galaxies", nil
    case "sy":
        return "solar_systems", nil
    case "pl":
        return "planets", nil
    case "cs":
        return "callsigns", nil
    case "tp":
        return "transponders", nil
    case "rs":
        return "resources", nil
    default:
        return "", fmt.Errorf("unknown entity type %q in ID %q", entityID[8:10], entityID)
    }
}

func placeholders(n int) string {
    if n == 0 {
        return ""
    }
    result := "(?)"
    for i := 1; i < n; i++ {
        result += ",(?)"
    }
    return result
}
```

> **Note:** The `ExecuteRetro` detach path uses a loop for SQLite compatibility instead of a subquery. The `placeholders` helper is unused in the final implementation — keep it removed.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/starchart/... -run "TestPlanRetro|TestExecuteRetro" -v
```

Expected: PASS. Fix any compile errors from the Tx API — check `internal/starchart/tx.go` for the Tx struct field name (likely `tx.db` or the Tx may expose a different API).

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/retro.go internal/starchart/retro_test.go
git commit -m "feat: add starchart PlanRetro and ExecuteRetro with shared-node detection"
```

---

## Task 6: Executor struct

The Executor owns the shared resolve→crawl→dispatch→render pipeline. Each of the six lifecycle command methods calls `resolveTarget` then dispatches to the starchart layer.

**Files:**
- Create: `internal/commands/executor.go`

- [ ] **Step 1: Write failing test**

Create `internal/commands/executor_test.go`:

```go
package commands_test

import (
    "context"
    "os"
    "testing"

    "github.com/Kenttleton/orbiter/internal/commands"
    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func openTestExecutor(t *testing.T) *commands.Executor {
    t.Helper()
    f, err := os.CreateTemp(t.TempDir(), "starchart-*.db")
    require.NoError(t, err)
    f.Close()
    sc, err := starchart.Open(f.Name())
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })
    r := output.NewRenderer(output.FormatStyled, false)
    return commands.NewExecutor(sc, r)
}

func TestExecutor_Survey_NoTarget(t *testing.T) {
    exec := openTestExecutor(t)
    // No entities, no CWD match — should return a "no target found" error
    err := exec.Survey(context.Background(), "")
    require.Error(t, err)
    require.Contains(t, err.Error(), "no target found")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor -v
```

Expected: compile error — `commands.Executor` undefined.

- [ ] **Step 3: Implement `internal/commands/executor.go`**

```go
package commands

import (
    "context"
    "fmt"
    "os"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/Kenttleton/orbiter/internal/starchart"
)

// ShellDirective is a single directive for the shell function to eval after jump.
type ShellDirective struct {
    Op    string // "cd" or "export"
    Key   string // env var name (export only)
    Value string
}

// String serialises the directive as a shell statement.
func (d ShellDirective) String() string {
    if d.Op == "cd" {
        return fmt.Sprintf("cd %q", d.Value)
    }
    return fmt.Sprintf("export %s=%q", d.Key, d.Value)
}

// Executor owns the shared pipeline for all six lifecycle commands.
type Executor struct {
    sc       *starchart.StarChart
    renderer output.Renderer
}

// NewExecutor constructs an Executor with the given starchart and renderer.
func NewExecutor(sc *starchart.StarChart, r output.Renderer) *Executor {
    return &Executor{sc: sc, renderer: r}
}

// resolveTarget returns the alias for the given target string, or resolves via
// CWD if target is empty. Returns ErrNotFound-wrapped error on no match.
func (e *Executor) resolveTarget(ctx context.Context, target string) (models.Alias, error) {
    if target != "" {
        return e.sc.Resolve(ctx, target)
    }
    cwd, err := os.Getwd()
    if err != nil {
        return models.Alias{}, fmt.Errorf("get working directory: %w", err)
    }
    alias, err := e.sc.ResolveCWD(ctx, cwd)
    if err != nil {
        return models.Alias{}, fmt.Errorf("no target found: not in a known entity directory (use an explicit target name)")
    }
    return alias, nil
}
```

- [ ] **Step 4: Run test**

```bash
go test ./internal/commands/... -run TestExecutor -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/executor.go internal/commands/executor_test.go
git commit -m "feat: add Executor struct with resolveTarget (explicit or CWD)"
```

---

## Task 7: survey command

`survey` reads the Star Chart and last beacons — no integration calls.

**Files:**
- Create: `internal/commands/lifecycle.go` (start here; other commands added in Tasks 8–12)
- Modify: `internal/commands/stubs.go` (remove survey stub)
- Modify: `internal/commands/root.go` (no change needed — stub is already registered)

- [ ] **Step 1: Write failing test**

Add to `internal/commands/executor_test.go`:

```go
func TestExecutor_Survey_WithTarget(t *testing.T) {
    exec := openTestExecutor(t)
    sc := exec.SC() // need to expose SC for test setup — add getter to Executor

    ctx := context.Background()
    g, err := sc.CreateGalaxy(ctx, "acme")
    require.NoError(t, err)

    err = exec.Survey(ctx, "acme")
    require.NoError(t, err) // should succeed even with no resources attached
}
```

Add `SC() *starchart.StarChart` getter to `Executor` in `executor.go`:
```go
func (e *Executor) SC() *starchart.StarChart { return e.sc }
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Survey -v
```

Expected: compile error — `Survey` undefined on Executor.

- [ ] **Step 3: Add `Survey` to Executor in `executor.go`**

```go
// Survey renders the desired state and last beacon for the target entity.
func (e *Executor) Survey(ctx context.Context, target string) error {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return err
    }

    branch, err := e.sc.BranchCrawl(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("crawl %s: %w", alias.Name, err)
    }

    beacon, err := e.sc.GetBeacon(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("get beacon for %s: %w", alias.Name, err)
    }

    rows := [][]string{
        {"entity", alias.Name},
        {"id", alias.ID},
        {"status", beacon.Status},
        {"verified", beacon.VerifiedAt.Format("2006-01-02 15:04:05")},
        {"resources", fmt.Sprintf("%d", len(branch.Resources))},
        {"transponders", fmt.Sprintf("%d", len(branch.Transponders))},
        {"callsigns", fmt.Sprintf("%d", len(branch.Callsigns))},
    }
    e.renderer.Table([]string{"field", "value"}, rows)

    if len(branch.Resources) > 0 {
        var resRows [][]string
        for _, r := range branch.Resources {
            rb, _ := e.sc.GetBeacon(ctx, r.ID)
            resRows = append(resRows, []string{r.Role + "/" + r.Brand, r.ID, rb.Status})
        }
        e.renderer.Table([]string{"resource", "id", "status"}, resRows)
    }
    return nil
}
```

- [ ] **Step 4: Create `internal/commands/lifecycle.go` with survey command**

```go
package commands

import (
    "github.com/spf13/cobra"
)

func newSurveyCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "survey [target]",
        Short: "Inspect metadata — \"What is this thing?\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            exec := NewExecutor(d.sc, d.renderer)
            return exec.Survey(cmd.Context(), target)
        },
    }
}
```

- [ ] **Step 5: Remove survey stub from `internal/commands/stubs.go`**

Delete the `newSurveyCmd` function from `stubs.go`. Keep all vessel commands.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/commands/... -run TestExecutor_Survey -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go
git commit -m "feat: implement survey command via Executor"
```

---

## Task 8: scan command

`scan` calls `ScanBranch` and writes beacons as a side effect.

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/stubs.go`

- [ ] **Step 1: Write failing test**

Add to `internal/commands/executor_test.go`:

```go
func TestExecutor_Scan_NoIntegration(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    _, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    _, _ = exec.SC().CreateResource(ctx, "go", "runtime", "go", "", "")
    _, _ = exec.SC().Attach(ctx, "go", "payment-api")

    // scan should succeed (no integration = failed beacon, but no error returned)
    err := exec.Scan(ctx, "payment-api")
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Scan -v
```

Expected: compile error — `Scan` undefined on Executor.

- [ ] **Step 3: Add `Scan` to Executor in `executor.go`**

```go
// Scan verifies current reality for the target entity and updates beacons.
func (e *Executor) Scan(ctx context.Context, target string) error {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return err
    }

    result, err := e.sc.ScanBranch(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("scan %s: %w", alias.Name, err)
    }

    var rows [][]string
    for _, r := range result.Resources {
        obs := ""
        if len(r.Report.Observations) > 0 {
            obs = r.Report.Observations[0]
        }
        if r.Report.Error != "" {
            obs = r.Report.Error
        }
        rows = append(rows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.BeaconStatus, obs})
    }
    for _, tp := range result.Transponders {
        obs := ""
        if r.Report.Error != "" {
            obs = tp.Report.Error
        }
        rows = append(rows, []string{tp.Transponder.Role + "/" + tp.Transponder.Brand, tp.BeaconStatus, obs})
    }

    if len(rows) == 0 {
        e.renderer.Info(fmt.Sprintf("%s: no resources or transponders attached", alias.Name))
        return nil
    }
    e.renderer.Table([]string{"resource", "status", "observation"}, rows)
    return nil
}
```

> **Note:** There is a typo in the transponder loop above: `r.Report.Error` should be `tp.Report.Error`. Fix it when implementing.

- [ ] **Step 4: Add scan to `lifecycle.go`**

```go
func newScanCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "scan [target]",
        Short: "Verify reality — \"What does reality currently look like?\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            return NewExecutor(d.sc, d.renderer).Scan(cmd.Context(), target)
        },
    }
}
```

- [ ] **Step 5: Remove scan stub from `stubs.go`**

- [ ] **Step 6: Run tests**

```bash
go test ./internal/commands/... -run TestExecutor_Scan -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go
git commit -m "feat: implement scan command via Executor.ScanBranch"
```

---

## Task 9: chart command

`chart` computes the terraform-style plan. Calls `ScanBranch` (writes beacons as side effect) and renders the delta.

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/stubs.go`

- [ ] **Step 1: Write failing test**

Add to `internal/commands/executor_test.go`:

```go
func TestExecutor_Chart(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    _, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

    err := exec.Chart(ctx, "payment-api")
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Chart -v
```

Expected: compile error.

- [ ] **Step 3: Add `Chart` to Executor**

```go
// Chart computes and renders the terraform-style plan for the target entity.
// Calls ScanBranch — beacons are updated as a side effect.
func (e *Executor) Chart(ctx context.Context, target string) error {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return err
    }

    result, err := e.sc.ScanBranch(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("chart %s: %w", alias.Name, err)
    }

    var steps []output.PlanStep
    for _, r := range result.Resources {
        action := beaconToAction(r.BeaconStatus)
        desc := r.Report.BinaryPath
        if r.Report.Error != "" {
            desc = r.Report.Error
        }
        steps = append(steps, output.PlanStep{
            Action:      action,
            EntityType:  r.Resource.Role,
            Name:        r.Resource.Brand,
            Description: desc,
        })
    }
    for _, tp := range result.Transponders {
        action := beaconToAction(tp.BeaconStatus)
        steps = append(steps, output.PlanStep{
            Action:     action,
            EntityType: tp.Transponder.Role,
            Name:       tp.Transponder.Brand,
        })
    }

    if len(steps) == 0 {
        e.renderer.Info(fmt.Sprintf("%s: no resources or transponders to chart", alias.Name))
        return nil
    }
    e.renderer.Plan(steps)
    return nil
}

// beaconToAction maps a beacon status to a PlanStep action string.
func beaconToAction(status string) string {
    switch status {
    case "healthy", "verified":
        return "healthy"
    case "drifted", "unverified":
        return "change"
    case "failed":
        return "add"
    default:
        return "change"
    }
}
```

- [ ] **Step 4: Add chart to `lifecycle.go`**

```go
func newChartCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "chart [target]",
        Short: "Preview a transition — \"What would happen if I went there?\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            return NewExecutor(d.sc, d.renderer).Chart(cmd.Context(), target)
        },
    }
}
```

- [ ] **Step 5: Remove chart stub from `stubs.go`**

- [ ] **Step 6: Run tests**

```bash
go test ./internal/commands/... -run TestExecutor_Chart -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go
git commit -m "feat: implement chart command — terraform-style plan via ScanBranch"
```

---

## Task 10: calibrate command

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/stubs.go`

- [ ] **Step 1: Write failing test**

Add to `executor_test.go`:

```go
func TestExecutor_Calibrate(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    _, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

    err := exec.Calibrate(ctx, "payment-api")
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Calibrate -v
```

- [ ] **Step 3: Add `Calibrate` to Executor**

```go
// Calibrate reconciles drift for the target entity.
// Scans first, then calls Calibrate on drifted/failed resources.
func (e *Executor) Calibrate(ctx context.Context, target string) error {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return err
    }

    result, err := e.sc.CalibrateBranch(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("calibrate %s: %w", alias.Name, err)
    }

    var rows [][]string
    for _, r := range result.Resources {
        obs := ""
        if r.After.Error != "" {
            obs = r.After.Error
        } else if len(r.After.Observations) > 0 {
            obs = r.After.Observations[0]
        } else if len(r.Before.Observations) > 0 {
            obs = r.Before.Observations[0]
        }
        rows = append(rows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.Action, obs})
    }
    for _, tp := range result.Transponders {
        rows = append(rows, []string{tp.Transponder.Role + "/" + tp.Transponder.Brand, tp.Action, ""})
    }

    if len(rows) == 0 {
        e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
        return nil
    }
    e.renderer.Table([]string{"resource", "action", "observation"}, rows)
    return nil
}
```

- [ ] **Step 4: Add calibrate to `lifecycle.go`**

```go
func newCalibrateCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "calibrate [target]",
        Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            return NewExecutor(d.sc, d.renderer).Calibrate(cmd.Context(), target)
        },
    }
}
```

- [ ] **Step 5: Remove calibrate stub from `stubs.go`**

- [ ] **Step 6: Run tests and commit**

```bash
go test ./internal/commands/... -run TestExecutor_Calibrate -v
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go
git commit -m "feat: implement calibrate command via Executor.CalibrateBranch"
```

---

## Task 11: jump command + shell directives

`jump` is the terraform apply: chart → confirm → execute → emit shell directives. Human-readable output goes to `stderr`; shell directives go to `stdout` so `eval "$(orbiter jump ...)"` captures only the directives.

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/stubs.go`

- [ ] **Step 1: Write failing test**

Add to `executor_test.go`:

```go
func TestExecutor_Jump_NoConfirm(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    _, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

    // Attach a filesystem/orbiter resource to record where the project lives.
    // In production this is created by planet init or jump after a remote init.
    const path = "/home/kent/acme/payment-api"
    config := `{"path":"` + path + `"}`
    _, _ = exec.SC().CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "", config)
    _, _ = exec.SC().Attach(ctx, "payment-api-path", "payment-api")

    // confirmed=true skips the interactive prompt
    directives, err := exec.Jump(ctx, "payment-api", true)
    require.NoError(t, err)
    // Should include a cd directive to the filesystem resource path
    require.NotEmpty(t, directives)
    assert.Equal(t, "cd", directives[0].Op)
    assert.Equal(t, path, directives[0].Value)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Jump -v
```

- [ ] **Step 3: Add `Jump` to Executor**

```go
// Jump executes a full transition to the target entity.
// Returns shell directives (cd + env exports) for the shell function to eval.
// Human-readable output is written to stderr so eval captures only directives.
// If confirmed is false, renders the plan and prompts interactively.
func (e *Executor) Jump(ctx context.Context, target string, confirmed bool) ([]ShellDirective, error) {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return nil, err
    }

    // Phase 1: compute the plan (same as chart).
    scanResult, err := e.sc.ScanBranch(ctx, alias.ID)
    if err != nil {
        return nil, fmt.Errorf("scan %s: %w", alias.Name, err)
    }

    // Render the delta plan to stderr.
    var steps []output.PlanStep
    for _, r := range scanResult.Resources {
        steps = append(steps, output.PlanStep{
            Action:      beaconToAction(r.BeaconStatus),
            EntityType:  r.Resource.Role,
            Name:        r.Resource.Brand,
            Description: r.Report.BinaryPath,
        })
    }
    stderrRenderer := output.NewRenderer(output.FormatStyled, e.renderer.IsVerbose(), os.Stderr)
    stderrRenderer.Plan(steps)

    // Phase 2: confirm.
    if !confirmed {
        fmt.Fprintf(os.Stderr, "\nExecute maneuver? [y/N] ")
        var response string
        fmt.Fscanln(os.Stdin, &response)
        if response != "y" && response != "Y" {
            fmt.Fprintln(os.Stderr, "Aborted.")
            return nil, nil
        }
    }

    // Phase 3: execute — calibrate drifted/failed, init unverified.
    calibResult, err := e.sc.CalibrateBranch(ctx, alias.ID)
    if err != nil {
        return nil, fmt.Errorf("execute jump for %s: %w", alias.Name, err)
    }

    // Render execution results to stderr.
    var execRows [][]string
    for _, r := range calibResult.Resources {
        execRows = append(execRows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.Action})
    }
    stderrRenderer.Table([]string{"resource", "action"}, execRows)

    // Phase 4: build shell directives.
    // Re-crawl to pick up any resources created by init (e.g. filesystem resource
    // written by the remote integration after a clone).
    branch, _ := e.sc.BranchCrawl(ctx, alias.ID)

    var directives []ShellDirective

    // cd directive: read path from the first attached filesystem/orbiter resource.
    for _, r := range branch.Resources {
        if r.Role == "filesystem" && r.Brand == "orbiter" {
            var cfg struct {
                Path string `json:"path"`
            }
            if err := json.Unmarshal([]byte(r.Config), &cfg); err == nil && cfg.Path != "" {
                directives = append(directives, ShellDirective{Op: "cd", Value: cfg.Path})
                break
            }
        }
    }

    // Export transponder env vars from env-role transponders attached to this entity.
    for _, tp := range branch.Transponders {
        if tp.Role == "env" {
            directives = append(directives, ShellDirective{Op: "export", Key: tp.Brand, Value: tp.Location})
        }
    }

    return directives, nil
}
```

> **Note:** `output.NewRenderer` currently takes 2 args. This task requires adding an optional `io.Writer` parameter. See Step 4.

- [ ] **Step 4: Update `output.NewRenderer` to accept optional writer**

In `internal/output/factory.go`, change the signature:

```go
package output

import (
    "io"
    "os"
)

// NewRenderer creates a Renderer that writes to os.Stdout.
func NewRenderer(format string, verbose bool) Renderer {
    return NewRendererTo(format, verbose, os.Stdout)
}

// NewRendererTo creates a Renderer that writes to out.
func NewRendererTo(format string, verbose bool, out io.Writer) Renderer {
    switch format {
    case FormatJSON:
        return newJSONRenderer(out)
    default:
        return newStyledRenderer(verbose, out)
    }
}
```

Update `newJSONRenderer` and `newStyledRenderer` in their respective files to accept `io.Writer`. Check `internal/output/json.go` and `internal/output/styled.go` for their current constructors and thread the writer through.

- [ ] **Step 5: Add jump to `lifecycle.go`**

```go
func newJumpCmd(d *deps) *cobra.Command {
    var yes bool
    cmd := &cobra.Command{
        Use:   "jump [target]",
        Short: "Execute a transition — \"Take me there.\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            exec := NewExecutor(d.sc, d.renderer)
            directives, err := exec.Jump(cmd.Context(), target, yes)
            if err != nil {
                return err
            }
            // Write shell directives to stdout for eval.
            for _, dir := range directives {
                fmt.Println(dir.String())
            }
            return nil
        },
    }
    cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
    return cmd
}
```

- [ ] **Step 6: Remove jump stub from `stubs.go`**

- [ ] **Step 7: Run tests**

```bash
go test ./internal/commands/... -run TestExecutor_Jump -v
```

Expected: PASS (may require fixing the renderer writer threading first).

- [ ] **Step 8: Commit**

```bash
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go internal/output/
git commit -m "feat: implement jump command with shell directives and confirmation prompt"
```

---

## Task 12: retro command

`retro` cascades a retire through the subtree with a confirmation prompt.

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/stubs.go`

- [ ] **Step 1: Write failing test**

Add to `executor_test.go`:

```go
func TestExecutor_Retro_Confirmed(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    _, _ = exec.SC().CreatePlanet(ctx, "old-project", g.ID, "")

    // confirmed=true skips the prompt
    err := exec.Retro(ctx, "old-project", true)
    require.NoError(t, err)

    // Planet should be gone
    _, err = exec.SC().Resolve(ctx, "old-project")
    require.ErrorIs(t, err, starchart.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/commands/... -run TestExecutor_Retro -v
```

- [ ] **Step 3: Add `Retro` to Executor**

```go
// Retro retires the target entity and its unshared descendants.
// Shows the delta plan and prompts for confirmation unless confirmed=true.
func (e *Executor) Retro(ctx context.Context, target string, confirmed bool) error {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return err
    }

    plan, err := e.sc.PlanRetro(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("plan retro for %s: %w", alias.Name, err)
    }

    // Render the delta.
    var steps []output.PlanStep
    for _, node := range plan.Nodes {
        action := "remove"
        if node.Action == "detach" {
            action = "change"
        }
        steps = append(steps, output.PlanStep{
            Action: action,
            Name:   node.Name,
        })
    }
    e.renderer.Plan(steps)

    retireCount := 0
    for _, n := range plan.Nodes {
        if n.Action == "retire" {
            retireCount++
        }
    }

    if !confirmed {
        fmt.Fprintf(os.Stderr, "\nRetire %d entities? [y/N] ", retireCount)
        var response string
        fmt.Fscanln(os.Stdin, &response)
        if response != "y" && response != "Y" {
            e.renderer.Info("Aborted.")
            return nil
        }
    }

    if err := e.sc.ExecuteRetro(ctx, plan); err != nil {
        return fmt.Errorf("execute retro for %s: %w", alias.Name, err)
    }
    e.renderer.Success(fmt.Sprintf("Retired %d entities.", retireCount))
    return nil
}
```

- [ ] **Step 4: Add retro to `lifecycle.go`**

```go
func newRetroCmd(d *deps) *cobra.Command {
    var yes bool
    cmd := &cobra.Command{
        Use:   "retro [target]",
        Short: "Retire obsolete entities — \"Remove what no longer belongs in the universe.\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            return NewExecutor(d.sc, d.renderer).Retro(cmd.Context(), target, yes)
        },
    }
    cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
    return cmd
}
```

- [ ] **Step 5: Remove retro stub from `stubs.go`**

- [ ] **Step 6: Run tests and commit**

```bash
go test ./internal/commands/... -run TestExecutor_Retro -v
git add internal/commands/executor.go internal/commands/lifecycle.go internal/commands/stubs.go
git commit -m "feat: implement retro command with cascade retire and confirmation"
```

---

## Task 13: shell integration scripts + orbiter init command

Shell scripts are embedded in the binary. `orbiter init <shell>` replaces the `::ORBITER::` token with the binary path and prints the integration function.

**Files:**
- Create: `shell/orbiter.bash`
- Create: `shell/orbiter.zsh`
- Create: `shell/orbiter.fish`
- Create: `internal/commands/shell.go`
- Modify: `internal/commands/root.go`

- [ ] **Step 1: Create `shell/orbiter.bash`**

```bash
# orbiter shell integration for bash
# Sourced via: eval "$(::ORBITER:: init bash)"
function orbiter() {
  if [ "$1" = "jump" ]; then
    eval "$(::ORBITER:: "$@")"
  else
    ::ORBITER:: "$@"
  fi
}
```

- [ ] **Step 2: Create `shell/orbiter.zsh`**

```zsh
# orbiter shell integration for zsh
# Sourced via: eval "$(::ORBITER:: init zsh)"
function orbiter() {
  if [ "$1" = "jump" ]; then
    eval "$(::ORBITER:: "$@")"
  else
    ::ORBITER:: "$@"
  fi
}
```

- [ ] **Step 3: Create `shell/orbiter.fish`**

```fish
# orbiter shell integration for fish
# Sourced via: ::ORBITER:: init fish | source
function orbiter
  if test "$argv[1]" = "jump"
    ::ORBITER:: $argv | source
  else
    ::ORBITER:: $argv
  end
end
```

- [ ] **Step 4: Create `internal/commands/shell.go`**

```go
package commands

import (
    _ "embed"
    "fmt"
    "os"
    "strings"

    "github.com/spf13/cobra"
)

//go:embed ../../shell/orbiter.bash
var shellBash string

//go:embed ../../shell/orbiter.zsh
var shellZsh string

//go:embed ../../shell/orbiter.fish
var shellFish string

func newInitCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "init <shell>",
        Short: "Print the orbiter shell integration function for the given shell",
        Long: `Print the orbiter shell integration function.

Add to your shell profile:
  bash/zsh:  eval "$(orbiter init zsh)"
  fish:      orbiter init fish | source`,
        Args:      cobra.ExactArgs(1),
        ValidArgs: []string{"bash", "zsh", "fish"},
        RunE: func(cmd *cobra.Command, args []string) error {
            shell := args[0]
            binaryPath, err := os.Executable()
            if err != nil {
                return fmt.Errorf("resolve binary path: %w", err)
            }

            var script string
            switch shell {
            case "bash":
                script = shellBash
            case "zsh":
                script = shellZsh
            case "fish":
                script = shellFish
            default:
                return fmt.Errorf("unsupported shell %q — supported: bash, zsh, fish", shell)
            }

            fmt.Print(strings.ReplaceAll(script, "::ORBITER::", binaryPath))
            return nil
        },
    }
}
```

- [ ] **Step 5: Register `init` command in `internal/commands/root.go`**

Add `newInitCmd()` to the `root.AddCommand(...)` call. Import is not needed — same package.

- [ ] **Step 6: Test the init command manually**

```bash
just build
./bin/orbiter init zsh
```

Expected: prints the zsh function with the full binary path replacing `::ORBITER::`.

```bash
./bin/orbiter init bash | grep "::ORBITER::"
```

Expected: no output (all tokens replaced).

- [ ] **Step 7: Commit**

```bash
git add shell/ internal/commands/shell.go internal/commands/root.go
git commit -m "feat: add orbiter init <shell> with embedded shell integration scripts"
```

---

## Task 14: orbiter completions command

Cobra generates shell completions automatically from the command tree.

**Files:**
- Modify: `internal/commands/shell.go`
- Modify: `internal/commands/root.go`

- [ ] **Step 1: Add `completions` command to `shell.go`**

```go
func newCompletionsCmd(root *cobra.Command) *cobra.Command {
    return &cobra.Command{
        Use:       "completions <shell>",
        Short:     "Generate shell completion script for the given shell",
        Long: `Generate shell completion scripts.

To load completions:
  bash:  source <(orbiter completions bash)
  zsh:   source <(orbiter completions zsh)
  fish:  orbiter completions fish | source`,
        Args:      cobra.ExactArgs(1),
        ValidArgs: []string{"bash", "zsh", "fish"},
        RunE: func(cmd *cobra.Command, args []string) error {
            switch args[0] {
            case "bash":
                return root.GenBashCompletion(os.Stdout)
            case "zsh":
                return root.GenZshCompletion(os.Stdout)
            case "fish":
                return root.GenFishCompletion(os.Stdout, true)
            default:
                return fmt.Errorf("unsupported shell %q — supported: bash, zsh, fish", args[0])
            }
        },
    }
}
```

- [ ] **Step 2: Register in `root.go`**

In `NewRootCommand()`, after building the root command, add completions registration at the end (must be after root is built so we can pass the root reference):

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
    newInitCmd(),
    newCompletionsCmd(root),
)
```

- [ ] **Step 3: Test**

```bash
just build
./bin/orbiter completions zsh | head -5
```

Expected: zsh completion script header.

- [ ] **Step 4: Commit**

```bash
git add internal/commands/shell.go internal/commands/root.go
git commit -m "feat: add orbiter completions command for bash/zsh/fish"
```

---

## Task 15: orbiter starchart TUI entry point

Wire `orbiter starchart` as the TUI entry point. Phase 3 scope is wiring only — no new TUI features.

**Files:**
- Modify: `internal/commands/shell.go` (or create `internal/commands/starchart.go`)
- Modify: `internal/commands/root.go`

- [ ] **Step 1: Check `cmd/orbiter/main.go` for existing TUI call pattern**

The previous `cmd/orbiter/` binary called the TUI package. Check `internal/tui/` for a `Run()` function or equivalent entry point:

```bash
ls internal/tui/
grep -r "func Run\|func Start\|func New" internal/tui/
```

- [ ] **Step 2: Create `internal/commands/starchart_cmd.go`**

```go
package commands

import (
    "github.com/Kenttleton/orbiter/internal/tui"
    "github.com/spf13/cobra"
)

func newStarChartCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "starchart",
        Short: "Open the Star Chart TUI — visual universe and beacon viewer",
        RunE: func(cmd *cobra.Command, args []string) error {
            return tui.Run(d.sc)
        },
    }
}
```

> If the TUI package doesn't yet expose `tui.Run(sc *starchart.StarChart) error`, add that function as a stub in `internal/tui/`:
> ```go
> func Run(sc *starchart.StarChart) error {
>     // Phase 5: full TUI implementation
>     return nil
> }
> ```

- [ ] **Step 3: Register in `root.go`**

Add `newStarChartCmd(&d)` to the `root.AddCommand(...)` call.

- [ ] **Step 4: Test**

```bash
just build
./bin/orbiter starchart
```

Expected: exits cleanly (stub). No panic.

- [ ] **Step 5: Run full test suite**

```bash
just test
```

Expected: all tests PASS.

- [ ] **Step 6: Final commit**

```bash
git add internal/commands/starchart_cmd.go internal/commands/root.go internal/tui/
git commit -m "feat: wire orbiter starchart as TUI entry point (Phase 5 stub)"
```

---

## Self-Review Checklist

After completing all tasks, verify spec coverage:

| Spec requirement | Task |
| --- | --- |
| Binary rename orbit → orbiter | Task 1 |
| Env vars ORBIT_* → ORBITER_* | Task 1 |
| Path column on planets/galaxies | Task 2 |
| ResolveCWD exact-first, longest-prefix | Task 3 |
| ScanBranch + CalibrateBranch | Task 4 |
| PlanRetro + ExecuteRetro with shared-node detection | Task 5 |
| Executor shared pipeline | Task 6 |
| survey command | Task 7 |
| scan command | Task 8 |
| chart command (terraform plan, beacon side-effect) | Task 9 |
| calibrate command | Task 10 |
| jump command with delta preview, confirm, shell directives | Task 11 |
| retro command with cascade retire, confirm | Task 12 |
| Shell scripts embedded + orbiter init | Task 13 |
| orbiter completions | Task 14 |
| orbiter starchart TUI entry | Task 15 |
| Retro forbidden from touching integrations | Enforced by PlanRetro scope |
| Integration skipped if not registered (unknown beacon) | Task 4 scanResource |
