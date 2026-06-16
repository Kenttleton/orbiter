use crate::context::ResolvedContext;
use crate::host;
use crate::report::{write_error, write_state, StateReport};

enum Protocol {
    Ssh,
    Https,
    Unknown,
}

fn detect_protocol(url: &str) -> Protocol {
    if url.starts_with("git@") || url.starts_with("ssh://") {
        Protocol::Ssh
    } else if url.starts_with("https://") || url.starts_with("http://") {
        Protocol::Https
    } else {
        Protocol::Unknown
    }
}

/// Extract the hostname from a remote URL.
/// - `git@github.com:user/repo.git` → `github.com`
/// - `https://github.com/user/repo.git` → `github.com`
fn extract_host(url: &str) -> &str {
    if url.starts_with("git@") {
        // git@<host>:...
        let after_at = &url[4..];
        if let Some(colon) = after_at.find(':') {
            return &after_at[..colon];
        }
        return after_at;
    }
    // http(s)://host/...
    if let Some(rest) = url.strip_prefix("https://").or_else(|| url.strip_prefix("http://")) {
        if let Some(slash) = rest.find('/') {
            return &rest[..slash];
        }
        return rest;
    }
    // ssh://[user@]host/...
    if let Some(rest) = url.strip_prefix("ssh://") {
        // may have user@
        let host_part = if let Some(at) = rest.find('@') {
            &rest[at + 1..]
        } else {
            rest
        };
        if let Some(slash) = host_part.find('/') {
            return &host_part[..slash];
        }
        return host_part;
    }
    url
}

pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    // --- Extract required config fields ---
    let location = match crate::context::config_str(&ctx.self_entity.config, "location") {
        Some(l) if !l.is_empty() => l,
        _ => {
            write_error("file transponder missing required config field: location");
            return;
        }
    };

    let remote_url = ctx
        .remote
        .as_ref()
        .map(|r| r.url.as_str())
        .unwrap_or("");

    let protocol = detect_protocol(remote_url);
    let is_windows = ctx.platform.os == "windows";

    let mut observations: Vec<String> = Vec::new();
    let mut present = false;
    let mut reachable = false;

    // --- File existence / permission check ---
    if is_windows {
        // On Windows, use ssh-keygen -l -f to check existence for SSH;
        // for HTTPS just try stat via dir command.
        match protocol {
            Protocol::Ssh => {
                let keygen_out = host::run_command("ssh-keygen", &["-l", "-f", &location]);
                if keygen_out.is_empty() {
                    observations.push(format!("ssh-keygen -l -f {} returned no output; file may not exist or is not a valid key", location));
                } else if keygen_out.contains("No such file") || keygen_out.to_lowercase().contains("error") {
                    observations.push(format!("key file check failed: {}", keygen_out));
                } else {
                    present = true;
                    reachable = true;
                    observations.push(format!("key fingerprint: {}", keygen_out));
                }
            }
            Protocol::Https | Protocol::Unknown => {
                // Use `cmd /c if exist <path> echo exists` as a basic check
                let check = host::run_command("cmd", &["/c", &format!("if exist \"{}\" echo exists", location)]);
                if check.contains("exists") {
                    present = true;
                    reachable = true;
                    observations.push(format!("token file found at {}", location));
                } else {
                    observations.push(format!("token file not found at {}", location));
                }
            }
        }
        if calibrate {
            observations.push("permission correction not available on Windows".to_string());
        }
    } else {
        // macOS / Linux: use `stat` to get octal permissions
        let stat_out = host::run_command("stat", &["-c", "%a", &location]);
        let stat_ok = !stat_out.is_empty()
            && !stat_out.to_lowercase().contains("no such file")
            && !stat_out.to_lowercase().contains("error")
            && !stat_out.to_lowercase().contains("illegal");

        if stat_ok {
            present = true;
            observations.push(format!("file permissions: {}", stat_out.trim()));

            if calibrate {
                // If permissions are more permissive than 600, correct them
                let perms = stat_out.trim();
                if should_fix_permissions(perms) {
                    let chmod_out = host::run_command("chmod", &["600", &location]);
                    if !chmod_out.is_empty() && chmod_out.to_lowercase().contains("error") {
                        observations.push(format!("chmod 600 {} failed: {}", location, chmod_out));
                    } else {
                        observations.push("permissions corrected to 600".to_string());
                    }
                }
            }
        } else {
            // stat may emit to stderr; a fallback for macOS where -c is not supported
            // Try macOS stat syntax: stat -f "%Lp" <path>
            let stat_mac = host::run_command("stat", &["-f", "%Lp", &location]);
            let stat_mac_ok = !stat_mac.is_empty()
                && !stat_mac.to_lowercase().contains("no such file")
                && !stat_mac.to_lowercase().contains("error");

            if stat_mac_ok {
                present = true;
                observations.push(format!("file permissions: {}", stat_mac.trim()));

                if calibrate {
                    let perms = stat_mac.trim();
                    if should_fix_permissions(perms) {
                        let chmod_out = host::run_command("chmod", &["600", &location]);
                        if !chmod_out.is_empty() && chmod_out.to_lowercase().contains("error") {
                            observations.push(format!("chmod 600 {} failed: {}", location, chmod_out));
                        } else {
                            observations.push("permissions corrected to 600".to_string());
                        }
                    }
                }
            } else {
                observations.push(format!("file not found or not accessible at {}", location));
            }
        }

        // Protocol-specific checks on non-Windows
        if present {
            match protocol {
                Protocol::Ssh => {
                    let keygen_out = host::run_command("ssh-keygen", &["-l", "-f", &location]);
                    if keygen_out.is_empty() || keygen_out.to_lowercase().contains("no such file") || keygen_out.to_lowercase().contains("invalid") {
                        observations.push(format!("ssh-keygen validation failed: {}", keygen_out));
                        // still mark reachable — file exists, key parsing may have just errored
                    } else {
                        reachable = true;
                        observations.push(format!("key fingerprint: {}", keygen_out));
                    }
                }
                Protocol::Https | Protocol::Unknown => {
                    reachable = true;
                    observations.push(format!("token file found at {}", location));
                }
            }
        }
    }

    let mut report = StateReport {
        present,
        reachable,
        manager: "file".to_string(),
        binary_path: location.clone(),
        observations,
        ..Default::default()
    };

    // --- Calibrate: set up GIT_SSH_COMMAND export for SSH ---
    if calibrate && reachable {
        if let Protocol::Ssh = detect_protocol(remote_url) {
            let ssh_cmd = format!("ssh -i {} -o IdentitiesOnly=yes", location);
            report.exports.insert("GIT_SSH_COMMAND".to_string(), ssh_cmd);

            // Report the remote host for context
            if !remote_url.is_empty() {
                let host_name = extract_host(remote_url);
                report.observations.push(format!(
                    "GIT_SSH_COMMAND configured for host {}",
                    host_name
                ));
            }
        }
    }

    write_state(&report);
}

/// Returns true if the octal permission string indicates that group or other
/// has any read/write/execute bit set (i.e., permissions are more than 600).
/// Input is a string like "644", "755", "600", "400" etc.
fn should_fix_permissions(perms: &str) -> bool {
    // We care about the last two octal digits (group and other).
    // A permission string like "600" means owner=6, group=0, other=0 — OK.
    // "644" → other digits are 4,4 — too permissive.
    let digits: Vec<char> = perms.chars().filter(|c| c.is_ascii_digit()).collect();
    if digits.len() < 3 {
        // Can't determine — be conservative and don't fix
        return false;
    }
    let group = digits[digits.len() - 2] as u8 - b'0';
    let other = digits[digits.len() - 1] as u8 - b'0';
    group != 0 || other != 0
}
