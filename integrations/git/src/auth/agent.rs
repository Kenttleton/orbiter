use crate::context::ResolvedContext;
use crate::host;
use crate::report::{write_state, InputRequest, StateReport};

fn resolve_socket(ctx: &ResolvedContext) -> Option<String> {
    // 1. Explicit config override
    if let Some(s) = crate::context::config_str(&ctx.self_entity.config, "socket") {
        if !s.is_empty() {
            return Some(s);
        }
    }
    // 2. External agent transponder passed socket in Responses
    if ctx.has_agent_transponder {
        if let Some(s) = ctx.responses.get("socket") {
            if !s.is_empty() {
                return Some(s.clone());
            }
        }
    }
    // 3. Platform default — None means use ambient SSH_AUTH_SOCK or Windows pipe
    None
}

pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    let is_windows = ctx.platform.os == "windows";
    let socket = resolve_socket(ctx);

    // Run ssh-add -l, optionally with a custom socket.
    let output = if let Some(ref path) = socket {
        if is_windows {
            let cmd = format!("set \"SSH_AUTH_SOCK={}\" && ssh-add -l", path);
            host::run_command("cmd", &["/c", &cmd])
        } else {
            let cmd = format!("SSH_AUTH_SOCK={} ssh-add -l", path);
            host::run_command("sh", &["-c", &cmd])
        }
    } else {
        host::run_command("ssh-add", &["-l"])
    };

    let lower = output.to_lowercase();

    let (present, reachable) = if lower.contains("error")
        || lower.contains("could not")
        || lower.contains("failed")
    {
        (false, false)
    } else if output.trim().is_empty() {
        (false, false)  // silent failure — treat as unreachable
    } else if lower.contains("no identities") {
        (true, false)
    } else {
        // Count identity lines: lines that start with a digit (e.g. "2048 SHA256:...")
        (true, true)
    };

    // Count identity lines for the observation
    let identity_count = if present && reachable {
        output
            .lines()
            .filter(|l| l.trim_start().starts_with(|c: char| c.is_ascii_digit()))
            .count()
    } else {
        0
    };

    if !calibrate {
        // scan mode
        let observation = if present && reachable {
            format!("agent has {} identit{} loaded", identity_count, if identity_count == 1 { "y" } else { "ies" })
        } else if present {
            "agent is reachable but has no identities loaded".to_string()
        } else {
            match &socket {
                Some(p) => format!("agent unreachable at {}", p),
                None => "ssh agent not reachable".to_string(),
            }
        };

        let report = StateReport {
            present,
            reachable,
            manager: "agent".to_string(),
            observations: vec![observation],
            ..Default::default()
        };
        write_state(&report);
    } else {
        // calibrate mode
        let mut report = StateReport {
            present,
            reachable,
            manager: "agent".to_string(),
            ..Default::default()
        };

        if !present {
            // Cannot connect to agent
            if let Some(ref path) = socket {
                report
                    .observations
                    .push(format!("agent unreachable at {} — is the agent running?", path));
            } else if is_windows {
                report.observations.push(
                    "OpenSSH Agent service may not be running — Orbiter does not start Windows services"
                        .to_string(),
                );
            } else {
                // macOS/Linux with no explicit socket — try to start ssh-agent
                let agent_out = host::run_command("ssh-agent", &["-s"]);
                if agent_out.contains("SSH_AUTH_SOCK=") {
                    // Parse: SSH_AUTH_SOCK=/path/to/socket; export SSH_AUTH_SOCK;
                    if let Some(sock_path) = parse_ssh_auth_sock(&agent_out) {
                        report
                            .exports
                            .insert("SSH_AUTH_SOCK".to_string(), sock_path);
                        if let Some(pid) = parse_env_var(&agent_out, "SSH_AGENT_PID") {
                            report
                                .exports
                                .insert("SSH_AGENT_PID".to_string(), pid);
                        }
                        report
                            .observations
                            .push("started ssh-agent, exported SSH_AUTH_SOCK".to_string());
                        report.present = true;
                    } else {
                        report
                            .observations
                            .push("started ssh-agent but could not parse SSH_AUTH_SOCK".to_string());
                    }
                } else {
                    report
                        .observations
                        .push("ssh-agent -s failed to start".to_string());
                }
            }
        } else if !reachable {
            // Agent reachable but no identities
            report.needs_input = vec![InputRequest {
                key: "add_key".to_string(),
                prompt: "No identities loaded. Run: ssh-add /path/to/your/key".to_string(),
                masked: false,
            }];
        } else {
            // Already healthy
            report.observations.push(format!(
                "agent has {} identit{} loaded",
                identity_count,
                if identity_count == 1 { "y" } else { "ies" }
            ));
        }

        write_state(&report);
    }
}

/// Parse the socket path from `ssh-agent -s` output.
/// Expected format: `SSH_AUTH_SOCK=/path/to/socket; export SSH_AUTH_SOCK;`
fn parse_ssh_auth_sock(output: &str) -> Option<String> {
    parse_env_var(output, "SSH_AUTH_SOCK")
}

/// Parse an environment variable value from `ssh-agent -s` output.
/// Expected format: `<KEY>=<value>; export <KEY>;`
fn parse_env_var(output: &str, key: &str) -> Option<String> {
    let needle = format!("{}=", key);
    let pos = output.find(&needle)?;
    let after = &output[pos + needle.len()..];
    // Value ends at ';' or whitespace
    let end = after
        .find(|c: char| c == ';' || c.is_whitespace())
        .unwrap_or(after.len());
    let value = &after[..end];
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}
