mod ffi {
    #[link(wasm_import_module = "orbiter")]
    extern "C" {
        pub fn read_input(ptr: *mut u8, max: u32) -> u32;
        pub fn write_output(ptr: *const u8, len: u32);
        pub fn run_command(
            spec_ptr: *const u8,
            spec_len: u32,
            out_ptr: *mut u8,
            out_max: u32,
        ) -> u32;
    }
}

pub fn read_input() -> Vec<u8> {
    let mut buf = vec![0u8; 64 * 1024];
    let n = unsafe { ffi::read_input(buf.as_mut_ptr(), buf.len() as u32) };
    buf.truncate(n as usize);
    buf
}

pub fn write_output(data: &[u8]) {
    unsafe { ffi::write_output(data.as_ptr(), data.len() as u32) };
}

pub fn run_command(cmd: &str, args: &[&str]) -> String {
    let spec = build_cmd_spec(cmd, args);
    let mut out = vec![0u8; 64 * 1024];
    let n = unsafe {
        ffi::run_command(
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

pub(crate) fn json_str(s: &str) -> String {
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
