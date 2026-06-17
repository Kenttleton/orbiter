# Integration Catalog — Community Example Suite

**Date:** 2026-06-16
**Status:** Spec

---

## Overview

This spec defines a catalog of 22 integrations covering all 10 Orbiter roles across 6 guest languages. The goals are:

1. Full role coverage for a software engineer's daily environment (runtimes, managers, tools, remotes, filesystem, and all five transponder roles)
2. One clear community example per supported guest language (TinyGo, Rust, AssemblyScript, Zig, C/wasi-sdk)
3. Establish the best-practice JSON library pattern for each language so integration authors have a reference to follow

Each integration ships as a WASM module + `manifest.toml` under `integrations/<brand>/` and is registered as a bundled integration compiled into the `orbit` binary.

---

## Role Taxonomy

**Resource roles** — capability providers:

- `runtime` — language runtimes (node, python, go, rust)
- `manager` — version and package managers (nvm, brew, asdf, uv, rustup)
- `tool` — CLI tools (git, docker, vscode, make, just)
- `remote` — remote services (github, google-drive)
- `filesystem` — filesystem resources (local paths)

**Transponder roles** — auth/credential providers:

- `file` — credential files (.env, key files)
- `env` — environment variables containing secrets
- `keychain` — system keychain credential stores
- `vault` — secret vault services (1Password, HashiCorp Vault)
- `agent` — process agents that provide auth on demand (SSH agent, gh auth)

Resources declare dependencies on transponders for their auth layer. A resource never becomes a transponder — the credential strategy is swappable without touching the resource integration.

---

## Host Convention: `ORBITER_CWD`

The `filesystem/local` Zig integration introduces the first host-reserved export key. `ORBITER_CWD` in `StateReport.Exports` signals the host to call `os.Chdir` and emit `cd` to the shell eval output during the `jump` lifecycle, rather than exporting it as a shell variable.

It must be declared in `[shell] exports` in the manifest like any other export. The host intercepts it before shell emission. This establishes the pattern for future host-side side effects that are not shell variables.

---

## Catalog

### 7-Spec Structure

| Spec | Integrations |
|---|---|
| TinyGo (phased) | `runtime/golang` (updated), `runtime/node`, `tool/make`, `file/dotenv` |
| Rust | `runtime/python`, `runtime/rust`, `manager/brew`, `manager/uv`, `manager/rustup`, `tool/docker`, `keychain/macos`, `vault/onepassword`, `agent/ssh` |
| AssemblyScript | `manager/nvm`, `tool/just`, `env/shell` |
| Zig | `manager/asdf`, `filesystem/local` |
| C/wasi-sdk | `tool/vscode` |
| GitHub (standalone) | `tool`+`remote`+`agent` / `github` |
| Google (standalone) | `agent/google-auth`, `remote/google-drive` |

---

### TinyGo Integrations (Phased)

TinyGo's `wasm-unknown` target is proven but has known stdlib restrictions (`encoding/json` and `strings.Builder` both crash at runtime). This spec phases the JSON library decision before delivering new integrations.

#### Phase 1 — JSON Library Evaluation

**Step 1a — Library trial:** Add `github.com/tidwall/gjson` and `github.com/tidwall/sjson` to `go.mod`. Verify both compile and execute correctly under `tinygo build -target=wasm-unknown`. Write a minimal guest module that reads a JSON input with gjson and writes a JSON output with sjson. Confirm no runtime traps.

**Step 1b (library works):** Replace the hand-rolled `jsonBytes` helpers in `runtime/golang` with gjson/sjson. Remove the hand-rolled code. Update `docs/integrations.md` to recommend gjson/sjson as the standard TinyGo JSON pattern.

**Step 1b (library fails):** Audit the hand-rolled helpers against the full range of JSON patterns an integration may encounter: nested objects, arrays, numbers, booleans, null values, unicode, escaped strings, empty values, and deeply nested paths. Fix any gaps. Extract the verified helpers to `integrations/sdk/tinygo/` as a shared importable package. Update `docs/integrations.md` to reference the SDK package.

Either path produces a single verified JSON pattern. All new TinyGo integrations in Phase 2 consume whichever pattern Phase 1 established.

#### Phase 2 — New TinyGo Integrations

**`runtime/node`**
- Detect: `package.json` present in CWD
- Scan/Calibrate: `node --version`, `which node`
- Suggested resource: `role=runtime, brand=node`
- Community showcase: demonstrates gjson input parsing for `files` map inspection

**`tool/make`**
- Detect: `Makefile` or `GNUmakefile` present in CWD
- Scan/Calibrate: `make --version`, `which make`
- Suggested resource: `role=tool, brand=make`

**`file/dotenv`** *(transponder)*
- Detect: `.env` file present in CWD
- Scan: verify `.env` exists and is readable; report key count without exposing values
- Calibrate: same as scan (file transponders are read-only from Orbiter's perspective)
- Demonstrates gjson for structured field extraction from a flat key=value format

---

## Remote Integration Model

Remote integrations check existence and linkage — they do not sync content. Orbiter is not a CI/CD or DevOps tool. Sync status is the captain's responsibility.

- **Detect:** the remote tool is installed and/or the expected path is reachable
- **Scan:** verify the stored path or resource is still present and reachable
- **Calibrate:** if missing, attempt to establish the link (e.g. clone a repo) or surface instructions when Orbiter cannot act autonomously (e.g. a GUI sync app)

The git integration is the reference: it checks whether the expected repo exists at the stored path and clones it if not. Google Drive follows the same shape — check the tool is installed and the sync folder is at the stored path; guide the captain if not.

---

### Rust Integrations

Rust's `wasm32-unknown-unknown` target is the recommended path for new integrations. `serde_json` works without restriction. All Rust integrations use `serde` + `serde_json` for JSON and `#[no_mangle] pub extern "C"` for handler exports.

**`runtime/python`**
- Detect: `pyproject.toml`, `requirements.txt`, or `setup.py`
- Scan/Calibrate: `python3 --version`, `which python3`
- Suggested resource: `role=runtime, brand=python`
- Manager observation: report whether managed by `uv`, `pyenv`, or system

**`runtime/rust`**
- Detect: `Cargo.toml` present in CWD
- Scan/Calibrate: `rustc --version`, `cargo --version`, `which rustc`
- Suggested resource: `role=runtime, brand=rust`
- Reports active toolchain channel (stable/beta/nightly) from `rustup show active-toolchain` if rustup is present

**`manager/brew`**
- Detect: `brew` binary in PATH (no file-based detection; homebrew is system-level)
- Scan/Calibrate: `brew --version`, `which brew`; `brew info --json=v2` for prefix and install path
- Suggested resource: `role=manager, brand=brew`
- `brew info --json` output is the showcase for serde_json deserialization in Rust guests

**`manager/uv`**
- Detect: `uv.lock` or `pyproject.toml` with `[tool.uv]` section
- Scan/Calibrate: `uv --version`, `which uv`
- Suggested resource: `role=manager, brand=uv`
- Natural dependency pairing with `runtime/python`

**`manager/rustup`**
- Detect: `~/.rustup/` directory exists or `rustup` binary in PATH
- Scan/Calibrate: `rustup --version`, `rustup show` for active toolchain and installed targets
- Suggested resource: `role=manager, brand=rustup`
- Natural dependency pairing with `runtime/rust`

**`tool/docker`**
- Detect: `Dockerfile` or `docker-compose.yml` present, or Docker socket reachable
- Scan: `docker version --format json`; check daemon reachability
- Calibrate: verify daemon running; report context name
- `docker version --format json` is a rich serde_json showcase

*`remote/google-drive` and `agent/google-auth` are covered in the standalone Google spec.*

**`keychain/macos`** *(transponder)*
- Detect: platform is `darwin`
- Scan/Calibrate: verify `security` binary is present; confirm keychain is unlocked via `security show-keychain-info`
- Exports: none (read-only credential provider; values are fetched by the host on behalf of dependent resources, not emitted to shell)

**`vault/onepassword`** *(transponder)*
- Detect: `op` binary in PATH
- Scan: `op --version`; `op account list` to verify signed-in state
- Calibrate: `op signin` flow if not authenticated; uses `NeedsInput` for account/password prompts
- Manifest declares `op` in `[commands] allowed`

**`agent/ssh`** *(transponder)*
- Detect: `SSH_AUTH_SOCK` set in environment or `ssh-agent` binary in PATH
- Scan: `ssh-add -l` to list loaded keys; report count and key fingerprints
- Calibrate: start `ssh-agent` if socket missing; `ssh-add` default key if no keys loaded
- Exports: `SSH_AUTH_SOCK` (already established by `git` integration pattern)

---

### AssemblyScript Integrations

AssemblyScript compiles TypeScript-like syntax to WASM. JSON is handled via `assemblyscript-json`. Binary sizes are small (20–60 KB). This is the recommended path for integration authors who come from a JavaScript/TypeScript background.

**`manager/nvm`**
- Detect: `.nvmrc` or `.node-version` present in CWD, or `~/.nvm/` directory exists
- Scan/Calibrate: read `.nvmrc` for declared version; `node --version` for active version; compare
- Suggested resource: `role=manager, brand=nvm`
- Community showcase: demonstrates AssemblyScript guest ABI and `assemblyscript-json`

**`tool/just`**
- Detect: `Justfile` or `justfile` present in CWD
- Scan/Calibrate: `just --version`, `which just`
- Suggested resource: `role=tool, brand=just`

**`env/shell`** *(transponder)*
- Detect: always detected (every environment has shell variables)
- Scan: inspect declared env keys from `ResolvedContext`; report which are set and non-empty
- Calibrate: same as scan (env transponders are read-only)
- Exports: none directly; acts as a validator that declared env vars are present before dependent resources proceed

---

### Zig Integrations

Zig's `wasm32-freestanding` target produces minimal binaries with no hidden runtime. `std.json` handles JSON in the standard library. This is the recommended path when binary size matters or when wrapping low-level system behavior.

**`manager/asdf`**
- Detect: `.tool-versions` present in CWD or `~/.asdf/` directory exists
- Scan/Calibrate: `asdf version`, `asdf current` for active tool versions
- Suggested resource: `role=manager, brand=asdf`
- Community showcase: demonstrates Zig guest ABI and `std.json`

**`filesystem/local`**
- Detect: always detected (Zig integration overrides native `filesystem/orbiter` when present)
- Scan: `os.Stat` equivalent via `run_command stat`; report path existence, type, permissions
- Calibrate: create directory if missing; set working directory via `ORBITER_CWD` export
- Manifest declares `ORBITER_CWD` in `[shell] exports`
- Host intercepts `ORBITER_CWD` during `jump`: calls `os.Chdir` on the host process and emits `cd <path>` to shell eval output. This is the first use of the host-reserved export key pattern.

---

### C/wasi-sdk Integration

The `wasm32-unknown-unknown` target via wasi-sdk gives a C standard library and predictable memory layout. Best for wrapping existing C tooling or demonstrating the C guest path for systems programmers.

**`tool/vscode`**
- Detect: `.vscode/` directory present in CWD or `code` binary in PATH
- Scan/Calibrate: `code --version`, `which code`
- Suggested resource: `role=tool, brand=vscode`
- Community showcase: demonstrates C guest ABI — host function imports as `extern` declarations, exported handler functions via `__attribute__((visibility("default")))`, manual memory management for the 64 KB buffers

---

### GitHub Integration (Standalone Spec)

GitHub is the only integration in this catalog that serves three roles. It warrants a dedicated spec because the full OAuth flow, credential storage strategy, and downstream auth agent pattern each have design decisions that would be underspecified in a language-grouped doc.

**Roles:** `tool`, `remote`, `agent`

**As `tool`:**
- Detect: `gh` binary in PATH
- Scan/Calibrate: `gh --version`, `gh auth status`

**As `remote`:**
- Detect: `.git/config` contains a `github.com` remote
- Scan: `gh repo view --json name,url,visibility` to verify remote is reachable and authenticated
- Calibrate: verify remote push access

**As `agent`:**
- `gh auth token` provides a credential on demand for integrations that declare `agent = ["github"]` in their manifest dependencies
- Calibrate: full `gh auth login` OAuth flow when not authenticated; uses `NeedsInput` for browser-based or token-based flows
- Exports: `GH_TOKEN` (host emits to shell; declared in `[shell] exports`)
- Other integrations that need GitHub auth declare `agent = ["github"]` in `[dependencies.transponders]` — they do not manage tokens themselves

The standalone GitHub spec covers: manifest design for multi-role declarations, the full `gh auth login` calibration flow, `GH_TOKEN` export lifecycle, and the downstream auth agent consumption pattern.

---

### Google Integration (Standalone Spec)

Google's brand warrants a standalone spec for the same reason as GitHub: it spans multiple integrations with shared auth infrastructure, and the OAuth workflow has enough design surface to be underspecified in a language-grouped doc.

All Google integrations follow the `<brand>-<application>` naming convention — `google-auth`, `google-drive`, `google-calendar`, `google-cloud`. Orbiter sees each full string as the brand key. This catalog delivers the first two.

**`agent/google-auth`** *(transponder, Rust)*

Orbiter's role is auth assistance — getting the captain through the authentication workflow so their environment is ready to use, not managing the resulting secrets. Google OAuth tokens are stored by the OS or the app itself; Orbiter verifies the auth state and guides the captain through the flow if it is absent.

- Detect: Google Drive desktop app installed (`/Applications/Google Drive.app` on macOS; equivalent paths on Linux/Windows) or `gcloud` CLI present
- Scan: verify the captain is authenticated — check Drive app login state or `gcloud auth list`; report the active account if present
- Calibrate: if not authenticated, guide the captain through the Google sign-in flow via the Drive app or `gcloud auth login`; uses `NeedsInput` for account selection when multiple accounts are available
- No token storage — authentication state is owned by the Drive app or gcloud credential store
- All future `google-*` resource integrations declare `agent = ["google-auth"]` in `[dependencies.transponders]`

**`remote/google-drive`** *(Rust)*

- Detect: Google Drive desktop app installed and sync folder present at the path stored in the resource config
- Scan: stat the stored sync folder path; report present/reachable; verify `google-auth` is authenticated
- Calibrate: if sync folder is missing, report the expected path and surface instructions — Orbiter cannot re-link a GUI sync app, the captain acts; auth calibration is delegated to the `google-auth` transponder dependency
- Declares dependency on `agent/google-auth`
- Brand name and pattern document the `<brand>-<application>` convention for community contributors extending the Google suite

The standalone Google spec covers: the `<brand>-<application>` naming convention, the `google-auth` OAuth assistance workflow, Drive sync folder linkage, and the shared auth dependency pattern for future `google-*` integrations.

---

## Implementation Plans

Each spec above maps to one implementation plan:

| Plan | Spec | Notes |
|---|---|---|
| TinyGo integrations | This doc — TinyGo section | Phase 1 gates Phase 2 |
| Rust integrations | This doc — Rust section | No gates; all parallel |
| AssemblyScript integrations | This doc — AS section | |
| Zig integrations | This doc — Zig section | `ORBITER_CWD` convention is a host-side change |
| C/wasi-sdk integration | This doc — C section | |
| GitHub integration | Separate standalone spec | Multi-role, full OAuth flow |
| Google integration | Separate standalone spec | `google-auth` transponder gates `google-drive` |

The Zig plan has one cross-cutting task: implementing `ORBITER_CWD` interception in the host (`internal/integrations/wasm/host.go`) before the `filesystem/local` integration can be fully tested. This host change is small but must land before the Zig integration's calibrate handler is verified end-to-end.

---

## Guest Language Reference

| Language | Target | JSON | Binary size | Notes |
|---|---|---|---|---|
| TinyGo | `wasm-unknown` | gjson/sjson (pending verification) or extracted SDK | ~100 KB | `encoding/json` and `strings.Builder` unusable |
| Rust | `wasm32-unknown-unknown` | serde_json | 50–200 KB | Recommended for new integrations |
| AssemblyScript | WASM | assemblyscript-json | 20–60 KB | TypeScript-like syntax |
| Zig | `wasm32-freestanding` | std.json | 20–100 KB | Minimal runtime, no surprises |
| C/wasi-sdk | `wasm32-unknown-unknown` | cJSON or jsmn | 30–150 KB | Best for wrapping C libraries |
