use crate::context::ResolvedContext;
use crate::host;
use crate::report::{write_state, InputRequest, StateReport};

/// Extract the hostname from a git remote URL.
/// Handles both SSH (`git@github.com:user/repo.git`) and
/// HTTPS (`https://github.com/user/repo.git`) forms.
fn extract_host(url: &str) -> &str {
    // SSH form: git@<host>:<path>
    if let Some(at_pos) = url.find('@') {
        let after_at = &url[at_pos + 1..];
        let end = after_at
            .find(|c: char| c == ':' || c == '/')
            .unwrap_or(after_at.len());
        return &after_at[..end];
    }
    // HTTPS form: https://<host>/...
    if let Some(scheme_end) = url.find("://") {
        let after_scheme = &url[scheme_end + 3..];
        let end = after_scheme
            .find('/')
            .unwrap_or(after_scheme.len());
        return &after_scheme[..end];
    }
    url
}

pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    let remote_url = ctx.remote.as_ref().map(|r| r.url.as_str()).unwrap_or("");

    if ctx.has_keychain_transponder {
        run_external_keychain(ctx, calibrate, remote_url);
    } else {
        run_platform_default(ctx, calibrate, remote_url);
    }
}

fn run_external_keychain(ctx: &ResolvedContext, calibrate: bool, remote_url: &str) {
    let credential = ctx.responses.get("credential").map(|s| s.as_str()).unwrap_or("").trim().to_string();
    let has_credential = !credential.is_empty();

    if !calibrate {
        // scan mode
        if has_credential {
            let report = StateReport {
                present: true,
                reachable: true,
                manager: "keychain".to_string(),
                observations: vec!["credential resolved by external keychain".to_string()],
                ..Default::default()
            };
            write_state(&report);
        } else {
            let report = StateReport {
                present: false,
                reachable: false,
                manager: "keychain".to_string(),
                needs_input: vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Keychain credential required".to_string(),
                    masked: true,
                }],
                ..Default::default()
            };
            write_state(&report);
        }
    } else {
        // calibrate mode
        if has_credential {
            let mut report = StateReport {
                present: true,
                reachable: true,
                manager: "keychain".to_string(),
                ..Default::default()
            };
            // Configure git for the session via environment overrides.
            // For HTTPS remotes, set http.extraHeader with a Bearer token.
            let _ = remote_url; // remote_url informs context but header applies broadly
            report.exports.insert("GIT_CONFIG_COUNT".to_string(), "1".to_string());
            report.exports.insert("GIT_CONFIG_KEY_0".to_string(), "http.extraHeader".to_string());
            report.exports.insert(
                "GIT_CONFIG_VALUE_0".to_string(),
                format!("Authorization: Bearer {}", credential),
            );
            report
                .observations
                .push("http.extraHeader configured for this session (credential not logged)".to_string());
            write_state(&report);
        } else {
            let report = StateReport {
                present: false,
                reachable: false,
                manager: "keychain".to_string(),
                needs_input: vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Keychain credential required".to_string(),
                    masked: true,
                }],
                ..Default::default()
            };
            write_state(&report);
        }
    }
}

fn run_platform_default(ctx: &ResolvedContext, calibrate: bool, remote_url: &str) {
    let host = extract_host(remote_url);
    let _ = host; // used for context; credential.helper check is global

    // Check if a credential helper is configured.
    let helper = host::run_command("git", &["config", "credential.helper"]);
    let helper = helper.trim().to_string();
    let has_helper = !helper.is_empty();

    if !calibrate {
        // scan mode
        if has_helper {
            let report = StateReport {
                present: true,
                reachable: true,
                manager: "keychain".to_string(),
                observations: vec![format!("git credential helper configured: {}", helper)],
                ..Default::default()
            };
            write_state(&report);
        } else {
            let report = StateReport {
                present: false,
                reachable: false,
                manager: "keychain".to_string(),
                observations: vec!["no git credential helper configured".to_string()],
                ..Default::default()
            };
            write_state(&report);
        }
    } else {
        // calibrate mode
        let mut report = StateReport {
            manager: "keychain".to_string(),
            ..Default::default()
        };

        if !has_helper {
            // Configure a platform-appropriate credential helper.
            let os = ctx.platform.os.as_str();
            let configure_result = match os {
                "darwin" => {
                    host::run_command(
                        "git",
                        &["config", "--global", "credential.helper", "osxkeychain"],
                    )
                }
                "windows" => {
                    host::run_command(
                        "git",
                        &["config", "--global", "credential.helper", "manager"],
                    )
                }
                _ => {
                    // Linux — try libsecret; fall back to GCM suggestion.
                    let result = host::run_command(
                        "git",
                        &[
                            "config",
                            "--global",
                            "credential.helper",
                            "/usr/share/doc/git/contrib/credential/libsecret/git-credential-libsecret",
                        ],
                    );
                    result
                }
            };

            // Check whether the helper was applied successfully.
            let helper_now = host::run_command("git", &["config", "credential.helper"])
                .trim()
                .to_string();

            if !helper_now.is_empty() {
                report.present = true;
                report.reachable = true;
                report
                    .observations
                    .push(format!("configured git credential helper: {}", helper_now));
            } else {
                let _ = configure_result;
                // Configuration failed — suggest GCM on Linux.
                if ctx.platform.os == "linux" {
                    report.observations.push(
                        "could not configure libsecret credential helper; consider installing Git Credential Manager (GCM)".to_string(),
                    );
                }
                report.needs_input = vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Authenticate via your git credential helper".to_string(),
                    masked: false,
                }];
                report.present = false;
                report.reachable = false;
            }
        } else {
            // Helper is configured but we could not verify an actual credential entry.
            report.present = true;
            report.reachable = true;
            report
                .observations
                .push(format!("git credential helper configured: {}", helper));
            // Prompt the user to authenticate if no credential was surfaced.
            if ctx.responses.get("credential").map(|s| s.is_empty()).unwrap_or(true) {
                report.needs_input = vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Authenticate via your git credential helper".to_string(),
                    masked: false,
                }];
            }
        }

        write_state(&report);
    }
}
