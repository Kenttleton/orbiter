# Google Integration (Standalone) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `google-auth` (Rust, `agent` role) and `google-drive` (Rust, `remote` role), establishing the `<brand>-<application>` naming convention for the Google suite and documenting the shared auth dependency pattern for future Google integrations.

**Architecture:** Two separate Rust integrations with separate WASM modules. `google-auth` is the auth transponder: it assists captains through Google OAuth via the Drive desktop app or `gcloud auth login`. No tokens are stored by Orbiter — auth state is owned by the Drive app or gcloud credential store. `google-drive` checks that the Drive desktop app is installed and the sync folder is present at the stored path; auth is delegated to the `google-auth` transponder dependency.

**Tech Stack:** Rust `wasm32-unknown-unknown`, `serde` + `serde_json`, Google Drive desktop app, `gcloud` CLI (optional), `go test ./integrations/...`

## Global Constraints

- Brand naming: `google-auth`, `google-drive` (hyphens allowed in brand names)
- Go package names in `generate.go` must be valid identifiers: `package googleauth`, `package googledrive`
- Directory names: `integrations/google-auth/`, `integrations/google-drive/`
- Target: `wasm32-unknown-unknown`
- No token storage — Orbiter assists auth, not stores credentials
- `google-drive` declares `agent = ["google-auth"]` in `[dependencies.transponders]`
- `integrations/bundle.go` `//go:embed` must be updated for both integrations
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`

---

### Task 1: agent/google-auth

**Files:**
- Create: `integrations/google-auth/Cargo.toml`
- Create: `integrations/google-auth/generate.go`
- Create: `integrations/google-auth/manifest.toml`
- Create: `integrations/google-auth/src/host.rs`
- Create: `integrations/google-auth/src/lib.rs`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GoogleAuth(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "google-auth")
	if !ok {
		t.Fatal("google-auth integration not registered")
	}

	t.Run("detect_drive_app_darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Google Drive app detection only on darwin")
		}
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		// Detection requires the Drive app to be installed — may not be in CI
		t.Logf("Detect (darwin): %+v", report)
		if report.Detected && len(report.Resources) > 0 {
			if report.Resources[0].Role != "agent" {
				t.Errorf("expected role=agent, got %q", report.Resources[0].Role)
			}
			if report.Resources[0].Brand != "google-auth" {
				t.Errorf("expected brand=google-auth, got %q", report.Resources[0].Brand)
			}
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_GoogleAuth -v
```

Expected: FAIL — "google-auth integration not registered"

- [ ] **Step 3: Create directory and copy host.rs**

```bash
mkdir -p integrations/google-auth/src
cp integrations/git/src/host.rs integrations/google-auth/src/host.rs
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/google-auth/Cargo.toml`:

```toml
[package]
name = "google_auth"
version = "0.1.0"
edition = "2021"

[lib]
name = "google_auth"
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

- [ ] **Step 5: Write lib.rs**

Create `integrations/google-auth/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};

#[derive(Deserialize, Default)]
struct Platform {
    #[serde(default)]
    os: String,
}

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    platform: Platform,
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

fn drive_app_installed(os: &str) -> bool {
    match os {
        "darwin" => {
            let result = host::run_command("stat", &["/Applications/Google Drive.app"]);
            !result.is_empty() && !result.contains("No such file")
        }
        "linux" => {
            // Google Drive has no official Linux app; check for gcloud as a proxy
            let gcloud = host::run_command("which", &["gcloud"]);
            !gcloud.is_empty()
        }
        _ => false,
    }
}

fn gcloud_authenticated() -> bool {
    let accounts = host::run_command("gcloud", &["auth", "list", "--format=value(account)"]);
    !accounts.is_empty() && !accounts.contains("Listed 0 items")
}

fn drive_app_authenticated(os: &str) -> bool {
    // The Drive desktop app manages its own auth state. On macOS we can check
    // whether it is running (which implies it is signed in).
    if os == "darwin" {
        let running = host::run_command("pgrep", &["-x", "Google Drive"]);
        return !running.is_empty();
    }
    false
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    let os = &ctx.platform.os;
    let app_installed = drive_app_installed(os);
    let gcloud_present = {
        let p = host::run_command("which", &["gcloud"]);
        !p.is_empty()
    };
    if !app_installed && !gcloud_present {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "agent".to_string(),
            brand: "google-auth".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    let os = &ctx.platform.os;

    let app_installed = drive_app_installed(os);
    let gcloud_path = host::run_command("which", &["gcloud"]);
    let gcloud_present = !gcloud_path.is_empty();

    if !app_installed && !gcloud_present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "Google Drive app not installed and gcloud not found — install from drive.google.com or cloud.google.com/sdk".to_string(),
            ..Default::default()
        });
        return;
    }

    let mut observations = Vec::new();
    let mut authenticated = false;

    if app_installed {
        observations.push("Google Drive app: installed".to_string());
        if drive_app_authenticated(os) {
            authenticated = true;
            observations.push("Drive app: running (signed in)".to_string());
        } else {
            observations.push("Drive app: not running — open Google Drive to sign in".to_string());
        }
    }

    if gcloud_present {
        observations.push(format!("gcloud: {}", gcloud_path));
        if gcloud_authenticated() {
            authenticated = true;
            let account = host::run_command("gcloud", &["config", "get-value", "account"]);
            observations.push(format!("gcloud account: {}", account));
        } else {
            observations.push("gcloud: not authenticated — run 'gcloud auth login'".to_string());
        }
    }

    write_state(StateReport {
        present: true,
        reachable: authenticated,
        binary_path: if gcloud_present { Some(gcloud_path) } else { None },
        in_path: gcloud_present,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    let os = &ctx.platform.os;

    let app_installed = drive_app_installed(os);
    let gcloud_path = host::run_command("which", &["gcloud"]);
    let gcloud_present = !gcloud_path.is_empty();

    if !app_installed && !gcloud_present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            observations: vec![
                "Google Drive app not installed".to_string(),
                "Install from: https://drive.google.com/drive/downloads".to_string(),
                "or: brew install --cask google-drive".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let mut observations = vec!["calibrated: Google auth".to_string()];
    let mut authenticated = false;

    if app_installed && drive_app_authenticated(os) {
        authenticated = true;
        observations.push("Drive app: signed in".to_string());
    } else if gcloud_present && gcloud_authenticated() {
        authenticated = true;
        let account = host::run_command("gcloud", &["config", "get-value", "account"]);
        observations.push(format!("gcloud: authenticated as {}", account));
    } else {
        // Surface auth steps — do not run them automatically (require browser/biometric)
        if app_installed {
            observations.push("action: open Google Drive app and sign in".to_string());
        }
        if gcloud_present {
            observations.push("action: run 'gcloud auth login' to authenticate gcloud".to_string());
        }
    }

    write_state(StateReport {
        present: true,
        reachable: authenticated,
        binary_path: if gcloud_present { Some(gcloud_path) } else { None },
        in_path: gcloud_present,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
```

- [ ] **Step 6: Create generate.go**

Create `integrations/google-auth/generate.go`:

```go
package googleauth

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/google_auth.wasm google-auth.wasm"
```

Note: the compiled output is `google_auth.wasm` (underscores from the Rust package name). The generate step copies it to `google-auth.wasm` to match the brand naming convention.

- [ ] **Step 7: Create manifest.toml**

Create `integrations/google-auth/manifest.toml`:

```toml
[integration]
brand = "google-auth"
name = "Google Auth"
description = "Guides captains through Google authentication for Google Suite integrations"
roles = ["agent"]

[commands]
allowed = ["gcloud", "stat", "pgrep", "which"]
timeout_seconds = 30

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 8: Update bundle.go embed**

Add `google-auth/google-auth.wasm google-auth/manifest.toml` to the `//go:embed` directive in `integrations/bundle.go`.

- [ ] **Step 9: Compile**

```bash
cd integrations/google-auth && go generate .
```

Expected: `google-auth.wasm` created in `integrations/google-auth/`.

- [ ] **Step 10: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_GoogleAuth -v
```

Expected: all subtests pass (detect skipped if not on darwin or Drive not installed).

- [ ] **Step 11: Commit**

```bash
git add integrations/google-auth/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add agent/google-auth Rust integration"
```

---

### Task 2: remote/google-drive

**Files:**
- Create: `integrations/google-drive/Cargo.toml`
- Create: `integrations/google-drive/generate.go`
- Create: `integrations/google-drive/manifest.toml`
- Create: `integrations/google-drive/src/host.rs`
- Create: `integrations/google-drive/src/lib.rs`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

`google-drive` checks that the Drive app is installed and the sync folder is present at the stored path. It does NOT sync content — sync is the captain's responsibility. Auth is delegated to the `google-auth` transponder.

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_GoogleDrive(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("remote", "google-drive")
	if !ok {
		t.Fatal("google-drive integration not registered")
	}

	t.Run("detect_drive_app", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Google Drive app detection only on darwin")
		}
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		t.Logf("Detect: %+v", report)
		if report.Detected && len(report.Resources) > 0 {
			if report.Resources[0].Role != "remote" {
				t.Errorf("expected role=remote, got %q", report.Resources[0].Role)
			}
			if report.Resources[0].Brand != "google-drive" {
				t.Errorf("expected brand=google-drive, got %q", report.Resources[0].Brand)
			}
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_GoogleDrive -v
```

Expected: FAIL — "google-drive integration not registered"

- [ ] **Step 3: Create directory and copy host.rs**

```bash
mkdir -p integrations/google-drive/src
cp integrations/git/src/host.rs integrations/google-drive/src/host.rs
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/google-drive/Cargo.toml`:

```toml
[package]
name = "google_drive"
version = "0.1.0"
edition = "2021"

[lib]
name = "google_drive"
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

- [ ] **Step 5: Write lib.rs**

Create `integrations/google-drive/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize, Default)]
struct Platform {
    #[serde(default)]
    os: String,
}

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    platform: Platform,
}

#[derive(Deserialize, Default)]
struct ResourceConfig {
    #[serde(default)]
    sync_folder: String,
}

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    platform: Platform,
    #[serde(default)]
    config: ResourceConfig,
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

fn drive_app_path(os: &str) -> Option<String> {
    let path = match os {
        "darwin" => "/Applications/Google Drive.app",
        _ => return None,
    };
    let result = host::run_command("stat", &[path]);
    if result.is_empty() || result.contains("No such file") {
        None
    } else {
        Some(path.to_string())
    }
}

fn sync_folder_exists(path: &str) -> bool {
    if path.is_empty() {
        return false;
    }
    let result = host::run_command("stat", &["-c", "%F", path]);
    !result.is_empty() && !result.contains("No such file")
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "remote".to_string(),
            brand: "google-drive".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        platform: Platform::default(),
        config: ResourceConfig::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "Google Drive app not installed".to_string(),
            ..Default::default()
        });
        return;
    }

    let sync_folder = &ctx.config.sync_folder;
    let folder_exists = sync_folder_exists(sync_folder);
    let mut observations = vec![format!("Drive app: {}", app_path.unwrap())];
    if sync_folder.is_empty() {
        observations.push("sync folder: not configured — add sync_folder to resource config".to_string());
    } else if folder_exists {
        observations.push(format!("sync folder: {} (present)", sync_folder));
    } else {
        observations.push(format!("sync folder: {} (not found)", sync_folder));
    }

    write_state(StateReport {
        present: true,
        reachable: folder_exists || sync_folder.is_empty(),
        in_path: false,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

// calibrate: verify Drive app is installed and sync folder is present.
// Orbiter cannot re-link a GUI sync app — if the folder is missing,
// surface instructions and let the captain act.
#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        platform: Platform::default(),
        config: ResourceConfig::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            observations: vec![
                "Google Drive app not installed".to_string(),
                "Install from: https://drive.google.com/drive/downloads".to_string(),
                "or: brew install --cask google-drive".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let sync_folder = &ctx.config.sync_folder;
    let folder_exists = sync_folder_exists(sync_folder);

    let observations = if sync_folder.is_empty() {
        vec![
            "calibrated: Drive app present".to_string(),
            "action: set sync_folder in resource config to link your Google Drive folder".to_string(),
        ]
    } else if folder_exists {
        vec![
            format!("calibrated: sync folder {} present", sync_folder),
        ]
    } else {
        vec![
            format!("sync folder {} not found", sync_folder),
            "action: open Google Drive app and wait for sync to complete".to_string(),
            format!("expected path: {}", sync_folder),
        ]
    };

    write_state(StateReport {
        present: true,
        reachable: folder_exists || sync_folder.is_empty(),
        in_path: false,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
```

- [ ] **Step 6: Create generate.go**

Create `integrations/google-drive/generate.go`:

```go
package googledrive

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/google_drive.wasm google-drive.wasm"
```

- [ ] **Step 7: Create manifest.toml**

Create `integrations/google-drive/manifest.toml`:

```toml
[integration]
brand = "google-drive"
name = "Google Drive"
description = "Verifies Google Drive app installation and sync folder linkage"
roles = ["remote"]

[dependencies]
  [dependencies.transponders]
  agent = ["google-auth"]

[commands]
allowed = ["stat", "pgrep"]
timeout_seconds = 15

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 8: Update bundle.go embed**

Add `google-drive/google-drive.wasm google-drive/manifest.toml` to the `//go:embed` directive.

- [ ] **Step 9: Compile**

```bash
cd integrations/google-drive && go generate .
```

Expected: `google-drive.wasm` created in `integrations/google-drive/`.

- [ ] **Step 10: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_GoogleDrive -v
```

Expected: all subtests pass (detect conditionally skipped if not on darwin).

- [ ] **Step 11: Run full suite**

```bash
go test ./integrations/... -v
```

Expected: all tests pass.

- [ ] **Step 12: Commit**

```bash
git add integrations/google-drive/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add remote/google-drive Rust integration"
```

---

### Task 3: Document the Google brand convention

**Files:**
- Modify: `docs/integrations.md`

The `<brand>-<application>` naming convention established by this pair is the reference for all future Google Suite integrations (`google-calendar`, `google-cloud`, etc.).

- [ ] **Step 1: Add Google brand convention to docs/integrations.md**

Find the brand naming section in `docs/integrations.md` and add:

```markdown
### Brand Naming: Multi-Application Suites

When a service provider offers multiple distinct applications or APIs, use the
`<provider>-<application>` convention:

- `google-auth` — transponder for Google OAuth assistance
- `google-drive` — remote integration for Google Drive sync folder
- `google-calendar` — (future) remote integration for Google Calendar

The full hyphenated string is the brand key. Orbiter sees `google-auth` and
`google-drive` as separate, independent brands.

**Go package names:** Directory names may contain hyphens but Go package names
cannot. Use a concatenated identifier in `generate.go`:

```go
// integrations/google-auth/generate.go
package googleauth

// integrations/google-drive/generate.go
package googledrive
```

**Shared auth dependency:** All Google Suite resource integrations declare a
dependency on the `google-auth` agent transponder:

```toml
[dependencies]
  [dependencies.transponders]
  agent = ["google-auth"]
```

This ensures the captain is authenticated before the resource integration runs.
The `google-auth` transponder handles the OAuth workflow; resource integrations
focus on their domain (sync folder, calendar events, etc.).
```

- [ ] **Step 2: Commit docs**

```bash
git add docs/integrations.md
git commit -m "docs: add Google Suite brand naming convention and shared auth pattern"
```
