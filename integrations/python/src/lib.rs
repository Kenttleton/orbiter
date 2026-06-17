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
