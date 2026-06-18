mod host;

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

// ── Types ──────────────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    files: HashMap<String, String>,
}

#[derive(Deserialize, Default)]
struct ResolvedContext {
    #[serde(default)]
    role: String,
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

// ── detect ─────────────────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        files: HashMap::new(),
    });
    if !ctx.files.contains_key(".git/config") {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    // Suggest all three roles — the registry will register all three
    let result = DetectResult {
        detected: true,
        resources: vec![
            SuggestedResource { role: "tool".to_string(), brand: "github".to_string() },
            SuggestedResource { role: "remote".to_string(), brand: "github".to_string() },
            SuggestedResource { role: "agent".to_string(), brand: "github".to_string() },
        ],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

// ── initialize / scan ──────────────────────────────────────────────────────
// Dispatches by role field from the context.

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    match ctx.role.as_str() {
        "remote" => scan_remote(),
        "agent"  => scan_agent(),
        _        => scan_tool(),  // "tool" and default
    }
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

// ── tool role ──────────────────────────────────────────────────────────────

fn scan_tool() {
    let binary_path = host::run_command("which", &["gh"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "gh CLI not found in PATH — install from cli.github.com".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("gh", &["--version"]);
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.contains("not logged in") && !auth_status.is_empty();
    let mut observations = vec![version];
    if authenticated {
        observations.push("auth: logged in".to_string());
    } else {
        observations.push("auth: not authenticated — run 'gh auth login'".to_string());
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

// ── remote role ────────────────────────────────────────────────────────────

fn scan_remote() {
    let repo_view = host::run_command("gh", &["repo", "view", "--json", "name,url,visibility"]);
    if repo_view.is_empty() || repo_view.contains("error") || repo_view.contains("not found") {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "no GitHub remote found or not authenticated".to_string(),
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: false,
        manager: "system".to_string(),
        observations: vec![repo_view],
        ..Default::default()
    });
}

// ── agent role ─────────────────────────────────────────────────────────────

fn scan_agent() {
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.is_empty()
        && !auth_status.contains("not logged in")
        && !auth_status.contains("not found");
    if !authenticated {
        write_state(StateReport {
            present: true, // gh is present
            reachable: false, // but not authenticated
            in_path: true,
            manager: "system".to_string(),
            observations: vec![
                "not authenticated — run calibrate to complete gh auth login".to_string(),
            ],
            ..Default::default()
        });
        return;
    }
    let token_check = host::run_command("gh", &["auth", "token"]);
    let has_token = !token_check.is_empty() && !token_check.contains("error");
    write_state(StateReport {
        present: true,
        reachable: has_token,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![
            auth_status,
            if has_token {
                "GH_TOKEN: available".to_string()
            } else {
                "GH_TOKEN: not available".to_string()
            },
        ],
        ..Default::default()
    });
}

// ── calibrate ──────────────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or_default();
    match ctx.role.as_str() {
        "remote" => calibrate_remote(),
        "agent"  => calibrate_agent(),
        _        => calibrate_tool(),
    }
}

fn calibrate_tool() {
    // Tool calibrate: verify gh is present and auth is valid
    scan_tool();
}

fn calibrate_remote() {
    // Remote calibrate: verify repo is reachable; surface clone command if not
    let repo_view = host::run_command("gh", &["repo", "view", "--json", "name,url"]);
    if repo_view.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            observations: vec![
                "calibrated: no GitHub remote found".to_string(),
                "hint: run 'gh repo clone <owner>/<repo>' to establish the remote".to_string(),
            ],
            ..Default::default()
        });
        return;
    }
    write_state(StateReport {
        present: true,
        reachable: true,
        in_path: false,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", repo_view)],
        ..Default::default()
    });
}

fn calibrate_agent() {
    // Agent calibrate: run gh auth login if not authenticated.
    // gh auth login is an interactive command — we surface it rather than running it
    // blindly. When the captain is already logged in, we emit GH_TOKEN via exports.
    let auth_status = host::run_command("gh", &["auth", "status"]);
    let authenticated = !auth_status.is_empty()
        && !auth_status.contains("not logged in")
        && !auth_status.contains("not found");

    if !authenticated {
        // Surface the login command — the host will prompt the captain to run it
        write_state(StateReport {
            present: true,
            reachable: false,
            in_path: true,
            manager: "system".to_string(),
            observations: vec![
                "not authenticated".to_string(),
                "run: gh auth login --web  (opens browser)".to_string(),
                "or:  gh auth login --with-token  (paste a PAT)".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let token = host::run_command("gh", &["auth", "token"]);
    if token.is_empty() {
        write_state(StateReport {
            present: true,
            reachable: true,
            in_path: true,
            manager: "system".to_string(),
            observations: vec!["authenticated but could not retrieve token".to_string()],
            ..Default::default()
        });
        return;
    }

    // Emit GH_TOKEN as a shell export. The host reads the exports field
    // from StateReport and emits "export GH_TOKEN=<value>" to the shell
    // eval output. The export key must be declared in [shell] exports in manifest.toml.
    #[derive(Serialize, Default)]
    struct AgentReport {
        present: bool,
        reachable: bool,
        in_path: bool,
        manager: String,
        observations: Vec<String>,
        exports: HashMap<String, String>,
    }
    let mut exports = HashMap::new();
    exports.insert("GH_TOKEN".to_string(), token);
    let report = AgentReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec!["calibrated: GH_TOKEN set".to_string()],
        exports,
    };
    host::write_output(&serde_json::to_vec(&report).unwrap_or_default());
}
