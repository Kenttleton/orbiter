mod host;

use serde::{Deserialize, Serialize};

#[derive(Deserialize, Default)]
struct Platform {
    #[serde(default)]
    os: String,
}

#[derive(Deserialize)]
struct DetectContext {
    #[serde(default)]
    platform: Platform,
    #[serde(default)]
    binaries: std::collections::HashMap<String, String>,
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

fn drive_app_installed(os: &str, binaries: &std::collections::HashMap<String, String>) -> bool {
    match os {
        "darwin" => {
            let result = host::run_command("stat", &["/Applications/Google Drive.app"]);
            !result.is_empty() && !result.contains("No such file")
        }
        "linux" => {
            // Google Drive has no official Linux app; check for gcloud as a proxy
            let gcloud = binaries.get("gcloud").cloned().unwrap_or_default();
            !gcloud.is_empty()
        }
        _ => false,
    }
}

fn gcloud_authenticated() -> bool {
    let accounts = host::run_command("gcloud", &["auth", "list", "--format=value(account)"]);
    !accounts.is_empty() && !accounts.contains("Listed 0 items")
}

fn drive_app_authenticated(os: &str) -> bool {
    // The Drive desktop app manages its own auth state. On macOS we can check
    // whether it is running (which implies it is signed in).
    if os == "darwin" {
        let running = host::run_command("pgrep", &["-x", "Google Drive"]);
        return !running.is_empty();
    }
    false
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
        binaries: std::collections::HashMap::new(),
    });
    let os = &ctx.platform.os;
    let app_installed = drive_app_installed(os, &ctx.binaries);
    let gcloud_present = !ctx.binaries.get("gcloud").cloned().unwrap_or_default().is_empty();
    if !app_installed && !gcloud_present {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "agent".to_string(),
            brand: "google-auth".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
        binaries: std::collections::HashMap::new(),
    });
    let os = &ctx.platform.os;

    let app_installed = drive_app_installed(os, &ctx.binaries);
    let gcloud_path = ctx.binaries.get("gcloud").cloned().unwrap_or_default();
    let gcloud_present = !gcloud_path.is_empty();

    if !app_installed && !gcloud_present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "Google Drive app not installed and gcloud not found — install from drive.google.com or cloud.google.com/sdk".to_string(),
            ..Default::default()
        });
        return;
    }

    let mut observations = Vec::new();
    let mut authenticated = false;

    if app_installed {
        observations.push("Google Drive app: installed".to_string());
        if drive_app_authenticated(os) {
            authenticated = true;
            observations.push("Drive app: running (signed in)".to_string());
        } else {
            observations.push("Drive app: not running — open Google Drive to sign in".to_string());
        }
    }

    if gcloud_present {
        observations.push(format!("gcloud: {}", gcloud_path));
        if gcloud_authenticated() {
            authenticated = true;
            let account = host::run_command("gcloud", &["config", "get-value", "account"]);
            observations.push(format!("gcloud account: {}", account));
        } else {
            observations.push("gcloud: not authenticated — run 'gcloud auth login'".to_string());
        }
    }

    write_state(StateReport {
        present: true,
        reachable: authenticated,
        binary_path: if gcloud_present { Some(gcloud_path) } else { None },
        in_path: gcloud_present,
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
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
        binaries: std::collections::HashMap::new(),
    });
    let os = &ctx.platform.os;

    let app_installed = drive_app_installed(os, &ctx.binaries);
    let gcloud_path = ctx.binaries.get("gcloud").cloned().unwrap_or_default();
    let gcloud_present = !gcloud_path.is_empty();

    if !app_installed && !gcloud_present {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            observations: vec![
                "Google Drive app not installed".to_string(),
                "Install from: https://drive.google.com/drive/downloads".to_string(),
                "or: brew install --cask google-drive".to_string(),
            ],
            ..Default::default()
        });
        return;
    }

    let mut observations = vec!["calibrated: Google auth".to_string()];
    let mut authenticated = false;

    if app_installed && drive_app_authenticated(os) {
        authenticated = true;
        observations.push("Drive app: signed in".to_string());
    } else if gcloud_present && gcloud_authenticated() {
        authenticated = true;
        let account = host::run_command("gcloud", &["config", "get-value", "account"]);
        observations.push(format!("gcloud: authenticated as {}", account));
    } else {
        // Surface auth steps — do not run them automatically (require browser/biometric)
        if app_installed {
            observations.push("action: open Google Drive app and sign in".to_string());
        }
        if gcloud_present {
            observations.push("action: run 'gcloud auth login' to authenticate gcloud".to_string());
        }
    }

    write_state(StateReport {
        present: true,
        reachable: authenticated,
        binary_path: if gcloud_present { Some(gcloud_path) } else { None },
        in_path: gcloud_present,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
