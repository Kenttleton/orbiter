# Binary Resolution via Shell Directive Protocol

> **Status:** Superseded by [2026-10-26-find-implementation.md](2026-10-26-find-implementation.md)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all Go-based binary discovery (`FindBinary` and shell-invocation code from `filesystem.go`) and replace it with a correct two-phase shell directive round-trip: Orbiter emits `FIND <name>` directives when binary paths are needed, the shell's already-defined `FIND` function resolves them, and the shell re-calls Orbiter with `--binary name=path` flags.

**Architecture:** Orbiter never calls the shell directly. Instead, each lifecycle command checks what binaries its integrations declare, emits `FIND <name>` directives for any not yet provided, and exits. The shell wrapper (already sourced in the Captain's profile) calls its own `FIND` function, collects paths, and re-calls Orbiter with `--binary` flags. On the second call, Orbiter has all paths: it compares each against the resource's stored `binary_path` in the StarChart config, sets beacon to `drifted` and halts if there's a mismatch (Captain must approve before StarChart is updated), and otherwise injects the paths into `ResolvedContext.Binaries` for the integration.

**Tech Stack:** Go 1.22, cobra, wazero, SQLite (via direct `database/sql`), bash/zsh/fish/PowerShell hook scripts.

## Global Constraints

- `filesystem.go` MUST NOT call any shell, process, or `os/exec` — it is OS-path logic only (`os.Stat`, `os.MkdirAll`, JSON config parsing)
- `FindBinary`, `findBinaryUnix`, `findBinaryWindows`, `runShellFind` MUST be deleted entirely
- `internal/wasm/resolver.go` and `internal/wasm/resolver_test.go` MUST be deleted entirely
- Binary drift MUST NOT auto-update the StarChart — only update the beacon to `drifted` and surface an observation; Captain resolves
- The Captain (user) is always in control; Orbiter surfaces state and waits
- No new migrations — the resource `config` column is already TEXT JSON; merge `binary_path` into existing config JSON
- No feature flags, no backwards-compat shims — update all call sites in the same task
- `go test ./...` must pass after every task
- `go vet ./...` must pass after every task

---

## File Map

| File | Change |
|------|--------|
| `internal/commands/executor.go` | Add `FIND` op to `Directive`; change `Scan`/`Calibrate`/`Chart` return type to `([]Directive, error)`; add `binaries map[string]string` param; add `collectNeededBinaries`; add pre-flight FIND emission |
| `internal/commands/lifecycle.go` | Add `--binary name=path` (StringSlice) flag to all commands; parse into map; handle FIND directives in print loop; update executor call sites |
| `internal/starchart/lifecycle.go` | Add `binaries map[string]string` to `ScanBranch`, `CalibrateBranch`, `scanResource`, `calibrateResource`; inject binaries into `rc.Binaries`; add `BinaryDrift bool` to `ResourceScanResult` |
| `internal/starchart/init.go` | After `integration.Init`, if `report.BinaryPath != ""` persist it to resource config JSON via `sc.Update` |
| `internal/starchart/lifecycle.go` | Binary drift check in `scanResource`: compare fresh path to stored `config["binary_path"]`; beacon → `drifted`, `BinaryDrift: true`, halt |
| `internal/wasm/loader.go` | Remove `populateBinaries` calls and delete the function (binaries set upstream) |
| `internal/integrations/filesystem.go` | Delete `FindBinary`, `findBinaryUnix`, `findBinaryWindows`, `runShellFind`; remove `os/exec`/`strings` imports |
| `internal/wasm/resolver.go` | **Delete file** |
| `internal/wasm/resolver_test.go` | **Delete file** |
| `integrations/bash/hook.bash` | FIND directive handling + pass-through for non-directive lines in `orbiter()` and `_orbiter_hook()` |
| `integrations/zsh/hook.zsh` | Same as bash |
| `integrations/fish/hook.fish` | Same, fish syntax |
| `integrations/powershell/hook.ps1` | Same, PowerShell syntax |

---

### Task 1: FIND directive type + shell hook round-trip

**Files:**
- Modify: `internal/commands/executor.go` (Directive.String only)
- Modify: `integrations/bash/hook.bash`
- Modify: `integrations/zsh/hook.zsh`
- Modify: `integrations/fish/hook.fish`
- Modify: `integrations/powershell/hook.ps1`

**Interfaces:**
- Produces: `Directive{Op: "FIND", Key: "brew"}` renders as `"FIND brew"` — consumed by Task 2 (executor pre-flight)
- Produces: shell wrappers handle `FIND` op and `*` pass-through — consumed by all subsequent tasks when Orbiter is called through the shell

**Background — the directive protocol:** Orbiter writes one directive per line to stdout. The shell `orbiter()` wrapper captures this output and interprets each line. Current ops: `DIR`, `SET`, `UNSET`, `DEPART`. We add `FIND`. The wrapper also currently silently drops non-directive lines — this is a bug we fix here with a pass-through case.

**Background — FIND in hook scripts:** Each shell hook already defines a `FIND` function:
- bash/zsh: `command -v "$1" 2>/dev/null`
- fish: `command -v $argv[1] 2>/dev/null`
- PowerShell: `Get-Command $Name -ErrorAction SilentlyContinue` returning `.Source`

The round-trip: Orbiter emits `FIND brew` → wrapper sees `FIND` op → calls `FIND brew` → gets `/opt/homebrew/bin/brew` → collects `--binary brew=/opt/homebrew/bin/brew` → re-calls `::ORBITER:: <same-cmd> --binary brew=/opt/homebrew/bin/brew` → Orbiter proceeds with paths provided.

- [ ] **Step 1: Add FIND case to `Directive.String()`**

In `internal/commands/executor.go`, the `Directive.String()` method (around line 27) currently handles `DIR`, `SET`, `UNSET`, `DEPART`. Add `FIND`:

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
	case "FIND":
		return "FIND " + d.Key
	}
	return ""
}
```

- [ ] **Step 2: Write the test**

In `internal/commands/executor_test.go`, add alongside the existing directive tests:

```go
func TestDirective_FIND(t *testing.T) {
	d := commands.Directive{Op: "FIND", Key: "brew"}
	if d.String() != "FIND brew" {
		t.Errorf("got %q, want %q", d.String(), "FIND brew")
	}
}
```

- [ ] **Step 3: Run test**

```
go test ./internal/commands/ -run TestDirective_FIND -v
```

Expected: `PASS`

- [ ] **Step 4: Update `integrations/bash/hook.bash`**

The `orbiter()` function captures Orbiter's stdout, processes directives, and must now:
1. Detect `FIND` directives, resolve them, collect `--binary` args
2. Re-call `::ORBITER::` with original args + `--binary` args if any FIND directives were seen
3. Echo non-directive lines (fixes the silent-drop bug that hides `scan`/`calibrate` output)

Replace the `orbiter()` function:

```bash
function orbiter() {
    local _out _exit
    local -a _binary_args
    _binary_args=()
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        echo "$_out" >&2
        return $_exit
    fi
    local _found_find=false
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            FIND)
                local _path
                _path="$(FIND "$_rest")"
                _binary_args+=("--binary" "${_rest}=${_path}")
                _found_find=true
                ;;
            DIR)   cd "$_rest" ;;
            SET)   export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET) unset "$_rest" ;;
            *)     echo "$_line" ;;
        esac
    done <<< "$_out"
    if $_found_find; then
        _out="$(::ORBITER:: "$@" "${_binary_args[@]}")"
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
                *)     echo "$_line" ;;
            esac
        done <<< "$_out"
    fi
}
```

Also update `_orbiter_hook()` — the same FIND handling is needed when the hook fires automatically on `cd`. Replace the existing directive-processing loop in `_orbiter_hook`:

```bash
function _orbiter_hook() {
    local _prev=$?
    [[ -n "$ORBITER_CWD" && ("$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"*) ]] && return $_prev
    local _out _exit
    local -a _binary_args
    _binary_args=()
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    if [[ $_exit -ne 0 ]]; then echo "$_out" >&2; return $_prev; fi
    [[ -z "$_out" ]] && return $_prev
    local _found_find=false
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            FIND)
                local _path
                _path="$(FIND "$_rest")"
                _binary_args+=("--binary" "${_rest}=${_path}")
                _found_find=true
                ;;
        esac
    done <<< "$_out"
    if $_found_find; then
        _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}" "${_binary_args[@]}")"
        _exit=$?
        if [[ $_exit -ne 0 ]]; then echo "$_out" >&2; return $_prev; fi
        [[ -z "$_out" ]] && return $_prev
    fi
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
```

- [ ] **Step 5: Update `integrations/zsh/hook.zsh`**

Replace the `orbiter()` function (same logic, zsh syntax — `print` instead of `echo`, `local -a` works the same):

```zsh
function orbiter() {
    local _out _exit
    local -a _binary_args
    _binary_args=()
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        print "$_out" >&2
        return $_exit
    fi
    local _found_find=false
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            FIND)
                local _path
                _path="$(FIND "$_rest")"
                _binary_args+=("--binary" "${_rest}=${_path}")
                _found_find=true
                ;;
            DIR)   cd "$_rest" ;;
            SET)   export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET) unset "$_rest" ;;
            *)     print "$_line" ;;
        esac
    done <<< "$_out"
    if $_found_find; then
        _out="$(::ORBITER:: "$@" "${_binary_args[@]}")"
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
                *)     print "$_line" ;;
            esac
        done <<< "$_out"
    fi
}
```

Replace `_orbiter_chpwd()` similarly — same pattern as the bash `_orbiter_hook` update above, using `print` instead of `echo`.

- [ ] **Step 6: Update `integrations/fish/hook.fish`**

Replace the `orbiter` function (fish syntax — `set` for arrays, `string split`, `string match`):

```fish
function orbiter
    set _out (::ORBITER:: $argv)
    set _exit $status
    if test $_exit -ne 0
        echo $_out >&2
        return $_exit
    end
    set -l _binary_args
    set -l _found_find false
    for _line in (string split \n -- $_out)
        test -z "$_line"; and continue
        set _op (string split -m 1 ' ' -- $_line)[1]
        set _rest (string split -m 1 ' ' -- $_line)[2]
        switch $_op
            case FIND
                set _path (FIND $_rest)
                set -a _binary_args "--binary" "$_rest=$_path"
                set _found_find true
            case DIR
                cd $_rest
            case SET
                set _key (string split -m 1 = -- $_rest)[1]
                set _val (string split -m 1 = -- $_rest)[2]
                set -gx $_key $_val
            case UNSET
                set -e $_rest
            case '*'
                echo $_line
        end
    end
    if test $_found_find = true
        set _out (::ORBITER:: $argv $_binary_args)
        set _exit $status
        if test $_exit -ne 0; echo $_out >&2; return $_exit; end
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
                case '*'
                    echo $_line
            end
        end
    end
end
```

Also update `_orbiter_hook` — same FIND round-trip pattern before processing SET/DEPART.

- [ ] **Step 7: Update `integrations/powershell/hook.ps1`**

Replace `Invoke-Orbiter` (PowerShell syntax — `-split`, arrays, `$binaryArgs`):

```powershell
function Invoke-Orbiter {
    $out = & ::ORBITER:: @args
    if ($LASTEXITCODE -ne 0) { Write-Error $out; return }
    $binaryArgs = @()
    $foundFind = $false
    foreach ($line in ($out -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split ' ', 2
        $op = $parts[0]
        $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
        switch ($op) {
            'FIND' {
                $path = FIND $rest
                $binaryArgs += "--binary"
                $binaryArgs += "$rest=$path"
                $foundFind = $true
            }
            'DIR'   { Set-Location $rest }
            'SET'   { $kv = $rest -split '=', 2; Set-Item "env:$($kv[0])" $kv[1] }
            'UNSET' { Remove-Item "env:$rest" -ErrorAction SilentlyContinue }
            default { Write-Output $line }
        }
    }
    if ($foundFind) {
        $out = & ::ORBITER:: @args @binaryArgs
        if ($LASTEXITCODE -ne 0) { Write-Error $out; return }
        foreach ($line in ($out -split "`n")) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            $parts = $line -split ' ', 2
            $op = $parts[0]
            $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
            switch ($op) {
                'DIR'   { Set-Location $rest }
                'SET'   { $kv = $rest -split '=', 2; Set-Item "env:$($kv[0])" $kv[1] }
                'UNSET' { Remove-Item "env:$rest" -ErrorAction SilentlyContinue }
                default { Write-Output $line }
            }
        }
    }
}
```

Also update `_OrbiterHook` with the same FIND round-trip before the SET/DEPART loop.

- [ ] **Step 8: Run tests**

```
go test ./internal/commands/ -v
```

Expected: all existing tests pass, `TestDirective_FIND` passes.

- [ ] **Step 9: Commit**

```
git add internal/commands/executor.go internal/commands/executor_test.go \
        integrations/bash/hook.bash integrations/zsh/hook.zsh \
        integrations/fish/hook.fish integrations/powershell/hook.ps1
git commit -m "feat: add FIND directive + shell hook round-trip for binary resolution"
```

---

### Task 2: --binary flags, executor signature changes, and binary threading to starchart

**Files:**
- Modify: `internal/commands/lifecycle.go`
- Modify: `internal/commands/executor.go`
- Modify: `internal/starchart/lifecycle.go`

**Interfaces:**
- Consumes: `Directive{Op: "FIND", Key: name}` from Task 1
- Produces:
  - `func (e *Executor) Scan(ctx context.Context, target string, binaries map[string]string) ([]Directive, error)`
  - `func (e *Executor) Calibrate(ctx context.Context, target string, binaries map[string]string) ([]Directive, error)`
  - `func (e *Executor) Chart(ctx context.Context, target string, binaries map[string]string) ([]Directive, error)`
  - `Survey` and `Retro` keep their existing `error` return type — they don't invoke integrations and need no pre-flight
  - `func (e *Executor) Hook(ctx context.Context, cwd, current string, binaries map[string]string) ([]Directive, error)` (signature unchanged but binaries added)
  - `func (e *Executor) Jump(ctx context.Context, target string, confirmed bool, binaries map[string]string) ([]Directive, error)`
  - `func (sc *StarChart) ScanBranch(ctx context.Context, entityID string, binaries map[string]string) (BranchScanResult, error)`
  - `func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string, binaries map[string]string) (BranchCalibrateResult, error)`
  - `BinaryDrift bool` added to `ResourceScanResult`

**Background — why signatures change:** `Scan`, `Calibrate`, `Chart` currently return `error`. They need to return `([]Directive, error)` so pre-flight FIND directives can be returned to the lifecycle command, which prints them to stdout and exits. The shell wrapper then does the round-trip. On re-call with `--binary` flags, these methods run normally and return `nil, nil` for directives.

- [ ] **Step 1: Add `--binary` parsing helper to `lifecycle.go`**

At the top of `internal/commands/lifecycle.go`, after the imports, add a helper to parse `--binary name=path` StringSlice flags into a map:

```go
// ParseBinaries converts a []string of "name=path" entries (from --binary flags)
// into a map. Malformed entries are silently skipped.
func ParseBinaries(entries []string) map[string]string {
    m := make(map[string]string, len(entries))
    for _, e := range entries {
        idx := strings.Index(e, "=")
        if idx <= 0 {
            continue
        }
        m[e[:idx]] = e[idx+1:]
    }
    return m
}
```

Add `"strings"` to the imports.

- [ ] **Step 2: Write the test for `parseBinaries`**

In `internal/commands/lifecycle.go` there is no test file — tests for this helper belong in `internal/commands/executor_test.go` or a new `internal/commands/lifecycle_test.go`. Use `executor_test.go`:

```go
func TestParseBinaries(t *testing.T) {
    tests := []struct {
        input []string
        want  map[string]string
    }{
        {nil, map[string]string{}},
        {[]string{"brew=/opt/homebrew/bin/brew"}, map[string]string{"brew": "/opt/homebrew/bin/brew"}},
        {[]string{"git=/usr/bin/git", "brew=/opt/homebrew/bin/brew"}, map[string]string{"git": "/usr/bin/git", "brew": "/opt/homebrew/bin/brew"}},
        {[]string{"malformed"}, map[string]string{}},
        {[]string{"=noval"}, map[string]string{}},
    }
    for _, tc := range tests {
        got := commands.ParseBinaries(tc.input)
        if !reflect.DeepEqual(got, tc.want) {
            t.Errorf("ParseBinaries(%v) = %v, want %v", tc.input, got, tc.want)
        }
    }
}
```

Note: the function must be exported for testing — name it `ParseBinaries` (capital P) in `lifecycle.go`.

- [ ] **Step 3: Run test to verify it fails**

```
go test ./internal/commands/ -run TestParseBinaries -v
```

Expected: FAIL — `commands.ParseBinaries` undefined (function not written yet).

- [ ] **Step 4: Add `--binary` flag to all lifecycle commands in `lifecycle.go`**

For every command that invokes an executor lifecycle method (`scan`, `calibrate`, `chart`, `survey`, `retro`, `hook`, `jump`), add a `--binary` StringSlice flag and pass the parsed map to the executor. Below is the pattern for `newScanCmd` — apply the same to all other commands:

```go
func newScanCmd(d *deps) *cobra.Command {
    var binaryEntries []string
    cmd := &cobra.Command{
        Use:   "scan [target]",
        Short: "Verify reality — \"What does reality currently look like?\"",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            target := ""
            if len(args) > 0 {
                target = args[0]
            }
            directives, err := NewExecutor(d.sc, d.renderer).Scan(cmd.Context(), target, ParseBinaries(binaryEntries))
            if err != nil {
                return err
            }
            for _, dir := range directives {
                fmt.Println(dir.String())
            }
            return nil
        },
    }
    cmd.Flags().StringSliceVar(&binaryEntries, "binary", nil, "resolved binary path: name=path (repeatable)")
    return cmd
}
```

Apply this same pattern to `newCalibrateCmd` and `newChartCmd`. `newSurveyCmd` and `newRetroCmd` keep their existing `error` return — no `--binary` flag needed there. For `newHookCmd` and `newJumpCmd`, add the `--binary` flag and thread `ParseBinaries(binaryEntries)` into the executor call.

For `newHookCmd`, the current executor call is:
```go
directives, err := exec.Hook(ctx, cwd, current)
```
Change to:
```go
directives, err := exec.Hook(ctx, cwd, current, ParseBinaries(binaryEntries))
```

For `newJumpCmd`:
```go
directives, err := exec.Jump(cmd.Context(), target, yes, ParseBinaries(binaryEntries))
```

- [ ] **Step 5: Update executor method signatures in `executor.go`**

Change `Scan`, `Calibrate`, `Chart`, `Hook`, `Jump` signatures. `Scan`/`Calibrate`/`Chart` return `([]Directive, error)` instead of `error` so pre-flight FIND directives can be returned. `Survey` and `Retro` keep `error` — they don't invoke integrations.

```go
func (e *Executor) Scan(ctx context.Context, target string, binaries map[string]string) ([]Directive, error) { ... }
func (e *Executor) Calibrate(ctx context.Context, target string, binaries map[string]string) ([]Directive, error) { ... }
func (e *Executor) Chart(ctx context.Context, target string, binaries map[string]string) ([]Directive, error) { ... }
func (e *Executor) Hook(ctx context.Context, cwd, current string, binaries map[string]string) ([]Directive, error) { ... }
func (e *Executor) Jump(ctx context.Context, target string, confirmed bool, binaries map[string]string) ([]Directive, error) { ... }
```

For now, update each method's signature and internal body to pass `binaries` down to `sc.ScanBranch`/`sc.CalibrateBranch`. The survey and retro commands don't use binaries — they keep `binaries` out of the signature. `Survey` just returns `nil, nil` for directives on success (it only displays data). For `Chart` and `Scan`, change the internal helper methods:

```go
func (e *Executor) scanBranchEntity(ctx context.Context, alias models.Alias, binaries map[string]string) error {
    result, err := e.sc.ScanBranch(ctx, alias.ID, binaries)
    ...
}
```

Pass `binaries` through all internal helpers (`scanBranchEntity`, `scanCallsignEntity`, `calibrateEntity`, etc.).

- [ ] **Step 6: Update `starchart/lifecycle.go` — add binaries to ScanBranch/CalibrateBranch and all callees**

`ScanBranch` becomes:
```go
func (sc *StarChart) ScanBranch(ctx context.Context, entityID string, binaries map[string]string) (BranchScanResult, error) {
```

`CalibrateBranch`:
```go
func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string, binaries map[string]string) (BranchCalibrateResult, error) {
```

`scanResource`:
```go
func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, lb LeveledBranch, binaries map[string]string) (ResourceScanResult, error) {
```

`calibrateResource`:
```go
func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, lb LeveledBranch, binaries map[string]string) (ResourceCalibrateResult, error) {
```

Also add `BinaryDrift bool` field to `ResourceScanResult`:
```go
type ResourceScanResult struct {
    Resource     models.Resource
    Report       integrations.StateReport
    BeaconStatus string
    BinaryDrift  bool // true when fresh binary path ≠ stored config path; Captain approval required
}
```

In `scanResource`, after building `rc` via `BuildResolvedContext`, inject the binary paths into `rc.Binaries`:
```go
rc := BuildResolvedContext(r, lb, integration.Meta())
if len(binaries) > 0 {
    rc.Binaries = make(map[string]string)
    for _, name := range integration.Meta().Integration.Binaries {
        if path, ok := binaries[name]; ok {
            rc.Binaries[name] = path
        }
    }
}
report := integration.Scan(rc)
```

For `ScanCallsign`, `CalibrateCallsign`, `ScanTransponder`, `CalibrateTransponder` — transponders don't declare binaries (they're credential/file stores), so leave their signatures unchanged for now.

Also update `Chart` in `executor.go` which calls `sc.ScanBranch` — pass binaries through.

- [ ] **Step 7: Run tests**

```
go test ./... 
```

Expected: all tests pass. The `parseBinaries` test passes. No compile errors.

- [ ] **Step 8: Commit**

```
git add internal/commands/executor.go internal/commands/executor_test.go \
        internal/commands/lifecycle.go internal/starchart/lifecycle.go
git commit -m "feat: thread --binary flags from shell through executor to starchart lifecycle"
```

---

### Task 3: Binary pre-flight FIND emission in executor

**Files:**
- Modify: `internal/commands/executor.go`

**Interfaces:**
- Consumes: `Directive{Op: "FIND", Key: name}` (Task 1), executor signatures from Task 2
- Produces: executor methods return FIND directives when any declared binary is absent from the provided `binaries` map; the shell wrapper resolves them and re-calls

**Background:** On the first `orbiter scan` invocation (no `--binary` flags), Orbiter must identify which binaries are declared by active integrations, emit `FIND` directives for each undeclared one, and return immediately. The shell wrapper resolves them and re-calls with `--binary` flags. On the second call, all binaries are present and the normal scan proceeds.

- [ ] **Step 1: Add `collectNeededBinaries` helper to `executor.go`**

```go
// collectNeededBinaries returns the deduplicated list of binary names declared
// in manifests for all resources attached to entityID, in role order.
func (e *Executor) collectNeededBinaries(ctx context.Context, entityID string) ([]string, error) {
    lb, err := e.sc.LeveledBranchCrawl(ctx, entityID)
    if err != nil {
        return nil, err
    }
    seen := make(map[string]bool)
    var names []string
    for _, level := range lb.Levels {
        for _, r := range level.Resources {
            integ, ok := integrations.Default.Get(r.Role, r.Brand)
            if !ok {
                continue
            }
            for _, name := range integ.Meta().Integration.Binaries {
                if !seen[name] {
                    seen[name] = true
                    names = append(names, name)
                }
            }
        }
    }
    return names, nil
}
```

Add the `bundle "github.com/Kenttleton/orbiter/integrations"` import if not already present (it is — check root.go uses it via `isCatalogBrand`). Use the same `integrations.Default` registry.

- [ ] **Step 2: Write the test**

The test needs a StarChart with a resource whose integration declares a binary. Use the existing test helpers in `executor_test.go`:

```go
func TestScan_EmitsFINDForMissingBinaries(t *testing.T) {
    // If the integration for a resource declares binaries = ["brew"]
    // and no --binary flags are provided, Scan must return FIND directives.
    // This test uses a mock/stub integration that declares binaries.
    // See executor_test.go for how stubs are set up — follow the same pattern.
    // For now, verify collectNeededBinaries returns declared names.
    // Full round-trip is verified manually via the shell hook.
}
```

Note: if the existing executor tests use an in-memory StarChart with stubs (see `internal/commands/stubs.go`), add a stub integration that declares `Binaries: []string{"testbinary"}` and verify `collectNeededBinaries` returns `["testbinary"]`.

- [ ] **Step 3: Add pre-flight logic to `Scan`, `Calibrate`, `Chart`**

At the top of each executor method (before calling `sc.ScanBranch`/etc.), add the pre-flight check:

```go
func (e *Executor) Scan(ctx context.Context, target string, binaries map[string]string) ([]Directive, error) {
    alias, err := e.resolveTarget(ctx, target)
    if err != nil {
        return nil, err
    }

    // Pre-flight: emit FIND directives for any binary declared by active
    // integrations that wasn't provided via --binary flags.
    needed, err := e.collectNeededBinaries(ctx, alias.ID)
    if err != nil {
        return nil, fmt.Errorf("collect declared binaries: %w", err)
    }
    var findDirectives []Directive
    for _, name := range needed {
        if _, ok := binaries[name]; !ok {
            findDirectives = append(findDirectives, Directive{Op: "FIND", Key: name})
        }
    }
    if len(findDirectives) > 0 {
        return findDirectives, nil
    }

    // All binaries provided — proceed normally.
    parsed, _ := models.ParseID(alias.ID)
    switch parsed.EntityType {
    case models.EntityTypeCallsign:
        return nil, e.scanCallsignEntity(ctx, alias, binaries)
    case models.EntityTypeTransponder:
        return nil, e.scanTransponderEntity(ctx, alias)
    default:
        return nil, e.scanBranchEntity(ctx, alias, binaries)
    }
}
```

Apply the same pre-flight pattern to `Calibrate` and `Chart`.

For `Hook`, pre-flight is also needed — the hook fires on `cd` and needs binaries for the ScanBranch call that follows:

```go
func (e *Executor) Hook(ctx context.Context, cwd, current string, binaries map[string]string) ([]Directive, error) {
    // ... existing alias/cwd resolution ...

    needed, err := e.collectNeededBinaries(ctx, alias.ID)
    if err != nil {
        return nil, err
    }
    var findDirectives []Directive
    for _, name := range needed {
        if _, ok := binaries[name]; !ok {
            findDirectives = append(findDirectives, Directive{Op: "FIND", Key: name})
        }
    }
    if len(findDirectives) > 0 {
        return findDirectives, nil
    }
    // ... existing ScanBranch + export directives logic ...
}
```

- [ ] **Step 4: Run tests**

```
go test ./...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```
git add internal/commands/executor.go internal/commands/executor_test.go
git commit -m "feat: emit FIND directives pre-flight when declared binaries are absent"
```

---

### Task 4: Binary path persistence after Init

**Files:**
- Modify: `internal/starchart/init.go`

**Interfaces:**
- Consumes: `StateReport.BinaryPath string` (already in `integrations/types.go`)
- Produces: after `InitResource`/`InitTransponder`, `resource.Config` JSON contains `"binary_path": "/opt/homebrew/bin/brew"` — consumed by Task 5 (drift detection reads this field)

**Background:** When an integration's `initialize` handler succeeds and returns a non-empty `BinaryPath`, Orbiter must persist it to the resource's `config` column in the StarChart. The `config` column is a JSON string (`"{}"` by default). We merge `binary_path` into the existing config object. This is the StarChart's approved binary path — the anchor for drift detection.

The `sc.Update(ctx, "resources", id, map[string]any{"config": newJSON})` method already exists in `crud.go`.

- [ ] **Step 1: Write the test**

Add to `internal/starchart/init_test.go`. The test pattern mirrors `TestInitResourceWithIntegration` — register a stub, create a resource, call `InitResource`, read back the config. Use `testify/require` which is already imported.

```go
// binaryIntegration is a stub that returns a BinaryPath from Init.
type binaryIntegration struct{}

func (b *binaryIntegration) Meta() integrations.Manifest {
    return integrations.Manifest{
        Integration: integrations.ManifestIntegration{
            Brand:    "testbrew",
            Binaries: []string{"testbrew"},
        },
    }
}
func (b *binaryIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport {
    return integrations.DetectReport{Detected: true}
}
func (b *binaryIntegration) Init(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test", BinaryPath: "/usr/local/bin/testbrew"}
}
func (b *binaryIntegration) Scan(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}
func (b *binaryIntegration) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{Present: true, Manager: "test"}
}

func TestInitResource_PersistsBinaryPath(t *testing.T) {
    ctx := context.Background()
    reg := integrations.NewRegistry(nil)
    reg.Register("tool", "testbrew", &binaryIntegration{})
    sc := testDBWithRegistry(t, reg)

    r, err := sc.CreateResource(ctx, "testbrew-tool", "tool", "testbrew", `[]`, `{}`)
    require.NoError(t, err)

    require.NoError(t, sc.InitResource(ctx, r.ID))

    var updated models.Resource
    require.NoError(t, sc.Get(ctx, "resources", r.ID, &updated))

    var cfg map[string]any
    require.NoError(t, json.Unmarshal([]byte(updated.Config), &cfg))

    got, ok := cfg["binary_path"]
    require.True(t, ok, "binary_path not persisted to resource config")
    require.Equal(t, "/usr/local/bin/testbrew", got)
}
```

Add `"encoding/json"` to `init_test.go` imports if not already there.

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/starchart/ -run TestInitResource_PersistsBinaryPath -v
```

Expected: FAIL.

- [ ] **Step 3: Implement binary path persistence in `init.go`**

Add a helper that merges `binary_path` into the resource's config JSON:

```go
// persistBinaryPath merges binary_path into the resource/transponder's config JSON.
// It is a no-op if path is empty.
func (sc *StarChart) persistBinaryPath(ctx context.Context, table, entityID, path string) error {
    if path == "" {
        return nil
    }
    // Fetch existing config.
    var cfg map[string]any
    if table == "resources" {
        var r models.Resource
        if err := sc.Get(ctx, "resources", entityID, &r); err != nil {
            return fmt.Errorf("get resource for config update: %w", err)
        }
        if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil || cfg == nil {
            cfg = make(map[string]any)
        }
    } else {
        var tp models.Transponder
        if err := sc.Get(ctx, "transponders", entityID, &tp); err != nil {
            return fmt.Errorf("get transponder for config update: %w", err)
        }
        if err := json.Unmarshal([]byte(tp.Config), &cfg); err != nil || cfg == nil {
            cfg = make(map[string]any)
        }
    }
    cfg["binary_path"] = path
    updated, err := json.Marshal(cfg)
    if err != nil {
        return fmt.Errorf("marshal updated config: %w", err)
    }
    return sc.Update(ctx, table, entityID, map[string]any{"config": string(updated)})
}
```

Call it in `InitResource` after `applyStateReport`:

```go
func (sc *StarChart) InitResource(ctx context.Context, resourceID string) error {
    // ... existing logic ...
    report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))
    if err := sc.applyStateReport(ctx, resourceID, report); err != nil {
        return err
    }
    return sc.persistBinaryPath(ctx, "resources", resourceID, report.BinaryPath)
}
```

Same in `InitTransponder`:
```go
    report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))
    if err := sc.applyStateReport(ctx, transponderID, report); err != nil {
        return err
    }
    return sc.persistBinaryPath(ctx, "transponders", transponderID, report.BinaryPath)
```

- [ ] **Step 4: Run the test**

```
go test ./internal/starchart/ -run TestInitResource_PersistsBinaryPath -v
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```
git add internal/starchart/init.go internal/starchart/init_test.go
git commit -m "feat: persist StateReport.BinaryPath to resource config after Init"
```

---

### Task 5: Binary drift detection in scanResource

**Files:**
- Modify: `internal/starchart/lifecycle.go`

**Interfaces:**
- Consumes: `ResourceScanResult.BinaryDrift bool` (Task 2), `binaries map[string]string` param (Task 2), `resource.Config["binary_path"]` stored by Task 4
- Produces: when fresh binary path ≠ stored path — beacon set to `drifted`, `BinaryDrift: true` returned, integration NOT called; `calibrateResource` skips calibration on `BinaryDrift`

**Background:** Orbiter's core purpose is drift detection. Every `scan` passes a freshly resolved binary path (from the shell's FIND function). If that path differs from what's stored in the StarChart config (the Captain-approved path), Orbiter updates the beacon to `drifted` with a clear observation and halts — it does NOT call the integration and does NOT update the StarChart config. The Captain must review and approve the change before StarChart is updated. With passthrough mode (future: `--yes` flag), this step could be automated, but that is out of scope here.

- [ ] **Step 1: Write the test**

In `internal/starchart/lifecycle_test.go` (or the existing test file for lifecycle), add:

```go
func TestScanBranch_BinaryPathDrift(t *testing.T) {
    sc := newTestStarChart(t)
    ctx := context.Background()

    // Set up a resource with config containing binary_path = "/old/path/brew"
    // and a stub integration that declares Binaries: ["brew"].
    // The fresh binary path passed in binaries["brew"] = "/new/path/brew" (different).

    binaries := map[string]string{"brew": "/new/path/brew"}
    result, err := sc.ScanBranch(ctx, planetID, binaries)
    if err != nil {
        t.Fatal(err)
    }
    if len(result.Resources) == 0 {
        t.Fatal("expected resource scan result")
    }
    rr := result.Resources[0]
    if !rr.BinaryDrift {
        t.Error("expected BinaryDrift=true when fresh path differs from stored path")
    }
    if rr.BeaconStatus != models.BeaconStatusDrifted {
        t.Errorf("beacon status = %q, want %q", rr.BeaconStatus, models.BeaconStatusDrifted)
    }
    // Verify the integration's Scan was NOT called (stub tracks whether Scan was called).
    if stubIntegration.ScanCalled {
        t.Error("integration.Scan must not be called when binary path has drifted")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/starchart/ -run TestScanBranch_BinaryPathDrift -v
```

Expected: FAIL.

- [ ] **Step 3: Implement drift detection in `scanResource`**

At the top of `scanResource`, before calling `integration.Scan`, check for binary path drift:

```go
func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, lb LeveledBranch, binaries map[string]string) (ResourceScanResult, error) {
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

    // Binary path drift check: compare freshly resolved paths against StarChart config.
    // If any declared binary path has changed, halt and notify the Captain.
    if drift, obs := binaryPathDrift(r, integration.Meta().Integration.Binaries, binaries); drift {
        if err := sc.setBeaconStatus(ctx, r.ID, models.BeaconStatusDrifted, obs); err != nil {
            return ResourceScanResult{}, err
        }
        return ResourceScanResult{
            Resource:     r,
            BeaconStatus: models.BeaconStatusDrifted,
            BinaryDrift:  true,
            Report: integrations.StateReport{
                Observations: obs,
            },
        }, nil
    }

    rc := BuildResolvedContext(r, lb, integration.Meta())
    if len(binaries) > 0 {
        rc.Binaries = make(map[string]string)
        for _, name := range integration.Meta().Integration.Binaries {
            if path, ok := binaries[name]; ok {
                rc.Binaries[name] = path
            }
        }
    }
    report := integration.Scan(rc)
    status := scanBeaconStatus(report)
    if err := sc.setBeaconStatus(ctx, r.ID, status, report.Observations); err != nil {
        return ResourceScanResult{}, err
    }
    return ResourceScanResult{Resource: r, Report: report, BeaconStatus: status}, nil
}
```

Add the `binaryPathDrift` helper below `scanResource`:

```go
// binaryPathDrift returns true if any declared binary's fresh path (from the shell
// FIND round-trip) differs from the path stored in the resource's config.
// Returns the drift observations for the Captain.
// A missing stored path is not drift — it means the binary was never initialized.
func binaryPathDrift(r models.Resource, declared []string, fresh map[string]string) (bool, []string) {
    if len(declared) == 0 || len(fresh) == 0 {
        return false, nil
    }
    var cfg map[string]any
    if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil || cfg == nil {
        return false, nil
    }
    stored, _ := cfg["binary_path"].(string)
    if stored == "" {
        return false, nil // not yet initialized; not a drift
    }
    var obs []string
    drifted := false
    for _, name := range declared {
        freshPath, ok := fresh[name]
        if !ok {
            continue
        }
        if freshPath != stored {
            obs = append(obs, fmt.Sprintf("binary path drifted for %q: stored=%q, found=%q — Captain approval required to update StarChart", name, stored, freshPath))
            drifted = true
        }
    }
    return drifted, obs
}
```

Note: this assumes one `binary_path` in config covers the primary binary (what `StateReport.BinaryPath` returns). If integrations declare multiple binaries, this needs extension — but most integrations declare exactly one.

In `calibrateResource`, skip calibration if `BinaryDrift` is true:

```go
func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, lb LeveledBranch, binaries map[string]string) (ResourceCalibrateResult, error) {
    scanResult, err := sc.scanResource(ctx, r, lb, binaries)
    if err != nil {
        return ResourceCalibrateResult{}, err
    }
    // Binary path drift requires Captain approval — do not attempt calibration.
    if scanResult.BinaryDrift {
        return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "drifted"}, nil
    }
    // Export-role resources must always run Calibrate even when healthy.
    if scanResult.BeaconStatus == models.BeaconStatusHealthy && r.Role != integrations.ResourceRoleExport {
        return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "healthy"}, nil
    }
    // ... existing calibrate logic unchanged ...
```

Add `"encoding/json"` to the imports if not already there (it's already used via `json.Marshal` in `setBeaconStatus`).

- [ ] **Step 4: Run the test**

```
go test ./internal/starchart/ -run TestScanBranch_BinaryPathDrift -v
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```
git add internal/starchart/lifecycle.go internal/starchart/lifecycle_test.go
git commit -m "feat: halt on binary path drift and notify Captain; no auto-StarChart update"
```

---

### Task 6: Delete FindBinary and rewrite populateBinaries

**Files:**
- Modify: `internal/integrations/filesystem.go`
- Delete: `internal/wasm/resolver.go`
- Delete: `internal/wasm/resolver_test.go`
- Modify: `internal/wasm/loader.go`

**Interfaces:**
- Consumes: `rc.Binaries` now populated by `scanResource`/`calibrateResource` from Task 5
- Produces: `filesystem.go` contains ONLY `filesystemIntegration` struct, `pathFromConfig`, and `init()` — no shell invocation; `populateBinaries` removed from loader.go

**Background:** `FindBinary` in `filesystem.go` was the wrong location for binary discovery — filesystem handles OS paths (stat, mkdir) only. `resolver.go` was a thin wrapper around it. Both are now dead code because binaries arrive pre-resolved via `--binary` flags and are injected into `rc.Binaries` in the starchart lifecycle layer. `populateBinaries` in `loader.go` called `ResolveBinaries` (which called `FindBinary`) — this is now replaced by the upstream injection.

- [ ] **Step 1: Verify filesystem_test.go covers only OS-path behavior**

Read `internal/integrations/filesystem_test.go`. It likely tests `FindBinary`. Those tests must be deleted along with the function. OS-path tests (for `filesystemIntegration.Init`/`Scan`/`Calibrate`) must be preserved.

```
go test ./internal/integrations/ -v
```

Note which tests pass currently. Any test that calls `FindBinary` or `ResolveBinaries` will be deleted.

- [ ] **Step 2: Delete `FindBinary` and related from `filesystem.go`**

Remove these functions entirely from `internal/integrations/filesystem.go`:
- `FindBinary(name, osName string) string`
- `findBinaryUnix(name string) string`
- `findBinaryWindows(name string) string`
- `runShellFind(shell, flag, expr string) string`

Remove the now-unused imports: `"os/exec"` and `"strings"`. Keep `"encoding/json"` and `"os"` (used by `pathFromConfig` and `os.MkdirAll`/`os.Stat`).

The resulting file should contain only: `filesystemIntegration` struct, `Meta()`, `Detect()`, `Init()`, `Scan()`, `Calibrate()`, `pathFromConfig()`, and `init()`.

- [ ] **Step 3: Delete `resolver.go` and `resolver_test.go`**

```
rm internal/wasm/resolver.go internal/wasm/resolver_test.go
```

- [ ] **Step 4: Remove `populateBinaries` from `loader.go`**

Delete the `populateBinaries` method entirely:

```go
// DELETE this entire method:
func (w *WASMIntegration) populateBinaries(ctx integrations.ResolvedContext) integrations.ResolvedContext {
    if len(w.manifest.Integration.Binaries) == 0 {
        return ctx
    }
    ctx.Binaries = ResolveBinaries(w.manifest.Integration.Binaries, ctx.Platform.OS)
    return ctx
}
```

Remove the three calls to `populateBinaries` in `Init`, `Scan`, and `Calibrate`:

```go
// In Init — remove this line:
ctx = w.populateBinaries(ctx)

// In Scan — remove this line:
ctx = w.populateBinaries(ctx)

// In Calibrate — remove this line:
ctx = w.populateBinaries(ctx)
```

The `ctx.Binaries` field is now populated upstream by `scanResource`/`calibrateResource` in `starchart/lifecycle.go` before the integration's `Scan`/`Calibrate` is called. No change needed to the `invoke` flow.

- [ ] **Step 5: Delete any `FindBinary` tests from `filesystem_test.go`**

Remove any test functions in `internal/integrations/filesystem_test.go` that test `FindBinary`, `findBinaryUnix`, `findBinaryWindows`, or `runShellFind`. Keep tests for `filesystemIntegration.Init`, `Scan`, `Calibrate`.

- [ ] **Step 6: Run full test suite**

```
go test ./...
```

Expected: all pass. No references to `FindBinary` or `ResolveBinaries` remain.

- [ ] **Step 7: Verify no stray references**

```
grep -r "FindBinary\|ResolveBinaries\|findBinaryUnix\|findBinaryWindows\|runShellFind" \
    internal/ integrations/ cmd/
```

Expected: no output.

- [ ] **Step 8: Commit**

```
git add internal/integrations/filesystem.go internal/integrations/filesystem_test.go \
        internal/wasm/loader.go
git rm internal/wasm/resolver.go internal/wasm/resolver_test.go
git commit -m "refactor: delete FindBinary; binaries now resolved via shell directive round-trip"
```

---

### Task 7: Verification sweep

**Files:** None (read-only + manual tests)

**Goal:** Confirm the full binary resolution pipeline works end-to-end.

- [ ] **Step 1: Full test suite**

```
go test ./... -count=1
```

Expected: all tests pass, no skips related to this feature.

- [ ] **Step 2: Build**

```
go build ./cmd/orbiter
```

Expected: clean build, no errors.

- [ ] **Step 3: Verify no shell-invocation code in filesystem.go**

```
grep -n "exec\|shell\|FIND\|command -v\|which" internal/integrations/filesystem.go
```

Expected: no matches.

- [ ] **Step 4: Verify FIND directive round-trip in bash**

In a bash shell with the hook sourced, with a resource that declares `binaries = ["brew"]` in its manifest, run:

```bash
orbiter scan
```

On first call (no `--binary` flags): should print `FIND brew` and exit 0 — the hook intercepts this, calls `FIND brew`, then re-calls `orbiter scan --binary brew=/opt/homebrew/bin/brew`.

To observe the first-call behavior directly:
```bash
::ORBITER:: scan
```

Expected output: `FIND brew` (directive line only, exits 0).

With `--binary` provided:
```bash
::ORBITER:: scan --binary brew=/opt/homebrew/bin/brew
```

Expected: normal scan output.

- [ ] **Step 5: Verify drift detection**

Set a resource's config to have `binary_path: "/old/path/brew"`. Run:

```bash
::ORBITER:: scan --binary brew=/new/path/brew
```

Expected: beacon updated to `drifted`, output includes observation `"binary path drifted for \"brew\": stored=\"/old/path/brew\", found=\"/new/path/brew\" — Captain approval required to update StarChart"`, scan halts (integration not invoked).

- [ ] **Step 6: Commit (if any minor fixes)**

```
go test ./...
git add -p
git commit -m "fix: verification sweep corrections"
```
