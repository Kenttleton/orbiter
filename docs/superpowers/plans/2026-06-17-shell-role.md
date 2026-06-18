# Shell Role: filesystem→shell Rename + Hook Command + Shell Integrations

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `filesystem` resource role to `shell`, add an `orbiter hook` command for automatic context detection on directory change, add per-shell WASM integrations with manifest env detection rules, and add PowerShell support.

**Architecture:** The `shell` role replaces `filesystem` — the resource still holds a `{"path":"..."}` config but is now explicitly about shell context entry, not generic filesystem state. Per-shell integrations (bash/zsh/fish/powershell) are AssemblyScript WASM at `integrations/<brand>/`, following the same pattern as all other integrations: `manifest.toml` + `assembly/index.ts` + pre-built `<brand>.wasm`. Each integration also includes a static hook script file (e.g. `hook.bash`) declared in the manifest as a verbose export entry: `exports = [{ hook = "hook.bash", description = "..." }]`. `printShellScript` finds the matching shell integration by detection env rules, calls `manifest.Shell.HookFile()` to get the filename, then reads that file from the embedded bundle FS. The `hook` subcommand is a fast, no-WASM path: resolve CWD → emit neutral directives → exit. The `env/shell` transponder declares `[dependencies.resources] shell = []` to formalize that env var management flows through the shell resource.

**Tech Stack:** Go 1.23+, AssemblyScript (`asc`), `assemblyscript-json`.

## Global Constraints

- Depends on Plan 1 (neutral `Directive` protocol and `ManifestDetection.Env` must already be merged).
- No database migration — old `filesystem` resources will not match after the rename. This is intentional; the codebase has no production users.
- `orbiter hook` must not call any WASM integration — pure Go only, latency budget is <20ms on a cold start.
- The hook shell function must never corrupt `$?` — always save the previous exit status before running and restore it before returning. The Go binary exits non-zero on real failures (DB error, timeout, dependency missing) and writes to stderr; stderr passes through naturally from the subshell so the Captain sees infrastructure errors. Empty stdout + exit 0 is the no-op case (directory not a planet).
- PowerShell script targets pwsh (cross-platform PowerShell 7+), not Windows PowerShell 5.
- Shell WASM integrations follow the exact same `integrations/<brand>/` pattern as all other integrations: `manifest.toml` + `assembly/index.ts` + `package.json` + `asconfig.json` + `<brand>.wasm` + `hook.<ext>`. They are added to `bundle.go`'s `//go:embed` line and loaded via `InstallSelected` like any other WASM integration.
- The hook script is a static file declared as a verbose export entry: `exports = [{ hook = "hook.bash", description = "..." }]`. `printShellScript` calls `manifest.Shell.HookFile()` and reads the file from the embedded bundle FS — no WASM invocation, no `Config["script"]`.

---

## File Map

| File | Change |
|---|---|
| `internal/integrations/roles.go` | Rename `ResourceRoleFilesystem` → `ResourceRoleShell = "shell"`, update `RoleTypes` |
| `internal/starchart/lifecycle.go` | Update `resourceRoleOrder` to use `ResourceRoleShell` |
| `internal/starchart/resolve_cwd.go` | Update SQL query from `role = 'filesystem'` to `role = 'shell'` |
| `internal/starchart/resolve_cwd_test.go` | Update all `"filesystem"` literals to `"shell"` |
| `internal/starchart/lifecycle_test.go` | Update `"filesystem"` literals to `"shell"` |
| `internal/commands/executor.go` | Update Jump role reference; add `Hook` method; add `DEPART` to `Directive.String()` |
| `internal/commands/executor_test.go` | Update Jump role reference; add Hook tests |
| `internal/commands/lifecycle.go` | Add `hook` subcommand |
| `internal/integrations/native/filesystem.go` | Delete — entire `native` package removed |
| `internal/integrations/native/filesystem_test.go` | Delete |
| `integrations/orbiter/shell.go` | New — native `shell/orbiter` integration (directory presence check; no script artifact) |
| `integrations/orbiter/shell_test.go` | New — tests for shell/orbiter |
| `cmd/orbiter/main.go` | Update blank import from `internal/integrations/native` → `integrations/orbiter` |
| `integrations/local/manifest.toml` | Update `roles = ["filesystem"]` → `roles = ["shell"]` |
| `integrations/shell/manifest.toml` | Add `[dependencies.resources] shell = []` (env transponder declares shell dependency) |
| `integrations/bash/manifest.toml` | New — brand=bash, roles=[shell], detection env=[BASH_VERSION], exports=[{hook="hook.bash",...}] |
| `integrations/bash/hook.bash` | New — static bash hook script (PROMPT_COMMAND, $? preservation) |
| `integrations/bash/assembly/index.ts` | New — AssemblyScript: detect only; scan/calibrate return present/reachable |
| `integrations/bash/package.json` | New — same pattern as integrations/nvm/package.json |
| `integrations/bash/asconfig.json` | New — outFile = "bash.wasm" |
| `integrations/bash/bash.wasm` | New — pre-built (built with `just build-integration-bash`) |
| `integrations/zsh/manifest.toml` | New — brand=zsh, roles=[shell], detection env=[ZSH_VERSION], exports=[{hook="hook.zsh",...}] |
| `integrations/zsh/hook.zsh` | New — static zsh hook script (chpwd, $? preservation) |
| `integrations/zsh/assembly/index.ts` | New — AssemblyScript: detect only |
| `integrations/zsh/package.json` | New |
| `integrations/zsh/asconfig.json` | New — outFile = "zsh.wasm" |
| `integrations/zsh/zsh.wasm` | New — pre-built |
| `integrations/fish/manifest.toml` | New — brand=fish, roles=[shell], detection env=[FISH_VERSION], exports=[{hook="hook.fish",...}] |
| `integrations/fish/hook.fish` | New — static fish hook script (--on-variable PWD, $status preservation) |
| `integrations/fish/assembly/index.ts` | New — AssemblyScript: detect only |
| `integrations/fish/package.json` | New |
| `integrations/fish/asconfig.json` | New — outFile = "fish.wasm" |
| `integrations/fish/fish.wasm` | New — pre-built |
| `integrations/powershell/manifest.toml` | New — brand=powershell, roles=[shell], detection env=[PSHOME], exports=[{hook="hook.ps1",...}] |
| `integrations/powershell/hook.ps1` | New — static PowerShell hook script (LocationChangedAction, $LASTEXITCODE preservation) |
| `integrations/powershell/assembly/index.ts` | New — AssemblyScript: detect only |
| `integrations/powershell/package.json` | New |
| `integrations/powershell/asconfig.json` | New — outFile = "powershell.wasm" |
| `integrations/powershell/powershell.wasm` | New — pre-built |
| `integrations/bundle.go` | Add `bash/bash.wasm bash/manifest.toml bash/hook.bash zsh/zsh.wasm zsh/manifest.toml zsh/hook.zsh fish/fish.wasm fish/manifest.toml fish/hook.fish powershell/powershell.wasm powershell/manifest.toml powershell/hook.ps1` to `//go:embed` line |
| `integrations/catalog_test.go` | Add tests asserting bash/zsh/fish/powershell appear in catalog with `shell` role |
| `internal/commands/shell.go` | Rewrite `printShellScript` to use `AllForRole("shell")` + `manifest.Shell.HookFile()` + `bundleFS.ReadFile`; remove old `//go:embed` vars |
| `internal/commands/shell_test.go` | Update tests |
| `internal/commands/shell/orbiter.bash` | Delete (script now lives in `integrations/bash/hook.bash`) |
| `internal/commands/shell/orbiter.zsh` | Delete (script now lives in `integrations/zsh/hook.zsh`) |
| `internal/commands/shell/orbiter.fish` | Delete (script now lives in `integrations/fish/hook.fish`) |
| `Justfile` | Add `build-integration-bash`, `build-integration-zsh`, `build-integration-fish`, `build-integration-powershell` recipes; add to `build-integrations` deps |

---

### Task 1: Rename filesystem → shell throughout + fix manifest alignment

**Files:**
- Modify: `internal/integrations/roles.go`
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/resolve_cwd.go`
- Modify: `internal/starchart/resolve_cwd_test.go`
- Modify: `internal/starchart/lifecycle_test.go`
- Modify: `internal/commands/executor.go` (one reference)
- Delete: `internal/integrations/native/filesystem.go`
- Delete: `internal/integrations/native/filesystem_test.go`
- Create: `integrations/orbiter/shell.go`
- Create: `integrations/orbiter/shell_test.go`
- Modify: `cmd/orbiter/main.go`
- Modify: `integrations/local/manifest.toml`
- Modify: `integrations/shell/manifest.toml`

**Interfaces:**
- Produces: `ResourceRoleShell = "shell"` constant, `shell/orbiter` native integration registered

- [ ] **Step 1: Update roles.go**

In `internal/integrations/roles.go`, replace:
```go
ResourceRoleFilesystem = "filesystem"
```
with:
```go
ResourceRoleShell = "shell"
```

And in `RoleTypes` map, replace:
```go
ResourceRoleFilesystem: IntegrationTypeResource,
```
with:
```go
ResourceRoleShell: IntegrationTypeResource,
```

- [ ] **Step 2: Update lifecycle.go role order**

In `internal/starchart/lifecycle.go`, replace:
```go
integrations.ResourceRoleFilesystem,
```
with:
```go
integrations.ResourceRoleShell,
```

- [ ] **Step 3: Update resolve_cwd.go SQL query**

In `internal/starchart/resolve_cwd.go`, replace the hardcoded role string in the query:
```go
WHERE r.role = 'filesystem' AND r.brand = 'orbiter'
```
with:
```go
WHERE r.role = 'shell' AND r.brand = 'orbiter'
```

Also update the error message:
```go
return models.Alias{}, fmt.Errorf("%w: no shell resource path matches %q", ErrNotFound, cwd)
```

- [ ] **Step 4: Update resolve_cwd_test.go**

In `internal/starchart/resolve_cwd_test.go`, replace all occurrences of `"filesystem"` with `"shell"`:

```go
// Every CreateResource call in this file changes from:
sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", ...)
// to:
sc.CreateResource(ctx, "acme-path", "shell", "orbiter", ...)
```

Also rename the helper:
```go
func shellConfig(t *testing.T, path string) string {  // was: filesystemConfig
    t.Helper()
    b, err := json.Marshal(map[string]string{"path": path})
    if err != nil {
        t.Fatal(err)
    }
    return string(b)
}
```

- [ ] **Step 5: Update lifecycle_test.go**

In `internal/starchart/lifecycle_test.go`, replace:
```go
sc.CreateResource(ctx, "fs", "filesystem", "orbiter", "[]", `{"path":"/tmp"}`)
```
with:
```go
sc.CreateResource(ctx, "fs", "shell", "orbiter", "[]", `{"path":"/tmp"}`)
```

And update the assertion:
```go
assert.Equal(t, "shell", results.Resources[0].Resource.Role, "shell must scan before runtime")
```

- [ ] **Step 6: Update executor.go Jump role reference**

In `internal/commands/executor.go`, replace:
```go
if r.Resource.Role != integrations.ResourceRoleFilesystem {
```
with:
```go
if r.Resource.Role != integrations.ResourceRoleShell {
```

- [ ] **Step 7: Create integrations/orbiter/shell.go (replaces filesystem.go)**

Create `integrations/orbiter/shell.go`:

```go
package orbiter

import (
    "encoding/json"
    "os"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

var shellManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand: "orbiter",
        Roles: []string{integrations.ResourceRoleShell},
    },
}

type shellIntegration struct{}

// NewShell returns a shellIntegration for testing.
func NewShell() integrations.Integration {
    return &shellIntegration{}
}

func (s *shellIntegration) Meta() integrations.Manifest {
    return shellManifest
}

func (s *shellIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}

func (s *shellIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    path := pathFromSelf(ctx)
    if path == "" {
        return integrations.StateReport{Error: "no path in resource config"}
    }
    if err := os.MkdirAll(path, 0755); err != nil {
        return integrations.StateReport{InstallDir: path, Error: err.Error()}
    }
    return integrations.StateReport{Present: true, Reachable: true, InstallDir: path}
}

func (s *shellIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    path := pathFromSelf(ctx)
    if path == "" {
        return integrations.StateReport{Error: "no path in resource config"}
    }
    info, err := os.Stat(path)
    if err != nil {
        return integrations.StateReport{Present: false, InstallDir: path}
    }
    return integrations.StateReport{
        Present:    true,
        Reachable:  info.IsDir(),
        InstallDir: path,
    }
}

func (s *shellIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return s.Init(ctx)
}

func pathFromSelf(ctx integrations.ResolvedContext) string {
    if ctx.Self == nil {
        return ""
    }
    var cfg struct {
        Path string `json:"path"`
    }
    if err := json.Unmarshal([]byte(ctx.Self.GetConfig()), &cfg); err != nil {
        return ""
    }
    return cfg.Path
}

func init() {
    integrations.Register(integrations.ResourceRoleShell, "orbiter", &shellIntegration{})
}
```

- [ ] **Step 8: Create integrations/orbiter/shell_test.go**

Create `integrations/orbiter/shell_test.go`:

```go
package orbiter_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/integrations/orbiter"
    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func makeShellRC(path string) integrations.ResolvedContext {
    return integrations.ResolvedContext{
        Platform: integrations.Platform{OS: "linux", Arch: "amd64"},
        Self: models.Resource{
            ID:     "r-sh-01",
            Role:   "shell",
            Brand:  "orbiter",
            Config: `{"path":"` + path + `"}`,
        },
        Resources:    map[string][]integrations.ResolvedResource{},
        Transponders: map[string][]integrations.ResolvedTransponder{},
    }
}

func TestShell_Scan_Present(t *testing.T) {
    dir := t.TempDir()
    si := orbiter.NewShell()
    report := si.Scan(makeShellRC(dir))
    assert.True(t, report.Present)
    assert.True(t, report.Reachable)
    assert.Equal(t, dir, report.InstallDir)
}

func TestShell_Scan_Missing(t *testing.T) {
    si := orbiter.NewShell()
    report := si.Scan(makeShellRC("/tmp/orbiter-does-not-exist-xyz-999"))
    assert.False(t, report.Present)
    assert.Equal(t, "/tmp/orbiter-does-not-exist-xyz-999", report.InstallDir)
}

func TestShell_Init_CreatesDir(t *testing.T) {
    dir := filepath.Join(t.TempDir(), "newproject")
    si := orbiter.NewShell()
    report := si.Init(makeShellRC(dir))
    require.True(t, report.Present)
    assert.Equal(t, dir, report.InstallDir)
    _, err := os.Stat(dir)
    assert.NoError(t, err)
}

func TestShell_Registered(t *testing.T) {
    i, ok := integrations.Default.Get("shell", "orbiter")
    require.True(t, ok, "shell/orbiter should be registered")
    m := i.Meta()
    require.Len(t, m.Integration.Roles, 1)
    assert.Equal(t, "shell", m.Integration.Roles[0])
    assert.Equal(t, "orbiter", m.Integration.Brand)
}
```

- [ ] **Step 9: Delete filesystem.go and filesystem_test.go; update main.go import**

```bash
git rm internal/integrations/native/filesystem.go
git rm internal/integrations/native/filesystem_test.go
```

In `cmd/orbiter/main.go`, replace:
```go
_ "github.com/Kenttleton/orbiter/internal/integrations/native"
```
with:
```go
_ "github.com/Kenttleton/orbiter/integrations/orbiter"
```

- [ ] **Step 10: Update integrations/local/manifest.toml**

In `integrations/local/manifest.toml`, replace:
```toml
roles = ["filesystem"]
```
with:
```toml
roles = ["shell"]
```

- [ ] **Step 11: Add shell dependency to integrations/shell/manifest.toml**

In `integrations/shell/manifest.toml`, append:
```toml
[dependencies]
  [dependencies.resources]
  shell = []
```

This formalizes that the env transponder flows through the shell resource: a resource depending on `env` will have shell resources available in its resolved context.

- [ ] **Step 12: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 13: Commit**

```bash
git add internal/integrations/roles.go \
        internal/starchart/lifecycle.go \
        internal/starchart/resolve_cwd.go \
        internal/starchart/resolve_cwd_test.go \
        internal/starchart/lifecycle_test.go \
        internal/commands/executor.go \
        internal/integrations/native/shell_orbiter.go \
        internal/integrations/native/shell_orbiter_test.go \
        integrations/local/manifest.toml \
        integrations/shell/manifest.toml
git commit -m "feat: rename filesystem role to shell; align local and env/shell manifests"
```

---

### Task 2: Add per-shell WASM integrations (bash, zsh, fish, powershell)

**Files:**
- Create: `integrations/bash/`, `integrations/zsh/`, `integrations/fish/`, `integrations/powershell/` — each with `manifest.toml`, `assembly/index.ts`, `package.json`, `asconfig.json`, `<brand>.wasm`
- Modify: `integrations/bundle.go` — add four new brands to `//go:embed`
- Modify: `integrations/catalog_test.go` — assert four new catalog entries
- Modify: `Justfile` — add four build recipes

**Interfaces:**
- Produces: four WASM integrations registered under `shell/bash`, `shell/zsh`, `shell/fish`, `shell/powershell`; each returns `Config["script"]` from `calibrate()`

> **Pattern:** copy `package.json` and `asconfig.json` from `integrations/nvm/` as starting point; change `outFile` in `asconfig.json` to `<brand>.wasm`. The AssemblyScript FFI boilerplate (read_input, write_output, readInput, writeStr) is identical across all four; only the script constant and brand strings differ.

- [ ] **Step 1: Create integrations/bash/**

Create `integrations/bash/manifest.toml`:
```toml
[integration]
brand = "bash"
name = "Bash Shell"
description = "Integrates Orbiter with bash via PROMPT_COMMAND hook"
roles = ["shell"]

[detection]
[[detection.env]]
key = "BASH_VERSION"

[shell]
exports = [
  { hook = "hook.bash", description = "PROMPT_COMMAND hook for directory-change context detection" },
]

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 16
```

Create `integrations/bash/asconfig.json` (copy from nvm, change outFile):
```json
{
  "targets": {
    "release": {
      "outFile": "bash.wasm",
      "optimizeLevel": 3,
      "shrinkLevel": 1,
      "converge": false,
      "noAssert": false
    }
  },
  "options": {
    "bindings": "esm"
  }
}
```

Create `integrations/bash/package.json` (copy from nvm, change name):
```json
{
  "name": "orbiter-integration-bash",
  "version": "1.0.0",
  "scripts": {
    "asbuild:release": "asc assembly/index.ts --target release"
  },
  "dependencies": {
    "assemblyscript": "^0.27.0",
    "assemblyscript-json": "^1.1.0"
  }
}
```

Create `integrations/bash/hook.bash` (the static hook script — this is what `printShellScript` will emit):
```bash
# Orbiter shell integration — bash
# Source this in ~/.bashrc or ~/.bash_profile:
#   eval "$(::ORBITER:: init shell)"

function orbiter() {
    local _out _exit
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        echo "$_out" >&2
        return $_exit
    fi
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DIR)   cd "$_rest" ;;
            SET)   export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET) unset "$_rest" ;;
        esac
    done <<< "$_out"
}

function _orbiter_hook() {
    local _prev=$?
    [[ "$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"* ]] && return $_prev
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    if [[ $_exit -ne 0 ]]; then echo "$_out" >&2; return $_prev; fi
    [[ -z "$_out" ]] && return $_prev
    local _new_exports=()
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DEPART)
                for _k in $ORBITER_EXPORTS; do unset "$_k"; done
                unset ORBITER_PLANET ORBITER_EXPORTS ORBITER_CWD
                ;;
            SET)
                local _key="${_rest%%=*}"
                export "${_key}=${_rest#*=}"
                [[ "$_key" != ORBITER_* ]] && _new_exports+=("$_key")
                ;;
        esac
    done <<< "$_out"
    export ORBITER_CWD="$PWD"
    [[ ${#_new_exports[@]} -gt 0 ]] && export ORBITER_EXPORTS="${_new_exports[*]}"
    return $_prev
}

if [[ "$PROMPT_COMMAND" != *"_orbiter_hook"* ]]; then
    PROMPT_COMMAND="_orbiter_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
fi
```

Create `integrations/bash/assembly/index.ts`:
```typescript
import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 65536;

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

export function detect(): void {
  readInput();
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "shell");
  resource.set("brand", "bash");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

function report(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function initialize(): void { report(); }
export function scan(): void { report(); }
export function calibrate(): void { report(); }
```

- [ ] **Step 2: Create integrations/zsh/**

Create `integrations/zsh/manifest.toml`:
```toml
[integration]
brand = "zsh"
name = "Zsh Shell"
description = "Integrates Orbiter with zsh via chpwd hook"
roles = ["shell"]

[detection]
[[detection.env]]
key = "ZSH_VERSION"

[shell]
exports = [
  { hook = "hook.zsh", description = "chpwd hook for directory-change context detection" },
]

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 16
```

Create `integrations/zsh/asconfig.json` (same as bash, change outFile to `zsh.wasm`).

Create `integrations/zsh/package.json` (same as bash, change name to `orbiter-integration-zsh`).

Create `integrations/zsh/hook.zsh`:
```zsh
# Orbiter shell integration — zsh
# Source this in ~/.zshrc:
#   eval "$(::ORBITER:: init shell)"

function orbiter() {
    local _out _exit
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        print "$_out" >&2
        return $_exit
    fi
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DIR)   cd "$_rest" ;;
            SET)   export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET) unset "$_rest" ;;
        esac
    done <<< "$_out"
}

function _orbiter_chpwd() {
    local _prev=$?
    [[ "$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"* ]] && return $_prev
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    if [[ $_exit -ne 0 ]]; then print "$_out" >&2; return $_prev; fi
    [[ -z "$_out" ]] && return $_prev
    local -a _new_exports
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DEPART)
                for _k in ${(z)ORBITER_EXPORTS}; do unset "$_k"; done
                unset ORBITER_PLANET ORBITER_EXPORTS ORBITER_CWD
                ;;
            SET)
                local _key="${_rest%%=*}"
                export "${_key}=${_rest#*=}"
                [[ "$_key" != ORBITER_* ]] && _new_exports+=("$_key")
                ;;
        esac
    done <<< "$_out"
    export ORBITER_CWD="$PWD"
    [[ ${#_new_exports[@]} -gt 0 ]] && export ORBITER_EXPORTS="${_new_exports[*]}"
    return $_prev
}

autoload -Uz add-zsh-hook
add-zsh-hook chpwd _orbiter_chpwd
```

Create `integrations/zsh/assembly/index.ts` (detect only — same boilerplate as bash, brand = "zsh"):
```typescript
import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 65536;

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

export function detect(): void {
  readInput();
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "shell");
  resource.set("brand", "zsh");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

function report(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function initialize(): void { report(); }
export function scan(): void { report(); }
export function calibrate(): void { report(); }
```

- [ ] **Step 3: Create integrations/fish/**

Create `integrations/fish/manifest.toml`:
```toml
[integration]
brand = "fish"
name = "Fish Shell"
description = "Integrates Orbiter with fish via PWD variable hook"
roles = ["shell"]

[detection]
[[detection.env]]
key = "FISH_VERSION"

[shell]
exports = [
  { hook = "hook.fish", description = "PWD variable hook for directory-change context detection" },
]

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 16
```

Create `integrations/fish/asconfig.json` (outFile = `fish.wasm`).

Create `integrations/fish/package.json` (name = `orbiter-integration-fish`).

Create `integrations/fish/hook.fish`:
```fish
# Orbiter shell integration — fish
# Source this in ~/.config/fish/config.fish:
#   ::ORBITER:: init shell | source

function orbiter
    set _out (::ORBITER:: $argv)
    set _exit $status
    if test $_exit -ne 0
        echo $_out >&2
        return $_exit
    end
    for _line in (string split \n -- $_out)
        test -z "$_line"; and continue
        set _op (string split -m 1 ' ' -- $_line)[1]
        set _rest (string split -m 1 ' ' -- $_line)[2]
        switch $_op
            case DIR
                cd $_rest
            case SET
                set _key (string split -m 1 = -- $_rest)[1]
                set _val (string split -m 1 = -- $_rest)[2]
                set -gx $_key $_val
            case UNSET
                set -e $_rest
        end
    end
end

function _orbiter_hook --on-variable PWD
    set _prev $status
    set _cwd (pwd)
    if string match -q "$ORBITER_CWD*" -- $_cwd
        return $_prev
    end
    set _out (::ORBITER:: hook --cwd $_cwd --current "$ORBITER_PLANET")
    set _exit $status
    if test $_exit -ne 0; echo $_out >&2; return $_prev; end
    test -z "$_out"; and return $_prev
    set -l _new_exports
    for _line in (string split \n -- $_out)
        test -z "$_line"; and continue
        set _op (string split -m 1 ' ' -- $_line)[1]
        set _rest (string split -m 1 ' ' -- $_line)[2]
        switch $_op
            case DEPART
                for _k in (string split ' ' -- $ORBITER_EXPORTS)
                    set -e $_k
                end
                set -e ORBITER_PLANET ORBITER_EXPORTS ORBITER_CWD
            case SET
                set _key (string split -m 1 = -- $_rest)[1]
                set _val (string split -m 1 = -- $_rest)[2]
                set -gx $_key $_val
                if not string match -q 'ORBITER_*' -- $_key
                    set -a _new_exports $_key
                end
        end
    end
    set -gx ORBITER_CWD $_cwd
    if test (count $_new_exports) -gt 0
        set -gx ORBITER_EXPORTS (string join ' ' $_new_exports)
    end
    return $_prev
end
```

Create `integrations/fish/assembly/index.ts` (detect only — same boilerplate as bash, brand = "fish"):
```typescript
import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 65536;

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

export function detect(): void {
  readInput();
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "shell");
  resource.set("brand", "fish");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

function report(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function initialize(): void { report(); }
export function scan(): void { report(); }
export function calibrate(): void { report(); }
```

- [ ] **Step 4: Create integrations/powershell/**

Create `integrations/powershell/manifest.toml`:
```toml
[integration]
brand = "powershell"
name = "PowerShell"
description = "Integrates Orbiter with pwsh (PowerShell 7+) via LocationChangedAction hook"
roles = ["shell"]

[detection]
[[detection.env]]
key = "PSHOME"

[shell]
exports = [
  { hook = "hook.ps1", description = "LocationChangedAction hook for directory-change context detection" },
]

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 16
```

Create `integrations/powershell/asconfig.json` (outFile = `powershell.wasm`).

Create `integrations/powershell/package.json` (name = `orbiter-integration-powershell`).

Create `integrations/powershell/hook.ps1`:
```powershell
# Orbiter shell integration — PowerShell (pwsh 7+)
# Add to your $PROFILE:
#   Invoke-Expression (& ::ORBITER:: init shell)

function Invoke-Orbiter {
    $out = & ::ORBITER:: @args
    if ($LASTEXITCODE -ne 0) {
        Write-Error $out
        return
    }
    foreach ($line in ($out -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split ' ', 2
        $op = $parts[0]
        $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
        switch ($op) {
            'DIR'   { Set-Location $rest }
            'SET'   {
                $kv = $rest -split '=', 2
                Set-Item "env:$($kv[0])" $kv[1]
            }
            'UNSET' { Remove-Item "env:$rest" -ErrorAction SilentlyContinue }
        }
    }
}
Set-Alias orbiter Invoke-Orbiter

function _OrbiterHook {
    $prevEC = $LASTEXITCODE
    $cwd = (Get-Location).Path
    $planet = $env:ORBITER_CWD
    if ($planet -and ($cwd -eq $planet -or $cwd.StartsWith("$planet/"))) {
        $global:LASTEXITCODE = $prevEC; return
    }
    $out = & ::ORBITER:: hook --cwd $cwd --current "$($env:ORBITER_PLANET)"
    if ($LASTEXITCODE -ne 0) { Write-Error $out; $global:LASTEXITCODE = $prevEC; return }
    if ([string]::IsNullOrWhiteSpace($out)) { $global:LASTEXITCODE = $prevEC; return }
    $newExports = @()
    foreach ($line in ($out -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split ' ', 2
        $op = $parts[0]
        $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
        switch ($op) {
            'DEPART' {
                foreach ($k in ($env:ORBITER_EXPORTS -split ' ')) {
                    Remove-Item "env:$k" -ErrorAction SilentlyContinue
                }
                Remove-Item env:ORBITER_PLANET, env:ORBITER_EXPORTS, env:ORBITER_CWD -ErrorAction SilentlyContinue
            }
            'SET' {
                $kv = $rest -split '=', 2
                Set-Item "env:$($kv[0])" $kv[1]
                if (-not $kv[0].StartsWith('ORBITER_')) { $newExports += $kv[0] }
            }
        }
    }
    $env:ORBITER_CWD = $cwd
    if ($newExports.Count -gt 0) { $env:ORBITER_EXPORTS = $newExports -join ' ' }
    $global:LASTEXITCODE = $prevEC
}

$ExecutionContext.SessionState.InvokeCommand.LocationChangedAction = { _OrbiterHook }
```

Create `integrations/powershell/assembly/index.ts` (detect only — same boilerplate as bash, brand = "powershell"):
```typescript
import { JSON } from "assemblyscript-json/assembly";

@external("orbiter", "read_input")
declare function read_input(ptr: i32, max: i32): i32;

@external("orbiter", "write_output")
declare function write_output(ptr: i32, len: i32): void;

const BUF_SIZE: i32 = 65536;

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

export function detect(): void {
  readInput();
  const out = new JSON.Obj();
  out.set("detected", true);
  const resources = new JSON.Arr();
  const resource = new JSON.Obj();
  resource.set("role", "shell");
  resource.set("brand", "powershell");
  resources.push(resource);
  out.set("resources", resources);
  writeStr(out.stringify());
}

function report(): void {
  readInput();
  const obj = new JSON.Obj();
  obj.set("present", true);
  obj.set("reachable", true);
  obj.set("manager", "shell");
  writeStr(obj.stringify());
}

export function initialize(): void { report(); }
export function scan(): void { report(); }
export function calibrate(): void { report(); }
```

- [ ] **Step 5: Build all four WASM files**

```bash
cd integrations/bash && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
cd integrations/zsh && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
cd integrations/fish && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
cd integrations/powershell && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

Verify each produces `<brand>.wasm` in the integration directory.

- [ ] **Step 6: Add Justfile build recipes**

In `Justfile`, add four recipes following the same pattern as `build-integration-shell`:
```just
build-integration-bash:
    cd integrations/bash && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-zsh:
    cd integrations/zsh && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-fish:
    cd integrations/fish && npm install && ./node_modules/.bin/asc assembly/index.ts --target release

build-integration-powershell:
    cd integrations/powershell && npm install && ./node_modules/.bin/asc assembly/index.ts --target release
```

Add all four to the `build-integrations` dependency list.

- [ ] **Step 7: Update bundle.go //go:embed line**

In `integrations/bundle.go`, add to the `//go:embed` line:
```
bash/bash.wasm bash/manifest.toml zsh/zsh.wasm zsh/manifest.toml fish/fish.wasm fish/manifest.toml powershell/powershell.wasm powershell/manifest.toml
```

- [ ] **Step 8: Add catalog tests**

In `integrations/catalog_test.go`, add:

```go
func TestCatalog_ContainsBash(t *testing.T) {
    entries := CatalogEntries()
    for _, e := range entries {
        if e.Brand == "bash" {
            if !slices.Contains(e.Roles, "shell") {
                t.Fatalf("bash integration found but missing shell role; roles: %v", e.Roles)
            }
            return
        }
    }
    t.Fatal("bash integration not found in catalog")
}

func TestCatalog_ContainsZsh(t *testing.T) {
    entries := CatalogEntries()
    for _, e := range entries {
        if e.Brand == "zsh" {
            if !slices.Contains(e.Roles, "shell") {
                t.Fatalf("zsh integration found but missing shell role; roles: %v", e.Roles)
            }
            return
        }
    }
    t.Fatal("zsh integration not found in catalog")
}

func TestCatalog_ContainsFish(t *testing.T) {
    entries := CatalogEntries()
    for _, e := range entries {
        if e.Brand == "fish" {
            if !slices.Contains(e.Roles, "shell") {
                t.Fatalf("fish integration found but missing shell role; roles: %v", e.Roles)
            }
            return
        }
    }
    t.Fatal("fish integration not found in catalog")
}

func TestCatalog_ContainsPowershell(t *testing.T) {
    entries := CatalogEntries()
    for _, e := range entries {
        if e.Brand == "powershell" {
            if !slices.Contains(e.Roles, "shell") {
                t.Fatalf("powershell integration found but missing shell role; roles: %v", e.Roles)
            }
            return
        }
    }
    t.Fatal("powershell integration not found in catalog")
}
```

- [ ] **Step 9: Run tests**

```bash
go test ./integrations/... -v
```
Expected: PASS — four new catalog entries found.

- [ ] **Step 10: Commit**

```bash
git add integrations/bash/ integrations/zsh/ integrations/fish/ integrations/powershell/ \
        integrations/bundle.go integrations/catalog_test.go Justfile
git commit -m "feat: add bash/zsh/fish/powershell WASM shell integrations"
```

---

### Task 3: Update printShellScript to use WASM shell integrations

**Files:**
- Modify: `internal/commands/shell.go`
- Modify: `internal/commands/shell_test.go`
- Delete: `internal/commands/shell/orbiter.bash`
- Delete: `internal/commands/shell/orbiter.zsh`
- Delete: `internal/commands/shell/orbiter.fish`

**Interfaces:**
- Consumes: `integrations.Default.AllForRole(integrations.ResourceRoleShell)`, `manifest.Shell.HookFile()`, `integrations.BundleFS` (the exported embed.FS from `integrations/bundle.go`)

- [ ] **Step 1: Export BundleFS from bundle.go**

In `integrations/bundle.go`, export the embedded FS so `printShellScript` can read hook files:

```go
// BundleFS is the embedded filesystem containing all bundled integration files.
// Exported so commands can read declared hook scripts via manifest.Shell.HookFile().
var BundleFS = bundleFS
```

Add this immediately after the `var bundleFS embed.FS` declaration.

- [ ] **Step 2: Write failing test**

In `internal/commands/shell_test.go`, replace existing tests:

```go
package commands_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/commands"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestInitCmd_Shell_CommandExists(t *testing.T) {
    root := commands.NewRootCommand()
    initCmd, _, err := root.Find([]string{"init"})
    require.NoError(t, err)
    assert.NotNil(t, initCmd)
}
```

- [ ] **Step 3: Rewrite printShellScript in shell.go**

Replace the `printShellScript` function and remove the old `//go:embed` variable declarations in `internal/commands/shell.go`. The function reads the hook script from the bundle FS using the filename declared in the manifest:

```go
func printShellScript() error {
    self, err := os.Executable()
    if err != nil {
        return fmt.Errorf("resolve binary path: %w", err)
    }
    self, err = filepath.EvalSymlinks(self)
    if err != nil {
        return fmt.Errorf("resolve symlinks: %w", err)
    }

    env := osEnvMap()
    all := integrations.Default.AllForRole(integrations.ResourceRoleShell)

    for _, i := range all {
        m := i.Meta()
        if !m.Detection.MatchesAny(nil, env) {
            continue
        }
        hookFile := m.Shell.HookFile()
        if hookFile == "" {
            continue
        }
        script, err := bundleintegrations.BundleFS.ReadFile(m.Integration.Brand + "/" + hookFile)
        if err != nil {
            continue
        }
        fmt.Print(strings.ReplaceAll(string(script), "::ORBITER::", self))
        return nil
    }
    return fmt.Errorf("no shell detected — run 'orbiter init shell bash|zsh|fish|powershell' to specify one")
}

// osEnvMap parses os.Environ() into a key→value map.
func osEnvMap() map[string]string {
    raw := os.Environ()
    m := make(map[string]string, len(raw))
    for _, kv := range raw {
        k, v, _ := strings.Cut(kv, "=")
        m[k] = v
    }
    return m
}
```

Add `bundleintegrations "github.com/Kenttleton/orbiter/integrations"` to imports (use an alias to avoid collision with `internal/integrations`). Remove the old `//go:embed` variable declarations and the `"runtime"` import if it was only used for `currentPlatform()`.

- [ ] **Step 3: Delete old embedded shell scripts**

```bash
git rm internal/commands/shell/orbiter.bash \
       internal/commands/shell/orbiter.zsh \
       internal/commands/shell/orbiter.fish
rmdir internal/commands/shell/  # if now empty
```

- [ ] **Step 4: Run full test suite**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/shell.go internal/commands/shell_test.go
git commit -m "feat: init shell reads hook script from manifest-declared static file"
```

---

### Task 4: Add orbiter hook command

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/executor_test.go`
- Modify: `internal/commands/lifecycle.go`

**Interfaces:**
- Produces: `(Executor).Hook(ctx, cwd, currentPlanet string) ([]Directive, error)`
- The `hook` subcommand calls `Hook`, prints neutral directives to stdout, and returns the error (cobra writes to stderr, exits non-zero) on infrastructure failures; empty stdout + exit 0 is the no-op case

- [ ] **Step 1: Write failing tests**

In `internal/commands/executor_test.go`, add:

```go
func TestExecutor_Hook_SamePlanet_NoOutput(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    path := t.TempDir()
    r, _ := exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
    _, _ = exec.SC().Attach(ctx, r.Name, p.Name)

    alias, _ := exec.SC().Resolve(ctx, "payment-api")
    directives, err := exec.Hook(ctx, path, alias.ID)
    require.NoError(t, err)
    assert.Empty(t, directives, "same planet should return no directives")
}

func TestExecutor_Hook_NewPlanet_EmitsSet(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    path := t.TempDir()
    r, _ := exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
    _, _ = exec.SC().Attach(ctx, r.Name, p.Name)

    directives, err := exec.Hook(ctx, path, "")
    require.NoError(t, err)
    require.NotEmpty(t, directives)

    var foundPlanet bool
    for _, d := range directives {
        if d.Op == "SET" && d.Key == "ORBITER_PLANET" {
            foundPlanet = true
            assert.Equal(t, p.ID, d.Value)
        }
    }
    assert.True(t, foundPlanet, "Hook must emit SET ORBITER_PLANET")
}

func TestExecutor_Hook_LeavingPlanet_EmitsDepart(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    path := t.TempDir()
    r, _ := exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
    _, _ = exec.SC().Attach(ctx, r.Name, p.Name)

    directives, err := exec.Hook(ctx, "/tmp", p.ID)
    require.NoError(t, err)
    require.Len(t, directives, 1)
    assert.Equal(t, "DEPART", directives[0].Op)
}

func TestExecutor_Hook_NoMatchNoCurrent_Silent(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    directives, err := exec.Hook(ctx, "/tmp/unknown", "")
    require.NoError(t, err)
    assert.Empty(t, directives)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/commands/... -run "TestExecutor_Hook" -v
```
Expected: FAIL — `exec.Hook` undefined, `"DEPART"` op doesn't exist.

- [ ] **Step 3: Add DEPART to Directive.String()**

In `internal/commands/executor.go`, extend `Directive.String()`:

```go
func (d Directive) String() string {
    switch d.Op {
    case "DIR":
        return "DIR " + d.Value
    case "SET":
        key := strings.ReplaceAll(d.Key, "\n", "\\n")
        val := strings.ReplaceAll(d.Value, "\n", "\\n")
        return "SET " + key + "=" + val
    case "UNSET":
        return "UNSET " + d.Key
    case "DEPART":
        return "DEPART"
    }
    return ""
}
```

- [ ] **Step 4: Implement Hook method**

Add to `internal/commands/executor.go`:

```go
// Hook resolves cwd to a planet and returns directives for the shell hook to eval.
// Returns DEPART if currentPlanet is set but cwd no longer matches any planet.
// Returns SET ORBITER_PLANET + any allowed shell exports if a new planet is entered.
// Returns nil, nil if no context change occurred (cwd not a planet, none active).
// Returns non-nil error only on infrastructure failures (DB, I/O); callers surface to stderr.
func (e *Executor) Hook(ctx context.Context, cwd, currentPlanet string) ([]Directive, error) {
    cwd = filepath.Clean(cwd)

    alias, err := e.sc.ResolveCWD(ctx, cwd)
    if err != nil {
        if currentPlanet != "" {
            return []Directive{{Op: "DEPART"}}, nil
        }
        return nil, nil
    }

    if alias.ID == currentPlanet {
        return nil, nil
    }

    var directives []Directive
    directives = append(directives, Directive{Op: "SET", Key: "ORBITER_PLANET", Value: alias.ID})

    scanResult, err := e.sc.ScanBranch(ctx, alias.ID)
    if err != nil {
        return directives, nil
    }
    for _, r := range scanResult.Resources {
        i, ok := e.sc.Integration(r.Resource.Role, r.Resource.Brand)
        if !ok {
            continue
        }
        allowedEnvs := i.Meta().Shell.AllowedEnvs()
        allowed := make(map[string]bool, len(allowedEnvs))
        for _, k := range allowedEnvs {
            allowed[k] = true
        }
        for k, v := range r.Report.Exports {
            if allowed[k] {
                directives = append(directives, Directive{Op: "SET", Key: k, Value: v})
            }
        }
    }
    return directives, nil
}
```

Add `"path/filepath"` to imports if not already present.

- [ ] **Step 5: Add hook subcommand to lifecycle.go**

In `internal/commands/lifecycle.go`, add:

```go
func newHookCmd(exec *Executor) *cobra.Command {
    var cwd, current string
    cmd := &cobra.Command{
        Use:    "hook",
        Short:  "Emit context directives for shell hook (called automatically on cd)",
        Hidden: true,
        Args:   cobra.NoArgs,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            directives, err := exec.Hook(ctx, cwd, current)
            if err != nil {
                return err
            }
            for _, d := range directives {
                fmt.Println(d.String())
            }
            return nil
        },
    }
    cmd.Flags().StringVar(&cwd, "cwd", "", "current working directory")
    cmd.Flags().StringVar(&current, "current", "", "currently active planet ID")
    return cmd
}
```

Register in the command tree where other lifecycle commands are added:
```go
root.AddCommand(newHookCmd(exec))
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/commands/... -run "TestExecutor_Hook" -v
```
Expected: PASS

- [ ] **Step 7: Run full test suite**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/commands/executor.go \
        internal/commands/executor_test.go \
        internal/commands/lifecycle.go
git commit -m "feat: add orbiter hook command for shell directory-change detection"
```
