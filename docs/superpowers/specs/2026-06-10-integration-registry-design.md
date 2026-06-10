# Orbiter — Integration Registry Design

**Date:** 2026-06-10
**Phase:** 2 — Entity Build Commands (Integration Layer)
**Status:** Draft

---

## Overview

Integrations are the behavior layer for Orbiter. Every resource and transponder that Orbiter can manage has a corresponding integration identified by `role + brand`. Integrations are stateless — they receive context, interact with the real world, and report current state. Orbiter owns all state management, Beacon lifecycle, and drift detection.

This document covers the integration interface, manifest format, package structure, compile-time discovery, and the Phase 3 WASM extension path.

---

## Core Principles

- **Integrations are stateless helpers.** They do not write to the Star Chart. They do not manage Beacons. They receive a resolved context, do work, and return observed state.
- **Orbiter owns state.** All Star Chart writes, Beacon updates, drift detection, and state resolution belong to Orbiter core.
- **The manifest is the query spec.** Orbiter reads manifests before invoking any code. The manifest tells Orbiter what to pass in — Orbiter never needs to know an integration's internals at compile time.
- **Role+brand is the dispatch key.** `runtime/node` dispatches to the node integration. `manager/nvm` dispatches to the nvm integration. The registry is keyed on `"role/brand"`.

---

## Integration Interface

```go
// Integration is implemented by every registered integration — compiled Go or WASM.
type Integration interface {
    Detect(ctx DetectContext) DetectReport
    Init(ctx ResolvedContext) StateReport
    Scan(ctx ResolvedContext) StateReport
    Calibrate(ctx ResolvedContext) StateReport
}
```

### Method Contracts

**`Detect`** — discover whether this integration is relevant to the current directory.
- File-pattern integrations: called only when manifest file patterns match
- Remote/filesystem integrations: always called during detection phase
- Returns suggested resources for the Captain to confirm

**`Init`** — provision the resource or transponder in the real world.
- Receives `ResolvedContext` with all declared dependencies pre-resolved
- Uses `Platform` for native install path decisions when no manager is present
- Returns observed state after provisioning

**`Scan`** — observe current reality. Always re-derives from the real world. Never reads cached state.
- Returns current observed state
- Does not modify anything

**`Calibrate`** — bring reality into alignment with desired state.
- `ResolvedContext` carries the desired state from the Star Chart
- Integration adjusts reality to match and returns resulting state

All three operational methods return the same `StateReport` type.

---

## Types

### Platform

```go
type Platform struct {
    OS   string   // "darwin" | "linux" | "windows"
    Arch string   // "amd64" | "arm64"
}
```

### DetectContext

```go
type DetectContext struct {
    Platform Platform
    CWD      string
    Files    map[string]string   // filename → contents; populated for file-pattern integrations only
}
```

For remote and filesystem integrations: `Files` is empty. The integration uses `CWD` and `Platform` to check its client config (e.g., Dropbox reads `~/.dropbox/info.json` to find the sync root, then checks if `CWD` is a subdirectory).

### DetectReport

```go
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

Detection suggests resources to attach — not managers. Manager resolution happens via the branch crawl after the Captain confirms the suggested resources.

### ResolvedContext

`ResolvedContext` is the integration boundary struct. It is assembled from the Phase 1 branch crawl result, filtered by the integration's manifest `[dependencies]`. It is a plain serializable struct throughout — JSON tags on all fields, no unexported types, no interface values. This constraint is required for Phase 3 WASM compatibility.

```go
type ResolvedContext struct {
    Platform     Platform
    Resources    map[string][]ResolvedResource    // role → matched resources + their StateReports
    Transponders map[string][]ResolvedTransponder // role → matched transponders
}

type ResolvedResource struct {
    Resource    models.Resource
    StateReport *StateReport   // populated after integration runs; nil if not yet initialized
}

type ResolvedTransponder struct {
    Transponder models.Transponder
}
```

When an integration's dependency (e.g., a manager) has already been initialized earlier in the Phase 2 execution graph, its `StateReport` is populated in `ResolvedContext`. This allows downstream integrations to use `BinaryPath` or `InstallDir` directly when the dependency is not on PATH.

### StateReport

```go
type StateReport struct {
    Present      bool
    Reachable    bool
    BinaryPath   string          // absolute path to binary
    InstallDir   string          // base installation directory
    InPath       bool            // whether binary is on system PATH
    Manager      string          // brand of manager used — always populated
    Config       map[string]any  // observed values: version, path, endpoint, auth state, etc.
    Observations []string        // human-readable notes
    Error        string          // non-empty if interaction failed
}
```

**`Manager` is always populated.** Every installation has a manager — nvm, homebrew, apt, winget, the OS itself, or compiled from source. There is no blank manager field.

**`InPath: false` is not a failure.** When `InPath` is false, downstream integrations use `BinaryPath` or `InstallDir` directly rather than assuming the binary is on PATH.

Orbiter uses `StateReport` to:
1. Compute drift by comparing observed `Config` against desired config in Star Chart
2. Update Beacon status (`verified` / `failed` / `healthy` / `degraded`)
3. Write config discoveries back to the resource row when new values are observed

---

## Manifest Format

Each integration carries a `manifest.toml` alongside its Go code. The manifest is static metadata — Orbiter reads it before invoking any code. No logic lives in the manifest.

### Resource Integration

```toml
[integration]
type  = "resource"    # "resource" | "transponder"
role  = "runtime"
brand = "node"

[detection]
files = [".nvmrc", ".node-version", "package.json"]
# remote/filesystem roles omit files — they run Detect always

[dependencies.resources]
manager = ["nvm", "volta", "native"]
# Key is the role. Value is a whitelist of acceptable brands.
# "native" in the manager list signals bare install is acceptable.
# Empty brand list [] means any brand for that role is acceptable.

[dependencies.transponders]
# none required for node
```

```toml
[integration]
type  = "resource"
role  = "manager"
brand = "nvm"

[detection]
files = [".nvmrc", ".node-version"]

[dependencies.resources]
manages = ["node"]
# "manages" is required and non-empty for all role=manager integrations.
# Declares the brands this manager controls. Used for branch crawl resolution.
```

```toml
[integration]
type  = "resource"
role  = "remote"
brand = "github"

[detection]
files = [".git/config"]   # present at repo root

[dependencies.resources]
tool = ["git"]

[dependencies.transponders]
file  = ["github"]    # file-based SSH key for github brand
agent = []            # or any SSH agent (empty = any brand)
```

```toml
[integration]
type  = "resource"
role  = "remote"
brand = "dropbox"
# remote role — Detect always runs; no file patterns needed

[detection]
files = []

[dependencies.transponders]
oauth = ["dropbox"]
```

### Transponder Integration

```toml
[integration]
type  = "transponder"
role  = "file"
brand = "github"

[detection]
files = []   # transponder detection is not file-pattern based

[dependencies.resources]
tool = ["git"]   # git tool should be present

[dependencies.transponders]
agent = []        # can use an SSH agent, or...
file  = ["github"] # ...a file-based key for github
```

### Dependency Rules

- Keys in `[dependencies.resources]` and `[dependencies.transponders]` are roles
- Values are brand whitelists — `[]` means any brand satisfies
- For `role=manager` integrations, `manages` key is required and non-empty
- `"native"` in a manager whitelist signals bare install is acceptable when no manager is in the branch
- Missing dependency that has no acceptable fallback → integration returns `StateReport.Error` explaining why

---

## Detection Strategy

Detection strategy is determined entirely by integration role. No flags or special fields needed.

| Role | Strategy |
| --- | --- |
| `runtime`, `manager`, `tool` | **File-pattern** — Detect invoked only when manifest `files` patterns match in CWD |
| `remote`, `filesystem` | **Always-run** — Detect invoked for all registered integrations of these roles |

Remote integrations check CWD against their client's own configuration to determine if they apply (e.g., Dropbox reads `~/.dropbox/info.json` for the sync root and checks if CWD is a subdirectory).

**Detection scope:** local filesystem only. Detection never inspects the contents of sync folders or remote locations.

### Detection Flow

```text
1. Read all integration manifests
2. Scan CWD for manifest file pattern matches
3. Invoke Detect for:
   a. runtime/manager/tool integrations whose file patterns matched
   b. ALL remote and filesystem integrations
4. Aggregate DetectReports → SuggestedResources for the Captain
```

---

## Package Structure

### Compile-Time (Phase 2)

Each integration is a Go package:

```text
internal/integrations/
  register.go              ← GENERATED by go:generate; imports all discovered packages
  nvm/
    manifest.toml          ← static metadata
    nvm.go                 ← implements Integration; init() calls registry.Register(...)
  node/
    manifest.toml
    node.go
  git/
    manifest.toml
    git.go
  github/
    manifest.toml
    github.go
  dropbox/
    manifest.toml
    dropbox.go
  ...
```

### Self-Registration Pattern

Each integration registers itself via `init()`:

```go
package nvm

import "github.com/Kenttleton/orbiter/internal/integrations"

func init() {
    integrations.Register("manager", "nvm", &NvmIntegration{})
}

type NvmIntegration struct{}

func (n *NvmIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport { ... }
func (n *NvmIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport   { ... }
func (n *NvmIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport   { ... }
func (n *NvmIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport { ... }
```

### go:generate Discovery

`go:generate` in `internal/integrations/` scans all subdirectories for `manifest.toml`, validates the manifest structure, and generates `register.go`:

```go
// Code generated by go generate. DO NOT EDIT.
package integrations

import (
    _ "github.com/Kenttleton/orbiter/internal/integrations/nvm"
    _ "github.com/Kenttleton/orbiter/internal/integrations/node"
    _ "github.com/Kenttleton/orbiter/internal/integrations/git"
    _ "github.com/Kenttleton/orbiter/internal/integrations/github"
    _ "github.com/Kenttleton/orbiter/internal/integrations/dropbox"
)
```

Adding a new integration: drop a package directory with `manifest.toml` and `integration.go`, run `just build`. No manual registry editing.

---

## Registry

```go
package integrations

var registry = map[string]Integration{}

// Register is called by each integration's init() function.
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
    var result []Integration
    prefix := role + "/"
    for key, i := range registry {
        if strings.HasPrefix(key, prefix) {
            result = append(result, i)
        }
    }
    return result
}
```

---

## Dispatch from StarChart

`InitResource` in `internal/starchart` dispatches to the integration registry:

```go
func (sc *StarChart) InitResource(ctx context.Context, id string) error {
    resource, err := sc.GetResource(ctx, id)
    if err != nil {
        return err
    }

    integration, ok := integrations.Get(resource.Role, resource.Brand)
    if !ok {
        return fmt.Errorf("no integration registered for %s/%s", resource.Role, resource.Brand)
    }

    branch, err := sc.BranchCrawl(ctx, id)
    if err != nil {
        return err
    }

    manifest, err := integrations.LoadManifest(resource.Role, resource.Brand)
    if err != nil {
        return err
    }

    resolved := sc.BuildResolvedContext(branch, manifest)
    report := integration.Init(resolved)

    return sc.UpdateBeaconFromReport(ctx, id, report)
}
```

The same pattern applies for `ScanResource` and `CalibrateResource`, calling `Scan` and `Calibrate` respectively.

---

## Phase 3 — WASM Extension Path

Phase 3 adds a runtime WASM loader alongside the compiled integration registry. The interface, manifest structure, and `ResolvedContext` are identical between compiled and WASM integrations.

### WASM Integration Package Structure

Dropped next to the Orbiter binary or in `~/.orbiter/integrations/`:

```text
<binary-dir>/integrations/nvm/
  manifest.toml          ← same format as compiled integrations
  integration.wasm       ← WASM module compiled from any WASM-compatible language

~/.orbiter/integrations/custom-tool/
  manifest.toml
  integration.wasm
```

### WASM Boundary Protocol

`ResolvedContext` is JSON-serialized and written into WASM memory. `StateReport` is JSON-deserialized back. The integration's exported WASM functions:

```
detect(ptr i32, len i32) i64     // receives DetectContext JSON, returns DetectReport JSON ptr+len
init(ptr i32, len i32) i64       // receives ResolvedContext JSON, returns StateReport JSON ptr+len
scan(ptr i32, len i32) i64       // receives ResolvedContext JSON, returns StateReport JSON ptr+len
calibrate(ptr i32, len i32) i64  // receives ResolvedContext JSON, returns StateReport JSON ptr+len
```

All `ResolvedContext` fields have JSON tags. No unexported types. No interface values. This constraint is enforced at the type level in Phase 2 so Phase 3 requires no refactor.

### WASM Loader (Phase 3)

The WASM loader runs at Orbiter startup:

1. Scans `<binary-dir>/integrations/` and `~/.orbiter/integrations/`
2. For each directory containing `manifest.toml` + `integration.wasm`:
   a. Reads and validates manifest
   b. Loads WASM module via wazero (pure Go, no CGo, cross-platform)
   c. Wraps in a `WASMIntegration` struct implementing the `Integration` interface
   d. Registers in the same registry via `Register(role, brand, wasmIntegration)`
3. Compiled integrations take precedence over WASM integrations for the same role+brand

**Why wazero:** pure Go WASM runtime, no CGo, cross-platform (darwin/linux/windows). WASM modules are sandboxed — they cannot access the filesystem or network beyond what Orbiter explicitly grants.

### Go Native Plugins — Not Viable

Go's native plugin system (`plugin` package) is not viable for this use case:
- Linux and macOS only (no Windows support)
- Plugin and host must be compiled with identical Go version and flags
- No unloading support

WASM is the correct runtime-loadable integration path.

---

## Writing an Integration (Developer Guide)

### Minimal integration

1. Create `internal/integrations/<brand>/`
2. Create `manifest.toml` with integration declaration and dependencies
3. Create `<brand>.go` implementing `Integration` with `init()` self-registration
4. Run `just build` — `go:generate` discovers and imports the new package

### Init implementation pattern

```go
func (n *NodeIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    // 1. Check for manager in ResolvedContext
    managers := ctx.Resources["manager"]
    
    if len(managers) > 0 {
        mgr := managers[0]
        // Use manager — prefer BinaryPath if not InPath
        return n.initWithManager(mgr, ctx.Platform)
    }
    
    // 2. No manager — use native path based on Platform
    // (acceptable because "native" is in this integration's manifest manager whitelist)
    return n.initNative(ctx.Platform)
}

func (n *NodeIntegration) initWithManager(mgr integrations.ResolvedResource, p integrations.Platform) integrations.StateReport {
    var nvmCmd string
    if mgr.StateReport != nil && !mgr.StateReport.InPath {
        // Manager exists but isn't on PATH — use direct path
        nvmCmd = filepath.Join(mgr.StateReport.InstallDir, "nvm.sh")
    } else {
        nvmCmd = mgr.Resource.Brand   // "nvm", "volta", etc.
    }
    // ... run install ...
}
```

### Reporting failure

```go
func (n *NodeIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    managers := ctx.Resources["manager"]
    if len(managers) == 0 {
        // No manager and integration has no default — report failure with reason
        return integrations.StateReport{
            Present:  false,
            Error:    "no manager found for runtime/node — attach nvm, volta, or native to this branch",
            Manager:  "none",
        }
    }
    // ...
}
```

The Captain resolves the failure. Orbiter records the `StateReport.Error` in the Beacon observations.

---

## Out of Scope for Phase 2

- Any specific integration implementations (nvm, node, git, dropbox, etc.)
- WASM loader and runtime integration loading
- WASM boundary memory management details
- Integration sandboxing and capability grants
