# New Roles: Export + Multiplexer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `export` and `multiplexer` resource roles to Orbiter's taxonomy and provide the first integration for each — `integrations/json/` (AssemblyScript) that writes the resolved branch context to a JSON file, and `integrations/tmux/` (Rust) that propagates context into a tmux session.

**Architecture:** Both roles follow the same integration pattern as every other integration in this project — a `integrations/<brand>/` directory with `manifest.toml`, language-specific source, and a pre-built `<brand>.wasm`. The roles are registered in `roles.go` and ordered in `lifecycle.go`. `Config map[string]any` on `StateReport` is the contract between Orbiter and integrations — integrations translate their specific output into canonical Config keys that Orbiter acts on. New integrations in this plan use Config from the start; existing integrations will be migrated in a separate plan. The two canonical Config keys introduced here are `write_files` (path→content map the host writes to disk for `export`-role resources) and `exports` (key→value env vars the shell hook sets). The tmux multiplexer calls `tmux set-environment` via `run_command`; it returns healthy when `tmux` is not in PATH (no-op outside a tmux session).

**Tech Stack:** Go 1.23+, AssemblyScript (`assemblyscript-json`), Rust (wasm32-unknown-unknown, serde/serde_json).

## Global Constraints

- No new external Go dependencies.
- Each integration lives at `integrations/<brand>/` — never in `internal/`.
- The `integrations/bundle.go` embed directive must be updated to include `json/json.wasm json/manifest.toml tmux/tmux.wasm tmux/manifest.toml`.
- **No new named fields on `StateReport`**. Integration-specific output goes in `StateReport.Config map[string]any`.
- Canonical Config keys Orbiter acts on (established by this plan):
  - `write_files` — `map[string]string`, keys are absolute paths, values are file content. Host writes these after calibrate for `export`-role resources.
  - `exports` — `map[string]string`, env var key→value pairs. Shell hook sets these in the current session.
- Export integrations must never include resolved secret values in `write_files` content. `ResolvedTransponder` carries only metadata (role, brand, config references), not resolved values — passing through the `ResolvedContext` JSON as-is is safe.
- The tmux integration must declare `"tmux"` in its manifest `[commands] allowed` list.
- Build commands: AssemblyScript → `cd integrations/json && npm install && ./node_modules/.bin/asc assembly/index.ts --target release`; Rust → `cd integrations/tmux && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/tmux.wasm .`
- All existing tests must pass after each task.

---

## File Map

| File | Change |
| --- | --- |
| `internal/integrations/roles.go` | Add `ResourceRoleExport`, `ResourceRoleMultiplexer`; update `RoleTypes` |
| `internal/starchart/lifecycle.go` | Add both new roles to `resourceRoleOrder`; read `Config["write_files"]` after export-role calibrate |
| `integrations/json/manifest.toml` | New |
| `integrations/json/package.json` | New — AssemblyScript npm config |
| `integrations/json/asconfig.json` | New — asc build config |
| `integrations/json/assembly/index.ts` | New — AssemblyScript WASM guest |
| `integrations/json/json.wasm` | Pre-built artifact (implementer builds) |
| `integrations/tmux/manifest.toml` | New |
| `integrations/tmux/Cargo.toml` | New |
| `integrations/tmux/src/lib.rs` | New — Rust WASM guest |
| `integrations/tmux/src/host.rs` | New — Rust FFI helpers (copy from any existing Rust integration) |
| `integrations/tmux/tmux.wasm` | Pre-built artifact (implementer builds) |
| `integrations/bundle.go` | Add json + tmux to `//go:embed` directive |

---

### Task 1: Register export and multiplexer roles + Config-based file-write hook

**Files:**

- Modify: `internal/integrations/roles.go`
- Modify: `internal/starchart/lifecycle.go`
- Test: `internal/integrations/roles_test.go` (create if absent)
- Test: `internal/starchart/lifecycle_test.go`

**Interfaces:**

- Produces: `ResourceRoleExport = "export"`, `ResourceRoleMultiplexer = "multiplexer"` constants
- Produces: lifecycle `resourceRoleOrder` slice includes both new roles
- Produces: lifecycle reads `StateReport.Config["write_files"]` after export-role calibrate and writes each path→content pair to disk

- [ ] **Step 1: Write failing tests**

Add to `internal/integrations/roles_test.go` (create if absent):

```go
package integrations_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/stretchr/testify/assert"
)

func TestRoleType_Export(t *testing.T) {
    assert.Equal(t, integrations.IntegrationTypeResource, integrations.RoleType("export"))
}

func TestRoleType_Multiplexer(t *testing.T) {
    assert.Equal(t, integrations.IntegrationTypeResource, integrations.RoleType("multiplexer"))
}
```

Add to `internal/starchart/lifecycle_test.go`:

```go
func TestResourceRoleOrder_ContainsExportAndMultiplexer(t *testing.T) {
    sc := openTestSC(t)
    ctx := context.Background()

    g, _ := sc.CreateGalaxy(ctx, "acme")
    p, _ := sc.CreatePlanet(ctx, "app", g.ID, "")

    expRes, _ := sc.CreateResource(ctx, "ctx-export", "export", "json", "[]", `{"path":"/tmp/test.json"}`)
    _, _ = sc.Attach(ctx, expRes.Name, p.Name)

    muxRes, _ := sc.CreateResource(ctx, "mux", "multiplexer", "tmux", "[]", `{}`)
    _, _ = sc.Attach(ctx, muxRes.Name, p.Name)

    _, err := sc.ScanBranch(ctx, p.ID)
    assert.NoError(t, err, "ScanBranch must not fail with export/multiplexer resources")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/integrations/... ./internal/starchart/... -run "TestRoleType_Export|TestRoleType_Multiplexer|TestResourceRoleOrder_Contains" -v
```

Expected: FAIL — constants undefined, `RoleType("export")` returns `""`.

- [ ] **Step 3: Add role constants and RoleTypes entries**

In `internal/integrations/roles.go`, add to the resource role constants block:

```go
const (
    ResourceRoleManager     = "manager"
    ResourceRoleRuntime     = "runtime"
    ResourceRoleTool        = "tool"
    ResourceRoleRemote      = "remote"
    ResourceRoleShell       = "shell"
    ResourceRoleExport      = "export"
    ResourceRoleMultiplexer = "multiplexer"
)
```

Add to `RoleTypes`:

```go
var RoleTypes = map[string]string{
    ResourceRoleManager:     IntegrationTypeResource,
    ResourceRoleRuntime:     IntegrationTypeResource,
    ResourceRoleTool:        IntegrationTypeResource,
    ResourceRoleRemote:      IntegrationTypeResource,
    ResourceRoleShell:       IntegrationTypeResource,
    ResourceRoleExport:      IntegrationTypeResource,
    ResourceRoleMultiplexer: IntegrationTypeResource,
    TransponderRoleFile:     IntegrationTypeTransponder,
    TransponderRoleEnv:      IntegrationTypeTransponder,
    TransponderRoleKeychain: IntegrationTypeTransponder,
    TransponderRoleVault:    IntegrationTypeTransponder,
    TransponderRoleAgent:    IntegrationTypeTransponder,
}
```

- [ ] **Step 4: Add roles to lifecycle dispatch order and Config-based file-write hook**

In `internal/starchart/lifecycle.go`, update `resourceRoleOrder`. Export runs last (after tools, since it reads their state); multiplexer runs after export:

```go
var resourceRoleOrder = []string{
    integrations.ResourceRoleShell,
    integrations.ResourceRoleManager,
    integrations.ResourceRoleRuntime,
    integrations.ResourceRoleRemote,
    integrations.ResourceRoleTool,
    integrations.ResourceRoleExport,
    integrations.ResourceRoleMultiplexer,
}
```

Find the section in `lifecycle.go` where calibrate results are handled after a calibrate dispatch. Add a helper that reads `StateReport.Config["write_files"]` and writes the files. Call it only for `export`-role resources:

```go
// writeConfigFiles writes path→content pairs declared in StateReport.Config["write_files"]
// by an export-role integration.
func writeConfigFiles(report integrations.StateReport) {
    raw, ok := report.Config["write_files"]
    if !ok {
        return
    }
    files, ok := raw.(map[string]any)
    if !ok {
        return
    }
    for path, v := range files {
        if content, ok := v.(string); ok {
            _ = os.WriteFile(path, []byte(content), 0644)
        }
    }
}
```

In the calibrate loop, after the dispatch returns a `StateReport`, add:

```go
if resource.Role == integrations.ResourceRoleExport {
    writeConfigFiles(report)
}
```

- [ ] **Step 5: Run targeted tests**

```bash
go test ./internal/integrations/... ./internal/starchart/... -run "TestRoleType_Export|TestRoleType_Multiplexer|TestResourceRoleOrder_Contains" -v
```

Expected: PASS

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
```

Expected: all pass (pre-existing `integrations` package failures for nvm/shell quarantined for `sh` are known — do not count against this task).

- [ ] **Step 7: Commit**

```bash
git add internal/integrations/roles.go \
        internal/starchart/lifecycle.go \
        internal/integrations/roles_test.go \
        internal/starchart/lifecycle_test.go
git commit -m "feat: add export and multiplexer resource roles with Config-based file-write hook"
```

---

### Task 2: export/json AssemblyScript integration

**Files:**

- Create: `integrations/json/manifest.toml`
- Create: `integrations/json/package.json`
- Create: `integrations/json/asconfig.json`
- Create: `integrations/json/assembly/index.ts`
- Build: `integrations/json/json.wasm`
- Modify: `integrations/bundle.go`
- Modify: `Justfile`

**Behavior:** On `calibrate`, reads the `ResolvedContext` JSON input, parses `self.config` to get the output path (`{"path":"/output/context.json"}`), and returns a `StateReport` with `export_files: {path: input_json}`. The host (Task 1) then writes the file. On `scan`, returns `present: true`. On `detect`, returns `{"detected":false}` — export integrations are attached explicitly, not auto-detected.

**Interfaces:**

- Produces: `export/json` registered in `integrations.Default` (via bundle.go embed + CatalogEntries)

- [ ] **Step 1: Create manifest.toml**

Create `integrations/json/manifest.toml`:

```toml
[integration]
brand = "json"
name = "JSON Export"
description = "Writes the resolved branch context to a JSON file (no secrets)"
roles = ["export"]

[detection]
# Not auto-detected — attach explicitly to a planet.

[runtime]
pool_size = 2
input_buffer_kb = 16
output_buffer_kb = 64
```

- [ ] **Step 2: Create package.json and asconfig.json**

Create `integrations/json/package.json`:

```json
{
  "name": "json-integration",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "asbuild": "asc assembly/index.ts --target release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

Create `integrations/json/asconfig.json`:

```json
{
  "targets": {
    "release": {
      "outFile": "json.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 2,
      "noAssert": true
    }
  },
  "options": {
    "exportRuntime": false
  }
}
```

- [ ] **Step 3: Write failing catalog test**

Add to `integrations/catalog_test.go` (or create it if absent following the pattern of `integrations/e2e_test.go`):

```go
func TestCatalog_ContainsJSON(t *testing.T) {
    entries := integrations.CatalogEntries()
    for _, e := range entries {
        if e.Brand == "json" {
            require.Contains(t, e.Roles, "export")
            return
        }
    }
    t.Fatal("json integration not found in catalog")
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./integrations/... -run "TestCatalog_ContainsJSON" -v
```

Expected: FAIL — json not in catalog.

- [ ] **Step 5: Implement assembly/index.ts**

The pattern mirrors `integrations/nvm/assembly/index.ts` — same FFI declarations, same `readInput`/`writeStr`/`runCmd`/`writeState` helpers. The `assemblyscript-json` library handles all JSON parsing and building natively; no hand-rolling required.

Create `integrations/json/assembly/index.ts`:

```typescript
import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

@external("orbiter", "run_command")
declare function run_command(specPtr: i32, specLen: i32, outPtr: i32, outMax: i32): i32;

const BUF_SIZE: i32 = 131072; // 128 KB — larger than default for JSON payloads

function readInput(): Uint8Array {
  const buf = new Uint8Array(BUF_SIZE);
  const n = read_input(buf.dataStart, buf.byteLength);
  return buf.slice(0, n);
}

function writeStr(s: string): void {
  const encoded = String.UTF8.encode(s, false);
  const buf = Uint8Array.wrap(encoded);
  write_output(buf.dataStart, buf.byteLength);
}

// runCmd is declared to keep run_command live in the WASM binary.
function runCmd(cmd: string, args: string[]): string {
  const spec = new JSON.Obj();
  spec.set("cmd", cmd);
  const argsArr = new JSON.Arr();
  for (let i = 0; i < args.length; i++) {
    argsArr.push(new JSON.Str(args[i]));
  }
  spec.set("args", argsArr);
  const specStr = spec.stringify();
  const specEncoded = String.UTF8.encode(specStr, false);
  const specBuf = Uint8Array.wrap(specEncoded);
  const outBuf = new Uint8Array(BUF_SIZE);
  const n = run_command(specBuf.dataStart, specBuf.byteLength, outBuf.dataStart, outBuf.byteLength);
  return String.UTF8.decode(outBuf.buffer.slice(0, n)).trimEnd();
}

// extractPath parses path out of a config JSON string like {"path":"/output/context.json"}.
function extractPath(configStr: string): string {
  const parsed = JSON.parse(configStr);
  if (!parsed.isObj) return "";
  const cfg = parsed as JSON.Obj;
  const pathVal = cfg.get("path");
  if (pathVal == null || !pathVal.isString) return "";
  return (pathVal as JSON.Str).valueOf();
}

export function detect(): void {
  writeStr('{"detected":false}');
}

export function initialize(): void {
  calibrate();
}

export function scan(): void {
  // Keep run_command live.
  runCmd("which", ["ls"]);
  writeStr('{"present":true,"reachable":true,"manager":""}');
}

export function calibrate(): void {
  const inputBytes = readInput();
  const inputStr = String.UTF8.decode(inputBytes.buffer);

  // Keep run_command live.
  runCmd("which", ["ls"]);

  const parsed = JSON.parse(inputStr);
  if (!parsed.isObj) {
    writeStr('{"present":false,"reachable":false,"manager":"","error":"invalid input"}');
    return;
  }

  const ctx = parsed as JSON.Obj;
  let path = "";

  // Navigate: ctx.self.config -> parse -> .path
  const selfVal = ctx.get("self");
  if (selfVal != null && selfVal.isObj) {
    const self = selfVal as JSON.Obj;
    const configVal = self.get("config");
    if (configVal != null && configVal.isString) {
      path = extractPath((configVal as JSON.Str).valueOf());
    }
  }

  if (path.length == 0) {
    writeStr('{"present":false,"reachable":false,"manager":"","error":"no path in self config"}');
    return;
  }

  // Build StateReport with config.write_files: {path: inputStr}
  // ResolvedContext contains no resolved secret values — safe to export as-is.
  const writeFiles = new JSON.Obj();
  writeFiles.set(path, inputStr);

  const config = new JSON.Obj();
  config.set("write_files", writeFiles);

  const report = new JSON.Obj();
  report.set("present", true);
  report.set("reachable", true);
  report.set("config", config);

  writeStr(report.stringify());
}
```

- [ ] **Step 6: Build the WASM**

```bash
cd integrations/json && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

Expected: `json.wasm` created in `integrations/json/`.

- [ ] **Step 7: Add build-integration-json to Justfile**

In `Justfile`, add `build-integration-json` to the `build-integrations` dependency list and add the recipe:

```makefile
build-integration-json:
    cd integrations/json && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

- [ ] **Step 8: Add json to bundle.go embed**

In `integrations/bundle.go`, add `json/json.wasm json/manifest.toml` to the `//go:embed` directive. Append the two new entries to the end of the existing directive line.

- [ ] **Step 9: Run targeted test**

```bash
go test ./integrations/... -run "TestCatalog_ContainsJSON" -v
```

Expected: PASS

- [ ] **Step 10: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 11: Commit**

```bash
git add integrations/json/ integrations/bundle.go Justfile
git commit -m "feat: add export/json AssemblyScript integration"
```

---

### Task 3: multiplexer/tmux Rust integration

**Files:**

- Create: `integrations/tmux/manifest.toml`
- Create: `integrations/tmux/Cargo.toml`
- Create: `integrations/tmux/src/lib.rs`
- Create: `integrations/tmux/src/host.rs`
- Build: `integrations/tmux/tmux.wasm`
- Modify: `integrations/bundle.go`

**Behavior:** On `calibrate`, reads vars from `self.config` (`{"vars":{"KEY":"val"}}`). For each key/value pair, calls `tmux set-environment -g KEY value` via `run_command`. If `tmux` is not in PATH or the config has no vars, returns healthy (no-op). On `scan`, checks if `tmux` is in PATH. On `detect`, returns `{"detected":false}`.

**Interfaces:**

- Produces: `multiplexer/tmux` registered in `integrations.Default`

- [ ] **Step 1: Create manifest.toml**

Create `integrations/tmux/manifest.toml`:

```toml
[integration]
brand = "tmux"
name = "tmux"
description = "Propagates Orbiter context to the tmux session environment"
roles = ["multiplexer"]

[detection]
# Not auto-detected — attach explicitly to a planet.

[commands]
allowed = ["tmux", "which"]
timeout_seconds = 5

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 2: Write failing catalog test**

Add to `integrations/catalog_test.go`:

```go
func TestCatalog_ContainsTmux(t *testing.T) {
    entries := integrations.CatalogEntries()
    for _, e := range entries {
        if e.Brand == "tmux" {
            require.Contains(t, e.Roles, "multiplexer")
            return
        }
    }
    t.Fatal("tmux integration not found in catalog")
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./integrations/... -run "TestCatalog_ContainsTmux" -v
```

Expected: FAIL — tmux not in catalog.

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/tmux/Cargo.toml`:

```toml
[package]
name = "tmux"
version = "0.1.0"
edition = "2021"

[lib]
name = "tmux"
crate-type = ["cdylib"]

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"

[profile.release]
opt-level = "s"
lto = true
strip = true
panic = "abort"
```

- [ ] **Step 5: Create src/host.rs**

Copy `integrations/docker/src/host.rs` verbatim to `integrations/tmux/src/host.rs`. It is identical across all Rust integrations.

- [ ] **Step 6: Implement src/lib.rs**

Create `integrations/tmux/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize, Default)]
struct TmuxConfig {
    #[serde(default)]
    vars: HashMap<String, String>,
}

#[derive(Deserialize)]
struct SelfResource {
    #[serde(default)]
    config: String,
}

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(rename = "self", default)]
    self_res: Option<SelfResource>,
}

#[derive(Serialize, Default)]
struct StateReport {
    present: bool,
    reachable: bool,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    manager: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    error: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    observations: Vec<String>,
}

fn write_state(report: StateReport) {
    host::write_output(&serde_json::to_vec(&report).unwrap_or_default());
}

fn parse_config(ctx: &ResolvedContext) -> TmuxConfig {
    ctx.self_res
        .as_ref()
        .and_then(|s| serde_json::from_str(&s.config).ok())
        .unwrap_or_default()
}

#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    calibrate();
}

#[no_mangle]
pub extern "C" fn scan() {
    let tmux_path = host::run_command("which", &["tmux"]);
    write_state(StateReport {
        present: !tmux_path.is_empty(),
        reachable: !tmux_path.is_empty(),
        manager: "system".to_string(),
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        self_res: None,
    });
    let cfg = parse_config(&ctx);

    if cfg.vars.is_empty() {
        write_state(StateReport {
            present: true,
            reachable: true,
            manager: "system".to_string(),
            ..Default::default()
        });
        return;
    }

    let tmux_path = host::run_command("which", &["tmux"]);
    if tmux_path.is_empty() {
        // tmux not installed or not in PATH — no-op, not an error.
        write_state(StateReport {
            present: true,
            reachable: true,
            manager: "system".to_string(),
            observations: vec!["tmux not in PATH — skipped".to_string()],
            ..Default::default()
        });
        return;
    }

    let mut applied = Vec::new();
    for (key, val) in &cfg.vars {
        let result = host::run_command("tmux", &["set-environment", "-g", key, val]);
        // Ignore errors — user may not be inside a tmux session.
        applied.push(format!("set {}={}", key, result));
    }

    write_state(StateReport {
        present: true,
        reachable: true,
        manager: "system".to_string(),
        observations: applied,
        ..Default::default()
    });
}
```

- [ ] **Step 7: Build the WASM**

```bash
cd integrations/tmux && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/tmux.wasm .
```

Expected: `tmux.wasm` created in `integrations/tmux/`.

- [ ] **Step 8: Add tmux to bundle.go embed**

In `integrations/bundle.go`, append `tmux/tmux.wasm tmux/manifest.toml` to the `//go:embed` directive (alongside the json entries added in Task 2).

- [ ] **Step 9: Run targeted test**

```bash
go test ./integrations/... -run "TestCatalog_ContainsTmux" -v
```

Expected: PASS

- [ ] **Step 10: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 11: Commit**

```bash
git add integrations/tmux/ integrations/bundle.go
git commit -m "feat: add multiplexer/tmux Rust integration"
```
