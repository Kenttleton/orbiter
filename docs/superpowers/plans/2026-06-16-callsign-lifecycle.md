# Callsign Lifecycle Phase Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the FILO branch queue to include callsign/transponder-only levels, unify context-building with a generic helper, add direct scan/calibrate/survey routing for callsigns and transponders, and fix the `--location` config bug.

**Architecture:** Three layers of change — the starchart crawl (FILO queue fix + generic `collectFILO` helper), new public lifecycle methods (`ScanCallsign`, `CalibrateCallsign`, `ScanTransponder`, `CalibrateTransponder`), and executor routing that dispatches to those methods based on entity type prefix in the orbit ID.

**Tech Stack:** Go 1.21+ generics, `modernc.org/sqlite`, `github.com/spf13/cobra`, `rogpeppe/go-internal/testscript`

---

## File Map

| File | Change |
|------|--------|
| `internal/commands/transponder.go` | Fix `--location` → JSON config |
| `internal/integrations/types.go` | Add `BeaconStatus string` to `TransponderScanResult` |
| `internal/starchart/crawl.go` | `directTranspondersAttachedTo`, FILO level-skip fix, `roleBranded` interface, `collectFILO`, new `BuildResolvedContext`, rename old to `BuildResolvedContextFromBranch` |
| `internal/starchart/lifecycle.go` | Drop `level BranchLevel` from `scanResource`/`scanTransponder`/`calibrateResource`/`calibrateTransponder`; add beacon updates to `scanTransponder`; new public `ScanCallsign`, `CalibrateCallsign`, `ScanTransponder`, `CalibrateTransponder` |
| `internal/starchart/init.go` | Update `InitResource`/`InitTransponder` to use `BuildResolvedContextFromBranch` |
| `internal/commands/executor.go` | Entity type routing in `Scan`, `Calibrate`, `Survey`; render transponders in branch scan output |
| `internal/commands/transponder_test.go` | New: `TestTransponderAdd_LocationBuildsJSON` |
| `internal/starchart/crawl_test.go` | New: `TestBuildResolvedContext_TranspondersFILO`, `TestCollectFILO_*` |
| `internal/starchart/lifecycle_test.go` | Add FILO inheritance + direct-supersedes-callsign tests, `ScanCallsign` tests |
| `internal/commands/executor_test.go` | Add callsign/transponder routing tests |
| `cmd/orbiter/testdata/script/10-callsign.txt` | New testscript |

---

## Task 1: Fix `--location` flag → JSON config

**Files:**
- Modify: `internal/commands/transponder.go`
- Create: `internal/commands/transponder_test.go`

The `transponder add --location /path` currently passes the raw path string as the `config` argument to `CreateTransponder`, but `CreateTransponder` expects a JSON object (`{"location": "/path"}`). This is a data corruption bug.

- [ ] **Step 1: Write the failing test**

Create `internal/commands/transponder_test.go`:

```go
package commands_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestSC(t *testing.T) *starchart.StarChart {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "sc-*.db")
	require.NoError(t, err)
	f.Close()
	sc, err := starchart.Open(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return sc
}

func TestTransponderAdd_LocationBuildsJSON(t *testing.T) {
	sc := openTestSC(t)
	root := commands.NewRootCommandWithSC(sc)
	root.SetArgs([]string{"transponder", "add", "gh-token",
		"--role", "file",
		"--brand", "github",
		"--location", "/home/kent/.github_token",
	})
	err := root.Execute()
	require.NoError(t, err)

	alias, err := sc.Resolve(context.Background(), "gh-token")
	require.NoError(t, err)
	var tp struct {
		Config string `db:"config"`
	}
	_ = sc.GetRaw(context.Background(), "transponders", alias.ID, &tp)

	var cfg map[string]string
	require.NoError(t, json.Unmarshal([]byte(tp.Config), &cfg))
	assert.Equal(t, "/home/kent/.github_token", cfg["location"])
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/commands/... -run TestTransponderAdd_LocationBuildsJSON -v
```

Expected: FAIL — config contains raw path string, not JSON.

> **Note:** If `NewRootCommandWithSC` or `GetRaw` don't exist yet, this test will fail to compile. That's expected — check what helpers are available in `internal/commands` and `internal/starchart` for injection. If `NewRootCommandWithSC` doesn't exist, test via the existing `executor_test.go` pattern instead: create an executor with `openTestExecutor`, call `SC().CreateTransponder` directly and verify config is valid JSON after the fix. The key assertion is `json.Unmarshal(config)` succeeds and `cfg["location"]` equals the path.

- [ ] **Step 3: Fix `transponder add` to build JSON**

In `internal/commands/transponder.go`, in `newTransponderAddCmd` `RunE`, replace the `CreateTransponder` call:

```go
// Before (passes raw path — wrong):
tp, err := d.sc.CreateTransponder(cmd.Context(), args[0], role, brand, location)

// After (builds JSON config):
config := fmt.Sprintf(`{"location": %q}`, location)
tp, err := d.sc.CreateTransponder(cmd.Context(), args[0], role, brand, config)
```

Same fix in `newTransponderInitCmd` `RunE`:

```go
// Before:
tp, err := d.sc.CreateTransponder(ctx, args[0], role, brand, location)

// After:
config := fmt.Sprintf(`{"location": %q}`, location)
tp, err := d.sc.CreateTransponder(ctx, args[0], role, brand, config)
```

- [ ] **Step 4: Run all command tests**

```bash
go test ./internal/commands/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/transponder.go internal/commands/transponder_test.go
git commit -m "fix: transponder add --location builds JSON config"
```

---

## Task 2: FILO queue fix — direct transponders + beacon parity

**Files:**
- Modify: `internal/starchart/crawl.go`
- Modify: `internal/integrations/types.go`
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/lifecycle_test.go`

Three fixes: (1) add `directTranspondersAttachedTo` for transponders attached directly to hierarchy entities; (2) stop skipping levels with no resources when transponders are present; (3) add `BeaconStatus` to `TransponderScanResult` and update beacons in `scanTransponder` (parity with `scanResource`).

Within-level ordering: direct transponders first in `BranchLevel.Transponders`, callsign transponders second. Because the FILO consumer iterates `lb.Levels` planet-first with a `seen` map, direct transponders are seen first and claim their role/brand slot — callsign transponders of the same role/brand are skipped.

- [ ] **Step 1: Write failing tests**

Add to `internal/starchart/lifecycle_test.go`:

```go
func TestScanBranch_GalaxyCallsignFlowsToPlanet(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payments", g.ID, "")

	// resource on planet so its level is included
	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payments")

	// callsign on galaxy — galaxy has NO resources, was previously skipped
	_, _ = sc.CreateCallsign(ctx, "corp-keys")
	tp, _ := sc.CreateTransponder(ctx, "corp-token", "file", "github", `{"location":"/tmp/token"}`)
	_, _ = sc.Attach(ctx, "corp-token", "corp-keys")
	_, _ = sc.Attach(ctx, "corp-keys", "acme")

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1,
		"galaxy callsign transponder must flow to planet scan even when galaxy has no resources")
	assert.Equal(t, tp.ID, result.Transponders[0].Transponder.ID)
	assert.NotEmpty(t, result.Transponders[0].BeaconStatus)
}

func TestScanBranch_DirectTransponderSupersedesCallsign(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payments", g.ID, "")

	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payments")

	// callsign with file/github transponder
	_, _ = sc.CreateCallsign(ctx, "corp-keys")
	_, _ = sc.CreateTransponder(ctx, "corp-token", "file", "github", `{"location":"/tmp/corp"}`)
	_, _ = sc.Attach(ctx, "corp-token", "corp-keys")
	_, _ = sc.Attach(ctx, "corp-keys", "payments")

	// direct file/github transponder on planet — same role/brand, should supersede
	direct, _ := sc.CreateTransponder(ctx, "personal-token", "file", "github", `{"location":"/tmp/personal"}`)
	_, _ = sc.Attach(ctx, "personal-token", "payments")

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1,
		"direct transponder supersedes callsign transponder of same role/brand")
	assert.Equal(t, direct.ID, result.Transponders[0].Transponder.ID)
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/starchart/... -run "TestScanBranch_GalaxyCallsignFlowsToPlanet|TestScanBranch_DirectTransponderSupersedesCallsign" -v
```

Expected: FAIL — galaxy callsign level is skipped; direct transponder dedup not implemented.

- [ ] **Step 3: Add `BeaconStatus` to `TransponderScanResult`**

In `internal/integrations/types.go`, update:

```go
// TransponderScanResult pairs a transponder with its scan report.
type TransponderScanResult struct {
	Transponder  models.Transponder
	Report       StateReport
	BeaconStatus string // "healthy" | "drifted" | "failed"
}
```

- [ ] **Step 4: Add `directTranspondersAttachedTo` to `crawl.go`**

Add this function to `internal/starchart/crawl.go` (after `transpondersAttachedTo`):

```go
// directTranspondersAttachedTo returns transponders attached directly to entityID,
// without going through a callsign. Used to collect isolated/specific auth pointers.
func (sc *StarChart) directTranspondersAttachedTo(ctx context.Context, entityID string) ([]models.Transponder, error) {
	const q = `
        SELECT tp.id, tp.role, tp.brand, tp.config, tp.created_at
        FROM transponders tp
        JOIN attachments a ON a.from_id = tp.id
        WHERE a.to_id = ?
    `
	rows, err := sc.db.QueryContext(ctx, q, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.Transponder
	for rows.Next() {
		var tp models.Transponder
		if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Config, &tp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, tp)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Fix `LeveledBranchCrawl` Pass 2**

In `internal/starchart/crawl.go`, replace the entire Pass 2 loop (currently starting at `// Pass 2: FILO`):

```go
// Pass 2: FILO — target first (planet), vessel last.
// Include a level if it has resources OR transponders.
// Within a level, direct transponders are placed first (specific — wins on FILO pop),
// callsign transponders second (general).
lb := LeveledBranch{Platform: currentPlatform()}
for _, levelID := range chain {
    resources, err := sc.resourcesAttachedTo(ctx, levelID)
    if err != nil {
        return LeveledBranch{}, fmt.Errorf("resources at level %s: %w", levelID, err)
    }
    directTPs, err := sc.directTranspondersAttachedTo(ctx, levelID)
    if err != nil {
        return LeveledBranch{}, fmt.Errorf("direct transponders at level %s: %w", levelID, err)
    }
    var callsignTPs []models.Transponder
    var cs *models.Callsign
    if ce := effectiveCallsign[levelID]; ce != nil {
        cs = &ce.cs
        callsignTPs = ce.tps
    }
    if len(resources) == 0 && len(directTPs) == 0 && len(callsignTPs) == 0 {
        continue
    }
    level := BranchLevel{
        EntityID:  levelID,
        Resources: resources,
        Callsign:  cs,
        // direct first (specific — supersedes callsign transponders on FILO pop),
        // callsign second (general).
        Transponders: append(directTPs, callsignTPs...),
    }
    lb.Levels = append(lb.Levels, level)
}
```

- [ ] **Step 6: Update `scanTransponder` to update beacons and set `BeaconStatus`**

In `internal/starchart/lifecycle.go`, replace `scanTransponder`:

```go
func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (integrations.TransponderScanResult, error) {
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, tp.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
		}); err != nil {
			return integrations.TransponderScanResult{}, err
		}
		return integrations.TransponderScanResult{Transponder: tp, BeaconStatus: status}, nil
	}
	rc := BuildResolvedContextForTransponder(tp, level, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
		return integrations.TransponderScanResult{}, err
	}
	return integrations.TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}
```

- [ ] **Step 7: Run the failing tests**

```bash
go test ./internal/starchart/... -v
```

Expected: `TestScanBranch_GalaxyCallsignFlowsToPlanet` and `TestScanBranch_DirectTransponderSupersedesCallsign` PASS. All other tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/integrations/types.go internal/starchart/crawl.go internal/starchart/lifecycle.go internal/starchart/lifecycle_test.go
git commit -m "fix: FILO queue includes transponder-only levels; direct transponders supersede callsign"
```

---

## Task 3: `collectFILO` generic helper + unified `BuildResolvedContext`

**Files:**
- Modify: `internal/starchart/crawl.go`
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/init.go`
- Create: `internal/starchart/crawl_test.go`

Extract the repeated FILO dedup loop into a single generic function. Replace `BuildResolvedContextForResource` and `BuildResolvedContextForTransponder` with one `BuildResolvedContext`. This also changes transponder scope in resource dispatch from "own level only" to "all levels FILO" — a resource at planet level can now see a GitHub token at galaxy level.

The old `BuildResolvedContext(BranchContext, Manifest)` is renamed to `BuildResolvedContextFromBranch` so `init.go` keeps using the BranchCrawl path unchanged.

- [ ] **Step 1: Write the failing test**

Create `internal/starchart/crawl_test.go`:

```go
package starchart_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildResolvedContext_TranspondersFILO(t *testing.T) {
	// Planet-level transponder and galaxy-level transponder share role/brand.
	// Planet (index 0 in lb.Levels) must win via FILO semantics.
	planet := models.Transponder{ID: "planet-tp", Role: "file", Brand: "github", Config: `{"location":"/planet"}`}
	galaxy := models.Transponder{ID: "galaxy-tp", Role: "file", Brand: "github", Config: `{"location":"/galaxy"}`}

	lb := starchart.LeveledBranch{
		Levels: []starchart.BranchLevel{
			{EntityID: "planet-id", Transponders: []models.Transponder{planet}},
			{EntityID: "galaxy-id", Transponders: []models.Transponder{galaxy}},
		},
	}
	manifest := integrations.Manifest{
		Dependencies: integrations.Dependencies{
			Transponders: map[string][]string{"file": {"github"}},
		},
	}
	self := models.Resource{ID: "res-id", Role: "remote", Brand: "github"}

	rc := starchart.BuildResolvedContext(self, lb, manifest)

	require.Len(t, rc.Transponders["file"], 1, "only one file/github entry — planet supersedes galaxy via FILO")
	assert.Equal(t, planet.ID, rc.Transponders["file"][0].Transponder.ID)
}

func TestBuildResolvedContext_ResourcesFILO(t *testing.T) {
	// Planet-level resource and galaxy-level resource share role/brand.
	// Planet wins.
	planet := models.Resource{ID: "planet-rs", Role: "runtime", Brand: "node", Config: "{}"}
	galaxy := models.Resource{ID: "galaxy-rs", Role: "runtime", Brand: "node", Config: "{}"}

	lb := starchart.LeveledBranch{
		Levels: []starchart.BranchLevel{
			{EntityID: "planet-id", Resources: []models.Resource{planet}},
			{EntityID: "galaxy-id", Resources: []models.Resource{galaxy}},
		},
	}
	manifest := integrations.Manifest{
		Dependencies: integrations.Dependencies{
			Resources: map[string][]string{"runtime": {"node"}},
		},
	}
	self := models.Resource{ID: "res-id", Role: "tool", Brand: "npm"}

	rc := starchart.BuildResolvedContext(self, lb, manifest)

	require.Len(t, rc.Resources["runtime"], 1, "only one runtime/node entry — planet supersedes galaxy via FILO")
	assert.Equal(t, planet.ID, rc.Resources["runtime"][0].Resource.ID)
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/starchart/... -run "TestBuildResolvedContext" -v
```

Expected: compile error — `starchart.BuildResolvedContext` doesn't yet accept `(Entity, LeveledBranch, Manifest)`.

- [ ] **Step 3: Add `roleBranded` interface and `collectFILO` to `crawl.go`**

Add to `internal/starchart/crawl.go` (before `BuildResolvedContext`):

```go
// roleBranded is satisfied by models.Resource and models.Transponder via duck typing.
// Defined at the consumer (idiomatic Go — not in the models package).
type roleBranded interface {
	GetRole() string
	GetBrand() string
}

// collectFILO walks lb.Levels in order, collecting items whose role appears in deps
// and whose brand is accepted by that role's whitelist. First match per role/brand wins
// (FILO semantics: planet-first levels supersede ancestor levels).
func collectFILO[T roleBranded, R any](
	levels []BranchLevel,
	getItems func(BranchLevel) []T,
	wrap func(T) R,
	deps map[string][]string,
) map[string][]R {
	seen := make(map[string]bool)
	result := make(map[string][]R)
	for _, l := range levels {
		for _, item := range getItems(l) {
			brands, ok := deps[item.GetRole()]
			if !ok {
				continue
			}
			key := item.GetRole() + "/" + item.GetBrand()
			if brandAccepted(item.GetBrand(), brands) && !seen[key] {
				seen[key] = true
				result[item.GetRole()] = append(result[item.GetRole()], wrap(item))
			}
		}
	}
	return result
}
```

- [ ] **Step 4: Add new `BuildResolvedContext` and rename old one**

In `internal/starchart/crawl.go`:

1. Rename `BuildResolvedContext(branch BranchContext, manifest integrations.Manifest)` to `BuildResolvedContextFromBranch` (keep body unchanged).

2. Replace `BuildResolvedContextForResource` and `BuildResolvedContextForTransponder` with a single new function:

```go
// BuildResolvedContext assembles the ResolvedContext for an integration dispatch.
// Resources and transponders are collected FILO across all branch levels
// (planet-first = narrow scope supersedes broad scope).
// self is the entity being dispatched — either a models.Resource or models.Transponder.
func BuildResolvedContext(self integrations.Entity, lb LeveledBranch, manifest integrations.Manifest) integrations.ResolvedContext {
	return integrations.ResolvedContext{
		Platform: lb.Platform,
		Self:     self,
		Resources: collectFILO(
			lb.Levels,
			func(l BranchLevel) []models.Resource { return l.Resources },
			func(r models.Resource) integrations.ResolvedResource { return integrations.ResolvedResource{Resource: r} },
			manifest.Dependencies.Resources,
		),
		Transponders: collectFILO(
			lb.Levels,
			func(l BranchLevel) []models.Transponder { return l.Transponders },
			func(tp models.Transponder) integrations.ResolvedTransponder { return integrations.ResolvedTransponder{Transponder: tp} },
			manifest.Dependencies.Transponders,
		),
	}
}
```

Delete `BuildResolvedContextForResource` and `BuildResolvedContextForTransponder` entirely.

- [ ] **Step 5: Update callers in `lifecycle.go`**

In `internal/starchart/lifecycle.go`:

Remove `level BranchLevel` from `scanResource`, `calibrateResource`, `scanTransponder`, `calibrateTransponder` signatures, and replace `BuildResolvedContextForResource`/`BuildResolvedContextForTransponder` calls with `BuildResolvedContext`:

```go
// scanResource — remove level param, use BuildResolvedContext
func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceScanResult, error) {
	integration, ok := sc.integrations.Get(r.Role, r.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, r.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand),
		}); err != nil {
			return ResourceScanResult{}, err
		}
		return ResourceScanResult{Resource: r, BeaconStatus: status}, nil
	}
	rc := BuildResolvedContext(r, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, r.ID, status, report.Observations); err != nil {
		return ResourceScanResult{}, err
	}
	return ResourceScanResult{Resource: r, Report: report, BeaconStatus: status}, nil
}

// calibrateResource — remove level param
func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceCalibrateResult, error) {
	scanResult, err := sc.scanResource(ctx, r, lb)
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
	rc := BuildResolvedContext(r, lb, integration.Meta())
	after := integration.Calibrate(rc)
	afterStatus := scanBeaconStatus(after)
	if err := sc.setBeaconStatus(ctx, r.ID, afterStatus, after.Observations); err != nil {
		return ResourceCalibrateResult{}, err
	}
	action := "calibrated"
	if afterStatus == models.BeaconStatusFailed || after.Error != "" {
		action = "failed"
	}
	return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, After: after, Action: action}, nil
}

// scanTransponder — remove level param, use BuildResolvedContext
func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, lb LeveledBranch) (integrations.TransponderScanResult, error) {
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, tp.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
		}); err != nil {
			return integrations.TransponderScanResult{}, err
		}
		return integrations.TransponderScanResult{Transponder: tp, BeaconStatus: status}, nil
	}
	rc := BuildResolvedContext(tp, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
		return integrations.TransponderScanResult{}, err
	}
	return integrations.TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}

// calibrateTransponder — remove level param
func (sc *StarChart) calibrateTransponder(ctx context.Context, tp models.Transponder, lb LeveledBranch) (integrations.TransponderCalibrateResult, error) {
	tr, err := sc.scanTransponder(ctx, tp, lb)
	if err != nil {
		return integrations.TransponderCalibrateResult{}, err
	}
	if tr.BeaconStatus == models.BeaconStatusHealthy {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report}, nil
	}
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report}, nil
	}
	rc := BuildResolvedContext(tp, lb, integration.Meta())
	after := integration.Calibrate(rc)
	return integrations.TransponderCalibrateResult{Transponder: tp, Report: after}, nil
}
```

Update `ScanBranch` and `CalibrateBranch` call sites — remove `level` from the inner calls:

```go
// In ScanBranch:
for _, level := range lb.Levels {
    for _, r := range sortedResources(level.Resources) {
        rr, err := sc.scanResource(ctx, r, lb)  // removed level
        ...
    }
    for _, tp := range level.Transponders {
        tr, err := sc.scanTransponder(ctx, tp, lb)  // removed level
        ...
    }
}

// In CalibrateBranch:
for _, level := range lb.Levels {
    for _, r := range sortedResources(level.Resources) {
        cr, err := sc.calibrateResource(ctx, r, lb)  // removed level
        ...
    }
    for _, tp := range level.Transponders {
        tr, err := sc.calibrateTransponder(ctx, tp, lb)  // removed level
        ...
    }
}
```

- [ ] **Step 6: Update `init.go` to use `BuildResolvedContextFromBranch`**

In `internal/starchart/init.go`, update both callers:

```go
// In InitResource:
report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))

// In InitTransponder:
report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))
```

- [ ] **Step 7: Run all starchart tests**

```bash
go test ./internal/starchart/... -v
```

Expected: all PASS including new `TestBuildResolvedContext_TranspondersFILO` and `TestBuildResolvedContext_ResourcesFILO`.

- [ ] **Step 8: Run full test suite**

```bash
go test ./... 
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/starchart/crawl.go internal/starchart/crawl_test.go internal/starchart/lifecycle.go internal/starchart/init.go
git commit -m "refactor: collectFILO generic helper unifies BuildResolvedContext; transponders now FILO across all levels"
```

---

## Task 4: `ScanCallsign`, `CalibrateCallsign`, `ScanTransponder`, `CalibrateTransponder`

**Files:**
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/lifecycle_test.go`

Add public methods for direct callsign and transponder lifecycle operations. These run in isolation — no branch context, `Self` set, empty resources and transponders maps. The integration receives only what it needs: the transponder's role/brand/config to check credential reachability.

- [ ] **Step 1: Write failing tests**

Add to `internal/starchart/lifecycle_test.go`:

```go
func TestScanCallsign_NoTransponders(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	cs, err := sc.CreateCallsign(ctx, "empty-keys")
	require.NoError(t, err)

	result, err := sc.ScanCallsign(ctx, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, result.Transponders)
}

func TestScanCallsign_WithTransponder(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	cs, _ := sc.CreateCallsign(ctx, "my-keys")
	tp, _ := sc.CreateTransponder(ctx, "gh-token", "file", "github", `{"location":"/tmp/token"}`)
	_, _ = sc.Attach(ctx, "gh-token", "my-keys")

	result, err := sc.ScanCallsign(ctx, cs.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1)
	assert.Equal(t, tp.ID, result.Transponders[0].Transponder.ID)
	// no integration registered → beacon status failed
	assert.Equal(t, models.BeaconStatusFailed, result.Transponders[0].BeaconStatus)
}

func TestScanTransponder_Isolation(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	tp, err := sc.CreateTransponder(ctx, "solo-token", "file", "github", `{"location":"/tmp/token"}`)
	require.NoError(t, err)
	// No callsign, no entity attachment — isolated scan

	result, err := sc.ScanTransponder(ctx, tp.ID)
	require.NoError(t, err)
	assert.Equal(t, tp.ID, result.Transponder.ID)
	assert.Equal(t, models.BeaconStatusFailed, result.BeaconStatus) // no integration
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/starchart/... -run "TestScanCallsign|TestScanTransponder" -v
```

Expected: compile error — `ScanCallsign`, `ScanTransponder` not defined.

- [ ] **Step 3: Add result types and public methods to `lifecycle.go`**

Add after the `BranchCalibrateResult` type definition in `internal/starchart/lifecycle.go`:

```go
// CallsignScanResult holds scan results for all transponders in a callsign.
type CallsignScanResult struct {
	Transponders []integrations.TransponderScanResult
}

// CallsignCalibrateResult holds calibration results for all transponders in a callsign.
type CallsignCalibrateResult struct {
	Transponders []integrations.TransponderCalibrateResult
}
```

Add the four public methods after `CalibrateBranch`:

```go
// ScanCallsign scans all transponders attached to callsignID in isolation.
// No branch resources are provided — only the transponder's own config is available.
// Updates beacons as a side effect.
func (sc *StarChart) ScanCallsign(ctx context.Context, callsignID string) (CallsignScanResult, error) {
	tps, err := sc.transpondersAttachedTo(ctx, callsignID)
	if err != nil {
		return CallsignScanResult{}, fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
	}
	emptyLB := LeveledBranch{Platform: currentPlatform()}
	var result CallsignScanResult
	for _, tp := range tps {
		tr, err := sc.scanTransponder(ctx, tp, emptyLB)
		if err != nil {
			return CallsignScanResult{}, err
		}
		result.Transponders = append(result.Transponders, tr)
	}
	return result, nil
}

// CalibrateCallsign scans then calibrates drifted/failed transponders in callsignID.
// Runs in isolation — no branch resources provided.
func (sc *StarChart) CalibrateCallsign(ctx context.Context, callsignID string) (CallsignCalibrateResult, error) {
	tps, err := sc.transpondersAttachedTo(ctx, callsignID)
	if err != nil {
		return CallsignCalibrateResult{}, fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
	}
	emptyLB := LeveledBranch{Platform: currentPlatform()}
	var result CallsignCalibrateResult
	for _, tp := range tps {
		tr, err := sc.calibrateTransponder(ctx, tp, emptyLB)
		if err != nil {
			return CallsignCalibrateResult{}, err
		}
		result.Transponders = append(result.Transponders, tr)
	}
	return result, nil
}

// ScanTransponder scans a single transponder in isolation.
func (sc *StarChart) ScanTransponder(ctx context.Context, transponderID string) (integrations.TransponderScanResult, error) {
	var tp models.Transponder
	if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
		return integrations.TransponderScanResult{}, fmt.Errorf("get transponder %s: %w", transponderID, err)
	}
	return sc.scanTransponder(ctx, tp, LeveledBranch{Platform: currentPlatform()})
}

// CalibrateTransponder calibrates a single transponder in isolation.
func (sc *StarChart) CalibrateTransponder(ctx context.Context, transponderID string) (integrations.TransponderCalibrateResult, error) {
	var tp models.Transponder
	if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
		return integrations.TransponderCalibrateResult{}, fmt.Errorf("get transponder %s: %w", transponderID, err)
	}
	return sc.calibrateTransponder(ctx, tp, LeveledBranch{Platform: currentPlatform()})
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/starchart/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/starchart/lifecycle.go internal/starchart/lifecycle_test.go
git commit -m "feat: ScanCallsign, CalibrateCallsign, ScanTransponder, CalibrateTransponder"
```

---

## Task 5: Executor routing + transponder display in branch scan

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/executor_test.go`

After `resolveTarget`, inspect the entity type from the orbit ID using `models.ParseID`. Route callsigns and transponders to the new isolated scan/calibrate methods. Update the branch `Scan` output to also render transponders (they are now part of `BranchScanResult`). Update `Survey` to show transponders for callsigns and transponder details for standalone transponders.

- [ ] **Step 1: Write failing tests**

Add to `internal/commands/executor_test.go`:

```go
func TestExecutor_Scan_Callsign_NoTransponders(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateCallsign(ctx, "my-keys")
	require.NoError(t, err)

	// scan callsign — should not error (routes to ScanCallsign)
	err = exec.Scan(ctx, "my-keys")
	require.NoError(t, err)
}

func TestExecutor_Survey_Callsign(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateCallsign(ctx, "my-keys")
	require.NoError(t, err)

	err = exec.Survey(ctx, "my-keys")
	require.NoError(t, err)
}

func TestExecutor_Scan_Transponder(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateTransponder(ctx, "solo-token", "file", "github", `{"location":"/tmp/t"}`)
	require.NoError(t, err)

	err = exec.Scan(ctx, "solo-token")
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/commands/... -run "TestExecutor_Scan_Callsign|TestExecutor_Survey_Callsign|TestExecutor_Scan_Transponder" -v
```

Expected: tests fail — callsign/transponder routed to `ScanBranch` which errors or returns wrong results.

- [ ] **Step 3: Add entity type routing to `Scan`**

In `internal/commands/executor.go`, replace the body of `Executor.Scan` with:

```go
func (e *Executor) Scan(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		return e.scanCallsignEntity(ctx, alias)
	case models.EntityTypeTransponder:
		return e.scanTransponderEntity(ctx, alias)
	default:
		return e.scanBranchEntity(ctx, alias)
	}
}
```

Add the three helper methods (add after `Scan`):

```go
func (e *Executor) scanCallsignEntity(ctx context.Context, alias models.Alias) error {
	result, err := e.sc.ScanCallsign(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan callsign %s: %w", alias.Name, err)
	}
	if len(result.Transponders) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no transponders attached", alias.Name))
		return nil
	}
	var rows [][]string
	for _, tr := range result.Transponders {
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		} else if len(tr.Report.Observations) > 0 {
			obs = tr.Report.Observations[0]
		}
		rows = append(rows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs})
	}
	e.renderer.Table([]string{"transponder", "status", "observation"}, rows)
	return nil
}

func (e *Executor) scanTransponderEntity(ctx context.Context, alias models.Alias) error {
	tr, err := e.sc.ScanTransponder(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan transponder %s: %w", alias.Name, err)
	}
	obs := ""
	if tr.Report.Error != "" {
		obs = tr.Report.Error
	} else if len(tr.Report.Observations) > 0 {
		obs = tr.Report.Observations[0]
	}
	e.renderer.Table([]string{"transponder", "status", "observation"}, [][]string{
		{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs},
	})
	return nil
}

func (e *Executor) scanBranchEntity(ctx context.Context, alias models.Alias) error {
	result, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan %s: %w", alias.Name, err)
	}

	var resourceRows [][]string
	for _, r := range result.Resources {
		obs := ""
		if r.Report.Error != "" {
			obs = r.Report.Error
		} else if len(r.Report.Observations) > 0 {
			obs = r.Report.Observations[0]
		}
		resourceRows = append(resourceRows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.BeaconStatus, obs})
	}

	var transponderRows [][]string
	for _, tr := range result.Transponders {
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		} else if len(tr.Report.Observations) > 0 {
			obs = tr.Report.Observations[0]
		}
		transponderRows = append(transponderRows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs})
	}

	if len(resourceRows) == 0 && len(transponderRows) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no resources attached", alias.Name))
		return nil
	}
	if len(resourceRows) > 0 {
		e.renderer.Table([]string{"resource", "status", "observation"}, resourceRows)
	}
	if len(transponderRows) > 0 {
		e.renderer.Table([]string{"transponder", "status", "observation"}, transponderRows)
	}
	return nil
}
```

Delete the old `Scan` body (the existing `result, err := e.sc.ScanBranch(...)` block) — it is fully replaced by `scanBranchEntity`.

- [ ] **Step 4: Add entity type routing to `Calibrate`**

Replace `Executor.Calibrate` body with:

```go
func (e *Executor) Calibrate(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		result, err := e.sc.CalibrateCallsign(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate callsign %s: %w", alias.Name, err)
		}
		if len(result.Transponders) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil
		}
		var rows [][]string
		for _, tr := range result.Transponders {
			obs := ""
			if tr.Report.Error != "" {
				obs = tr.Report.Error
			}
			rows = append(rows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, obs})
		}
		e.renderer.Table([]string{"transponder", "observation"}, rows)
		return nil
	case models.EntityTypeTransponder:
		tr, err := e.sc.CalibrateTransponder(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate transponder %s: %w", alias.Name, err)
		}
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		}
		e.renderer.Table([]string{"transponder", "observation"}, [][]string{
			{tr.Transponder.Role + "/" + tr.Transponder.Brand, obs},
		})
		return nil
	default:
		// existing branch calibrate path
		result, err := e.sc.CalibrateBranch(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate %s: %w", alias.Name, err)
		}
		if len(result.Resources) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil
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
		e.renderer.Table([]string{"resource", "action", "observation"}, rows)
		return nil
	}
}
```

- [ ] **Step 5: Add entity type routing to `Survey`**

In `Executor.Survey`, after the `isCatalogBrand` check and before `resolveTarget`, add routing after resolution. Replace the block starting with `alias, err := e.resolveTarget(ctx, target)` through the end of `Survey`:

```go
alias, err := e.resolveTarget(ctx, target)
if err != nil {
    return err
}

parsed, _ := models.ParseID(alias.ID)
switch parsed.EntityType {
case models.EntityTypeCallsign:
    tps, err := e.sc.TranspondersForCallsign(ctx, alias.ID)
    if err != nil {
        return fmt.Errorf("survey callsign %s: %w", alias.Name, err)
    }
    rows := [][]string{
        {"callsign", alias.Name},
        {"id", alias.ID},
        {"transponders", fmt.Sprintf("%d", len(tps))},
    }
    e.renderer.Table([]string{"field", "value"}, rows)
    if len(tps) > 0 {
        var tpRows [][]string
        for _, tp := range tps {
            b, _ := e.sc.GetBeacon(ctx, tp.ID)
            tpRows = append(tpRows, []string{tp.Role + "/" + tp.Brand, tp.ID, b.Status})
        }
        e.renderer.Table([]string{"transponder", "id", "status"}, tpRows)
    }
    return nil
case models.EntityTypeTransponder:
    var tp models.Transponder
    if err := e.sc.GetEntity(ctx, "transponders", alias.ID, &tp); err != nil {
        return fmt.Errorf("survey transponder %s: %w", alias.Name, err)
    }
    b, _ := e.sc.GetBeacon(ctx, tp.ID)
    rows := [][]string{
        {"transponder", alias.Name},
        {"id", alias.ID},
        {"role", tp.Role},
        {"brand", tp.Brand},
        {"status", b.Status},
    }
    e.renderer.Table([]string{"field", "value"}, rows)
    return nil
}

// default: existing BranchCrawl-based survey (unchanged)
branch, err := e.sc.BranchCrawl(ctx, alias.ID)
// ... rest of existing survey code unchanged
```

> **Note:** `TranspondersForCallsign` and `GetEntity` may not exist yet. Check what public methods `StarChart` exposes. `transpondersAttachedTo` is private — you may need to add a public `TranspondersForCallsign(ctx, callsignID) ([]models.Transponder, error)` wrapper to `starchart`. For `GetEntity`, use `sc.Get(ctx, "transponders", id, &tp)` — `Get` is already public. Check `internal/starchart/starchart.go` for the public API surface and add minimal wrappers as needed.

- [ ] **Step 6: Run all tests**

```bash
go test ./internal/commands/... -v
go test ./... 
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/executor.go internal/commands/executor_test.go
git commit -m "feat: executor routes scan/calibrate/survey to callsign and transponder entity types"
```

---

## Task 6: Testscript `10-callsign.txt`

**Files:**
- Create: `cmd/orbiter/testdata/script/10-callsign.txt`

End-to-end coverage via `rogpeppe/go-internal/testscript`. All `orbiter` commands run against a fresh in-memory star chart per script. The `--yes` flag is required on `orbiter init` to skip TUI. Assertions use `stdout` and `stderr` regex patterns.

- [ ] **Step 1: Create the testscript**

Create `cmd/orbiter/testdata/script/10-callsign.txt`:

```
# Callsign lifecycle: add, attach, scan, inheritance, direct transponder supersedes.

exec orbiter init --yes

# Create galaxy and callsign
exec orbiter galaxy add acme-corp
exec orbiter callsign add acme-keys
stdout 'callsign.*acme-keys.*registered'

# Survey callsign shows zero transponders
exec orbiter survey acme-keys
stdout 'acme-keys'
stdout 'transponders.*0'

# Add transponder and attach to callsign
exec orbiter transponder add corp-gh --role file --brand github --location /tmp/corp-github-token
stdout 'transponder.*corp-gh.*registered'
exec orbiter attach corp-gh acme-keys
stdout 'attached'

# Survey callsign now shows 1 transponder
exec orbiter survey acme-keys
stdout 'transponders.*1|corp-gh'

# Direct scan of callsign — isolated, shows transponder beacon
exec orbiter scan acme-keys
stdout 'file/github'

# Planet with callsign: scan planet shows transponders via callsign
exec orbiter planet add payments --galaxy acme-corp
exec orbiter attach acme-keys payments
exec orbiter resource add pay-git --role tool --brand git
exec orbiter attach pay-git payments
exec orbiter scan payments
stdout 'file/github'

# Galaxy-level callsign flows down to a planet with no own callsign
exec orbiter callsign add galaxy-keys
exec orbiter transponder add galaxy-gl --role file --brand gitlab --location /tmp/gitlab-token
exec orbiter attach galaxy-gl galaxy-keys
exec orbiter attach galaxy-keys acme-corp

exec orbiter planet add billing --galaxy acme-corp
exec orbiter resource add bill-git --role tool --brand git
exec orbiter attach bill-git billing
exec orbiter scan billing
stdout 'file/gitlab'

# Direct transponder on planet supersedes galaxy callsign transponder of same role/brand
exec orbiter transponder add billing-gl --role file --brand gitlab --location /tmp/billing-gl-token
exec orbiter attach billing-gl billing
exec orbiter scan billing
stdout 'file/gitlab'

# Retro callsign retires it and its transponders
exec orbiter retro acme-keys --yes
stdout 'acme-keys'
! exec orbiter survey acme-keys
stderr 'not found|no target'
! exec orbiter survey corp-gh
stderr 'not found|no target'
```

- [ ] **Step 2: Run the testscript**

```bash
go test ./cmd/orbiter/... -run TestScript/10-callsign -v
```

Expected: PASS.

- [ ] **Step 3: Run all testscripts**

```bash
go test ./cmd/orbiter/... -v
```

Expected: all PASS. If any testscript regresses, investigate and fix before proceeding.

- [ ] **Step 4: Run full test suite**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/orbiter/testdata/script/10-callsign.txt
git commit -m "test: callsign lifecycle testscript — FILO inheritance, direct transponder supersedes, retro cascade"
```

---

## Cleanup Note

The `collectFILO` pattern likely applies elsewhere in the codebase (e.g., detection phase, any future multi-level data collection). Flag this file (`internal/starchart/crawl.go`) for a broader cleanup pass after this feature lands — look for similar manual `seen` map dedup loops and consider consolidating them.
