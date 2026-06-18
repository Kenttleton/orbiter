mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize, Default)]
struct TmuxConfig {
    #[serde(default)]
    vars: HashMap<String, String>,
}

#[derive(Deserialize)]
struct SelfResource {
    #[serde(default)]
    config: String,
}

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(rename = "self", default)]
    self_res: Option<SelfResource>,
}

#[derive(Serialize, Default)]
struct StateReport {
    present: bool,
    reachable: bool,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    manager: String,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    error: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    observations: Vec<String>,
}

fn write_state(report: StateReport) {
    host::write_output(&serde_json::to_vec(&report).unwrap_or_default());
}

fn parse_config(ctx: &ResolvedContext) -> TmuxConfig {
    ctx.self_res
        .as_ref()
        .and_then(|s| serde_json::from_str(&s.config).ok())
        .unwrap_or_default()
}

#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    calibrate();
}

#[no_mangle]
pub extern "C" fn scan() {
    let tmux_path = host::run_command("which", &["tmux"]);
    write_state(StateReport {
        present: !tmux_path.is_empty(),
        reachable: !tmux_path.is_empty(),
        manager: "system".to_string(),
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        self_res: None,
    });
    let cfg = parse_config(&ctx);

    if cfg.vars.is_empty() {
        write_state(StateReport {
            present: true,
            reachable: true,
            manager: "system".to_string(),
            ..Default::default()
        });
        return;
    }

    let tmux_path = host::run_command("which", &["tmux"]);
    if tmux_path.is_empty() {
        // tmux not installed or not in PATH — no-op, not an error.
        write_state(StateReport {
            present: true,
            reachable: true,
            manager: "system".to_string(),
            observations: vec!["tmux not in PATH — skipped".to_string()],
            ..Default::default()
        });
        return;
    }

    let mut applied = Vec::new();
    for (key, val) in &cfg.vars {
        let result = host::run_command("tmux", &["set-environment", "-g", key, val]);
        // Ignore errors — user may not be inside a tmux session.
        applied.push(format!("set {}={}", key, result));
    }

    write_state(StateReport {
        present: true,
        reachable: true,
        manager: "system".to_string(),
        observations: applied,
        ..Default::default()
    });
}
