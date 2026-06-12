# Phase 4: Integration System Hardening & Expansion

## Goal

Harden the integration system from its Phase 2.5 scaffolding into a production-ready contract. This phase addresses five interrelated concerns: the integration model (brand-centric manifests, multi-role dispatch, role semantics), the data model (transponder config, unified entity interface, dependency ordering), the WASM ABI (interactive auth, shell activation, command auditing), security (Captain approval, command banlist, trust persistence), and performance (module pooling, parallel dispatch, shared runtime).

---

## Security Model

Integrations execute commands on the Captain's machine. This is intentional — an integration wraps branded software and must interact with it. The threat model is an integration that claims to be legitimate (copied manifest, convincing name) but executes malicious commands.

**Defense in depth — Phase 4:**

- **Command banlist**: a hardcoded set of commands the host always rejects, regardless of what the manifest declares. Shell interpreters and privilege escalation tools cannot be whitelisted by any integration.
- **Claimed allowlist**: the manifest declares every executable the integration may call. Any `run_command` for an undeclared executable is silently rejected and logged — the Captain is never prompted for undeclared commands.
- **Captain approval**: for commands that pass both checks (declared and not banned), the host pauses dispatch and prompts the Captain the first time a specific command string is seen. Dispatch resumes only after approval.
- **Trust persistence**: Captain approvals are stored in `~/.orbiter/settings.json` keyed on `(brand, full_command_string)`. Subsequent calls with identical strings run without prompting.
- **Audit log**: every `run_command` invocation is written to `~/.orbiter/audit.log` — timestamp, brand, full command string, exit code, duration, whether it was banned, rejected by allowlist, approved, or trusted. All four outcomes are logged.
- **No ambient environment**: subprocesses inherit only `PATH`. No secrets, tokens, or session credentials leak via environment.
- **No shell output channel**: integrations report state and activation env vars. Orbiter validates `StateReport.Exports` against the manifest `[shell] exports` allowlist before emitting any shell directive.

### Command banlist

The host maintains a hardcoded banlist of commands that are rejected regardless of the manifest allowlist. Ban reason is included in the audit log.

**Shell interpreters** (execute arbitrary code; WASM logic should handle conditionals instead):
`sh`, `bash`, `zsh`, `fish`, `dash`, `ksh`, `tcsh`, `csh`, `ash`

**Privilege escalation** (no integration needs to elevate; Captain action required outside Orbiter):
`sudo`, `su`, `doas`, `pkexec`, `runas`

Other commands (`systemctl`, `launchctl`, `osascript`, `crontab`, `powershell`, etc.) are legitimate for specific roles (agent transponders checking daemon reachability, macOS integrations, etc.) and may be declared in the allowlist. They appear in the audit log and trigger Captain approval on first use.

**Limitation**: the banlist operates at the command name level, not argument level. A manifest declaring `python` is allowed to call it; the audit log captures the full argument list including `-c`/`-e` patterns. Argument-level filtering is future work.

### Captain approval flow

When `run_command` is called with a declared, non-banned command:

```
1. Look up (brand, full_command_string) in ~/.orbiter/settings.json trust entries
2. If trusted → run immediately, log as "trusted"
3. If not trusted → pause dispatch, prompt Captain:

   The nvm integration wants to run:
     nvm install 20.11.0

   [a] always allow   [o] allow once   [d] decline

4. "always allow" → write to settings.json, run, log as "approved:always"
5. "allow once"   → run, not stored, log as "approved:once"
6. "decline"      → empty output returned to guest, log as "declined"
```

Declined commands are never stored. An integration that is declined repeatedly is one the Captain should consider removing.

`nvm install 20` and `nvm install 24` are different trust entries — the Captain approves each distinct command string once.

### Unattended mode

`--unattended` flag on `jump`/`calibrate`. Displays a warning at startup:

```
WARNING: Running in unattended mode. Unapproved commands will prompt for confirmation.
New approvals will NOT be persisted. Use interactive mode to build trust.
```

In unattended mode: already-trusted commands (in settings.json) run silently. New command strings prompt inline as normal but "always allow" is downgraded to "allow once" — nothing new is written to settings.json. This preserves the property that unattended runs cannot silently accumulate trust.

### Trust storage — `~/.orbiter/settings.json`

Trust is a meta-property of the vessel's Orbiter installation, not of the universe state (StarChart). It lives in a JSON file alongside the integration directory:

```json
{
  "trust": {
    "git": {
      "git version": "allow",
      "git -C /home/captain/repos/acme status": "allow",
      "git clone https://github.com/acme/repo /home/captain/repos/acme": "allow"
    },
    "go": {
      "go version": "allow",
      "go env GOPATH": "allow"
    },
    "nvm": {
      "nvm install 20.11.0": "allow",
      "nvm use 20.11.0": "allow"
    }
  }
}
```

Only `"allow"` entries are stored. Declines are ephemeral (logged, not persisted). The file is human-readable and directly editable by the Captain. Future vessel-level settings (output preferences, update channels, etc.) will be added as top-level keys.

---

## Integration Catalog & Installation

### Integrations are not bundled in the binary

Integrations ship alongside the Orbiter binary but are not compiled into it (except the native filesystem integration). The binary embeds a **dormant catalog** — the WASM binaries and manifests for all first-party integrations — which is only consulted during `vessel init`. It is not loaded at startup.

At startup, Orbiter loads only from `~/.orbiter/integrations/`. If the directory is empty or missing, no WASM integrations are active (only native filesystem).

This keeps the binary lean, makes installed integrations auditable (the Captain can read `~/.orbiter/integrations/nvm/manifest.toml`), and lets integrations be updated or removed independently of the Orbiter binary.

### Vessel init — catalog selection

During `orbiter vessel init`, Orbiter presents a checklist of available integrations drawn from the embedded catalog. Each entry shows the manifest `name` and `description`:

```
Select integrations to install:

  [x] Native Filesystem — always included, cannot be deselected
  [ ] Go Runtime        — manage Go toolchain versions via the go distribution
  [ ] Git               — wrap the git CLI for repository operations
  [ ] nvm               — install and switch Node.js versions via nvm
  [ ] uv                — install and switch Python versions via uv
  [ ] rbenv             — install and switch Ruby versions via rbenv
  [ ] rustup            — install and manage the Rust toolchain

  Space to toggle. Enter to confirm.
```

Selected integrations are extracted from the embedded catalog and written to `~/.orbiter/integrations/<brand>/` (manifest.toml + brand.wasm). Startup then loads them normally via the plugin directory scan.

The Captain can add integrations later with `orbiter integration install <brand>` (Phase 5 scope), or drop WASM + manifest files into `~/.orbiter/integrations/<brand>/` manually.

### Native filesystem exception

`internal/integrations/native/filesystem.go` is the only integration compiled directly into the Orbiter binary. It implements `Init`/`Scan`/`Calibrate` as native Go with no subprocess calls and no manifest allowlist. It is always registered, always active, and cannot be overridden or removed.

---

## Architecture

### Integrations are brand-centric

An integration is a brand. `gh` is one integration that satisfies multiple roles. Orbiter owns the static mapping of which roles are resource-type and which are transponder-type — the integration never declares its type.

**Role taxonomy (Orbiter-owned, static):**

| Role | Type | Lifecycle contract |
|---|---|---|
| `manager` | resource | installs and manages other resources; always the dependency root for its managed brands |
| `runtime` | resource | language runtime; always has a manager (system, brew, or explicit) |
| `tool` | resource | CLI tool in PATH; may have a manager |
| `remote` | resource | local checkout of a remote source; checks tool presence + local location |
| `filesystem` | resource | local path on disk; no manager |
| `file` | transponder | credential in a file (key, cert, token file) |
| `env` | transponder | non-secret env var (profile name, registry URL, account ID) |
| `keychain` | transponder | interactive auth (username/password, OAuth, MFA) |
| `vault` | transponder | secrets manager (1Password, AWS Secrets Manager, Keychain) |
| `agent` | transponder | network or credential agent (SSH agent, VPN, GPG agent) |

The `type` field is removed from the manifest entirely. Role membership in the table above is the type declaration.

### Role semantics

**`manager`**
Installs and manages other resources. Every runtime and most tools have a manager — even "native" installs are managed by something: `manager/homebrew`, `manager/apt`, `manager/system` (OS package manager), `manager/script` (a shell script), `manager/website` (downloaded binary). There is no unmanaged runtime. The dependency graph tracks every manager → managed relationship.

`Init` = install the manager itself. `Scan` = is manager present and functional. `Calibrate` = repair or update manager. During branch calibration, managers always calibrate before their managed resources (see Dependency Ordering).

**`runtime`**
Language runtime. Always has a manager. The runtime integration receives its manager's `StateReport` in `ResolvedContext.Resources` and uses it to determine the correct activation and installation commands — only the integration knows how its brand activates under a given manager (`nvm use 20` vs `pyenv shell 3.11.0` vs `brew link node@20`).

File discovery for detection: `DetectContext.Files` carries file contents (`map[string]string`), allowing runtime integrations to read `.nvmrc`, `.python-version`, `go.mod` go directive, etc. for version requirements.

**`tool`**
CLI tool in PATH. May have a manager. `Init` = install. `Scan` = is binary present and reachable. `Calibrate` = reinstall or update.

**`remote`**
A local checkout of a remote source. Orbiter is not responsible for continuous sync — that is the Captain's concern and the sync tool's job. Orbiter checks: does the local location exist, and is the sync tool present and reachable.

Git is the exception: `Init` performs a clone. After that, `Scan`/`Calibrate` check for `.git/` in the expected filesystem location and that `tool/git` is present. No pull, no fetch — Orbiter does not manage git state beyond confirming the repository exists.

Other remotes (Dropbox, Google Drive, sftp, S3): Orbiter checks that the sync tool is installed (`tool/dropbox`, `tool/rclone`, etc.) and the local sync folder exists (`filesystem` resource). The tool handles sync entirely.

Remote integrations typically declare a `tool` dependency in `[dependencies]`.

**`filesystem`**
A local path. `Init` = `mkdir -p`. `Scan` = path exists and is accessible. `Calibrate` = ensure path exists. This is the only resource role with a native Go implementation (`internal/integrations/native/filesystem.go`) — it needs no subprocess.

**`file`** (transponder)
Credential stored in a file: SSH key, certificate, token file, `.gitconfig`, license key. Config holds `{"location": "~/.ssh/id_ed25519_acme"}`. `Scan` = file exists, is readable, has correct permissions (e.g. 600 for SSH keys). `Calibrate` = fix permissions if wrong; report missing file as unrecoverable drift (Captain must provide the file).

**`env`** (transponder)
A non-secret environment variable that must be set during a jump: profile name, registry URL, account identifier, tool configuration. **API keys, tokens, and passwords must never be stored here** — those belong in `keychain`, `vault`, or `file` transponders. The config holds the variable name and value: `{"var": "AWS_PROFILE", "value": "acme"}`. `Scan` = is the var currently set to the expected value. `Calibrate` = set it (via shell directive on next jump). The value is non-secret, so storing it in config is safe.

**`keychain`** (transponder)
Interactive auth: password-based login, OAuth, MFA. Config holds the non-secret partial: `{"username": "kent", "account": "acme"}`. Secrets are never stored. `Scan` = verify auth is currently valid (e.g. `gh auth status`). `Calibrate` = re-authenticate, using `NeedsInput` to request credentials from the Captain at jump time.

**`vault`** (transponder)
Delegates credential retrieval to a secrets manager. Config holds the item reference: `{"item": "acme-github-token", "field": "password"}`. The vault itself may require unlock via `NeedsInput` (master password, biometric). Retrieved secrets are transient — used in the current jump, never stored.

**`agent`** (transponder)
A running network or credential agent: SSH agent, GPG agent, VPN connection. Orbiter checks existence and reachability — it does not start or stop agents. That is the Captain's responsibility (shell profile, system service, manual). `Scan` = is the agent reachable (socket exists, `ssh-add -l` succeeds, VPN interface is up). `Calibrate` = report drift; remediation is manual. If the agent is not running, calibrate reports `drifted` and describes what the Captain needs to do.

VPN is an `agent` transponder. Orbiter checks if the VPN interface is up and the target network is reachable, nothing more.

### Manifest: brand + roles + contracts

```toml
[integration]
brand = "nvm"
name = "Node Version Manager"
description = "Installs and switches Node.js versions via nvm"
roles = ["manager"]

[detection]
files = [".nvmrc"]

[commands]
# Every executable this integration may call via run_command.
# Calls to unlisted executables are rejected without prompting the Captain.
allowed = ["nvm", "node", "npm"]
timeout_seconds = 30

[shell]
# Every env var this integration may return in StateReport.Exports.
# Orbiter rejects any export key not declared here before it reaches the shell.
exports = ["NVM_DIR", "NVM_BIN", "NVM_INC"]

[[config.fields]]
key = "version"
type = "string"
required = false
description = "Default Node.js version to activate on jump"

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

`name` and `description` are displayed in catalog selection at `vessel init` and in TUI integration views. They are the Captain's primary signal of what an integration is and does.

`bundle.go` registers the WASM under each `(role, brand)` pair from `roles`. The same compiled module handles all roles, branching on `ctx.Self.Role()` at runtime.

### Entity interface: unified Self

Both `models.Resource` and `models.Transponder` implement:

```go
type Entity interface {
    GetID()     string
    GetRole()   string
    GetBrand()  string
    GetConfig() string
}
```

`ResolvedContext.Self` becomes `Entity`. WASM serialization shape is unchanged from the guest's perspective — the guest sees `id`, `role`, `brand`, `config` regardless of whether the host dispatched a Resource or Transponder.

### Dependency ordering

Branch calibration follows role order within each FILO level:

```
manager → runtime → tool → remote → filesystem
```

Within a role tier, brand-level dependencies are respected: `manager/nvm` before `runtime/node` managed by nvm. The dependency graph (tracked via the `manages` field on Resource and explicit dependency declarations) determines intra-tier ordering. Parallel dispatch applies within each tier after ordering is resolved.

Transponder calibration runs after all resource tiers complete, in parallel among themselves.

### Shell activation

Integrations communicate required shell state changes through `StateReport.Exports map[string]string`. Orbiter validates every key against the manifest `[shell] exports` allowlist before emitting any directive. Undeclared keys are dropped and logged.

`filesystem` resources continue to use `StateReport.InstallDir` for the `cd` directive — native Go, no subprocess, no manifest declaration needed.

---

## Data Model Changes

### Transponder config

`models.Transponder` gains `Config string` (JSON blob). `Location string` is dropped and migrated into `{"location": "..."}` in the config blob.

```json
{"location": "~/.ssh/id_ed25519_acme"}          // file transponder (was: location column)
{"username": "kent", "account": "acme-corp"}     // keychain partial
{"var": "AWS_PROFILE", "value": "acme"}          // env transponder
```

Migration: add `config TEXT NOT NULL DEFAULT '{}'`, copy `location` → `{"location": location}` for existing rows, drop `location` column.

---

## WASM ABI Changes

### Interactive auth: NeedsInput / Responses

```go
type InputRequest struct {
    Key    string `json:"key"`
    Prompt string `json:"prompt"`
    Masked bool   `json:"masked"`
}

// StateReport gains:
NeedsInput []InputRequest    `json:"needs_input,omitempty"`

// ResolvedContext gains:
Responses  map[string]string `json:"responses,omitempty"`
Exports    map[string]string `json:"exports,omitempty"`
```

Dispatch loop (executor/lifecycle layer):
```
1. Call integration (Responses = nil)
2. If NeedsInput non-empty:
   a. CLI: prompt on stderr, masked if Masked=true
   b. TUI: render input modal
   c. Call again with Responses = collected answers
3. Repeat until NeedsInput empty or error
4. Validate StateReport.Exports against manifest [shell] exports; drop and log violations
5. Return final StateReport
```

Secrets never stored. Orbiter never persists Responses.

### run_command enforcement

Every `run_command` invocation passes through checks in order:

1. **Banlist**: command in hardcoded banlist → reject, log (`"banned": true, "reason": "..."`), no prompt.
2. **Allowlist**: command not in manifest `[commands] allowed` → reject, log (`"rejected": true, "reason": "not declared"`), no prompt.
3. **Trust check**: look up `(brand, full_command_string)` in `~/.orbiter/settings.json`.
   - Found → run, log (`"trusted": true`).
   - Not found → pause, prompt Captain inline.
4. **Captain decision**:
   - `always allow` → write to settings.json, run, log (`"approved": "always"`).
   - `allow once` → run, log (`"approved": "once"`).
   - `decline` → empty output to guest, log (`"declined": true`).
5. **Timeout**: enforced per invocation via `exec.CommandContext` using `[commands] timeout_seconds`.
6. **Environment**: subprocess inherits only `PATH`.

All outcomes are written to `~/.orbiter/audit.log` as JSON lines:

```jsonl
{"ts":"...","brand":"nvm","cmd":"nvm","args":["install","20.11.0"],"exit":0,"duration_ms":4200,"trusted":true}
{"ts":"...","brand":"nvm","cmd":"curl","args":["evil.com"],"exit":-1,"rejected":true,"reason":"not declared"}
{"ts":"...","brand":"bad","cmd":"bash","args":["-c","..."],"exit":-1,"banned":true,"reason":"shell interpreter"}
{"ts":"...","brand":"nvm","cmd":"nvm","args":["install","24.0.0"],"exit":0,"duration_ms":3100,"approved":"always"}
```

### Dynamic buffer allocation

Static 64KB buffers replaced with manifest-declared hints (`[runtime] input_buffer_kb`, `output_buffer_kb`). Default 8KB. Pool size controlled by `[runtime] pool_size` (default 4).

---

## Performance Hardening

### Shared wazero runtime

One `wazero.Runtime` shared across all integrations. Eliminates repeated JIT compilation of the host module.

### Module instance pooling

Replace `sync.Mutex` + single instance with a buffered channel pool:

```go
type WASMIntegration struct {
    manifest integrations.Manifest
    compiled wazero.CompiledModule
    pool     chan api.Module  // capacity = pool size (default 4)
}
```

Compile once on `Load()`, instantiate pool size times. Dispatch acquires from pool, defers return.

### Parallel branch dispatch

Resources calibrate in role-tier order; within each tier, parallel goroutines. Transponders calibrate in a separate parallel pass after all resource tiers complete.

### Integration catalog (startup performance)

Because integrations load from `~/.orbiter/integrations/` at startup — not from an embedded bundle — the binary startup path no longer embeds or compiles WASM modules it doesn't need. The Captain controls exactly which integrations are active.

---

## Files Created or Modified

**`internal/integrations/manifest.go`** (new)

- `Manifest`, `ManifestIntegration` (`Name`, `Description`, `Brand`, `Roles []string`, no `Type`)
- `ManifestCommands` (`Allowed []string`, `TimeoutSeconds int`)
- `ManifestShell` (`Exports []string`)
- `ManifestRuntime` (`PoolSize`, `InputBufferKB`, `OutputBufferKB`)
- `ManifestConfigField`, `ManifestConfig`
- `RoleType(role string) string` — static role→type lookup

**`internal/integrations/types.go`**

- Add `Entity` interface; `ResolvedContext.Self` → `Entity`
- `StateReport`: add `NeedsInput []InputRequest`, `Exports map[string]string`
- `ResolvedContext`: add `Responses map[string]string`
- Add `InputRequest` struct

**`internal/integrations/roles.go`**

- Add `RoleTypes map[string]string`, `RoleType()`

**`internal/models/transponder.go`**

- Add `Config string`; remove `Location string`; implement Entity

**`internal/models/resource.go`**

- Implement Entity interface

**`internal/migrations/0002_transponder_config.sql`** (new)

- Add `config`, migrate `location` values, drop `location` column

**`internal/wasm/runtime.go`** (new)

- Shared `wazero.Runtime` singleton via `sync.Once`

**`internal/wasm/audit.go`** (new)

- `AuditLog` — append-only JSON lines to `~/.orbiter/audit.log`
- Structured entries with banned/rejected/trusted/approved/declined fields

**`internal/wasm/trust.go`** (new)

- `TrustStore` — read/write `~/.orbiter/settings.json`
- `IsAllowed(brand, fullCommandString) bool`
- `Allow(brand, fullCommandString)` — persists to settings.json

**`internal/wasm/host.go`**

- Hardcoded banlist (`sh`, `bash`, `zsh`, `fish`, `dash`, `ksh`, `tcsh`, `csh`, `ash`, `sudo`, `su`, `doas`, `pkexec`, `runas`)
- Allowlist check
- Trust check + inline Captain prompt via context callback
- Timeout enforcement; PATH-only environment
- Audit log on every outcome

**`internal/wasm/loader.go`**

- Pool-based dispatch; validate `StateReport.Exports` against `[shell] exports`

**`internal/starchart/lifecycle.go`**

- `TransponderScanResult`, `TransponderCalibrateResult`
- Role-ordered resource dispatch; parallel transponder pass

**`integrations/bundle.go`**

- Embed changes to dormant catalog (not auto-registered at startup)
- `CatalogEntries() []CatalogEntry` — list of available integrations with Name + Description from manifest
- `InstallFromCatalog(brand, destDir string) error` — extracts wasm + manifest to destDir

**`internal/commands/vessel.go`** (or existing init command)

- `vessel init` presents catalog checklist, installs selected integrations to `~/.orbiter/integrations/`

**`integrations/golang/manifest.toml`**, **`integrations/git/manifest.toml`**

- Add `name`, `description`; update to `roles = [...]`; add `[commands]`, `[shell]`, `[runtime]`

---

## Out of Scope

- `Detect` dispatch from `planet init` (Phase 5)
- `vault` transponder implementation
- `orbiter integration install <brand>` CLI command (Phase 5)
- WASM module signing and provenance verification (future)
- Subprocess sandboxing via seccomp/namespaces/sandbox profiles (future)
- Argument-level command filtering (blocking `-c`/`-e` on interpreters) (future)
- Integration versioning and upgrade paths
