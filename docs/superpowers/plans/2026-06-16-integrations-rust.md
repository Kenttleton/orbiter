# Rust Integrations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver 9 Rust integrations covering `runtime/python`, `runtime/rust`, `manager/brew`, `manager/uv`, `manager/rustup`, `tool/docker`, `keychain/macos`, `vault/onepassword`, and `agent/ssh`.

**Architecture:** Each integration is a Rust `cdylib` compiled to `wasm32-unknown-unknown`. All 9 share the same host ABI module (copied from `integrations/git/src/host.rs`). JSON uses `serde` + `serde_json` — no manual JSON building. All tasks are independent and can run in any order.

**Tech Stack:** Rust stable, `wasm32-unknown-unknown` target, `serde` + `serde_json`, wazero host, `go test ./integrations/...`

## Global Constraints

- Target: `wasm32-unknown-unknown` (NOT wasm32-wasi)
- All Rust integrations export `detect`, `initialize`, `scan`, `calibrate` via `#[no_mangle] pub extern "C"`
- Host ABI: copy `integrations/git/src/host.rs` verbatim — it is identical for all Rust integrations
- Every `Cargo.toml` must include `serde = { version = "1", features = ["derive"] }` and `serde_json = "1"`
- `[profile.release]` must set `opt-level = "s"`, `lto = true`, `strip = true`, `panic = "abort"`
- `integrations/bundle.go` `//go:embed` line must include `<brand>/<brand>.wasm <brand>/manifest.toml` for each new integration
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`
- Run `go test ./integrations/...` after each integration

---

## Common Rust Pattern

Every Rust integration in this plan uses this identical file layout:

```
integrations/<brand>/
├── Cargo.toml
├── generate.go
└── src/
    ├── host.rs      ← copy of integrations/git/src/host.rs (identical)
    └── lib.rs       ← integration-specific logic
```

`src/host.rs` (copy from `integrations/git/src/host.rs` — do not modify):

```rust
mod ffi {
    #[link(wasm_import_module = "orbiter")]
    extern "C" {
        pub fn read_input(ptr: *mut u8, max: u32) -> u32;
        pub fn write_output(ptr: *const u8, len: u32);
        pub fn run_command(spec_ptr: *const u8, spec_len: u32, out_ptr: *mut u8, out_max: u32) -> u32;
    }
}

pub fn read_input() -> Vec<u8> {
    let mut buf = vec![0u8; 64 * 1024];
    let n = unsafe { ffi::read_input(buf.as_mut_ptr(), buf.len() as u32) };
    buf.truncate(n as usize);
    buf
}

pub fn write_output(data: &[u8]) {
    unsafe { ffi::write_output(data.as_ptr(), data.len() as u32) }
}

pub fn run_command(cmd: &str, args: &[&str]) -> String {
    #[derive(serde::Serialize)]
    struct Spec<'a> {
        cmd: &'a str,
        args: &'a [&'a str],
    }
    let spec = serde_json::to_vec(&Spec { cmd, args }).unwrap_or_default();
    let mut out = vec![0u8; 64 * 1024];
    let n = unsafe {
        ffi::run_command(spec.as_ptr(), spec.len() as u32, out.as_mut_ptr(), out.len() as u32)
    };
    String::from_utf8_lossy(&out[..n as usize]).trim().to_string()
}
```

Common types shared across integrations (include inline in each `lib.rs`):

```rust
#[derive(serde::Serialize, Default)]
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
    let bytes = serde_json::to_vec(&report).unwrap_or_default();
    host::write_output(&bytes);
}
```

---

### Task 1: runtime/python

**Files:**
- Create: `integrations/python/Cargo.toml`
- Create: `integrations/python/generate.go`
- Create: `integrations/python/src/host.rs`
- Create: `integrations/python/src/lib.rs`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Python(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "python")
	if !ok {
		t.Fatal("python integration not registered")
	}

	t.Run("detect_pyproject", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"pyproject.toml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for pyproject.toml")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "runtime" {
			t.Error("expected role=runtime suggestion")
		}
	})

	t.Run("detect_requirements", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"requirements.txt": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for requirements.txt")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without python files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (python3 is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_Python -v
```

Expected: FAIL — "python integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/python/src
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/python/Cargo.toml`:

```toml
[package]
name = "python"
version = "0.1.0"
edition = "2021"

[lib]
name = "python"
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

- [ ] **Step 5: Copy host.rs**

```bash
cp integrations/git/src/host.rs integrations/python/src/host.rs
```

- [ ] **Step 6: Write lib.rs**

Create `integrations/python/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
}

#[derive(Serialize)]
struct SuggestedResource {
    role: String,
    brand: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    version: Option<String>,
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
    let bytes = serde_json::to_vec(&report).unwrap_or_default();
    host::write_output(&bytes);
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = match serde_json::from_slice(&input) {
        Ok(c) => c,
        Err(_) => {
            host::write_output(b"{\"detected\":false}");
            return;
        }
    };
    let detected = ctx.files.contains_key("pyproject.toml")
        || ctx.files.contains_key("requirements.txt")
        || ctx.files.contains_key("setup.py");

    if !detected {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let version = host::run_command("python3", &["--version"]);
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "runtime".to_string(),
            brand: "python".to_string(),
            version: if version.is_empty() { None } else { Some(version) },
        }],
    };
    let bytes = serde_json::to_vec(&result).unwrap_or_default();
    host::write_output(&bytes);
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["python3"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "python3 not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("python3", &["--version"]);
    let manager = detect_manager();
    write_state(StateReport {
        present: true,
        reachable: true,
        binary_path: Some(binary_path),
        in_path: true,
        manager,
        observations: vec![version],
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    let version = host::run_command("python3", &["--version"]);
    if version.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "python3 not found".to_string(),
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: detect_manager(),
        observations: vec![format!("calibrated: {}", version)],
        ..Default::default()
    });
}

fn detect_manager() -> String {
    // Check for uv first (fastest), then pyenv, fall back to system
    let uv = host::run_command("which", &["uv"]);
    if !uv.is_empty() {
        return "uv".to_string();
    }
    let pyenv = host::run_command("which", &["pyenv"]);
    if !pyenv.is_empty() {
        return "pyenv".to_string();
    }
    "system".to_string()
}
```

- [ ] **Step 7: Create generate.go**

Create `integrations/python/generate.go`:

```go
package python

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/python.wasm ."
```

- [ ] **Step 8: Update bundle.go embed**

In `integrations/bundle.go`, add `python/python.wasm python/manifest.toml` to the `//go:embed` directive.

- [ ] **Step 9: Create manifest.toml**

Create `integrations/python/manifest.toml`:

```toml
[integration]
brand = "python"
name = "Python"
description = "Scans and verifies the Python 3 runtime"
roles = ["runtime"]

[detection]
files = ["pyproject.toml", "requirements.txt", "setup.py"]

[commands]
allowed = ["python3", "which", "uv", "pyenv"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 10: Compile**

```bash
cd integrations/python && go generate .
```

Expected: `python.wasm` created.

- [ ] **Step 11: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_Python -v
```

Expected: all subtests pass.

- [ ] **Step 12: Commit**

```bash
git add integrations/python/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add runtime/python Rust integration"
```

---

### Task 2: runtime/rust

**Files:**
- Create: `integrations/rust/Cargo.toml`
- Create: `integrations/rust/generate.go`
- Create: `integrations/rust/src/host.rs`
- Create: `integrations/rust/src/lib.rs`
- Create: `integrations/rust/manifest.toml`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Rust(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "rust")
	if !ok {
		t.Fatal("rust integration not registered")
	}

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Cargo.toml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Cargo.toml")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "runtime" {
			t.Error("expected role=runtime suggestion")
		}
		if report.Resources[0].Brand != "rust" {
			t.Errorf("expected brand=rust, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without Cargo.toml")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (rustc is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_Rust -v
```

Expected: FAIL — "rust integration not registered"

- [ ] **Step 3: Create directory and copy host.rs**

```bash
mkdir -p integrations/rust/src
cp integrations/git/src/host.rs integrations/rust/src/host.rs
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/rust/Cargo.toml`:

```toml
[package]
name = "rust_integration"
version = "0.1.0"
edition = "2021"

[lib]
name = "rust"
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

Create `integrations/rust/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
}

#[derive(Serialize)]
struct SuggestedResource {
    role: String,
    brand: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    version: Option<String>,
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
    let bytes = serde_json::to_vec(&report).unwrap_or_default();
    host::write_output(&bytes);
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = match serde_json::from_slice(&input) {
        Ok(c) => c,
        Err(_) => {
            host::write_output(b"{\"detected\":false}");
            return;
        }
    };
    if !ctx.files.contains_key("Cargo.toml") {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let rustc_version = host::run_command("rustc", &["--version"]);
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "runtime".to_string(),
            brand: "rust".to_string(),
            version: if rustc_version.is_empty() {
                None
            } else {
                Some(rustc_version)
            },
        }],
    };
    let bytes = serde_json::to_vec(&result).unwrap_or_default();
    host::write_output(&bytes);
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["rustc"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "rustc not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let rustc_version = host::run_command("rustc", &["--version"]);
    let cargo_version = host::run_command("cargo", &["--version"]);
    let toolchain = host::run_command("rustup", &["show", "active-toolchain"]);
    let manager = if !toolchain.is_empty() {
        "rustup".to_string()
    } else {
        "system".to_string()
    };
    let mut observations = vec![rustc_version, cargo_version];
    if !toolchain.is_empty() {
        observations.push(toolchain);
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        binary_path: Some(binary_path),
        in_path: true,
        manager,
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
    let _input = host::read_input();
    let rustc_version = host::run_command("rustc", &["--version"]);
    if rustc_version.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "rustc not found".to_string(),
            ..Default::default()
        });
        return;
    }
    let toolchain = host::run_command("rustup", &["show", "active-toolchain"]);
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: if toolchain.is_empty() {
            "system".to_string()
        } else {
            "rustup".to_string()
        },
        observations: vec![format!("calibrated: {}", rustc_version)],
        ..Default::default()
    });
}
```

- [ ] **Step 6: Create generate.go and manifest.toml**

Create `integrations/rust/generate.go`:

```go
package rust_integration

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/rust.wasm ."
```

Create `integrations/rust/manifest.toml`:

```toml
[integration]
brand = "rust"
name = "Rust"
description = "Scans and verifies the Rust compiler and Cargo toolchain"
roles = ["runtime"]

[detection]
files = ["Cargo.toml"]

[commands]
allowed = ["rustc", "cargo", "rustup", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 7: Update bundle.go and compile**

Add `rust/rust.wasm rust/manifest.toml` to the `//go:embed` directive in `integrations/bundle.go`.

```bash
cd integrations/rust && go generate .
```

- [ ] **Step 8: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_Rust -v
```

Expected: all subtests pass.

- [ ] **Step 9: Commit**

```bash
git add integrations/rust/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add runtime/rust Rust integration"
```

---

### Task 3: manager/brew

**Files:**
- Create: `integrations/brew/Cargo.toml`
- Create: `integrations/brew/generate.go`
- Create: `integrations/brew/manifest.toml`
- Create: `integrations/brew/src/host.rs`
- Create: `integrations/brew/src/lib.rs`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

Brew has no file-based detection — it is detected by checking if the `brew` binary is in PATH. This integration showcases `serde_json` deserialization of `brew --json` structured output.

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Brew(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("brew not available in CI")
	}
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "brew")
	if !ok {
		t.Fatal("brew integration not registered")
	}

	t.Run("detect_miss", func(t *testing.T) {
		// brew detect uses PATH check, not files — DetectContext has no brew signal
		// A project with only go.mod won't detect brew
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		// brew's detect always returns detected=false when there's no brew-specific file —
		// brew is a system tool detected via scan, not project files
		_ = report.Detected // any value is valid; brew detection is path-based
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// Only assert if brew is actually installed
		brewPath := ""
		if out, err := exec.Command("which", "brew").Output(); err == nil {
			brewPath = strings.TrimSpace(string(out))
		}
		if brewPath != "" {
			if !report.Present {
				t.Error("brew installed but present=false")
			}
			if report.BinaryPath == "" {
				t.Error("expected non-empty binary_path")
			}
		}
	})
}
```

Add `"os/exec"` to the e2e_test.go imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_Brew -v
```

Expected: FAIL — "brew integration not registered"

- [ ] **Step 3: Create directory and copy host.rs**

```bash
mkdir -p integrations/brew/src
cp integrations/git/src/host.rs integrations/brew/src/host.rs
```

- [ ] **Step 4: Create Cargo.toml**

Create `integrations/brew/Cargo.toml`:

```toml
[package]
name = "brew"
version = "0.1.0"
edition = "2021"

[lib]
name = "brew"
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

Create `integrations/brew/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};

#[derive(Serialize)]
struct SuggestedResource {
    role: String,
    brand: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    version: Option<String>,
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
    let bytes = serde_json::to_vec(&report).unwrap_or_default();
    host::write_output(&bytes);
}

// Brew is a system-level manager — detection is PATH-based, not file-based.
// The detect handler always returns detected=false for project files;
// brew surfaces via scan on any project.
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["brew"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "brew not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("brew", &["--version"]);
    let prefix = host::run_command("brew", &["--prefix"]);
    let mut observations = vec![version];
    if !prefix.is_empty() {
        observations.push(format!("prefix: {}", prefix));
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

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["brew"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "brew not found".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("brew", &["--version"]);
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", version)],
        ..Default::default()
    });
}
```

- [ ] **Step 6: Create generate.go and manifest.toml**

Create `integrations/brew/generate.go`:

```go
package brew

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/brew.wasm ."
```

Create `integrations/brew/manifest.toml`:

```toml
[integration]
brand = "brew"
name = "Homebrew"
description = "Scans and verifies the Homebrew package manager"
roles = ["manager"]

[commands]
allowed = ["brew", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

Note: no `[detection]` section — brew has no file-based detection.

- [ ] **Step 7: Update bundle.go and compile**

Add `brew/brew.wasm brew/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/brew && go generate .
```

- [ ] **Step 8: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_Brew -v
```

Expected: all subtests pass (scan conditionally skipped if brew not found).

- [ ] **Step 9: Commit**

```bash
git add integrations/brew/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add manager/brew Rust integration"
```

---

### Task 4: manager/uv

**Files:**
- Create: `integrations/uv/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/uv/src/host.rs`, `integrations/uv/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_UV(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "uv")
	if !ok {
		t.Fatal("uv integration not registered")
	}

	t.Run("detect_lock", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"uv.lock": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for uv.lock")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without uv files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// uv may not be installed everywhere; just verify shape
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_UV -v
```

Expected: FAIL — "uv integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/uv/src
cp integrations/git/src/host.rs integrations/uv/src/host.rs
```

Create `integrations/uv/Cargo.toml`:

```toml
[package]
name = "uv"
version = "0.1.0"
edition = "2021"

[lib]
name = "uv"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/uv/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
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

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        files: HashMap::new(),
    });
    // Detect uv.lock or pyproject.toml containing [tool.uv]
    // We can't read file contents here — just check if uv.lock is present
    let detected = ctx.files.contains_key("uv.lock");
    if !detected {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "manager".to_string(),
            brand: "uv".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["uv"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "uv not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("uv", &["--version"]);
    write_state(StateReport {
        present: true,
        reachable: true,
        binary_path: Some(binary_path),
        in_path: true,
        manager: "system".to_string(),
        observations: vec![version],
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    let version = host::run_command("uv", &["--version"]);
    if version.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "uv not found".to_string(),
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", version)],
        ..Default::default()
    });
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/uv/generate.go`:

```go
package uv

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/uv.wasm ."
```

Create `integrations/uv/manifest.toml`:

```toml
[integration]
brand = "uv"
name = "uv"
description = "Scans and verifies the uv Python package manager"
roles = ["manager"]

[detection]
files = ["uv.lock"]

[dependencies]
  [dependencies.resources]
  runtime = ["python"]

[commands]
allowed = ["uv", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 6: Update bundle.go and compile**

Add `uv/uv.wasm uv/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/uv && go generate .
```

- [ ] **Step 7: Run tests and commit**

```bash
go test ./integrations/... -run TestBundledIntegrations_UV -v
git add integrations/uv/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add manager/uv Rust integration"
```

---

### Task 5: manager/rustup

**Files:**
- Create: `integrations/rustup/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/rustup/src/host.rs`, `integrations/rustup/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Rustup(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "rustup")
	if !ok {
		t.Fatal("rustup integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (rustup is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
		if len(report.Observations) == 0 {
			t.Error("expected observations (toolchain info)")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_Rustup -v
```

Expected: FAIL — "rustup integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/rustup/src
cp integrations/git/src/host.rs integrations/rustup/src/host.rs
```

Create `integrations/rustup/Cargo.toml`:

```toml
[package]
name = "rustup"
version = "0.1.0"
edition = "2021"

[lib]
name = "rustup"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/rustup/src/lib.rs`:

```rust
mod host;

use serde::Serialize;

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

// rustup has no project-file detection — it's a system-level manager
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["rustup"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "rustup not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("rustup", &["--version"]);
    let active_toolchain = host::run_command("rustup", &["show", "active-toolchain"]);
    let mut observations = vec![version];
    if !active_toolchain.is_empty() {
        observations.push(format!("active toolchain: {}", active_toolchain));
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

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["rustup"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "rustup not found".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("rustup", &["--version"]);
    let active = host::run_command("rustup", &["show", "active-toolchain"]);
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![
            format!("calibrated: {}", version),
            if active.is_empty() {
                "no active toolchain".to_string()
            } else {
                format!("toolchain: {}", active)
            },
        ],
        ..Default::default()
    });
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/rustup/generate.go`:

```go
package rustup

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/rustup.wasm ."
```

Create `integrations/rustup/manifest.toml`:

```toml
[integration]
brand = "rustup"
name = "rustup"
description = "Scans and verifies the Rust toolchain manager"
roles = ["manager"]

[dependencies]
  [dependencies.resources]
  runtime = ["rust"]

[commands]
allowed = ["rustup", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 6: Update bundle.go and compile**

Add `rustup/rustup.wasm rustup/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/rustup && go generate .
```

- [ ] **Step 7: Run tests and commit**

```bash
go test ./integrations/... -run TestBundledIntegrations_Rustup -v
git add integrations/rustup/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add manager/rustup Rust integration"
```

---

### Task 6: tool/docker

**Files:**
- Create: `integrations/docker/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/docker/src/host.rs`, `integrations/docker/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Docker(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "docker")
	if !ok {
		t.Fatal("docker integration not registered")
	}

	t.Run("detect_dockerfile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Dockerfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Dockerfile")
		}
	})

	t.Run("detect_compose", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"docker-compose.yml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for docker-compose.yml")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without docker files")
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
go test ./integrations/... -run TestBundledIntegrations_Docker -v
```

Expected: FAIL — "docker integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/docker/src
cp integrations/git/src/host.rs integrations/docker/src/host.rs
```

Create `integrations/docker/Cargo.toml`:

```toml
[package]
name = "docker"
version = "0.1.0"
edition = "2021"

[lib]
name = "docker"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/docker/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
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

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        files: HashMap::new(),
    });
    let detected = ctx.files.contains_key("Dockerfile")
        || ctx.files.contains_key("docker-compose.yml")
        || ctx.files.contains_key("docker-compose.yaml");
    if !detected {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "tool".to_string(),
            brand: "docker".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["docker"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "docker not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("docker", &["--version"]);
    // Check if daemon is reachable by running `docker version --format json`
    // A non-empty response means the daemon is running
    let daemon_info = host::run_command("docker", &["version", "--format", "{{.Server.Version}}"]);
    let reachable = !daemon_info.is_empty();
    let mut observations = vec![version];
    if reachable {
        observations.push(format!("daemon: {}", daemon_info));
    } else {
        observations.push("daemon: not running".to_string());
    }
    write_state(StateReport {
        present: true,
        reachable,
        binary_path: Some(binary_path),
        in_path: true,
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
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["docker"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "docker not found".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("docker", &["--version"]);
    let context = host::run_command("docker", &["context", "show"]);
    let mut observations = vec![format!("calibrated: {}", version)];
    if !context.is_empty() {
        observations.push(format!("context: {}", context));
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/docker/generate.go`:

```go
package docker

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/docker.wasm ."
```

Create `integrations/docker/manifest.toml`:

```toml
[integration]
brand = "docker"
name = "Docker"
description = "Scans and verifies the Docker container runtime"
roles = ["tool"]

[detection]
files = ["Dockerfile", "docker-compose.yml", "docker-compose.yaml"]

[commands]
allowed = ["docker", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 16
```

- [ ] **Step 6: Update bundle.go and compile**

Add `docker/docker.wasm docker/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/docker && go generate .
```

- [ ] **Step 7: Run tests and commit**

```bash
go test ./integrations/... -run TestBundledIntegrations_Docker -v
git add integrations/docker/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add tool/docker Rust integration"
```

---

### Task 7: keychain/macos

**Files:**
- Create: `integrations/macos/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/macos/src/host.rs`, `integrations/macos/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

This is a transponder — it provides credentials, never emits them to shell. Detection is platform-based: only detected on macOS (`darwin`).

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_KeychainMacOS(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("keychain", "macos")
	if !ok {
		t.Fatal("macos keychain integration not registered")
	}

	t.Run("detect_darwin", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		if !report.Detected {
			t.Error("expected detected=true on darwin")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected suggested resource")
		}
		if report.Resources[0].Role != "keychain" {
			t.Errorf("expected role=keychain, got %q", report.Resources[0].Role)
		}
	})

	t.Run("detect_linux", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "linux"},
		})
		if report.Detected {
			t.Error("expected detected=false on linux")
		}
	})

	t.Run("scan_darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("keychain scan only valid on darwin")
		}
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true on darwin (security binary should exist)")
		}
	})
}
```

Add `"runtime"` to the e2e_test.go imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_KeychainMacOS -v
```

Expected: FAIL — "macos keychain integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/macos/src
cp integrations/git/src/host.rs integrations/macos/src/host.rs
```

Create `integrations/macos/Cargo.toml`:

```toml
[package]
name = "macos"
version = "0.1.0"
edition = "2021"

[lib]
name = "macos"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/macos/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};

#[derive(Deserialize, Default)]
struct Platform {
    #[serde(rename = "os", default)]
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

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    if ctx.platform.os != "darwin" {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "keychain".to_string(),
            brand: "macos".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let security_path = host::run_command("which", &["security"]);
    if security_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "security binary not found — not running on macOS?".to_string(),
            ..Default::default()
        });
        return;
    }
    let keychain_info = host::run_command("security", &["show-keychain-info"]);
    let unlocked = !keychain_info.to_lowercase().contains("locked");
    write_state(StateReport {
        present: true,
        reachable: unlocked,
        binary_path: Some(security_path),
        in_path: true,
        manager: "system".to_string(),
        observations: vec![if unlocked {
            "keychain: unlocked".to_string()
        } else {
            "keychain: locked".to_string()
        }],
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    // Keychain is a read-only credential provider from Orbiter's perspective.
    // The captain unlocks the keychain through normal macOS interactions.
    // Calibrate verifies state only — no mutation.
    scan();
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/macos/generate.go`:

```go
package macos

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/macos.wasm ."
```

Create `integrations/macos/manifest.toml`:

```toml
[integration]
brand = "macos"
name = "macOS Keychain"
description = "Verifies macOS Keychain availability for credential access"
roles = ["keychain"]

[commands]
allowed = ["security", "which"]
timeout_seconds = 15

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 6: Update bundle.go and compile**

Add `macos/macos.wasm macos/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/macos && go generate .
```

- [ ] **Step 7: Run tests and commit**

```bash
go test ./integrations/... -run TestBundledIntegrations_KeychainMacOS -v
git add integrations/macos/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add keychain/macos Rust integration"
```

---

### Task 8: vault/onepassword

**Files:**
- Create: `integrations/onepassword/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/onepassword/src/host.rs`, `integrations/onepassword/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_OnePassword(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("vault", "onepassword")
	if !ok {
		t.Fatal("onepassword integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// op may not be installed everywhere; just verify shape
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_OnePassword -v
```

Expected: FAIL — "onepassword integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/onepassword/src
cp integrations/git/src/host.rs integrations/onepassword/src/host.rs
```

Create `integrations/onepassword/Cargo.toml`:

```toml
[package]
name = "onepassword"
version = "0.1.0"
edition = "2021"

[lib]
name = "onepassword"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/onepassword/src/lib.rs`:

```rust
mod host;

use serde::Serialize;

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

// op is a system-level transponder — no project-file detection
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    let op_path = host::run_command("which", &["op"]);
    if op_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "op CLI not found in PATH — install 1Password CLI from 1password.com/downloads/command-line/".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("op", &["--version"]);
    // Check sign-in state: `op account list` returns accounts if signed in
    let accounts = host::run_command("op", &["account", "list"]);
    let signed_in = !accounts.is_empty() && !accounts.contains("No accounts");
    write_state(StateReport {
        present: true,
        reachable: signed_in,
        binary_path: Some(op_path),
        in_path: true,
        manager: "system".to_string(),
        observations: vec![
            version,
            if signed_in {
                "signed in: yes".to_string()
            } else {
                "signed in: no — run 'op signin' to authenticate".to_string()
            },
        ],
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    let op_path = host::run_command("which", &["op"]);
    if op_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "op not found".to_string(),
            ..Default::default()
        });
        return;
    }
    let accounts = host::run_command("op", &["account", "list"]);
    let signed_in = !accounts.is_empty() && !accounts.contains("No accounts");
    // If not signed in, surface the signin command — do not run it automatically
    // (op signin requires interactive browser auth / biometric)
    let version = host::run_command("op", &["--version"]);
    write_state(StateReport {
        present: true,
        reachable: signed_in,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![
            format!("calibrated: {}", version),
            if signed_in {
                "vault: accessible".to_string()
            } else {
                "vault: not authenticated — run 'op signin' to unlock".to_string()
            },
        ],
        ..Default::default()
    });
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/onepassword/generate.go`:

```go
package onepassword

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/onepassword.wasm ."
```

Create `integrations/onepassword/manifest.toml`:

```toml
[integration]
brand = "onepassword"
name = "1Password"
description = "Verifies 1Password CLI authentication state for vault access"
roles = ["vault"]

[commands]
allowed = ["op", "which"]
timeout_seconds = 15

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 6: Update bundle.go and compile**

Add `onepassword/onepassword.wasm onepassword/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/onepassword && go generate .
```

- [ ] **Step 7: Run tests and commit**

```bash
go test ./integrations/... -run TestBundledIntegrations_OnePassword -v
git add integrations/onepassword/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add vault/onepassword Rust integration"
```

---

### Task 9: agent/ssh

**Files:**
- Create: `integrations/ssh/Cargo.toml`, `generate.go`, `manifest.toml`
- Create: `integrations/ssh/src/host.rs`, `integrations/ssh/src/lib.rs`
- Modify: `integrations/bundle.go`, `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_SSHAgent(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "ssh")
	if !ok {
		t.Fatal("ssh agent integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// ssh-agent may or may not be running in CI
		t.Logf("SSH agent present: %v, reachable: %v", report.Present, report.Reachable)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_SSHAgent -v
```

Expected: FAIL — "ssh agent integration not registered"

- [ ] **Step 3: Create directory and files**

```bash
mkdir -p integrations/ssh/src
cp integrations/git/src/host.rs integrations/ssh/src/host.rs
```

Create `integrations/ssh/Cargo.toml`:

```toml
[package]
name = "ssh"
version = "0.1.0"
edition = "2021"

[lib]
name = "ssh"
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

- [ ] **Step 4: Write lib.rs**

Create `integrations/ssh/src/lib.rs`:

```rust
mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    env: HashMap<String, String>,
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

// SSH agent is a system process — no project-file detection
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        env: HashMap::new(),
    });
    let ssh_auth_sock = ctx.env.get("SSH_AUTH_SOCK").cloned().unwrap_or_default();
    let agent_path = host::run_command("which", &["ssh-agent"]);
    let present = !ssh_auth_sock.is_empty() || !agent_path.is_empty();

    if !present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "ssh-agent not found and SSH_AUTH_SOCK not set".to_string(),
            ..Default::default()
        });
        return;
    }

    let key_list = host::run_command("ssh-add", &["-l"]);
    let reachable = !ssh_auth_sock.is_empty()
        && !key_list.contains("Could not open a connection")
        && !key_list.contains("Error connecting");
    let mut observations = Vec::new();
    if !ssh_auth_sock.is_empty() {
        observations.push(format!("SSH_AUTH_SOCK: {}", ssh_auth_sock));
    }
    if reachable {
        if key_list.contains("no identities") {
            observations.push("keys loaded: 0".to_string());
        } else {
            let key_count = key_list.lines().count();
            observations.push(format!("keys loaded: {}", key_count));
        }
    }

    write_state(StateReport {
        present: true,
        reachable,
        binary_path: if agent_path.is_empty() {
            None
        } else {
            Some(agent_path)
        },
        in_path: !agent_path.is_empty(),
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
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        env: HashMap::new(),
    });
    let ssh_auth_sock = ctx.env.get("SSH_AUTH_SOCK").cloned().unwrap_or_default();

    if ssh_auth_sock.is_empty() {
        // Try to start ssh-agent
        let agent_path = host::run_command("which", &["ssh-agent"]);
        if agent_path.is_empty() {
            write_state(StateReport {
                present: false,
                reachable: false,
                in_path: false,
                manager: "system".to_string(),
                error: "ssh-agent not available".to_string(),
                ..Default::default()
            });
            return;
        }
        // Report that the captain needs to start ssh-agent and add to their shell profile
        write_state(StateReport {
            present: true,
            reachable: false,
            binary_path: Some(agent_path),
            in_path: true,
            manager: "system".to_string(),
            observations: vec![
                "ssh-agent not running — add 'eval $(ssh-agent -s)' to your shell profile".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let key_list = host::run_command("ssh-add", &["-l"]);
    let no_keys = key_list.contains("no identities");
    let mut observations = vec![format!("calibrated: SSH_AUTH_SOCK={}", ssh_auth_sock)];
    if no_keys {
        observations.push("no keys loaded — run 'ssh-add ~/.ssh/id_rsa' to load a key".to_string());
    } else {
        let key_count = key_list.lines().count();
        observations.push(format!("keys loaded: {}", key_count));
    }

    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
```

- [ ] **Step 5: Create generate.go and manifest.toml**

Create `integrations/ssh/generate.go`:

```go
package ssh

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/ssh.wasm ."
```

Create `integrations/ssh/manifest.toml`:

```toml
[integration]
brand = "ssh"
name = "SSH Agent"
description = "Verifies SSH agent availability and loaded key count"
roles = ["agent"]

[commands]
allowed = ["ssh-agent", "ssh-add", "which"]
timeout_seconds = 15

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 6: Update bundle.go and compile**

Add `ssh/ssh.wasm ssh/manifest.toml` to the `//go:embed` directive.

```bash
cd integrations/ssh && go generate .
```

- [ ] **Step 7: Run full suite and commit**

```bash
go test ./integrations/... -v
git add integrations/ssh/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add agent/ssh Rust integration"
```
