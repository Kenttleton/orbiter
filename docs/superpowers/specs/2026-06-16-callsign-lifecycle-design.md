# Callsign Lifecycle Phase — Design Spec

## Goal

Extend Orbiter's FILO branch model to fully support callsigns and transponders: fix the FILO queue to include transponder-only levels, unify context-building with a generic `collectFILO` helper, add direct scan/calibrate/survey routing for callsigns and transponders, fix the `--location` config bug, and add testscript coverage.

---

## Background: The FILO Queue Model

The branch crawl produces a FILO queue that is the handoff between the crawl phase and the execute phase.

**Crawl (vessel → planet, broad to narrow):** Items are pushed into the queue from largest scope to smallest. At each level, callsign transponders are pushed first (general/keyring), then direct transponders (specific). Resources follow in role order. Planet items are pushed last and therefore sit at the top of the stack.

**Execute (pop planet → vessel, narrow to broad):** Items are popped LIFO. First pop per role/brand claims that slot. Later pops for the same role/brand are skipped — credentials that have been overridden by a narrower scope are never activated. Auth isolation is per-branch: the queue only contains items from the active vessel→planet path.

Within a level, direct transponders supersede callsign transponders of the same role/brand. Across levels, planet supersedes galaxy supersedes vessel.

---

## Architecture

### 1. FILO Queue Fix — `LeveledBranchCrawl`

**Current bug:** `LeveledBranchCrawl` (Pass 2) skips any level that has no resources. A galaxy with only a callsign is never pushed into the queue, so its transponders are invisible to planet-level scans.

**Fix:** Include a level if it has resources **or** transponders (direct or via callsign).

**New query — `directTranspondersAttachedTo(entityID)`:**
Queries transponders attached directly to a hierarchy entity (not via a callsign):
```sql
SELECT tp.id, tp.role, tp.brand, tp.config, tp.created_at
FROM transponders tp
JOIN attachments a ON a.from_id = tp.id
WHERE a.to_id = ?
```
The existing `transpondersAttachedTo(callsignID)` remains unchanged and continues to query via callsign.

**Within-level transponder ordering in `BranchLevel.Transponders`:**
Build the slice callsign-transponders-first, direct-transponders-second. This ensures that when the FILO consumer iterates `lb.Levels` (planet-first) with a `seenTransponder` map, direct transponders win over callsign transponders of the same role/brand at the same level.

**`LeveledBranchCrawl` Pass 1** continues to resolve the effective callsign per level (own callsign only — no special inheritance needed, since the FILO walk already threads ancestor transponders through via the level iteration order).

---

### 2. Generic `collectFILO` Helper — `starchart/crawl.go`

Resources and transponders share identical FILO dedup logic. Extract a single generic helper. The interface is defined here at the consumer (Go idiom: interfaces where consumed, not where produced):

```go
// roleBranded is satisfied by models.Resource and models.Transponder via duck typing.
type roleBranded interface {
    GetRole() string
    GetBrand() string
}

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
            key := item.GetRole() + "/" + item.GetBrand()
            brands := deps[item.GetRole()]
            if brandAccepted(item.GetBrand(), brands) && !seen[key] {
                seen[key] = true
                result[item.GetRole()] = append(result[item.GetRole()], wrap(item))
            }
        }
    }
    return result
}
```

**`BuildResolvedContextForResource` and `BuildResolvedContextForTransponder` collapse into one:**

```go
func BuildResolvedContext(self any, lb LeveledBranch, manifest integrations.Manifest) integrations.ResolvedContext {
    return integrations.ResolvedContext{
        Platform: lb.Platform,
        Self:     self,
        Resources: collectFILO(lb.Levels,
            func(l BranchLevel) []models.Resource { return l.Resources },
            func(r models.Resource) integrations.ResolvedResource { return integrations.ResolvedResource{Resource: r} },
            manifest.Dependencies.Resources,
        ),
        Transponders: collectFILO(lb.Levels,
            func(l BranchLevel) []models.Transponder { return l.Transponders },
            func(tp models.Transponder) integrations.ResolvedTransponder { return integrations.ResolvedTransponder{Transponder: tp} },
            manifest.Dependencies.Transponders,
        ),
    }
}
```

All callers of `BuildResolvedContextForResource` and `BuildResolvedContextForTransponder` are updated to call `BuildResolvedContext`.

**Transponder scope change:** Previously `BuildResolvedContextForResource` only provided transponders from the resource's own level. With `collectFILO` iterating all levels, a resource at planet level now sees a GitHub token callsign at galaxy level — correct FILO behavior.

**Broader cleanup note:** The `collectFILO` pattern may apply elsewhere in the codebase. Flag for a follow-on cleanup pass after this feature lands.

---

### 3. Direct Scan / Calibrate / Survey Routing

**New starchart methods:**

`ScanCallsign(ctx context.Context, callsignID string) (CallsignScanResult, error)`
- Queries transponders attached to the callsign
- For each transponder: calls integration `Scan` with a minimal `ResolvedContext` (`Self` set, empty resources and transponders maps — no branch context)
- Updates beacons as a side effect
- Returns per-transponder results

`CalibrateCallsign(ctx context.Context, callsignID string) (CallsignCalibrateResult, error)`
- Same as `ScanCallsign` but calls `Calibrate` on drifted/failed transponders

`ScanTransponder(ctx context.Context, transponderID string) (integrations.TransponderScanResult, error)`
- Single-transponder variant of the above

`CalibrateTransponder(ctx context.Context, transponderID string) (integrations.TransponderCalibrateResult, error)`

**Routing in `executor.go`:**

After `resolveTarget`, inspect `alias.ID[8:10]` (entity type prefix in orbit ID):
- `"cs"` → route to `ScanCallsign` / `CalibrateCallsign`
- `"tp"` → route to `ScanTransponder` / `CalibrateTransponder`
- anything else → existing `ScanBranch` / `CalibrateBranch`

**`Survey` routing (same pattern):**
- Callsign: show transponders attached to callsign and their beacon statuses
- Transponder: show role/brand/config and beacon status
- Other entities: existing behavior, now includes transponders in output (from `BranchCrawl` which already collects transponders via callsigns — updated to also collect direct transponders)

---

### 4. Bug Fix — `transponder add --location`

`newTransponderAddCmd` currently passes the raw `location` string as the `config` argument to `CreateTransponder`. `CreateTransponder` expects a JSON object.

Fix in the command:
```go
config := fmt.Sprintf(`{"location": %q}`, location)
tp, err := d.sc.CreateTransponder(cmd.Context(), args[0], role, brand, config)
```

---

### 5. Testscript — `cmd/orbiter/testdata/script/10-callsign.txt`

Covers:
- `callsign add` + `survey <callsign>` shows empty transponders
- `transponder add --role file --brand github --location /path/to/token`
- `attach <transponder> <callsign>` + `survey <callsign>` shows 1 transponder
- `scan <callsign>` direct — transponder beacon shown
- `planet add` + `attach <callsign> <planet>` + `scan <planet>` — transponder visible via callsign
- Galaxy-level callsign + planet with no own callsign: `scan <planet>` shows inherited transponder
- Direct transponder on planet supersedes galaxy callsign transponder of same role/brand
- `retro <callsign> --yes` — callsign and its transponders are retired

---

## Files Changed

| File | Change |
|------|--------|
| `internal/starchart/crawl.go` | `directTranspondersAttachedTo`, `collectFILO`, unified `BuildResolvedContext`, fix level-skip predicate |
| `internal/starchart/lifecycle.go` | Update `InitResource` / `InitTransponder` callers to `BuildResolvedContext` |
| `internal/starchart/starchart.go` or new `callsign.go` | `ScanCallsign`, `CalibrateCallsign`, `ScanTransponder`, `CalibrateTransponder` |
| `internal/commands/executor.go` | Entity type routing in `Scan`, `Calibrate`, `Survey` |
| `internal/commands/transponder.go` | Fix `--location` → JSON config |
| `cmd/orbiter/testdata/script/10-callsign.txt` | New testscript |
| `internal/starchart/crawl_test.go` or `lifecycle_test.go` | Unit tests for `collectFILO`, FILO dedup, level-skip fix, transponder inheritance |
