# StateReport Config Contract Migration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish `Config map[string]any` as the sole contract between Orbiter and integrations by removing integration-specific named fields from `StateReport` and migrating all 21 existing integrations to put their output in `Config` instead.

**Architecture:** `StateReport` currently mixes two concerns: Orbiter's own state tracking (`Present`, `Reachable`, `Error`, `NeedsInput`, `Observations`) and integration-specific output that Orbiter just passes through or displays (`BinaryPath`, `InstallDir`, `InPath`, `Manager`, `Exports`). The migration moves the second group into `Config map[string]any` using canonical keys. Orbiter reads specific keys it acts on (`exports`, `write_files`); everything else is available for display or tooling. Integrations are responsible for translating their domain knowledge into these canonical keys — that translation is the purpose of an integration module.

**Tech Stack:** Go 1.23+, TinyGo, AssemblyScript, Rust (wasm32-unknown-unknown), Zig 0.16, C/wasi-sdk.

## Global Constraints

- No new external dependencies.
- After migration `StateReport` contains exactly: `Present`, `Reachable`, `Error`, `NeedsInput`, `Observations`, `Config`. No other fields.
- Canonical `Config` keys (Orbiter acts on these):
  - `exports` — `map[string]string`, env vars the shell hook sets in the current session
  - `write_files` — `map[string]string`, path→content pairs the host writes to disk (export role only)
- Informational `Config` keys (Orbiter stores/displays, does not act on):
  - `binary_path` — string
  - `install_dir` — string
  - `manager` — string
  - `in_path` — bool
- All existing tests must pass after each task.
- Each integration's WASM must be rebuilt after its source is updated. Build commands are per-language (see task steps).
- The host code that reads `BinaryPath`, `InstallDir`, `InPath`, `Manager`, `Exports` as named fields must be updated to read from `Config` in the same task that removes those fields from the struct.

---

## File Map

| File | Change |
| --- | --- |
| `internal/integrations/types.go` | Remove named fields; add canonical key constants |
| `internal/starchart/lifecycle.go` | Update all reads of removed fields to use `Config` |
| `internal/commands/executor.go` | Update any reads of removed fields to use `Config` |
| `integrations/golang/guest/main.go` | Move output to Config |
| `integrations/node/guest/main.go` | Move output to Config |
| `integrations/make/guest/main.go` | Move output to Config |
| `integrations/dotenv/guest/main.go` | Move output to Config |
| `integrations/nvm/assembly/index.ts` | Move output to Config |
| `integrations/just/assembly/index.ts` | Move output to Config |
| `integrations/shell/assembly/index.ts` | Move output to Config |
| `integrations/git/src/lib.rs` | Move output to Config |
| `integrations/python/src/lib.rs` | Move output to Config |
| `integrations/rust/src/lib.rs` | Move output to Config |
| `integrations/brew/src/lib.rs` | Move output to Config |
| `integrations/uv/src/lib.rs` | Move output to Config |
| `integrations/rustup/src/lib.rs` | Move output to Config |
| `integrations/docker/src/lib.rs` | Move output to Config |
| `integrations/macos/src/lib.rs` | Move output to Config |
| `integrations/onepassword/src/lib.rs` | Move output to Config |
| `integrations/ssh/src/lib.rs` | Move output to Config |
| `integrations/github/src/lib.rs` | Move output to Config |
| `integrations/asdf/src/main.zig` | Move output to Config |
| `integrations/local/src/main.zig` | Move output to Config |
| `integrations/vscode/src/vscode.c` | Move output to Config |

---

### Task 1: Strip StateReport and update host reads

**Files:**

- Modify: `internal/integrations/types.go`
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/commands/executor.go` (if it reads named fields)

**Interfaces:**

- Produces: `StateReport` with only `Present`, `Reachable`, `Error`, `NeedsInput`, `Observations`, `Config`
- Produces: `ConfigKeyBinaryPath`, `ConfigKeyInstallDir`, `ConfigKeyManager`, `ConfigKeyInPath`, `ConfigKeyExports` string constants for canonical key names
- Consumes: existing host reads of `report.BinaryPath`, `report.Manager`, etc. — all must be migrated to `report.Config[ConfigKeyBinaryPath]` etc.

- [ ] **Step 1: Read the current StateReport and all host-side reads**

Before writing any code, read these files to understand every place the removed fields are referenced:

```bash
grep -rn "\.BinaryPath\|\.InstallDir\|\.InPath\|\.Manager\b\|\.Exports\b" \
    internal/starchart/ internal/commands/ internal/output/ --include="*.go"
```

Record every callsite — you will fix all of them in this task.

- [ ] **Step 2: Write failing tests**

Add to `internal/integrations/types_test.go` (create if absent):

```go
package integrations_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/stretchr/testify/assert"
)

func TestStateReport_OnlyContractFields(t *testing.T) {
    // Compile-time proof: construct StateReport using only the allowed fields.
    // If a removed field still exists this will not compile.
    _ = integrations.StateReport{
        Present:      true,
        Reachable:    true,
        Error:        "e",
        Observations: []string{"o"},
        Config:       map[string]any{"binary_path": "/usr/bin/foo"},
    }
}

func TestStateReport_ConfigKeys(t *testing.T) {
    assert.Equal(t, "binary_path", integrations.ConfigKeyBinaryPath)
    assert.Equal(t, "install_dir", integrations.ConfigKeyInstallDir)
    assert.Equal(t, "manager",     integrations.ConfigKeyManager)
    assert.Equal(t, "in_path",     integrations.ConfigKeyInPath)
    assert.Equal(t, "exports",     integrations.ConfigKeyExports)
    assert.Equal(t, "write_files", integrations.ConfigKeyWriteFiles)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/integrations/... -run "TestStateReport" -v
```

Expected: FAIL — `StateReport` still has the old fields; constants undefined.

- [ ] **Step 4: Update types.go**

Replace the `StateReport` struct and add canonical key constants. Read the current file first to get the exact existing struct, then replace:

```go
// Canonical Config keys — integrations write these; Orbiter reads them.
const (
    ConfigKeyBinaryPath = "binary_path"
    ConfigKeyInstallDir = "install_dir"
    ConfigKeyManager    = "manager"
    ConfigKeyInPath     = "in_path"
    ConfigKeyExports    = "exports"
    ConfigKeyWriteFiles = "write_files"
)

// StateReport is returned by Init, Scan, and Calibrate.
// Present and Reachable are the only fields Orbiter acts on for lifecycle decisions.
// All integration-specific output goes in Config using the canonical keys above.
type StateReport struct {
    Present      bool              `json:"present"`
    Reachable    bool              `json:"reachable"`
    Error        string            `json:"error,omitempty"`
    NeedsInput   []InputRequest    `json:"needs_input,omitempty"`
    Observations []string          `json:"observations,omitempty"`
    Config       map[string]any    `json:"config,omitempty"`
}
```

- [ ] **Step 5: Fix all host-side reads**

For every callsite found in Step 1, replace field access with Config lookup. Helper pattern:

```go
func configString(r integrations.StateReport, key string) string {
    v, _ := r.Config[key].(string)
    return v
}

func configBool(r integrations.StateReport, key string) bool {
    v, _ := r.Config[key].(bool)
    return v
}

func configStringMap(r integrations.StateReport, key string) map[string]string {
    raw, ok := r.Config[key]
    if !ok {
        return nil
    }
    // Config values from WASM JSON decode as map[string]any
    if m, ok := raw.(map[string]any); ok {
        out := make(map[string]string, len(m))
        for k, v := range m {
            if s, ok := v.(string); ok {
                out[k] = s
            }
        }
        return out
    }
    // Config values set directly in Go tests decode as map[string]string
    if m, ok := raw.(map[string]string); ok {
        return m
    }
    return nil
}
```

Replace every `report.BinaryPath` → `configString(report, integrations.ConfigKeyBinaryPath)`, `report.Manager` → `configString(report, integrations.ConfigKeyManager)`, `report.InPath` → `configBool(report, integrations.ConfigKeyInPath)`, `report.Exports` → `configStringMap(report, integrations.ConfigKeyExports)`.

Also update `writeConfigFiles` in `lifecycle.go` to use `configStringMap(report, integrations.ConfigKeyWriteFiles)`.

- [ ] **Step 6: Run targeted tests**

```bash
go test ./internal/integrations/... -run "TestStateReport" -v
```

Expected: PASS

- [ ] **Step 7: Run full Go test suite**

```bash
go test ./...
```

Expected: all pass. The integration WASM tests will fail because WASM guests still output the old field names — that is expected and will be fixed in later tasks.

- [ ] **Step 8: Commit**

```bash
git add internal/integrations/types.go \
        internal/starchart/lifecycle.go \
        internal/commands/executor.go
git commit -m "feat: strip StateReport to contract fields; add canonical Config key constants"
```

---

### Task 2: Migrate TinyGo integrations (golang, node, make, dotenv)

**Files:**

- Modify: `integrations/golang/guest/main.go`
- Modify: `integrations/node/guest/main.go`
- Modify: `integrations/make/guest/main.go`
- Modify: `integrations/dotenv/guest/main.go`

**Pattern:** In each TinyGo guest, the `writeState` helper currently emits a flat JSON object with `binary_path`, `manager`, `in_path`, etc. as top-level keys. Change it to nest those under a `"config"` key:

Before:

```json
{"present":true,"reachable":true,"in_path":true,"manager":"system","binary_path":"/usr/bin/go"}
```

After:

```json
{"present":true,"reachable":true,"config":{"binary_path":"/usr/bin/go","manager":"system","in_path":true}}
```

The `writeState` signature and all call sites in each file change. `Exports` (if present) moves to `config.exports`.

- [ ] **Step 1: Update golang/guest/main.go**

Read the current file. Locate `writeState`. Replace the flat JSON builder with one that nests integration-specific keys under `"config"`:

```go
func writeState(present, reachable bool, cfg map[string]any, errMsg string, observations []string) {
    buf := append(append([]byte(`{"present":`), boolBytes(present)...), `,"reachable":`...)
    buf = append(buf, boolBytes(reachable)...)
    if errMsg != "" {
        buf = append(buf, `,"error":`...)
        buf = append(buf, jsonBytes(errMsg)...)
    }
    if len(observations) > 0 {
        buf = append(buf, `,"observations":[`...)
        for i, o := range observations {
            if i > 0 {
                buf = append(buf, ',')
            }
            buf = append(buf, jsonBytes(o)...)
        }
        buf = append(buf, ']')
    }
    if len(cfg) > 0 {
        buf = append(buf, `,"config":{`...)
        first := true
        for k, v := range cfg {
            if !first {
                buf = append(buf, ',')
            }
            first = false
            buf = append(buf, jsonBytes(k)...)
            buf = append(buf, ':')
            switch val := v.(type) {
            case string:
                buf = append(buf, jsonBytes(val)...)
            case bool:
                buf = append(buf, boolBytes(val)...)
            }
        }
        buf = append(buf, '}')
    }
    writeRaw(append(buf, '}'))
}
```

Update all `writeState` call sites in the file to pass a `map[string]any` for config instead of positional `binaryPath`, `manager`, `inPath` args.

Build: `cd integrations/golang && tinygo build -o golang.wasm -target=wasm-unknown ./guest/`

- [ ] **Step 2: Update node/guest/main.go — same pattern**

Read, update `writeState`, update call sites, rebuild:

```bash
cd integrations/node && tinygo build -o node.wasm -target=wasm-unknown ./guest/
```

- [ ] **Step 3: Update make/guest/main.go — same pattern**

Read, update `writeState`, update call sites, rebuild:

```bash
cd integrations/make && tinygo build -o make.wasm -target=wasm-unknown ./guest/
```

- [ ] **Step 4: Update dotenv/guest/main.go — same pattern**

Read, update `writeState`, update call sites, rebuild:

```bash
cd integrations/dotenv && tinygo build -o dotenv.wasm -target=wasm-unknown ./guest/
```

- [ ] **Step 5: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add integrations/golang/ integrations/node/ integrations/make/ integrations/dotenv/
git commit -m "feat: migrate TinyGo integrations to Config contract"
```

---

### Task 3: Migrate AssemblyScript integrations (nvm, just, shell)

**Files:**

- Modify: `integrations/nvm/assembly/index.ts`
- Modify: `integrations/just/assembly/index.ts`
- Modify: `integrations/shell/assembly/index.ts`

**Pattern:** Same as Task 2 but in TypeScript. The `writeState` helper currently builds a flat JSON object via `assemblyscript-json`. Change it to nest integration-specific keys under `config`:

```typescript
function writeState(
  present: bool, reachable: bool,
  cfg: JSON.Obj | null,
  errMsg: string, observations: string[]
): void {
  const obj = new JSON.Obj();
  obj.set("present", present);
  obj.set("reachable", reachable);
  if (errMsg.length > 0) obj.set("error", errMsg);
  if (observations.length > 0) {
    const arr = new JSON.Arr();
    for (let i = 0; i < observations.length; i++) {
      arr.push(new JSON.Str(observations[i]));
    }
    obj.set("observations", arr);
  }
  if (cfg != null) obj.set("config", cfg);
  writeStr(obj.stringify());
}
```

Call sites build a `JSON.Obj` for config and pass it in. Any `exports` map moves to `cfg.set("exports", ...)`.

- [ ] **Step 1: Update nvm/assembly/index.ts**

Read the current file. Update `writeState` signature. Update all call sites. Rebuild:

```bash
cd integrations/nvm && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

- [ ] **Step 2: Update just/assembly/index.ts — same pattern**

Read, update, rebuild:

```bash
cd integrations/just && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

- [ ] **Step 3: Update shell/assembly/index.ts — same pattern**

Read, update, rebuild:

```bash
cd integrations/shell && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

- [ ] **Step 4: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add integrations/nvm/ integrations/just/ integrations/shell/
git commit -m "feat: migrate AssemblyScript integrations to Config contract"
```

---

### Task 4: Migrate Rust integrations batch 1 (git, python, rust, brew, uv, rustup)

**Files:**

- Modify: `integrations/git/src/lib.rs`
- Modify: `integrations/python/src/lib.rs`
- Modify: `integrations/rust/src/lib.rs`
- Modify: `integrations/brew/src/lib.rs`
- Modify: `integrations/uv/src/lib.rs`
- Modify: `integrations/rustup/src/lib.rs`

**Pattern:** The Rust `StateReport` struct currently has named fields. Change it to:

```rust
use std::collections::HashMap;

#[derive(Serialize, Default)]
struct StateReport {
    present: bool,
    reachable: bool,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    error: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    observations: Vec<String>,
    #[serde(skip_serializing_if = "HashMap::is_empty", default)]
    config: HashMap<String, serde_json::Value>,
}
```

Each integration builds its `config` map before calling `write_state`. For example:

```rust
let mut config = HashMap::new();
config.insert("binary_path".to_string(), serde_json::Value::String(binary_path));
config.insert("manager".to_string(), serde_json::Value::String("system".to_string()));
config.insert("in_path".to_string(), serde_json::Value::Bool(true));
write_state(StateReport { present: true, reachable: true, config, ..Default::default() });
```

- [ ] **Step 1: Update git/src/lib.rs**

Read the current file. Replace `StateReport` struct. Update all `write_state` call sites to build a config map. Rebuild:

```bash
cd integrations/git && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/git.wasm .
```

- [ ] **Step 2: Update python/src/lib.rs — same pattern**

Rebuild: `cd integrations/python && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/python.wasm .`

- [ ] **Step 3: Update rust/src/lib.rs — same pattern**

Rebuild: `cd integrations/rust && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/rust.wasm .`

- [ ] **Step 4: Update brew/src/lib.rs — same pattern**

Rebuild: `cd integrations/brew && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/brew.wasm .`

- [ ] **Step 5: Update uv/src/lib.rs — same pattern**

Rebuild: `cd integrations/uv && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/uv.wasm .`

- [ ] **Step 6: Update rustup/src/lib.rs — same pattern**

Rebuild: `cd integrations/rustup && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/rustup.wasm .`

- [ ] **Step 7: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add integrations/git/ integrations/python/ integrations/rust/ \
        integrations/brew/ integrations/uv/ integrations/rustup/
git commit -m "feat: migrate Rust integrations batch 1 to Config contract"
```

---

### Task 5: Migrate Rust integrations batch 2 (docker, macos, onepassword, ssh, github)

**Files:**

- Modify: `integrations/docker/src/lib.rs`
- Modify: `integrations/macos/src/lib.rs`
- Modify: `integrations/onepassword/src/lib.rs`
- Modify: `integrations/ssh/src/lib.rs`
- Modify: `integrations/github/src/lib.rs`

Same pattern as Task 4. Each integration already has a `StateReport` struct with named fields — replace with the Config map pattern.

- [ ] **Step 1: Update docker/src/lib.rs**

Rebuild: `cd integrations/docker && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/docker.wasm .`

- [ ] **Step 2: Update macos/src/lib.rs — same pattern**

Rebuild: `cd integrations/macos && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/macos.wasm .`

- [ ] **Step 3: Update onepassword/src/lib.rs — same pattern**

Rebuild: `cd integrations/onepassword && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/onepassword.wasm .`

- [ ] **Step 4: Update ssh/src/lib.rs — same pattern**

Rebuild: `cd integrations/ssh && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/ssh.wasm .`

- [ ] **Step 5: Update github/src/lib.rs — same pattern**

Rebuild: `cd integrations/github && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/github.wasm .`

- [ ] **Step 6: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add integrations/docker/ integrations/macos/ integrations/onepassword/ \
        integrations/ssh/ integrations/github/
git commit -m "feat: migrate Rust integrations batch 2 to Config contract"
```

---

### Task 6: Migrate Zig and C integrations (asdf, local, vscode)

**Files:**

- Modify: `integrations/asdf/src/main.zig`
- Modify: `integrations/local/src/main.zig`
- Modify: `integrations/vscode/src/vscode.c`

**Zig pattern:** The Zig integration outputs hand-built JSON. Move named fields under a `"config"` key in the output JSON string, following the same before/after shape as the TinyGo pattern.

**C pattern:** Same — hand-built JSON output, move named fields under `"config"`.

- [ ] **Step 1: Update asdf/src/main.zig**

Read the current file. Find the JSON output builder. Wrap `binary_path`, `manager`, `in_path` under a `config` object. Rebuild:

```bash
cd integrations/asdf && zig build -Doptimize=ReleaseSmall && cp zig-out/lib/asdf.wasm .
```

(Check the actual Justfile target if the build command differs.)

- [ ] **Step 2: Update local/src/main.zig — same pattern**

Rebuild using the Justfile target for `local`.

- [ ] **Step 3: Update vscode/src/vscode.c — same pattern**

Rebuild using the Justfile target for `vscode`.

- [ ] **Step 4: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add integrations/asdf/ integrations/local/ integrations/vscode/
git commit -m "feat: migrate Zig and C integrations to Config contract"
```
