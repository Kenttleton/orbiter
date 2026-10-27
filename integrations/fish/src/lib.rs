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

/// What Orbiter tells every integration that is asked "is this environment yours?"
#[derive(Deserialize, Default)]
struct DetectContext {
    #[serde(default)]
    env: HashMap<String, String>,
}

/// True if the final path component of `path` (either separator, optional .exe) is one of `names`.
fn is_executable_named(path: &str, names: &[&str]) -> bool {
    let base = path.rsplit(|c| c == '/' || c == '\\').next().unwrap_or(path);
    let base = base.strip_suffix(".exe").unwrap_or(base);
    names.contains(&base)
}

/// This integration decides whether it is the active shell; Orbiter only asks. The shell
/// announces itself through FISH_VERSION when something has exported it (shells keep their own
/// version variable to themselves by default), and the login shell ($SHELL) names it.
#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or_default();
    let active = ctx.env.contains_key("FISH_VERSION")
        || ctx
            .env
            .get("SHELL")
            .map_or(false, |shell| is_executable_named(shell, &["fish"]));
    let out = if active {
        "{\"detected\":true,\"resources\":[{\"role\":\"shell\",\"brand\":\"fish\"}]}"
    } else {
        "{\"detected\":false}"
    };
    host::write_output(out.as_bytes());
}

fn check_fish() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    let binary_path = ctx.binaries.get("fish").cloned().unwrap_or_default();
    let present = !binary_path.is_empty();
    if !present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "shell".to_string(),
            error: "fish not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("fish", &["--version"]);
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
    check_fish();
}

#[no_mangle]
pub extern "C" fn scan() {
    check_fish();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    check_fish();
}
