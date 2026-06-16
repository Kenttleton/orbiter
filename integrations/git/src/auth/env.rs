use crate::context::ResolvedContext;
use crate::host;
use crate::report::{write_error, write_state, InputRequest, StateReport};

pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    let variable = match crate::context::config_str(&ctx.self_entity.config, "variable") {
        Some(v) if !v.is_empty() => v,
        _ => {
            write_error("env transponder missing required config field: variable");
            return;
        }
    };

    let is_windows = ctx.platform.os == "windows";

    // Check whether the environment variable is set without reading its value.
    let set_output = if is_windows {
        let cmd = format!("if defined {} (echo set) else (echo unset)", variable);
        host::run_command("cmd", &["/c", &cmd])
    } else {
        let cmd = format!(
            "test -n \"${{{}+set}}\" && echo set || echo unset",
            variable
        );
        host::run_command("sh", &["-c", &cmd])
    };

    let is_set = set_output.trim() == "set";

    if !calibrate {
        // scan mode
        let (present, reachable, observation) = if is_set {
            (true, true, format!("{} is set", variable))
        } else {
            (false, false, format!("{} is not set", variable))
        };

        let report = StateReport {
            present,
            reachable,
            manager: "env".to_string(),
            observations: vec![observation],
            ..Default::default()
        };
        write_state(&report);
    } else {
        // calibrate mode
        let mut report = StateReport {
            manager: "env".to_string(),
            ..Default::default()
        };

        if !is_set {
            // Variable is unset — check if a credential was provided in responses
            if let Some(credential) = ctx.responses.get("credential") {
                // Export the variable for this session
                report.exports.insert(variable.clone(), credential.clone());
                report.present = true;
                report.reachable = true;
                report
                    .observations
                    .push(format!("{} configured for this session", variable));
            } else {
                // Request the credential from the user
                report.needs_input = vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: format!("Enter value for {}", variable),
                    masked: true,
                }];
                report.present = false;
                report.reachable = false;
            }
        } else {
            // Already set — healthy
            report.present = true;
            report.reachable = true;
            report
                .observations
                .push(format!("{} is set", variable));
        }

        write_state(&report);
    }
}
