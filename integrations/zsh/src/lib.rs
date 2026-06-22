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
    let bytes = serde_json::to_vec(&report).unwrap_or_default();
    host::write_output(&bytes);
}

#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(
        b"{\"detected\":true,\"resources\":[{\"role\":\"shell\",\"brand\":\"zsh\"}]}",
    );
}

fn check_zsh() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("zsh").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "shell".to_string(),
            error: "zsh not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("zsh", &["--version"]);
    write_state(StateReport {
        present: true,
        reachable: !version.is_empty(),
        binary_path: Some(binary_path),
        in_path: true,
        manager: "shell".to_string(),
        observations: if !version.is_empty() { vec![version] } else { vec![] },
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn initialize() {
    check_zsh();
}

#[no_mangle]
pub extern "C" fn scan() {
    check_zsh();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    check_zsh();
}
