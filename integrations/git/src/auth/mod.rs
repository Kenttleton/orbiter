pub mod agent;
pub mod env;
pub mod file;
pub mod keychain;
pub mod vault;

use crate::context::ResolvedContext;

pub fn dispatch(ctx: &ResolvedContext, calibrate: bool) {
    match ctx.self_entity.role.as_str() {
        "file"     => file::run(ctx, calibrate),
        "env"      => env::run(ctx, calibrate),
        "agent"    => agent::run(ctx, calibrate),
        "keychain" => keychain::run(ctx, calibrate),
        "vault"    => vault::run(ctx, calibrate),
        role => crate::report::write_error(
            &format!("unsupported transponder role: {}", role)
        ),
    }
}
