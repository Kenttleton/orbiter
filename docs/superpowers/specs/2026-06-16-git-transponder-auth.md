# Git Integration — Transponder Auth Extension

## Goal

Extend the existing `integrations/git/` Rust WASM integration to handle all five
transponder access roles in addition to its existing `tool` role. The result is one
integration, one brand (`git`), one WASM binary — dispatching on `Self.role` at
runtime. Cross-platform throughout (macOS, Linux, Windows).

---

## Background

The current integration claims `roles = ["tool"]` and verifies the git binary is
present and reachable. Transponder support adds the five access-mechanism roles from
the Constitution: `file`, `env`, `agent`, `keychain`, `vault`.

Key invariants that never change:

- Transponder configs hold **pointers only** — no secrets, no tokens, no passwords.
- The remote URL never lives in the transponder config. It arrives via
  `Resources["remote"][0]` from the branch crawl.
- Secrets resolved by Orbiter (from vault integrations or interactive prompts) arrive
  transiently in `Responses`. The git integration uses them but never stores or logs them.
- Orbiter owns all state management. The integration owns all interaction with external
  tools and the local environment.

---

## Context Flow

```text
Branch crawl (vessel → planet)
  │
  ├── BranchLevel.Transponders  →  Self (the git transponder, role + config)
  └── BranchLevel.Resources     →  Resources["remote"][0]  (remote URL source)

Orbiter pre-resolves vault secrets (if Self.role == "vault")
  → Responses["credential"] = <transient value, never persisted>

ResolvedContext passed to git integration:
  {
    "platform":     { "os": "darwin|linux|windows", "arch": "amd64|arm64" },
    "self":         { "id": "...", "role": "file", "brand": "git", "config": {...} },
    "resources":    { "remote": [{ "resource": { "config": "{\"url\":\"git@...\"}" } }] },
    "transponders": {},
    "responses":    {}   // populated only for vault role
  }
```

---

## Config Shapes (stored in Star Chart)

Each config is a JSON object in `transponders.config`. No secrets.

### `file + git`

```json
{ "location": "~/.ssh/id_ed25519_acme" }
```

`location` is a path to the private key file. Protocol (SSH vs HTTPS token) is inferred
from the remote URL: `git@` → SSH key; `https://` → token file or `.netrc`.

### `env + git`

```json
{ "variable": "GIT_TOKEN" }
```

`variable` is the name of the environment variable that holds the credential. The value
is never read by the integration — only its presence is checked during scan.

### `agent + git`

```json
{ "socket": "/path/to/agent.sock" }
```

`socket` is optional. When absent the integration uses the ambient agent socket
(`SSH_AUTH_SOCK` on macOS/Linux, the Windows OpenSSH Agent pipe on Windows). Providing
an explicit path supports non-default agents (e.g. 1Password's dedicated SSH agent).

### `keychain + git`

```json
{}
```

No config fields. The credential store is selected by platform at runtime. The remote
host is parsed from the `remote` resource URL and used as the lookup key.

### `vault + git`

```json
{}
```

No config fields. Orbiter resolves the credential via the vault integration upstream,
then passes it to the git integration in `Responses["credential"]`. The git integration
treats this identically to `env` — it receives a transient token and configures git to
use it, without knowing its origin.

---

## Handler Contracts

All four exported handlers (`detect`, `initialize`, `scan`, `calibrate`) dispatch as
follows:

```text
read Self.role from context
  "tool"                  → tool module (existing behavior)
  "file"|"env"|"agent"
  |"keychain"|"vault"     → auth dispatcher (new)
```

### `detect`

Unchanged: returns `detected: true` when `.git/config` is present in the files map.
Suggests `{ role: "tool", brand: "git" }`. Transponder roles are never suggested by
detect — they are registered explicitly via `transponder add`.

### `initialize` and `scan`

Read-only. Verify the pointer is valid and the auth path is usable. Return a
`StateReport` with observations. No mutations to git config, SSH config, or any file.

### `calibrate`

May mutate. Repair missing config, fix key permissions, configure credential helpers.
May emit `NeedsInput` to request interactive input when the pointer requires user action
(e.g. adding a key to the agent, storing a credential in the keychain).

---

## Per-Role Behavior

### `tool` (existing, unchanged)

`scan`/`initialize`: `which git` → `git --version` → report present/reachable.
`calibrate`: same as scan (git is a system binary; Orbiter does not install it).

---

### `file`

**Scan:**

1. Expand `~` in `config.location` to the user home directory.
2. `stat` the file (cross-platform: `git ls-files --error-unmatch` is not applicable
   here; use platform-appropriate stat):
   - macOS/Linux: `stat -c %a <path>` → check permissions are 600 or 400
   - Windows: file existence check via `git` or filesystem; permission model differs
3. If protocol is SSH (remote URL starts with `git@`): run `ssh-keygen -l -f <path>`
   to verify the file is a valid key. Report key fingerprint in observations (not the
   key itself).
4. If protocol is HTTPS (remote URL starts with `https://`): verify file is non-empty
   and readable.

**Calibrate:**

1. macOS/Linux: `chmod 600 <path>` if permissions are too open.
2. Windows: emit observation that permissions cannot be set automatically.
3. Optionally configure an SSH config entry (`Host <remote-host> IdentityFile <path>`)
   if not already present — write to `~/.ssh/config`.
4. Export `GIT_SSH_COMMAND` = `ssh -i <path> -o IdentitiesOnly=yes` for the session.

---

### `env`

**Scan:**

1. Parse variable name from `config.variable`.
2. Check variable is non-empty in the current environment. Do not read its value.
3. If remote is HTTPS: report reachable=true if variable is set, false otherwise.
4. Report observations: `"GIT_TOKEN is set"` / `"GIT_TOKEN is not set"`. Never log the
   value.

**Calibrate:**

1. If variable is unset: emit `NeedsInput` with `key="credential"`, `prompt="Enter
   value for <variable>"`, `masked=true`. Orbiter collects the value interactively.
2. Export the variable for the session via `StateReport.Exports`. The value is in
   `Responses["credential"]` — emit it as an export. It is never stored.
3. Optionally configure `git config credential.helper` to use the env var via a
   generated `GIT_ASKPASS` helper script path.

---

### `agent`

The agent role resolves the socket in priority order:

```text
1. External agent transponder in branch context
   → Orbiter ran the agent integration upstream; socket path arrives via
     Responses["socket"] or SSH_AUTH_SOCK is already exported into the environment
   → use it directly, skip platform default discovery

2. No external agent in branch context
   → fall back to platform default agent socket
```

This means a Captain can attach any `agent + <brand>` transponder to their callsign
(e.g. 1Password SSH agent, Secretive, gpg-agent) and git will route through it
automatically. If none is attached, git uses whatever agent the platform provides.

**Scan:**

1. Check `Transponders["agent"]` in the resolved context.
   - If populated: an external agent transponder is in the branch. Check
     `Responses["socket"]` for an explicit socket path, or check that `SSH_AUTH_SOCK`
     is set in the environment (exported by the agent integration upstream). Run
     `ssh-add -l` against that socket to verify the agent is alive and has identities.
     Report `present: true, reachable: true` if identities are loaded.
   - If absent: no external agent in branch — proceed to platform default below.
2. Platform default path (no external agent):
   - macOS/Linux: use ambient `SSH_AUTH_SOCK`. Run `ssh-add -l`.
   - Windows: connect to the OpenSSH Agent named pipe
     (`\\.\pipe\openssh-ssh-agent`). Run `ssh-add -l`.
   - Exit 0 with identities → `present: true, reachable: true`
   - Exit 1 ("agent has no identities") → `present: true, reachable: false`
   - Exit 2 (cannot connect) → `present: false, reachable: false`
3. Report identity count in observations (not fingerprints of specific keys).

**Calibrate:**

1. External agent path: if `Responses["socket"]` is populated, export `SSH_AUTH_SOCK`
   pointing to it via `StateReport.Exports`. If the agent has no identities, emit
   `NeedsInput` prompting the Captain to add a key — do not run `ssh-add` on their
   behalf.
2. Platform default path (no external agent):
   - macOS/Linux: if `SSH_AUTH_SOCK` is unset, attempt `ssh-agent -s` to start the
     default agent; export `SSH_AUTH_SOCK` in `StateReport.Exports`.
   - Windows: check if the OpenSSH Agent service is running. Report status; Orbiter
     does not start Windows services.
3. If agent is running but has no identities in either path: emit `NeedsInput`
   prompting the Captain to run `ssh-add /path/to/key`.

---

### `keychain`

The keychain role resolves credentials in priority order:

```text
1. External keychain transponder in branch context
   → Responses["credential"] populated by Orbiter upstream
   → use it directly, skip git credential helper entirely

2. No external keychain in branch context
   → fall back to git's platform default credential helper
```

This means a Captain can attach any `keychain + <brand>` transponder to their callsign
or planet and git will use it automatically. If none is attached, git handles keychain
access natively using whatever is available on the platform.

**Scan:**

1. Check `Transponders["keychain"]` in the resolved context.
   - If populated: an external keychain transponder is in the branch. Check
     `Responses["credential"]` — if present, report `present: true, reachable: true`
     with observation `"credential resolved by external keychain"`. If absent, Orbiter
     has not resolved it yet — report `present: false` and emit `NeedsInput` with
     `key="credential"`.
   - If absent: no external keychain in branch — proceed to platform default below.
2. Platform default path (no external keychain):
   - Parse the remote host from `Resources["remote"][0].Resource.Config["url"]`.
   - Run `git credential fill` with `protocol` and `host` piped on stdin. This
     delegates to whatever `credential.helper` is configured in git — no platform
     branching needed at this layer; git owns the platform dispatch.
   - A successful response → `present: true, reachable: true`. Discard the password
     immediately. Never log it.
   - A failed response → `present: false`. Report which helper git attempted in
     observations.

**Calibrate:**

1. External keychain path (Responses populated): configure git for the session via
   `GIT_ASKPASS` or `http.extraHeader` export. Identical to `vault` calibrate path.
2. Platform default path (no external keychain):
   - Check `git config credential.helper`. If unconfigured, set the platform default:
     - macOS: `git config --global credential.helper osxkeychain`
     - Linux: `git config --global credential.helper /usr/share/doc/git/contrib/credential/libsecret/git-credential-libsecret`
       or `manager` if GCM is installed. Emit an observation if neither is found —
       Linux keychain support is not guaranteed.
     - Windows: `git config --global credential.helper manager` (GCM bundled with
       Git for Windows).
   - If helper is configured but no entry exists for the remote host: emit `NeedsInput`
     prompting the Captain to authenticate via the helper's interactive flow (e.g.
     `git credential approve` or browser OAuth via GCM).
3. Export no shell variables in either path — credential helpers are invoked by git
   internally.

---

### `vault`

**Scan:**

1. Check `Responses["credential"]` is populated (Orbiter resolved the secret upstream).
2. If present: `present: true, reachable: true`. Report `"credential resolved by
   Orbiter"` in observations (no value).
3. If absent: `present: false, reachable: false`. Report that the vault credential was
   not provided. Emit `NeedsInput` with `key="credential"` so Orbiter knows to resolve
   it before the next call.

**Calibrate:**

Same as scan. The git integration does not call vault CLIs — resolution is entirely
Orbiter's responsibility. If `Responses["credential"]` is populated, configure git for
the session via `GIT_ASKPASS` or `http.extraHeader` export, identical to the `env`
calibrate path.

---

## Cross-Platform Matrix

| Concern | macOS | Linux | Windows |
| --- | --- | --- | --- |
| File stat | `stat -c %a` | `stat -c %a` | skip permission check, existence only |
| File permissions | `chmod 600` | `chmod 600` | emit observation only |
| SSH config | `~/.ssh/config` | `~/.ssh/config` | `%USERPROFILE%\.ssh\config` |
| Agent (external) | `Responses["socket"]` or exported `SSH_AUTH_SOCK` | `Responses["socket"]` or exported `SSH_AUTH_SOCK` | `Responses["socket"]` or exported `SSH_AUTH_SOCK` |
| Agent (default) socket | `SSH_AUTH_SOCK` | `SSH_AUTH_SOCK` | Named pipe (no socket path) |
| Agent (default) start | `ssh-agent -s` | `ssh-agent -s` | Windows service (not started by Orbiter) |
| Keychain (external) | `Responses["credential"]` | `Responses["credential"]` | `Responses["credential"]` |
| Keychain (default) | `credential-osxkeychain` | `credential-libsecret` or GCM | `credential-manager` (GCM) |
| Home dir expansion | `$HOME` | `$HOME` | `%USERPROFILE%` |
| SSH binary | `ssh` | `ssh` | `ssh` (OpenSSH bundled with Git for Windows) |

Platform is read from `context.platform.os` at the top of each handler. All platform
branches are exhaustive — Windows always has a defined code path, even if that path is
"emit observation, manual action required."

---

## Updated Manifest

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
  "which",        # tool detection on macOS/Linux
  "ssh-keygen",   # file: key fingerprint verification
  "ssh-add",      # agent: identity list and add
  "ssh-agent",    # agent: start agent on macOS/Linux
  "stat",         # file: permission check on macOS/Linux
  "chmod",        # file: permission repair on macOS/Linux
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

`security`, `git-credential-manager`, and `git-credential-libsecret` are invoked
indirectly via `git credential-<helper>` and do not need to appear in the allowlist.

---

## Revised Source Structure

The single `src/lib.rs` file is replaced by a module hierarchy. All modules compile to
the same `cdylib` — no runtime overhead.

```text
integrations/git/src/
├── lib.rs          # ABI entry points (detect, initialize, scan, calibrate)
│                   # reads context, dispatches to tool or auth module
├── host.rs         # host ABI wrappers: read_input, write_output, run_command
├── context.rs      # parse ResolvedContext from input bytes; extract remote URL,
│                   # platform, self role/config, responses
├── report.rs       # StateReport and DetectReport serialization (hand-built JSON,
│                   # no encoding/json equivalent needed in Rust but kept consistent)
├── tool.rs         # role=tool: git binary verification (existing behavior, extracted)
└── auth/
    ├── mod.rs      # dispatch_auth(role, ctx) → calls appropriate submodule
    ├── file.rs     # role=file: key/token file verification and SSH config setup
    ├── env.rs      # role=env: environment variable presence check and ASKPASS config
    ├── agent.rs    # role=agent: SSH agent socket verification and start
    ├── keychain.rs # role=keychain: platform credential helper check and configuration
    └── vault.rs    # role=vault: Responses["credential"] check and session config
```

### `lib.rs` shape (entry points only)

```rust
mod host;
mod context;
mod report;
mod tool;
mod auth;

use context::ResolvedContext;

#[no_mangle]
pub extern "C" fn detect() {
    let input = host::read_input();
    let detected = context::has_git_config(&input);
    let version = if detected { tool::git_version() } else { String::new() };
    report::write_detect(detected, "tool", "git", &version);
}

#[no_mangle]
pub extern "C" fn initialize() { dispatch(false); }

#[no_mangle]
pub extern "C" fn scan() { dispatch(false); }

#[no_mangle]
pub extern "C" fn calibrate() { dispatch(true); }

fn dispatch(calibrate: bool) {
    let input = host::read_input();
    let ctx = match context::parse(&input) {
        Ok(c) => c,
        Err(e) => { report::write_error(&e); return; }
    };
    match ctx.self_role() {
        "tool" => tool::run(&ctx, calibrate),
        _ => auth::dispatch(&ctx, calibrate),
    }
}
```

### `auth/mod.rs` shape

```rust
pub mod agent;
pub mod env;
pub mod file;
pub mod keychain;
pub mod vault;

use crate::context::ResolvedContext;

pub fn dispatch(ctx: &ResolvedContext, calibrate: bool) {
    match ctx.self_role() {
        "file"     => file::run(ctx, calibrate),
        "env"      => env::run(ctx, calibrate),
        "agent"    => agent::run(ctx, calibrate),
        "keychain" => keychain::run(ctx, calibrate),
        "vault"    => vault::run(ctx, calibrate),
        role       => crate::report::write_error(
                          &format!("unsupported transponder role: {}", role)
                      ),
    }
}
```

---

## Files Changed

| File | Change |
| --- | --- |
| `integrations/git/manifest.toml` | Add transponder roles, commands, shell exports, remote dependency |
| `integrations/git/src/lib.rs` | Replace flat file with module entry points + dispatch |
| `integrations/git/src/host.rs` | Extract host ABI wrappers from lib.rs |
| `integrations/git/src/context.rs` | New: parse ResolvedContext, extract remote URL, platform, role |
| `integrations/git/src/report.rs` | Extract StateReport/DetectReport serialization from lib.rs |
| `integrations/git/src/tool.rs` | Extract existing tool verification logic from lib.rs |
| `integrations/git/src/auth/mod.rs` | New: auth role dispatcher |
| `integrations/git/src/auth/file.rs` | New: file role implementation |
| `integrations/git/src/auth/env.rs` | New: env role implementation |
| `integrations/git/src/auth/agent.rs` | New: agent role implementation |
| `integrations/git/src/auth/keychain.rs` | New: keychain role implementation |
| `integrations/git/src/auth/vault.rs` | New: vault role implementation |

`register.go` and `generate.go` are unchanged.
