# Git Integration — Transponder Auth Extension Plan

**Spec:** `docs/superpowers/specs/2026-06-16-git-transponder-auth.md`

**Goal:** Extend `integrations/git/` (Rust WASM, `cdylib`) to handle all five
transponder access roles (`file`, `env`, `agent`, `keychain`, `vault`) in addition to
the existing `tool` role. Dispatch on `Self.role` at runtime. Cross-platform throughout.

**Constraint:** No worktrees. Work directly on main.

---

## Task 1 — Manifest update

**File:** `integrations/git/manifest.toml`

Update the manifest to declare transponder roles, new allowed commands, shell exports,
and transponder dependencies:

```toml
[integration]
brand = "git"
name = "Git"
description = "Verifies the git tool and authenticates git remote operations"
roles = ["tool", "file", "env", "agent", "keychain", "vault"]

[detection]
files = [".git/config"]

[commands]
allowed = [
  "git",
  "which",
  "ssh-keygen",
  "ssh-add",
  "ssh-agent",
  "stat",
  "chmod",
]
timeout_seconds = 30

[shell]
exports = ["SSH_AUTH_SOCK", "GIT_SSH_COMMAND", "GIT_ASKPASS"]

[dependencies]
[dependencies.resources]
remote = []

[dependencies.transponders]
agent    = []
keychain = []
```

---

## Task 2 — Foundation modules (host, report, context)

Extract and create the three foundation modules that all other modules depend on.

### `integrations/git/src/host.rs` (extract from lib.rs)

Move the three host ABI wrappers out of `lib.rs` into `host.rs`:

```rust
pub fn read_input() -> Vec<u8>
pub fn write_output(data: &[u8])
pub fn run_command(cmd: &str, args: &[&str]) -> String
```

Keep the private `build_cmd_spec` and `json_str` helpers in `host.rs`.

### `integrations/git/src/report.rs` (extract from lib.rs)

Move `StateReport`, `write_state_report`, `write_detect_report` out of `lib.rs`.
Rename to use `pub` visibility. Keep hand-built JSON serialization.

Add a `write_error` helper:
```rust
pub fn write_error(msg: &str)
```
Writes `{"present":false,"reachable":false,"in_path":false,"manager":"","error":"<msg>"}`.

### `integrations/git/src/context.rs` (new)

Parse `ResolvedContext` from raw input bytes. The context is JSON. Use manual byte
scanning (no serde — keep binary small).

Provide:

```rust
pub struct Platform { pub os: String, pub arch: String }

pub struct SelfEntity {
    pub role: String,
    pub brand: String,
    pub config: String,   // raw JSON object string
}

pub struct RemoteResource {
    pub url: String,      // parsed from Resources["remote"][0].resource.config["url"]
}

pub struct ResolvedContext {
    pub platform: Platform,
    pub self_entity: SelfEntity,
    pub remote: Option<RemoteResource>,
    pub has_agent_transponder: bool,    // Transponders["agent"] is non-empty
    pub has_keychain_transponder: bool, // Transponders["keychain"] is non-empty
    pub responses: std::collections::HashMap<String, String>,
}

pub fn parse(input: &[u8]) -> Result<ResolvedContext, String>
pub fn has_git_config(input: &[u8]) -> bool  // for detect handler
```

Config field extraction helper (used by auth modules to read from `SelfEntity.config`):
```rust
pub fn config_str(config: &str, key: &str) -> Option<String>
```

---

## Task 3 — Tool module + lib.rs module scaffold

### `integrations/git/src/tool.rs` (extract from lib.rs)

Move existing tool logic (binary detection, version check) into `tool.rs`:

```rust
pub fn git_version() -> String       // runs `git --version`, returns version string
pub fn run(ctx: &ResolvedContext, calibrate: bool)  // existing scan/calibrate behavior
```

`run` for tool role: `calibrate` flag is ignored (git is a system binary Orbiter does
not install). Scan and calibrate behave identically — verify binary is present.

### `integrations/git/src/lib.rs` (rewrite as module entry points)

Replace the flat file with module declarations and ABI entry points only:

```rust
mod host;
mod context;
mod report;
mod tool;
mod auth;

#[no_mangle] pub extern "C" fn detect()     { ... }
#[no_mangle] pub extern "C" fn initialize() { dispatch(false); }
#[no_mangle] pub extern "C" fn scan()       { dispatch(false); }
#[no_mangle] pub extern "C" fn calibrate()  { dispatch(true); }

fn dispatch(calibrate: bool) {
    let input = host::read_input();
    let ctx = match context::parse(&input) { ... };
    match ctx.self_entity.role.as_str() {
        "tool" => tool::run(&ctx, calibrate),
        _      => auth::dispatch(&ctx, calibrate),
    }
}
```

At this point `auth` is an empty stub module (just `pub fn dispatch`). The code must
compile after this task. Run `cargo build --target wasm32-unknown-unknown --release`
to verify.

---

## Task 4 — auth/file.rs

**File:** `integrations/git/src/auth/file.rs`

Implement the `file` transponder role. Read from `SelfEntity.config`:
- `location` — path to SSH key or HTTPS token file (required)

Remote URL comes from `ctx.remote.url`. Protocol inferred:
- `git@` prefix → SSH key file
- `https://` prefix → token/credential file

**scan (read-only):**
1. Expand `~` in location using platform home dir (`$HOME` on macOS/Linux,
   `%USERPROFILE%` on Windows)
2. macOS/Linux: run `stat -c %a <path>` — report permissions in observations
3. Windows: verify file exists via `git ls-remote --get-url` (indirect) or just report
   existence; permission model differs, skip permission check
4. SSH path: run `ssh-keygen -l -f <path>` — report key fingerprint in observations
   (not the key content)
5. HTTPS path: report file is present and non-empty

**calibrate:**
1. macOS/Linux: if permissions are not 600/400, run `chmod 600 <path>`
2. Windows: emit observation that permissions cannot be set automatically
3. SSH path: write SSH config entry if not present:
   ```
   Host <remote-host>
     IdentityFile <expanded-location>
   ```
   SSH config path: `~/.ssh/config` (macOS/Linux), `%USERPROFILE%\.ssh\config` (Windows)
4. Export `GIT_SSH_COMMAND=ssh -i <path> -o IdentitiesOnly=yes` via StateReport.Exports

---

## Task 5 — auth/env.rs and auth/agent.rs

### `integrations/git/src/auth/env.rs`

Implement the `env` transponder role. Config field:
- `variable` — environment variable name holding the credential (required)

**scan:**
1. Check if `variable` is set in environment. Do not read its value.
2. Report `present: true` if set, `present: false` if not.
3. Observation: `"<VARNAME> is set"` or `"<VARNAME> is not set"`. Never log the value.

**calibrate:**
1. If variable is unset: emit `NeedsInput` with `key="credential"`,
   `prompt="Enter value for <variable>"`, `masked=true`
2. If `Responses["credential"]` is populated: export the variable via
   `StateReport.Exports`. Value comes from Responses — never stored.
3. Report observation that the variable has been configured for this session.

### `integrations/git/src/auth/agent.rs`

Implement the `agent` transponder role. Priority:

```
1. External agent transponder in branch (ctx.has_agent_transponder == true)
   → Responses["socket"] or SSH_AUTH_SOCK already exported by upstream
2. No external agent → platform default
```

Config field:
- `socket` — optional explicit socket path override

**scan:**
1. If `ctx.has_agent_transponder`:
   - Check `ctx.responses["socket"]` for explicit socket, else check `SSH_AUTH_SOCK`
   - Run `ssh-add -l` with that socket
2. Platform default:
   - macOS/Linux: use `SSH_AUTH_SOCK` from environment
   - Windows: use named pipe `\\.\pipe\openssh-ssh-agent` (pass directly to ssh-add)
3. Interpret `ssh-add -l` exit code:
   - 0 + output → identities loaded → `present: true, reachable: true`
   - 1 ("no identities") → `present: true, reachable: false`
   - 2 (cannot connect) → `present: false, reachable: false`
4. Report identity count in observations (not fingerprints)

**calibrate:**
1. External path: if `Responses["socket"]` is set, export `SSH_AUTH_SOCK` pointing to it
2. Default path, socket unset (macOS/Linux): run `ssh-agent -s`, export `SSH_AUTH_SOCK`
3. Default path, Windows: report agent service status, emit observation that Orbiter
   does not start Windows services
4. If agent alive but no identities: emit `NeedsInput` prompting Captain to run
   `ssh-add /path/to/key`. Do not run ssh-add on their behalf.

---

## Task 6 — auth/keychain.rs and auth/vault.rs

### `integrations/git/src/auth/keychain.rs`

Implement the `keychain` transponder role. Priority:

```
1. External keychain transponder in branch (ctx.has_keychain_transponder == true)
   → Responses["credential"] populated by Orbiter upstream
   → use it directly

2. No external keychain → git's platform default credential helper
```

**scan:**
1. External path: check `ctx.responses["credential"]`
   - Present → `present: true, reachable: true`, observation: "credential resolved by
     external keychain". Never log value.
   - Absent → emit `NeedsInput` with `key="credential"`, `masked=true`
2. Platform default path: parse remote host from `ctx.remote.url`.
   Run `git credential fill` with `protocol=<proto>\nhost=<host>\n\n` on stdin.
   A successful `password=` line → present. Discard immediately. Never log.
   Report which helper git attempted in observations.

**calibrate:**
1. External path: if Responses populated, configure git for the session via
   `GIT_ASKPASS` export or `http.extraHeader "AUTHORIZATION: Bearer <token>"` export.
   Value comes from Responses — never stored.
2. Platform default path: check `git config credential.helper`
   - Unconfigured → set platform default:
     - macOS: `git config --global credential.helper osxkeychain`
     - Linux: attempt `git config --global credential.helper libsecret`; if not
       available, report GCM as alternative
     - Windows: `git config --global credential.helper manager`
   - Configured but no entry for host: emit `NeedsInput` prompting Captain to
     authenticate via the helper's interactive flow.

### `integrations/git/src/auth/vault.rs`

Implement the `vault` transponder role. Orbiter always resolves the secret upstream
before calling this handler. The git integration is a consumer only.

**scan:**
1. Check `ctx.responses["credential"]`
   - Present → `present: true, reachable: true`, observation: "credential resolved by
     Orbiter". Never log value.
   - Absent → `present: false, reachable: false`. Emit `NeedsInput` with
     `key="credential"`, `masked=true` so Orbiter knows to resolve upstream.

**calibrate:**
1. Same check as scan.
2. If Responses populated: configure git for the session via `GIT_ASKPASS` or
   `http.extraHeader` export. Identical to keychain external path.

---

## Task 7 — auth/mod.rs + final compile verification

### `integrations/git/src/auth/mod.rs`

Wire the auth dispatcher:

```rust
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
```

**Final verification:**

Run `cargo build --target wasm32-unknown-unknown --release` from
`integrations/git/`. The build must succeed with zero errors and zero warnings
(treat warnings as errors: `RUSTFLAGS="-D warnings"`).

Confirm the output WASM is written to `target/wasm32-unknown-unknown/release/git.wasm`.
Copy it to `integrations/git/git.wasm` (committed binary).

Run existing tests: `cargo test` (native target, not WASM — for any unit tests present).
