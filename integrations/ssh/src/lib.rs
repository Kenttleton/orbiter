mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    env: HashMap<String, String>,
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
        binaries: HashMap::new(),
    });
    let ssh_auth_sock = ctx.env.get("SSH_AUTH_SOCK").cloned().unwrap_or_default();
    let agent_path = ctx.binaries.get("ssh-agent").cloned().unwrap_or_default();
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
        in_path: !agent_path.is_empty(),
        binary_path: if agent_path.is_empty() {
            None
        } else {
            Some(agent_path)
        },
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
    // Delegate to initialize to ensure calibrate reports real state (SSH_AUTH_SOCK
    // reachability, key count) rather than hardcoding reachable=true.
    initialize();
}
