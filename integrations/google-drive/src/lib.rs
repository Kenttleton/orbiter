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
}

#[derive(Deserialize, Default)]
struct ResourceConfig {
    #[serde(default)]
    sync_folder: String,
}

#[derive(Deserialize)]
struct ResolvedContext {
    #[serde(default)]
    platform: Platform,
    #[serde(default)]
    config: ResourceConfig,
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

fn drive_app_path(os: &str) -> Option<String> {
    let path = match os {
        "darwin" => "/Applications/Google Drive.app",
        _ => return None,
    };
    let result = host::run_command("stat", &[path]);
    if result.is_empty() || result.contains("No such file") {
        None
    } else {
        Some(path.to_string())
    }
}

fn sync_folder_exists(path: &str) -> bool {
    if path.is_empty() {
        return false;
    }
    let result = host::run_command("stat", &["-c", "%F", path]);
    !result.is_empty() && !result.contains("No such file")
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let ctx: DetectContext = serde_json::from_slice(&input).unwrap_or(DetectContext {
        platform: Platform::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
        host::write_output(b"{\"detected\":false}");
        return;
    }
    let result = DetectResult {
        detected: true,
        resources: vec![SuggestedResource {
            role: "remote".to_string(),
            brand: "google-drive".to_string(),
        }],
    };
    host::write_output(&serde_json::to_vec(&result).unwrap_or_default());
}

#[no_mangle]
pub extern "C" fn initialize() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        platform: Platform::default(),
        config: ResourceConfig::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "system".to_string(),
            error: "Google Drive app not installed".to_string(),
            ..Default::default()
        });
        return;
    }

    let sync_folder = &ctx.config.sync_folder;
    let folder_exists = sync_folder_exists(sync_folder);
    let mut observations = vec![format!("Drive app: {}", app_path.unwrap())];
    if sync_folder.is_empty() {
        observations.push("sync folder: not configured — add sync_folder to resource config".to_string());
    } else if folder_exists {
        observations.push(format!("sync folder: {} (present)", sync_folder));
    } else {
        observations.push(format!("sync folder: {} (not found)", sync_folder));
    }

    write_state(StateReport {
        present: true,
        reachable: folder_exists || sync_folder.is_empty(),
        in_path: false,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

// calibrate: verify Drive app is installed and sync folder is present.
// Orbiter cannot re-link a GUI sync app — if the folder is missing,
// surface instructions and let the captain act.
#[no_mangle]
pub extern "C" fn calibrate() {
    let input = host::read_input();
    let ctx: ResolvedContext = serde_json::from_slice(&input).unwrap_or(ResolvedContext {
        platform: Platform::default(),
        config: ResourceConfig::default(),
    });
    let app_path = drive_app_path(&ctx.platform.os);
    if app_path.is_none() {
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

    let sync_folder = &ctx.config.sync_folder;
    let folder_exists = sync_folder_exists(sync_folder);

    let observations = if sync_folder.is_empty() {
        vec![
            "calibrated: Drive app present".to_string(),
            "action: set sync_folder in resource config to link your Google Drive folder".to_string(),
        ]
    } else if folder_exists {
        vec![
            format!("calibrated: sync folder {} present", sync_folder),
        ]
    } else {
        vec![
            format!("sync folder {} not found", sync_folder),
            "action: open Google Drive app and wait for sync to complete".to_string(),
            format!("expected path: {}", sync_folder),
        ]
    };

    write_state(StateReport {
        present: true,
        reachable: folder_exists || sync_folder.is_empty(),
        in_path: false,
        manager: "system".to_string(),
        observations,
        ..Default::default()
    });
}
