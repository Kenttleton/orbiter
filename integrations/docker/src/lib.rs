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
        || ctx.files.contains_key("docker-compose.yaml")
        || ctx.files.contains_key(".dockerignore");
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
