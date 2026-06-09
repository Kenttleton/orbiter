# Orbiter

<!-- Banner art: replace with docs/assets/banner.png when ready -->
<!-- ![Orbiter](docs/assets/banner.png) -->

[![Build](https://github.com/kutterback/orbiter/actions/workflows/build.yml/badge.svg)](https://github.com/kutterback/orbiter/actions/workflows/build.yml)
[![Tests](https://github.com/kutterback/orbiter/actions/workflows/test.yml/badge.svg)](https://github.com/kutterback/orbiter/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kutterback/orbiter)](https://goreportcard.com/report/github.com/kutterback/orbiter)
[![Release](https://img.shields.io/github/v/release/kutterback/orbiter)](https://github.com/kutterback/orbiter/releases/latest)
[![Pre-release](https://img.shields.io/github/v/release/kutterback/orbiter?include_prereleases&label=pre-release)](https://github.com/kutterback/orbiter/releases)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](go.mod)

A cross-platform CLI/TUI for freelance and contract software engineers who regularly move between organizations, teams, projects, identities, credentials, and development environments.

<!-- Demo: replace with docs/assets/demo.gif when ready -->
<!-- ![orbit jump demo](docs/assets/demo.gif) -->

---

## What It Is

Orbiter is a **state-driven navigation and environment orchestration platform** centered around a persistent Star Chart — a SQLite database that records your complete development universe.

It is not a project management system. It is not a package manager. It is not a deployment platform.

Orbiter exists to answer one question: **how do I get back to working on this project, for this client, with the right identity, credentials, and tooling — as fast as possible?**

---

## The Mental Model

You are the **Captain**. Your workstation is the **Orbiter**. The places you work are organized as:

| Concept | Represents | Example |
|---|---|---|
| **Galaxy** | Organization or client | `acme`, `personal`, `spacex` |
| **Solar System** | Team or subdivision *(optional)* | `platform`, `mobile`, `backend` |
| **Planet** | Project | `payment-api`, `website`, `mobile-app` |
| **Callsign** | Your active identity | `kent-acme`, `kent-personal` |
| **Transponder** | Credential pointer | GitHub credentials, AWS profile, 1Password |
| **Resource** | Tooling or runtime | Node.js via nvm, Python via uv, Docker |

The **Star Chart** is the source of truth. It describes desired state. The filesystem is not.

**Beacons** record observed reality — what was verified to be true and when. The gap between desired state and observed state is **drift**.

---

## The Six Commands

Orbiter operations revolve around six primary commands:

```
orbit survey    — inspect desired state          "What is this thing?"
orbit chart     — preview a transition           "What would happen if I went there?"
orbit jump      — execute a transition           "Take me there."

orbit scan      — verify reality                 "What does reality currently look like?"
orbit calibrate — reconcile drift                "Bring reality back into alignment."
orbit retro     — retire obsolete entities       "Remove what no longer belongs."
```

### Example workflow

```bash
# Preview what switching to a new client project would require
orbit chart payment-api

# Execute the transition — clones the repo, activates credentials, installs tooling
orbit jump payment-api

# Later: verify everything is still healthy
orbit scan payment-api

# After a period away: reconcile any drift (expired credentials, missing tools)
orbit calibrate payment-api
```

---

## Building the Universe

Separate CRUD commands construct the Star Chart:

```bash
orbit galaxy add acme
orbit planet add payment-api --galaxy acme --repo https://github.com/acme/payment-api

orbit callsign add kent-acme --galaxy acme
orbit transponder add acme-github --callsign kent-acme --service github --location ~/.ssh/id_ed25519_acme

orbit resource add --planet payment-api --kind node --manager nvm --version 20
```

Context is inferred from current navigation state where possible — you don't need to specify `--galaxy` if you're already in that galaxy.

---

## Portability

The Star Chart is portable. Moving to a new machine:

```bash
# 1. Copy ~/.orbiter/starchart.db to the new machine
# 2. Install Orbiter
# 3. Verify current state
orbit scan
# 4. Reconcile any drift
orbit calibrate
# 5. Navigate to your project
orbit jump payment-api
```

Orbiter reconstructs your working environment from the Star Chart with minimal manual intervention.

---

## Architecture

### Two Binaries

| Binary | Role |
|---|---|
| `orbit` | CLI — all commands, Star Chart operations, source of truth |
| `orbiter` | TUI — visual universe and Beacon viewer, shells out to `orbit` |

`orbiter` requires `orbit` in PATH. Installing `orbiter` without `orbit` is not supported.

### Technology

| Concern | Choice |
|---|---|
| Language | Go — single static binaries, native cross-compilation |
| CLI | Cobra |
| TUI | Bubble Tea + Lipgloss |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGo) |
| Entity IDs | ULID — sortable, readable, globally unique |
| Build | Just (Justfile) |

### Star Chart Location

| Priority | Location |
|---|---|
| 1 | `ORBIT_STARCHART` environment variable |
| 2 | `~/.orbiter/starchart.db` |

---

## Design Principles

**The Star Chart must never be silently modified by external events.**

External systems drift — repositories disappear, credentials expire, organizations rename things. Orbiter records observations of drift in Beacons but does not automatically alter desired state. Only explicit Orbiter operations may modify the Star Chart.

**All state-changing operations follow a strict pipeline:**

```
Prepare → Validate → Execute → Verify → Commit
```

Failed operations roll back. The Star Chart is never left in an invalid state.

**Orbiter coordinates ecosystem tooling — it does not replace it.**

| Ecosystem | Manager |
|---|---|
| Node.js | nvm |
| Python | uv |
| Ruby | rbenv |
| Rust | rustup |
| .NET | official SDK tooling |
| Go | native tooling |

---

## Output

Orbiter outputs styled, Terraform-inspired terminal output by default. JSON output is available for scripting and TUI integration.

```bash
orbit survey payment-api           # styled (default)
orbit survey payment-api --output json
ORBIT_OUTPUT=json orbit scan       # env override
```

Progress during multi-step operations (like `orbit jump`) shows a persistent step list with live status:

```
Executing maneuver...

  [1/5] ✓ Plotting course...          Cloned acme/payment-api
  [2/5] ✓ Calibrating transponder...  GitHub credentials verified
  [3/5] ⠸ Acquiring resource...       Installing node v20 via nvm
  [4/5]   Acquiring resource...       Installing pnpm
  [5/5]   Sweeping sector...          Scanning payment-api
```

Add `--verbose` (or `ORBIT_VERBOSE=1`) to replace thematic labels with plain operational output and stream raw tool output inline — useful for CI and debugging stalled steps.

---

## Building

Requires [Just](https://github.com/casey/just).

```bash
just build        # build both binaries to bin/
just install      # go install both binaries
just test         # run all tests
just lint         # run golangci-lint
just clean        # remove bin/
```

---

## Non-Goals

Orbiter will not:

- Require repository metadata files or create project-specific configuration
- Replace language package managers or version managers
- Become CI/CD tooling, project management software, or infrastructure orchestration

---

## Status

Orbiter is in active development. The Star Chart schema and core internal packages are being built first; the full command surface follows in subsequent phases.
