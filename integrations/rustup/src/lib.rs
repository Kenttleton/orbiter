mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize, Default)]
struct ResolvedContext {
    #[serde(default)]
    binaries: HashMap<String, String>,
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

// rustup has no project-file detection — it's a system-level manager
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("rustup").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
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
    let mut observations = vec![version.clone()];
    if !active_toolchain.is_empty() {
        observations.push(format!("active toolchain: {}", active_toolchain));
    }
    write_state(StateReport {
        present: true,
        reachable: !version.is_empty(),
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
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("rustup").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
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
        reachable: !version.is_empty(),
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
