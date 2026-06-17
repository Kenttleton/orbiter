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
