# Shell Role: filesystem→shell Rename + Hook Command + PowerShell

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `filesystem` resource role to `shell`, add an `orbiter hook` command for automatic context detection on directory change, add per-shell native integrations with manifest env detection rules, and add PowerShell support.

**Architecture:** The `shell` role replaces `filesystem` — the resource still holds a `{"path":"..."}` config but is now explicitly about shell context entry, not generic filesystem state. Native Go integrations for bash/zsh/fish/powershell each embed their hook script and declare env detection rules in their manifest, enabling `init shell` to match the running shell without branded code in the binary. The `hook` subcommand is a fast, no-WASM path: resolve CWD → emit neutral directives → exit.

**Tech Stack:** Go 1.23+, `go:embed`, bash/zsh/fish/PowerShell scripts.

## Global Constraints

- Depends on Plan 1 (neutral `Directive` protocol and `ManifestDetection.Env` must already be merged).
- No database migration — old `filesystem` resources will not match after the rename. This is intentional; the codebase has no production users.
- `orbiter hook` must not call any WASM integration — pure Go only, latency budget is <20ms on a cold start.
- The hook must be silent on errors (non-zero exit must not break the user's prompt).
- PowerShell script targets pwsh (cross-platform PowerShell 7+), not Windows PowerShell 5.

---

## File Map

| File | Change |
|---|---|
| `internal/integrations/roles.go` | Rename `ResourceRoleFilesystem` → `ResourceRoleShell = "shell"`, update `RoleTypes` |
| `internal/starchart/lifecycle.go` | Update `resourceRoleOrder` to use `ResourceRoleShell` |
| `internal/starchart/resolve_cwd.go` | Update SQL query from `role = 'filesystem'` to `role = 'shell'` |
| `internal/starchart/resolve_cwd_test.go` | Update all `"filesystem"` literals to `"shell"` |
| `internal/starchart/lifecycle_test.go` | Update `"filesystem"` literals to `"shell"` |
| `internal/integrations/native/filesystem.go` | Delete (replaced by shell_orbiter.go) |
| `internal/integrations/native/filesystem_test.go` | Delete (replaced by shell_orbiter_test.go) |
| `internal/integrations/native/shell_orbiter.go` | New — native `shell/orbiter` integration (directory presence check) |
| `internal/integrations/native/shell_orbiter_test.go` | New — tests for shell/orbiter |
| `internal/integrations/native/shell_bash.go` | New — native `shell/bash` integration with embedded script + env detection |
| `internal/integrations/native/shell_zsh.go` | New — native `shell/zsh` integration with embedded script + env detection |
| `internal/integrations/native/shell_fish.go` | New — native `shell/fish` integration with embedded script + env detection |
| `internal/integrations/native/shell_powershell.go` | New — native `shell/powershell` integration with embedded script + env detection |
| `internal/integrations/native/shell/bash.sh` | New — bash hook script (moved from `internal/commands/shell/orbiter.bash`) |
| `internal/integrations/native/shell/zsh.sh` | New — zsh hook script (moved from `internal/commands/shell/orbiter.zsh`) |
| `internal/integrations/native/shell/fish.fish` | New — fish hook script (moved from `internal/commands/shell/orbiter.fish`) |
| `internal/integrations/native/shell/powershell.ps1` | New — PowerShell hook script |
| `internal/integrations/types.go` | Add `ShellScripter` interface |
| `internal/commands/shell.go` | Update `printShellScript` to use manifest env detection |
| `internal/commands/shell/orbiter.bash` | Delete (script now lives in `native/shell/bash.sh`) |
| `internal/commands/shell/orbiter.zsh` | Delete (script now lives in `native/shell/zsh.sh`) |
| `internal/commands/shell/orbiter.fish` | Delete (script now lives in `native/shell/fish.fish`) |
| `internal/commands/executor.go` | Add `Hook` method, update Jump role reference |
| `internal/commands/executor_test.go` | Add Hook tests, update Jump role reference |
| `internal/commands/lifecycle.go` | Add `hook` subcommand |

---

### Task 1: Rename filesystem → shell role throughout

**Files:**
- Modify: `internal/integrations/roles.go`
- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/resolve_cwd.go`
- Modify: `internal/starchart/resolve_cwd_test.go`
- Modify: `internal/starchart/lifecycle_test.go`
- Modify: `internal/commands/executor.go` (one reference)
- Delete: `internal/integrations/native/filesystem.go`
- Delete: `internal/integrations/native/filesystem_test.go`
- Create: `internal/integrations/native/shell_orbiter.go`
- Create: `internal/integrations/native/shell_orbiter_test.go`

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

Also update the error message at the bottom:
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

Also update the helper function name and role string:
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

- [ ] **Step 7: Create shell_orbiter.go (replaces filesystem.go)**

Create `internal/integrations/native/shell_orbiter.go`:

```go
package native

import (
    "encoding/json"
    "os"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

var shellOrbiterManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand: "orbiter",
        Roles: []string{integrations.ResourceRoleShell},
    },
}

type shellOrbiter struct{}

// NewShellOrbiter returns a shellOrbiter for testing.
func NewShellOrbiter() integrations.Integration {
    return &shellOrbiter{}
}

func (f *shellOrbiter) Meta() integrations.Manifest {
    return shellOrbiterManifest
}

func (f *shellOrbiter) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}

func (f *shellOrbiter) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    path := shellPathFromSelf(ctx)
    if path == "" {
        return integrations.StateReport{Error: "no path in resource config"}
    }
    if err := os.MkdirAll(path, 0755); err != nil {
        return integrations.StateReport{InstallDir: path, Error: err.Error()}
    }
    return integrations.StateReport{Present: true, Reachable: true, InstallDir: path}
}

func (f *shellOrbiter) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    path := shellPathFromSelf(ctx)
    if path == "" {
        return integrations.StateReport{Error: "no path in resource config"}
    }
    info, err := os.Stat(path)
    if err != nil {
        return integrations.StateReport{Present: false, InstallDir: path}
    }
    return integrations.StateReport{
        Present:   true,
        Reachable: info.IsDir(),
        InstallDir: path,
    }
}

func (f *shellOrbiter) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return f.Init(ctx)
}

func shellPathFromSelf(ctx integrations.ResolvedContext) string {
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
    integrations.Register(integrations.ResourceRoleShell, "orbiter", &shellOrbiter{})
}
```

- [ ] **Step 8: Create shell_orbiter_test.go**

Create `internal/integrations/native/shell_orbiter_test.go`:

```go
package native_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/integrations/native"
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

func TestShellOrbiter_Scan_Present(t *testing.T) {
    dir := t.TempDir()
    si := native.NewShellOrbiter()
    report := si.Scan(makeShellRC(dir))
    assert.True(t, report.Present)
    assert.True(t, report.Reachable)
    assert.Equal(t, dir, report.InstallDir)
}

func TestShellOrbiter_Scan_Missing(t *testing.T) {
    si := native.NewShellOrbiter()
    report := si.Scan(makeShellRC("/tmp/orbiter-does-not-exist-xyz-999"))
    assert.False(t, report.Present)
    assert.Equal(t, "/tmp/orbiter-does-not-exist-xyz-999", report.InstallDir)
}

func TestShellOrbiter_Init_CreatesDir(t *testing.T) {
    dir := filepath.Join(t.TempDir(), "newproject")
    si := native.NewShellOrbiter()
    report := si.Init(makeShellRC(dir))
    require.True(t, report.Present)
    assert.Equal(t, dir, report.InstallDir)
    _, err := os.Stat(dir)
    assert.NoError(t, err)
}

func TestShellOrbiter_Registered(t *testing.T) {
    i, ok := integrations.Default.Get("shell", "orbiter")
    require.True(t, ok, "shell/orbiter should be registered")
    m := i.Meta()
    require.Len(t, m.Integration.Roles, 1)
    assert.Equal(t, "shell", m.Integration.Roles[0])
    assert.Equal(t, "orbiter", m.Integration.Brand)
}
```

- [ ] **Step 9: Delete filesystem.go and filesystem_test.go**

```bash
rm internal/integrations/native/filesystem.go
rm internal/integrations/native/filesystem_test.go
```

- [ ] **Step 10: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 11: Commit**

```bash
git add internal/integrations/roles.go \
        internal/starchart/lifecycle.go \
        internal/starchart/resolve_cwd.go \
        internal/starchart/resolve_cwd_test.go \
        internal/starchart/lifecycle_test.go \
        internal/commands/executor.go \
        internal/integrations/native/shell_orbiter.go \
        internal/integrations/native/shell_orbiter_test.go
git rm internal/integrations/native/filesystem.go \
       internal/integrations/native/filesystem_test.go
git commit -m "feat: rename filesystem role to shell throughout"
```

---

### Task 2: Create per-shell native integrations with scripts and env detection

**Files:**
- Add `ShellScripter` interface to `internal/integrations/types.go`
- Create dir `internal/integrations/native/shell/`
- Create `internal/integrations/native/shell/bash.sh`
- Create `internal/integrations/native/shell/zsh.sh`
- Create `internal/integrations/native/shell/fish.fish`
- Create `internal/integrations/native/shell/powershell.ps1`
- Create `internal/integrations/native/shell_bash.go`
- Create `internal/integrations/native/shell_zsh.go`
- Create `internal/integrations/native/shell_fish.go`
- Create `internal/integrations/native/shell_powershell.go`

**Interfaces:**
- Produces: `integrations.ShellScripter` interface with `Script() string`; four integrations registered under `shell/bash`, `shell/zsh`, `shell/fish`, `shell/powershell`

- [ ] **Step 1: Add ShellScripter to types.go**

In `internal/integrations/types.go`, append:

```go
// ShellScripter is implemented by native shell integrations that embed a hook script.
// printShellScript uses this interface to retrieve the script without a type assertion
// on a concrete type from the native package.
type ShellScripter interface {
    Script() string
}
```

- [ ] **Step 2: Create bash hook script**

Create `internal/integrations/native/shell/bash.sh`:

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
    [[ "$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"* ]] && return 0
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    [[ $_exit -ne 0 ]] && return 0
    [[ -z "$_out" ]] && return 0
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
}

if [[ "$PROMPT_COMMAND" != *"_orbiter_hook"* ]]; then
    PROMPT_COMMAND="_orbiter_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
fi
```

- [ ] **Step 3: Create zsh hook script**

Create `internal/integrations/native/shell/zsh.sh`:

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
    [[ "$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"* ]] && return 0
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    [[ $_exit -ne 0 ]] && return 0
    [[ -z "$_out" ]] && return 0
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
}

autoload -Uz add-zsh-hook
add-zsh-hook chpwd _orbiter_chpwd
```

- [ ] **Step 4: Create fish hook script**

Create `internal/integrations/native/shell/fish.fish`:

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
    set _cwd (pwd)
    if string match -q "$ORBITER_CWD*" -- $_cwd
        return 0
    end
    set _out (::ORBITER:: hook --cwd $_cwd --current "$ORBITER_PLANET")
    test $status -ne 0; and return 0
    test -z "$_out"; and return 0
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
end
```

- [ ] **Step 5: Create PowerShell hook script**

Create `internal/integrations/native/shell/powershell.ps1`:

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
    $cwd = (Get-Location).Path
    $planet = $env:ORBITER_CWD
    if ($planet -and ($cwd -eq $planet -or $cwd.StartsWith("$planet/"))) { return }
    $out = & ::ORBITER:: hook --cwd $cwd --current "$($env:ORBITER_PLANET)"
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($out)) { return }
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
}

$ExecutionContext.SessionState.InvokeCommand.LocationChangedAction = { _OrbiterHook }
```

- [ ] **Step 6: Create shell_bash.go**

Create `internal/integrations/native/shell_bash.go`:

```go
package native

import (
    _ "embed"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

//go:embed shell/bash.sh
var bashScript string

var shellBashManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand:       "bash",
        Name:        "Bash Shell",
        Description: "Integrates Orbiter with bash via PROMPT_COMMAND hook",
        Roles:       []string{integrations.ResourceRoleShell},
    },
    Detection: integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{
            {Key: "BASH_VERSION"},
        },
    },
}

type shellBash struct{}

func (s *shellBash) Meta() integrations.Manifest         { return shellBashManifest }
func (s *shellBash) Script() string                      { return bashScript }
func (s *shellBash) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}
func (s *shellBash) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellBash) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellBash) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}

func init() {
    integrations.Register(integrations.ResourceRoleShell, "bash", &shellBash{})
}
```

- [ ] **Step 7: Create shell_zsh.go**

Create `internal/integrations/native/shell_zsh.go`:

```go
package native

import (
    _ "embed"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

//go:embed shell/zsh.sh
var zshScript string

var shellZshManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand:       "zsh",
        Name:        "Zsh Shell",
        Description: "Integrates Orbiter with zsh via chpwd hook",
        Roles:       []string{integrations.ResourceRoleShell},
    },
    Detection: integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{
            {Key: "ZSH_VERSION"},
        },
    },
}

type shellZsh struct{}

func (s *shellZsh) Meta() integrations.Manifest         { return shellZshManifest }
func (s *shellZsh) Script() string                      { return zshScript }
func (s *shellZsh) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}
func (s *shellZsh) Init(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellZsh) Scan(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellZsh) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}

func init() {
    integrations.Register(integrations.ResourceRoleShell, "zsh", &shellZsh{})
}
```

- [ ] **Step 8: Create shell_fish.go**

Create `internal/integrations/native/shell_fish.go`:

```go
package native

import (
    _ "embed"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

//go:embed shell/fish.fish
var fishScript string

var shellFishManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand:       "fish",
        Name:        "Fish Shell",
        Description: "Integrates Orbiter with fish via PWD variable hook",
        Roles:       []string{integrations.ResourceRoleShell},
    },
    Detection: integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{
            {Key: "FISH_VERSION"},
        },
    },
}

type shellFish struct{}

func (s *shellFish) Meta() integrations.Manifest         { return shellFishManifest }
func (s *shellFish) Script() string                      { return fishScript }
func (s *shellFish) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}
func (s *shellFish) Init(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellFish) Scan(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellFish) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}

func init() {
    integrations.Register(integrations.ResourceRoleShell, "fish", &shellFish{})
}
```

- [ ] **Step 9: Create shell_powershell.go**

Create `internal/integrations/native/shell_powershell.go`:

```go
package native

import (
    _ "embed"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

//go:embed shell/powershell.ps1
var powershellScript string

var shellPowershellManifest = integrations.Manifest{
    Integration: integrations.ManifestIntegration{
        Brand:       "powershell",
        Name:        "PowerShell",
        Description: "Integrates Orbiter with pwsh via LocationChangedAction hook",
        Roles:       []string{integrations.ResourceRoleShell},
    },
    Detection: integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{
            {Key: "PSHOME"},
        },
    },
}

type shellPowershell struct{}

func (s *shellPowershell) Meta() integrations.Manifest         { return shellPowershellManifest }
func (s *shellPowershell) Script() string                      { return powershellScript }
func (s *shellPowershell) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: false}
}
func (s *shellPowershell) Init(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellPowershell) Scan(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}
func (s *shellPowershell) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Reachable: true}
}

func init() {
    integrations.Register(integrations.ResourceRoleShell, "powershell", &shellPowershell{})
}
```

- [ ] **Step 10: Write registration tests**

Create `internal/integrations/native/shell_brands_test.go`:

```go
package native_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    _ "github.com/Kenttleton/orbiter/internal/integrations/native"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestShellBrands_Registered(t *testing.T) {
    for _, brand := range []string{"bash", "zsh", "fish", "powershell"} {
        t.Run(brand, func(t *testing.T) {
            i, ok := integrations.Default.Get("shell", brand)
            require.True(t, ok, "shell/%s should be registered", brand)
            m := i.Meta()
            assert.Equal(t, brand, m.Integration.Brand)
            require.Len(t, m.Integration.Roles, 1)
            assert.Equal(t, "shell", m.Integration.Roles[0])
            require.NotEmpty(t, m.Detection.Env, "shell/%s must declare env detection rules", brand)
        })
    }
}

func TestShellBrands_ScriptNonEmpty(t *testing.T) {
    for _, brand := range []string{"bash", "zsh", "fish", "powershell"} {
        t.Run(brand, func(t *testing.T) {
            i, ok := integrations.Default.Get("shell", brand)
            require.True(t, ok)
            ss, ok := i.(integrations.ShellScripter)
            require.True(t, ok, "shell/%s must implement ShellScripter", brand)
            script := ss.Script()
            assert.NotEmpty(t, script)
            assert.Contains(t, script, "::ORBITER::", "script must contain ::ORBITER:: token")
        })
    }
}

func TestShellBrands_EnvDetection(t *testing.T) {
    cases := []struct {
        brand   string
        envKey  string
        envVal  string
    }{
        {"bash", "BASH_VERSION", "5.2.15"},
        {"zsh", "ZSH_VERSION", "5.9"},
        {"fish", "FISH_VERSION", "3.6.0"},
        {"powershell", "PSHOME", "/usr/local/microsoft/powershell/7"},
    }
    for _, tc := range cases {
        t.Run(tc.brand, func(t *testing.T) {
            i, _ := integrations.Default.Get("shell", tc.brand)
            m := i.Meta()
            env := map[string]string{tc.envKey: tc.envVal}
            assert.True(t, m.Detection.MatchesAny(nil, env),
                "shell/%s detection should match when %s is set", tc.brand, tc.envKey)
            assert.False(t, m.Detection.MatchesAny(nil, map[string]string{}),
                "shell/%s detection should not match when %s is absent", tc.brand, tc.envKey)
        })
    }
}
```

- [ ] **Step 11: Run tests**

```
go test ./internal/integrations/... -v
```
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add internal/integrations/types.go \
        internal/integrations/native/shell_bash.go \
        internal/integrations/native/shell_zsh.go \
        internal/integrations/native/shell_fish.go \
        internal/integrations/native/shell_powershell.go \
        internal/integrations/native/shell_brands_test.go \
        internal/integrations/native/shell/bash.sh \
        internal/integrations/native/shell/zsh.sh \
        internal/integrations/native/shell/fish.fish \
        internal/integrations/native/shell/powershell.ps1
git commit -m "feat: add per-shell native integrations with env detection and hook scripts"
```

---

### Task 3: Update printShellScript to use manifest detection, remove old embedded scripts

**Files:**
- Modify: `internal/commands/shell.go`
- Delete: `internal/commands/shell/orbiter.bash`
- Delete: `internal/commands/shell/orbiter.zsh`
- Delete: `internal/commands/shell/orbiter.fish`

**Interfaces:**
- Consumes: `integrations.ShellScripter`, `integrations.Default.AllForRole("shell")`

- [ ] **Step 1: Write test for manifest-based detection in init shell**

In `internal/commands/shell_test.go`, replace the existing tests with:

```go
package commands_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/commands"
    _ "github.com/Kenttleton/orbiter/internal/integrations/native"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestInitCmd_Shell_CommandExists(t *testing.T) {
    root := commands.NewRootCommand()
    initCmd, _, err := root.Find([]string{"init"})
    require.NoError(t, err)
    assert.NotNil(t, initCmd)
    assert.Equal(t, "init [shell|vessel]", initCmd.Use)
}

func TestInitCmd_Shell_NoOrbiterToken(t *testing.T) {
    root := commands.NewRootCommand()
    shellCmd, _, err := root.Find([]string{"shell", "init"})
    require.NoError(t, err)
    assert.NotNil(t, shellCmd)
}
```

- [ ] **Step 2: Rewrite printShellScript in shell.go**

Replace the entire `printShellScript` function and remove the three `go:embed` declarations for the old scripts in `internal/commands/shell.go`:

```go
package commands

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/spf13/cobra"

    bundle "github.com/Kenttleton/orbiter/integrations"
    "github.com/Kenttleton/orbiter/internal/integrations"
)

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
    shellIntegrations := integrations.Default.AllForRole(integrations.ResourceRoleShell)

    var matches []integrations.Integration
    for _, i := range shellIntegrations {
        if i.Meta().Detection.MatchesAny(nil, env) {
            matches = append(matches, i)
        }
    }

    if len(matches) == 0 {
        return fmt.Errorf(
            "no shell detected — run 'orbiter init shell bash|zsh|fish|powershell' to specify one",
        )
    }

    // Use first match; multiple matches are uncommon and the first is most specific.
    ss, ok := matches[0].(integrations.ShellScripter)
    if !ok {
        return fmt.Errorf("shell integration %q does not provide a hook script", matches[0].Meta().Integration.Brand)
    }
    fmt.Print(strings.ReplaceAll(ss.Script(), "::ORBITER::", self))
    return nil
}

// osEnvMap parses os.Environ() into a key→value map.
// Defined here for use by printShellScript; starchart/discover.go has its own copy.
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

Note: the rest of `shell.go` (vesselInitRun, newInitCmd, newShellCmd) remains unchanged.

- [ ] **Step 3: Delete old embedded shell scripts**

```bash
rm internal/commands/shell/orbiter.bash
rm internal/commands/shell/orbiter.zsh
rm internal/commands/shell/orbiter.fish
```

If the `internal/commands/shell/` directory is now empty, remove it:
```bash
rmdir internal/commands/shell/
```

- [ ] **Step 4: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/shell.go
git rm internal/commands/shell/orbiter.bash \
       internal/commands/shell/orbiter.zsh \
       internal/commands/shell/orbiter.fish
git commit -m "feat: init shell uses manifest env detection; scripts move to native integrations"
```

---

### Task 4: Add orbiter hook command

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/executor_test.go`
- Modify: `internal/commands/lifecycle.go`

**Interfaces:**
- Produces: `(Executor).Hook(ctx, cwd, currentPlanet string) ([]Directive, error)`
- The `hook` subcommand calls `Hook` and prints neutral directives to stdout, exits 0 on all errors (silent)

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

    // Must include SET ORBITER_PLANET=<id>
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

    // "currently in" the planet, moving to /tmp (no planet)
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

```
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
        return "SET " + d.Key + "=" + d.Value
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
// Returns SET ORBITER_PLANET + any shell exports if a new planet is entered.
// Returns nil if no context change occurred. Errors are swallowed — the hook
// must never break the user's prompt.
func (e *Executor) Hook(ctx context.Context, cwd, currentPlanet string) ([]Directive, error) {
    cwd = filepath.Clean(cwd)

    alias, err := e.sc.ResolveCWD(ctx, cwd)
    if err != nil {
        // No planet at cwd.
        if currentPlanet != "" {
            return []Directive{{Op: "DEPART"}}, nil
        }
        return nil, nil
    }

    if alias.ID == currentPlanet {
        return nil, nil
    }

    // Entering a new planet — collect shell exports.
    var directives []Directive
    directives = append(directives, Directive{Op: "SET", Key: "ORBITER_PLANET", Value: alias.ID})

    scanResult, err := e.sc.ScanBranch(ctx, alias.ID)
    if err != nil {
        // Return the planet directive even if scan fails.
        return directives, nil
    }
    for _, r := range scanResult.Resources {
        i, ok := e.sc.Integration(r.Resource.Role, r.Resource.Brand)
        if !ok {
            continue
        }
        allowed := make(map[string]bool, len(i.Meta().Shell.Exports))
        for _, k := range i.Meta().Shell.Exports {
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

In `internal/commands/lifecycle.go`, add a `newHookCmd` function and register it:

```go
func newHookCmd(exec *Executor) *cobra.Command {
    var cwd, current string
    cmd := &cobra.Command{
        Use:    "hook",
        Short:  "Emit context directives for shell hook (called automatically on cd)",
        Hidden: true, // not a user-facing command
        Args:   cobra.NoArgs,
        // Override star chart requirement — hook must be silent on all errors.
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            directives, _ := exec.Hook(ctx, cwd, current)
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

Register in the command tree (find where lifecycle commands are added to the root and add `newHookCmd`):

```go
root.AddCommand(newHookCmd(exec))
```

- [ ] **Step 6: Run tests**

```
go test ./internal/commands/... -run "TestExecutor_Hook" -v
```
Expected: PASS

- [ ] **Step 7: Run full test suite**

```
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
