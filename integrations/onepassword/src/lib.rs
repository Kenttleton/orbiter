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

// op is a system-level transponder — no project-file detection
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let op_path = ctx.binaries.get("op").cloned().unwrap_or_default();
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
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let op_path = ctx.binaries.get("op").cloned().unwrap_or_default();
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
