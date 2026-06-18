# Foundation: Neutral Directive Protocol + Manifest Env Detection

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace bash-syntax `ShellDirective` output with a brand-agnostic line protocol, and extend `ManifestDetection` with env-var rules so integrations can declare their own detection conditions without branded code in the binary.

**Architecture:** `Directive` replaces `ShellDirective` — the binary now emits `DIR`, `SET`, `UNSET` lines that any shell script can parse without knowing about the binary's internals. `ManifestDetection` gains an `Env []ManifestEnvRule` field; a new `MatchesAny` method lets `DiscoverPlanet` pre-filter integrations before calling their WASM.

**Tech Stack:** Go 1.23+, `github.com/BurntSushi/toml`, bash/zsh/fish shell scripts.

## Global Constraints

- No new external dependencies.
- All existing tests must pass after each task.
- Shell scripts (`orbiter.bash`, `orbiter.zsh`, `orbiter.fish`) remain embedded in the binary via `go:embed`.
- `Directive.String()` output must be parseable by a simple line-by-line `case` in any POSIX shell.

---

## File Map

| File | Change |
|---|---|
| `internal/integrations/manifest.go` | Add `ManifestEnvRule`, extend `ManifestDetection.Env`, add `MatchesAny` |
| `internal/integrations/manifest_test.go` | Tests for env rules and `MatchesAny` |
| `internal/integrations/types.go` | Add `Env map[string]string` to `DetectContext` |
| `internal/starchart/discover.go` | Populate `DetectContext.Env` from `os.Environ()`, add `MatchesAny` pre-filter |
| `internal/starchart/discover_test.go` | Test pre-filter skips non-matching integrations |
| `internal/commands/executor.go` | Rename `ShellDirective` → `Directive`, update `String()`, update `Jump` |
| `internal/commands/executor_test.go` | Update Jump test assertions for new format |
| `internal/commands/shell/orbiter.bash` | Parse `DIR`/`SET`/`UNSET` instead of eval-ing shell syntax |
| `internal/commands/shell/orbiter.zsh` | Same |
| `internal/commands/shell/orbiter.fish` | Same |

---

### Task 1: Add ManifestEnvRule, extend ManifestDetection, add MatchesAny

**Files:**
- Modify: `internal/integrations/manifest.go`
- Modify: `internal/integrations/manifest_test.go`

**Interfaces:**
- Produces: `ManifestEnvRule{Key, Pattern string}`, `ManifestDetection.Env []ManifestEnvRule`, `(ManifestDetection).MatchesAny(files map[string]string, env map[string]string) bool`

- [ ] **Step 1: Write the failing tests**

```go
// internal/integrations/manifest_test.go — add after existing tests

func TestManifestDetection_MatchesAny_NoRules(t *testing.T) {
    d := integrations.ManifestDetection{}
    // no rules = always matches (integration may have WASM detect logic)
    if !d.MatchesAny(nil, nil) {
        t.Error("empty detection should match anything")
    }
}

func TestManifestDetection_MatchesAny_FileHit(t *testing.T) {
    d := integrations.ManifestDetection{Files: []string{"go.mod"}}
    files := map[string]string{"go.mod": "", "README.md": ""}
    if !d.MatchesAny(files, nil) {
        t.Error("go.mod present should match")
    }
}

func TestManifestDetection_MatchesAny_FileMiss(t *testing.T) {
    d := integrations.ManifestDetection{Files: []string{"go.mod"}}
    files := map[string]string{"package.json": ""}
    if d.MatchesAny(files, nil) {
        t.Error("no matching file should not match")
    }
}

func TestManifestDetection_MatchesAny_EnvKeyPresent(t *testing.T) {
    d := integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{{Key: "ZSH_VERSION"}},
    }
    env := map[string]string{"ZSH_VERSION": "5.9", "USER": "kent"}
    if !d.MatchesAny(nil, env) {
        t.Error("ZSH_VERSION present should match")
    }
}

func TestManifestDetection_MatchesAny_EnvKeyAbsent(t *testing.T) {
    d := integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{{Key: "ZSH_VERSION"}},
    }
    env := map[string]string{"BASH_VERSION": "5.2"}
    if d.MatchesAny(nil, env) {
        t.Error("ZSH_VERSION absent should not match")
    }
}

func TestManifestDetection_MatchesAny_EnvPattern(t *testing.T) {
    d := integrations.ManifestDetection{
        Env: []integrations.ManifestEnvRule{{Key: "SHELL", Pattern: "zsh"}},
    }
    if !d.MatchesAny(nil, map[string]string{"SHELL": "/usr/bin/zsh"}) {
        t.Error("SHELL containing 'zsh' should match")
    }
    if d.MatchesAny(nil, map[string]string{"SHELL": "/usr/bin/bash"}) {
        t.Error("SHELL not containing 'zsh' should not match")
    }
}

func TestManifestDetection_MatchesAny_FileOrEnvEither(t *testing.T) {
    d := integrations.ManifestDetection{
        Files: []string{"go.mod"},
        Env:   []integrations.ManifestEnvRule{{Key: "GOPATH"}},
    }
    // env hit only
    if !d.MatchesAny(nil, map[string]string{"GOPATH": "/home/kent/go"}) {
        t.Error("env hit should match even without file hit")
    }
    // file hit only
    if !d.MatchesAny(map[string]string{"go.mod": ""}, nil) {
        t.Error("file hit should match even without env hit")
    }
    // neither
    if d.MatchesAny(map[string]string{"package.json": ""}, map[string]string{"NODE_ENV": "dev"}) {
        t.Error("neither file nor env hit should not match")
    }
}

func TestManifest_ParseEnvDetection(t *testing.T) {
    const src = `
[integration]
brand = "zsh"
roles = ["shell"]

[detection]
env = [
  { key = "ZSH_VERSION" },
  { key = "SHELL", pattern = "zsh" },
]
`
    var m integrations.Manifest
    if _, err := toml.Decode(src, &m); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(m.Detection.Env) != 2 {
        t.Fatalf("expected 2 env rules, got %d", len(m.Detection.Env))
    }
    if m.Detection.Env[0].Key != "ZSH_VERSION" {
        t.Errorf("rule[0].key = %q, want ZSH_VERSION", m.Detection.Env[0].Key)
    }
    if m.Detection.Env[1].Pattern != "zsh" {
        t.Errorf("rule[1].pattern = %q, want zsh", m.Detection.Env[1].Pattern)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/integrations/... -run "TestManifestDetection_MatchesAny|TestManifest_ParseEnvDetection" -v
```
Expected: FAIL — `ManifestEnvRule` undefined, `MatchesAny` undefined.

- [ ] **Step 3: Add ManifestEnvRule, extend ManifestDetection, implement MatchesAny**

In `internal/integrations/manifest.go`, add `strings` import and update `ManifestDetection`:

```go
import "strings"

// ManifestEnvRule is one env-var detection condition in the [detection] section.
// If Pattern is non-empty, the env var's value must contain it as a substring.
// If Pattern is empty, the env var need only be present with a non-empty value.
type ManifestEnvRule struct {
    Key     string `toml:"key"`
    Pattern string `toml:"pattern"`
}

// ManifestDetection is the [detection] section.
type ManifestDetection struct {
    Files []string          `toml:"files"`
    Env   []ManifestEnvRule `toml:"env"`
}

// MatchesAny reports whether at least one detection rule is satisfied.
// Returns true when there are no rules — the integration may have custom WASM detect logic.
func (d ManifestDetection) MatchesAny(files map[string]string, env map[string]string) bool {
    if len(d.Files) == 0 && len(d.Env) == 0 {
        return true
    }
    for _, f := range d.Files {
        if _, ok := files[f]; ok {
            return true
        }
    }
    for _, rule := range d.Env {
        val, ok := env[rule.Key]
        if !ok || val == "" {
            continue
        }
        if rule.Pattern == "" || strings.Contains(val, rule.Pattern) {
            return true
        }
    }
    return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/integrations/... -run "TestManifestDetection_MatchesAny|TestManifest_ParseEnvDetection" -v
```
Expected: PASS

- [ ] **Step 5: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/integrations/manifest.go internal/integrations/manifest_test.go
git commit -m "feat: add ManifestEnvRule and MatchesAny to ManifestDetection"
```

---

### Task 2: Add Env to DetectContext, pre-filter DiscoverPlanet

**Files:**
- Modify: `internal/integrations/types.go`
- Modify: `internal/starchart/discover.go`
- Create: `internal/starchart/discover_test.go`

**Interfaces:**
- Consumes: `ManifestDetection.MatchesAny` from Task 1
- Produces: `DetectContext.Env map[string]string` populated by `DiscoverPlanet`

- [ ] **Step 1: Write failing tests**

```go
// internal/starchart/discover_test.go
package starchart_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// skippingIntegration counts how many times Detect is called.
type countingIntegration struct {
    calls int
    brand string
    files []string
    envRules []integrations.ManifestEnvRule
}

func (c *countingIntegration) Meta() integrations.Manifest {
    return integrations.Manifest{
        Integration: integrations.ManifestIntegration{Brand: c.brand, Roles: []string{"tool"}},
        Detection:   integrations.ManifestDetection{Files: c.files, Env: c.envRules},
    }
}
func (c *countingIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
    c.calls++
    return integrations.DetectReport{Detected: false}
}
func (c *countingIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{}
}
func (c *countingIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{}
}
func (c *countingIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
    return integrations.StateReport{}
}

func openTestSC(t *testing.T) *starchart.StarChart {
    t.Helper()
    f, err := os.CreateTemp(t.TempDir(), "sc-*.db")
    require.NoError(t, err)
    f.Close()
    sc, err := starchart.Open(f.Name())
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })
    return sc
}

func TestDiscoverPlanet_EnvInContext(t *testing.T) {
    sc := openTestSC(t)
    reg := integrations.NewRegistry(nil)
    counter := &countingIntegration{brand: "test-env", files: []string{"go.mod"}}
    reg.Register("tool", "test-env", counter)
    sc.SetIntegrations(reg)

    dir := t.TempDir()
    gomod := filepath.Join(dir, "go.mod")
    os.WriteFile(gomod, []byte("module test"), 0644)

    t.Setenv("TEST_ENV_VAR", "hello")
    _, err := sc.DiscoverPlanet(context.Background(), dir)
    require.NoError(t, err)

    assert.Equal(t, 1, counter.calls, "integration with matching file should be called")
}

func TestDiscoverPlanet_PreFilterSkipsNonMatchingEnv(t *testing.T) {
    sc := openTestSC(t)
    reg := integrations.NewRegistry(nil)
    counter := &countingIntegration{
        brand:    "env-only",
        envRules: []integrations.ManifestEnvRule{{Key: "DEFINITELY_NOT_SET_XYZ"}},
    }
    reg.Register("tool", "env-only", counter)
    sc.SetIntegrations(reg)

    dir := t.TempDir()
    os.Unsetenv("DEFINITELY_NOT_SET_XYZ")
    _, err := sc.DiscoverPlanet(context.Background(), dir)
    require.NoError(t, err)

    assert.Equal(t, 0, counter.calls, "integration whose env rule doesn't match should be skipped")
}

func TestDiscoverPlanet_PreFilterSkipsNoMatchingFile(t *testing.T) {
    sc := openTestSC(t)
    reg := integrations.NewRegistry(nil)
    counter := &countingIntegration{
        brand: "file-only",
        files: []string{"Cargo.toml"},
    }
    reg.Register("tool", "file-only", counter)
    sc.SetIntegrations(reg)

    dir := t.TempDir()
    // no Cargo.toml in dir
    _, err := sc.DiscoverPlanet(context.Background(), dir)
    require.NoError(t, err)

    assert.Equal(t, 0, counter.calls, "integration whose file rule doesn't match should be skipped")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/starchart/... -run "TestDiscoverPlanet" -v
```
Expected: FAIL — `sc.SetIntegrations` may not exist or `DiscoverPlanet` doesn't pre-filter.

- [ ] **Step 3: Add Env to DetectContext**

In `internal/integrations/types.go`, update `DetectContext`:

```go
// DetectContext is passed to Detect. Files is populated only for file-pattern
// roles (runtime, manager, tool). Remote and shell integrations receive
// an empty Files map and inspect CWD or Env directly.
type DetectContext struct {
    Platform Platform          `json:"platform"`
    CWD      string            `json:"cwd"`
    Files    map[string]string `json:"files"`
    Env      map[string]string `json:"env"`
}
```

- [ ] **Step 4: Update DiscoverPlanet to populate env and pre-filter**

In `internal/starchart/discover.go`:

```go
package starchart

import (
    "context"
    "os"
    "path/filepath"
    "strings"

    "github.com/Kenttleton/orbiter/internal/integrations"
)

// DiscoverPlanet runs all registered integrations' Detect against cwd and returns
// suggested resources. Pre-filters by manifest detection rules before calling WASM.
func (sc *StarChart) DiscoverPlanet(ctx context.Context, cwd string) ([]integrations.SuggestedResource, error) {
    if sc.integrations == nil {
        return nil, nil
    }

    files, err := listFiles(cwd)
    if err != nil {
        return nil, err
    }
    env := osEnvMap()

    dc := integrations.DetectContext{
        Platform: currentPlatform(),
        CWD:      cwd,
        Files:    files,
        Env:      env,
    }

    var suggestions []integrations.SuggestedResource
    for _, i := range sc.integrations.All() {
        if !i.Meta().Detection.MatchesAny(files, env) {
            continue
        }
        report := i.Detect(dc)
        if report.Detected {
            suggestions = append(suggestions, report.Resources...)
        }
    }
    return suggestions, nil
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

// listFiles returns a map of filename → "" for every file directly in dir.
func listFiles(dir string) (map[string]string, error) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }
    files := make(map[string]string, len(entries))
    for _, e := range entries {
        if !e.IsDir() {
            files[filepath.Base(e.Name())] = ""
        }
    }
    return files, nil
}
```

- [ ] **Step 5: Verify SetIntegrations exists on StarChart**

Check `internal/starchart/` for a `SetIntegrations` method. If it doesn't exist, add it to `internal/starchart/starchart.go`:

```go
// SetIntegrations replaces the integration registry (used in tests).
func (sc *StarChart) SetIntegrations(r *integrations.Registry) {
    sc.integrations = r
}
```

- [ ] **Step 6: Run tests to verify they pass**

```
go test ./internal/starchart/... -run "TestDiscoverPlanet" -v
```
Expected: PASS

- [ ] **Step 7: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/integrations/types.go internal/starchart/discover.go internal/starchart/discover_test.go
git commit -m "feat: add env detection to DetectContext and pre-filter DiscoverPlanet by manifest rules"
```

---

### Task 3: Replace ShellDirective with neutral Directive, update Jump, update shell scripts

**Files:**
- Modify: `internal/commands/executor.go`
- Modify: `internal/commands/executor_test.go`
- Modify: `internal/commands/shell/orbiter.bash`
- Modify: `internal/commands/shell/orbiter.zsh`
- Modify: `internal/commands/shell/orbiter.fish`

**Interfaces:**
- Consumes: `StateReport.Exports map[string]string`, `StateReport.InstallDir string`
- Produces: `Directive{Op, Key, Value string}`, `Directive.String()` emitting `DIR <path>`, `SET <key>=<value>`, `UNSET <key>`

**Protocol spec:**
```
DIR /absolute/path           → shell should cd to this path
SET KEY=value with spaces    → shell should export KEY with the given value
UNSET KEY                    → shell should unset KEY
```

- [ ] **Step 1: Write failing tests**

In `internal/commands/executor_test.go`, update the Jump test and add directive format tests:

```go
func TestDirective_String_DIR(t *testing.T) {
    d := commands.Directive{Op: "DIR", Value: "/home/kent/work"}
    assert.Equal(t, "DIR /home/kent/work", d.String())
}

func TestDirective_String_SET(t *testing.T) {
    d := commands.Directive{Op: "SET", Key: "NODE_VERSION", Value: "20"}
    assert.Equal(t, "SET NODE_VERSION=20", d.String())
}

func TestDirective_String_SET_WithSpaces(t *testing.T) {
    d := commands.Directive{Op: "SET", Key: "GREETING", Value: "hello world"}
    assert.Equal(t, "SET GREETING=hello world", d.String())
}

func TestDirective_String_UNSET(t *testing.T) {
    d := commands.Directive{Op: "UNSET", Key: "NODE_VERSION"}
    assert.Equal(t, "UNSET NODE_VERSION", d.String())
}

func TestExecutor_Jump_EmitsNeutralDirectives(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    path := t.TempDir()
    r, _ := exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
    _, _ = exec.SC().Attach(ctx, r.Name, p.Name)

    directives, err := exec.Jump(ctx, "payment-api", true)
    require.NoError(t, err)
    require.NotEmpty(t, directives)

    assert.Equal(t, "DIR", directives[0].Op)
    assert.Equal(t, path, directives[0].Value)
    // String() must use neutral format, not shell syntax
    assert.Equal(t, "DIR "+path, directives[0].String())
    assert.NotContains(t, directives[0].String(), "cd ")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/commands/... -run "TestDirective|TestExecutor_Jump_EmitsNeutral" -v
```
Expected: FAIL — `commands.Directive` undefined, Jump test fails on role mismatch (`"shell"` vs `"filesystem"`).

Note: the role change (`"shell"` vs `"filesystem"`) means these tests will continue to fail until Plan 2 Task 1. That is fine — write the test with the target state. The Plan 1 test for Jump uses `"filesystem"` until the rename lands.

Revise the Jump test to use `"filesystem"` for now to let Plan 1 pass independently:

```go
func TestExecutor_Jump_EmitsNeutralDirectives(t *testing.T) {
    exec := openTestExecutor(t)
    ctx := context.Background()

    g, _ := exec.SC().CreateGalaxy(ctx, "acme")
    p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
    path := t.TempDir()
    r, _ := exec.SC().CreateResource(ctx, "root", "filesystem", "orbiter", "[]", `{"path":"`+path+`"}`)
    _, _ = exec.SC().Attach(ctx, r.Name, p.Name)

    directives, err := exec.Jump(ctx, "payment-api", true)
    require.NoError(t, err)
    require.NotEmpty(t, directives)

    assert.Equal(t, "DIR", directives[0].Op)
    assert.Equal(t, path, directives[0].Value)
    assert.Equal(t, "DIR "+path, directives[0].String())
    assert.NotContains(t, directives[0].String(), "cd ")
}
```

- [ ] **Step 3: Replace ShellDirective with Directive in executor.go**

Replace the `ShellDirective` block at lines 16-28 of `internal/commands/executor.go`:

```go
// Directive is a single neutral instruction for the shell wrapper to interpret.
// Op is one of "DIR" (change directory), "SET" (export env var), "UNSET" (unset env var).
// String() emits the line-protocol format: "OP KEY=VALUE" or "OP PATH".
type Directive struct {
    Op    string // "DIR", "SET", "UNSET"
    Key   string // SET/UNSET: variable name
    Value string // DIR: path, SET: variable value
}

func (d Directive) String() string {
    switch d.Op {
    case "DIR":
        return "DIR " + d.Value
    case "SET":
        return "SET " + d.Key + "=" + d.Value
    case "UNSET":
        return "UNSET " + d.Key
    }
    return ""
}
```

- [ ] **Step 4: Update Jump to use Directive and collect exports**

Replace the `Jump` method's directive-building section (lines 354-373) in `internal/commands/executor.go`:

```go
// Phase 4: build neutral directives.
var directives []Directive

for _, r := range calibResult.Resources {
    if r.Resource.Role != integrations.ResourceRoleFilesystem {
        continue
    }
    dir := r.After.InstallDir
    if dir == "" {
        dir = r.Before.InstallDir
    }
    if dir != "" {
        directives = append(directives, Directive{Op: "DIR", Value: dir})
        break
    }
}

// Collect shell exports from all calibrated resources (filtered by manifest allowlist).
for _, r := range calibResult.Resources {
    i, ok := e.sc.Integration(r.Resource.Role, r.Resource.Brand)
    if !ok {
        continue
    }
    allowed := make(map[string]bool, len(i.Meta().Shell.Exports))
    for _, k := range i.Meta().Shell.Exports {
        allowed[k] = true
    }
    after := r.After.Exports
    if after == nil {
        after = r.Before.Exports
    }
    for k, v := range after {
        if allowed[k] {
            directives = append(directives, Directive{Op: "SET", Key: k, Value: v})
        }
    }
}

return directives, nil
```

Note: this requires `e.sc.Integration(role, brand string) (integrations.Integration, bool)` to exist on StarChart. Check `internal/starchart/` for this method. If absent, add to `internal/starchart/starchart.go`:

```go
// Integration returns the registered integration for role+brand, or (nil, false).
func (sc *StarChart) Integration(role, brand string) (integrations.Integration, bool) {
    if sc.integrations == nil {
        return nil, false
    }
    return sc.integrations.Get(role, brand)
}
```

Also update the `Jump` return signature: `([]Directive, error)` — update the function signature and callers.

- [ ] **Step 5: Fix callers of Jump**

In `internal/commands/lifecycle.go`, find where `Jump` is called and update the `eval` output loop:

```go
directives, err := exec.Jump(ctx, target, yes)
if err != nil {
    return err
}
for _, d := range directives {
    fmt.Println(d.String())
}
```

- [ ] **Step 6: Run tests to verify they pass**

```
go test ./internal/commands/... -run "TestDirective|TestExecutor_Jump_EmitsNeutral" -v
```
Expected: PASS

- [ ] **Step 7: Update orbiter.bash to parse neutral protocol**

Replace `internal/commands/shell/orbiter.bash` entirely:

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
```

- [ ] **Step 8: Update orbiter.zsh to parse neutral protocol**

Replace `internal/commands/shell/orbiter.zsh` entirely:

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
```

- [ ] **Step 9: Update orbiter.fish to parse neutral protocol**

Replace `internal/commands/shell/orbiter.fish` entirely:

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
```

- [ ] **Step 10: Run full test suite**

```
go test ./...
```
Expected: all pass.

- [ ] **Step 11: Commit**

```bash
git add internal/commands/executor.go internal/commands/executor_test.go \
        internal/commands/shell/orbiter.bash \
        internal/commands/shell/orbiter.zsh \
        internal/commands/shell/orbiter.fish
git commit -m "feat: replace ShellDirective with neutral Directive protocol (DIR/SET/UNSET)"
```
