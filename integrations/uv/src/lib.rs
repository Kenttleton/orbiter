mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
}

#[derive(Deserialize, Default)]
struct ResolvedContext {
    #[serde(default)]
    binaries: HashMap<String, String>,
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
    // Detect uv.lock presence
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
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("uv").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
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
        reachable: !version.is_empty(),
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
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("uv").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
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
    let version = host::run_command("uv", &["--version"]);
    write_state(StateReport {
        present: true,
        reachable: !version.is_empty(),
        in_path: true,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", version)],
        ..Default::default()
    });
}
