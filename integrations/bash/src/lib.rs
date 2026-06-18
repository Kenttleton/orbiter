mod host;

use serde::Serialize;

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

// detect() is called only when BASH_VERSION is in the env (manifest pre-filter).
// We are running inside bash — suggest the shell/bash resource.
#[no_mangle]
pub extern "C" fn detect() {
    host::write_output(
        b"{\"detected\":true,\"resources\":[{\"role\":\"shell\",\"brand\":\"bash\"}]}",
    );
}

fn check_bash() {
    let _input = host::read_input();
    let binary_path = host::run_command("which", &["bash"]);
    if binary_path.is_empty() {
        write_state(StateReport {
            present: false,
            reachable: false,
            in_path: false,
            manager: "shell".to_string(),
            error: "bash not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host::run_command("bash", &["--version"]);
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
    check_bash();
}

#[no_mangle]
pub extern "C" fn scan() {
    check_bash();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    check_bash();
}
