# Security

How an integration is contained, what the tests try, and what is still open. The policy is
transparency: an integration declares in its manifest what it does, everything else is banned, and
straying quarantines it. This page is the detail behind the summary in `docs/integrations.md`.

## Trust model

A manifest is the integration's whole claim: its commands, the environment they see, the variables
it may export, its memory and engine. Orbiter enforces the claim; reading it is the Captain's job,
as it is with any package cloned from a repository. Nothing prompts the Captain to approve a
manifest, so the manifest must be readable (claims are verbose and explicit) and `vessel inspect`
prints them.

## The sandbox

An integration is a WASM module (the guest) that Orbiter (the host) runs in wazero. The guest has
its own linear memory and no way out except the host functions in the `orbiter` module:
`read_input`, `write_output` and `run_command`. There is no WASI, so no files, console, clock or
network; a module that imports anything else does not load. `run_command` goes through the gate:
quarantine, then the manifest allowlist (names only, never paths), then trust, then the Captain's
approval, then execution.

### Engines

A manifest picks the engine in `[runtime]`:

```toml
[runtime]
engine = "interpreter"   # the default; or "compiler"
memory_limit = 67108864  # bytes; the default (64 MiB); at most 1073741824 (1 GiB)
```

| Engine | Cost | Use |
|---|---|---|
| `interpreter` | Nothing at start-up; slower per instruction | Default. Integrations do little work, so it is also faster overall. |
| `compiler` | Translates the module to native code on every process start (about 0.4 s) | Only a guest that does heavy computation. |

Both engines enforce the same sandbox and every limit below; the security tests run against both.
`orbiter vessel inspect <brand>` shows the engine an integration declares. Orbiter creates a
runtime only for an engine some loaded integration asks for.

### Limits

| Limit | Value | Where |
|---|---|---|
| Guest compute per call | 30 s (`GuestTimeout`); time spent waiting for the Captain or a command is not counted | `internal/wasm/limits.go` |
| Guest memory | the manifest's `[runtime] memory_limit` in bytes, else 67108864 (64 MiB); at most 1073741824 (1 GiB), at least one 65536-byte page | `internal/integrations/manifest.go` |
| Command run time | the manifest's `timeout_seconds`, else 5 min (`DefaultCommandTimeout`) | `internal/wasm/limits.go` |
| Command output | 4 MiB, then the command is killed (`MaxCommandOutput`) | `internal/wasm/limits.go` |

### Stateless, serverless-style

An integration keeps nothing. Every call instantiates a fresh module, runs one export and closes the
instance, so nothing a call leaves in memory reaches the next. Orbiter is the state manager: the
star chart holds the state, and an integration is handed what it needs for each call. `pool_size`
is how many instances may run at once. `Load` compiles the module once and rejects a guest that
imports anything the host does not offer; no instance exists between calls.

### Environment

A command an integration runs receives exactly the variables its manifest claims, and nothing is
implicit: `PATH` is claimed like any other variable. Manifests are meant to be verbose; claim
everything the commands need and no more.

```toml
[commands]
env = ["PATH", "HOME", "GIT_*", "SSH_AUTH_SOCK"]   # a trailing * claims every variable with that prefix
```

`vessel inspect` prints the claim. Everything else in Orbiter's environment is withheld. Orbiter
still finds the command itself, so a manifest that does not claim `PATH` can run its commands; the
commands just do not see a `PATH`.

Secrets are the next step: the `sensitive` flag on `[shell] exports` is declared in the manifest and
read by nothing yet. It is meant to mark secrets, so that their values are scrubbed from command
output and from every guest-controlled string before it is shown, stored or sent to a client
(roadmap, Phase 3). Until then a claimed secret can be echoed into a report.

## What the tests try

`internal/wasm/security_test.go` runs hostile guests (`internal/wasm/testdata/guests/*.wat`,
assembled by `just build-test-guests`) under both engines, and `integrations/security_test.go` and
`internal/wasm/loader_test.go` cover the loader and the prompt.

| Attack | Result |
|---|---|
| Out-of-bounds read, `unreachable`, endless recursion | Call fails; Orbiter and the integration survive |
| Garbage pointers and lengths passed to every host function | Refused; no crash |
| Import WASI or a host function that does not exist | Does not load |
| Spin forever | Stopped at `GuestTimeout` |
| Grow memory without end | Capped |
| Run an undeclared command | Refused and quarantined; stays blocked afterward |
| Run `/tmp/x/git` when `git` is declared | Refused and quarantined (names, not paths) |
| Malformed, empty, huge or NUL-containing command specs | Refused; no crash |
| A declared command that never ends or never stops talking | Killed |
| A folder whose manifest claims another integration's brand | Does not load (it would inherit that brand's trust) |
| Control characters in a command shown to the Captain | Written out as `\x1b`, `\r`, `\n`; never sent to the terminal |

## Open work

Direction is set for each of these; none is finished.

1. **Argument-level validation.** The gate checks the command, not its arguments, so a declared
   interpreter (`sh`, `python`) runs whatever it is told and `git -c` can set arbitrary config. The
   manifest claim, with Orbiter as the mediator between integrations, is to be the basis for
   Orbiter validating a request before it is handed on. How much a manifest can say about arguments
   is undecided. Until then the Captain sees the full command line before approving, and "always
   allow" trusts that exact line (arguments joined by spaces, so `["a b"]` and `["a","b"]` share
   one entry). Pinned by `TestKnownHole_ADeclaredInterpreterRunsWhatItIsTold`, which fails when this
   changes.
2. **Shell hooks need a sandbox.** A hook a manifest declares under `[shell]` is shell code the
   Captain's shell runs through `eval "$(orbiter shell init)"`: full privileges, outside WASM, and
   the manifest names its file without a path check. Shells are to run with limited privileges;
   how a hook sourced into an interactive shell is limited is undecided.
3. **Signing.** TODO. Nothing verifies who built a `.wasm` or that it is unchanged since install;
   the folder is the only identity. Signing is off during development only: once integrations are
   stable, a signed module will be required.
4. **A hostile guest can cost 30 s per call.** The timeout bounds one call, not the next. Quarantine
   is by command, not by CPU use.
5. **The engines are wazero's.** Their isolation is the library's, not something Orbiter
   re-implements; a wazero bug would be one here. The security tests catch regressions in how
   Orbiter configures it, not flaws inside it.
