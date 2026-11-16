# Roadmap and Checklist

What is done, what is left, and in what order. Update this file in the same change that
finishes or adds an item. The desktop GUI is last.

Last updated: 2026-10-26.

## How the order was chosen

1. **Prove the core first.** Several features were built and tested in isolation (see the
   2026-10-26 wiring audit). A test that runs a real integration through the whole
   lifecycle comes first, so every later item can be shown to work.
2. **Then what users depend on:** credentials reach the shell, then binary trust.
3. **Then the integration contract.** The StateReport migration waits on the binary-path
   design, because it decides what goes in `Config`.
4. **Then polish and verification in other environments.**
5. **GUI, then TUI.** Both need a contract that is complete and trustworthy.

Items inside a phase are in dependency order; "needs:" names a prerequisite.

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
- [x] JSON `scan` and `calibrate` results leaked export values (tokens); they now name exports and redact the values.
- [x] The hard-coded banned list is gone. The policy is transparency: everything is banned except the commands an integration's manifest declares, the Captain approves each run, and straying quarantines the integration. The allowlist is now enforced for every integration (one that declares nothing may run nothing; before, it was only enforced when something was declared). The four shell manifests declare their health command (`bash --version` and the like) and shells scan healthy; git's declared `chmod` is no longer blocked by a list that named it. Flow 2 runs to the end.
- [x] Shell drift: a scan flags the vessel's shell when its integration folder is gone or the shell is not where it was recorded; `calibrate` finds shells through the integrations (discovery asks every integration, and the shell suggestions are used), shows what was found and what is missing, and the Captain chooses, sync or async (`needs_decision`, answered with `relay` then `uplink`). The chosen shell is registered and the drifted one retired. FIND looks for a recorded shell first. Flow 2 covers it up to the shell's health check (see Phase 3).
- [x] `retro` retires the thing it was asked to retire. On a resource attached to its parent (a planet, the vessel) it printed "retired" but left the resource in the tree; the target is now always retired. It only touches the star chart, never an integration's folder.
- [x] A warning is a `warning` event on stdout in JSON mode; `uplink`'s error lines no longer go to stderr.
- [x] Flow harness and flow 1 (register a project and calibrate git, nine steps, real integration, real shell): `docs/testing.md`.
- [x] "Always allow this exact command?" goes through the transaction flow (sync with `-i`, async with `relay`), sharing one session with the approval prompt.
- [x] The shell follows the same pattern as every other resource. With no shell on the vessel, `shell init` runs discovery for the vessel (the code the planet uses, `DiscoverVessel`), registers and attaches what the integrations suggest (`registerSuggestions`, shared with `planet init --discover`), and delegates to the registered integration; afterwards it uses the vessel's record. Swapping a shell needs no new command. The four shell manifests dropped detection rules that could never match. Earlier in the same work: Shell selection is out, in, out, with no shell named in Orbiter. With no record of a shell, Orbiter asks each registered shell-role integration to `detect` (it was a constant "yes" in all four, and detection by the manifests' `BASH_VERSION`/`ZSH_VERSION` rules could never match because shells do not export them), registers the one that answers as a shell resource on the vessel, and delegates to its hook. After that Orbiter uses its own record. Installing an integration now also installs the files its manifest declares (the hook), and `LoadInstalled` can load just the roles a command needs.

## Phase 1: Prove the core works end to end

Flows live in `cmd/orbiter/testdata/flows/` and are not mocked; unit tests are. See `docs/testing.md`.

- [x] **Flow harness.** Isolated per flow (own `HOME`, own `PATH` holding exactly the tools it provides), numbered steps, findings recorded without failing, a client that answers prompts like a GUI would, and an end-of-run report.
- [x] **Flow 1: register a project and calibrate git.** Nine steps with the real git integration: `vessel init`, register, survey, calibrate (the client answers the approval and always-allow prompts), the user's own shell answers `FIND_BINARY` through the hook Orbiter auto-detects, `uplink` records the path and calibrates, the path is in the star chart, a later scan is healthy. It found two bugs, both fixed (the always-allow prompt read raw stdin; shell detection never worked in a real shell) and one open finding:
  - [ ] the first approval names the bare command (`git --version`) instead of a resolved binary path, because the first scan runs before FIND (Phase 3)
- [x] **More flows, each an ordered real use case** (flows 02 to 06): the shell drifts and the Captain reconciles it; several matches (SELECT, answered with `-select`); the approval denied; retro (the resource leaves the tree, the integration folder stays); an unattached client answering with `relay`. The transponder-credential and manager-governed flows are written with the features they exercise (Phase 2 and Phase 3).
- [x] **The older CLI scripts are flows now** (07 to 11), with real registered resources. `vessel init` runs first, prompts go through the client, and the `BASH_VERSION` hack is gone. The script directory and its test are removed.
- [x] **WASM approval path with a real integration** is exercised by flows 01, 06 and 08 (approve and always-allow, answered by the client, attached and by relay).

Phase 1 is complete. The findings its flows produced are in the phases they belong to.

## Phase 2: Credentials work

- [ ] **Transponder exports reach the shell.** `Hook` and `Jump` read only `Resources`; the exports a transponder's report carries (the git auth roles: file, env, agent, keychain, vault) are dropped, so the credentials those integrations resolve are never set. `Chart` and the jump plan ignore transponders too. *Highest-value gap: the auth integrations do nothing for the user until this is wired.*
- [ ] **Transponder-only integrations get their binaries.** keychain (macos), vault (1password) and agent (ssh-agent) read `ctx.binaries`, but `NeededFinds` only looks at resources. Needs a short design: where a transponder records its binary path.
- [ ] **Remaining prompts go through the transaction flow.** These read `os.Stdin` directly, so `-i` and `relay` do not cover them:
  - [ ] `jump` confirmation (`executor.go`, `Jump`)
  - [ ] `retro` confirmation (`executor.go`, `Retro`); JSON clients pass `-y` meanwhile
  - [x] "Always allow this exact command?" after an approval (done; flow 1 stalled on it before the fix)
- [ ] **A flow for it:** a transponder credential reaching the shell (the flow lands with the feature).
- [ ] **Credential prompting.** `StateReport.NeedsInput` and `ResolvedContext.Responses` exist, and the comment says the host collects answers and calls the integration again, but no production code reads either. This is what a `TEXT` (obscured) prompt through the transaction flow is for. *needs: the prompts item above.*

## Phase 3: Binary trust

- [ ] **Design the expected path.** The manifest declares only binary names today. Decide how a manifest states the expected location for a local install and how a manager supplies its override (see Design notes).
- [ ] **FIND before the first scan.** A resource with no recorded binary is scanned first, so the integration asks the Captain to approve running a bare command name (`git --version`), which executes whatever is first on `PATH`. Decide the binary before anything runs, so approvals name the resolved path. Found by flow 1.
- [ ] **A flow for it:** a manager-governed install, where the manager supplies the install location and Orbiter reconciles it with what FIND found (the flow lands with the feature).
- [ ] **Expected vs actual.** Compare the expected location (local install) or the manager's override against the path FIND found, and ask the Captain to rectify a mismatch. *needs: the design item.*
- [ ] **Honor `sensitive`.** `shell.exports[].sensitive` (the github manifest sets it) is read by nothing; redact those values wherever exports are logged or shown.

## Phase 4: The integration contract

- [ ] **Manifest design review.** The manifest syntax redesigned as a whole: metadata, runtime, external-resource and health claims, one place for environment read and write claims, artifact declarations, a manifest version. Plan: `2026-10-27-manifest-design.md` (inventory done; design is the next step). Shell-mediated execution and the hook artifact depend on it.
- [ ] **Shell-mediated execution.** Orbiter runs every command and every integration is a function; hooks become artifacts Orbiter holds, never inside the WASM. Shell init discovers and migrates them. Plan: `2026-10-27-shell-mediated-execution.md`; Option B is chosen; hook storage, the artifact syntax, the lint behavior and the file and directory searches are open.
- [ ] **Signing.** TODO: off during development only; required once integrations are stable (`docs/security.md`, open work 3).
- [ ] **Pre-translated modules for the compiler engine.** The interpreter is the default and starts at once; an integration that opts into `compiler` still pays about 0.4 s per process. A compilation cache keyed by the wasm hash would remove it; only worth building when a bundled integration needs the compiler.
- [ ] **Remove the remaining stderr writes.** Nothing should be written to stderr. These predate this work and still are: the `jump` plan and its "Execute maneuver?" prompt and the `retro` "Execute retro?" prompt (`executor.go`); the terminal fallbacks for the approval and always-allow questions (`internal/wasm/loader.go`); the quarantine notice (`internal/integrations/registry.go`) and the settings parse warning (`settings.go`); `orbiter: load integrations` (`root.go`); and the plain-text error `Execute` prints in styled mode. The hard part: the shell hook captures stdout to read directives, so a prompt on stdout stays invisible until the command exits. The shell hooks should stream events and answer prompts on stdin (the hook acting as the client `-i` describes) instead of capturing the output.
- [ ] **StateReport config contract.** Plan: `2026-06-18-statereport-config-contract.md`. Drops the named fields (`BinaryPath`, `InstallDir`, `InPath`, `Manager`, `Exports`) from `StateReport` and moves them into `Config`. The plan predates FIND and the design notes below: it lists 20 integrations plus the deleted `local` and misses 8 added since (bash, zsh, fish, powershell, google-auth, google-drive, tmux, json), so 28 integrations exist today. The host acts only on `exports` (`git` and `github` emit it) and `install_dir`; `binary_path`, `in_path` and `manager` are informational. Refresh it before executing; in `Config`, `binary_path` is the actual path an integration reports using, separate from the manifest's expected path. *needs: Phase 3 design.*
- [ ] **Validate config against the manifest's `[config]` schema.** `ManifestConfigField.Required` and `Fields` are read by nothing, so `resource add --config` is not validated.
- [ ] **Runtime buffer hints.** `runtime.input_buffer_kb` and `runtime.output_buffer_kb` are ignored; honor them or remove them.
- [ ] **`defaults` and `navigation_history`.** The constitution lists both and the CRUD spec defines `vessel defaults add`; Go models and ID codes exist, but the schema has no tables and nothing writes them. Build them or drop them from the specs. Decide first.
- [ ] **Code reachable only from tests.** Wire it, delete it, or keep it as a test helper.
  - `PlanFinds` and `ApplyFindResults` (`NeededFinds` took over; the unsafe-name validation lives in both)
  - `InstallSelected`, `RenderCatalogChecklist`, `WriteInspectReport`, `RoleType`, `IsID`
  - `Path.IntoUnix`, `Path.IntoWindows`, `Path.IntoShell` (specified; only `IntoNative` is used)
  - Test helpers, fine as they are: `Executor.SC`, `NewRootCommand`, `NewStyledRendererWith`
- [ ] **Present/Reachable spec.** The FIND spec references a "Present/Reachable refactor" that was never written and no longer applies: integrations self-report both in `StateReport` and Orbiter stores them. Reduce the references to that one sentence.

## Phase 5: Contract polish and other environments

- [ ] Contract coverage test: run every non-exempt command with `--output json` and assert the terminal event, so a new command cannot ship without one. *(Proposed.)*
- [ ] Schema version and compatibility policy for the contract, since third parties will build on it.
- [ ] `hook` and `jump` have no event form (they emit the shell line protocol). Decide whether a client that wants shell state needs one.
- [ ] fish and PowerShell hooks: never run (neither shell is installed on the dev machine).

## Phase 6: Deferred FIND review minors

- [ ] zsh `FIND_DIR` uses a local named `path`, which overwrites `PATH` inside the function
- [ ] JSON escaping in the hooks covers only `\` and `"`, not tab or newline
- [ ] A SELECT answer is not checked against the matches that were offered
- [ ] Sessions opened for discovery stay active if no hook ever runs `uplink` (no expiry)
- [ ] `calibrate` scans the branch twice (once to decide on FIND, once to calibrate)
- [ ] bash exports the `FIND_*` functions but not the helpers they call
- [ ] PowerShell `FIND_FILE` throws when `ORBITER_CWD` is unset

## Phase 7: Desktop GUI (last of the features)

Direction, not yet specified: Wails shell, Vue/Quasar UI, Pixi.js for the 2D orbit scene (space theme, warp transitions between galaxies, semantic zoom). It drives the CLI through the JSON contract, the same way any third-party client would.

- [ ] Design pass (open questions: how the scene maps to the entity hierarchy, how the GUI learns about changes it did not cause, whether anything reads the StarChart other than through the CLI).
- [ ] Spec.
- [ ] Build.
- [ ] Windows and Git Bash verification pass (the paths are tested with simulated hosts only).

## Phase 8: After the GUI

- [ ] TUI. Deprioritized in favor of the GUI as the first UI; the stubs (`tui.Run`, the models, the no-op `orbiter starchart`) stay until then.

## Design notes

- **Filesystem is Orbiter's.** It is native Go on every OS, not an integration, and cannot be replaced.
- **Local is the default.** A resource that is not a child of a manager is local (a standalone install, such as Node from the website). Its integration's manifest holds the information for that case; managers (nvm, brew) hold references for install locations and supply contextual overrides. There is no `native` manager integration to build.
- **Two kinds of binary path.** In a manifest, `binary_path` is the expected (theoretical) location for a local install; only a manager can supersede it. The path FIND returns is the actual one. Orbiter reconciles the two for security and pushes the Captain toward rectifying a mismatch. This reconciliation is not built yet (Phase 3).
- **`in_path` is informational.** It tells a user whether the command can be run by name instead of by passing the binary path each time. It does not decide Reachable.
- **Present and Reachable are the integration's report.** Orbiter stores them and does not stat binaries.
- **Integrations are folders; discovery is the filesystem.** There is no install or uninstall mechanism, so a folder can be carried between machines. Orbiter uses an integration if its folder is in `~/.orbiter/integrations`; `vessel init` copies the bundled folders there. `retro` removes a resource from the tree and leaves the folder alone.
- **A shell is a resource on the vessel**, discovered and registered like any other (see Done). A shell's health check commands belong in its manifest, like any integration's.
- **No stderr.** Everything goes out on stdout and comes in on stdin. Orbiter holds to this in what it was built to do this session; the places that still write to stderr are listed in Phase 4.

## Where shell and executable names still appear in Orbiter (not changed; decide)

Orbiter should name no integration or shell. These remain and are not detection logic:
- `internal/commands/completions.go`: `orbiter completions bash|zsh|fish|powershell` is Cobra's built-in completion generators, not an integration.
- Help text and comments that show profile lines or mention Git Bash.

## How the 2026-10-26 audit was done

`deadcode` (functions unreachable from `main`), a read of the manifest and schema against production code, and a read of the specs against the registered commands. It is not exhaustive: it does not review tests one by one or look inside the WASM guests.

## Conventions to keep

- New flags and changes to a flag's meaning go in `docs/cli-conventions.md` in the same change.
- Every non-exempt command emits a terminal event in JSON mode.
- Plans are edited only for what was asked.
