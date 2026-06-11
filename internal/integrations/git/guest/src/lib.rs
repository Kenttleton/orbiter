// Import host functions from the "orbiter" WASM module.
// These are the only way to do I/O or run processes on wasm32-unknown-unknown.
#[link(wasm_import_module = "orbiter")]
extern "C" {
    fn read_input(ptr: *mut u8, max: u32) -> u32;
    fn write_output(ptr: *const u8, len: u32);
    fn run_command(spec_ptr: *const u8, spec_len: u32, out_ptr: *mut u8, out_max: u32) -> u32;
}

fn host_read_input() -> Vec<u8> {
    let mut buf = vec![0u8; 64 * 1024];
    let n = unsafe { read_input(buf.as_mut_ptr(), buf.len() as u32) };
    buf.truncate(n as usize);
    buf
}

fn host_write_output(data: &[u8]) {
    unsafe { write_output(data.as_ptr(), data.len() as u32) };
}

fn host_run_command(cmd: &str, args: &[&str]) -> String {
    let spec = build_cmd_spec(cmd, args);
    let mut out = vec![0u8; 64 * 1024];
    let n = unsafe {
        run_command(
            spec.as_ptr(),
            spec.len() as u32,
            out.as_mut_ptr(),
            out.len() as u32,
        )
    };
    out.truncate(n as usize);
    String::from_utf8_lossy(&out).trim().to_string()
}

fn build_cmd_spec(cmd: &str, args: &[&str]) -> Vec<u8> {
    let mut s = format!(r#"{{"cmd":{}"#, json_str(cmd));
    s.push_str(r#","args":["#);
    for (i, a) in args.iter().enumerate() {
        if i > 0 {
            s.push(',');
        }
        s.push_str(&json_str(a));
    }
    s.push_str("]}");
    s.into_bytes()
}

fn json_str(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

fn has_key(input: &[u8], key: &str) -> bool {
    let needle = format!("\"{}\"", key);
    input.windows(needle.len()).any(|w| w == needle.as_bytes())
}

#[derive(Default)]
struct StateReport {
    present: bool,
    reachable: bool,
    binary_path: String,
    in_path: bool,
    manager: String,
    error: String,
    observations: Vec<String>,
}

fn write_state_report(r: &StateReport) {
    let mut buf = format!(
        r#"{{"present":{},"reachable":{},"in_path":{},"manager":{}"#,
        r.present,
        r.reachable,
        r.in_path,
        json_str(&r.manager),
    );
    if !r.binary_path.is_empty() {
        buf.push_str(&format!(r#","binary_path":{}"#, json_str(&r.binary_path)));
    }
    if !r.error.is_empty() {
        buf.push_str(&format!(r#","error":{}"#, json_str(&r.error)));
    }
    if !r.observations.is_empty() {
        buf.push_str(r#","observations":["#);
        for (i, o) in r.observations.iter().enumerate() {
            if i > 0 {
                buf.push(',');
            }
            buf.push_str(&json_str(o));
        }
        buf.push(']');
    }
    buf.push('}');
    host_write_output(buf.as_bytes());
}

fn write_detect_report(detected: bool, role: &str, brand: &str, version: &str) {
    if !detected {
        host_write_output(b"{\"detected\":false}");
        return;
    }
    let buf = format!(
        r#"{{"detected":true,"resources":[{{"role":{},"brand":{},"version":{}}}]}}"#,
        json_str(role),
        json_str(brand),
        json_str(version),
    );
    host_write_output(buf.as_bytes());
}

fn parse_git_version(s: &str) -> String {
    // "git version 2.39.2" -> "2.39.2"
    s.split_whitespace().last().unwrap_or("").to_string()
}

#[no_mangle]
pub extern "C" fn detect() {
    let input = host_read_input();
    // Detect if this is a git repository by checking for .git/config in the files map.
    if !has_key(&input, ".git/config") {
        write_detect_report(false, "", "", "");
        return;
    }
    let version_out = host_run_command("git", &["--version"]);
    let version = parse_git_version(&version_out);
    write_detect_report(true, "tool", "git", &version);
}

#[no_mangle]
pub extern "C" fn initialize() {
    host_read_input();
    let binary_path = host_run_command("which", &["git"]);
    if binary_path.is_empty() {
        write_state_report(&StateReport {
            manager: "system".to_string(),
            error: "git binary not found in PATH".to_string(),
            ..Default::default()
        });
        return;
    }
    let version = host_run_command("git", &["--version"]);
    write_state_report(&StateReport {
        present: true,
        reachable: true,
        binary_path,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![version],
        ..Default::default()
    });
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    host_read_input();
    let version_out = host_run_command("git", &["--version"]);
    if version_out.is_empty() {
        write_state_report(&StateReport {
            manager: "system".to_string(),
            error: "git binary not found".to_string(),
            ..Default::default()
        });
        return;
    }
    write_state_report(&StateReport {
        present: true,
        reachable: true,
        in_path: true,
        manager: "system".to_string(),
        observations: vec![format!("calibrated: {}", version_out)],
        ..Default::default()
    });
}
