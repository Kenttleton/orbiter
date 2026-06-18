mod host;

// Stub: returns safe zero-value responses for all handlers.
// Full implementation follows in subsequent tasks.

#[no_mangle]
pub extern "C" fn detect() {
    let _input = host::read_input();
    host::write_output(b"{\"detected\":false}");
}

#[no_mangle]
pub extern "C" fn initialize() {
    let _input = host::read_input();
    host::write_output(b"{\"present\":false,\"reachable\":false,\"in_path\":false,\"manager\":\"system\"}");
}

#[no_mangle]
pub extern "C" fn scan() {
    initialize();
}

#[no_mangle]
pub extern "C" fn calibrate() {
    let _input = host::read_input();
    host::write_output(b"{\"present\":false,\"reachable\":false,\"in_path\":false,\"manager\":\"system\"}");
}
