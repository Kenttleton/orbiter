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
