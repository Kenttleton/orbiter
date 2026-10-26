# FIND Interface Specification

> **Status:** Implemented by [`2026-10-26-find-implementation.md`](2026-10-26-find-implementation.md). Where this spec and the I/O contract spec differ, that plan's Reconciliation Decisions apply: the search method belongs to the shell integration (D1), and directives are `FIND_X <txn> <key>` lines (D2).
> **Depends on:** [`2026-06-24-filesystem-spec.md`](2026-06-24-filesystem-spec.md) ✅ implemented (commits df1ff72..6f97bab) — provides `ParsePath`, `Path`, `PathFromConfig`; native lifecycle handlers for filesystem role; `populateBinaries` stub in `loader.go`
> **Blocks:** exec.Command / run_command execution model refactor, Present/Reachable refactor
> **Followed by:** [`2026-06-22-binary-resolution-shell-directive.md`](2026-06-22-binary-resolution-shell-directive.md) (unblocked after dependent specs complete)

**Goal:** Define FIND as a generic, extensible set of Orbiter-owned shell functions for binary, file, directory, and environment variable discovery that all four supported shells implement identically.

**Architecture:** FIND functions live in each shell's hook script and are transparently owned by Orbiter — not integration logic. Orbiter emits FIND directives through the directive protocol; the shell executes the matching function and returns JSON. Orbiter injects all results into `ResolvedContext` before invoking any WASM handler. The callback transport (shell → Orbiter reply path) is out of scope here; it is defined in the execution model refactor spec.

**Filesystem resolution is a lifecycle responsibility.** Finding and locating files, binaries, and directories is the lifecycle's job during branch crawls — not any WASM integration's job. This applies to two cases: (1) filesystem-role resources, where the lifecycle detects drift at the recorded path (`Present=false` or `Reachable=false` — see below); (2) all other integrations that declare binary or file needs in their manifest, where the lifecycle pre-resolves those via FIND directives and injects the results into `ResolvedContext` before the WASM handler is ever called. WASM handlers receive a fully-resolved context; they never discover paths themselves. The FIND implementation therefore lives in `internal/starchart/lifecycle.go`, replacing the temporary `populateBinaries` stub in `loader.go` that is left behind by the filesystem spec cleanup.

**Tech Stack:** Shell functions (bash, zsh, fish, PowerShell), JSON output, Go `ResolvedContext` injection, manifest `[find]` section

---

## 1. Why FIND Exists

`FindBinary` in `internal/integrations/filesystem.go` violates architecture: it branch-checks `$SHELL` (`strings.HasSuffix(shell, "bash")`), calls `exec.Command` with shell flags to invoke a shell function, and delegates to `sh -c command -v` as a fallback. `filesystem.go` must only do OS-path work via Go stdlib — it must never know what shell it is in. The `FindBinary` family is deleted as part of the filesystem spec (dependency above).

FIND replaces this with a two-layer design:

- **Shell layer** — each shell knows how to ask its own environment whether a binary or file exists. Orbiter calls FIND; the shell translates to PATH walks, `find`, `Get-ChildItem`, `printenv`, etc.
- **Lifecycle layer** (`lifecycle.go`) — reads each integration's `[find]` manifest section during the branch crawl, emits FIND directives, receives JSON results, normalizes paths via `filesystem.ParsePath`, and injects the resolved values into `ResolvedContext` before dispatching to the WASM handler.

Integrations never call FIND. They declare what they need in their manifest `[find]` section and the lifecycle delivers a fully-resolved `ResolvedContext.Binaries`, `ResolvedContext.Files`, `ResolvedContext.Dirs`, and `ResolvedContext.Envs` before any handler is invoked.

---

## 2. FIND Command Types

### 2.1 `FIND_BINARY <name>`

Searches the shell's PATH for all executables named `<name>`.

**Usage:**

```text
FIND_BINARY brew
FIND_BINARY node
FIND_BINARY python3
```

**Semantics:**

- Walks the shell's PATH entries and returns every executable matching `<name>`.
- Returns all matches, not just the first. Orbiter or the Captain decides which path to record in the StarChart — a single guess is insufficient when multiple versions coexist (e.g., multiple Python installs, system vs. manager-installed binaries).
- Empty `matches` array = binary not in PATH. Valid result, not an error.
- All returned paths are from PATH by definition; there is no `in_path` field. Whether the StarChart-recorded path is reachable via PATH is determined by a separate presence check (see Present/Reachable refactor spec).

**Output schema:**
```json
{ "status": "ok", "matches": ["/opt/homebrew/bin/brew", "/usr/local/bin/brew"] }
{ "status": "ok", "matches": [] }
{ "status": "error", "message": "unexpected error during PATH walk" }
```

---

### 2.2 `FIND_FILE <pattern> [location]`

Recursively searches for files matching `<pattern>` under `[location]`. Defaults to `$ORBITER_CWD` when `location` is omitted.

**Usage:**

```text
FIND_FILE "*.json"
FIND_FILE "*.lock" /home/user/project
FIND_FILE ".gitconfig" ~
FIND_FILE "package.json" /project/root
```

**Semantics:**

- `pattern` is a glob expression. Always quote it to prevent premature shell expansion.
- `location` defaults to `$ORBITER_CWD` — the CWD Orbiter resolved at startup, not the shell's current working directory. Orbiter always has a resolved CWD.
- Search is **fully recursive**: descends into all subdirectories until no more children exist. This is the correct default for software projects where files of interest (lock files, configs, manifests) may be nested at any depth.
- Returns all matches as a JSON array. Empty array = no matches, which is `"ok"` — not an error.
- `"not_found"` status means the search root (`location`) does not exist as a directory.

**Output schema:**
```json
{ "status": "ok", "matches": ["/project/package.json", "/project/sub/package.json"] }
{ "status": "ok", "matches": [] }
{ "status": "not_found", "message": "location does not exist: /nonexistent" }
{ "status": "error", "message": "permission denied reading /protected" }
```

---

### 2.3 `FIND_DIR <path>`

Checks whether a directory exists at a known `<path>` and returns its normalized form.

**Usage:**

```text
FIND_DIR /usr/local/lib
FIND_DIR ~/.config/orbiter
FIND_DIR /opt/homebrew
```

**Semantics:**

- Existence check on a known path — not a glob search.
- Useful for verifying config directories, installation roots, and manager-managed paths (e.g., homebrew prefix, nvm root) without integrations hardcoding platform-specific paths.
- Returns the path as-is from the shell. Orbiter normalizes it via `filesystem.ParsePath` before storing.
- Empty `path` with `"exists": false` is a valid result — the directory is absent, no error occurred.

**Output schema:**
```json
{ "status": "ok", "path": "/usr/local/lib", "exists": true }
{ "status": "ok", "path": "", "exists": false }
{ "status": "error", "message": "..." }
```

---

### 2.4 `FIND_ENV <name>`

Reads an environment variable's value from the current shell session.

**Usage:**

```text
FIND_ENV HOMEBREW_PREFIX
FIND_ENV NODE_PATH
FIND_ENV JAVA_HOME
```

**Semantics:**

- Exact name lookup only (no glob). Declared in manifest as a list.
- Returns the variable's value as a string. Empty value means unset or explicitly set to empty — both are treated as absent.
- This is a read-only operation. FIND_ENV never sets or modifies environment variables.
- Useful for integrations that depend on environment variables set by other tools (e.g., `HOMEBREW_PREFIX` set by the homebrew integration's shell exports).

**Output schema:**
```json
{ "status": "ok", "name": "HOMEBREW_PREFIX", "value": "/opt/homebrew" }
{ "status": "ok", "name": "JAVA_HOME", "value": "" }
{ "status": "error", "message": "..." }
```

---

## 3. Status Semantics

All FIND types use the same `status` discriminator, modeled on a database query pattern:

| `status`      | Meaning                                                                         |
|---------------|---------------------------------------------------------------------------------|
| `"ok"`        | Query ran successfully. Results may be empty — empty is not an error.           |
| `"not_found"` | Search root does not exist. Applies to FIND_FILE and FIND_DIR only.             |
| `"error"`     | The matching mechanism failed unexpectedly (permissions, signal, etc.).         |

Empty results (`matches: []`, `path: ""`, `exists: false`, `value: ""`) all carry `"status": "ok"`. Empty is information, not an error.

---

## 4. Shell Implementations

Each shell hook script defines all four FIND functions. They produce JSON via `printf` (Unix) or string interpolation (PowerShell).

> **JSON escaping note:** These implementations build JSON inline with `printf`. Paths containing embedded double-quotes or backslashes are not escaped — this covers the vast majority of real paths. A hardening pass should add proper escaping for edge cases before shipping.

### 4.1 bash (`integrations/bash/hook.bash`)

Replace the current `FIND` function with:

```bash
function FIND_BINARY() {
    local name="$1"
    local found=()
    local dir
    IFS=: read -ra _orbiter_paths <<< "$PATH"
    for dir in "${_orbiter_paths[@]}"; do
        if [ -x "$dir/$name" ]; then
            found+=("\"$dir/$name\"")
        fi
    done
    local matches
    if [ ${#found[@]} -gt 0 ]; then
        matches=$(IFS=,; echo "${found[*]}")
    fi
    printf '{"status":"ok","matches":[%s]}\n' "${matches:-}"
}
export -f FIND_BINARY

function FIND_FILE() {
    local pattern="$1"
    local location="${2:-$ORBITER_CWD}"
    if [ ! -d "$location" ]; then
        printf '{"status":"not_found","message":"location does not exist: %s"}\n' "$location"
        return
    fi
    local matches
    matches="$(find "$location" -name "$pattern" 2>/dev/null | \
        awk 'BEGIN{printf "["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]"}')"
    printf '{"status":"ok","matches":%s}\n' "${matches:-[]}"
}
export -f FIND_FILE

function FIND_DIR() {
    local path="$1"
    if [ -d "$path" ]; then
        printf '{"status":"ok","path":"%s","exists":true}\n' "$path"
    else
        printf '{"status":"ok","path":"","exists":false}\n'
    fi
}
export -f FIND_DIR

function FIND_ENV() {
    local name="$1"
    local value
    value="$(printenv "$name" 2>/dev/null)"
    printf '{"status":"ok","name":"%s","value":"%s"}\n' "$name" "${value:-}"
}
export -f FIND_ENV
```

The existing `FIND` function (plain `command -v`) is removed.

---

### 4.2 zsh (`integrations/zsh/hook.zsh`)

```zsh
function FIND_BINARY() {
    local name="$1"
    local -a found
    local dir
    local -a paths
    paths=("${(@s/:/)PATH}")
    for dir in "${paths[@]}"; do
        if [[ -x "$dir/$name" ]]; then
            found+=("\"$dir/$name\"")
        fi
    done
    printf '{"status":"ok","matches":[%s]}\n' "${(j:,:)found}"
}

function FIND_FILE() {
    local pattern="$1"
    local location="${2:-$ORBITER_CWD}"
    if [[ ! -d "$location" ]]; then
        printf '{"status":"not_found","message":"location does not exist: %s"}\n' "$location"
        return
    fi
    local matches
    matches="$(find "$location" -name "$pattern" 2>/dev/null | \
        awk 'BEGIN{printf "["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]"}')"
    printf '{"status":"ok","matches":%s}\n' "${matches:-[]}"
}

function FIND_DIR() {
    local path="$1"
    if [[ -d "$path" ]]; then
        printf '{"status":"ok","path":"%s","exists":true}\n' "$path"
    else
        printf '{"status":"ok","path":"","exists":false}\n'
    fi
}

function FIND_ENV() {
    local name="$1"
    local value
    value="$(printenv "$name" 2>/dev/null)"
    printf '{"status":"ok","name":"%s","value":"%s"}\n' "$name" "${value:-}"
}
```

The existing `FIND` function is removed.

---

### 4.3 fish (`integrations/fish/hook.fish`)

```fish
function FIND_BINARY
    set -l name $argv[1]
    set -l found
    for dir in (string split : $PATH)
        if test -x "$dir/$name"
            set found $found "\"$dir/$name\""
        end
    end
    printf '{"status":"ok","matches":[%s]}\n' (string join , $found)
end

function FIND_FILE
    set -l pattern $argv[1]
    set -l location $ORBITER_CWD
    if test (count $argv) -ge 2
        set location $argv[2]
    end
    if not test -d $location
        printf '{"status":"not_found","message":"location does not exist: %s"}\n' $location
        return
    end
    set -l matches (find $location -name $pattern 2>/dev/null | \
        awk 'BEGIN{printf "["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]"}')
    if test -z "$matches"
        set matches "[]"
    end
    printf '{"status":"ok","matches":%s}\n' $matches
end

function FIND_DIR
    set -l path $argv[1]
    if test -d $path
        printf '{"status":"ok","path":"%s","exists":true}\n' $path
    else
        printf '{"status":"ok","path":"","exists":false}\n'
    end
end

function FIND_ENV
    set -l name $argv[1]
    set -l value (printenv $name 2>/dev/null)
    printf '{"status":"ok","name":"%s","value":"%s"}\n' $name "$value"
end
```

The existing `FIND` function is removed.

---

### 4.4 PowerShell (`integrations/powershell/hook.ps1`)

```powershell
function FIND_BINARY {
    param([string]$Name)
    $cmds = Get-Command $Name -All -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandType -eq 'Application' } |
        ForEach-Object { "`"$($_.Source -replace '\\', '/')`"" }
    if ($null -eq $cmds) { $cmds = @() }
    Write-Output "{`"status`":`"ok`",`"matches`":[$($cmds -join ',')]}"
}

function FIND_FILE {
    param([string]$Pattern, [string]$Location = $env:ORBITER_CWD)
    if (-not (Test-Path $Location -PathType Container)) {
        Write-Output "{`"status`":`"not_found`",`"message`":`"location does not exist: $Location`"}"
        return
    }
    $items = Get-ChildItem -Path $Location -Filter $Pattern -File -Recurse -ErrorAction SilentlyContinue
    if ($null -eq $items) { $items = @() }
    $escaped = $items | ForEach-Object { "`"$($_.FullName -replace '\\', '/')`"" }
    Write-Output "{`"status`":`"ok`",`"matches`":[$($escaped -join ',')]}"
}

function FIND_DIR {
    param([string]$Path)
    if (Test-Path $Path -PathType Container) {
        $normPath = $Path -replace '\\', '/'
        Write-Output "{`"status`":`"ok`",`"path`":`"$normPath`",`"exists`":true}"
    } else {
        Write-Output '{"status":"ok","path":"","exists":false}'
    }
}

function FIND_ENV {
    param([string]$Name)
    $value = [System.Environment]::GetEnvironmentVariable($Name)
    if ($null -eq $value) { $value = '' }
    Write-Output "{`"status`":`"ok`",`"name`":`"$Name`",`"value`":`"$value`"}"
}
```

The existing `FIND` function (via `Get-Command`) is removed.

---

## 5. Directive Protocol Integration

FIND commands are Orbiter-emitted directives, exactly like `DIR`, `SET`, `UNSET`, and `DEPART`. The shell's `orbiter()` wrapper already processes lines with `::ORBITER::` markers. FIND directives follow the same pattern:

```text
::ORBITER::FIND_BINARY brew
::ORBITER::FIND_FILE "*.json" /path/to/project
::ORBITER::FIND_DIR /usr/local/lib
::ORBITER::FIND_ENV HOMEBREW_PREFIX
```

The shell's directive handler extracts the command after `::ORBITER::` and executes it as a function call, capturing JSON output. **How that JSON output is returned to Orbiter is defined by the execution model refactor spec.** This spec only defines what the functions produce.

The `Directive` struct in `internal/commands/executor.go` maps naturally:

| Field | Meaning for FIND directives |
| ----- | --------------------------- |
| `Op` | `"FIND_BINARY"`, `"FIND_FILE"`, `"FIND_DIR"`, or `"FIND_ENV"` |
| `Key` | Binary name (FIND_BINARY, FIND_ENV) or glob pattern (FIND_FILE) |
| `Value` | Optional path / location (FIND_FILE, FIND_DIR) |

---

## 6. Manifest Declaration

Integrations declare which FIND operations Orbiter should run on their behalf. Orbiter runs them all before calling any handler and passes results in `ResolvedContext`.

### Existing: `[integration] binaries`

Already used to trigger `FIND_BINARY` per name. No change.

```toml
[integration]
binaries = ["brew"]  # Orbiter calls FIND_BINARY brew before any handler
```

### New: `[find]` section

```toml
[find]
files = ["Brewfile", "*.rb"]       # Orbiter calls FIND_FILE per pattern, location = CWD
dirs  = ["/opt/homebrew"]          # Orbiter calls FIND_DIR per path
envs  = ["HOMEBREW_PREFIX", "HOMEBREW_CELLAR"]  # Orbiter calls FIND_ENV per name
```

### Go types (new in `internal/integrations/manifest.go`)

```go
// ManifestFind is the [find] section.
type ManifestFind struct {
    Files []string `toml:"files"`
    Dirs  []string `toml:"dirs"`
    Envs  []string `toml:"envs"`
}
```

Add to `Manifest`:

```go
type Manifest struct {
    Integration  ManifestIntegration  `toml:"integration"`
    Detection    ManifestDetection    `toml:"detection"`
    Dependencies ManifestDependencies `toml:"dependencies"`
    Commands     ManifestCommands     `toml:"commands"`
    Shell        ManifestShell        `toml:"shell"`
    Config       ManifestConfig       `toml:"config"`
    Runtime      ManifestRuntime      `toml:"runtime"`
    Find         ManifestFind         `toml:"find"`   // new
}
```

---

## 7. ResolvedContext Changes

WASM integrations receive pre-resolved results. Orbiter populates all fields from the manifest declarations before any handler is called.

### Changes to `ResolvedContext` (`internal/integrations/types.go`)

`Path` is defined in `types.go` by the filesystem spec (dependency above). It holds `Unix` (always forward-slash) and `Windows` (always backslash) as exported fields persisted in the StarChart. Four `Into*` methods provide a uniform calling convention: `IntoUnix()`, `IntoWindows()`, `IntoNative()` (OS-native form at runtime), and `IntoShell()` (native, double-quoted when it contains a space).

```go
type ResolvedContext struct {
    // ... existing fields ...

    // Binaries: binary name -> path. Zero Path = not found.
    // The path here is the Captain-chosen or StarChart-recorded path, not the
    // raw FIND_BINARY result. FIND_BINARY may return multiple matches; Orbiter
    // or the Captain selects one and records it in the StarChart.
    // Use Binaries["brew"].IntoNative() when calling exec.Command; .Unix for storage.
    Binaries map[string]Path

    // Files: glob pattern -> []paths. Empty slice = no matches (not an error).
    // Populated from manifest.Find.Files via recursive FIND_FILE relative to CWD.
    Files map[string][]Path

    // Dirs: path -> exists. false = directory absent (not an error).
    // Populated from manifest.Find.Dirs via FIND_DIR.
    Dirs map[string]bool

    // Envs: env var name -> value. "" = unset or empty (not an error).
    // Populated from manifest.Find.Envs via FIND_ENV.
    Envs map[string]string
}
```

`Envs` values are not path-normalized — they are raw strings from the environment.

---

## 8. filesystem.go Role

`filesystem.go` is **not** part of the FIND call chain — it never calls the shell. Its role in this context:

1. **`ParsePath(raw string) (Path, error)`** — added by the filesystem spec (dependency above). Called by Orbiter after FIND returns a raw path string from the shell. Handles:
   - Shell-quoted input (strips double-quotes)
   - Windows absolute paths (drive letter detection, no `filepath.Abs` misfire)
   - `~` expansion to home directory
   - Relative → absolute via `filepath.Abs`
   - Returns zero `Path` and nil error for empty input (pass-through for not-found)

2. **`PathFromConfig`** — exported function from the filesystem spec. Reads the `"path"` key from a resource's config JSON and returns a `Path`.

The `FindBinary`, `findBinaryUnix`, `findBinaryWindows`, and `runShellFind` functions are **deleted** as part of the filesystem spec — before this FIND spec becomes a plan.

---

## 9. Security

FIND functions are Orbiter-owned shell infrastructure declared directly in the Captain's profile. They are **not** subject to the `bannedCommands` list or the manifest allowlist/Captain-approval gate in `runCommandFn`.

The `bannedCommands` ban on shells (`bash`, `zsh`, `fish`, etc.) applies to integration-called executables routed through `run_command`. FIND functions are shell builtins/functions already resident in the Captain's session — they are not arbitrary paths an integration can inject.

If an integration attempts to invoke `FIND_BINARY`, `FIND_FILE`, `FIND_DIR`, or `FIND_ENV` through `run_command`, the call is denied: FIND functions are not listed in any manifest's `[commands.allowed]` and are not executable binaries. They are called exclusively through the directive protocol.

---

## 10. Future Extensions

These are explicitly **not** in scope for the initial implementation (YAGNI):

- **FIND_BINARY non-PATH search** — searching outside PATH for binaries installed in non-standard locations (e.g., a binary installed but not linked). Requires a broader search strategy and is out of scope until there is a concrete need.
- **FIND_FILE depth control** — a `--depth N` flag to limit recursion. Not needed now; the default is fully recursive.
- **FIND_ENV pattern matching** — glob-style env var discovery. Exact name lookup is sufficient for declared manifest needs.

**FIND_VERSION is not a shell function.** Binary version reporting is the integration's responsibility: call the binary's self-report command (e.g., `brew --version`) via `run_command`, or receive version as part of dependency resolution context before the integration is called. A shell function for version lookup would require parsing version strings in every shell dialect — that belongs in integration code or Orbiter's Go layer.

---

## 11. Dependency Chain

This spec depends on the filesystem spec. No implementation work before both specs are written.

```text
filesystem spec
    └── [this FIND spec]
          └── exec.Command / run_command execution model refactor
                └── Present/Reachable refactor
                      └── 2026-06-22-binary-resolution-shell-directive.md
```

**filesystem spec defines (implemented):** `Path` type and `ParsePath`, deletes `FindBinary` family, leaves a temporary `populateBinaries` stub in `loader.go` to keep compilation clean. Native lifecycle handlers `scanFilesystemResource` / `calibrateFilesystemResource` live in `lifecycle.go`.

**Confirmed behavior from implementation:**

- `Reachable = info.IsDir()` — a path that exists but is a regular file (not a directory) sets `Present=true, Reachable=false`, which yields `status = drifted`. Drift covers both absent paths and wrong-type paths.
- `action` in `calibrateFilesystemResource` derives from `status != models.BeaconStatusHealthy` (not just `!Present`), so `action` and `status` always agree.

**This spec defines:** what FIND functions produce, what manifests/ResolvedContext look like, what the shell implementations are, and that the resolution implementation lives in `lifecycle.go`.

**Execution model refactor defines:** the wire protocol that delivers FIND JSON from the shell back to the Orbiter process (the callback/round-trip mechanism).

**Present/Reachable refactor defines:** removing Present/Reachable from integration responsibility; Orbiter owns Present (recorded binary path is reachable via `os.Stat`); Reachable becomes Captain-facing read-only info (binary appears in FIND_BINARY matches).

**Binary resolution plan:** wires FIND results into `lifecycle.go` context population (deletes the `populateBinaries` stub from `loader.go`), adds binary drift detection.
