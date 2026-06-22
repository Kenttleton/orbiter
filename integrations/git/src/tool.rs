use crate::context::ResolvedContext;
use crate::report::{StateReport, write_state};

/// Returns the git version string (e.g. "2.39.2") or empty string if not found.
pub fn git_version() -> String {
    let out = crate::host::run_command("git", &["--version"]);
    parse_git_version(&out)
}

/// Run scan or calibrate for role=tool.
/// calibrate flag is ignored — tool role: scan and calibrate behave identically.
/// Verifies `git` binary is present and reachable.
pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    let _ = calibrate;

    // Try to get git binary path from context binaries map.
    let binary_path = ctx.binaries.get("git").cloned().unwrap_or_default();
    let version_out = crate::host::run_command("git", &["--version"]);

    if binary_path.is_empty() && ctx.platform.os != "windows" {
        write_state(&StateReport {
            manager: "system".to_string(),
            error: "git binary not found".to_string(),
            ..Default::default()
        });
        return;
    }

    if version_out.is_empty() {
        write_state(&StateReport {
            manager: "system".to_string(),
            error: "git --version failed".to_string(),
            ..Default::default()
        });
        return;
    }

    write_state(&StateReport {
        present: true,
        reachable: true,
        binary_path,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![version_out],
        ..Default::default()
    });
}

fn parse_git_version(s: &str) -> String {
    // "git version 2.39.2" -> "2.39.2"
    // "git version 2.50.1 (Apple Git-155)" -> "2.50.1"
    s.split_whitespace()
        .find(|tok| tok.starts_with(|c: char| c.is_ascii_digit()))
        .unwrap_or("")
        .to_string()
}
