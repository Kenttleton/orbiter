# Phase 4: Integration System Hardening & Expansion

## Goal

Harden the integration system from its Phase 2.5 scaffolding into a production-ready contract. This phase addresses four interrelated concerns: the integration model (brand-centric manifests, multi-role dispatch), the data model (transponder config, unified entity interface), the WASM ABI (interactive auth, dynamic buffers), and performance (module pooling, parallel dispatch, shared runtime).

## Architecture

### Integrations are brand-centric

An integration is a brand, not a role. `gh` is one integration that can satisfy multiple roles. Orbiter owns the static mapping of which roles are resource-type and which are transponder-type — the integration never declares its type.

**Role taxonomy (Orbiter-owned, static):**

| Role | Type | Contract |
|---|---|---|
| `manager` | resource | installs and manages other tools |
| `runtime` | resource | language runtime (go, node, python) |
| `tool` | resource | CLI tool available in PATH |
| `remote` | resource | remote repository or service endpoint |
| `filesystem` | resource | local path that must exist |
| `file` | transponder | credential stored in a file (cert, key, token) |
| `env` | transponder | credential stored in environment variable |
| `keychain` | transponder | interactive auth (username/password, OAuth) |
| `vault` | transponder | secrets manager (1Password, AWS Secrets Manager) |
| `agent` | transponder | SSH or credential agent |

This replaces `type = "resource" | "transponder"` in the manifest. A role's presence in the resource or transponder column is the type declaration.

### Manifest: brand + roles + config schema

```toml
[integration]
brand = "gh"
roles = ["tool", "keychain"]

[detection]
files = [".git/config"]

[commands]
# allowlist for run_command sandboxing — empty = deny all
allowed = ["git", "gh", "which"]
timeout_seconds = 30

[[config.fields]]
key = "username"
type = "string"
required = false
description = "GitHub username — pre-filled on keychain auth prompts"

[[config.fields]]
key = "account"
type = "string"
required = false
description = "Organization or personal account name"
```

`bundle.go` registers the WASM under each `(role, brand)` pair from `roles`. The same compiled module handles all roles, branching on `ctx.Self.Role()` at runtime.

`config.fields` is the contract between Orbiter and the integration. Orbiter uses it for guided `orbiter resource add`, TUI config editors, and input validation. The integration reads the JSON blob from `ctx.Self.Config()` at runtime — it does not re-declare the schema.

### Entity interface: unified Self

`ResolvedContext.Self` currently types to `*models.Resource`. Transponder dispatch requires it to accommodate both types. The idiomatic Go solution is an interface both implement:

```go
// Entity is the entity being operated on in a dispatch call.
// Both models.Resource and models.Transponder implement it.
type Entity interface {
    GetID()     string
    GetRole()   string
    GetBrand()  string
    GetConfig() string
}
```

`ResolvedContext.Self` becomes `Entity`. The WASM serialization shape is unchanged from the guest's perspective:

```json
{"id": "...", "role": "keychain", "brand": "gh", "config": "{\"username\":\"kent\"}"}
```

The guest calls `read_input`, deserializes this shape, and doesn't know or care whether the host produced it from a Resource or Transponder.

### Transponder config

`models.Transponder` gains a `Config string` column (JSON blob, same convention as `models.Resource.Config`). The existing `Location` column folds into `Config` as the `"location"` key:

```json
{"location": "~/.ssh/id_ed25519_acme"}          // was: location column
{"username": "kent", "account": "acme-corp"}     // keychain partial
{"location": "~/.aws/credentials", "profile": "acme"} // file transponder
```

A migration drops `location` and adds `config`, copying existing location values into `{"location": "..."}` in the config blob. New transponders have no `location` column — the config blob is the only storage.

### Active transponder dispatch

Transponders backed by a registered integration are dispatched during `ScanBranch` and `CalibrateBranch`, alongside resources. Transponders with no registered integration are passive pointers and are skipped.

`Scan` for a transponder means: verify the credential is reachable and valid (e.g. `gh auth status`, `ssh-add -l`, check file exists).

`Calibrate` for a transponder means: restore validity — re-authenticate, refresh a token, re-add a key to the agent.

New result types parallel the resource equivalents:

```go
type TransponderScanResult struct {
    Transponder  models.Transponder
    Report       integrations.StateReport
    BeaconStatus string
}

type TransponderCalibrateResult struct {
    Transponder models.Transponder
    Before      integrations.StateReport
    After       integrations.StateReport
    Action      string
}
```

`BranchScanResult` and `BranchCalibrateResult` each grow a `Transponders` slice.

## WASM ABI Changes

### Interactive auth: NeedsInput / Responses

Interactive credential flows (password prompts, OAuth, MFA) require a request/response cycle. The integration stays stateless — Orbiter mediates all interaction.

**New types:**

```go
type InputRequest struct {
    Key    string `json:"key"`
    Prompt string `json:"prompt"`
    Masked bool   `json:"masked"` // true for passwords, tokens
}
```

**StateReport** gains:

```go
NeedsInput []InputRequest `json:"needs_input,omitempty"`
```

**ResolvedContext** gains:

```go
Responses map[string]string `json:"responses,omitempty"`
```

**Dispatch loop** (in executor or lifecycle layer):

```
1. Call integration with ctx (Responses = nil or partial)
2. If report.NeedsInput is non-empty:
   a. CLI: prompt captain on stderr for each InputRequest (masked if Masked=true)
   b. TUI: render modal for each InputRequest
   c. Call integration again with ctx (Responses = collected answers)
3. Repeat until NeedsInput is empty or error
4. Return final StateReport
```

The integration never sees plaintext secrets beyond the single call invocation. Orbiter never stores them.

### Dynamic buffer allocation

Static 64KB buffers (`make([]byte, 64*1024)`) in `host.go` and guest code are replaced with size hints from the manifest:

```toml
[runtime]
input_buffer_kb  = 8   # default; most integrations need < 1KB
output_buffer_kb = 8
```

Host `read_input` and `write_output` allocate based on the compiled manifest hint. Guest code uses the same hint via a manifest-provided constant or falls back to a reasonable default. Eliminates 128KB+ of stack allocation per dispatch call.

### run_command sandboxing

`run_command` currently executes any command with no restrictions. The manifest `[commands]` section defines an allowlist and timeout:

- Commands not in `allowed` return empty output and a non-zero length indicator the guest can check.
- `timeout_seconds` applies per command invocation (default 30).
- Commands inherit no environment variables beyond `PATH` — secrets are not leaked via env.

## Performance Hardening

### Shared wazero runtime

`newRuntime(ctx)` currently creates a separate wazero `Runtime` per integration. One shared runtime serves all modules:

```go
// internal/wasm/runtime.go
var sharedRuntime wazero.Runtime  // initialized once in package init()
```

All `Load()` calls use the shared runtime. Reduces memory overhead and eliminates repeated JIT compilation of the wazero host module.

### Module instance pooling

`WASMIntegration` currently holds one module instance guarded by `sync.Mutex`, serializing all calls. Replace with a pool of pre-warmed instances:

```go
type WASMIntegration struct {
    manifest integrations.Manifest
    compiled wazero.CompiledModule
    pool     chan api.Module  // buffered; capacity = pool size
}
```

On `Load()`: compile once, instantiate `poolSize` times (default 4), push all into the channel. On dispatch: `<-pool` to acquire, defer `pool <-` to return. Pool size is configurable per-integration via manifest or global default. Eliminates serialization for concurrent lifecycle calls across a branch.

### Parallel branch dispatch

`ScanBranch` and `CalibrateBranch` currently iterate resources serially. Both dispatch in parallel with goroutines, collecting results in original order:

```go
results := make([]ResourceScanResult, len(resources))
var wg sync.WaitGroup
for i, r := range resources {
    wg.Add(1)
    go func(i int, r models.Resource) {
        defer wg.Done()
        results[i] = sc.scanResource(ctx, r, lb)
    }(i, r)
}
wg.Wait()
```

Transponder dispatch runs in a separate parallel pass after resources (resources may produce context that transponders depend on, so they run second, also in parallel among themselves).

### External plugin directory

On startup, scan `~/.orbiter/integrations/` for subdirectories containing `manifest.toml` + `<brand>.wasm`. Load each with the same `wasm.Load()` path used for bundled integrations. Manifest format is identical — there is no distinction between bundled and external integrations at runtime.

The scan runs after bundled `init()` registration. An external integration with the same `(role, brand)` as a bundled one overrides it (user-local takes precedence).

## Files Created or Modified

**`internal/integrations/types.go`**
- Add `Entity` interface
- `ResolvedContext.Self` → `Entity`
- `StateReport`: add `NeedsInput []InputRequest`
- `ResolvedContext`: add `Responses map[string]string`
- Add `InputRequest` struct

**`internal/integrations/manifest.go`** (new — split from types.go)
- `Manifest`, `ManifestIntegration`, `ManifestDetection`, `ManifestDependencies`
- `ManifestIntegration`: replace `Type string` with `Roles []string`
- Add `ManifestConfigField`, `ManifestConfig`, `ManifestCommands`
- Add `RoleType(role string) string` — static role→type lookup

**`internal/integrations/roles.go`**
- Add `RoleTypes map[string]string` mapping each role constant to `IntegrationTypeResource` or `IntegrationTypeTransponder`
- Remove `IntegrationTypeResource` / `IntegrationTypeTransponder` export from types.go (consolidate here)

**`internal/models/transponder.go`**
- Add `Config string` field
- Remove `Location string` field (folded into Config)

**`internal/migrations/`**
- New migration: drop `location` from `transponders`, add `config TEXT NOT NULL DEFAULT '{}'`, copy existing location values

**`internal/wasm/runtime.go`** (new)
- Shared `wazero.Runtime` singleton
- `init()` creates and holds it for the process lifetime

**`internal/wasm/host.go`**
- Use shared runtime from `runtime.go` instead of `newRuntime(ctx)` per integration
- `run_command`: enforce allowlist from manifest, apply timeout

**`internal/wasm/loader.go`**
- Replace `sync.Mutex` + single `api.Module` with `chan api.Module` pool
- `Load()`: compile once, instantiate pool
- `invoke()`: acquire/release from pool

**`internal/starchart/lifecycle.go`**
- Add `TransponderScanResult`, `TransponderCalibrateResult`
- `BranchScanResult`, `BranchCalibrateResult`: add `Transponders` slice
- `ScanBranch`, `CalibrateBranch`: parallel dispatch + transponder pass
- Add `scanTransponder()`, `calibrateTransponder()`

**`integrations/bundle.go`**
- Register WASM under each role from `manifest.roles` (loop, not single call)
- Load external integrations from `~/.orbiter/integrations/` after bundled

**`integrations/golang/manifest.toml`**, **`integrations/git/manifest.toml`**
- Update to new format (`roles = [...]`, remove `type`, add `[commands]`, add `[[config.fields]]` if applicable)

## Out of Scope

- `Detect` dispatch from `planet init` (Phase 5)
- `agent` transponder role implementation
- `vault` transponder role implementation  
- WASM module signing or integrity verification
- Integration versioning or upgrade paths
- Windows `where` fallback for `which` in tool integrations
