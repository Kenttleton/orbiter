mod host;
mod context;
mod report;
mod tool;
mod auth;

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let detected = context::has_git_config(&input);
    let version = if detected { tool::git_version() } else { String::new() };
    report::write_detect(detected, "tool", "git", &version);
}

#[no_mangle]
pub extern "C" fn initialize() {
    dispatch(false);
}

#[no_mangle]
pub extern "C" fn scan() {
    dispatch(false);
}

#[no_mangle]
pub extern "C" fn calibrate() {
    dispatch(true);
}

fn dispatch(calibrate: bool) {
    let input = host::read_input();
    let ctx = match context::parse(&input) {
        Ok(c) => c,
        Err(e) => { report::write_error(&e); return; }
    };
    match ctx.self_entity.role.as_str() {
        "tool" => tool::run(&ctx, calibrate),
        _      => auth::dispatch(&ctx, calibrate),
    }
}
