# GitHub Integration (Standalone) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the `github` integration covering all three roles — `tool` (gh CLI), `remote` (GitHub repo reachability), and `agent` (GH_TOKEN via OAuth) — in a single brand with a multi-role manifest.

**Architecture:** A single Rust `cdylib` (`github.wasm`) handles all three roles by reading the `role` field from the resolved context to branch behavior. The manifest declares `roles = ["tool", "remote", "agent"]`. The `agent` role owns the OAuth flow: `gh auth login` is the calibrate path when the captain is not authenticated. `GH_TOKEN` is exported to the shell via `[shell] exports`. Downstream integrations that need GitHub auth declare `agent = ["github"]` in their `[dependencies.transponders]`.

**Tech Stack:** Rust `wasm32-unknown-unknown`, `serde` + `serde_json`, `gh` CLI, `go test ./integrations/...`

## Global Constraints

- Brand: `github` — a single integration serving three roles (tool, remote, agent)
- Roles: `["tool", "remote", "agent"]` declared in `[integration] roles`
- Target: `wasm32-unknown-unknown`
- Role dispatch: the guest reads `role` from the `ResolvedContext` to route each handler
- OAuth flow: `gh auth login` — the guest surfaces the command; Orbiter runs it via `run_command`
- GH_TOKEN export: declared in `[shell] exports`; set by calibrate via `NeedsInput` pattern
- `integrations/bundle.go` `//go:embed` must be updated
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`

---

### Task 1: Scaffold and manifest

**Files:**
- Create: `integrations/github/Cargo.toml`
- Create: `integrations/github/generate.go`
- Create: `integrations/github/manifest.toml`
- Create: `integrations/github/src/host.rs`
- Create: `integrations/github/src/lib.rs` (stub — exports all four handlers, panics with "TODO")
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

This task establishes the scaffolding and manifest. The guest stubs return safe zero-value responses so the registry loads successfully.

- [ ] **Step 1: Write the registration test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GitHub_Registration(t *testing.T) {
	reg := setupBundleRegistry(t)

	t.Run("tool_role", func(t *testing.T) {
		_, ok := reg.Get("tool", "github")
		if !ok {
			t.Fatal("github not registered as tool")
		}
	})

	t.Run("remote_role", func(t *testing.T) {
		_, ok := reg.Get("remote", "github")
		if !ok {
			t.Fatal("github not registered as remote")
		}
	})

	t.Run("agent_role", func(t *testing.T) {
		_, ok := reg.Get("agent", "github")
		if !ok {
			t.Fatal("github not registered as agent")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_GitHub_Registration -v
```

Expected: FAIL — "github not registered as tool"

- [ ] **Step 3: Create directory and copy host.rs**

```bash
mkdir -p integrations/github/src
cp integrations/git/src/host.rs integrations/github/src/host.rs
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/github/Cargo.toml`:

```toml
[package]
name = "github"
version = "0.1.0"
edition = "2021"

[lib]
name = "github"
crate-type = ["cdylib"]

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"

[profile.release]
opt-level = "s"
lto = true
strip = true
panic = "abort"
```

- [ ] **Step 5: Write stub lib.rs**

Create `integrations/github/src/lib.rs`:

```rust
mod host;

// Stub: returns safe zero-value responses for all handlers.
// Full implementation follows in subsequent tasks.

#[no_mangle]
pub extern "C" fn detect() {
    let _input = host::read_input();
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    host::write_output(b"{\"present\":false,\"reachable\":false,\"in_path\":false,\"manager\":\"system\"}");
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    host::write_output(b"{\"present\":false,\"reachable\":false,\"in_path\":false,\"manager\":\"system\"}");
}
```

- [ ] **Step 6: Create manifest.toml**

Create `integrations/github/manifest.toml`:

```toml
[integration]
brand = "github"
name = "GitHub"
description = "Manages the GitHub CLI (gh), remote repo access, and GitHub auth token"
roles = ["tool", "remote", "agent"]

[detection]
files = [".git/config"]

[commands]
allowed = ["gh", "git", "which"]
timeout_seconds = 60

[shell]
exports = ["GH_TOKEN"]

[runtime]
pool_size = 4
input_buffer_kb = 16
output_buffer_kb = 16
```

- [ ] **Step 7: Create generate.go**

Create `integrations/github/generate.go`:

```go
package github

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/github.wasm ."
```

- [ ] **Step 8: Update bundle.go and compile stub**

Add `github/github.wasm github/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/github && go generate .
```

- [ ] **Step 9: Run registration test**

```bash
go test ./integrations/... -run TestBundledIntegrations_GitHub_Registration -v
```

Expected: all three role registration subtests pass.

- [ ] **Step 10: Commit stub**

```bash
git add integrations/github/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: scaffold multi-role github integration (stub)"
```

---

### Task 2: tool role — detect and scan

**Files:**
- Modify: `integrations/github/src/lib.rs` (implement detect and scan for tool role)

The `tool/github` role checks whether `gh` is installed and authenticated. Detection uses `.git/config` file presence combined with a `github.com` remote URL.

- [ ] **Step 1: Write tool role tests**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GitHub_Tool(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "github")
	if !ok {
		t.Fatal("github tool not registered")
	}

	t.Run("detect_git_config_github", func(t *testing.T) {
		// The detect handler checks for .git/config — if it also contains
		// "github.com" that strengthens detection, but .git/config alone is enough.
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".git/config": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .git/config")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected suggested resources")
		}
		// Tool role should be first suggestion
		found := false
		for _, r := range report.Resources {
			if r.Role == "tool" && r.Brand == "github" {
				found = true
			}
		}
		if !found {
			t.Error("expected tool/github in suggested resources")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .git/config")
		}
	})

	t.Run("scan_tool", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Tool Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (gh is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./integrations/... -run TestBundledIntegrations_GitHub_Tool -v
```

Expected: detect_git_config_github returns detected=false (stub always returns false).

- [ ] **Step 3: Implement detect and tool scan in lib.rs**

Replace `integrations/github/src/lib.rs` with:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

// ── Types ──────────────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
}

#[derive(Deserialize, Default)]
struct ResolvedContext {
    #[serde(default)]
    role: String,
}

#[derive(Serialize)]
struct SuggestedResource {
    role: String,
    brand: String,
}

#[derive(Serialize)]
struct DetectResult {
    detected: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    resources: Vec<SuggestedResource>,
}

#[derive(Serialize, Default)]
struct StateReport {
    present: bool,
    reachable: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    binary_path: Option<String>,
    in_path: bool,
    manager: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    error: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    observations: Vec<String>,
}

fn write_state(report: StateReport) {
    host::write_output(&serde_json::to_vec(&report).unwrap_or_default());
}

// ── detect ─────────────────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        files: HashMap::new(),
    });
    if !ctx.files.contains_key(".git/config") {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    // Suggest all three roles — the registry will register all three
    let result = DetectResult {
        detected: true,
        resources: vec![
            SuggestedResource { role: "tool".to_string(), brand: "github".to_string() },
            SuggestedResource { role: "remote".to_string(), brand: "github".to_string() },
            SuggestedResource { role: "agent".to_string(), brand: "github".to_string() },
        ],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

// ── initialize / scan ──────────────────────────────────────────────────────
// Dispatches by role field from the context.

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    match ctx.role.as_str() {
        "remote" => scan_remote(),
        "agent"  => scan_agent(),
        _        => scan_tool(),  // "tool" and default
    }
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

// ── tool role ──────────────────────────────────────────────────────────────

fn scan_tool() {
    let binary_path = host::run_command("which", &["gh"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "gh CLI not found in PATH — install from cli.github.com".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("gh", &["--version"]);
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.contains("not logged in") && !auth_status.is_empty();
    let mut observations = vec![version];
    if authenticated {
        observations.push("auth: logged in".to_string());
    } else {
        observations.push("auth: not authenticated — run 'gh auth login'".to_string());
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        binary_path: Some(binary_path),
        in_path: true,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}

// ── remote role ────────────────────────────────────────────────────────────

fn scan_remote() {
    let repo_view = host::run_command("gh", &["repo", "view", "--json", "name,url,visibility"]);
    if repo_view.is_empty() || repo_view.contains("error") || repo_view.contains("not found") {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "no GitHub remote found or not authenticated".to_string(),
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: false,
        manager: "system".to_string(),
        observations: vec![repo_view],
        ..Default::default()
    });
}

// ── agent role ─────────────────────────────────────────────────────────────

fn scan_agent() {
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.is_empty()
        && !auth_status.contains("not logged in")
        && !auth_status.contains("not found");
    if !authenticated {
        write_state(StateReport {
            present: true, // gh is present
            reachable: false, // but not authenticated
            in_path: true,
            manager: "system".to_string(),
            observations: vec![
                "not authenticated — run calibrate to complete gh auth login".to_string(),
            ],
            ..Default::default()
        });
        return;
    }
    let token_check = host::run_command("gh", &["auth", "token"]);
    let has_token = !token_check.is_empty() && !token_check.contains("error");
    write_state(StateReport {
        present: true,
        reachable: has_token,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![
            auth_status,
            if has_token {
                "GH_TOKEN: available".to_string()
            } else {
                "GH_TOKEN: not available".to_string()
            },
        ],
        ..Default::default()
    });
}

// ── calibrate ──────────────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    match ctx.role.as_str() {
        "remote" => calibrate_remote(),
        "agent"  => calibrate_agent(),
        _        => calibrate_tool(),
    }
}

fn calibrate_tool() {
    // Tool calibrate: verify gh is present and auth is valid
    scan_tool();
}

fn calibrate_remote() {
    // Remote calibrate: verify repo is reachable; surface clone command if not
    let repo_view = host::run_command("gh", &["repo", "view", "--json", "name,url"]);
    if repo_view.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            observations: vec![
                "calibrated: no GitHub remote found".to_string(),
                "hint: run 'gh repo clone <owner>/<repo>' to establish the remote".to_string(),
            ],
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: false,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", repo_view)],
        ..Default::default()
    });
}

fn calibrate_agent() {
    // Agent calibrate: run gh auth login if not authenticated.
    // gh auth login is an interactive command — we surface it rather than running it
    // blindly. When the captain is already logged in, we emit GH_TOKEN via exports.
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.is_empty()
        && !auth_status.contains("not logged in")
        && !auth_status.contains("not found");

    if !authenticated {
        // Surface the login command — the host will prompt the captain to run it
        write_state(StateReport {
            present: true,
            reachable: false,
            in_path: true,
            manager: "system".to_string(),
            observations: vec![
                "not authenticated".to_string(),
                "run: gh auth login --web  (opens browser)".to_string(),
                "or:  gh auth login --with-token  (paste a PAT)".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let token = host::run_command("gh", &["auth", "token"]);
    if token.is_empty() {
        write_state(StateReport {
            present: true,
            reachable: true,
            in_path: true,
            manager: "system".to_string(),
            observations: vec!["authenticated but could not retrieve token".to_string()],
            ..Default::default()
        });
        return;
    }

    // Emit GH_TOKEN as a shell export. The host reads the exports field
    // from StateReport and emits "export GH_TOKEN=<value>" to the shell
    // eval output. The export key must be declared in [shell] exports in manifest.toml.
    #[derive(Serialize, Default)]
    struct AgentReport {
        present: bool,
        reachable: bool,
        in_path: bool,
        manager: String,
        observations: Vec<String>,
        exports: HashMap<String, String>,
    }
    let mut exports = HashMap::new();
    exports.insert("GH_TOKEN".to_string(), token);
    let report = AgentReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec!["calibrated: GH_TOKEN set".to_string()],
        exports,
    };
    host::write_output(&serde_json::to_vec(&report).unwrap_or_default());
}
```

- [ ] **Step 4: Rebuild and run tool tests**

```bash
cd integrations/github && go generate .
go test ./integrations/... -run TestBundledIntegrations_GitHub_Tool -v
```

Expected: all tool subtests pass.

- [ ] **Step 5: Commit**

```bash
git add integrations/github/src/lib.rs integrations/github/github.wasm integrations/e2e_test.go
git commit -m "feat: implement github tool role (detect, scan)"
```

---

### Task 3: remote role — repo reachability

**Files:**
- Modify: `integrations/e2e_test.go` (add remote role tests)
- (No lib.rs changes — remote role is already implemented in Task 2's lib.rs)

The remote role was implemented in `scan_remote()` in Task 2. This task adds dedicated tests for it.

- [ ] **Step 1: Write remote role tests**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GitHub_Remote(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("remote", "github")
	if !ok {
		t.Fatal("github remote not registered")
	}

	t.Run("scan_remote", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{
			Role: "remote",
		})
		t.Logf("Remote Scan: %+v", report)
		// Remote scan is only meaningful in a git project with a github.com remote.
		// In CI / generic checkout, it may return present=false — that's valid.
		// We only assert the shape.
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

Note: `core.ResolvedContext.Role` field must exist for role dispatch to work. Check the struct definition in `internal/integrations/types.go` or similar. If `Role` is not a field on `ResolvedContext`, read the struct and add it before running this test.

- [ ] **Step 2: Verify ResolvedContext has Role field**

```bash
grep -r "ResolvedContext" internal/integrations/ | head -20
```

If `Role` field is missing from `ResolvedContext`, add it:

```go
// In internal/integrations/types.go (or wherever ResolvedContext is defined):
type ResolvedContext struct {
    // ... existing fields ...
    Role string `json:"role,omitempty"`
}
```

- [ ] **Step 3: Run remote tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_GitHub_Remote -v
```

Expected: scan_remote passes (shape assertion only — repo may not be present in test environment).

- [ ] **Step 4: Commit**

```bash
git add integrations/e2e_test.go
git commit -m "test: add github remote role scan test"
```

---

### Task 4: agent role — OAuth flow and GH_TOKEN

**Files:**
- Modify: `integrations/e2e_test.go` (add agent role tests)
- (No lib.rs changes — agent role is already implemented in Task 2's lib.rs)

- [ ] **Step 1: Write agent role tests**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GitHub_Agent(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "github")
	if !ok {
		t.Fatal("github agent not registered")
	}

	t.Run("scan_agent", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{
			Role: "agent",
		})
		t.Logf("Agent Scan: %+v", report)
		// Agent scan reports present=true if gh binary exists, reachable=true if authenticated
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// Binary should be in PATH on this machine
		if !report.Present {
			t.Error("expected present=true (gh is installed)")
		}
	})

	t.Run("calibrate_agent_authenticated", func(t *testing.T) {
		// This test only runs a meaningful assertion if gh is authenticated.
		// In CI, gh auth login is performed via GH_TOKEN env var.
		report := i.Calibrate(core.ResolvedContext{
			Role: "agent",
		})
		t.Logf("Agent Calibrate: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// If authenticated, GH_TOKEN export should be set
		if report.Reachable {
			if report.Exports["GH_TOKEN"] == "" {
				t.Error("expected GH_TOKEN in exports when authenticated")
			}
		}
	})
}
```

Note: `report.Exports` requires a map on `StateReport`. Check the struct definition. If `Exports` is not present, see Step 2.

- [ ] **Step 2: Verify StateReport has Exports field**

```bash
grep -r "Exports\|exports" internal/integrations/ | grep -v "_test" | head -20
```

If `Exports` is not present on `StateReport`:

```go
// In internal/integrations/types.go or state.go:
type StateReport struct {
    // ... existing fields ...
    Exports map[string]string `json:"exports,omitempty"`
}
```

The host already reads exports during `FilterExports` in `internal/wasm/loader.go`. Confirm that the `Exports` field is parsed from the guest output correctly.

- [ ] **Step 3: Run agent tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_GitHub_Agent -v
```

Expected: scan_agent passes. calibrate_agent_authenticated passes if gh is authenticated (in CI, this may require `GH_TOKEN` env var to be set).

- [ ] **Step 4: Run full integration suite**

```bash
go test ./integrations/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add integrations/e2e_test.go
git commit -m "test: add github agent role tests (OAuth flow, GH_TOKEN export)"
```

---

### Task 5: Integration documentation

**Files:**
- Modify: `docs/integrations.md` (add multi-role manifest pattern section)

- [ ] **Step 1: Add multi-role manifest pattern to docs/integrations.md**

Find the manifest documentation section in `docs/integrations.md` and add:

```markdown
### Multi-Role Integrations

A single brand can serve multiple roles by listing them in `[integration] roles`:

```toml
[integration]
brand = "github"
name = "GitHub"
roles = ["tool", "remote", "agent"]
```

The registry registers the integration once per declared role. When a role-specific
handler is invoked, the host passes `role` in the `ResolvedContext` so the guest can
dispatch to role-specific behavior:

```rust
let role = ctx.role.as_str();
match role {
    "remote" => scan_remote(),
    "agent"  => scan_agent(),
    _        => scan_tool(),
}
```

**Agent role exports:** Agents that need to export credentials to the shell declare
the export key in `[shell] exports` in the manifest and write them to the `exports`
map in `StateReport`. The host emits `export KEY=VALUE` to the shell eval output
during `jump`:

```toml
[shell]
exports = ["GH_TOKEN"]
```

Other integrations that consume this credential declare:

```toml
[dependencies]
  [dependencies.transponders]
  agent = ["github"]
```
```

- [ ] **Step 2: Commit docs**

```bash
git add docs/integrations.md
git commit -m "docs: add multi-role integration pattern with github as reference"
```
