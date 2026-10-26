# Roadmap and Checklist

What is done, what is left, and in what order. Update this file in the same change that
finishes or adds an item. Order of work: harden, then the JSON contract, then the desktop GUI last.

Last updated: 2026-10-26.

## Done

- [x] Module foundation, StarChart, CRUD, lifecycle commands, integration hardening and lifecycle, callsign lifecycle, the integration catalog (see the plans in `docs/superpowers/plans/`).
- [x] FIND: shell integrations ship `FIND_BINARY/FILE/DIR/ENV`; Orbiter records the resolved path; the Captain confirms changes. Plan: `2026-10-26-find-implementation.md`.
- [x] Sessions and transactions as call-and-response breakpoints; `uplink` continues the interrupted operation.
- [x] I/O contract (relay, uplink, `Renderer.Prompt`).
- [x] Git Bash and Cygwin paths on Windows hosts (`ParsePath`).
- [x] JSON contract: every command ends in one `result`, `plan` or `error` event (`docs/cli-conventions.md`).
- [x] `survey` is the enumeration command (`-a` all descendants, `-v` detail).
- [x] Flag conventions: `-v` volume, `-t/--trace` process transparency, `-i/--interactive` attached client, `-a` scope.
- [x] WASM approval flow works in JSON mode, sync (`-i`, answered on stdin) or async (`orbiter relay`).
- [x] Cleanup: removed the stale `build-integration-local` target, the unused `which` allowlist entries, and `stubs.go`; marked the I/O contract spec implemented.

## 1. Harden (before the GUI)

- [ ] **StateReport config contract.** Plan: `2026-06-18-statereport-config-contract.md`. Drops the named fields (`BinaryPath`, `InstallDir`, `InPath`, `Manager`, `Exports`) from `StateReport` and moves them into `Config`. The plan predates FIND: it lists 20 integrations plus the deleted `local`, and misses 8 added since (bash, zsh, fish, powershell, google-auth, google-drive, tmux, json), so 28 integrations exist today. Refresh before executing.
- [ ] **Transponder-only integrations never get binaries.** keychain (macos), vault (1password) and agent (ssh-agent) read `ctx.binaries`, but `NeededFinds` only looks at resources. Needs a short design: where a transponder records its binary path.
- [ ] **Prompts that bypass the transaction flow.** These read `os.Stdin` directly, so `-i` and `relay` do not cover them:
  - [ ] `jump` confirmation (`executor.go`, `Jump`)
  - [ ] `retro` confirmation (`executor.go`, `Retro`); JSON clients pass `-y` meanwhile
  - [ ] "Always allow this exact command?" after an approval (`StdinAlwaysAllowFunc`, `internal/wasm/loader.go`)
- [ ] **Present/Reachable spec.** The FIND spec references a "Present/Reachable refactor" that was never written and no longer applies: integrations self-report both in `StateReport` and Orbiter stores them. Reduce the references to that one sentence.
- [ ] **Deferred review minors (FIND).**
  - [ ] zsh `FIND_DIR` uses a local named `path`, which overwrites `PATH` inside the function
  - [ ] JSON escaping in the hooks covers only `\` and `"`, not tab or newline
  - [ ] A SELECT answer is not checked against the matches that were offered
  - [ ] Sessions opened for discovery stay active if no hook ever runs `uplink` (no expiry)
  - [ ] `calibrate` scans the branch twice (once to decide on FIND, once to calibrate)
  - [ ] bash exports the `FIND_*` functions but not the helpers they call
  - [ ] PowerShell `FIND_FILE` throws when `ORBITER_CWD` is unset
- [ ] **Verification that needs another environment.**
  - [ ] fish and PowerShell hooks (never run: neither shell is installed on the dev machine)
  - [ ] Windows and Git Bash (planned once the GUI exists)
  - [ ] WASM approval end to end, with a real integration calling `run_command` (the approval function is tested directly)

## 2. JSON contract follow-ups

- [ ] `hook` and `jump` have no event form (they emit the shell line protocol). Decide whether a client that wants shell state needs one.
- [ ] Schema version and compatibility policy for the contract, since third parties will build on it.
- [ ] Contract coverage test: run every non-exempt command with `--output json` and assert the terminal event, so a new command cannot ship without one.

## 3. Desktop GUI (last)

Direction, not yet specified: Wails shell, Vue/Quasar UI, Pixi.js for the 2D orbit scene (space theme, warp transitions between galaxies, semantic zoom). It drives the CLI through the JSON contract above, the same way any third-party client would.

- [ ] Design pass (open questions: how the scene maps to the entity hierarchy, how the GUI learns about changes it did not cause, whether anything reads the StarChart other than through the CLI).
- [ ] Spec.
- [ ] Build.
- [ ] Windows verification pass (see section 1).

## 4. After the GUI

- [ ] TUI. Deprioritized in favor of the GUI as the first UI; the stubs (`tui.Run`, the models, the no-op `orbiter starchart`) stay until then.

## Design notes

- **Filesystem is Orbiter's.** It is native Go on every OS, not an integration, and cannot be replaced.
- **Native installs are standalone.** A tool installed without a manager (Node from the website) is its own block, not a child of a manager. The tool's integration holds the native information; managers (nvm, brew) supply contextual overrides to install locations. There is no `native` manager integration to build.

## Conventions to keep

- New flags and changes to a flag's meaning go in `docs/cli-conventions.md` in the same change.
- Every non-exempt command emits a terminal event in JSON mode.
- Plans are edited only for what was asked.
