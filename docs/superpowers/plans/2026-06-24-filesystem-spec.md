# filesystem.go Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform `filesystem.go` from an integration into a pure utility module. Define a `Path` type that carries all OS-valid path representations so Orbiter and integrations can select the form they need. Handle the filesystem resource role natively in Go lifecycle code instead of through the integration dispatch.

**Architecture:**

`filesystem.go` becomes a utility-only file. It provides `ParsePath` and `PathFromConfig` as package-level functions — no struct, no `Integration` interface, no registry entry. The filesystem role is NOT removed from the role taxonomy: resources with role `filesystem` still exist in the StarChart and the executor still reads their `InstallDir` to emit `DIR` directives. What changes is that scan and calibrate for filesystem resources no longer go through integration dispatch — they are handled natively in `internal/starchart/lifecycle.go` directly. Filesystem behavior cannot be overridden by WASM integrations.

**Filesystem resolution is a lifecycle concern, not an integration concern.** Finding and locating files, binaries, and directories is the lifecycle's job during branch crawls — both for filesystem-role resources (path existence and drift detection) and for any integration that needs binary or file context (pre-populated into `ResolvedContext` before the WASM handler is called). WASM integrations never discover paths themselves; they receive fully resolved context from the lifecycle. The `populateBinaries` stub deleted from `loader.go` in Task 3 is a temporary compilation fix — its permanent home is `lifecycle.go`, where it will be implemented as part of the FIND spec.

`Path` persists only two forms to the StarChart: `Unix` (always forward-slash) and `Windows` (always backslash), both as exported fields. Four `Into*` methods provide a uniform calling convention across all forms: `IntoUnix()`, `IntoWindows()`, `IntoNative()` (OS-native at runtime), and `IntoShell()` (native, double-quoted when it contains a space). There is no "resolve after load" step — the methods compute correctly on demand. `ParsePath` accepts any of the four forms as input — it strips shell quotes, detects Windows absolute paths (drive letter), and normalizes separators before producing the two stored fields.

**Tech Stack:** Go stdlib only — `os`, `path/filepath`, `fmt`, `encoding/json`, `strings`. No new dependencies.

## Global Constraints

- `os/exec` must not appear in `filesystem.go` after this plan
- `filesystemIntegration` struct and its `init()` registry call must not exist after this plan
- `ParsePath` returns `Path{}, nil` for empty input — empty is not an error
- `ParsePath` accepts any of the four output forms as input: shell-quoted, Windows, Unix, or native
- `Path.Unix` always uses forward slashes; `Path.Windows` always uses backslashes; `Path.IntoNative()` returns the OS-native form at runtime
- `PathFromConfig` is the exported name; the existing private `pathFromConfig` is replaced
- `internal/wasm` must compile cleanly after Task 3
- All existing non-FindBinary tests must continue to pass after every task

---

## File Map

**Modified:**

- `internal/integrations/types.go` — add `Path` struct
- `internal/integrations/filesystem.go` — remove `filesystemIntegration` + `FindBinary` family; add `ParsePath`, `PathFromConfig`
- `internal/integrations/filesystem_test.go` — remove `TestFindBinary_*`; add `TestParsePath_*`, `TestPathFromConfig_*`
- `internal/starchart/lifecycle.go` — add native filesystem dispatch in `scanResource` and `calibrateResource`
- `internal/wasm/loader.go` — stub `populateBinaries` to no-op

**Deleted:**

- `internal/wasm/resolver.go` — thin wrapper around deleted `FindBinary`
- `internal/wasm/resolver_test.go` — tests only for deleted `ResolveBinaries`

---

### Task 1: Path type and ParsePath function

Define the shared path type and the constructor function. No deletions in this task — the codebase is fully additive and compiles cleanly throughout.

**Files:**

- Modify: `internal/integrations/types.go`
- Modify: `internal/integrations/filesystem.go`
- Modify: `internal/integrations/filesystem_test.go`

**Interfaces:**

- Produces: `Path{Unix, Windows string}` with `Native()` and `Shell()` methods in `internal/integrations/types.go`
- Produces: `ParsePath(raw string) (Path, error)` in `filesystem.go`
- Produces: `PathFromConfig(ctx ResolvedContext) Path` in `filesystem.go` (exported, replaces private `pathFromConfig`)

- [ ] **Step 1: Add Path to types.go**

Add to `internal/integrations/types.go` (after the existing `Platform` struct, before `DetectContext`). Add `"path/filepath"` and `"strings"` to the import block if not already present:

```go
// Path carries the Unix and Windows forms of a single absolute path.
// Both fields are exported for direct access and JSON serialization.
// Four Into* methods provide a uniform calling convention across all forms,
// including the two computed runtime forms (Native and Shell).
//
//   Unix      — exported field; always forward-slash; persisted in StarChart.
//   Windows   — exported field; always backslash; persisted in StarChart.
//   IntoUnix()    — method alias for Unix; use when a uniform call site matters.
//   IntoWindows() — method alias for Windows.
//   IntoNative()  — returns Windows on Windows, Unix everywhere else.
//   IntoShell()   — returns IntoNative() wrapped in double-quotes when it contains a space.
type Path struct {
    Unix    string `json:"unix"`    // persisted; always forward-slash
    Windows string `json:"windows"` // persisted; always backslash
}

func (p Path) IntoUnix() string    { return p.Unix }
func (p Path) IntoWindows() string { return p.Windows }

// IntoNative returns the OS-native separator form of the path.
func (p Path) IntoNative() string {
    if filepath.Separator == '\\' {
        return p.Windows
    }
    return p.Unix
}

// IntoShell returns the path for embedding in a shell command string.
// Wraps IntoNative() in double-quotes when it contains a space.
func (p Path) IntoShell() string {
    n := p.IntoNative()
    if strings.Contains(n, " ") {
        return `"` + n + `"`
    }
    return n
}
```

- [ ] **Step 2: Write the failing tests**

Replace `internal/integrations/filesystem_test.go` entirely:

```go
package integrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParsePath_Empty(t *testing.T) {
	got, err := ParsePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoUnix() != "" || got.IntoWindows() != "" || got.IntoNative() != "" || got.IntoShell() != "" {
		t.Errorf("expected zero Path for empty input, got %+v", got)
	}
}

func TestParsePath_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath("/usr/local/bin/brew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoNative() != "/usr/local/bin/brew" {
		t.Errorf("Native: expected /usr/local/bin/brew, got %q", got.IntoNative())
	}
	if got.IntoShell() != "/usr/local/bin/brew" {
		t.Errorf("Shell: expected /usr/local/bin/brew (no spaces), got %q", got.IntoShell())
	}
	if got.IntoUnix() != "/usr/local/bin/brew" {
		t.Errorf("Unix: expected /usr/local/bin/brew, got %q", got.IntoUnix())
	}
	if got.IntoWindows() != `\usr\local\bin\brew` {
		t.Errorf("Windows: expected \\usr\\local\\bin\\brew, got %q", got.IntoWindows())
	}
}

func TestParsePath_SpaceInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath("/home/captain/my projects/bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Shell must wrap in double-quotes when Native contains a space
	if got.IntoShell() != `"/home/captain/my projects/bin"` {
		t.Errorf("Shell: expected quoted path, got %q", got.IntoShell())
	}
	// Native and Unix are raw (unquoted)
	if got.IntoNative() != "/home/captain/my projects/bin" {
		t.Errorf("Native: expected unquoted path, got %q", got.IntoNative())
	}
}

func TestParsePath_ShellQuoted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath(`"/home/captain/my projects/bin"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Shell-quoted input must produce the same result as the unquoted form
	if got.IntoUnix() != "/home/captain/my projects/bin" {
		t.Errorf("Unix: expected unquoted path, got %q", got.IntoUnix())
	}
	if got.IntoShell() != `"/home/captain/my projects/bin"` {
		t.Errorf("Shell: expected re-quoted output, got %q", got.IntoShell())
	}
}

func TestParsePath_WindowsForm(t *testing.T) {
	got, err := ParsePath(`C:\Users\captain\bin`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoUnix() != "C:/Users/captain/bin" {
		t.Errorf("Unix: expected C:/Users/captain/bin, got %q", got.IntoUnix())
	}
	if got.IntoWindows() != `C:\Users\captain\bin` {
		t.Errorf("Windows: expected C:\\Users\\captain\\bin, got %q", got.IntoWindows())
	}
	// Shell must not wrap when no spaces present
	if strings.Contains(got.IntoShell(), `"`) {
		t.Errorf("Shell: expected no quotes for path without spaces, got %q", got.IntoShell())
	}
}

func TestParsePath_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := ParsePath("~/bin/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNative := filepath.Join(home, "bin", "foo")
	wantUnix := filepath.ToSlash(wantNative)
	wantWindows := strings.ReplaceAll(wantUnix, "/", `\`)
	if got.IntoNative() != wantNative {
		t.Errorf("Native: expected %q, got %q", wantNative, got.IntoNative())
	}
	if got.IntoUnix() != wantUnix {
		t.Errorf("Unix: expected %q, got %q", wantUnix, got.IntoUnix())
	}
	if got.IntoWindows() != wantWindows {
		t.Errorf("Windows: expected %q, got %q", wantWindows, got.IntoWindows())
	}
}

func TestParsePath_TildeAlone(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := ParsePath("~")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNative := home
	wantUnix := filepath.ToSlash(home)
	wantWindows := strings.ReplaceAll(wantUnix, "/", `\`)
	if got.IntoNative() != wantNative {
		t.Errorf("Native: expected %q, got %q", wantNative, got.IntoNative())
	}
	if got.IntoUnix() != wantUnix {
		t.Errorf("Unix: expected %q, got %q", wantUnix, got.IntoUnix())
	}
	if got.IntoWindows() != wantWindows {
		t.Errorf("Windows: expected %q, got %q", wantWindows, got.IntoWindows())
	}
}

func TestParsePath_WindowsNativeMethod(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows native-form test")
	}
	got, err := ParsePath(`C:\Users\captain\bin`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Native() must return backslash form on Windows
	if strings.Contains(got.IntoNative(), "/") {
		t.Errorf("Native() on Windows must not contain forward slashes, got %q", got.IntoNative())
	}
	// Shell must equal Native when no spaces present
	if got.IntoShell() != got.IntoNative() {
		t.Errorf("Shell: expected equal to Native when no spaces, got %q", got.IntoShell())
	}
	// Unix must use forward slashes
	if strings.Contains(got.IntoUnix(), `\`) {
		t.Errorf("Unix() must not contain backslashes, got %q", got.IntoUnix())
	}
}

func TestPathFromConfig_EmptyConfig(t *testing.T) {
	ctx := ResolvedContext{}
	got := PathFromConfig(ctx)
	if got.IntoUnix() != "" || got.IntoWindows() != "" {
		t.Errorf("expected empty Path for nil Self, got %+v", got)
	}
}
```

- [ ] **Step 3: Run tests to confirm they fail**

```
go test ./internal/integrations/ -run "TestParsePath|TestPathFromConfig" -v
```

Expected: `FAIL` — `ParsePath` and `PathFromConfig` are not defined yet.

- [ ] **Step 4: Rewrite filesystem.go**

Replace `internal/integrations/filesystem.go` with only the utility functions — no integration struct, no FindBinary, no exec:

```go
package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParsePath converts any path string into a Path carrying all separator forms.
// Accepts any of the four output forms as input: shell-quoted (strips double-quotes),
// Windows absolute (drive letter), Unix absolute (forward-slash), or native OS path.
// Also expands ~ and resolves relative paths to absolute.
// Returns a zero Path and nil error for empty input — empty signals "not found".
func ParsePath(raw string) (Path, error) {
	if raw == "" {
		return Path{}, nil
	}

	// Strip shell quoting — a Shell-form path may arrive wrapped in double-quotes.
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}

	// Tilde expansion.
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Path{}, fmt.Errorf("parse path: resolve home: %w", err)
		}
		if raw == "~" {
			raw = home
		} else {
			raw = home + raw[1:]
		}
	}

	// Windows absolute path: drive letter + colon (e.g. C:\ or C:/).
	// filepath.Abs on a non-Windows host treats this as a relative path and
	// prepends CWD, producing a wrong result. Handle it directly instead.
	if len(raw) >= 3 && raw[1] == ':' {
		c := raw[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return Path{
				Unix:    strings.ReplaceAll(raw, `\`, "/"),
				Windows: strings.ReplaceAll(raw, "/", `\`),
			}, nil
		}
	}

	// For all other forms (Unix absolute, relative, ~-expanded native):
	// filepath.Abs resolves relative paths; ToSlash and ReplaceAll fill both fields.
	abs, err := filepath.Abs(raw)
	if err != nil {
		return Path{}, fmt.Errorf("parse path: %w", err)
	}
	unix := filepath.ToSlash(abs)
	return Path{
		Unix:    unix,
		Windows: strings.ReplaceAll(unix, "/", `\`),
	}, nil
}

// PathFromConfig reads the "path" key from a resource's config JSON and
// returns the parsed form. Returns a zero Path on any failure.
func PathFromConfig(ctx ResolvedContext) Path {
	if ctx.Self == nil {
		return Path{}
	}
	var cfg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(ctx.Self.GetConfig()), &cfg); err != nil {
		return Path{}
	}
	p, err := ParsePath(cfg.Path)
	if err != nil {
		return Path{}
	}
	return p
}
```

- [ ] **Step 5: Run the tests**

```
go test ./internal/integrations/ -v
```

Expected: all `TestParsePath_*` and `TestPathFromConfig_*` pass. All other existing tests still pass. `TestFindBinary_*` still passes (FindBinary is still in the file — removal is Task 3).

Wait — `filesystem.go` is being replaced, which removes `filesystemIntegration` and `FindBinary`. This means `TestFindBinary_*` in `filesystem_test.go` will fail to compile. The test file references `FindBinary`.

Instead: this task only ADDS the new functions. Keep `filesystemIntegration` and `FindBinary` in place for now. Do this by appending to the existing `filesystem.go` rather than replacing it.

Revised approach for Step 4 — ADD to `filesystem.go` (do not replace):

Add `ParsePath` and `PathFromConfig` functions after line 72 (the existing private `pathFromConfig`). Imports: add `"fmt"` and `"path/filepath"` to the existing import block. Do not remove anything in this task.

```
go test ./internal/integrations/ -v
```

Expected: all new tests pass; all existing tests pass including `TestFindBinary_*`.

- [ ] **Step 6: Commit**

```
git add internal/integrations/types.go internal/integrations/filesystem.go internal/integrations/filesystem_test.go
git commit -m "feat: add Path type and ParsePath utility to filesystem"
```

---

### Task 2: Native filesystem dispatch in lifecycle; remove filesystemIntegration

Remove the integration struct and handle filesystem resources directly in starchart lifecycle code.

**Files:**

- Modify: `internal/integrations/filesystem.go` — remove `filesystemIntegration` struct, all its methods, and the `init()` registration call
- Modify: `internal/starchart/lifecycle.go` — add native filesystem handling in `scanResource` and `calibrateResource`

**Interfaces:**

- Consumes: `ParsePath`, `PathFromConfig` from Task 1
- Consumes: `os.Stat` directly (no integration indirection)
- `executor.go:416` still reads `r.After.InstallDir` — this is satisfied by the native handler populating `StateReport.InstallDir`

**Context on executor.go:** After calibration, `executor.go` reads the `InstallDir` from the filesystem-role resource's result and emits a `DIR` directive so the shell `cd`s to the project root. The native dispatch must populate `StateReport.InstallDir` from the resource config path.

**Context on lifecycle:** `scanResource` and `calibrateResource` currently call `Default.Get(r.Role, r.Brand)` which returns the registered integration. After removing the registration, this call returns `nil, false` for filesystem resources — the native branch must handle this before the integration lookup.

- [ ] **Step 1: Add native filesystem handling to lifecycle.go**

In `internal/starchart/lifecycle.go`, find `scanResource` (the function that handles a single resource). Add a native branch at the top, before the integration lookup:

```go
// Filesystem resources are handled natively — no WASM integration dispatch.
if r.Role == integrations.ResourceRoleFilesystem {
    return sc.scanFilesystemResource(ctx, r, lb)
}
```

Add the helper function to lifecycle.go:

```go
// scanFilesystemResource handles resources with the filesystem role natively.
// It normalizes the configured path and checks existence via os.Stat.
// A missing path signals drift — the recorded location no longer exists.
// Orbiter surfaces this via the beacon so the Captain can act.
func (sc *StarChart) scanFilesystemResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceScanResult, error) {
    resolved := sc.BuildResolvedContextFromBranch(r, lb)
    path := integrations.PathFromConfig(resolved)

    report := integrations.StateReport{InstallDir: path.IntoNative()}
    if path.IntoNative() != "" {
        if _, err := os.Stat(path.IntoNative()); err == nil {
            report.Present = true
        }
    }

    status := scanBeaconStatus(report)
    if err := sc.setBeaconStatus(ctx, r.ID, status); err != nil {
        return ResourceScanResult{}, err
    }
    return ResourceScanResult{
        Resource:     r,
        Report:       report,
        BeaconStatus: status,
    }, nil
}
```

In `calibrateResource`, add the same native branch before the integration lookup:

```go
if r.Role == integrations.ResourceRoleFilesystem {
    return sc.calibrateFilesystemResource(ctx, r, lb)
}
```

Add the calibrate helper:

```go
// calibrateFilesystemResource handles calibration for filesystem-role resources natively.
// Filesystem drift — a recorded path that no longer exists at its expected location —
// is Orbiter's concern: the resource was registered at a specific location and that
// location is now gone or moved. Orbiter detects this and surfaces it so the Captain
// can update the path or reinstall. Orbiter cannot auto-correct a moved path; only the
// Captain knows whether the directory moved, was deleted, or needs to be recreated.
func (sc *StarChart) calibrateFilesystemResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceCalibrateResult, error) {
    resolved := sc.BuildResolvedContextFromBranch(r, lb)
    path := integrations.PathFromConfig(resolved)

    before := integrations.StateReport{InstallDir: path.IntoNative()}
    if path.IntoNative() != "" {
        if _, err := os.Stat(path.IntoNative()); err == nil {
            before.Present = true
        }
    }

    // No corrective action is possible — report the current state as-is.
    // A missing path means drift; action reflects this for the Captain.
    status := scanBeaconStatus(before)
    if err := sc.setBeaconStatus(ctx, r.ID, status); err != nil {
        return ResourceCalibrateResult{}, err
    }
    action := "healthy"
    if !before.Present {
        action = "drifted"
    }
    return ResourceCalibrateResult{
        Resource: r,
        Before:   before,
        After:    before,
        Action:   action,
    }, nil
}
```

- [ ] **Step 2: Run tests to confirm lifecycle still works**

```
go test ./internal/starchart/ -v
```

Expected: all starchart tests pass. The native filesystem handlers are additive at this point — the integration dispatch still runs for filesystem resources too (the native branch short-circuits first, so the integration is never reached, but it still compiles).

- [ ] **Step 3: Remove filesystemIntegration from filesystem.go**

Delete from `internal/integrations/filesystem.go`:
- The `filesystemIntegration` struct (lines 10-15 in current file)
- `(f *filesystemIntegration) Meta()` method
- `(f *filesystemIntegration) Detect()` method
- `(f *filesystemIntegration) Init()` method
- `(f *filesystemIntegration) Scan()` method
- `(f *filesystemIntegration) Calibrate()` method
- The `init()` function that calls `Default.Register`
- The existing private `pathFromConfig` function (replaced by `PathFromConfig` from Task 1)

After deletion, `filesystem.go` contains only `ParsePath` and `PathFromConfig`.

- [ ] **Step 4: Build and test**

```
go build ./...
go test ./...
```

Expected: clean build and all tests pass. The native lifecycle handlers in lifecycle.go take over for filesystem resources. `executor.go:416` continues to read `r.After.InstallDir` from the native handler's output.

- [ ] **Step 5: Commit**

```
git add internal/integrations/filesystem.go internal/starchart/lifecycle.go
git commit -m "refactor: make filesystem role native Go; remove filesystemIntegration"
```

---

### Task 3: Delete FindBinary family and stub populateBinaries

Delete all shell-spawning code from `filesystem.go` and the resolver wrapper from `internal/wasm`. This task is an atomic deletion — `resolver.go` deletion breaks `loader.go` compilation until `populateBinaries` is stubbed in the same pass.

**Files:**

- Modify: `internal/integrations/filesystem.go` — remove `FindBinary`, `findBinaryUnix`, `findBinaryWindows`, `runShellFind`; remove `os/exec` import
- Modify: `internal/integrations/filesystem_test.go` — remove `TestFindBinary_*`; remove `runtime` import
- Delete: `internal/wasm/resolver.go`
- Delete: `internal/wasm/resolver_test.go`
- Modify: `internal/wasm/loader.go` — stub `populateBinaries`

- [ ] **Step 1: Remove FindBinary family from filesystem.go**

Delete from `internal/integrations/filesystem.go`:
- `FindBinary` function and its block comment
- `findBinaryUnix` function
- `findBinaryWindows` function
- `runShellFind` function

Update the import block — remove `"os/exec"`. The `"strings"` import is still needed for `ParsePath`.

Final import block:

```go
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)
```

- [ ] **Step 2: Remove TestFindBinary_* from filesystem_test.go**

Delete `TestFindBinary_KnownBinary` and `TestFindBinary_UnknownBinary` from `internal/integrations/filesystem_test.go`.

Remove the `"runtime"` import (it was only used by `TestFindBinary_KnownBinary`).

- [ ] **Step 3: Delete resolver.go and resolver_test.go**

```
rm internal/wasm/resolver.go
rm internal/wasm/resolver_test.go
```

At this point `internal/wasm` does not compile — `loader.go:125` still calls `ResolveBinaries`.

- [ ] **Step 4: Stub populateBinaries in loader.go**

Replace the `populateBinaries` method in `internal/wasm/loader.go`:

```go
// populateBinaries is a temporary compilation stub. The real implementation
// belongs in lifecycle.go — filesystem resolution (binaries, files, dirs, envs)
// is a lifecycle concern resolved during branch crawls before WASM dispatch,
// not a WASM integration concern. This stub will be deleted when the FIND spec
// is implemented and lifecycle.go takes over context population.
func (w *WASMIntegration) populateBinaries(ctx integrations.ResolvedContext) integrations.ResolvedContext {
	return ctx
}
```

- [ ] **Step 5: Build and test**

```
go build ./...
go test ./...
```

Expected: clean build; all tests pass. No reference to `FindBinary`, `ResolveBinaries`, or `os/exec` in `internal/integrations`.

- [ ] **Step 6: Commit**

```
git add internal/integrations/filesystem.go internal/integrations/filesystem_test.go internal/wasm/loader.go
git rm internal/wasm/resolver.go internal/wasm/resolver_test.go
git commit -m "refactor: delete FindBinary shell-spawning code; stub populateBinaries pending FIND transport"
```
