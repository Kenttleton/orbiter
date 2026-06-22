# Binary Discovery + Reachable Detection Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all hardcoded `present`/`reachable` values and all `which` calls from WASM integrations; Orbiter resolves declared binaries via shell FIND functions and passes paths in `ResolvedContext`; integrations only validate reachability.

**Architecture:** The manifest's `[integration] binaries` field is the transparent contract declaring what executables an integration operates on. Orbiter calls each shell's `FIND` function (defined in hook scripts, platform-aware) to resolve paths before invoking any WASM handler, then injects the map into `ResolvedContext.Binaries`. The WASM integration reads `ctx.binaries["X"]` to learn if the binary was found (`present`), then probes it with `run_command("X", "--version")` to confirm it responds (`reachable`). Integrations never call `which` — that is a POSIX-only tool and binary discovery is not the integration's responsibility.

**Tech Stack:** Go (host, manifest, context), Rust (wasm32-unknown-unknown), TinyGo (wasm-unknown), AssemblyScript (asc), bash/zsh/fish/PowerShell (hook scripts)

## Global Constraints

- No `which` in any integration source file or manifest `[commands] allowed` after this plan
- No new crate/package dependencies — all integrations remain self-contained
- Platform OS is always available as `ctx.platform.os` (`"darwin" | "linux" | "windows"`)
- Build commands: `just build-integration-<brand>`
- Test command: `go test ./integrations/... -v -run TestBundledIntegrations`
- FIND is called by Orbiter (Go host), not by WASM guests — WASM guests only read `ctx.binaries`
- Shell integrations (bash/zsh/fish/powershell) follow the same pattern — their binary IS the shell itself

---

### Task 1: Add FIND to all four shell hook scripts

`FIND` is a platform-aware binary resolver defined in each shell's hook script. Orbiter calls it (from Go) to resolve binaries declared in integration manifests before invoking WASM. Each shell implements FIND using the correct idiom for that shell and platform.

For bash: `export -f FIND` makes the function available to child processes (Orbiter Go binary) without needing an interactive shell invocation. For other shells, Orbiter invokes with the appropriate flag to load the profile.

**Files:**
- Modify: `integrations/bash/hook.bash`
- Modify: `integrations/zsh/hook.zsh`
- Modify: `integrations/fish/hook.fish`
- Modify: `integrations/powershell/hook.ps1`

**Interfaces:**
- Produces: `FIND <name>` outputs the absolute path to the binary or empty string if not found

- [ ] **Step 1: Add FIND to bash hook**

Append to `integrations/bash/hook.bash` before the final `if [[ "$PROMPT_COMMAND"...` block:

```bash
function FIND() {
    command -v "$1" 2>/dev/null
}
export -f FIND
```

Full file after edit:
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

function FIND() {
    command -v "$1" 2>/dev/null
}
export -f FIND

function _orbiter_hook() {
    local _prev=$?
    [[ -n "$ORBITER_CWD" && ("$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"*) ]] && return $_prev
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

- [ ] **Step 2: Add FIND to zsh hook**

Append FIND to `integrations/zsh/hook.zsh` before the `autoload` line:

```zsh
function FIND() {
    command -v "$1" 2>/dev/null
}
```

Full file after edit:
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

function FIND() {
    command -v "$1" 2>/dev/null
}

function _orbiter_chpwd() {
    local _prev=$?
    [[ -n "$ORBITER_CWD" && ("$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"*) ]] && return $_prev
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

- [ ] **Step 3: Add FIND to fish hook**

Read `integrations/fish/hook.fish`, then append:

```fish
function FIND
    command -v $argv[1] 2>/dev/null
end
```

- [ ] **Step 4: Add FIND to powershell hook**

Read `integrations/powershell/hook.ps1`, then append:

```powershell
function FIND {
    param([string]$Name)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { $cmd.Source } else { "" }
}
```

- [ ] **Step 5: Commit**

```bash
git add integrations/bash/hook.bash integrations/zsh/hook.zsh integrations/fish/hook.fish integrations/powershell/hook.ps1
git commit -m "feat: add FIND function to all shell hook scripts for Orbiter binary resolution"
```

---

### Task 2: Add `binaries` field to manifest schema and all affected manifests

The manifest's `[integration]` section gains a `binaries` field listing every executable the integration operates on. Orbiter reads this list before invoking WASM and resolves each name to an absolute path via FIND.

**Files:**
- Modify: `internal/integrations/manifest.go` — add `Binaries []string` to `ManifestIntegration`
- Modify: `integrations/brew/manifest.toml`
- Modify: `integrations/uv/manifest.toml`
- Modify: `integrations/rust/manifest.toml`
- Modify: `integrations/rustup/manifest.toml`
- Modify: `integrations/python/manifest.toml`
- Modify: `integrations/github/manifest.toml`
- Modify: `integrations/tmux/manifest.toml`
- Modify: `integrations/node/manifest.toml`
- Modify: `integrations/golang/manifest.toml`
- Modify: `integrations/make/manifest.toml`
- Modify: `integrations/just/manifest.toml`
- Modify: `integrations/nvm/manifest.toml`
- Modify: `integrations/bash/manifest.toml`
- Modify: `integrations/zsh/manifest.toml`
- Modify: `integrations/fish/manifest.toml`
- Modify: `integrations/powershell/manifest.toml`

**Interfaces:**
- Produces: `ManifestIntegration.Binaries []string` consumed by Task 3 resolver

- [ ] **Step 1: Read manifest.go**

Read `internal/integrations/manifest.go` to find the `ManifestIntegration` struct.

- [ ] **Step 2: Add Binaries field**

In `internal/integrations/manifest.go`, add `Binaries []string` to `ManifestIntegration`:

```go
type ManifestIntegration struct {
    Brand       string   `toml:"brand"`
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Roles       []string `toml:"roles"`
    Binaries    []string `toml:"binaries"`
}
```

- [ ] **Step 3: Write a test that reads binaries from a manifest**

In `internal/integrations/manifest_test.go` (or the existing manifest test file), add:

```go
func TestManifest_Binaries(t *testing.T) {
    raw := `
[integration]
brand = "brew"
name = "Homebrew"
description = "Package manager"
roles = ["manager"]
binaries = ["brew"]

[commands]
allowed = [
    { cmd = "brew", description = "Run brew commands" },
]
`
    var m Manifest
    if err := toml.Decode(raw, &m); err != nil {
        t.Fatalf("toml decode: %v", err)
    }
    if len(m.Integration.Binaries) != 1 || m.Integration.Binaries[0] != "brew" {
        t.Errorf("expected binaries=[brew], got %v", m.Integration.Binaries)
    }
}
```

- [ ] **Step 4: Run test to verify it fails before changes**

```bash
go test ./internal/integrations/... -v -run TestManifest_Binaries
```

Expected: FAIL — field not recognized yet.

- [ ] **Step 5: Run test again after adding the field**

```bash
go test ./internal/integrations/... -v -run TestManifest_Binaries
```

Expected: PASS.

- [ ] **Step 6: Update brew/manifest.toml**

Read the file, then add `binaries = ["brew"]` to `[integration]` and remove `which` from `[commands] allowed`:

```toml
[integration]
brand = "brew"
# ... existing fields ...
binaries = ["brew"]

[commands]
allowed = [
    { cmd = "brew", description = "Run Homebrew package manager commands" },
    # which removed — Orbiter resolves brew path via FIND
]
```

- [ ] **Step 7: Update remaining manifests**

For each integration below, read the manifest, add `binaries = [...]` to `[integration]`, remove `which` from `[commands] allowed`:

| Integration | binaries value |
|---|---|
| `uv/manifest.toml` | `["uv"]` |
| `rust/manifest.toml` | `["rustc"]` |
| `rustup/manifest.toml` | `["rustup"]` |
| `python/manifest.toml` | `["python3"]` |
| `github/manifest.toml` | `["gh"]` |
| `tmux/manifest.toml` | `["tmux"]` |
| `node/manifest.toml` | `["node"]` |
| `golang/manifest.toml` | `["go"]` |
| `make/manifest.toml` | `["make"]` |
| `just/manifest.toml` | `["just"]` |
| `nvm/manifest.toml` | `["node"]` (nvm validates via node) |
| `bash/manifest.toml` | `["bash"]` |
| `zsh/manifest.toml` | `["zsh"]` |
| `fish/manifest.toml` | `["fish"]` |
| `powershell/manifest.toml` | `["pwsh"]` |

Note: `dotenv` is a file-role integration with no binary — leave its manifest unchanged.

- [ ] **Step 8: Commit**

```bash
git add internal/integrations/manifest.go internal/integrations/manifest_test.go \
    integrations/brew/manifest.toml integrations/uv/manifest.toml \
    integrations/rust/manifest.toml integrations/rustup/manifest.toml \
    integrations/python/manifest.toml integrations/github/manifest.toml \
    integrations/tmux/manifest.toml integrations/node/manifest.toml \
    integrations/golang/manifest.toml integrations/make/manifest.toml \
    integrations/just/manifest.toml integrations/nvm/manifest.toml \
    integrations/bash/manifest.toml integrations/zsh/manifest.toml \
    integrations/fish/manifest.toml integrations/powershell/manifest.toml
git commit -m "feat: add binaries declaration to manifest schema and all affected integration manifests"
```

---

### Task 3: Add binary lookup to filesystem integration and wire into ResolvedContext

Binary discovery belongs to the `filesystem` integration — it is always registered, sits first in the resolution order, and owns local path operations. Orbiter calls `integrations.FindBinary` (backed by `filesystemIntegration`) before invoking WASM handlers. The shell's `FIND` function (Task 1) is what the filesystem integration delegates to for PATH-aware lookup — the filesystem role calls FIND, not `resolver.go` directly.

**Files:**
- Modify: `internal/integrations/filesystem.go` — add `FindBinary(name, osName string) string` function
- Modify: `internal/integrations/types.go` — add `Binaries map[string]string` to `ResolvedContext`
- Create: `internal/wasm/resolver.go` — thin `ResolveBinaries` that calls `integrations.FindBinary`
- Modify: `internal/wasm/loader.go` — call resolver before Init/Scan/Calibrate

**Interfaces:**
- Consumes: `ManifestIntegration.Binaries []string` (Task 2), `Platform.OS string`
- Produces: `ResolvedContext.Binaries map[string]string` — key is binary name, value is absolute path or `""`

- [ ] **Step 1: Read types.go**

Read `internal/integrations/types.go` to see the current `ResolvedContext` definition.

- [ ] **Step 2: Add Binaries to ResolvedContext**

```go
type ResolvedContext struct {
    Platform     Platform          `json:"platform"`
    Self         models.Resource   `json:"self"`
    Resources    []models.Resource `json:"resources"`
    Transponders []models.Resource `json:"transponders"`
    Responses    map[string]any    `json:"responses"`
    Binaries     map[string]string `json:"binaries,omitempty"`
}
```

- [ ] **Step 3: Write failing tests for FindBinary in filesystem**

Add to `internal/integrations/filesystem_test.go` (create if it doesn't exist):

```go
package integrations

import (
    "runtime"
    "testing"
)

func TestFindBinary_KnownBinary(t *testing.T) {
    // sh exists on all non-Windows platforms this test runs on
    if runtime.GOOS == "windows" {
        t.Skip("sh not available on windows")
    }
    path := FindBinary("sh", runtime.GOOS)
    if path == "" {
        t.Error("expected non-empty path for sh")
    }
}

func TestFindBinary_UnknownBinary(t *testing.T) {
    path := FindBinary("__orbiter_nonexistent_binary__", runtime.GOOS)
    if path != "" {
        t.Errorf("expected empty path for nonexistent binary, got %q", path)
    }
}
```

- [ ] **Step 4: Run tests to confirm they fail**

```bash
go test ./internal/integrations/... -v -run TestFindBinary
```

Expected: FAIL — `FindBinary` not defined.

- [ ] **Step 5: Add FindBinary to internal/integrations/filesystem.go**

`FindBinary` is a package-level function backed by the filesystem integration. It delegates to the shell's `FIND` function (defined in hook scripts, Task 1) for PATH-aware resolution, falling back to POSIX `command -v` on Unix and `where.exe` on Windows if FIND is not yet available.

```go
import (
    "os"
    "os/exec"
    "strings"
)

// FindBinary resolves a binary name to its absolute path via the filesystem
// integration's path lookup. It delegates to the shell's FIND function defined
// in the Orbiter hook scripts, which is platform-aware (command -v on Unix,
// Get-Command on PowerShell). Falls back to sh/where.exe if FIND is unavailable.
func FindBinary(name, osName string) string {
    if osName == "windows" {
        return findBinaryWindows(name)
    }
    return findBinaryUnix(name)
}

func findBinaryUnix(name string) string {
    shell := os.Getenv("SHELL")

    // bash: hook exports FIND via export -f, available in child processes without -i
    if shell != "" && strings.HasSuffix(shell, "bash") {
        if p := runShellFind(shell, "-c", "FIND "+name); p != "" {
            return p
        }
    }

    // zsh/fish: invoke via profile so FIND is loaded from hook
    if shell != "" {
        flag := "-i"
        if strings.HasSuffix(shell, "fish") {
            flag = "-c"
        }
        if p := runShellFind(shell, flag, "FIND "+name); p != "" {
            return p
        }
    }

    // Fallback: POSIX command -v (no FIND dependency)
    out, err := exec.Command("sh", "-c", "command -v "+name).Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}

func findBinaryWindows(name string) string {
    // PowerShell profile defines FIND using Get-Command
    cmd := exec.Command("pwsh", "-NoLogo", "-NonInteractive", "-Command",
        ". $PROFILE; FIND "+name)
    cmd.Stderr = nil
    if out, err := cmd.Output(); err == nil {
        if p := strings.TrimSpace(string(out)); p != "" {
            return p
        }
    }
    // Fallback: where.exe
    out, err := exec.Command("where.exe", name).Output()
    if err != nil {
        return ""
    }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    return strings.TrimSpace(lines[0])
}

func runShellFind(shell, flag, expr string) string {
    cmd := exec.Command(shell, flag, expr)
    cmd.Stderr = nil
    out, err := cmd.Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}
```

- [ ] **Step 6: Run filesystem tests**

```bash
go test ./internal/integrations/... -v -run TestFindBinary
```

Expected: both pass.

- [ ] **Step 7: Write tests for the WASM resolver**

Create `internal/wasm/resolver_test.go`:

```go
package wasm

import (
    "runtime"
    "testing"
)

func TestResolveBinaries_EmptyList(t *testing.T) {
    result := ResolveBinaries([]string{}, runtime.GOOS)
    if len(result) != 0 {
        t.Errorf("expected empty map, got %v", result)
    }
}

func TestResolveBinaries_KnownBinary(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("sh not available on windows")
    }
    result := ResolveBinaries([]string{"sh"}, runtime.GOOS)
    if result["sh"] == "" {
        t.Error("expected non-empty path for sh")
    }
}

func TestResolveBinaries_UnknownBinary(t *testing.T) {
    result := ResolveBinaries([]string{"__orbiter_nonexistent_binary__"}, runtime.GOOS)
    if result["__orbiter_nonexistent_binary__"] != "" {
        t.Errorf("expected empty path for nonexistent binary")
    }
}
```

- [ ] **Step 8: Create internal/wasm/resolver.go**

This is a thin delegation to the filesystem integration — no path logic lives here:

```go
package wasm

import "github.com/orbiterops/orbiter/internal/integrations"

// ResolveBinaries resolves each declared binary name to an absolute path
// by delegating to the filesystem integration's FindBinary function.
// Returns a map of name → path; missing binaries have empty-string values.
func ResolveBinaries(names []string, osName string) map[string]string {
    result := make(map[string]string)
    for _, name := range names {
        result[name] = integrations.FindBinary(name, osName)
    }
    return result
}
```

- [ ] **Step 9: Run resolver tests**

```bash
go test ./internal/wasm/... -v -run TestResolveBinaries
```

Expected: all pass.

- [ ] **Step 10: Read loader.go**

Read `internal/wasm/loader.go` to find the Init/Scan/Calibrate methods and where `ctx` is marshaled.

- [ ] **Step 11: Update loader.go to populate Binaries before WASM invocation**

```go
// populateBinaries resolves manifest-declared binaries via the filesystem
// integration and injects them into ctx before the WASM handler is invoked.
func (w *WASMIntegration) populateBinaries(ctx integrations.ResolvedContext) integrations.ResolvedContext {
    if len(w.manifest.Integration.Binaries) == 0 {
        return ctx
    }
    ctx.Binaries = ResolveBinaries(w.manifest.Integration.Binaries, ctx.Platform.OS)
    return ctx
}
```

Add `ctx = w.populateBinaries(ctx)` as the first line of `Init`, `Scan`, and `Calibrate` before `json.Marshal(ctx)`.

- [ ] **Step 12: Run Go tests**

```bash
go test ./internal/... -v
```

Expected: all pass.

- [ ] **Step 13: Commit**

```bash
git add internal/integrations/types.go internal/integrations/filesystem.go \
    internal/integrations/filesystem_test.go \
    internal/wasm/resolver.go internal/wasm/resolver_test.go internal/wasm/loader.go
git commit -m "feat: filesystem integration owns binary lookup; resolver and loader wire it into ResolvedContext"
```

---

### Task 4: Fix Rust integrations — read binary path from context

All Rust integrations currently call `host::run_command("which", &["X"])` to get the binary path. Replace with reading `ctx.binaries["X"]` from context. The `present` flag becomes `!binary_path.is_empty()`. The `reachable` flag remains derived from the version probe. Remove `which` from every integration's Rust source.

Affected integrations: `brew`, `uv`, `rust` (rustc), `rustup`, `python`, `github`, `tmux`, `bash`, `zsh`, `fish`, `powershell`.

**Files:**
- Modify: `integrations/brew/src/lib.rs`
- Modify: `integrations/uv/src/lib.rs`
- Modify: `integrations/rust/src/lib.rs`
- Modify: `integrations/rustup/src/lib.rs`
- Modify: `integrations/python/src/lib.rs`
- Modify: `integrations/github/src/lib.rs`
- Modify: `integrations/tmux/src/lib.rs`
- Modify: `integrations/bash/src/lib.rs`
- Modify: `integrations/zsh/src/lib.rs`
- Modify: `integrations/fish/src/lib.rs`
- Modify: `integrations/powershell/src/lib.rs`

**Interfaces:**
- Consumes: `ResolvedContext.binaries` map via serde deserialization
- Produces: `StateReport { present: !binary_path.is_empty(), reachable: !version.is_empty(), binary_path: Some(binary_path), ... }`

- [ ] **Step 1: Read integrations/brew/src/lib.rs**

Read the file to see the current ResolvedContext deserialization struct and the initialize/calibrate functions.

- [ ] **Step 2: Add binaries field to Rust ResolvedContext struct**

In each Rust integration's `lib.rs`, find the `ResolvedContext` struct (usually near the top) and add a `binaries` field. Use `HashMap` from std::collections:

```rust
use std::collections::HashMap;

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    binaries: HashMap<String, String>,
    // ... other existing fields unchanged
}
```

If the struct already has a `#[serde(default)]` or similar, just add the field.

- [ ] **Step 3: Fix brew/src/lib.rs initialize()**

Current (wrong):
```rust
let binary_path = host::run_command("which", &["brew"]);
// ...
reachable: true,
```

Fixed:
```rust
let binary_path = ctx.binaries.get("brew").cloned().unwrap_or_default();
let present = !binary_path.is_empty();
if !present {
    write_state(StateReport {
        present: false,
        reachable: false,
        manager: "system".to_string(),
        observations: vec!["brew not found".to_string()],
        ..Default::default()
    });
    return;
}
let version = host::run_command("brew", &["--version"]);
write_state(StateReport {
    present: true,
    reachable: !version.is_empty(),
    binary_path: Some(binary_path),
    in_path: true,
    manager: "system".to_string(),
    observations: vec![version],
    ..Default::default()
});
```

- [ ] **Step 4: Fix brew/src/lib.rs calibrate()**

```rust
let binary_path = ctx.binaries.get("brew").cloned().unwrap_or_default();
let present = !binary_path.is_empty();
if !present {
    write_state(StateReport {
        present: false,
        reachable: false,
        manager: "system".to_string(),
        ..Default::default()
    });
    return;
}
let version = host::run_command("brew", &["--version"]);
write_state(StateReport {
    present: true,
    reachable: !version.is_empty(),
    in_path: true,
    manager: "system".to_string(),
    observations: vec![format!("calibrated: {}", version)],
    ..Default::default()
});
```

- [ ] **Step 5: Apply same pattern to uv, rust, rustup, python, github**

For each integration, the pattern is identical — substitute the binary name:

| Integration | binary key | version command |
|---|---|---|
| `uv` | `"uv"` | `uv --version` |
| `rust` | `"rustc"` | `rustc --version` |
| `rustup` | `"rustup"` | `rustup --version` |
| `python` | `"python3"` | `python3 --version` |
| `github` | `"gh"` | `gh --version` |

For `rust` (rustc), note it has `detect_manager()` logic — preserve that, just change the binary resolution from `which` to `ctx.binaries.get("rustc")`.

For `github`, there are multiple role handlers (scan_tool, scan_auth, etc.) — apply the fix to whichever currently calls `which gh`.

- [ ] **Step 6: Fix tmux/src/lib.rs calibrate()**

Replace the entire `calibrate()` body:

```rust
#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        binaries: HashMap::new(),
        self_res: None,
    });
    let cfg = parse_config(&ctx);

    let binary_path = ctx.binaries.get("tmux").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();

    if cfg.vars.is_empty() {
        write_state(StateReport {
            present,
            reachable: present,
            manager: "system".to_string(),
            ..Default::default()
        });
        return;
    }

    if !present {
        write_state(StateReport {
            present: false,
            reachable: false,
            manager: "system".to_string(),
            observations: vec!["tmux not found — skipped".to_string()],
            ..Default::default()
        });
        return;
    }

    let mut applied = Vec::new();
    for (key, val) in &cfg.vars {
        let result = host::run_command("tmux", &["set-environment", "-g", key, val]);
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

Also fix `scan()` and `initialize()` in tmux to use `ctx.binaries.get("tmux")` instead of `which`.

- [ ] **Step 7: Fix bash, zsh, fish, powershell Rust integrations**

Each shell integration's binary key matches the shell name:
- `bash/src/lib.rs`: `ctx.binaries.get("bash")`
- `zsh/src/lib.rs`: `ctx.binaries.get("zsh")`
- `fish/src/lib.rs`: `ctx.binaries.get("fish")`
- `powershell/src/lib.rs`: `ctx.binaries.get("pwsh")`

Pattern for each (example: bash):
```rust
let binary_path = ctx.binaries.get("bash").cloned().unwrap_or_default();
let present = !binary_path.is_empty();
if !present {
    write_state(StateReport {
        present: false,
        reachable: false,
        manager: "system".to_string(),
        ..Default::default()
    });
    return;
}
let version = host::run_command("bash", &["--version"]);
write_state(StateReport {
    present: true,
    reachable: !version.is_empty(),
    binary_path: Some(binary_path),
    in_path: true,
    manager: "system".to_string(),
    observations: vec![version],
    ..Default::default()
});
```

- [ ] **Step 8: Build all Rust integrations**

```bash
just build-integration-brew build-integration-uv build-integration-rust build-integration-rustup \
    build-integration-python build-integration-github build-integration-tmux \
    build-integration-bash build-integration-zsh build-integration-fish build-integration-powershell
```

Expected: all complete without errors.

- [ ] **Step 9: Run integration tests**

```bash
go test ./integrations/... -v -run 'TestBundledIntegrations_(Brew|UV|RustLang|Rustup|Python|GitHub|Tmux|Bash|Zsh|Fish|Powershell)'
```

Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add integrations/brew/src/lib.rs integrations/brew/brew.wasm \
    integrations/uv/src/lib.rs integrations/uv/uv.wasm \
    integrations/rust/src/lib.rs integrations/rust/rust.wasm \
    integrations/rustup/src/lib.rs integrations/rustup/rustup.wasm \
    integrations/python/src/lib.rs integrations/python/python.wasm \
    integrations/github/src/lib.rs integrations/github/github.wasm \
    integrations/tmux/src/lib.rs integrations/tmux/tmux.wasm \
    integrations/bash/src/lib.rs integrations/bash/bash.wasm \
    integrations/zsh/src/lib.rs integrations/zsh/zsh.wasm \
    integrations/fish/src/lib.rs integrations/fish/fish.wasm \
    integrations/powershell/src/lib.rs integrations/powershell/powershell.wasm
git commit -m "fix: Rust integrations read binary path from context instead of calling which"
```

---

### Task 5: Fix TinyGo integrations — read binary path from context

TinyGo integrations use hand-rolled JSON parsing (gjson/sjson fail on wasm-unknown). The `ResolvedContext` JSON now includes a `"binaries"` key with a nested object. Add a helper to extract the value for a given binary name from the raw input bytes.

Affected integrations: `node`, `golang`, `make`. (`dotenv` is a file-role — no binary to resolve, handled separately below.)

**Files:**
- Modify: `integrations/node/guest/main.go`
- Modify: `integrations/golang/guest/main.go`
- Modify: `integrations/make/guest/main.go`

**Interfaces:**
- Consumes: raw JSON from `readInput()` with `binaries` key
- Produces: `present: binaryPath != "", reachable: version != ""`

- [ ] **Step 1: Read integrations/node/guest/main.go**

Read the file to understand the current JSON parsing approach and `readInput()` / `writeState()` signatures.

- [ ] **Step 2: Add extractBinaryPath helper to node/guest/main.go**

Add this function after the existing JSON helpers:

```go
// extractBinaryPath extracts ctx.binaries["name"] from raw ResolvedContext JSON.
// The JSON shape is: {"binaries":{"node":"/usr/bin/node",...},...}
// Uses simple string scanning since gjson is unavailable in wasm-unknown.
func extractBinaryPath(input []byte, name string) string {
    s := string(input)
    key := `"binaries":{`
    start := strings.Index(s, key)
    if start < 0 {
        return ""
    }
    sub := s[start+len(key):]
    end := strings.Index(sub, "}")
    if end >= 0 {
        sub = sub[:end]
    }
    needle := `"` + name + `":"`
    idx := strings.Index(sub, needle)
    if idx < 0 {
        return ""
    }
    rest := sub[idx+len(needle):]
    close := strings.Index(rest, `"`)
    if close < 0 {
        return ""
    }
    return rest[:close]
}
```

- [ ] **Step 3: Fix node/guest/main.go initialize()**

Replace the current initialize that calls `runCmd("which", "node")`:

```go
//export initialize
func initialize() {
    input := readInput()
    binaryPath := extractBinaryPath(input, "node")
    present := binaryPath != ""
    if !present {
        writeState(false, false, false, "", "system", "node not found", nil)
        return
    }
    version := runCmd("node", "--version")
    reachable := version != ""
    writeState(true, reachable, reachable, binaryPath, "system", "", []string{version})
}
```

- [ ] **Step 4: Add extractBinaryPath to golang/guest/main.go and make/guest/main.go**

Copy the same `extractBinaryPath` function into each file (they are independent WASM modules with no shared packages).

Fix `initialize()` in each with the same pattern:

golang:
```go
//export initialize
func initialize() {
    input := readInput()
    binaryPath := extractBinaryPath(input, "go")
    present := binaryPath != ""
    if !present {
        writeState(false, false, false, "", "system", "go binary not found", nil)
        return
    }
    version := runCmd("go", "version")
    reachable := version != ""
    writeState(true, reachable, reachable, binaryPath, "system", "", []string{version})
}
```

make:
```go
//export initialize
func initialize() {
    input := readInput()
    binaryPath := extractBinaryPath(input, "make")
    present := binaryPath != ""
    if !present {
        writeState(false, false, false, "", "system", "make not found", nil)
        return
    }
    version := runCmd("make", "--version")
    reachable := version != ""
    writeState(true, reachable, reachable, binaryPath, "system", "", []string{version})
}
```

- [ ] **Step 5: Fix dotenv/guest/main.go — remove which ls dead-code workaround**

dotenv is a file-role transponder with no binary. Replace `runCmd("which", "ls")` dead-code workaround with a symbol-table retain:

```go
func main() {
    // Retain hostRunCommand in the symbol table to prevent TinyGo dead-code elimination
    // of the wasm import. File-role integrations never invoke run_command at runtime.
    _ = hostRunCommand
}

//export initialize
func initialize() {
    readInput()
    writeState(true, true, false, "", "file", "", []string{"dotenv transponder active"})
}
```

- [ ] **Step 6: Build all TinyGo integrations**

```bash
just build-integration-node build-integration-golang build-integration-make build-integration-dotenv
```

Expected: all complete without errors.

- [ ] **Step 7: Run tests**

```bash
go test ./integrations/... -v -run 'TestBundledIntegrations_(Node|Golang|Make|Dotenv)'
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add integrations/node/guest/main.go integrations/node/node.wasm \
    integrations/golang/guest/main.go integrations/golang/golang.wasm \
    integrations/make/guest/main.go integrations/make/make.wasm \
    integrations/dotenv/guest/main.go integrations/dotenv/dotenv.wasm
git commit -m "fix: TinyGo integrations read binary path from context; remove which and dead-code workaround"
```

---

### Task 6: Fix AssemblyScript integrations — read binary path from context

AssemblyScript lacks runtime JSON parsing. Add an `extractBinaryPath` helper function that scans the raw UTF-8 JSON string for `binaries["name"]`. Affected integrations: `just`, `nvm`, `json`.

**Files:**
- Modify: `integrations/just/assembly/index.ts`
- Modify: `integrations/nvm/assembly/index.ts`
- Modify: `integrations/json/assembly/index.ts`

**Interfaces:**
- Consumes: raw UTF-8 JSON string with `binaries` key via existing `readInput()` helper
- Produces: `present: binaryPath.length > 0, reachable: version.length > 0`

- [ ] **Step 1: Read integrations/just/assembly/index.ts**

Read the file to understand the existing `runCmd`, `readInput`, and `writeState` signatures and the current JSON parsing approach.

- [ ] **Step 2: Add extractBinaryPath helper to just/assembly/index.ts**

Add before `export function detect()`:

```typescript
function extractBinaryPath(input: string, name: string): string {
  const binKey = '"binaries":{"';
  const start = input.indexOf('"binaries":{');
  if (start < 0) return "";
  const sub = input.slice(start + '"binaries":{'.length);
  const end = sub.indexOf("}");
  const block = end >= 0 ? sub.slice(0, end) : sub;
  const needle = '"' + name + '":"';
  const idx = block.indexOf(needle);
  if (idx < 0) return "";
  const rest = block.slice(idx + needle.length);
  const close = rest.indexOf('"');
  if (close < 0) return "";
  return rest.slice(0, close);
}
```

- [ ] **Step 3: Fix just/assembly/index.ts initialize()**

```typescript
export function initialize(): void {
  const input = readInput();
  const binaryPath = extractBinaryPath(input, "just");
  const present = binaryPath.length > 0;
  if (!present) {
    writeState(false, false, false, "", "system", "just not found", []);
    return;
  }
  const version = runCmd("just", ["--version"]);
  const reachable = version.length > 0;
  writeState(true, reachable, reachable, binaryPath, "system", "", [version]);
}
```

- [ ] **Step 4: Fix just/assembly/index.ts calibrate()**

```typescript
export function calibrate(): void {
  const input = readInput();
  const binaryPath = extractBinaryPath(input, "just");
  const present = binaryPath.length > 0;
  if (!present) {
    writeState(false, false, false, "", "system", "just not found", []);
    return;
  }
  const version = runCmd("just", ["--version"]);
  const reachable = version.length > 0;
  writeState(true, reachable, reachable, binaryPath, "system", "", ["calibrated: " + version]);
}
```

- [ ] **Step 5: Fix nvm/assembly/index.ts**

nvm validates via `node` (nvm is a shell function, not a binary). Its manifest declares `binaries = ["node"]`. The integration reads the resolved node path to determine if nvm activated node:

Add `extractBinaryPath` (same function), then fix `initialize()`:

```typescript
export function initialize(): void {
  const input = readInput();
  const nodePath = extractBinaryPath(input, "node");
  const present = nodePath.length > 0;
  const observations: string[] = ["manager: nvm"];
  if (present) {
    const version = runCmd("node", ["--version"]);
    if (version.length > 0) observations.push("active node: " + version);
  }
  writeState(present, present, false, "", "nvm", "", observations);
}
```

Fix `calibrate()` the same way.

- [ ] **Step 6: Fix json/assembly/index.ts scan()**

`scan()` currently returns hardcoded `present:true` without reading input. Fix it to delegate to `calibrate()`:

```typescript
export function scan(): void {
  calibrate();
}
```

- [ ] **Step 7: Build AssemblyScript integrations**

```bash
just build-integration-just build-integration-nvm build-integration-json
```

Expected: all complete without errors, emit `build/release.wasm`.

- [ ] **Step 8: Run tests**

```bash
go test ./integrations/... -v -run 'TestBundledIntegrations_(Just|NVM|JSON)'
```

Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add integrations/just/assembly/index.ts integrations/just/build/release.wasm \
    integrations/nvm/assembly/index.ts integrations/nvm/build/release.wasm \
    integrations/json/assembly/index.ts integrations/json/build/release.wasm
git commit -m "fix: AssemblyScript integrations read binary path from context; fix json scan hardcode"
```

---

### Task 7: Full suite verification

- [ ] **Step 1: Verify no `which` remains in any integration source**

```bash
grep -r '"which"' integrations/*/src/ integrations/*/guest/ integrations/*/assembly/ 2>/dev/null
grep -r '"which"' integrations/*/manifest.toml 2>/dev/null
```

Expected: no output. Any remaining occurrences are bugs.

- [ ] **Step 2: Run full integration test suite**

```bash
go test ./integrations/... -v
```

Expected: all pass.

- [ ] **Step 3: Run full Go test suite**

```bash
go test ./...
```

Expected: all pass. Any failures indicate a regression introduced in Tasks 1-6.

- [ ] **Step 4: Commit if any minor fixes were needed**

```bash
git add -p
git commit -m "fix: test and integration cleanup after binary discovery refactor"
```

---

## Self-Review

**Spec coverage:**
- FIND added to all 4 shell hook scripts → Task 1 ✓
- `binaries` field in manifest schema + all 15 affected manifests → Task 2 ✓
- `ResolvedContext.Binaries` + Orbiter resolver + loader injection → Task 3 ✓
- 11 Rust integrations read from context, no `which` → Task 4 ✓
- 4 TinyGo integrations (+ dotenv dead-code fix) → Task 5 ✓
- 3 AssemblyScript integrations (+ json scan fix) → Task 6 ✓
- No `which` audit → Task 7 ✓

**Placeholder scan:** None — every step has exact code.

**Type consistency:**
- `ResolveBinaries(names []string, osName string) map[string]string` — used in Task 3 resolver, referenced in loader
- `ResolvedContext.Binaries map[string]string` — Go struct matches JSON key `"binaries"` parsed by all guest languages
- Rust: `ctx.binaries.get("X").cloned().unwrap_or_default()` → `String`
- TinyGo: `extractBinaryPath(input, "X")` → `string`
- AssemblyScript: `extractBinaryPath(input, "X")` → `string`

**Cross-platform note:** On Windows, brew/fish/zsh/bash binaries won't be found by FIND → `present: false` in those integrations. This is correct behavior, not a bug. The integration handles it gracefully in every task's "not found" early-return branch.
