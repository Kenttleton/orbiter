# Testing

Two kinds of tests, with different rules.

| Kind | Where | Rule |
| ---- | ----- | ---- |
| **Unit tests** | `internal/...`, `integrations/` | Mocked. Fake integrations, in-memory stores, no real tools. Fast and deterministic. |
| **Flows** | `cmd/orbiter/testdata/flows/*.txt` | Not mocked. Real bundled integrations installed by `vessel init`, real tools, real prompts. Each flow is an ordered list of steps that mimics real use end to end. |

Every end-to-end test is a flow; there are no other scripts. The flows (`cmd/orbiter/testdata/flows/`)
are numbered and each tells one story:

| Flow | Story |
| ---- | ----- |
| 01 | Register a project that uses git and have Orbiter find it (approval, discovery through the user's own shell, record, healthy) |
| 02 | The vessel's shell drifts and the Captain reconciles it |
| 03 | The shell finds git in two places and the Captain chooses |
| 04 | The Captain denies an approval |
| 05 | Retiring things from the tree; the integration folder stays |
| 06 | A client that is not attached answers prompts with `relay` |
| 07 | Setting up a vessel and managing its integrations (init, profile line, inspect, quarantine) |
| 08 | Building the universe: hierarchy, resources, callsigns, transponders |
| 09 | Calibrate and jump, with a tool the Captain has trusted |
| 10 | The mistakes a Captain makes, and what Orbiter says |
| 11 | The JSON contract of every command |

## Running flows

```bash
just flows-brief                      # the story: steps, prompts, findings, report
just flows                            # full output: every command and its stdout
just flow 01-register-and-calibrate-git   # one flow, full output
just unit                             # unit tests only (mocked)
just build-test-guests                # assemble the hostile/well-behaved wasm test guests (needs binaryen wasm-as)
```

Without `-v` (which the recipes pass) a flow only prints when it fails.

## Isolation, setup and teardown

Every flow gets its own work directory, `HOME` and `PATH`.

- **Setup** (`flowSetup` in `flows_test.go`): creates `HOME`, links the host tools a flow may
  discover into the flow's own `bin/` (today: `git`), and sets `PATH` to that directory plus
  the `orbiter` binary only. It also plays the user's login: `$SHELL` is the host's real
  login shell (zsh on a Mac) and `$FLOW_SHELL` its name, so a flow can run its commands in
  that shell. Orbiter is never told which shell; it auto-detects, which is what the flow checks. FIND therefore sees exactly one of each tool, and a flow never
  depends on what else is installed. It also sets a short approval timeout, so a flow that
  forgets to answer a prompt fails in seconds.
- **Teardown:** the work directory is removed by testscript, and each flow adds a line to
  the report printed at the end: its steps and its findings.

## Writing a flow

A flow is a testscript file. Besides testscript's own commands:

| Command | What it does |
| ------- | ------------ |
| `step <title>` | Logs `STEP n: title`, so the output reads as a story. |
| `finding <text>` | Records something the flow learned without failing it. Findings appear in the end-of-run report. |
| `client [-deny] [-select=N] [-text=V] [!] -- <orbiter args>` | Runs `orbiter --output json -i <args>` as a client, the way a GUI would: events go to stdout, and every `captain_prompt` is answered (approve by default; select option N; answer text V). Each answer is logged. If orbiter says nothing for 15 seconds the step fails with orbiter's stderr, because that almost always means it is waiting for input the client cannot see. |
| `capture <NAME> <regex>` | Sets `$NAME` to the first group of the regex in the last stdout. |

When a step hits a known gap, write the intended steps, record a `finding`, and end the flow
with `skip '<reason and roadmap item>'`. The suite stays green, the report says what is
blocked and why, and removing the `skip` re-enables the rest once the gap is fixed.
