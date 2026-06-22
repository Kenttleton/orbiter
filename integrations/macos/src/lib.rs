mod host;

use serde::{Deserialize, Serialize};

#[derive(Deserialize, Default)]
struct Platform {
    #[serde(rename = "os", default)]
    os: String,
}

/// Context passed to detect() — no binaries field; binaries are not yet resolved.
#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    platform: Platform,
}

/// Context passed to initialize(), scan(), calibrate() — includes resolved binaries.
#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    platform: Platform,
    #[serde(default)]
    binaries: std::collections::HashMap<String, String>,
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
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        platform: Platform::default(),
        binaries: std::collections::HashMap::new(),
    });
    let security_path = ctx.binaries.get("security").cloned().unwrap_or_default();
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
    // "unlocked".contains("locked") == true, so we cannot use contains("locked") to
    // detect a locked keychain. Instead: if the command returned any output the
    // security binary is accessible and the keychain is reachable; treat as unlocked.
    // A truly locked keychain would surface via an error message — detect that instead.
    let locked = keychain_info.to_lowercase().contains("error")
        || keychain_info.to_lowercase().contains("authorization failed");
    let reachable = !security_path.is_empty() && !locked;
    write_state(StateReport {
        present: true,
        reachable,
        binary_path: Some(security_path),
        in_path: true,
        manager: "system".to_string(),
        observations: vec![if reachable {
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
