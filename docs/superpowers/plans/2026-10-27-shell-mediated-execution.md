# Shell-Mediated Execution: Design Plan

**Status:** design. The model is uniform (sections 0 and 1). The hook and OS path declaration syntax, the lint rules and the manifest layout are planning items, not decisions (section 9).

**Goal:** Let Orbiter be the single actor that runs commands, with shell integrations working like every other integration (data in, a decision out), so that every request and every output is checked against contracts.

**Context:** `docs/security.md` (open work 1 and 2), `2026-10-26-find-implementation.md` (the FIND design this reconsiders), `2026-10-27-manifest-design.md` (the manifest refactor this depends on), `docs/integrations.md` ("Operating as a shell").

## 0. The contracts

Orbiter performs operations for integrations, by domain and brand, and runs them on a Captain's machine with trust. Three contracts make that possible. They belong together and this plan treats them as one family:

1. **The manifest.** The contract for an integration's functions and metadata, including its runtime claims. Runtime claims are also security contracts: Orbiter enforces them, and the Captain reads them to decide on trust.
2. **The op protocol.** PATH, FIND and the other hooks are the contract Orbiter uses to run operations for integrations and for the flows it builds. They live in the op section of the protocol.
3. **The wasm contract.** The agreed interface between Orbiter and an integration for communication.

Orbiter, integrations, planets and the rest are all held to internal contracts like these to perform management duties. They are documented in `docs/contracts.md`.

## 1. Decisions

Made by the Captain on 2026-10-27:

1. **Orbiter runs every command.** Orbiter already proxies every integration and owns stdout and stdin. No integration has direct shell access: a guest never executes anything itself. Because Orbiter runs everything, it validates every request and output against the contracts.
2. **Every integration is a function.** Orbiter hands the integration data, the integration decides and returns output, and Orbiter acts on that output. There are no exceptions; shell integrations change to fit the model.
3. **A guest may request anything its manifest declares.** Orbiter enforces the manifest at runtime. The Captain is responsible for reading the manifest and deciding to trust it; transparency is what makes that possible.
4. **What Orbiter knows.** Orbiter knows the OS for certain only because of Go. It knows the shell only through the shell integration, as a byproduct of setting the shell at the vessel layer.
5. **OS-agnostic paths.** Orbiter's internal filesystem parser holds both path types (Windows and POSIX) and uses the native one through Go's detection. An integration that does not support the current OS still takes part, because its manifest holds file paths for both.
6. **A shell integration is a functional service, hooks, and a manifest** that packages them. Hooks are plain text files passed by reference in the manifest, by file name. A hook inside the WASM binary is expressly forbidden.
7. **Shell init discovers through the integration** and then installs the hooks (section 3).
8. **Hooks live in the shell.** A copy can be kept anywhere as a backup and a byte-check for changes. Orbiter handles hooks, with respect to their integrations, for its whole lifetime. A failing or missing hook, a missing integration package, and shell drift are all linked to the shell integration they came from.
9. **Hook validation.** Orbiter validates a hook with linting and other rules that roughly judge its safety. **A flagged hook is shown to the Captain.** Orbiter cannot have a policy good enough to refuse a hook without the Captain's input.
10. **Transparency for hooks.** `vessel inspect` shows the exact hook and its hash, and a changed hook is not installed until the Captain confirms it with the difference shown.
11. **FIND is a shell hook concern**, whether it is a binary, a file, a directory or a variable. Orbiter uses the hook contract for these operations.
12. **PATH is treated like a shell hook.** Each shell finds PATH differently, through its PATH contract. The content is OS-specific, so Orbiter parses it.
13. **The manifest syntax is redesigned as a whole**, not patched section by section (`2026-10-27-manifest-design.md`).
14. **FIND is the fallback that avoids PATH.** A bare command (`git status`, `nvm use node@version`) assumes its binary is on PATH. A FIND can ask for an integration's own binary, or a file or directory tied to it, or be a raw request. It is a hook contract used by integrations and by Orbiter, and the answer goes back to whoever asked: an integration gets it if an integration asked, Orbiter gets it if Orbiter asked. Integrations own the health and lifecycle of their binary; Orbiter does not use it for its own purposes.
15. **Orbiter checks and holds the binary location.** When it initializes an integration, Orbiter does a PATH check and a FIND to get the actual binary location, and validates whether the PATH location is the same as the FIND location. The difference is decided at init, and Orbiter holds one config, not both results. With it, Orbiter can optionally substitute the binary name with the full path when it runs a command. Handling of a differing location already exists (`internal/commands/uplink_find.go`).
16. **Integrations chain through Orbiter.** `nvm` is managed by `node`: the node integration gives Orbiter node commands, or its manifest carries the binary name and version, and the nvm integration outputs commands from what Orbiter scraped from the node integration. Orbiter runs the command requests with all of this context and returns the result to the appropriate integration for further action. The data chain and walk already exist and are the foundation of Orbiter (`internal/starchart/crawl.go`).

## 2. What happens today

Facts from the code, because the model below differs from them.

| Piece | Where it runs | Gated by Orbiter? |
|---|---|---|
| Shell integration **guest** (WASM): `detect`, `scan` | wazero sandbox | Yes. Its only command is a health check (`bash --version`) through `run_command` and the gate |
| `FIND_BINARY` / `FIND_FILE` / `FIND_DIR` / `FIND_ENV` | **Shell hook script**, in the Captain's interactive shell | No. Orbiter emits `FIND_X <txn> ...`; the hook searches (a `PATH` walk, `find`, `printenv`) and pipes JSON to `orbiter relay` |
| `DIR`, `SET`, `UNSET`, `SESSION` directives | Shell hook script | Partly. Orbiter filters `SET` keys against `[shell] exports`, escapes newlines, and the hook quotes values. Nothing else is checked |
| Prompt hook (`orbiter hook` at every prompt) | Shell hook script | No |
| The hook file | `hook.<shell>` in the integration folder, named by `[shell] exports`; copied by `vessel init`, read by `shell init` | No path check on the name |
| Shell discovery | `internal/commands/shell.go`: registered integrations detect, the result is registered and attached to the vessel | Yes (the gate) |

Two earlier decisions this design touches (`2026-10-26-find-implementation.md`):
- **D1**: the search method belongs to the shell integration; Orbiter specifies only the reply.
- **Global constraint**: FIND functions are called only through directives, never through `run_command`.

The hook is native code with the Captain's full privileges, so it cannot be sandboxed; it can only be shrunk, reviewed and made transparent.

## 3. Shell init: discovery and install

Shell init runs a handshake between the shell integration and Orbiter. Orbiter sits in the middle of every step:

1. The shell integration passes CLI commands that check for the shell.
2. Orbiter checks the commands against the manifest.
3. Orbiter runs them if they are deemed safe.
4. Orbiter does a quick security check on the response.
5. If the response is clean, Orbiter returns it to the integration.
6. The integration validates the response with a yes or no.
7. Orbiter adds the hooks to the shell, using the file names in the integration's manifest, from within the integration folder.

This is the existing discovery pattern (registered integrations detect, the result is registered and attached to the vessel) with Orbiter mediating the commands the way the model requires. Setting the shell in the vessel layer is how Orbiter comes to know the shell (decision 4).

After install:

- **The hook lives in the shell.** A backup copy kept anywhere, with its hash, lets Orbiter byte-check for a changed hook.
- **Lifetime.** A missing or failing hook, a missing integration package and shell drift are each recorded against the originating shell integration and reconciled through it.
- **Changed hook.** A hook that no longer matches its recorded hash is not installed again until the Captain confirms it with the difference shown. Swapping the shell integration runs the same flow with the new integration's hook.
- **Transparent.** `vessel inspect <shell>` prints the exact hook and its hash.

### What "validates a hook" can honestly mean

Orbiter cannot prove arbitrary shell code safe, and the plan does not pretend to. The controls are layered and each is checkable:

- **Small:** a size cap declared in the manifest; a hook over it is flagged.
- **Lint:** the hook may only do what the directive interpreter does. Orbiter checks that the text contains the interpreter's required parts and finds constructs outside a conservative set (command substitution other than the call back into `orbiter`, pipes to anything but `orbiter`, network or file commands). The lint can miss things in a language as loose as shell; it is a tripwire, not a proof.
- **Shown, never silently refused or served:** anything flagged, and any change, goes to the Captain with the reason (decision 9).
- **Readable:** the Captain can read thirty lines. Smallness is what makes review possible, so the cap matters more than the lint.
- **Signed later:** when integrations are signed, the hooks are covered with the rest of the integration folder.

## 4. FIND and PATH under the hook contract

FIND (binary, file, directory, variable) and PATH are shell hook concerns, and Orbiter uses the hook contract to run them (decisions 11 and 12). They are ops in the op protocol, held to the same contract family as everything else (section 0).

- **PATH.** Each shell finds PATH its own way, through its PATH contract. Orbiter takes the result and parses the content, which is OS-specific, with its internal filesystem parser (decision 5).
- **FIND.** The shell knows how a name resolves for the Captain (PATH, shims, functions, version managers), which is why FIND is a hook concern. The result goes back to the requester, whether that is an integration or Orbiter.
- **At integration init.** Orbiter runs a PATH check and a FIND for the integration's binary, compares the two locations, decides at init, and holds one config (decision 15).
- **At command time.** Orbiter runs the commands integrations request (decision 1). A bare command assumes the binary is on PATH; with the held location, Orbiter can optionally substitute the full path for the name.
- **Chains.** An integration managed by another (`nvm` by `node`) gets its context through Orbiter: Orbiter reads what the managing integration provides (commands, or the binary name and version in its manifest), hands it to the dependent integration, runs the commands that integration outputs, and returns the result to it (decision 16).

## 5. The model as a chain

```text
requester (git)          Orbiter                              shell integration (guest)
declares what it    -->  plans the op, validates it against
needs in its manifest    the requester's manifest

  FIND, PATH (hook contract):
                            --> op + args + platform  -->     the shell answers (a location)
                             validates the answer's shape, normalizes paths
                             returns it to the requester (integration or Orbiter)
                             at init, holds the decided binary config (decision 15)

  commands the integration requests:
                             checks against the integration's manifest (commands, environment)
                             optionally substitutes the held full path for the binary name
                             runs it, collects output (capped, timed, audited)
                             returns the result to the requesting integration
```

Properties that make this safer than today:

- No shell code runs anything on behalf of a guest. The shell guest returns data, and Orbiter's existing gate (allowlist, environment claim, timeout, output cap, audit log) applies to what it returns.
- A name being searched for is passed as an argument, never interpolated into a script, so a hostile name cannot become a command.
- Every step has a contract: the requester's manifest (what may be asked), the shell's manifest (what may be run), and the op's output schema (what a reply may contain).

## 6. Work that is needed whatever the final shape

1. **Validate directives Orbiter emits.** `SET` keys must be listed in the manifest's write claim; `DIR` values must be normalized paths; ops outside the fixed vocabulary are dropped. Add tests for each.
2. **The `sensitive` flag.** Declared in the manifest, read by nothing. Make it mean secret: scrub those values from command output and every guest-controlled string before it is shown, stored or sent to a client (roadmap Phase 3).
3. **Hooks:** the handshake in section 3, the backup copy and byte-check, the change confirmation, `vessel inspect` showing the hook, and a test that a guest carrying a hook is refused. Until the hook declaration syntax exists, add a path check to the file `[shell] exports` names today (inside the integration folder, no traversal).
4. **Environment claims** move with the manifest redesign, so they are not changed again here.

## 7. Phases

1. **Common work** (section 6), once the manifest redesign settles how hooks and claims are declared.
2. **The discovery handshake.** Rewrite shell init to the seven steps, with the yes or no from the integration and the hook install from the manifest's file names.
3. **The hook contract for FIND and PATH.** Design the op shapes, build `PATH` parsing on the filesystem parser, move `FIND_BINARY` first behind a flag, rewrite flow 01's FIND step to run both ways, and compare on macOS and Windows (Git Bash and PowerShell).
4. **Move the rest and retire the old path.** `find_file`, `find_dir`, `find_env`; shrink the four hooks; update the FIND specs and the roadmap.

Windows risk: Git Bash path forms (`/c/...`), `PATHEXT` and PowerShell lookups (`Get-Command`) all pass through the PATH contract. The `Path` type already normalizes, but this needs real Windows verification (roadmap Phase 7).

## 8. Tests

- A hostile shell guest, for each of: a command outside the shell's declaration, an injection attempt through the name, output outside the schema, output over the cap, a slow command.
- An unclaimed environment variable never reaches a command run for a shell.
- A guest module carrying a hook is refused at load.
- A hook over the size cap, a hook the lint flags (shown to the Captain), and a hook with a changed hash each follow their path in shell init.
- The shell-init handshake: a command outside the manifest is not run, an unclean response is not returned, a "no" from the integration installs nothing.
- The directive validation tests from section 6.
- Existing flows rewritten to drive the new path.

## 9. Open planning items

Decided: sections 0 and 1. Still open:

1. **The hook declaration syntax.** How a manifest names its hooks and gives the size cap, purpose and export. This is part of the manifest redesign.
2. **The OS path declaration syntax.** How a manifest holds file paths for both platforms. This is part of the manifest redesign.
3. **The lint rules.** Which constructs it flags. (Decided: flagged hooks are shown, not refused.)
4. **Tools defined in rc files** (`nvm` and similar). The command Orbiter runs has to load the Captain's rc files, which runs arbitrary rc code and is slow. Acceptable for the uniform model, or does a lighter way exist?
