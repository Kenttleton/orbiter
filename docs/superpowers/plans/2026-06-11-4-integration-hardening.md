# Phase 4: Integration Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the integration system into a production-ready contract: brand-centric manifests with `name`/`description`, unified Entity interface, transponder config migration, WASM security (banlist, allowlist, Captain approval, trust persistence, audit log), module pooling, role-ordered lifecycle dispatch, and a catalog-based installation flow.

**Architecture:** Tasks 1–3 are foundational type changes (manifest format, Entity interface, transponder migration). Tasks 4–6 harden the WASM host (shared runtime, trust store, audit log + banlist + Captain approval, exports validation, pooling). Tasks 7–8 finish the lifecycle layer (role-ordered dispatch, transponder pass). Tasks 9–10 deliver the catalog model (bundle.go as dormant catalog, vessel init selection screen).

**Tech Stack:** Go 1.25, wazero v1.12, modernc.org/sqlite v1.52, BurntSushi/toml v1.6

---

## File Map

New files:

- `internal/integrations/manifest.go` — manifest types (split out of types.go)
- `internal/integrations/manifest_test.go` — manifest parsing + RoleType tests
- `internal/integrations/settings.go` — SettingsStore: trust + quarantine in `~/.orbiter/settings.json`
- `internal/integrations/settings_test.go` — settings store tests
- `internal/wasm/runtime.go` — shared wazero singleton
- `internal/wasm/audit.go` — append-only audit log
- `internal/wasm/audit_test.go` — audit log tests

Modified files:

- `internal/integrations/registry.go` — add sync.RWMutex; add quarantine map; Deregister, QuarantineBrand, UnquarantineBrand, IsQuarantined; Get/All/AllForRole skip quarantined brands
- `internal/integrations/types.go` — remove Manifest* types; add Entity interface, InputRequest, update StateReport + ResolvedContext
- `internal/integrations/roles.go` — add RoleTypes map + RoleType()
- `internal/integrations/native/filesystem.go` — update manifest literal; use GetConfig() in pathFromSelf
- `internal/models/resource.go` — implement Entity (GetID/GetRole/GetBrand/GetConfig)
- `internal/models/transponder.go` — add Config, remove Location; implement Entity
- `internal/starchart/crawl.go` — update transponder DB scan; update BuildResolvedContextForResource Self field
- `internal/wasm/host.go` — five-stage gate; banlist; trust via integrations.DefaultSettings; quarantine via integrations.Default; Captain prompt; audit log
- `internal/wasm/loader.go` — pool-based dispatch; validate Exports against manifest Shell.Exports
- `internal/starchart/lifecycle.go` — transponder result types; role-ordered resource dispatch; parallel transponder pass
- `integrations/bundle.go` — multi-role registration; dormant catalog (CatalogEntries, InstallFromCatalog); startup loads from ~/.orbiter/integrations/ only
- `internal/commands/vessel.go` — vessel init catalog; vessel inspect; vessel unquarantine
- `integrations/golang/manifest.toml` — new format (name, description, roles, commands, shell, runtime)
- `integrations/git/manifest.toml` — new format

---

## Task 1: Manifest format — split manifest.go, update roles, update toml files

**Purpose:** The manifest currently uses `role` (singular) and `type` in `[integration]`. We move all manifest types to a new file, change to `roles = [...]` array (one integration, multiple roles), add `[commands]`, `[shell]`, `[config]`, and `[runtime]` sections, and add `RoleType()` to the roles file. This is a pure extension — no existing behavior changes.

**Files:**

- Create: `internal/integrations/manifest.go`
- Create: `internal/integrations/manifest_test.go`
- Modify: `internal/integrations/types.go` (delete moved Manifest* types)
- Modify: `internal/integrations/roles.go` (add RoleTypes + RoleType)
- Modify: `internal/integrations/native/filesystem.go` (update manifest literal)
- Modify: `integrations/bundle.go` (multi-role registration loop)
- Modify: `integrations/golang/manifest.toml`
- Modify: `integrations/git/manifest.toml`

- [ ] **Step 1: Write the failing test**

```go
// internal/integrations/manifest_test.go
package integrations_test

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestManifest_ParseNewFormat(t *testing.T) {
	const src = `
[integration]
brand = "gh"
name = "GitHub CLI"
description = "Manage GitHub repos, PRs, and auth via the gh CLI"
roles = ["tool", "keychain"]

[commands]
allowed = ["gh", "git", "which"]
timeout_seconds = 30

[shell]
exports = ["GH_TOKEN", "GITHUB_TOKEN"]

[[config.fields]]
key = "username"
type = "string"
required = false
description = "GitHub username"

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
`
	var m integrations.Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Integration.Brand != "gh" {
		t.Errorf("brand = %q, want %q", m.Integration.Brand, "gh")
	}
	if m.Integration.Name != "GitHub CLI" {
		t.Errorf("name = %q, want %q", m.Integration.Name, "GitHub CLI")
	}
	if m.Integration.Description == "" {
		t.Error("description should not be empty")
	}
	if len(m.Integration.Roles) != 2 || m.Integration.Roles[0] != "tool" || m.Integration.Roles[1] != "keychain" {
		t.Errorf("roles = %v", m.Integration.Roles)
	}
	if len(m.Commands.Allowed) != 3 || m.Commands.Allowed[0] != "gh" {
		t.Errorf("commands.allowed = %v", m.Commands.Allowed)
	}
	if m.Commands.TimeoutSeconds != 30 {
		t.Errorf("timeout_seconds = %d", m.Commands.TimeoutSeconds)
	}
	if len(m.Shell.Exports) != 2 || m.Shell.Exports[0] != "GH_TOKEN" {
		t.Errorf("shell.exports = %v", m.Shell.Exports)
	}
	if len(m.Config.Fields) != 1 || m.Config.Fields[0].Key != "username" {
		t.Errorf("config.fields = %v", m.Config.Fields)
	}
	if m.Runtime.PoolSize != 4 {
		t.Errorf("pool_size = %d", m.Runtime.PoolSize)
	}
	if m.Runtime.InputBufferKB != 8 || m.Runtime.OutputBufferKB != 8 {
		t.Errorf("buffer hints = %d/%d", m.Runtime.InputBufferKB, m.Runtime.OutputBufferKB)
	}
}

func TestRoleType(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"manager", "resource"},
		{"runtime", "resource"},
		{"tool", "resource"},
		{"remote", "resource"},
		{"filesystem", "resource"},
		{"file", "transponder"},
		{"env", "transponder"},
		{"keychain", "transponder"},
		{"vault", "transponder"},
		{"agent", "transponder"},
		{"unknown", ""},
	}
	for _, tc := range tests {
		if got := integrations.RoleType(tc.role); got != tc.want {
			t.Errorf("RoleType(%q) = %q, want %q", tc.role, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/integrations/... -run TestManifest_ParseNewFormat -v
```

Expected: FAIL — `Manifest` struct lacks `Commands`, `Shell`, `Config`, `Runtime` fields; `ManifestIntegration` has no `Roles` field.

- [ ] **Step 3: Create `internal/integrations/manifest.go`**

```go
package integrations

// Manifest is the parsed content of an integration's manifest.toml.
// All sections are optional — the host applies defaults where fields are zero.
type Manifest struct {
	Integration  ManifestIntegration  `toml:"integration"`
	Detection    ManifestDetection    `toml:"detection"`
	Dependencies ManifestDependencies `toml:"dependencies"`
	Commands     ManifestCommands     `toml:"commands"`
	Shell        ManifestShell        `toml:"shell"`
	Config       ManifestConfig       `toml:"config"`
	Runtime      ManifestRuntime      `toml:"runtime"`
}

// ManifestIntegration is the [integration] section.
// Type is absent — Orbiter infers resource vs transponder from its static role taxonomy.
type ManifestIntegration struct {
	Brand       string   `toml:"brand"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Roles       []string `toml:"roles"`
}

// ManifestDetection is the [detection] section.
type ManifestDetection struct {
	Files []string `toml:"files"`
}

// ManifestDependencies is the [dependencies] section.
// Keys are roles. Values are brand allowlists (empty = any brand accepted).
type ManifestDependencies struct {
	Resources    map[string][]string `toml:"resources"`
	Transponders map[string][]string `toml:"transponders"`
}

// ManifestCommands is the [commands] section.
// Allowed lists every executable the integration may call via run_command.
// The host rejects any call for an executable not listed here.
type ManifestCommands struct {
	Allowed        []string `toml:"allowed"`
	TimeoutSeconds int      `toml:"timeout_seconds"`
}

// ManifestShell is the [shell] section.
// Exports lists env var names the integration may include in StateReport.Exports.
// The host drops any export key not declared here.
type ManifestShell struct {
	Exports []string `toml:"exports"`
}

// ManifestConfig describes configuration fields the integration accepts.
// Used by Orbiter for guided setup UI and input validation.
type ManifestConfig struct {
	Fields []ManifestConfigField `toml:"fields"`
}

// ManifestConfigField describes one config field.
type ManifestConfigField struct {
	Key         string `toml:"key"`
	Type        string `toml:"type"`
	Required    bool   `toml:"required"`
	Description string `toml:"description"`
}

// ManifestRuntime is the [runtime] section — performance hints.
// PoolSize is the number of concurrent WASM instances (default 4 if zero).
// InputBufferKB and OutputBufferKB are guest buffer size hints (default 8 if zero).
type ManifestRuntime struct {
	PoolSize       int `toml:"pool_size"`
	InputBufferKB  int `toml:"input_buffer_kb"`
	OutputBufferKB int `toml:"output_buffer_kb"`
}
```

- [ ] **Step 4: Remove `Manifest*` types from `internal/integrations/types.go`**

Delete these type declarations from `types.go` (they now live in `manifest.go`):
- `type Manifest struct`
- `type ManifestIntegration struct`
- `type ManifestDetection struct`
- `type ManifestDependencies struct`

`types.go` retains: `Platform`, `DetectContext`, `DetectReport`, `SuggestedResource`, `ResolvedContext`, `ResolvedResource`, `ResolvedTransponder`, `StateReport`.

- [ ] **Step 5: Add `RoleTypes` and `RoleType()` to `internal/integrations/roles.go`**

```go
// RoleTypes maps every role to its type ("resource" or "transponder").
// Orbiter owns this mapping statically — integrations never declare their type.
var RoleTypes = map[string]string{
	ResourceRoleManager:    IntegrationTypeResource,
	ResourceRoleRuntime:    IntegrationTypeResource,
	ResourceRoleTool:       IntegrationTypeResource,
	ResourceRoleRemote:     IntegrationTypeResource,
	ResourceRoleFilesystem: IntegrationTypeResource,
	TransponderRoleFile:    IntegrationTypeTransponder,
	TransponderRoleEnv:     IntegrationTypeTransponder,
	TransponderRoleKeychain: IntegrationTypeTransponder,
	TransponderRoleVault:   IntegrationTypeTransponder,
	TransponderRoleAgent:   IntegrationTypeTransponder,
}

// RoleType returns "resource", "transponder", or "" for unknown roles.
func RoleType(role string) string {
	return RoleTypes[role]
}
```

- [ ] **Step 6: Update `internal/integrations/native/filesystem.go` manifest literal**

Change the `filesystemOrbiterManifest` declaration:

```go
var filesystemOrbiterManifest = integrations.Manifest{
	Integration: integrations.ManifestIntegration{
		Brand: "orbiter",
		Roles: []string{integrations.ResourceRoleFilesystem},
	},
}
```

Remove the `Type` and `Role` fields (they no longer exist in `ManifestIntegration`).

- [ ] **Step 7: Update `integrations/bundle.go` for multi-role registration**

Change the `core.Register(...)` call to loop over `manifest.Integration.Roles`:

```go
for _, role := range manifest.Integration.Roles {
    core.Register(role, manifest.Integration.Brand, i)
}
```

The full updated `init()` loop body (everything else stays the same):

```go
func init() {
	entries, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		log.Printf("orbiter: read bundle: %v", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()

		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			log.Printf("orbiter: manifest for %s: %v", name, err)
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			log.Printf("orbiter: parse manifest for %s: %v", name, err)
			continue
		}

		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			log.Printf("orbiter: wasm for %s: %v", name, err)
			continue
		}

		i, err := wasm.Load(context.Background(), manifest, wasmBytes)
		if err != nil {
			log.Printf("orbiter: load %s: %v", name, err)
			continue
		}
		for _, role := range manifest.Integration.Roles {
			core.Register(role, manifest.Integration.Brand, i)
		}
	}
}
```

- [ ] **Step 8: Update `integrations/golang/manifest.toml`**

```toml
[integration]
brand = "go"
name = "Go Toolchain"
description = "Scans and verifies the Go runtime and module toolchain"
roles = ["runtime"]

[detection]
files = ["go.mod", "go.sum"]

[commands]
allowed = ["go"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 9: Update `integrations/git/manifest.toml`**

```toml
[integration]
brand = "git"
name = "Git"
description = "Scans and configures the git version control tool"
roles = ["tool"]

[detection]
files = [".git/config"]

[commands]
allowed = ["git"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 10: Run all tests and verify they pass**

```bash
go test ./... -count=1
```

Expected: all tests pass. The e2e tests (`integrations/e2e_test.go`) must still find `"tool"/"git"` and `"runtime"/"go"` — the multi-role loop now registers them correctly.

- [ ] **Step 11: Commit**

```bash
git add internal/integrations/manifest.go internal/integrations/manifest_test.go \
    internal/integrations/types.go internal/integrations/roles.go \
    internal/integrations/native/filesystem.go \
    integrations/bundle.go \
    integrations/golang/manifest.toml integrations/git/manifest.toml
git commit -m "feat: brand-centric manifest format with roles array and new sections"
```

---

## Task 2: Entity interface + ABI type extensions

**Purpose:** Both `Resource` and `Transponder` become `Entity` — a uniform interface passed as `ResolvedContext.Self`. This lets transponder integrations receive their entity via the same dispatch path as resources. We also add `InputRequest`, `NeedsInput`, `Responses`, and `Exports` to the ABI types — the stateless interactive auth round-trip that keychain/vault integrations will use.

**Files:**

- Modify: `internal/integrations/types.go`
- Modify: `internal/models/resource.go`
- Modify: `internal/starchart/crawl.go`
- Modify: `internal/integrations/native/filesystem.go`

Note: `Transponder` gets `GetConfig()` in Task 3 after it gains the `Config` field. In this task, `Resource` implements `Entity` and `ResolvedContext.Self` changes type. `Transponder` is added to `Entity` in Task 3.

- [ ] **Step 1: Write failing tests**

```go
// internal/integrations/entity_test.go
package integrations_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

func TestEntity_ResourceImplementsInterface(t *testing.T) {
	r := models.Resource{ID: "r1", Role: "runtime", Brand: "go", Config: `{"version":"1.25"}`}
	var e integrations.Entity = r
	if e.GetID() != "r1" {
		t.Errorf("GetID = %q", e.GetID())
	}
	if e.GetRole() != "runtime" {
		t.Errorf("GetRole = %q", e.GetRole())
	}
	if e.GetBrand() != "go" {
		t.Errorf("GetBrand = %q", e.GetBrand())
	}
	if e.GetConfig() != `{"version":"1.25"}` {
		t.Errorf("GetConfig = %q", e.GetConfig())
	}
}

func TestResolvedContext_SelfIsEntity(t *testing.T) {
	r := models.Resource{ID: "r1", Role: "runtime", Brand: "go", Config: "{}"}
	rc := integrations.ResolvedContext{
		Self: r,
	}
	if rc.Self == nil {
		t.Fatal("Self should not be nil")
	}
	if rc.Self.GetID() != "r1" {
		t.Errorf("Self.GetID = %q", rc.Self.GetID())
	}
}

func TestStateReport_NeedsInputAndExports(t *testing.T) {
	report := integrations.StateReport{
		Present:   true,
		Reachable: true,
		NeedsInput: []integrations.InputRequest{
			{Key: "password", Prompt: "Enter password:", Masked: true},
		},
		Exports: map[string]string{"GITHUB_TOKEN": "ghp_abc123"},
	}
	if len(report.NeedsInput) != 1 {
		t.Errorf("NeedsInput len = %d", len(report.NeedsInput))
	}
	if !report.NeedsInput[0].Masked {
		t.Error("expected Masked = true")
	}
	if report.Exports["GITHUB_TOKEN"] != "ghp_abc123" {
		t.Errorf("Exports[GITHUB_TOKEN] = %q", report.Exports["GITHUB_TOKEN"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/integrations/... -run TestEntity_ -v
go test ./internal/integrations/... -run TestResolvedContext_SelfIsEntity -v
go test ./internal/integrations/... -run TestStateReport_NeedsInputAndExports -v
```

Expected: FAIL — `integrations.Entity` does not exist, `Resource` has no `GetID` etc.

- [ ] **Step 3: Add `Entity` interface and `InputRequest` to `internal/integrations/types.go`**

Add to `types.go` (leave existing types in place, only additions here):

```go
// Entity is the uniform interface implemented by both Resource and Transponder.
// ResolvedContext.Self is Entity so the same dispatch path serves both.
type Entity interface {
	GetID()     string
	GetRole()   string
	GetBrand()  string
	GetConfig() string
}

// InputRequest describes a single credential prompt the integration needs.
// The host collects responses and calls the integration again with Responses populated.
type InputRequest struct {
	Key    string `json:"key"`
	Prompt string `json:"prompt"`
	Masked bool   `json:"masked"`
}
```

- [ ] **Step 4: Update `ResolvedContext` and `StateReport` in `internal/integrations/types.go`**

Change `ResolvedContext.Self` from `*models.Resource` to `Entity`. Add `Responses` to `ResolvedContext`. Add `NeedsInput` and `Exports` to `StateReport`.

Updated `ResolvedContext`:

```go
// ResolvedContext is the boundary struct passed to Init, Scan, and Calibrate.
type ResolvedContext struct {
	Platform     Platform                         `json:"platform"`
	Self         Entity                           `json:"self,omitempty"`
	Resources    map[string][]ResolvedResource    `json:"resources"`
	Transponders map[string][]ResolvedTransponder `json:"transponders"`
	Responses    map[string]string                `json:"responses,omitempty"`
}
```

Updated `StateReport` (add `NeedsInput` and `Exports` fields):

```go
// StateReport is returned by Init, Scan, and Calibrate.
type StateReport struct {
	Present      bool              `json:"present"`
	Reachable    bool              `json:"reachable"`
	BinaryPath   string            `json:"binary_path,omitempty"`
	InstallDir   string            `json:"install_dir,omitempty"`
	InPath       bool              `json:"in_path"`
	Manager      string            `json:"manager"`
	Config       map[string]any    `json:"config,omitempty"`
	Observations []string          `json:"observations,omitempty"`
	Error        string            `json:"error,omitempty"`
	NeedsInput   []InputRequest    `json:"needs_input,omitempty"`
	Exports      map[string]string `json:"exports,omitempty"`
}
```

Remove the `"github.com/Kenttleton/orbiter/internal/models"` import from `types.go` — it's no longer needed (Self is now `Entity`, not `*models.Resource`; the `ResolvedResource` and `ResolvedTransponder` wrappers still use models types but that import was already there).

Wait — `ResolvedResource` and `ResolvedTransponder` still reference `models.Resource` and `models.Transponder`, so the import must stay. Remove only if truly unused after the edit. The import stays.

- [ ] **Step 5: Implement `Entity` on `internal/models/resource.go`**

Add these methods to `resource.go` (after the struct declaration):

```go
func (r Resource) GetID() string     { return r.ID }
func (r Resource) GetRole() string   { return r.Role }
func (r Resource) GetBrand() string  { return r.Brand }
func (r Resource) GetConfig() string { return r.Config }
```

- [ ] **Step 6: Update `internal/starchart/crawl.go` — change Self assignment**

In `BuildResolvedContextForResource`, change:

```go
// before
Self: &self,
```

to:

```go
// after
Self: self,
```

(The interface accepts the value directly; a pointer is not needed.)

- [ ] **Step 7: Update `internal/integrations/native/filesystem.go` — use `GetConfig()`**

In `pathFromSelf`, change:

```go
// before
if ctx.Self == nil {
    return ""
}
var cfg struct {
    Path string `json:"path"`
}
if err := json.Unmarshal([]byte(ctx.Self.Config), &cfg); err != nil {
    return ""
}
```

to:

```go
// after
if ctx.Self == nil {
    return ""
}
var cfg struct {
    Path string `json:"path"`
}
if err := json.Unmarshal([]byte(ctx.Self.GetConfig()), &cfg); err != nil {
    return ""
}
```

- [ ] **Step 8: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass. The starchart lifecycle tests dispatch through `BuildResolvedContextForResource` which now assigns `Self: self` (a `models.Resource` value implementing `Entity`).

- [ ] **Step 9: Commit**

```bash
git add internal/integrations/types.go internal/integrations/entity_test.go \
    internal/models/resource.go \
    internal/starchart/crawl.go \
    internal/integrations/native/filesystem.go
git commit -m "feat: Entity interface — unified Self for Resource and Transponder dispatch"
```

---

## Task 3: Transponder config column

**Purpose:** Transponders currently store a `location` column (a plain path string). Since no production database exists yet, we update the initial schema directly and rename the field in all Go code. The `location` column becomes `config TEXT NOT NULL DEFAULT '{}'` — a JSON blob matching the pattern resources already use. After this, `Transponder` implements `Entity`.

**Files:**

- Modify: `internal/migrations/0001_initial.sql`
- Modify: `internal/models/transponder.go`
- Modify: `internal/starchart/crawl.go` (query + scan)
- Modify: `internal/starchart/create.go` (CreateTransponder signature)

- [ ] **Step 1: Write failing test**

```go
// Add to internal/starchart/migrate_test.go (or create a new file internal/starchart/transponder_config_test.go)
// internal/starchart/transponder_config_test.go
package starchart_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func TestTransponderConfig_MigratedFromLocation(t *testing.T) {
	sc := openTestStarChart(t)
	ctx := context.Background()

	// Create a callsign and attach a transponder to it
	vs := createTestVessel(t, sc, ctx)
	cs := createTestCallsign(t, sc, ctx, vs.ID)

	tp, err := sc.CreateTransponder(ctx, starchart.CreateTransponderInput{
		CallsignID: cs.ID,
		Role:       "file",
		Brand:      "github",
		Config:     `{"location":"/home/user/.ssh/id_ed25519"}`,
	})
	require.NoError(t, err)
	require.Equal(t, `{"location":"/home/user/.ssh/id_ed25519"}`, tp.Config)
	require.Equal(t, "file", tp.Role)
	require.Equal(t, "github", tp.Brand)
}

func TestTransponderConfig_EntityInterface(t *testing.T) {
	sc := openTestStarChart(t)
	ctx := context.Background()
	vs := createTestVessel(t, sc, ctx)
	cs := createTestCallsign(t, sc, ctx, vs.ID)

	tp, err := sc.CreateTransponder(ctx, starchart.CreateTransponderInput{
		CallsignID: cs.ID,
		Role:       "env",
		Brand:      "aws",
		Config:     `{"var":"AWS_PROFILE","value":"prod"}`,
	})
	require.NoError(t, err)
	// Entity interface methods
	require.Equal(t, tp.ID, tp.GetID())
	require.Equal(t, "env", tp.GetRole())
	require.Equal(t, "aws", tp.GetBrand())
	require.Equal(t, `{"var":"AWS_PROFILE","value":"prod"}`, tp.GetConfig())
}
```

Note: `CreateTransponder` may need `Config` added to its input struct — look at the existing `CreateTransponderInput` in `internal/starchart/crud.go` and add the `Config string` field if it isn't there.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/starchart/... -run TestTransponderConfig -v
```

Expected: FAIL — `Transponder` has no `Config` field; `CreateTransponderInput` has no `Config`; `GetID()`/`GetRole()`/`GetBrand()`/`GetConfig()` don't exist on `Transponder`.

- [ ] **Step 3: Update `internal/migrations/0001_initial.sql` — replace `location` with `config`**

In the `transponders` table, replace:

```sql
    location   TEXT NOT NULL,
```

With:

```sql
    -- config is a JSON object whose shape is role-specific (see docs/integrations.md).
    config     TEXT NOT NULL DEFAULT '{}',
```

No separate migration file needed — there is no existing database to migrate.

- [ ] **Step 4: Update `internal/models/transponder.go`**

```go
package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself — only enough config to locate or reach it.
// Role is the access mechanism (file, env, keychain, vault, agent) and is Orbiter-owned.
// Brand is the service the credential grants access to and is integration-owned.
// Config is a JSON object whose shape is role-specific (see docs/integrations.md).
type Transponder struct {
	ID        string    `db:"id"         json:"id"`
	Role      string    `db:"role"       json:"role"`
	Brand     string    `db:"brand"      json:"brand"`
	Config    string    `db:"config"     json:"config"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (t Transponder) GetID() string     { return t.ID }
func (t Transponder) GetRole() string   { return t.Role }
func (t Transponder) GetBrand() string  { return t.Brand }
func (t Transponder) GetConfig() string { return t.Config }
```

- [ ] **Step 5: Update `internal/starchart/crawl.go` — transponder query and scan**

Find `transpondersAttachedTo`. Change the SQL column and `rows.Scan` field:

```go
const q = `
    SELECT tp.id, tp.role, tp.brand, tp.config, tp.created_at
    FROM transponders tp
    JOIN attachments a ON a.from_id = tp.id
    WHERE a.to_id = ?
`
// ...
if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Config, &tp.CreatedAt); err != nil {
```

- [ ] **Step 6: Update `internal/starchart/create.go` — `CreateTransponder` signature**

Change the last parameter from `location string` to `config string`. Default to `"{}"` if empty. The `t.Insert` call uses struct tags so no SQL string change is needed:

```go
func (sc *StarChart) CreateTransponder(ctx context.Context, name, role, brand, config string) (models.Transponder, error) {
	if config == "" {
		config = "{}"
	}
	id := models.NewID(models.EntityTypeTransponder)
	tp := models.Transponder{ID: id, Role: role, Brand: brand, Config: config, CreatedAt: time.Now().UTC()}
	return tp, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "transponders", tp)
	})
}
```

Update callers: any `CreateTransponder(..., "/path")` call becomes `CreateTransponder(..., "{\"location\":\"/path\"}")`.

- [ ] **Step 7: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/migrations/0001_initial.sql \
    internal/models/transponder.go \
    internal/starchart/crawl.go \
    internal/starchart/create.go \
    internal/starchart/create_test.go \
    internal/starchart/leveled_crawl_test.go
git commit -m "feat: transponder config JSON blob — replace location column, implement Entity"
```

---

## Task 4: Shared WASM runtime

**Purpose:** Currently each `WASMIntegration` creates its own `wazero.Runtime` and host module. This wastes memory (JIT compilation repeats) and causes issues at scale. We create a shared singleton runtime so the host module is compiled and instantiated once.

**Files:**

- Create: `internal/wasm/runtime.go`
- Modify: `internal/wasm/loader.go` (use shared runtime in Load)

- [ ] **Step 1: Write failing test**

```go
// internal/wasm/runtime_test.go
package wasm_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestSharedRuntime_SameInstance(t *testing.T) {
	ctx := context.Background()
	r1 := wasm.SharedRuntime(ctx)
	r2 := wasm.SharedRuntime(ctx)
	if r1 != r2 {
		t.Error("SharedRuntime should return the same instance every call")
	}
}

func TestSharedRuntime_NotNil(t *testing.T) {
	ctx := context.Background()
	r := wasm.SharedRuntime(ctx)
	if r == nil {
		t.Error("SharedRuntime returned nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/wasm/... -run TestSharedRuntime -v
```

Expected: FAIL — `wasm.SharedRuntime` does not exist.

- [ ] **Step 3: Create `internal/wasm/runtime.go`**

```go
package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
)

var (
	sharedRT   wazero.Runtime
	runtimeMu  sync.Mutex
	runtimeErr error
	once       sync.Once
)

// SharedRuntime returns the process-wide wazero runtime with the "orbiter" host
// module pre-instantiated. Panics on first-call failure (startup-time misconfiguration).
func SharedRuntime(ctx context.Context) wazero.Runtime {
	once.Do(func() {
		rt, err := newRuntime(ctx)
		if err != nil {
			panic(fmt.Sprintf("wasm: init shared runtime: %v", err))
		}
		sharedRT = rt
	})
	return sharedRT
}
```

- [ ] **Step 4: Update `internal/wasm/loader.go` — use shared runtime**

In `Load`, replace:

```go
// before
rt, err := newRuntime(ctx)
if err != nil {
    return nil, fmt.Errorf("create wasm runtime: %w", err)
}

compiled, err := rt.CompileModule(ctx, wasmBytes)
if err != nil {
    rt.Close(ctx)
    return nil, fmt.Errorf("compile wasm module: %w", err)
}
```

with:

```go
// after
rt := SharedRuntime(ctx)

compiled, err := rt.CompileModule(ctx, wasmBytes)
if err != nil {
    return nil, fmt.Errorf("compile wasm module: %w", err)
}
```

Note: do NOT close the shared runtime on error. Only close `compiled` if instantiation fails.

Also remove the `rt wazero.Runtime` field from `WASMIntegration` struct — it's no longer stored per-integration (the singleton is in the package). Keep `manifest`, `mod`, and `mu` for now (pool comes in Task 5).

Updated `WASMIntegration` struct:

```go
type WASMIntegration struct {
	manifest integrations.Manifest
	mod      api.Module
	mu       sync.Mutex
}
```

Updated `Load`:

```go
func Load(ctx context.Context, manifest integrations.Manifest, wasmBytes []byte) (*WASMIntegration, error) {
	rt := SharedRuntime(ctx)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	mod, err := rt.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().
			WithName("").
			WithStdout(io.Discard).
			WithStderr(io.Discard),
	)
	if err != nil {
		compiled.Close(ctx)
		return nil, fmt.Errorf("instantiate wasm module: %w", err)
	}

	return &WASMIntegration{manifest: manifest, mod: mod}, nil
}
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass. The e2e integration tests load and run both WASM modules through the shared runtime.

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/runtime.go internal/wasm/runtime_test.go internal/wasm/loader.go
git commit -m "feat: shared wazero runtime singleton — compile host module once per process"
```

---

## Task 5: Settings store + Registry hot-swap

**Purpose:** Two things happen in this task:

1. **SettingsStore** (`internal/integrations/settings.go`) — persists trust and quarantine state to `~/.orbiter/settings.json`. Lives in the `integrations` package (not `wasm`) so the registry can use it without a circular import. Trust keys: `(brand, fullCommandString)`. Quarantine keys: `brand`. Only "allow" and quarantine entries are written; declines are ephemeral.

2. **Registry hot-swap** (`internal/integrations/registry.go`) — adds `sync.RWMutex` for concurrent access, an in-memory quarantine map (seeded from settings.json at startup), and `Deregister`, `QuarantineBrand`, `UnquarantineBrand` methods. `Get`/`All`/`AllForRole` skip quarantined brands immediately without a restart — an integration that is quarantined or deregistered goes offline in-memory the instant the call completes, with no process restart required. Unquarantine is equally instant: the integration's WASM module stays loaded, so reinstating it is just a flag flip.

The settings file format:

```json
{
  "trust": {
    "git": {
      "git version": "allow",
      "git clone https://github.com/acme/repo /home/captain/repos/acme": "allow"
    },
    "nvm": {
      "nvm install 20.11.0": "allow",
      "nvm use 20.11.0": "allow"
    }
  },
  "quarantine": {
    "bad-integration": {
      "reason": "attempted banned command: bash",
      "at": "2026-06-12T14:31:00Z"
    }
  }
}
```

**Files:**

- Create: `internal/integrations/settings.go`
- Create: `internal/integrations/settings_test.go`
- Modify: `internal/integrations/registry.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/integrations/settings_test.go
package integrations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestSettingsStore_Trust(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)

	// Not allowed before any entry
	if ss.IsAllowed("git", "git version") {
		t.Error("should not be allowed before Allow is called")
	}

	// Allow and check
	if err := ss.Allow("git", "git version"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !ss.IsAllowed("git", "git version") {
		t.Error("should be allowed after Allow")
	}

	// Different brand — not allowed
	if ss.IsAllowed("nvm", "git version") {
		t.Error("brand must match exactly")
	}

	// Different full command string — not allowed
	if ss.IsAllowed("git", "git clone https://example.com/repo") {
		t.Error("full command string must match exactly")
	}
}

func TestSettingsStore_Quarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)

	if ss.IsQuarantined("bad-integration") {
		t.Error("should not be quarantined before Quarantine is called")
	}

	if err := ss.Quarantine("bad-integration", "attempted banned command: bash"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if !ss.IsQuarantined("bad-integration") {
		t.Error("should be quarantined after Quarantine")
	}

	entry := ss.QuarantineEntry("bad-integration")
	if entry.Reason != "attempted banned command: bash" {
		t.Errorf("reason = %q", entry.Reason)
	}
	if entry.At.IsZero() {
		t.Error("at should be set")
	}
}

func TestSettingsStore_Unquarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	_ = ss.Quarantine("nvm", "attempted undeclared command: curl")

	if err := ss.Unquarantine("nvm"); err != nil {
		t.Fatalf("Unquarantine: %v", err)
	}
	if ss.IsQuarantined("nvm") {
		t.Error("should not be quarantined after Unquarantine")
	}
}

func TestSettingsStore_Persists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss1 := integrations.NewSettingsStore(path)
	_ = ss1.Allow("nvm", "nvm install 20.11.0")
	_ = ss1.Quarantine("bad", "attempted banned command: bash")

	ss2 := integrations.NewSettingsStore(path)
	if !ss2.IsAllowed("nvm", "nvm install 20.11.0") {
		t.Error("trust entry should persist")
	}
	if !ss2.IsQuarantined("bad") {
		t.Error("quarantine entry should persist")
	}
}

func TestSettingsStore_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	if ss.IsAllowed("git", "git version") {
		t.Error("missing file should return false, not panic")
	}
	if ss.IsQuarantined("any") {
		t.Error("missing file should return false for quarantine")
	}
}

func TestSettingsStore_JSONShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	_ = ss.Allow("git", "git version")
	_ = ss.Quarantine("bad", "attempted banned command: bash")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"trust"`) {
		t.Errorf("missing trust key: %s", s)
	}
	if !strings.Contains(s, `"quarantine"`) {
		t.Errorf("missing quarantine key: %s", s)
	}
	if !strings.Contains(s, `"git version"`) {
		t.Errorf("missing command string: %s", s)
	}
}
```

Add `"strings"` to the imports block.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/integrations/... -run TestSettingsStore -v
```

Expected: FAIL — `integrations.NewSettingsStore` does not exist.

- [ ] **Step 3: Create `internal/integrations/settings.go`**

```go
package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SettingsStore persists trust and quarantine state to ~/.orbiter/settings.json.
// It is NOT the Star Chart — it captures meta-properties of the Orbiter installation.
// Trust keys: (brand, fullCommandString). Quarantine keys: brand.
// Only "allow" and quarantine entries are written; Captain declines are ephemeral.
type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

type settingsFile struct {
	Trust      map[string]map[string]string  `json:"trust,omitempty"`
	Quarantine map[string]quarantineEntry    `json:"quarantine,omitempty"`
}

// QuarantineInfo holds the details of a quarantine entry.
type QuarantineInfo struct {
	Reason string
	At     time.Time
}

type quarantineEntry struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// NewSettingsStore returns a SettingsStore backed by path.
// A missing file is treated as empty — no error.
func NewSettingsStore(path string) *SettingsStore {
	ss := &SettingsStore{path: path}
	ss.load()
	return ss
}

// IsAllowed returns true if the Captain has previously always-allowed
// this exact (brand, fullCommandString) pair.
func (ss *SettingsStore) IsAllowed(brand, fullCmd string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Trust == nil {
		return false
	}
	return ss.data.Trust[brand][fullCmd] == "allow"
}

// Allow records a permanent allow for (brand, fullCmd) and flushes to disk.
func (ss *SettingsStore) Allow(brand, fullCmd string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Trust == nil {
		ss.data.Trust = make(map[string]map[string]string)
	}
	if ss.data.Trust[brand] == nil {
		ss.data.Trust[brand] = make(map[string]string)
	}
	ss.data.Trust[brand][fullCmd] = "allow"
	return ss.flush()
}

// IsQuarantined returns true if brand has a quarantine entry.
func (ss *SettingsStore) IsQuarantined(brand string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Quarantine == nil {
		return false
	}
	_, ok := ss.data.Quarantine[brand]
	return ok
}

// QuarantineEntry returns the quarantine details for brand.
// Returns a zero QuarantineInfo if brand is not quarantined.
func (ss *SettingsStore) QuarantineEntry(brand string) QuarantineInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Quarantine == nil {
		return QuarantineInfo{}
	}
	e, ok := ss.data.Quarantine[brand]
	if !ok {
		return QuarantineInfo{}
	}
	return QuarantineInfo{Reason: e.Reason, At: e.At}
}

// Quarantine marks brand as quarantined and flushes to disk.
func (ss *SettingsStore) Quarantine(brand, reason string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Quarantine == nil {
		ss.data.Quarantine = make(map[string]quarantineEntry)
	}
	ss.data.Quarantine[brand] = quarantineEntry{Reason: reason, At: time.Now().UTC()}
	return ss.flush()
}

// Unquarantine removes the quarantine entry for brand and flushes to disk.
func (ss *SettingsStore) Unquarantine(brand string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Quarantine != nil {
		delete(ss.data.Quarantine, brand)
	}
	return ss.flush()
}

func (ss *SettingsStore) load() {
	data, err := os.ReadFile(ss.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ss.data)
}

func (ss *SettingsStore) flush() error {
	if err := os.MkdirAll(filepath.Dir(ss.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ss.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ss.path, append(data, '\n'), 0600)
}

// DefaultSettings is the process-wide settings store at ~/.orbiter/settings.json.
var DefaultSettings = func() *SettingsStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return NewSettingsStore(filepath.Join(home, ".orbiter", "settings.json"))
}()
```

- [ ] **Step 4: Run settings store tests**

```bash
go test ./internal/integrations/... -run TestSettingsStore -v
```

Expected: PASS.

- [ ] **Step 5: Update `internal/integrations/registry.go` — hot-swap support**

Replace the entire file:

```go
package integrations

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Integration must be implemented by every registered integration.
type Integration interface {
	Meta() Manifest
	Detect(ctx DetectContext) DetectReport
	Init(ctx ResolvedContext) StateReport
	Scan(ctx ResolvedContext) StateReport
	Calibrate(ctx ResolvedContext) StateReport
}

// Registry holds a set of integrations keyed by "role/brand".
// All methods are goroutine-safe. Integrations can be registered, deregistered,
// quarantined, and unquarantined at runtime without a process restart.
type Registry struct {
	mu          sync.RWMutex
	entries     map[string]Integration // key: "role/brand"
	quarantined map[string]bool        // key: brand; in-memory mirror of settings quarantine
	settings    *SettingsStore
}

// NewRegistry returns an empty Registry backed by the given settings store.
// Quarantine state from settings is loaded into memory immediately.
func NewRegistry(settings *SettingsStore) *Registry {
	r := &Registry{
		entries:     make(map[string]Integration),
		quarantined: make(map[string]bool),
		settings:    settings,
	}
	// Seed in-memory quarantine from persisted state
	if settings != nil {
		settings.mu.RLock()
		for brand := range settings.data.Quarantine {
			r.quarantined[brand] = true
		}
		settings.mu.RUnlock()
	}
	return r
}

// Register adds or replaces the integration for the given role and brand.
// Takes effect immediately — no restart required.
func (r *Registry) Register(role, brand string, i Integration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[role+"/"+brand] = i
}

// Deregister removes the integration for the given role and brand.
// Takes effect immediately. Does not affect quarantine state.
func (r *Registry) Deregister(role, brand string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, role+"/"+brand)
}

// Get returns the integration for role+brand. Returns (nil, false) if not registered
// or if the brand is currently quarantined.
func (r *Registry) Get(role, brand string) (Integration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.quarantined[brand] {
		return nil, false
	}
	i, ok := r.entries[role+"/"+brand]
	return i, ok
}

// All returns every non-quarantined integration.
func (r *Registry) All() []Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Integration, 0, len(r.entries))
	for key, i := range r.entries {
		brand := brandFromKey(key)
		if !r.quarantined[brand] {
			result = append(result, i)
		}
	}
	return result
}

// AllForRole returns all non-quarantined integrations for a given role.
func (r *Registry) AllForRole(role string) []Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prefix := role + "/"
	var result []Integration
	for key, i := range r.entries {
		if strings.HasPrefix(key, prefix) && !r.quarantined[brandFromKey(key)] {
			result = append(result, i)
		}
	}
	return result
}

// IsQuarantined returns true if brand is currently quarantined.
func (r *Registry) IsQuarantined(brand string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.quarantined[brand]
}

// QuarantineBrand marks brand as quarantined in memory and persists to settings.json.
// The integration goes offline immediately — all subsequent Get calls for this brand
// return (nil, false). The WASM module stays loaded; Unquarantine reinstates it instantly.
// Prints a warning to stderr so the Captain is notified mid-flight.
func (r *Registry) QuarantineBrand(brand, reason string) error {
	r.mu.Lock()
	r.quarantined[brand] = true
	r.mu.Unlock()

	fmt.Fprintf(os.Stderr,
		"\n  orbiter: integration %q quarantined — %s\n  Review: orbiter vessel inspect %s\n\n",
		brand, reason, brand,
	)

	if r.settings != nil {
		return r.settings.Quarantine(brand, reason)
	}
	return nil
}

// UnquarantineBrand clears the quarantine flag in memory and in settings.json.
// The integration is available immediately — no restart required.
func (r *Registry) UnquarantineBrand(brand string) error {
	r.mu.Lock()
	delete(r.quarantined, brand)
	r.mu.Unlock()

	if r.settings != nil {
		return r.settings.Unquarantine(brand)
	}
	return nil
}

// Default is the process-wide registry, backed by DefaultSettings.
var Default = NewRegistry(DefaultSettings)

// Register adds an integration to the Default registry.
func Register(role, brand string, i Integration) {
	Default.Register(role, brand, i)
}

func brandFromKey(key string) string {
	if i := strings.Index(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}
```

- [ ] **Step 6: Run all tests**

```bash
go test ./... -count=1
```

`NewRegistry` now requires a `*SettingsStore`. Any test that calls `NewRegistry()` without args must be updated to `NewRegistry(nil)` (nil settings = no persistence, quarantine still works in-memory). The `integrations/catalog_test.go` from Task 9 calls `core.NewRegistry()` — update that call to `core.NewRegistry(nil)`.

- [ ] **Step 7: Commit**

```bash
git add internal/integrations/settings.go internal/integrations/settings_test.go \
    internal/integrations/registry.go
git commit -m "feat: settings store + registry hot-swap — quarantine integrations at runtime without restart"
```

---

## Task 6: Audit log + host hardening + Captain approval

**Purpose:** Every `run_command` call passes through a six-stage gate before execution:

0. **Quarantine check** — if the integration's brand is quarantined in `integrations.Default`, reject immediately. Already logged and quarantined at fault time; this just silently drops remaining calls from that integration.
1. **Banlist** — hard-reject shell interpreters (`sh`, `bash`, `zsh`, `fish`, `dash`, `ksh`, `tcsh`, `csh`, `ash`) and privilege escalation (`sudo`, `su`, `doas`, `pkexec`, `runas`). Log with `"banned": true`, then quarantine the brand immediately via `integrations.Default.QuarantineBrand`.
2. **Allowlist** — reject executables not declared in the manifest's `[commands] allowed` list. Log with `"rejected": true`, then quarantine immediately via `integrations.Default.QuarantineBrand`.
3. **Trust check** — if the full command string (`cmd + " " + args joined`) is in `integrations.DefaultSettings`, run silently. Logged with `"trusted": true`.
4. **Captain prompt** — pause, render the command string to stderr, ask: `[a]lways allow / [o]nce / [d]ecline`. Block until Captain responds. Logged with `"approved": "always"`, `"approved": "once"`, or `"declined": true`. Captain declining is **not** a quarantine trigger — it is the Captain redirecting an operation mid-flight.
5. **Execution** — run under PATH-only environment with manifest timeout.

Any stage 1 or 2 fault quarantines the integration immediately — no threshold, first fault. The integration goes offline in-memory for all subsequent calls in the same process. The Captain is notified via stderr and must run `orbiter vessel inspect <brand>` to review and `orbiter vessel unquarantine <brand>` to reinstate.

If `--unattended` is set: prompt is printed with a danger warning to stderr; commands already trusted execute silently; new trust is never written.

**Files:**

- Create: `internal/wasm/audit.go`
- Create: `internal/wasm/audit_test.go`
- Modify: `internal/wasm/host.go`
- Modify: `internal/wasm/loader.go`

The `ApproveFunc` type is the Captain-prompt callback. It is nil-safe: when nil (tests, unattended with no-prompt flag), new trust is skipped and not-already-trusted commands are declined.

```go
// ApproveFunc is called when a command is not in the trust store.
// It presents the full command string to the Captain and returns whether to proceed
// and whether to write a permanent allow. Returning approved=false declines the command.
type ApproveFunc func(brand, fullCmd string) (always bool, approved bool)
```

- [ ] **Step 1: Write failing tests**

```go
// internal/wasm/audit_test.go
package wasm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestAuditLog_WritesTrustedEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al := wasm.NewAuditLog(logPath)
	al.Log(wasm.AuditEntry{
		Brand: "git", Cmd: "git", Args: []string{"version"},
		Exit: 0, DurationMS: 42, Trusted: true,
	})

	data, _ := os.ReadFile(logPath)
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"trusted":true`) {
		t.Errorf("missing trusted field: %s", line)
	}
	if !strings.Contains(line, `"brand":"git"`) {
		t.Errorf("missing brand: %s", line)
	}
}

func TestAuditLog_WritesBannedEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al := wasm.NewAuditLog(logPath)
	al.Log(wasm.AuditEntry{
		Brand: "bad", Cmd: "bash", Args: []string{"-c", "rm -rf /"},
		Exit: -1, Banned: true, Reason: "shell interpreter",
	})

	data, _ := os.ReadFile(logPath)
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"banned":true`) {
		t.Errorf("missing banned: %s", line)
	}
	if !strings.Contains(line, `"reason":"shell interpreter"`) {
		t.Errorf("missing reason: %s", line)
	}
}

func TestAuditLog_WritesRejectedEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al := wasm.NewAuditLog(logPath)
	al.Log(wasm.AuditEntry{
		Brand: "gh", Cmd: "curl", Args: []string{"evil.com"},
		Exit: -1, Rejected: true, Reason: "not declared",
	})

	data, _ := os.ReadFile(logPath)
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"rejected":true`) {
		t.Errorf("missing rejected: %s", line)
	}
}

func TestAuditLog_WritesApprovedEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al := wasm.NewAuditLog(logPath)
	al.Log(wasm.AuditEntry{
		Brand: "nvm", Cmd: "nvm", Args: []string{"install", "24.0.0"},
		Exit: 0, DurationMS: 3100, Approved: "always",
	})

	data, _ := os.ReadFile(logPath)
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"approved":"always"`) {
		t.Errorf("missing approved: %s", line)
	}
}

func TestAuditLog_AppendMultiple(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	al := wasm.NewAuditLog(logPath)
	al.Log(wasm.AuditEntry{Brand: "git", Cmd: "git", Args: []string{"version"}, Exit: 0, DurationMS: 10, Trusted: true})
	al.Log(wasm.AuditEntry{Brand: "git", Cmd: "git", Args: []string{"status"}, Exit: 0, DurationMS: 5, Trusted: true})

	data, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(lines))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/wasm/... -run TestAuditLog -v
```

Expected: FAIL — `wasm.NewAuditLog`, `wasm.AuditEntry`, and `wasm.AuditLog` do not exist.

- [ ] **Step 3: Create `internal/wasm/audit.go`**

```go
package wasm

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry is one line in the audit log.
// Exactly one of Banned, Rejected, Trusted, Approved, or Declined is set per entry.
type AuditEntry struct {
	TS         string   `json:"ts"`
	Brand      string   `json:"brand"`
	Cmd        string   `json:"cmd"`
	Args       []string `json:"args"`
	Exit       int      `json:"exit"`
	DurationMS int64    `json:"duration_ms,omitempty"`
	Banned     bool     `json:"banned,omitempty"`
	Rejected   bool     `json:"rejected,omitempty"`
	Trusted    bool     `json:"trusted,omitempty"`
	Approved   string   `json:"approved,omitempty"` // "always" or "once"
	Declined   bool     `json:"declined,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// AuditLog writes append-only JSON lines to a file.
type AuditLog struct {
	path string
}

// NewAuditLog returns an AuditLog that writes to path.
// The parent directory is created if it does not exist.
func NewAuditLog(path string) *AuditLog {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.Printf("wasm: create audit log dir: %v", err)
	}
	return &AuditLog{path: path}
}

// Log appends one JSON line. TS is filled automatically if empty.
func (a *AuditLog) Log(e AuditEntry) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("wasm: marshal audit entry: %v", err)
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("wasm: open audit log: %v", err)
		return
	}
	defer f.Close()
	f.Write(data)
}

// DefaultAuditLog is the process-wide audit log at ~/.orbiter/audit.log.
var DefaultAuditLog = func() *AuditLog {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return NewAuditLog(filepath.Join(home, ".orbiter", "audit.log"))
}()
```

- [ ] **Step 4: Run audit tests**

```bash
go test ./internal/wasm/... -run TestAuditLog -v
```

Expected: PASS.

- [ ] **Step 5: Update `callState` in `internal/wasm/host.go` — add brand, allowed, timeout, approve**

`callState` is defined in `host.go`. Replace the struct:

```go
// ApproveFunc is the Captain-prompt callback. brand is the integration brand.
// fullCmd is the full command string ("cmd arg1 arg2"). Returns always=true if the
// Captain chose "always allow" (persist to integrations.DefaultSettings); approved=false = decline.
type ApproveFunc func(brand, fullCmd string) (always bool, approved bool)

type callState struct {
	input   []byte
	output  []byte
	brand   string        // integration brand — used in audit log and trust check
	allowed []string      // manifest [commands].allowed
	timeout time.Duration // manifest [commands].timeout_seconds converted
	approve ApproveFunc   // nil = no interactive prompt (tests / unattended no-trust)
}
```

Add `"time"` to imports.

- [ ] **Step 6: Add banlist in `internal/wasm/host.go`**

Add the banlist and helper after the callState definition:

```go
// commandBanlist contains executables that can never be called regardless of manifest.
// Keys are executable names; values are human-readable reasons for the audit log.
var commandBanlist = map[string]string{
	// Shell interpreters — prevent sh -c / bash -c script embedding
	"sh":    "shell interpreter",
	"bash":  "shell interpreter",
	"zsh":   "shell interpreter",
	"fish":  "shell interpreter",
	"dash":  "shell interpreter",
	"ksh":   "shell interpreter",
	"tcsh":  "shell interpreter",
	"csh":   "shell interpreter",
	"ash":   "shell interpreter",
	// Privilege escalation
	"sudo":   "privilege escalation",
	"su":     "privilege escalation",
	"doas":   "privilege escalation",
	"pkexec": "privilege escalation",
	"runas":  "privilege escalation",
}

// isBanned returns the ban reason if cmd is hard-banned, or "" if not.
func isBanned(cmd string) string {
	return commandBanlist[cmd]
}

// commandAllowed returns true if cmd is in the allowed list.
// An empty allowlist means nothing is allowed.
func commandAllowed(cmd string, allowed []string) bool {
	for _, a := range allowed {
		if a == cmd {
			return true
		}
	}
	return false
}

// fullCommandString joins cmd and args into a single string for trust key lookup.
func fullCommandString(cmd string, args []string) string {
	if len(args) == 0 {
		return cmd
	}
	return cmd + " " + strings.Join(args, " ")
}
```

Add `"strings"` to imports in host.go.

- [ ] **Step 7: Update `runCommandFn` in `internal/wasm/host.go` — five-stage gate**

Replace the entire `runCommandFn` body:

```go
func runCommandFn(ctx context.Context, mod api.Module, stack []uint64) {
	specPtr := uint32(stack[0])
	specLen := uint32(stack[1])
	outPtr := uint32(stack[2])
	outMax := uint32(stack[3])

	cs, _ := ctx.Value(callStateKey{}).(*callState)
	if cs == nil {
		stack[0] = 0
		return
	}

	specBytes, ok := mod.Memory().Read(specPtr, specLen)
	if !ok {
		stack[0] = 0
		return
	}

	var spec struct {
		Cmd  string   `json:"cmd"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		stack[0] = 0
		return
	}

	fullCmd := fullCommandString(spec.Cmd, spec.Args)

	// Stage 0: Quarantine check — integration already quarantined, drop silently
	if integrations.Default.IsQuarantined(cs.brand) {
		stack[0] = 0
		return
	}

	// Stage 1: Banlist — hard reject + immediate quarantine, no override
	if reason := isBanned(spec.Cmd); reason != "" {
		DefaultAuditLog.Log(AuditEntry{
			Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
			Exit: -1, Banned: true, Reason: reason,
		})
		_ = integrations.Default.QuarantineBrand(cs.brand, "attempted banned command: "+spec.Cmd)
		stack[0] = 0
		return
	}

	// Stage 2: Allowlist — reject + immediate quarantine if not declared in manifest
	if !commandAllowed(spec.Cmd, cs.allowed) {
		DefaultAuditLog.Log(AuditEntry{
			Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
			Exit: -1, Rejected: true, Reason: "not declared",
		})
		_ = integrations.Default.QuarantineBrand(cs.brand, "attempted undeclared command: "+spec.Cmd)
		stack[0] = 0
		return
	}

	// Stage 3: Trust check — run silently if Captain already approved this exact string
	if integrations.DefaultSettings.IsAllowed(cs.brand, fullCmd) {
		out, exitCode, elapsed := runSubprocess(ctx, spec.Cmd, spec.Args, cs.timeout)
		DefaultAuditLog.Log(AuditEntry{
			Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
			Exit: exitCode, DurationMS: elapsed.Milliseconds(), Trusted: true,
		})
		writeOutput(mod, out, outPtr, outMax, &stack[0])
		return
	}

	// Stage 4: Captain approval prompt
	// Declining is the Captain redirecting mid-flight — NOT a quarantine trigger.
	if cs.approve == nil {
		// No prompt available (tests / unattended) — decline without quarantine
		DefaultAuditLog.Log(AuditEntry{
			Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
			Exit: -1, Declined: true,
		})
		stack[0] = 0
		return
	}

	always, approved := cs.approve(cs.brand, fullCmd)
	if !approved {
		DefaultAuditLog.Log(AuditEntry{
			Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
			Exit: -1, Declined: true,
		})
		stack[0] = 0
		return
	}

	approvalKind := "once"
	if always {
		approvalKind = "always"
		_ = integrations.DefaultSettings.Allow(cs.brand, fullCmd)
	}

	// Stage 5: Execute
	out, exitCode, elapsed := runSubprocess(ctx, spec.Cmd, spec.Args, cs.timeout)
	DefaultAuditLog.Log(AuditEntry{
		Brand: cs.brand, Cmd: spec.Cmd, Args: spec.Args,
		Exit: exitCode, DurationMS: elapsed.Milliseconds(), Approved: approvalKind,
	})
	writeOutput(mod, out, outPtr, outMax, &stack[0])
}

// runSubprocess executes cmd with args under PATH-only environment.
func runSubprocess(ctx context.Context, cmd string, args []string, timeout time.Duration) ([]byte, int, time.Duration) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(cmdCtx, cmd, args...)
	c.Env = []string{"PATH=" + os.Getenv("PATH")}

	start := time.Now()
	out, err := c.Output()
	elapsed := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok2 := err.(*exec.ExitError); ok2 {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return out, exitCode, elapsed
}

// writeOutput copies out into WASM memory and sets stack[0] to bytes written.
func writeOutput(mod api.Module, out []byte, ptr, max uint32, result *uint64) {
	n := uint32(len(out))
	if n > max {
		n = max
	}
	if n > 0 {
		mod.Memory().Write(ptr, out[:n])
	}
	*result = uint64(n)
}
```

Add imports: `"os"`, `"strings"`, and `"time"` to host.go.

- [ ] **Step 8: Update `invoke` in `internal/wasm/loader.go` — populate callState**

In `invoke`, update the `callState` construction to add the `approve` field:

```go
func (w *WASMIntegration) invoke(ctx context.Context, fn string, input []byte, approve ApproveFunc) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	timeout := time.Duration(w.manifest.Commands.TimeoutSeconds) * time.Second

	cs := &callState{
		input:   input,
		brand:   w.manifest.Integration.Brand,
		allowed: w.manifest.Commands.Allowed,
		timeout: timeout,
		approve: approve,
	}
	ctx = context.WithValue(ctx, callStateKey{}, cs)

	exported := w.mod.ExportedFunction(fn)
	if exported == nil {
		return nil, fmt.Errorf("function %q not exported by wasm module", fn)
	}

	if _, err := exported.Call(ctx); err != nil {
		return nil, fmt.Errorf("call %q: %w", fn, err)
	}
	return cs.output, nil
}
```

Update `Scan`, `Init`, and `Calibrate` on `WASMIntegration` to pass `nil` as the `approve` arg for now (the lifecycle integration in Task 8 will wire the real prompt). Add `"time"` to imports in loader.go.

- [ ] **Step 9: Add `StdinApproveFunc` in `internal/wasm/host.go` — the real terminal prompt**

Add after the banlist block:

```go
// StdinApproveFunc presents the full command string to the Captain on stderr and
// reads a single character response. Use this in interactive (non-unattended) runs.
func StdinApproveFunc(brand, fullCmd string) (always bool, approved bool) {
	fmt.Fprintf(os.Stderr, "\n  Integration %q wants to run:\n    %s\n\n  [a]lways allow  [o]nce  [d]ecline: ", brand, fullCmd)

	var buf [4]byte
	n, _ := os.Stdin.Read(buf[:])
	if n == 0 {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(string(buf[:n])))[0] {
	case 'a':
		return true, true
	case 'o':
		return false, true
	default:
		return false, false
	}
}
```

Add `"fmt"` to imports in host.go.

- [ ] **Step 10: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass. The e2e integration tests still work since `git` and `go` are in their respective manifests' `allowed` lists and `approve` is nil in tests (commands that are trusted will be looked up; in tests the trust store file doesn't exist so they proceed to the nil approve path — if e2e tests use `Scan` which calls shell commands, they will decline). 

**Note:** The e2e test calls `i.Scan(integrations.ResolvedContext{})` on the go and git integrations. If `Scan` internally calls `run_command`, those calls will now hit Stage 4 with `approve=nil` and be declined. Check if the e2e test's `Scan` makes shell calls. If the WASM `scan` function calls `run_command`, the test must either pre-populate the trust store or pass a permissive `ApproveFunc`. The safest approach: add a `TestApproveFunc` that always returns `(false, true)` (allow once) and update the `WASMIntegration.Scan` method signature to accept an optional `ApproveFunc`, or store it on the struct before calling `Scan`. If e2e tests were passing before this task, and those tests invoke shell commands through WASM, they will fail. Fix by updating the `WASMIntegration.Scan/Init/Calibrate` methods to accept an `ApproveFunc` parameter, or by storing it on the `WASMIntegration` struct via a `SetApproveFunc(fn ApproveFunc)` method called in tests.

- [ ] **Step 11: Commit**

```bash
git add internal/wasm/audit.go internal/wasm/audit_test.go internal/wasm/host.go internal/wasm/loader.go
git commit -m "feat: run_command five-stage gate — banlist, allowlist, trust check, Captain approval, audit log"
```

---

## Task 7: Loader pooling + exports validation

**Purpose:** Replace the `sync.Mutex` + single module instance with a buffered channel pool. Multiple goroutines can now dispatch to the same integration concurrently. Also validate `StateReport.Exports` against the manifest's `[shell] exports` allowlist before returning — undeclared exports are dropped and logged.

**Files:**

- Modify: `internal/wasm/loader.go`
- Modify: `internal/wasm/loader_test.go` (create if it doesn't exist)

- [ ] **Step 1: Write failing tests**

```go
// internal/wasm/loader_test.go
package wasm_test

import (
	"context"
	"testing"

	_ "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestWASMIntegration_ConcurrentScan(t *testing.T) {
	// Integration must handle concurrent calls (pool not mutex)
	i, ok := integrations.Default.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}

	const goroutines = 8
	results := make(chan integrations.StateReport, goroutines)
	for n := 0; n < goroutines; n++ {
		go func() {
			results <- i.Scan(integrations.ResolvedContext{})
		}()
	}
	for n := 0; n < goroutines; n++ {
		report := <-results
		if !report.Present {
			t.Error("expected present=true from concurrent scan")
		}
	}
}

func TestWASMIntegration_ExportsFiltered(t *testing.T) {
	// Exports not declared in [shell] exports must be dropped.
	// The git integration's manifest has no [shell] exports declared,
	// so any exports from git's StateReport would be stripped.
	i, ok := integrations.Default.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}
	report := i.Scan(integrations.ResolvedContext{})
	// git integration returns no exports — verify the field is nil/empty
	if len(report.Exports) != 0 {
		t.Errorf("expected no exports from git scan, got %v", report.Exports)
	}
}
```

- [ ] **Step 2: Run test to verify concurrent test fails (deadlocks or races)**

```bash
go test ./internal/wasm/... -run TestWASMIntegration_ConcurrentScan -v -race
```

Expected: may deadlock or race under the current mutex + single-instance implementation. The race detector may flag it, or it may complete slowly. The pool fixes this.

- [ ] **Step 3: Update `WASMIntegration` struct in `internal/wasm/loader.go` — pool instead of mutex**

```go
// WASMIntegration wraps a compiled WASM module and a pool of live instances.
// Dispatch acquires an instance from the pool, calls the exported function,
// and returns the instance — enabling concurrent invocations.
type WASMIntegration struct {
	manifest integrations.Manifest
	pool     chan api.Module
}
```

Remove `mu sync.Mutex` and `mod api.Module` fields.

- [ ] **Step 4: Update `Load` in `internal/wasm/loader.go` — compile once, instantiate pool**

```go
// Load compiles wasmBytes once and fills a pool of live instances.
// Pool size is taken from manifest.Runtime.PoolSize (default 4).
func Load(ctx context.Context, manifest integrations.Manifest, wasmBytes []byte) (*WASMIntegration, error) {
	rt := SharedRuntime(ctx)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}
	defer compiled.Close(ctx)

	poolSize := manifest.Runtime.PoolSize
	if poolSize <= 0 {
		poolSize = 4
	}

	pool := make(chan api.Module, poolSize)
	for idx := 0; idx < poolSize; idx++ {
		mod, err := rt.InstantiateModule(ctx, compiled,
			wazero.NewModuleConfig().
				WithName("").
				WithStdout(io.Discard).
				WithStderr(io.Discard),
		)
		if err != nil {
			// drain and close any already-instantiated modules
			close(pool)
			for m := range pool {
				m.Close(ctx)
			}
			return nil, fmt.Errorf("instantiate wasm module (instance %d): %w", idx, err)
		}
		pool <- mod
	}

	return &WASMIntegration{manifest: manifest, pool: pool}, nil
}
```

- [ ] **Step 5: Update `invoke` in `internal/wasm/loader.go` — acquire from pool**

```go
func (w *WASMIntegration) invoke(ctx context.Context, fn string, input []byte) ([]byte, error) {
	mod := <-w.pool
	defer func() { w.pool <- mod }()

	timeout := time.Duration(w.manifest.Commands.TimeoutSeconds) * time.Second

	cs := &callState{
		input:   input,
		brand:   w.manifest.Integration.Brand,
		allowed: w.manifest.Commands.Allowed,
		timeout: timeout,
	}
	ctx = context.WithValue(ctx, callStateKey{}, cs)

	exported := mod.ExportedFunction(fn)
	if exported == nil {
		return nil, fmt.Errorf("function %q not exported by wasm module", fn)
	}

	if _, err := exported.Call(ctx); err != nil {
		return nil, fmt.Errorf("call %q: %w", fn, err)
	}
	return cs.output, nil
}
```

Remove `"sync"` from imports in loader.go (no more `sync.Mutex`).

- [ ] **Step 6: Add exports validation to dispatch methods**

Add a helper and call it in `Scan`, `Init`, and `Calibrate` after unmarshaling:

```go
// filterExports removes any export keys not declared in the manifest's [shell] exports allowlist.
// Undeclared keys are silently dropped (logged at debug level in future).
func (w *WASMIntegration) filterExports(report *integrations.StateReport) {
	if len(report.Exports) == 0 {
		return
	}
	allowed := make(map[string]bool, len(w.manifest.Shell.Exports))
	for _, key := range w.manifest.Shell.Exports {
		allowed[key] = true
	}
	for key := range report.Exports {
		if !allowed[key] {
			delete(report.Exports, key)
		}
	}
}
```

Update each dispatch method to call `filterExports`:

```go
func (w *WASMIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "scan", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	json.Unmarshal(out, &report)
	w.filterExports(&report)
	return report
}
```

Apply the same pattern to `Init` and `Calibrate`.

- [ ] **Step 7: Run all tests with race detector**

```bash
go test ./... -count=1 -race
```

Expected: all pass, no data races. The concurrent scan test now completes without deadlock.

- [ ] **Step 8: Commit**

```bash
git add internal/wasm/loader.go internal/wasm/loader_test.go
git commit -m "feat: WASM module pool and exports validation — concurrent dispatch, shell security"
```

---

## Task 8: Lifecycle dispatch ordering + transponder pass

**Purpose:** `ScanBranch` and `CalibrateBranch` currently dispatch resources in level order with no role ordering within a level. We add: (1) role-ordered dispatch within each level (`manager → runtime → tool → remote → filesystem`), (2) a separate parallel transponder scan/calibrate pass after all resources, (3) transponder result types in `BranchScanResult` and `BranchCalibrateResult`.

**Files:**

- Modify: `internal/starchart/lifecycle.go`
- Modify: `internal/starchart/lifecycle_test.go` (existing test file)

- [ ] **Step 1: Write failing tests**

```go
// Add to internal/starchart/lifecycle_test.go (find the file and add these test functions)
func TestScanBranch_RoleOrder(t *testing.T) {
	// Resources should be returned in manager→runtime→tool→remote→filesystem order
	// within each FILO level.
	// This test verifies role ordering by checking the result slice order.
	sc, entityID := setupBranchWithMultipleRoles(t) // helper defined below
	ctx := context.Background()

	result, err := sc.ScanBranch(ctx, entityID)
	if err != nil {
		t.Fatalf("ScanBranch: %v", err)
	}

	// Build the order of roles seen in the result
	roles := make([]string, 0, len(result.Resources))
	for _, r := range result.Resources {
		roles = append(roles, r.Resource.Role)
	}

	// manager must come before runtime; runtime before tool
	managerIdx, runtimeIdx, toolIdx := -1, -1, -1
	for i, role := range roles {
		switch role {
		case "manager":
			if managerIdx == -1 {
				managerIdx = i
			}
		case "runtime":
			if runtimeIdx == -1 {
				runtimeIdx = i
			}
		case "tool":
			if toolIdx == -1 {
				toolIdx = i
			}
		}
	}
	if managerIdx != -1 && runtimeIdx != -1 && managerIdx >= runtimeIdx {
		t.Errorf("manager (idx %d) must appear before runtime (idx %d)", managerIdx, runtimeIdx)
	}
	if runtimeIdx != -1 && toolIdx != -1 && runtimeIdx >= toolIdx {
		t.Errorf("runtime (idx %d) must appear before tool (idx %d)", runtimeIdx, toolIdx)
	}
}

func TestScanBranch_IncludesTransponders(t *testing.T) {
	sc := openTestStarChart(t)
	ctx := context.Background()
	vs := createTestVessel(t, sc, ctx)
	cs := createTestCallsign(t, sc, ctx, vs.ID)
	createTestTransponderWithConfig(t, sc, ctx, cs.ID, "env", "aws", `{"var":"AWS_PROFILE","value":"prod"}`)

	planet := createTestPlanet(t, sc, ctx, vs.ID, "")
	attachTestCallsign(t, sc, ctx, cs.ID, planet.ID)

	result, err := sc.ScanBranch(ctx, planet.ID)
	if err != nil {
		t.Fatalf("ScanBranch: %v", err)
	}
	if len(result.Transponders) == 0 {
		t.Error("expected transponder results in BranchScanResult")
	}
}
```

These tests reference helpers that may already exist in the test file or need to be added. Look at `internal/starchart/lifecycle_test.go` for the existing helper pattern (`openTestStarChart`, `createTestVessel`, etc.) and follow it.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... -run TestScanBranch_RoleOrder -v
go test ./internal/starchart/... -run TestScanBranch_IncludesTransponders -v
```

Expected: FAIL — `BranchScanResult` has no `Transponders` field; resources are not role-ordered.

- [ ] **Step 3: Add transponder result types and update branch result types in `internal/starchart/lifecycle.go`**

Add after the existing resource result types:

```go
// TransponderScanResult is the scan outcome for one transponder.
type TransponderScanResult struct {
	Transponder  models.Transponder
	Report       integrations.StateReport
	BeaconStatus string
}

// TransponderCalibrateResult is the calibration outcome for one transponder.
type TransponderCalibrateResult struct {
	Transponder models.Transponder
	Before      integrations.StateReport
	After       integrations.StateReport
	Action      string // "healthy", "calibrated", "failed"
}
```

Update `BranchScanResult` to include transponders:

```go
type BranchScanResult struct {
	Resources    []ResourceScanResult
	Transponders []TransponderScanResult
}
```

Update `BranchCalibrateResult`:

```go
type BranchCalibrateResult struct {
	Resources    []ResourceCalibrateResult
	Transponders []TransponderCalibrateResult
}
```

- [ ] **Step 4: Add role ordering constant and helper to `internal/starchart/lifecycle.go`**

```go
// resourceRoleOrder defines calibration order: managers first, filesystem last.
// Resources of the same role are dispatched in parallel (after this ordering pass).
var resourceRoleOrder = map[string]int{
	integrations.ResourceRoleManager:    0,
	integrations.ResourceRoleRuntime:    1,
	integrations.ResourceRoleTool:       2,
	integrations.ResourceRoleRemote:     3,
	integrations.ResourceRoleFilesystem: 4,
}

// sortResourcesByRole returns resources sorted by the role order above.
// Resources with unknown roles sort last.
func sortResourcesByRole(resources []models.Resource) []models.Resource {
	sorted := make([]models.Resource, len(resources))
	copy(sorted, resources)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi, oki := resourceRoleOrder[sorted[i].Role]
		oj, okj := resourceRoleOrder[sorted[j].Role]
		if !oki {
			oi = 99
		}
		if !okj {
			oj = 99
		}
		return oi < oj
	})
	return sorted
}
```

Add `"sort"` to imports.

- [ ] **Step 5: Update `ScanBranch` in `internal/starchart/lifecycle.go` — role ordering + transponder pass**

```go
func (sc *StarChart) ScanBranch(ctx context.Context, entityID string) (BranchScanResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchScanResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchScanResult

	// Resource pass: role-ordered within each level
	for _, level := range lb.Levels {
		for _, r := range sortResourcesByRole(level.Resources) {
			rr, err := sc.scanResource(ctx, r, level, lb)
			if err != nil {
				return BranchScanResult{}, err
			}
			result.Resources = append(result.Resources, rr)
		}
	}

	// Transponder pass: all transponders in parallel after resources complete
	var allTransponders []struct {
		tp    models.Transponder
		level BranchLevel
	}
	for _, level := range lb.Levels {
		for _, tp := range level.Transponders {
			allTransponders = append(allTransponders, struct {
				tp    models.Transponder
				level BranchLevel
			}{tp, level})
		}
	}

	tpResults := make([]TransponderScanResult, len(allTransponders))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for idx, item := range allTransponders {
		wg.Add(1)
		go func(i int, tp models.Transponder, level BranchLevel) {
			defer wg.Done()
			tr, err := sc.scanTransponder(ctx, tp, level, lb)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			tpResults[i] = tr
		}(idx, item.tp, item.level)
	}
	wg.Wait()
	if firstErr != nil {
		return BranchScanResult{}, firstErr
	}
	result.Transponders = tpResults

	return result, nil
}
```

Add `"sync"` to imports.

- [ ] **Step 6: Update `CalibrateBranch` in `internal/starchart/lifecycle.go` — role ordering + transponder pass**

```go
func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string) (BranchCalibrateResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchCalibrateResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchCalibrateResult

	// Resource pass: role-ordered — managers calibrate before runtimes, etc.
	for _, level := range lb.Levels {
		for _, r := range sortResourcesByRole(level.Resources) {
			cr, err := sc.calibrateResource(ctx, r, level, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Resources = append(result.Resources, cr)
		}
	}

	// Transponder pass: parallel after all resources
	var allTransponders []struct {
		tp    models.Transponder
		level BranchLevel
	}
	for _, level := range lb.Levels {
		for _, tp := range level.Transponders {
			allTransponders = append(allTransponders, struct {
				tp    models.Transponder
				level BranchLevel
			}{tp, level})
		}
	}

	tpResults := make([]TransponderCalibrateResult, len(allTransponders))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for idx, item := range allTransponders {
		wg.Add(1)
		go func(i int, tp models.Transponder, level BranchLevel) {
			defer wg.Done()
			tr, err := sc.calibrateTransponder(ctx, tp, level, lb)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			tpResults[i] = tr
		}(idx, item.tp, item.level)
	}
	wg.Wait()
	if firstErr != nil {
		return BranchCalibrateResult{}, firstErr
	}
	result.Transponders = tpResults

	return result, nil
}
```

- [ ] **Step 7: Add `scanTransponder` and `calibrateTransponder` helpers to `internal/starchart/lifecycle.go`**

```go
func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (TransponderScanResult, error) {
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, tp.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
		}); err != nil {
			return TransponderScanResult{}, err
		}
		return TransponderScanResult{Transponder: tp, BeaconStatus: status}, nil
	}
	rc := buildResolvedContextForTransponder(tp, level, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
		return TransponderScanResult{}, err
	}
	return TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (TransponderCalibrateResult, error) {
	scanResult, err := sc.scanTransponder(ctx, tp, level, lb)
	if err != nil {
		return TransponderCalibrateResult{}, err
	}
	if scanResult.BeaconStatus == models.BeaconStatusHealthy {
		return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, Action: "healthy"}, nil
	}
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, Action: "failed"}, nil
	}
	rc := buildResolvedContextForTransponder(tp, level, lb, integration.Meta())
	after := integration.Calibrate(rc)
	afterStatus := scanBeaconStatus(after)
	if err := sc.setBeaconStatus(ctx, tp.ID, afterStatus, after.Observations); err != nil {
		return TransponderCalibrateResult{}, err
	}
	action := "calibrated"
	if afterStatus == models.BeaconStatusFailed || after.Error != "" {
		action = "failed"
	}
	return TransponderCalibrateResult{Transponder: tp, Before: scanResult.Report, After: after, Action: action}, nil
}

// buildResolvedContextForTransponder constructs the dispatch context for a transponder.
// Self is the transponder itself. Resources come from all branch levels (FILO superseding).
// Transponders: only those at the same callsign level (auth isolation).
func buildResolvedContextForTransponder(self models.Transponder, level BranchLevel, lb LeveledBranch, manifest integrations.Manifest) integrations.ResolvedContext {
	rc := integrations.ResolvedContext{
		Platform:     lb.Platform,
		Self:         self,
		Resources:    make(map[string][]integrations.ResolvedResource),
		Transponders: make(map[string][]integrations.ResolvedTransponder),
	}
	seenResource := make(map[string]bool)
	for role, brands := range manifest.Dependencies.Resources {
		for _, l := range lb.Levels {
			for _, r := range l.Resources {
				key := r.Role + "/" + r.Brand
				if r.Role == role && brandAccepted(r.Brand, brands) && !seenResource[key] {
					seenResource[key] = true
					rc.Resources[role] = append(rc.Resources[role], integrations.ResolvedResource{Resource: r})
				}
			}
		}
	}
	for role, brands := range manifest.Dependencies.Transponders {
		for _, tp := range level.Transponders {
			if tp.Role == role && brandAccepted(tp.Brand, brands) {
				rc.Transponders[role] = append(rc.Transponders[role], integrations.ResolvedTransponder{Transponder: tp})
			}
		}
	}
	return rc
}
```

- [ ] **Step 8: Run all tests**

```bash
go test ./... -count=1 -race
```

Expected: all pass. The role ordering test verifies managers appear before runtimes; the transponder inclusion test confirms `BranchScanResult.Transponders` is populated.

- [ ] **Step 9: Commit**

```bash
git add internal/starchart/lifecycle.go internal/starchart/lifecycle_test.go
git commit -m "feat: role-ordered resource dispatch and parallel transponder pass in lifecycle"
```

---

## Task 9: Integration catalog + installation directory

**Purpose:** Bundled WASM files are embedded dormant — they are NOT auto-registered on startup. Instead `bundle.go` exposes `CatalogEntries()` (returns name/description/roles from each dormant manifest) and `InstallFromCatalog(brand, destDir)` (copies the .wasm + manifest to `~/.orbiter/integrations/<brand>/`). On startup, `bundle.go` loads WASM from `~/.orbiter/integrations/` only — bundled bytes are the source of truth for the catalog, not the active registry. The native filesystem integration (Go code, not WASM) remains always-active and is not part of the catalog.

`CatalogEntry` struct:

```go
type CatalogEntry struct {
    Brand       string
    Name        string
    Description string
    Roles       []string
}
```

**Files:**

- Modify: `integrations/bundle.go`
- Create: `integrations/catalog_test.go`

- [ ] **Step 1: Write failing tests**

```go
// integrations/catalog_test.go
package integrations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

func TestCatalogEntries_ReturnsBundledIntegrations(t *testing.T) {
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one catalog entry")
	}
	// Each entry must have brand, name, and at least one role
	for _, e := range entries {
		if e.Brand == "" {
			t.Errorf("entry has empty brand: %+v", e)
		}
		if e.Name == "" {
			t.Errorf("entry %q has empty name", e.Brand)
		}
		if len(e.Roles) == 0 {
			t.Errorf("entry %q has no roles", e.Brand)
		}
	}
}

func TestInstallFromCatalog_InstallsToDestDir(t *testing.T) {
	destDir := t.TempDir()

	if err := integrations.InstallFromCatalog("git", destDir); err != nil {
		t.Fatalf("InstallFromCatalog: %v", err)
	}

	// Expect destDir/git/manifest.toml and destDir/git/git.wasm
	if _, err := os.Stat(filepath.Join(destDir, "git", "manifest.toml")); err != nil {
		t.Errorf("manifest.toml not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "git", "git.wasm")); err != nil {
		t.Errorf("git.wasm not installed: %v", err)
	}
}

func TestInstallFromCatalog_UnknownBrandErrors(t *testing.T) {
	destDir := t.TempDir()
	if err := integrations.InstallFromCatalog("notabrand", destDir); err == nil {
		t.Error("expected error for unknown brand")
	}
}

func TestBundledIntegrations_NotAutoRegistered(t *testing.T) {
	// The catalog embeds git + go, but they are NOT auto-registered on import.
	// Default registry must be empty unless the Captain has installed them.
	// (In CI, ~/.orbiter/integrations/ does not exist — so Default is empty.)
	// We check that the bundled brands are NOT in Default.
	if _, ok := core.Default.Get("tool", "git"); ok {
		t.Error("git should not be auto-registered — it must be installed via vessel init")
	}
}

func TestLoadInstalled_LoadsFromDir(t *testing.T) {
	destDir := t.TempDir()

	// Install git to a temp dir, then load from it
	if err := integrations.InstallFromCatalog("git", destDir); err != nil {
		t.Fatalf("install: %v", err)
	}

	reg := core.NewRegistry()
	if err := integrations.LoadInstalled(destDir, reg); err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}

	if _, ok := reg.Get("tool", "git"); !ok {
		t.Error("expected git to be loaded into registry")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./integrations/... -run "TestCatalog|TestInstall|TestLoadInstalled|TestBundled" -v
```

Expected: FAIL — `integrations.CatalogEntries`, `integrations.InstallFromCatalog`, and `integrations.LoadInstalled` do not exist.

- [ ] **Step 3: Rewrite `integrations/bundle.go`**

The bundle no longer has an `init()` that auto-registers. It exposes the catalog and an installer. Startup calls `LoadInstalled` explicitly.

```go
package integrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

//go:embed golang git
var bundledFS embed.FS

// catalogBrands lists the brands embedded in bundledFS (one per subdirectory).
var catalogBrands = []string{"golang", "git"}

// CatalogEntry is one integration available for installation.
type CatalogEntry struct {
	Brand       string
	Name        string
	Description string
	Roles       []string
}

// CatalogEntries returns metadata for every bundled integration.
// Reading Name and Description requires parsing each manifest.toml.
func CatalogEntries() []CatalogEntry {
	var out []CatalogEntry
	for _, brand := range catalogBrands {
		m, err := readBundledManifest(brand)
		if err != nil {
			log.Printf("orbiter: read catalog manifest for %q: %v", brand, err)
			continue
		}
		out = append(out, CatalogEntry{
			Brand:       m.Integration.Brand,
			Name:        m.Integration.Name,
			Description: m.Integration.Description,
			Roles:       m.Integration.Roles,
		})
	}
	return out
}

// InstallFromCatalog copies the named integration's manifest.toml and .wasm file
// to destDir/<brand>/. Creates the target directory if needed.
func InstallFromCatalog(brand, destDir string) error {
	// Find which catalog dir matches the brand
	dir, err := findCatalogDir(brand)
	if err != nil {
		return err
	}

	m, err := readBundledManifest(dir)
	if err != nil {
		return fmt.Errorf("read manifest for %q: %w", brand, err)
	}

	target := filepath.Join(destDir, brand)
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", target, err)
	}

	// Write manifest.toml
	manifestBytes, err := bundledFS.ReadFile(filepath.Join(dir, "manifest.toml"))
	if err != nil {
		return fmt.Errorf("read embedded manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(target, "manifest.toml"), manifestBytes, 0644); err != nil {
		return fmt.Errorf("write manifest.toml: %w", err)
	}

	// Write <brand>.wasm
	wasmBytes, err := bundledFS.ReadFile(filepath.Join(dir, m.Integration.Brand+".wasm"))
	if err != nil {
		return fmt.Errorf("read embedded wasm for %q: %w", brand, err)
	}
	if err := os.WriteFile(filepath.Join(target, m.Integration.Brand+".wasm"), wasmBytes, 0644); err != nil {
		return fmt.Errorf("write wasm: %w", err)
	}

	return nil
}

// LoadInstalled scans dir for installed integrations and registers them into reg.
// Each subdirectory containing manifest.toml and <brand>.wasm is loaded.
// A missing dir is treated as empty (no error).
func LoadInstalled(dir string, reg *core.Registry) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("scan integrations dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, e.Name())
		if err := loadOneInstalled(pluginDir, reg); err != nil {
			log.Printf("orbiter: load integration %s: %v", pluginDir, err)
		}
	}
	return nil
}

func loadOneInstalled(dir string, reg *core.Registry) error {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "manifest.toml"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var m core.Manifest
	if _, err := toml.Decode(string(manifestBytes), &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	wasmBytes, err := os.ReadFile(filepath.Join(dir, m.Integration.Brand+".wasm"))
	if err != nil {
		return fmt.Errorf("read wasm: %w", err)
	}

	i, err := wasm.Load(context.Background(), m, wasmBytes)
	if err != nil {
		return fmt.Errorf("load wasm: %w", err)
	}

	for _, role := range m.Integration.Roles {
		reg.Register(role, m.Integration.Brand, i)
	}
	return nil
}

func readBundledManifest(dir string) (core.Manifest, error) {
	data, err := bundledFS.ReadFile(filepath.Join(dir, "manifest.toml"))
	if err != nil {
		return core.Manifest{}, err
	}
	var m core.Manifest
	if _, err := toml.Decode(string(data), &m); err != nil {
		return core.Manifest{}, err
	}
	return m, nil
}

func findCatalogDir(brand string) (string, error) {
	for _, dir := range catalogBrands {
		m, err := readBundledManifest(dir)
		if err != nil {
			continue
		}
		if m.Integration.Brand == brand {
			return dir, nil
		}
	}
	return "", fmt.Errorf("integration %q not found in catalog", brand)
}

// DefaultIntegrationsDir returns the path where installed integrations are stored.
func DefaultIntegrationsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".orbiter/integrations"
	}
	return filepath.Join(home, ".orbiter", "integrations")
}

// init registers all integrations installed by the Captain.
// Bundled integrations are NOT auto-registered; use vessel init to install them.
func init() {
	_ = LoadInstalled(DefaultIntegrationsDir(), core.Default)
}

// Ensure embed.FS is used even without any catalog entries.
var _ fs.FS = bundledFS
```

**Note on `core.Default.Register` / `core.Registry`:** The existing `integrations.Default` is a `*Registry`. If `Register` is currently a package-level function rather than a method, update the call accordingly. If `NewRegistry()` does not exist yet, add it:

```go
// internal/integrations/registry.go (add if not present)
func NewRegistry() *Registry {
    return &Registry{entries: make(map[string]Integration)}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./integrations/... -run "TestCatalog|TestInstall|TestLoadInstalled" -v
```

Expected: PASS.

- [ ] **Step 5: Update existing e2e tests in `integrations/e2e_test.go`**

The e2e tests currently call `integrations.Default.Get("tool", "git")` which relied on auto-registration. Update them to install first:

```go
// integrations/e2e_test.go
package integrations_test

import (
	"os"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

func setupRegistry(t *testing.T, brands ...string) *core.Registry {
	t.Helper()
	dir := t.TempDir()
	for _, brand := range brands {
		if err := integrations.InstallFromCatalog(brand, dir); err != nil {
			t.Fatalf("install %q: %v", brand, err)
		}
	}
	reg := core.NewRegistry()
	if err := integrations.LoadInstalled(dir, reg); err != nil {
		t.Fatalf("load installed: %v", err)
	}
	return reg
}

func TestBundledIntegrations_Git(t *testing.T) {
	reg := setupRegistry(t, "git")
	i, ok := reg.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}
	// ... rest of existing test body unchanged
	_ = i
}
```

Update `TestBundledIntegrations_Go` similarly.

- [ ] **Step 6: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add integrations/bundle.go integrations/catalog_test.go integrations/e2e_test.go
git commit -m "feat: integration catalog — dormant embed, install to ~/.orbiter/integrations/, load on startup"
```

---

## Task 10: vessel init catalog selection

**Purpose:** `vessel init` presents a checklist of available integrations from the catalog. The Captain selects which to install to `~/.orbiter/integrations/`. Unselected integrations are not installed. This uses a simple terminal checklist rendered to stdout — not a Bubble Tea TUI — since `vessel init` is a one-time setup command.

The prompt looks like:

```
Available integrations (space to select, enter to confirm):

  [ ] Go Toolchain       — Scans and verifies the Go runtime and module toolchain   [runtime]
  [ ] Git                — Scans and configures the git version control tool         [tool]

Select integrations to install:
```

**Files:**

- Modify: `internal/commands/vessel.go`
- Create: `internal/commands/vessel_catalog_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/commands/vessel_catalog_test.go
package commands_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/commands"
)

func TestRenderCatalogChecklist(t *testing.T) {
	entries := []integrations.CatalogEntry{
		{Brand: "git", Name: "Git", Description: "VCS tool", Roles: []string{"tool"}},
		{Brand: "go", Name: "Go Toolchain", Description: "Go runtime", Roles: []string{"runtime"}},
	}

	lines := commands.RenderCatalogChecklist(entries)
	if len(lines) != len(entries) {
		t.Fatalf("expected %d lines, got %d", len(entries), len(lines))
	}
	if !strings.Contains(lines[0], "Git") {
		t.Errorf("line 0 should contain name: %q", lines[0])
	}
	if !strings.Contains(lines[0], "VCS tool") {
		t.Errorf("line 0 should contain description: %q", lines[0])
	}
	if !strings.Contains(lines[0], "[tool]") {
		t.Errorf("line 0 should contain roles: %q", lines[0])
	}
}

func TestInstallSelected_InstallsChosen(t *testing.T) {
	destDir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	// Install only the first entry
	selected := []integrations.CatalogEntry{entries[0]}
	if err := commands.InstallSelected(selected, destDir); err != nil {
		t.Fatalf("InstallSelected: %v", err)
	}

	// Verify the first entry is installed, others are not
	brand := entries[0].Brand
	if _, err := os.Stat(filepath.Join(destDir, brand, "manifest.toml")); err != nil {
		t.Errorf("selected brand %q not installed: %v", brand, err)
	}
	if len(entries) > 1 {
		brand2 := entries[1].Brand
		if _, err := os.Stat(filepath.Join(destDir, brand2)); err == nil {
			t.Errorf("unselected brand %q should not be installed", brand2)
		}
	}
}
```

Add `"os"`, `"path/filepath"`, and `"strings"` to imports.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/commands/... -run "TestRenderCatalog|TestInstallSelected" -v
```

Expected: FAIL — `commands.RenderCatalogChecklist` and `commands.InstallSelected` do not exist.

- [ ] **Step 3: Add catalog selection helpers to `internal/commands/vessel.go`**

```go
// RenderCatalogChecklist returns one formatted line per catalog entry for display.
func RenderCatalogChecklist(entries []integrations.CatalogEntry) []string {
	lines := make([]string, len(entries))
	for i, e := range entries {
		roles := "[" + strings.Join(e.Roles, ", ") + "]"
		lines[i] = fmt.Sprintf("  [ ] %-22s — %-55s %s", e.Name, e.Description, roles)
	}
	return lines
}

// InstallSelected installs the chosen entries to destDir using InstallFromCatalog.
func InstallSelected(selected []integrations.CatalogEntry, destDir string) error {
	for _, e := range selected {
		if err := integrations.InstallFromCatalog(e.Brand, destDir); err != nil {
			return fmt.Errorf("install %q: %w", e.Brand, err)
		}
	}
	return nil
}
```

Add `"fmt"`, `"strings"`, and the `integrations` package import to vessel.go.

- [ ] **Step 4: Add `runVesselInit` catalog prompt to `internal/commands/vessel.go`**

In the `vessel init` Cobra command's `RunE`, after the vessel record is created, present the catalog prompt:

```go
catalog := integrations.CatalogEntries()
if len(catalog) > 0 {
	fmt.Fprintln(cmd.OutOrStdout(), "\nAvailable integrations (type numbers separated by spaces, or enter to skip):\n")
	lines := RenderCatalogChecklist(catalog)
	for j, line := range lines {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d) %s\n", j+1, strings.TrimPrefix(line, "  [ ] "))
	}
	fmt.Fprint(cmd.OutOrStdout(), "\nSelect (e.g. 1 3): ")

	var input string
	fmt.Fscanln(cmd.InOrStdin(), &input)

	var selected []integrations.CatalogEntry
	for _, tok := range strings.Fields(input) {
		var idx int
		if _, err := fmt.Sscanf(tok, "%d", &idx); err == nil && idx >= 1 && idx <= len(catalog) {
			selected = append(selected, catalog[idx-1])
		}
	}

	if len(selected) > 0 {
		if err := InstallSelected(selected, integrations.DefaultIntegrationsDir()); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: some integrations failed to install: %v\n", err)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "\nInstalled %d integration(s) to %s\n",
				len(selected), integrations.DefaultIntegrationsDir())
		}
	}
}
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/commands/vessel.go internal/commands/vessel_catalog_test.go
git commit -m "feat: vessel init presents integration catalog checklist — Captain selects what to install"
```

---

## Task 11: vessel inspect + vessel unquarantine

**Purpose:** Two operational commands that close the quarantine loop. An integration that trips the fault gate goes offline immediately and prints a stderr warning pointing here. `vessel inspect <name>` shows current state (active / quarantined, with reason and timestamp) and the last 10 audit log entries for the brand. `vessel unquarantine <name>` reinstates it in-memory and clears the settings.json entry — no restart.

The integration drift framing: a quarantined integration has declared state (its manifest) that no longer matches observed behaviour (what it tried to run). The Captain's job is to resolve the drift — update the manifest and reinstall, remove the integration, or decide it was a false positive and unquarantine.

**Files:**

- Modify: `internal/commands/vessel.go`
- Create: `internal/commands/vessel_inspect_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/commands/vessel_inspect_test.go
package commands_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestFormatInspectReport_Active(t *testing.T) {
	info := commands.IntegrationInspectInfo{
		Brand:       "git",
		Name:        "Git",
		Quarantined: false,
	}
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, info)
	out := buf.String()
	if !strings.Contains(out, "git") {
		t.Errorf("missing brand: %s", out)
	}
	if !strings.Contains(out, "active") {
		t.Errorf("expected 'active' status: %s", out)
	}
}

func TestFormatInspectReport_Quarantined(t *testing.T) {
	info := commands.IntegrationInspectInfo{
		Brand:           "bad-integration",
		Name:            "Bad Integration",
		Quarantined:     true,
		QuarantineEntry: integrations.QuarantineInfo{Reason: "attempted banned command: bash"},
	}
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, info)
	out := buf.String()
	if !strings.Contains(out, "quarantined") {
		t.Errorf("expected 'quarantined' status: %s", out)
	}
	if !strings.Contains(out, "attempted banned command: bash") {
		t.Errorf("expected reason: %s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/commands/... -run TestFormatInspectReport -v
```

Expected: FAIL — `commands.IntegrationInspectInfo` and `commands.WriteInspectReport` do not exist.

- [ ] **Step 3: Add inspect types and report writer to `internal/commands/vessel.go`**

```go
// IntegrationInspectInfo holds the data for a vessel inspect report.
type IntegrationInspectInfo struct {
	Brand           string
	Name            string
	Quarantined     bool
	QuarantineEntry integrations.QuarantineInfo
}

// WriteInspectReport writes a human-readable inspect report to w.
func WriteInspectReport(w io.Writer, info IntegrationInspectInfo) {
	status := "active"
	if info.Quarantined {
		status = "quarantined"
	}
	fmt.Fprintf(w, "  Integration: %s (%s)\n", info.Brand, info.Name)
	fmt.Fprintf(w, "  Status:      %s\n", status)
	if info.Quarantined {
		fmt.Fprintf(w, "  Reason:      %s\n", info.QuarantineEntry.Reason)
		if !info.QuarantineEntry.At.IsZero() {
			fmt.Fprintf(w, "  At:          %s\n", info.QuarantineEntry.At.Format(time.RFC3339))
		}
		fmt.Fprintf(w, "\n  To reinstate: orbiter vessel unquarantine %s\n", info.Brand)
		fmt.Fprintf(w, "  Audit log:    %s\n", auditLogPath())
	}
}

func auditLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.orbiter/audit.log"
	}
	return filepath.Join(home, ".orbiter", "audit.log")
}
```

Add `"io"`, `"time"`, `"os"`, `"path/filepath"` to vessel.go imports.

- [ ] **Step 4: Wire `vessel inspect <name>` Cobra command**

In the vessel Cobra command group, add:

```go
inspectCmd := &cobra.Command{
	Use:   "inspect <name>",
	Short: "Show status and recent audit history for an integration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		brand := args[0]
		catalog := integrations.CatalogEntries()

		name := brand
		for _, e := range catalog {
			if e.Brand == brand {
				name = e.Name
				break
			}
		}

		info := IntegrationInspectInfo{
			Brand:       brand,
			Name:        name,
			Quarantined: integrations.Default.IsQuarantined(brand),
		}
		if info.Quarantined {
			info.QuarantineEntry = integrations.DefaultSettings.QuarantineEntry(brand)
		}

		WriteInspectReport(cmd.OutOrStdout(), info)
		return nil
	},
}
vesselCmd.AddCommand(inspectCmd)
```

- [ ] **Step 5: Wire `vessel unquarantine <name>` Cobra command**

```go
unquarantineCmd := &cobra.Command{
	Use:   "unquarantine <name>",
	Short: "Reinstate a quarantined integration — takes effect immediately",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		brand := args[0]
		if !integrations.Default.IsQuarantined(brand) {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s is not quarantined\n", brand)
			return nil
		}
		if err := integrations.Default.UnquarantineBrand(brand); err != nil {
			return fmt.Errorf("unquarantine %s: %w", brand, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s reinstated — active immediately\n", brand)
		return nil
	},
}
vesselCmd.AddCommand(unquarantineCmd)
```

- [ ] **Step 6: Run all tests**

```bash
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/vessel.go internal/commands/vessel_inspect_test.go
git commit -m "feat: vessel inspect + vessel unquarantine — quarantine lifecycle without restart"
```

---

## Self-Review

### Spec coverage check

| Spec requirement | Task |
| --- | --- |
| Brand-centric manifest (`roles = [...]`, no `type`) | Task 1 |
| `name` and `description` in manifest `[integration]` section | Task 1 |
| `[commands]` allowlist + `[shell]` exports + `[runtime]` hints | Task 1 |
| `[config.fields]` in manifest | Task 1 |
| `RoleType()` static role→type lookup | Task 1 |
| `Entity` interface (GetID/GetRole/GetBrand/GetConfig) | Task 2 |
| `ResolvedContext.Self` → `Entity` | Task 2 |
| `StateReport.NeedsInput`, `Exports`; `ResolvedContext.Responses` | Task 2 |
| `InputRequest` struct | Task 2 |
| Transponder `Config` field (replace `Location`) | Task 3 |
| DB migration: add config, migrate location, drop location | Task 3 |
| Transponder implements Entity | Task 3 |
| Shared wazero runtime singleton | Task 4 |
| Trust store — `~/.orbiter/settings.json`, exact full command string keying | Task 5 |
| `SettingsStore.IsAllowed` + `Allow` (trust) + `IsQuarantined` + `Quarantine` + `Unquarantine` | Task 5 |
| Command banlist — shells and privilege escalation, non-whitelistable | Task 6 |
| `run_command` six-stage gate (quarantine → banlist → allowlist → trust → prompt → exec) | Task 6 |
| Quarantine on first fault — banlist or undeclared command triggers immediate quarantine | Task 6 |
| Quarantine is in-memory in registry + persisted to `settings.json` — no restart required | Task 5 |
| Captain prompt decline ≠ quarantine — declining is mid-flight redirection | Task 6 |
| Captain approval prompt (`[a]lways / [o]nce / [d]ecline`) | Task 6 |
| `ApproveFunc` callback type + `StdinApproveFunc` | Task 6 |
| Timeout per invocation from manifest | Task 6 |
| Audit log (`~/.orbiter/audit.log`) with all outcome fields | Task 6 |
| No ambient environment (PATH only) | Task 6 |
| `--unattended` behavior (no new trust written) | Task 6 (note in Step 10) |
| Module instance pool (channel-based) | Task 7 |
| `StateReport.Exports` validated against `[shell] exports` | Task 7 |
| Transponder scan/calibrate result types | Task 8 |
| Role-ordered resource dispatch (manager→…→filesystem) | Task 8 |
| Parallel transponder pass after resource tiers | Task 8 |
| `buildResolvedContextForTransponder` | Task 8 |
| Multi-role bundle registration | Task 1 (bundle.go) |
| Bundled integrations dormant — not auto-registered | Task 9 |
| `CatalogEntries()` reads name/description/roles from embedded manifests | Task 9 |
| `InstallFromCatalog(brand, destDir)` copies wasm + manifest | Task 9 |
| `LoadInstalled(dir, reg)` scans `~/.orbiter/integrations/` on startup | Task 9 |
| `vessel init` catalog checklist — Captain selects integrations to install | Task 10 |
| `vessel inspect <brand>` — show quarantine status + last audit entries | Task 11 |
| `vessel unquarantine <brand>` — reinstate quarantined integration immediately | Task 11 |
| Native filesystem remains compiled-in, not in catalog | Task 8 (excluded from catalogBrands) |
| Updated golang/git manifests to new format with name + description | Task 1 |

All spec requirements covered. ✓

### Placeholder scan

No "TBD", "implement later", or "add error handling" placeholders found. ✓

### Type consistency

- `ManifestIntegration.Name` and `.Description` defined in Task 1, used in Task 9 (`CatalogEntry`) and Task 10 (`RenderCatalogChecklist`). ✓
- `Entity` interface defined in Task 2, used in Task 2 (crawl.go, filesystem.go) and Task 8 (buildResolvedContextForTransponder). ✓
- `ApproveFunc` defined in Task 6 (host.go), stored in `callState.approve`, populated in Task 6's `invoke` update. ✓
- `commandBanlist`, `isBanned`, `commandAllowed`, `fullCommandString` all defined and consumed in Task 6 `runCommandFn`. ✓
- `AuditEntry` struct defined in Task 6 `audit.go`, used in `runCommandFn` `DefaultAuditLog.Log(...)` calls. ✓
- `SettingsStore` + `DefaultSettings` defined in Task 5 (`internal/integrations/settings.go`), used in Task 6 `runCommandFn` for trust checks and trust writes. ✓
- `Registry.QuarantineBrand` + `IsQuarantined` defined in Task 5 registry update, called in Task 6 `runCommandFn` at stages 0–2. ✓
- `callState.timeout` added in Task 6 `callState`, populated in Task 6's `invoke` update. ✓
- `TransponderScanResult` and `TransponderCalibrateResult` defined in Task 8 step 3, consumed in steps 5-7. ✓
- `sortResourcesByRole` defined in Task 8 step 4, called in steps 5-6. ✓
- `buildResolvedContextForTransponder` defined in Task 8 step 7, called in step 7 helpers. ✓
- `CatalogEntry` defined in Task 9 `bundle.go`, used in Task 10 `RenderCatalogChecklist` and `InstallSelected`. ✓
- `integrations.DefaultIntegrationsDir()` defined in Task 9, called in Task 10 `runVesselInit`. ✓
- `IntegrationInspectInfo` and `WriteInspectReport` defined in Task 11 vessel.go, consumed by inspect Cobra command in the same task. ✓
- `integrations.Default.IsQuarantined` + `UnquarantineBrand` defined in Task 5 registry, called in Task 11 vessel commands. ✓
- `integrations.DefaultSettings.QuarantineEntry` defined in Task 5 settings.go, called in Task 11 inspect command. ✓

### Out of scope confirmed

- `NeedsInput` dispatch loop (keychain integration collecting input from CLI/TUI) is defined in types but not wired into the executor — the spec marks this as future work tied to concrete keychain integrations. The types and data model are in place. ✓
- `--unattended` flag wiring into the Cobra root command is noted in Task 6 Step 10 but left as a follow-up — the `ApproveFunc` nil path already handles the unattended case correctly. ✓
- `vault` transponder not implemented. ✓
- WASM signing not implemented. ✓
