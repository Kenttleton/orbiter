# FIND Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `populateBinaries` stub with real FIND-based resolution of binaries, files, dirs and env vars, so WASM handlers receive a fully-resolved `ResolvedContext` and `orbiter uplink` can ingest FIND results.

**Architecture:** Shell hook scripts own four FIND functions (`FIND_BINARY/FILE/DIR/ENV`) that take a transaction ID first and pipe their JSON result into `orbiter relay`. Orbiter emits FIND directives (after writing the session + transaction rows), exits, and `orbiter uplink` ingests relayed results, persists the chosen path into the resource's config, and the lifecycle injects recorded values into `ResolvedContext` on every later scan. Resolution lives in `internal/starchart`, never in `internal/wasm` or `filesystem.go`.

**Tech Stack:** Go, cobra, BurntSushi/toml, SQLite (existing sessions/transactions tables), bash/zsh/fish/PowerShell hook scripts, testify.

**Spec:** [`2026-06-23-find-interface-spec.md`](2026-06-23-find-interface-spec.md) (function semantics, manifest, context) and [`2026-06-25-io-contract-spec.md`](2026-06-25-io-contract-spec.md) §5 (sessions, transactions, relay/uplink). Where they disagree, the **Reconciliation Decisions** below win.

## Reconciliation Decisions

The two specs were written a day apart and conflict. These are the resolutions this plan implements. **Review these first; changing one changes tasks.**

| # | Conflict | Decision | Why |
|---|----------|----------|-----|
| D1 | FIND spec: `FIND_BINARY` walks `$PATH`. I/O spec: global `find / -perm +111`. | **The search method belongs to the shell integration; Orbiter specifies only the contract**: `FIND_BINARY <txn> <name>` relays JSON listing every candidate location (schema in FIND spec §2.1). A `PATH` walk, `which`, `Get-Command`, Spotlight or a global search are all valid. This plan ships a `PATH` walk as the default implementation in each hook because it is fast and needs no OS-specific tools. | Terminal-native tooling differs and the integration knows it best. One caveat the global-search version exposed: `-perm +111` is BSD-only (GNU find needs `/111`), which is exactly the kind of detail an integration should own. |
| D2 | FIND spec: `::ORBITER::FIND_X arg`. I/O spec: `::ORBITER::FIND_X:<txn> arg`. | **Line protocol: `FIND_X <txn> <key> [value]`**, plus `SESSION <id>`. | `::ORBITER::` in hook scripts is the install-time placeholder for the binary path (replaced by `printShellScript`), not a wire marker. Existing directives are `OP rest` lines. |
| D3 | FIND spec: `Binaries map[string]Path`, plus `Files/Dirs/Envs`. Current guests parse `"binaries":{"go":"/path"}` strings. | **Keep wire type `map[string]string` (native path)**; add `Files map[string][]string`, `Dirs map[string]bool`, `Envs map[string]string`. | 20+ built guests (Rust, TinyGo, AssemblyScript) read string values. Typed `Path` is used internally and converted with `IntoNative()` at injection. |
| D4 | FIND spec: lifecycle pre-resolves on every scan. I/O spec: FIND only at INIT / drift recovery. | **FIND at INIT and drift recovery only.** Scans inject the *recorded* path from resource config (`config.binaries`) as-is. Orbiter does not check it: the integration reports Present/Reachable in its `StateReport`, and Orbiter stores that state. | Per-scan shell round trips would make `survey`/`scan` slow and session-bound. Whether a binary is present and reachable is the integration's call (it knows how its tool is installed and run), so Orbiter never second-guesses it with `os.Stat` or `PATH` rules. |
| D5 | Directive carries only `Key`/`Value`. | Add `Txn string` to `Directive`. | Needed by every FIND line. |
| D6 | `FIND_FILE` pattern is positional, location optional. | Pattern **must not contain spaces**; location is "rest of line" so may. | Line protocol has no quoting; planner rejects bad patterns up front. |

## Global Constraints

- `internal/integrations/filesystem.go` MUST NOT import `os/exec` or know about shells.
- WASM handlers never discover paths; `internal/wasm` MUST NOT resolve binaries.
- No new migrations; sessions/transactions tables already exist in `0001_initial.sql`. Store resolved paths in the existing resource `config` TEXT JSON.
- Binary drift MUST NOT auto-update the StarChart; the Captain confirms (CONFIRM/SELECT transaction) before a recorded path changes.
- FIND functions are called only through directives, never via `run_command`.
- `go build ./... && go vet ./... && go test ./...` pass after every task.
- Commit trailer: `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Review Focus

Inputs the spec implies but no happy-path test would hit, most likely first (each is pinned by a test in the owning task):

1. **Directory named like the binary** (e.g. `~/bin/brew/` is a dir) — `[ -x ]` is true for dirs; `FIND_BINARY` must require a regular file. (Task 4)
2. **Duplicate and empty `PATH` entries** — same dir twice or via symlink-free repeats must yield one match; an empty entry means "cwd" and must be skipped. (Task 4)
3. **Paths with spaces, quotes or backslashes** — JSON must stay valid (the FIND spec flagged escaping as unfinished). (Task 4)
4. **Git Bash paths on a Windows host** — `/c/Users/x/git.exe` must become `C:/Users/x/git.exe`, while the same string on macOS/Linux stays a Unix path. (Task 5a)
5. **Zero matches** — valid `ok` result; must record an observation, not crash or write an empty path. (Task 8)
6. **Relay for unknown/already-relayed transaction, or uplink with a still-pending FIND** — must not corrupt state or hang. (Task 8)

## File Map

| File | Change |
|------|--------|
| `integrations/e2e_test.go` | Test helper supplies `Binaries` so e2e scans are green again |
| `internal/integrations/manifest.go` | `ManifestFind`, `Manifest.Find` |
| `internal/integrations/types.go` | `ResolvedContext.Files/Dirs/Envs` |
| `internal/commands/executor.go` | `Directive.Txn`; `FIND_*`/`SESSION` ops in `String()` |
| `integrations/{bash,zsh,fish,powershell}/hook.*` | New FIND functions, wrapper handles `FIND_*`/`SESSION`, pass-through |
| `internal/commands/hook_find_test.go` | Runs real bash/zsh hook functions against a fake `orbiter` |
| `internal/integrations/filesystem.go` | Host-aware `ParsePath` (Git Bash/Cygwin drive paths on Windows hosts) |
| `internal/starchart/find.go` (new) | `FindRequest`, `PlanFinds`, `ParseFindResponse`, `ApplyFindResults`, recorded-binary lookup |
| `internal/starchart/find_test.go` (new) | Unit tests for the above |
| `internal/starchart/crawl.go` | `BuildResolvedContext` injects recorded binaries |
| `internal/wasm/loader.go` | Delete `populateBinaries` and its 3 call sites |
| `internal/starchart/lifecycle.go` | `NeededFinds` (emit FIND when no path is recorded or the integration reports not-present) |
| `internal/commands/uplink.go` | Real FIND ingestion; remove stub |
| `docs/superpowers/plans/2026-06-22-binary-resolution-shell-directive.md` | Mark superseded |

---

# Part A — Resolution engine (no session-lifecycle dependency)

Part A is safe to build now. It ends with scans reading recorded binary paths and a green suite.

### Task 1: Make the e2e suite green again

**Why:** `go test ./...` currently fails in `integrations/e2e_test.go` (nvm scan, GitHub tool scan, macOS keychain scan) because the tests pass `core.ResolvedContext{}` with no `Binaries`, and the guests now read `ctx.binaries[...]`. This is a test-side gap, and fixing it gives every later task a clean baseline.

**Files:**
- Modify: `integrations/e2e_test.go`

**Interfaces:**
- Produces: `func resolvedFor(t *testing.T, i core.Integration) core.ResolvedContext` — builds a context whose `Binaries` holds a `exec.LookPath` result for every name in `i.Meta().Integration.Binaries` that exists. (Test-only; production never uses `LookPath`.)

- [ ] **Step 1: Confirm the failure**

Run: `go test ./integrations/ -run 'NVM|GitHub_Tool|KeychainMacOS' 2>&1 | tail -20`
Expected: FAIL with "expected present=true" for each.

- [ ] **Step 2: Add the helper** (near `setupBundleRegistry` in `integrations/e2e_test.go`; add `"os/exec"` to imports)

```go
// resolvedFor stands in for the lifecycle's FIND resolution: it looks up each
// binary the manifest declares and passes the result as ctx.Binaries, exactly
// as BuildResolvedContext does in production.
func resolvedFor(t *testing.T, i core.Integration) core.ResolvedContext {
	t.Helper()
	bins := map[string]string{}
	for _, name := range i.Meta().Integration.Binaries {
		if p, err := exec.LookPath(name); err == nil {
			bins[name] = p
		}
	}
	return core.ResolvedContext{Binaries: bins}
}
```

- [ ] **Step 3: Use it in the failing scans**

In `TestBundledIntegrations_NVM` change `i.Scan(core.ResolvedContext{})` to `i.Scan(resolvedFor(t, i))`. Do the same in the keychain `scan_darwin` subtest (`TestBundledIntegrations_KeychainMacOS`) and the GitHub `scan_tool` subtest (`TestBundledIntegrations_GitHub_Tool`). Replace the `ResolvedContext{}` argument only; keep every other assertion.

- [ ] **Step 4: Run the suite**

Run: `go test ./... 2>&1 | grep -v 'no test files'`
Expected: all `ok`. If a test still fails because the tool isn't installed on this machine (`gh`, `nvm` is a shell function and may not be a binary), add `t.Skip("<tool> not installed")` guarded by `if len(resolvedFor(t, i).Binaries) == 0`, and note it in the commit message.

- [ ] **Step 5: Commit**

```bash
git add integrations/e2e_test.go
git commit -m "test: pass resolved binaries to e2e scans

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

### Task 2: Manifest `[find]` and context fields

**Files:**
- Modify: `internal/integrations/manifest.go`, `internal/integrations/types.go`
- Test: `internal/integrations/manifest_test.go`

**Interfaces:**
- Produces: `ManifestFind{Files, Dirs, Envs []string}`; `Manifest.Find`; `ResolvedContext.Files map[string][]string`, `.Dirs map[string]bool`, `.Envs map[string]string` (all `omitempty`).

- [ ] **Step 1: Write the failing test** (append to `manifest_test.go`)

```go
func TestManifest_ParseFindSection(t *testing.T) {
	const src = `
[integration]
brand = "brew"
binaries = ["brew"]

[find]
files = ["Brewfile"]
dirs  = ["/opt/homebrew"]
envs  = ["HOMEBREW_PREFIX"]
`
	var m Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Find.Files; len(got) != 1 || got[0] != "Brewfile" {
		t.Errorf("Files = %v", got)
	}
	if got := m.Find.Dirs; len(got) != 1 || got[0] != "/opt/homebrew" {
		t.Errorf("Dirs = %v", got)
	}
	if got := m.Find.Envs; len(got) != 1 || got[0] != "HOMEBREW_PREFIX" {
		t.Errorf("Envs = %v", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/integrations/ -run ParseFindSection`
Expected: FAIL — `m.Find undefined`.

- [ ] **Step 3: Implement**

In `manifest.go` add to `Manifest` the line `Find         ManifestFind         \`toml:"find"\`` and:

```go
// ManifestFind is the [find] section: discovery Orbiter performs on the
// integration's behalf before any handler runs. Binaries are declared in
// [integration] binaries.
type ManifestFind struct {
	Files []string `toml:"files"` // glob patterns, no spaces, searched under the CWD
	Dirs  []string `toml:"dirs"`  // known paths whose existence is checked
	Envs  []string `toml:"envs"`  // exact env var names
}
```

In `types.go`, extend `ResolvedContext` after `Binaries`:

```go
	Files map[string][]string `json:"files,omitempty"` // pattern -> native paths (empty = no match)
	Dirs  map[string]bool     `json:"dirs,omitempty"`  // path -> exists
	Envs  map[string]string   `json:"envs,omitempty"`  // name -> value ("" = unset)
```

- [ ] **Step 4: Run** `go build ./... && go test ./internal/integrations/` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: add manifest [find] section and ResolvedContext find fields` (with trailer).

### Task 3: FIND and SESSION directives

**Files:**
- Modify: `internal/commands/executor.go:19-43`
- Test: `internal/commands/executor_test.go`

**Interfaces:**
- Produces: `Directive.Txn string`; `String()` renders `SESSION <id>` (id in `Key`), `FIND_BINARY|FIND_ENV|FIND_DIR <txn> <key>`, and `FIND_FILE <txn> <pattern> [location]` (pattern in `Key`, location in `Value`).

- [ ] **Step 1: Failing test**

```go
func TestDirective_String_Find(t *testing.T) {
	cases := []struct {
		d    commands.Directive
		want string
	}{
		{commands.Directive{Op: "SESSION", Key: "a1ss"}, "SESSION a1ss"},
		{commands.Directive{Op: "FIND_BINARY", Txn: "t1tx", Key: "brew"}, "FIND_BINARY t1tx brew"},
		{commands.Directive{Op: "FIND_ENV", Txn: "t2tx", Key: "JAVA_HOME"}, "FIND_ENV t2tx JAVA_HOME"},
		{commands.Directive{Op: "FIND_DIR", Txn: "t3tx", Key: "/opt/homebrew"}, "FIND_DIR t3tx /opt/homebrew"},
		{commands.Directive{Op: "FIND_FILE", Txn: "t4tx", Key: "*.json"}, "FIND_FILE t4tx *.json"},
		{commands.Directive{Op: "FIND_FILE", Txn: "t4tx", Key: "*.json", Value: "/my proj"}, "FIND_FILE t4tx *.json /my proj"},
	}
	for _, c := range cases {
		if got := c.d.String(); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/commands/ -run Directive_String_Find` — Expected: FAIL (`Txn` unknown field).

- [ ] **Step 3: Implement** — add `Txn string // FIND_*: transaction ID` to `Directive`, update the doc comment's op list, and add to `String()`:

```go
	case "SESSION":
		return "SESSION " + d.Key
	case "FIND_BINARY", "FIND_ENV", "FIND_DIR":
		return d.Op + " " + d.Txn + " " + d.Key
	case "FIND_FILE":
		if d.Value != "" {
			return "FIND_FILE " + d.Txn + " " + d.Key + " " + d.Value
		}
		return "FIND_FILE " + d.Txn + " " + d.Key
```

- [ ] **Step 4: Run** `go test ./internal/commands/` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: add SESSION and FIND_* directives` (with trailer).

### Task 4: Shell FIND functions and wrapper handling

**Files:**
- Modify: `integrations/bash/hook.bash`, `integrations/zsh/hook.zsh`, `integrations/fish/hook.fish`, `integrations/powershell/hook.ps1`
- Create: `internal/commands/hook_find_test.go`

**Interfaces:**
- Consumes: Task 3 line format.
- Produces: shell functions `FIND_BINARY <txn> <name>`, `FIND_FILE <txn> <pattern> [location]`, `FIND_DIR <txn> <path>`, `FIND_ENV <txn> <name>`; each pipes one JSON object (schemas in FIND spec §2) to `orbiter relay --txn=<txn>`. The wrapper runs them for `FIND_*` lines, remembers `SESSION`, and runs `orbiter uplink --session <id>` afterwards.

Remove the old one-argument `FIND` function from every hook.

- [ ] **Step 1: Write the failing test** `internal/commands/hook_find_test.go`

It sources the real hook with `::ORBITER::` replaced by a fake binary that writes `relay` stdin to `$OUT/<txn>`, and checks the Review Focus cases (dir named like the binary, duplicate and empty PATH entries, a space and a quote in a path).

```go
package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type findResp struct {
	Shell   string   `json:"shell"`
	Status  string   `json:"status"`
	Matches []string `json:"matches"`
	Exists  bool     `json:"exists"`
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Value   string   `json:"value"`
}

// runFind sources the real hook for shell, runs one FIND function and returns
// the JSON the function relayed for txn "tx1".
func runFind(t *testing.T, shell, hook, call string, env []string) findResp {
	t.Helper()
	bin, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s not installed", shell)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	_ = os.MkdirAll(out, 0o755)
	fake := filepath.Join(dir, "orbiter-fake")
	// Fake binary: `relay --txn=X` stores stdin in $OUT/X.
	script := "#!/bin/sh\ncase \"$1\" in relay) cat > \"" + out + "/${2#--txn=}\" ;; esac\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "integrations", hook))
	if err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(dir, hook)
	body := strings.ReplaceAll(string(src), "::ORBITER::", fake)
	if err := os.WriteFile(hookPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-c", "source '"+hookPath+"'; "+call)
	cmd.Env = append(os.Environ(), env...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", shell, err, b)
	}
	raw, err := os.ReadFile(filepath.Join(out, "tx1"))
	if err != nil {
		t.Fatalf("nothing relayed for tx1: %v", err)
	}
	var r findResp
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return r
}

func TestFindBinary_PathWalk(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a dir") // space in path
	b := filepath.Join(root, `b"quote`) // quote in path
	c := filepath.Join(root, "c")       // holds a *directory* named like the binary
	for _, d := range []string{a, b, c} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{a, b} {
		if err := os.WriteFile(filepath.Join(d, "mytool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(c, "mytool"), 0o755); err != nil {
		t.Fatal(err)
	}
	// duplicate entry for a, an empty entry, and the directory-only entry c
	path := strings.Join([]string{a, "", a, c, b}, ":") + ":/usr/bin:/bin"

	for _, tc := range []struct{ shell, hook string }{
		{"bash", "bash/hook.bash"},
		{"zsh", "zsh/hook.zsh"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			r := runFind(t, tc.shell, tc.hook, "FIND_BINARY tx1 mytool", []string{"PATH=" + path})
			want := []string{filepath.Join(a, "mytool"), filepath.Join(b, "mytool")}
			if r.Status != "ok" || len(r.Matches) != 2 || r.Matches[0] != want[0] || r.Matches[1] != want[1] {
				t.Errorf("got %+v, want matches %v (deduped, no dir, no empty entry)", r, want)
			}
		})
	}
}

func TestFindBinary_NoMatchIsOK(t *testing.T) {
	r := runFind(t, "bash", "bash/hook.bash", "FIND_BINARY tx1 definitely-not-a-binary", nil)
	if r.Status != "ok" || len(r.Matches) != 0 {
		t.Errorf("got %+v, want ok with no matches", r)
	}
}

func TestFindDirAndEnv(t *testing.T) {
	d := t.TempDir()
	r := runFind(t, "bash", "bash/hook.bash", "FIND_DIR tx1 '"+d+"'", nil)
	if r.Status != "ok" || !r.Exists || r.Path != d {
		t.Errorf("FIND_DIR existing: %+v", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_DIR tx1 /no/such/dir", nil)
	if r.Status != "ok" || r.Exists || r.Path != "" {
		t.Errorf("FIND_DIR missing: %+v", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_ENV tx1 ORBITER_T", []string{`ORBITER_T=va"l\ue`})
	if r.Status != "ok" || r.Name != "ORBITER_T" || r.Value != `va"l\ue` {
		t.Errorf("FIND_ENV: %+v", r)
	}
}

func TestFindFile_Recursive(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	for _, p := range []string{"package.json", "sub/package.json"} {
		_ = os.WriteFile(filepath.Join(root, p), []byte("{}"), 0o644)
	}
	r := runFind(t, "bash", "bash/hook.bash", "FIND_FILE tx1 package.json '"+root+"'", nil)
	if r.Status != "ok" || len(r.Matches) != 2 {
		t.Errorf("got %+v, want 2 recursive matches", r)
	}
	r = runFind(t, "bash", "bash/hook.bash", "FIND_FILE tx1 package.json /no/such/root", nil)
	if r.Status != "not_found" {
		t.Errorf("missing root: got %+v, want not_found", r)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/commands/ -run 'TestFind' -v 2>&1 | tail -20`
Expected: FAIL — `FIND_BINARY: command not found` (or empty relay).

- [ ] **Step 3: Implement bash** — in `integrations/bash/hook.bash` delete `FIND()` and add (before `_orbiter_hook`):

```bash
# JSON-quote one string (escapes backslash and double-quote).
function _orbiter_json_str() {
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    printf '"%s"' "$s"
}

function _orbiter_find_reply() { # <txn> <json>
    printf '%s\n' "$2" | orbiter relay --txn="$1"
}

function FIND_BINARY() {
    local txn="$1" name="$2" dir p out="" seen=$'\n'
    local -a dirs
    IFS=: read -ra dirs <<< "$PATH"
    for dir in "${dirs[@]}"; do
        [ -z "$dir" ] && continue          # empty entry means cwd; never a match
        p="$dir/$name"
        [ -f "$p" ] && [ -x "$p" ] || continue   # regular executable file, not a directory
        case "$seen" in *$'\n'"$p"$'\n'*) continue ;; esac
        seen+="$p"$'\n'
        out+="${out:+,}$(_orbiter_json_str "$p")"
    done
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"matches\":[$out]}"
}
export -f FIND_BINARY

function FIND_FILE() {
    local txn="$1" pattern="$2" location="${3:-$ORBITER_CWD}" out="" f
    location="${location/#\~/$HOME}"
    if [ ! -d "$location" ]; then
        _orbiter_find_reply "$txn" "{\"status\":\"not_found\",\"message\":$(_orbiter_json_str "location does not exist: $location")}"
        return
    fi
    while IFS= read -r f; do
        [ -z "$f" ] && continue
        out+="${out:+,}$(_orbiter_json_str "$f")"
    done < <(find "$location" -type f -name "$pattern" 2>/dev/null)
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"matches\":[$out]}"
}
export -f FIND_FILE

function FIND_DIR() {
    local txn="$1" path="${2/#\~/$HOME}"
    if [ -d "$path" ]; then
        _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"path\":$(_orbiter_json_str "$path"),\"exists\":true}"
    else
        _orbiter_find_reply "$txn" '{"status":"ok","path":"","exists":false}'
    fi
}
export -f FIND_DIR

function FIND_ENV() {
    local txn="$1" name="$2" value
    value="$(printenv "$name" 2>/dev/null)"
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"name\":$(_orbiter_json_str "$name"),\"value\":$(_orbiter_json_str "$value")}"
}
export -f FIND_ENV
```

Replace the `orbiter()` function with:

```bash
function orbiter() {
    local _out _exit _session="" _ran_find=0
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
            DIR)     cd "$_rest" ;;
            SET)     export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET)   unset "$_rest" ;;
            SESSION) _session="$_rest" ;;
            FIND_BINARY|FIND_DIR|FIND_ENV|FIND_FILE)
                local _txn="${_rest%% *}" _args="${_rest#* }" _key _val=""
                _key="${_args%% *}"
                [[ "$_args" == *" "* ]] && _val="${_args#* }"
                if [[ -n "$_val" ]]; then "$_op" "$_txn" "$_key" "$_val"; else "$_op" "$_txn" "$_key"; fi
                _ran_find=1
                ;;
            *)       printf '%s\n' "$_line" ;;   # pass through ordinary output
        esac
    done <<< "$_out"
    if [[ $_ran_find -eq 1 && -n "$_session" ]]; then
        orbiter uplink --session "$_session"
    fi
}
```

- [ ] **Step 4: Run bash tests** — `go test ./internal/commands/ -run 'TestFind' -v` — Expected: bash PASS, zsh SKIP/FAIL until Step 5.

- [ ] **Step 5: Implement zsh** — same structure in `integrations/zsh/hook.zsh` (delete `FIND()`; no `export -f`). Differences: split `PATH` with `dirs=("${(@s/:/)PATH}")`, and make the wrapper's FIND case identical to bash (zsh supports the same `${var%% *}` forms). `_orbiter_json_str`, `_orbiter_find_reply`, `FIND_FILE`, `FIND_DIR` and `FIND_ENV` bodies are identical to bash. `FIND_BINARY` for zsh:

```zsh
function FIND_BINARY() {
    local txn="$1" name="$2" dir p out="" seen=$'\n'
    local -a dirs
    dirs=("${(@s/:/)PATH}")
    for dir in "${dirs[@]}"; do
        [[ -z "$dir" ]] && continue
        p="$dir/$name"
        [[ -f "$p" && -x "$p" ]] || continue
        case "$seen" in *$'\n'"$p"$'\n'*) continue ;; esac
        seen+="$p"$'\n'
        out+="${out:+,}$(_orbiter_json_str "$p")"
    done
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"matches\":[$out]}"
}
```

Run: `go test ./internal/commands/ -run 'TestFind' -v` — Expected: bash and zsh PASS.

- [ ] **Step 6: Implement fish** in `integrations/fish/hook.fish` (delete `FIND`):

```fish
function _orbiter_json_str
    set -l s (string replace -a '\\' '\\\\' -- $argv[1] | string replace -a '"' '\\"')
    printf '"%s"' $s
end

function _orbiter_find_reply # txn json
    printf '%s\n' $argv[2] | orbiter relay --txn=$argv[1]
end

function FIND_BINARY
    set -l txn $argv[1]
    set -l name $argv[2]
    set -l items
    set -l seen
    for dir in $PATH
        test -n "$dir"; or continue
        set -l p "$dir/$name"
        test -f "$p" -a -x "$p"; or continue
        contains -- "$p" $seen; and continue
        set seen $seen "$p"
        set items $items (_orbiter_json_str "$p")
    end
    _orbiter_find_reply $txn '{"status":"ok","matches":['(string join , $items)']}'
end

function FIND_FILE
    set -l txn $argv[1]
    set -l pattern $argv[2]
    set -l location $ORBITER_CWD
    test (count $argv) -ge 3; and set location $argv[3]
    set location (string replace -r '^~' $HOME -- $location)
    if not test -d "$location"
        _orbiter_find_reply $txn '{"status":"not_found","message":'(_orbiter_json_str "location does not exist: $location")'}'
        return
    end
    set -l items
    for f in (find "$location" -type f -name "$pattern" 2>/dev/null)
        set items $items (_orbiter_json_str "$f")
    end
    _orbiter_find_reply $txn '{"status":"ok","matches":['(string join , $items)']}'
end

function FIND_DIR
    set -l txn $argv[1]
    set -l path (string replace -r '^~' $HOME -- $argv[2])
    if test -d "$path"
        _orbiter_find_reply $txn '{"status":"ok","path":'(_orbiter_json_str "$path")',"exists":true}'
    else
        _orbiter_find_reply $txn '{"status":"ok","path":"","exists":false}'
    end
end

function FIND_ENV
    set -l txn $argv[1]
    set -l value (printenv $argv[2] 2>/dev/null)
    _orbiter_find_reply $txn '{"status":"ok","name":'(_orbiter_json_str $argv[2])',"value":'(_orbiter_json_str "$value")'}'
end
```

Update fish's `orbiter` wrapper `switch` with the same cases: `SESSION` stores `$_session`; `FIND_*` splits `$rest` with `string split -m2 ' ' -- $rest` into txn/key/value, calls the function, sets `_ran_find`; a `case '*'` prints the line; after the loop run `orbiter uplink --session $_session` when `_ran_find` and `_session` are set. fish is not installed on this machine: verify manually (`fish -c 'source <hook>; FIND_BINARY tx1 git'`) or on CI, and leave the Go test's `skip` in place.

- [ ] **Step 7: Implement PowerShell** in `integrations/powershell/hook.ps1` (delete `FIND`):

```powershell
function _OrbiterJson([string]$s) { '"' + ($s -replace '\\','\\' -replace '"','\"') + '"' }
function _OrbiterReply([string]$Txn, [string]$Json) { $Json | orbiter relay "--txn=$Txn" }

function FIND_BINARY {
    param([string]$Txn, [string]$Name)
    $paths = Get-Command $Name -All -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandType -eq 'Application' } |
        ForEach-Object { $_.Source -replace '\\','/' } | Select-Object -Unique
    $items = @($paths | ForEach-Object { _OrbiterJson $_ })
    _OrbiterReply $Txn ('{"status":"ok","matches":[' + ($items -join ',') + ']}')
}

function FIND_FILE {
    param([string]$Txn, [string]$Pattern, [string]$Location = $env:ORBITER_CWD)
    if (-not (Test-Path -LiteralPath $Location -PathType Container)) {
        _OrbiterReply $Txn ('{"status":"not_found","message":' + (_OrbiterJson "location does not exist: $Location") + '}')
        return
    }
    $items = @(Get-ChildItem -LiteralPath $Location -Filter $Pattern -File -Recurse -ErrorAction SilentlyContinue |
        ForEach-Object { _OrbiterJson ($_.FullName -replace '\\','/') })
    _OrbiterReply $Txn ('{"status":"ok","matches":[' + ($items -join ',') + ']}')
}

function FIND_DIR {
    param([string]$Txn, [string]$Path)
    if (Test-Path -LiteralPath $Path -PathType Container) {
        _OrbiterReply $Txn ('{"status":"ok","path":' + (_OrbiterJson ($Path -replace '\\','/')) + ',"exists":true}')
    } else {
        _OrbiterReply $Txn '{"status":"ok","path":"","exists":false}'
    }
}

function FIND_ENV {
    param([string]$Txn, [string]$Name)
    $v = [Environment]::GetEnvironmentVariable($Name); if ($null -eq $v) { $v = '' }
    _OrbiterReply $Txn ('{"status":"ok","name":' + (_OrbiterJson $Name) + ',"value":' + (_OrbiterJson $v) + '}')
}
```

Update the PowerShell `orbiter` wrapper's directive `switch` with `SESSION`, the four `FIND_*` ops (split `$rest` with `-split ' ', 3` into txn/key/value), a default branch that writes the line through, and the trailing `orbiter uplink --session $session`. pwsh is not installed here; verify manually or on CI.

- [ ] **Step 8: Run everything** — `go build ./... && go vet ./... && go test ./...` — Expected: PASS (fish/pwsh tests, if any, skip).

- [ ] **Step 9: Commit** `feat: add txn-based FIND_* shell functions and directive handling to all four hooks` (with trailer).

### Task 5: Request planner, response parser, context injection (pure functions)

**Files:**
- Create: `internal/starchart/find.go`, `internal/starchart/find_test.go`

**Interfaces:**
- Consumes: `integrations.Manifest` (`Integration.Binaries`, `Find.*`), `integrations.ParsePath(raw string) (Path, error)`, `Path.IntoNative()`.
- Produces:
  - `type FindRequest struct{ Op, Key, Value string }` — `Op` is `FIND_BINARY|FIND_FILE|FIND_DIR|FIND_ENV`; `Key` is name/pattern/path; `Value` is the optional location.
  - `func PlanFinds(m integrations.Manifest) ([]FindRequest, error)` — one request per declared binary, file pattern, dir, env, in that order; error if a file pattern contains whitespace (D6).
  - `type FindResult struct{ Matches []integrations.Path; Exists bool; Value string }`
  - `func ParseFindResponse(op, raw string) (FindResult, error)` — handles `ok`, `not_found` (empty result, no error), `error` (returns an error carrying `message`); normalizes paths via `ParsePath`.
  - `func ApplyFindResults(rc *integrations.ResolvedContext, req FindRequest, res FindResult)` — for `FIND_BINARY` stores the first match only if exactly one (multi-match selection is Task 8); `FIND_FILE` stores all in `Files[pattern]`; `FIND_DIR` sets `Dirs[path]`; `FIND_ENV` sets `Envs[name]`.

- [ ] **Step 1: Write failing tests** (`package starchart_test`)

```go
func TestPlanFinds_OrderAndContent(t *testing.T) {
	m := integrations.Manifest{
		Integration: integrations.ManifestIntegration{Binaries: []string{"brew"}},
		Find: integrations.ManifestFind{
			Files: []string{"Brewfile"}, Dirs: []string{"/opt/homebrew"}, Envs: []string{"HOMEBREW_PREFIX"},
		},
	}
	got, err := starchart.PlanFinds(m)
	require.NoError(t, err)
	assert.Equal(t, []starchart.FindRequest{
		{Op: "FIND_BINARY", Key: "brew"},
		{Op: "FIND_FILE", Key: "Brewfile"},
		{Op: "FIND_DIR", Key: "/opt/homebrew"},
		{Op: "FIND_ENV", Key: "HOMEBREW_PREFIX"},
	}, got)
}

func TestPlanFinds_RejectsPatternWithSpace(t *testing.T) {
	_, err := starchart.PlanFinds(integrations.Manifest{Find: integrations.ManifestFind{Files: []string{"my file"}}})
	assert.Error(t, err)
}

func TestParseFindResponse(t *testing.T) {
	res, err := starchart.ParseFindResponse("FIND_BINARY", `{"status":"ok","matches":["/usr/bin/git","/opt/homebrew/bin/git"]}`)
	require.NoError(t, err)
	require.Len(t, res.Matches, 2)
	assert.Equal(t, "/usr/bin/git", res.Matches[0].Unix)

	res, err = starchart.ParseFindResponse("FIND_BINARY", `{"status":"ok","matches":[]}`)
	require.NoError(t, err)
	assert.Empty(t, res.Matches)

	res, err = starchart.ParseFindResponse("FIND_FILE", `{"status":"not_found","message":"location does not exist: /x"}`)
	require.NoError(t, err)
	assert.Empty(t, res.Matches)

	_, err = starchart.ParseFindResponse("FIND_ENV", `{"status":"error","message":"boom"}`)
	assert.ErrorContains(t, err, "boom")

	res, err = starchart.ParseFindResponse("FIND_ENV", `{"status":"ok","name":"X","value":"v"}`)
	require.NoError(t, err)
	assert.Equal(t, "v", res.Value)

	res, err = starchart.ParseFindResponse("FIND_DIR", `{"status":"ok","path":"/tmp","exists":true}`)
	require.NoError(t, err)
	assert.True(t, res.Exists)

	_, err = starchart.ParseFindResponse("FIND_BINARY", `not json`)
	assert.Error(t, err)
}

func TestApplyFindResults(t *testing.T) {
	rc := integrations.ResolvedContext{}
	p, _ := integrations.ParsePath("/usr/bin/git")
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_BINARY", Key: "git"}, starchart.FindResult{Matches: []integrations.Path{p}})
	assert.Equal(t, p.IntoNative(), rc.Binaries["git"])

	// two matches: ambiguous, must NOT be injected (the Captain picks in Task 8)
	q, _ := integrations.ParsePath("/opt/homebrew/bin/git")
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_BINARY", Key: "node"}, starchart.FindResult{Matches: []integrations.Path{p, q}})
	_, ok := rc.Binaries["node"]
	assert.False(t, ok)

	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_ENV", Key: "JAVA_HOME"}, starchart.FindResult{Value: "/jdk"})
	assert.Equal(t, "/jdk", rc.Envs["JAVA_HOME"])
	starchart.ApplyFindResults(&rc, starchart.FindRequest{Op: "FIND_DIR", Key: "/opt"}, starchart.FindResult{Exists: true})
	assert.True(t, rc.Dirs["/opt"])
}
```

- [ ] **Step 2: Run** `go test ./internal/starchart/ -run 'PlanFinds|ParseFindResponse|ApplyFindResults'` — Expected: FAIL (undefined).

- [ ] **Step 3: Implement** `internal/starchart/find.go`

```go
package starchart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// FindRequest is one discovery question Orbiter asks the shell.
type FindRequest struct {
	Op    string // FIND_BINARY | FIND_FILE | FIND_DIR | FIND_ENV
	Key   string // binary name, glob pattern, path, or env var name
	Value string // optional: FIND_FILE location
}

// FindResult is a parsed, path-normalized FIND response.
type FindResult struct {
	Matches []integrations.Path
	Exists  bool
	Value   string
}

// PlanFinds lists every FIND the manifest asks Orbiter to run on its behalf.
func PlanFinds(m integrations.Manifest) ([]FindRequest, error) {
	var reqs []FindRequest
	for _, b := range m.Integration.Binaries {
		reqs = append(reqs, FindRequest{Op: "FIND_BINARY", Key: b})
	}
	for _, f := range m.Find.Files {
		if strings.ContainsAny(f, " \t\n") {
			return nil, fmt.Errorf("find pattern %q must not contain whitespace", f)
		}
		reqs = append(reqs, FindRequest{Op: "FIND_FILE", Key: f})
	}
	for _, d := range m.Find.Dirs {
		reqs = append(reqs, FindRequest{Op: "FIND_DIR", Key: d})
	}
	for _, e := range m.Find.Envs {
		reqs = append(reqs, FindRequest{Op: "FIND_ENV", Key: e})
	}
	return reqs, nil
}

type findWire struct {
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Matches []string `json:"matches"`
	Path    string   `json:"path"`
	Exists  bool     `json:"exists"`
	Value   string   `json:"value"`
}

// ParseFindResponse decodes the JSON a shell FIND function relayed.
// "ok" and "not_found" both succeed (empty result); "error" is an error.
func ParseFindResponse(op, raw string) (FindResult, error) {
	var w findWire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return FindResult{}, fmt.Errorf("%s: invalid response: %w", op, err)
	}
	switch w.Status {
	case "ok":
	case "not_found":
		return FindResult{}, nil
	case "error":
		return FindResult{}, fmt.Errorf("%s: %s", op, w.Message)
	default:
		return FindResult{}, fmt.Errorf("%s: unknown status %q", op, w.Status)
	}
	res := FindResult{Exists: w.Exists, Value: w.Value}
	for _, m := range w.Matches {
		p, err := integrations.ParsePath(m)
		if err != nil {
			return FindResult{}, fmt.Errorf("%s: bad path %q: %w", op, m, err)
		}
		res.Matches = append(res.Matches, p)
	}
	return res, nil
}

// ApplyFindResults injects one result into rc. A FIND_BINARY with more than one
// match is ambiguous and is left unset: the Captain chooses (see uplink SELECT).
func ApplyFindResults(rc *integrations.ResolvedContext, req FindRequest, res FindResult) {
	switch req.Op {
	case "FIND_BINARY":
		if len(res.Matches) != 1 {
			return
		}
		if rc.Binaries == nil {
			rc.Binaries = map[string]string{}
		}
		rc.Binaries[req.Key] = res.Matches[0].IntoNative()
	case "FIND_FILE":
		if rc.Files == nil {
			rc.Files = map[string][]string{}
		}
		paths := make([]string, 0, len(res.Matches))
		for _, m := range res.Matches {
			paths = append(paths, m.IntoNative())
		}
		rc.Files[req.Key] = paths
	case "FIND_DIR":
		if rc.Dirs == nil {
			rc.Dirs = map[string]bool{}
		}
		rc.Dirs[req.Key] = res.Exists
	case "FIND_ENV":
		if rc.Envs == nil {
			rc.Envs = map[string]string{}
		}
		rc.Envs[req.Key] = res.Value
	}
}
```

- [ ] **Step 4: Run** `go test ./internal/starchart/ -run 'PlanFinds|ParseFindResponse|ApplyFindResults'` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: add FIND request planning, response parsing, and context injection` (with trailer).

### Task 5a: Host-aware `ParsePath` for Git Bash and Cygwin paths

**Why:** On Windows, the bash and zsh FIND functions run under Git Bash, which reports drive paths as `/c/Users/me/bin/git.exe` (Cygwin: `/cygdrive/c/Users/me/...`). `ParsePath` only understands `C:\` and Unix paths, so that string would become a bogus Unix path (`Windows` = `\c\Users\me\...`) that never exists. Orbiter owns OS path semantics, so Orbiter normalizes it. The conversion applies only when the *host* is Windows: `/c/Users` is a perfectly valid Unix path anywhere else.

**Files:**
- Modify: `internal/integrations/filesystem.go` (`ParsePath`)
- Test: `internal/integrations/filesystem_test.go` (same `package integrations`, so unexported helpers are reachable)

**Interfaces:**
- Consumes: existing `ParsePath(raw string) (Path, error)` behavior for quoting, `~`, drive letters, and `filepath.Abs`.
- Produces: `ParsePath` keeps its signature and delegates to `parsePathFor(goos, raw string) (Path, error)` (so tests can exercise the Windows branch on any machine) and `msysToDrive(raw string) string` (pure, host-independent).

**Verification limit:** this is tested through simulated `goos` values only. It has not run on a real Windows host or Git Bash. Real verification is planned once the GUI exists; until then treat the Git Bash behavior (that it emits `/c/...`) as an assumption.

- [ ] **Step 1: Write the failing tests** (append to `filesystem_test.go`)

```go
func TestParsePath_GitBashOnWindowsHost(t *testing.T) {
	cases := []struct{ in, wantUnix, wantWin string }{
		{"/c/Users/me/bin/git.exe", "C:/Users/me/bin/git.exe", `C:\Users\me\bin\git.exe`},
		{"/d/tools", "D:/tools", `D:\tools`},
		{"/cygdrive/c/Users/me", "C:/Users/me", `C:\Users\me`},
		{"/c", "C:/", `C:\`},
		{`"/c/Program Files/Git/bin/git.exe"`, "C:/Program Files/Git/bin/git.exe", `C:\Program Files\Git\bin\git.exe`},
		// PowerShell-style input must keep working unchanged.
		{"C:/Users/me/git.exe", "C:/Users/me/git.exe", `C:\Users\me\git.exe`},
	}
	for _, c := range cases {
		got, err := parsePathFor("windows", c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got.Unix != c.wantUnix || got.Windows != c.wantWin {
			t.Errorf("%q: got {%q %q}, want {%q %q}", c.in, got.Unix, got.Windows, c.wantUnix, c.wantWin)
		}
	}
}

func TestParsePath_GitBashFormLeftAloneOnUnixHosts(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		got, err := parsePathFor(goos, "/c/Users/me")
		if err != nil {
			t.Fatal(err)
		}
		if got.Unix != "/c/Users/me" {
			t.Errorf("%s: got %q, want the path untouched", goos, got.Unix)
		}
	}
}

func TestMsysToDrive_NonDriveInputUnchanged(t *testing.T) {
	for _, in := range []string{"/usr/bin/git", "/code/x", "/cygdrive", "/cygdrivefoo/x", "C:/x", "relative/p", ""} {
		if got := msysToDrive(in); got != in {
			t.Errorf("msysToDrive(%q) = %q, want unchanged", in, got)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/integrations/ -run 'GitBash|MsysToDrive'`
Expected: FAIL — `parsePathFor` and `msysToDrive` undefined.

- [ ] **Step 3: Implement** — in `filesystem.go` add `"runtime"` to imports. Rename the existing function body to `parsePathFor`, keeping the doc comment on `ParsePath`:

```go
// ParsePath converts any path string into a Path carrying all separator forms.
// (existing doc comment unchanged, plus:)
// On a Windows host it also accepts Git Bash (/c/Users/x) and Cygwin
// (/cygdrive/c/Users/x) drive paths.
func ParsePath(raw string) (Path, error) {
	return parsePathFor(runtime.GOOS, raw)
}

func parsePathFor(goos, raw string) (Path, error) {
	// ... existing body of ParsePath, unchanged, up to and including the tilde block ...

	// Git Bash / Cygwin report drive paths as /c/... — meaningful only on a
	// Windows host, where it must become C:/... before the drive-letter check.
	if goos == "windows" {
		raw = msysToDrive(raw)
	}

	// ... existing drive-letter check and filepath.Abs tail, unchanged ...
}

// msysToDrive rewrites /c/Users/x and /cygdrive/c/Users/x to C:/Users/x.
// Anything that is not a single-letter drive prefix is returned unchanged.
func msysToDrive(raw string) string {
	rest := strings.TrimPrefix(raw, "/cygdrive")
	if len(rest) < 2 || rest[0] != '/' {
		return raw
	}
	c := rest[1]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return raw
	}
	tail := rest[2:]
	if tail == "" {
		tail = "/"
	} else if tail[0] != '/' {
		return raw // "/code/x" is a directory name, not a drive letter
	}
	return strings.ToUpper(string(c)) + ":" + tail
}
```

- [ ] **Step 4: Run** `go test ./internal/integrations/` — Expected: PASS, including every pre-existing `ParsePath` test.

- [ ] **Step 5: Commit** `feat: normalize Git Bash and Cygwin drive paths on Windows hosts` (with trailer).

### Task 6: Inject recorded binaries; delete the stub

**Why:** After this task, scans work for any resource whose binary path is already recorded (`config.binaries`), which is the steady state after Part B's INIT flow. The recorded path is injected as-is, even if it is stale; the integration reports whether the binary is still present and reachable, and Orbiter stores that report. (Part B turns a not-present report into FIND-based recovery.)

**Files:**
- Modify: `internal/starchart/find.go` (add helper), `internal/starchart/crawl.go:BuildResolvedContext`, `internal/wasm/loader.go`
- Test: `internal/starchart/find_test.go`

**Interfaces:**
- Produces: `func recordedBinaries(self integrations.Entity, declared []string) map[string]string` — reads `self`'s config JSON key `binaries` (object name → path string), returns every name in `declared` that has a recorded path, each normalized with `ParsePath(...).IntoNative()` for the current host. It does not check that the path exists. Called inside `BuildResolvedContext` to fill `Binaries`.

Config shape written by Part B and read here: `{"binaries":{"brew":"/opt/homebrew/bin/brew"}}`.

- [ ] **Step 1: Check what `Entity` exposes for config** — `grep -n "type Entity" -A12 internal/integrations/types.go`. Use whichever getter returns the config JSON string (the resource's `Config`). Use that in Step 3.

- [ ] **Step 2: Write failing test** (export the helper through `internal/starchart/export_test.go` as `RecordedBinaries`, matching existing `export_test.go` conventions, or test through `BuildResolvedContext` directly)

```go
func TestBuildResolvedContext_InjectsRecordedBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "brew")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	cfg, _ := json.Marshal(map[string]any{"binaries": map[string]string{"brew": bin, "gone": "/no/such/gone"}})
	r := models.Resource{ID: "r1", Role: "manager", Brand: "brew", Config: string(cfg)}
	m := integrations.Manifest{Integration: integrations.ManifestIntegration{Binaries: []string{"brew", "gone", "unrecorded"}}}

	rc := starchart.BuildResolvedContext(r, starchart.LeveledBranch{}, m)

	assert.Equal(t, bin, rc.Binaries["brew"])
	// Orbiter does not verify the path: a stale record is injected as-is and the
	// integration's own report says whether the binary is still there.
	assert.Equal(t, "/no/such/gone", filepath.ToSlash(rc.Binaries["gone"]))
	_, ok := rc.Binaries["unrecorded"]
	assert.False(t, ok, "a declared binary with no recorded path is simply absent")
}
```

- [ ] **Step 3: Run to fail, then implement**

Run: `go test ./internal/starchart/ -run InjectsRecordedBinary` — Expected: FAIL (`Binaries` empty).

Add to `find.go`:

```go
// recordedBinaries returns the recorded path for each declared binary,
// normalized for this host. It deliberately does not check that the path
// exists: whether the binary is still present and reachable is the
// integration's report to make. Recording happens at INIT/drift recovery (uplink).
func recordedBinaries(configJSON string, declared []string) map[string]string {
	var cfg struct {
		Binaries map[string]string `json:"binaries"`
	}
	if configJSON == "" || json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return nil
	}
	out := map[string]string{}
	for _, name := range declared {
		raw, ok := cfg.Binaries[name]
		if !ok {
			continue
		}
		p, err := integrations.ParsePath(raw)
		if err != nil || p.IntoNative() == "" {
			continue
		}
		out[name] = p.IntoNative()
	}
	return out
}
```

In `BuildResolvedContext` set the returned struct's `Binaries` field from `recordedBinaries(<self config JSON>, manifest.Integration.Binaries)` using the getter found in Step 1. In `internal/wasm/loader.go` delete `populateBinaries` (the doc comment at ~145 plus function) and the three `ctx = w.populateBinaries(ctx)` lines in `Init`, `Scan`, `Calibrate`.

- [ ] **Step 4: Run** `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: inject recorded binary paths into ResolvedContext; remove populateBinaries stub` (with trailer).

---

# Part B — Round trip (uses the existing sessions and transactions)

> **Existing pieces Part B builds on:** the `sessions` and `transactions` tables, `InsertSession`/`GetSession`/`UpdateSessionStatus`/`InsertTransaction`/`RelayTransaction`/`GetRelayedTransactions` in `internal/starchart/transaction.go`, `orbiter relay` and `orbiter uplink`, and the `tx` IDs already carried on output events. What FIND adds is the first production call sites that write a session and its transactions when emitting directives (Task 7) and the `uplink` handling of FIND results (Task 8). Written against the I/O spec §5.8.

### Task 7: Emit sessions and FIND directives when a binary has no recorded path

**Files:**
- Modify: `internal/commands/executor.go` (`Calibrate`, `Chart`; used by `Scan` too if the Captain wants discovery on scan), `internal/commands/lifecycle.go` (print directives), `internal/starchart/lifecycle.go` (helper `NeededFinds`)
- Test: `internal/commands/executor_test.go`, `internal/starchart/find_test.go`

**Interfaces:**
- Consumes: `PlanFinds`, `recordedBinaries` (Task 5/6), `sc.InsertSession(ctx, id, meta string) error`, `sc.InsertTransaction(ctx, starchart.Transaction) error`, `models.NewID("ss"|"tx")`.
- Produces: `func (sc *StarChart) NeededFinds(scan BranchScanResult) []ResourceFind` where `type ResourceFind struct{ ResourceID string; Request FindRequest }` lists a `FIND_BINARY` request for each non-filesystem resource and each binary its manifest declares when **either** the resource config has no recorded path for that binary (first INIT) **or** the integration's own scan report says `Present == false` (the integration could not use the recorded path). Orbiter never inspects the path itself. `FIND_FILE/DIR/ENV` requests are not emitted in this plan: no integration declares `[find]` yet. `Calibrate` scans first, and if finds are needed, writes one session (`meta` = `{"op":"<command>","target":"<alias>"}`) and one pending transaction per request (op = request op, args = JSON `{"resource_id":..., "key":..., "value":...}`) **before** returning `SESSION` + `FIND_*` directives.

- [ ] **Step 1: Failing tests** — (a) `NeededFinds` returns the `FIND_BINARY` request for a resource whose manifest declares `brew` and whose config has no `binaries`; returns one when a path is recorded but the scan report has `Present == false`; and returns nothing when a path is recorded and the report has `Present == true`; (b) executor `Calibrate` on such a resource returns directives `[SESSION <id>, FIND_BINARY <txn> brew]`, and the session and transaction rows exist with status `active`/`pending` when the directives are returned.

Write both using the fake integration pattern at the top of `internal/starchart/lifecycle_test.go` (`healthyIntegration`, with `Meta()` returning a manifest that declares `Binaries: []string{"brew"}`).

- [ ] **Step 2: Run to fail**, then **Step 3: implement** `NeededFinds` (iterate `scan.Resources`, skip filesystem-role, get the integration's manifest, and apply the rule above using `recordedBinaries` to see whether a path is recorded). In the executor's `Calibrate`, run `ScanBranch` first and call `NeededFinds`; if non-empty, `InsertSession`, insert one transaction per request, and return the directives instead of calibrating. Command handlers print each `Directive.String()` on its own line (the existing hook/jump printing loop) and return exit 0.

- [ ] **Step 4: Run** `go test ./internal/...` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: emit session and FIND directives for unresolved binaries` (with trailer).

### Task 8: `uplink` ingests FIND results

**Files:**
- Modify: `internal/commands/uplink.go`, `internal/commands/uplink_test.go`
- Modify: `internal/starchart/find.go` (add `RecordBinary`)

**Interfaces:**
- Consumes: `ParseFindResponse`, `sc.GetRelayedTransactions`, `sc.MarkTransactionProcessed`, `renderer.Prompt(output.CaptainPrompt)`.
- Produces: `func (sc *StarChart) RecordBinary(ctx, resourceID, name, path string) error` — merges `{"binaries":{name:path}}` into the resource's existing config JSON (preserving other keys) and saves it.

Behavior for each relayed FIND transaction, driven by its `args`:

| Parsed result | Action |
|---------------|--------|
| error status / invalid JSON | print a warning to stderr, mark processed, do not change config |
| `FIND_BINARY`, 0 matches | stderr observation "`<name>` not found in PATH", mark processed, config unchanged |
| `FIND_BINARY`, 1 match, nothing recorded yet | `RecordBinary` (first INIT) |
| `FIND_BINARY`, 1 match, a different path already recorded | insert a pending `CONFIRM` transaction ("`<name>` moved from `<old>` to `<new>`, update?") and prompt; `RecordBinary` only after the Captain approves (no auto-update on recovery) |
| `FIND_BINARY`, >1 matches | insert a pending `SELECT` transaction (`args.matches` = candidates, `args.resource_id`, `args.key`), `renderer.Prompt(CaptainPrompt{Op:"SELECT", Variant:"single", Options: matches})`, leave session `active` |
| `SELECT` | `RecordBinary` with the chosen option, then mark processed |
| other `FIND_*` | no persistence in this plan; mark processed |

Each interaction in the flow is its own breakpoint: a multi-match or an approval creates a **new** `tx` in the same session, and `uplink` returns after emitting it. The session stores what is needed to continue (`meta` = `{"op":"calibrate","target":"<alias>"}` from Task 7). Once no `pending` transactions remain, `uplink` continues the flow by re-running `meta.op` on `meta.target` with the newly recorded paths, then marks the session `complete`. Remove the stub branch and the "FIND_* op processing is deferred" text in the command's `Long` help.

- [ ] **Step 1: Replace `TestUplinkCmd_FIND_stub`** with tests using `setupUplinkDB` (already in `uplink_test.go`):
  - single match → `RecordBinary` ran (read the resource back and assert `config.binaries.brew`);
  - zero matches → exit 0, config has no `binaries`, stderr mentions "not found";
  - two matches → a `SELECT` transaction now exists and the session is still `active`;
  - invalid JSON response → exit 0, config unchanged (Review Focus #5);
  - relaying to an unknown txn, then running uplink → no panic and no state change.
  Assert on StarChart state, not on stdout text.

- [ ] **Step 2: Run to fail**, then **Step 3: implement** the table above in `processTransaction` (it now needs the `sc`/session; pass `d.sc` and the session ID in).

- [ ] **Step 4: Run** `go test ./internal/commands/ ./internal/starchart/` — Expected: PASS.

- [ ] **Step 5: Commit** `feat: uplink ingests FIND results and records resolved binaries` (with trailer).

### Task 9: Recovery round trip and cleanup

Drift is whatever the integration reports (`Present == false`); Orbiter stores the beacon from that report as it already does, and adds no path checks of its own. This task proves the whole recovery loop works end to end and finishes the docs.

**Files:**
- Test: `internal/starchart/lifecycle_test.go`, `internal/commands/uplink_test.go`
- Modify: docs

- [ ] **Step 1: Write the round-trip test** (fake integration whose `Scan` returns `Present: true` only when `ctx.Binaries["brew"]` points at an existing file, and `Present: false` otherwise):
  1. Resource config records `binaries.brew = /no/such/brew`. `ScanBranch` yields a non-healthy beacon (from the integration's report) and the resource config is **unchanged** (no auto-update).
  2. `Calibrate` emits `SESSION` + `FIND_BINARY` for it (Task 7).
  3. Relay a one-match reply naming a real temp file, run `uplink`: a `CONFIRM` is raised (a different path was recorded). Relay approval, run `uplink` again: config now records the new path.
  4. After the approval, `uplink` has continued the flow by re-running `calibrate` from the session `meta`; the resource is healthy and no further FIND is needed.

- [ ] **Step 2: Run** `go test ./internal/starchart/ ./internal/commands/` — Expected: PASS. If step 1 fails, fix the earlier task that owns the broken link; this task adds no new production code.

- [ ] **Step 3: Run** `go test ./...` — Expected: PASS.

- [ ] **Step 4: Docs and cleanup**
  - Edit `docs/superpowers/plans/2026-06-22-binary-resolution-shell-directive.md`: replace the top with `> **Status:** Superseded by [2026-10-26-find-implementation.md](2026-10-26-find-implementation.md)`.
  - Edit `2026-06-23-find-interface-spec.md` status to `Implemented by 2026-10-26-find-implementation.md`, and in `2026-06-25-io-contract-spec.md` §5.4–5.5 note that the directive format and FIND_BINARY semantics follow Reconciliation Decisions D1/D2 of this plan.
  - Update `docs/integrations.md` with the `[find]` manifest section.

- [ ] **Step 5: Commit** `test: end-to-end FIND recovery loop; mark FIND specs implemented` (with trailer).

---

## Self-Review

**Spec coverage.** FIND functions ×4 shells → Task 4. `[find]` manifest + Go types → Task 2. `ResolvedContext` fields → Tasks 2, 5, 6. Directive mapping → Task 3. `populateBinaries` removal → Task 6. Lifecycle emission → Task 7. Uplink ingestion + Captain selection → Task 8. Drift → Task 9. Security (FIND not callable via `run_command`) needs no code: the functions are not in any manifest allowlist; no task adds them. FIND spec §10 items (non-PATH search, depth, env globs) are deliberately excluded.

**Known gaps (intentional).** `ResolvedContext.Files/Dirs/Envs` are populated by `ApplyFindResults` but nothing persists them across scans; D4 limits discovery to INIT/drift, and no built-in guest reads them yet. Add persistence when the first integration declares `[find]`.

**Unverified on Windows.** Task 5a is tested with simulated `goos` values; that Git Bash emits `/c/...`, and that bash `-f`/`-x` find `git.exe` on Windows, are assumptions. Verify on a real Windows host once the GUI exists.

**Present/Reachable.** The FIND spec's Present/Reachable refactor (Orbiter owns Present via `os.Stat`, Reachable derived from FIND matches) is **not** adopted. Integrations self-report both in their `StateReport` and Orbiter stores it. FIND only supplies candidate locations. The refactor spec, if still wanted, should be reduced to that.

**Open questions for the Captain.** (1) D3: confirm keeping string-typed `Binaries` on the wire.
