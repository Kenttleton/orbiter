use crate::context::ResolvedContext;
use crate::report::{write_state, InputRequest, StateReport};

pub fn run(ctx: &ResolvedContext, calibrate: bool) {
    let credential = ctx
        .responses
        .get("credential")
        .map(|s| s.trim().to_string())
        .unwrap_or_default();
    let has_credential = !credential.is_empty();

    if !calibrate {
        // scan mode
        if has_credential {
            let report = StateReport {
                present: true,
                reachable: true,
                manager: "vault".to_string(),
                observations: vec!["credential resolved by Orbiter".to_string()],
                ..Default::default()
            };
            write_state(&report);
        } else {
            let report = StateReport {
                present: false,
                reachable: false,
                manager: "vault".to_string(),
                observations: vec![
                    "vault credential not provided — Orbiter must resolve upstream".to_string(),
                ],
                needs_input: vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Vault credential required".to_string(),
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
                manager: "vault".to_string(),
                ..Default::default()
            };
            report.exports.insert("GIT_CONFIG_COUNT".to_string(), "1".to_string());
            report.exports.insert(
                "GIT_CONFIG_KEY_0".to_string(),
                "http.extraHeader".to_string(),
            );
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
                manager: "vault".to_string(),
                observations: vec![
                    "vault credential not provided — Orbiter must resolve upstream".to_string(),
                ],
                needs_input: vec![InputRequest {
                    key: "credential".to_string(),
                    prompt: "Vault credential required".to_string(),
                    masked: true,
                }],
                ..Default::default()
            };
            write_state(&report);
        }
    }
}
