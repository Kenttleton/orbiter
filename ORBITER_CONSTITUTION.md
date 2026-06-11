# ORBITER CONSTITUTION

## Overview

Orbiter is a cross-platform CLI/TUI for freelance and contract software engineers who regularly move between organizations, teams, projects, identities, credentials, and development environments.

The primary interface is the `orbit` CLI.

Orbiter is not a project management system.

Orbiter is not a package manager.

Orbiter is not a deployment platform.

Orbiter is a state-driven navigation and environment orchestration platform centered around a persistent Star Chart.

---

# Core Philosophy

The engineer operates a single Orbiter.

The engineer is the Captain.

The Orbiter travels between Galaxies, Solar Systems, and Planets.

The Orbiter maintains a Star Chart describing the engineer's universe.

The Star Chart is the source of truth.

The Star Chart is portable.

A Captain should be able to move the Star Chart to another machine and reconstruct their development universe through Orbiter.

---

# The Star Chart

The Star Chart is a SQLite database.

The Star Chart contains:

* Galaxies
* Solar Systems
* Planets
* Callsigns
* Transponders
* Resources
* Aliases
* Policies
* Defaults
* Navigation History
* Beacons

The filesystem is not the source of truth.

Repositories are destinations.

The Star Chart defines relationships.

---

# Architectural Rule

The Star Chart must never be silently modified by external events.

External systems may drift.

Repositories may disappear.

Credentials may expire.

Organizations may rename resources.

Orbiter records observations of drift but does not automatically alter desired state.

Only explicit Orbiter operations may modify desired state.

---

# Universe Model

## Captain

The user operating the Orbiter.

Only one Captain is active.

---

## Orbiter

Represents the local workstation.

Contains:

* Installed tooling
* Version managers
* Star Chart database
* Local caches

---

## Galaxy

Represents an organization or client.

Examples:

* personal
* spacex
* acme

Galaxies may define:

* default repository locations
* default Callsigns
* default Transponders
* default Resources
* policies

---

## Solar System

Optional organizational subdivision.

Examples:

* platform
* mobile
* backend

Solar Systems are optional.

Most users may never create them.

---

## Planet

Represents a project.

Examples:

* payment-api
* website
* mobile-app
* infrastructure

Planets are the primary navigation target.

Most Orbiter activity revolves around Planets.

---

# Callsigns

A Callsign represents the active identity of the Captain.

Only one Callsign may be active at a time.

Examples:

* kent-personal
* kent-acme
* kent-spacex

Galaxy transitions may automatically suggest Callsign changes.

---

# Transponders

Transponders point to credential locations and authentication services. They never store secrets.

Storing keys and secrets is strictly prohibited.

Orbiter stores where credentials live and how to access them — not the credentials themselves.

Every transponder has a `role` (access mechanism) and a `brand` (the service it grants access to):

| Role | Represents |
| --- | --- |
| `file` | Path to a key or cert file on disk |
| `env` | Environment variable name |
| `keychain` | OS keychain or credential store reference |
| `vault` | External secret manager (1Password, Doppler, HashiCorp Vault) |
| `agent` | Auth agent socket (SSH agent, GPG agent) |

Examples:

* `file + github` → SSH key file for GitHub access
* `vault + aws` → AWS credentials in 1Password
* `env + claude` → Claude API key in an environment variable
* `agent + github` → GitHub access via SSH agent

Galaxy transitions may automatically suggest Transponder changes.

---

# Resources

Resources describe tooling, dependencies, runtimes, and capabilities.

Every resource has a `role` (classification) and a `brand` (specific implementation). The combination drives behavior.

Resources are standalone entities. Scope is determined by attachments to hierarchy nodes via the Star Chart graph.

**Resource roles:**

| Role | Represents | Example brands |
| --- | --- | --- |
| `manager` | Installs and manages other resources | nvm, volta, homebrew, apt, uv |
| `runtime` | Application that runs on the machine | node, python, ruby, postgres, figma |
| `tool` | CLI tool or command-line application | git, docker, kubectl, make |
| `remote` | Remote sync target accessed via protocol | github, dropbox, s3, onedrive |
| `filesystem` | Local path or directory | (config-driven) |

Examples of role+brand combinations:

* `manager + nvm` = nvm version manager
* `runtime + node` = Node.js runtime
* `runtime + figma` = Figma desktop application
* `tool + git` = git CLI
* `remote + github` = a GitHub repository
* `remote + dropbox` = a Dropbox sync folder

Resources at the vessel level are available to all branches. Resources at galaxy or planet level are scoped to that branch. Lower-level resources override higher-level resources of the same role+brand.

---

# Resource Management Philosophy

Orbiter coordinates existing ecosystem tooling.

Orbiter does not replace ecosystem tooling.

Orbiter orchestrates tools via the Integration Registry — a set of compiled packages (one per role+brand) that know how to install, verify, and configure each tool. The Captain chooses which integrations apply to each branch of their universe.

Default integrations:

| Ecosystem | Manager role+brand |
| --------- | ------------------ |
| Node.js   | manager + nvm      |
| Python    | manager + uv       |
| Ruby      | manager + rbenv    |
| Rust      | manager + rustup   |
| .NET      | manager + dotnet   |
| Go        | runtime + go       |

Orbiter should not reimplement what these tools already do well.

# Integrations

Integrations are the behavior layer for Orbiter. Every resource and transponder role+brand pair that Orbiter can manage has a corresponding integration.

Integrations are stateless. They receive a resolved context describing their dependencies, interact with the real world, and report current observed state back to Orbiter.

Orbiter owns all state management. Integrations own all interaction with external tools, services, and the local environment.

Integrations have four responsibilities:

* **Detect** — identify whether this integration is relevant to the current directory
* **Init** — provision the resource or transponder in the real world
* **Scan** — observe and report current state without modifying anything
* **Calibrate** — bring reality into alignment with desired state

New integrations can be added by dropping a package into the integrations directory and rebuilding. Integrations are compiled WASM modules (Phase 2.5+). Phase 4 will add runtime plugin directories so integrations can be dropped next to the binary without recompiling.

---

# Discovery

Orbiter supports automatic discovery during `planet init` and `resource init`. Discovery is owned entirely by integrations — Orbiter core has no awareness of specific file types, binary names, or sync conventions.

Each integration declares what it looks for:

* **File-pattern integrations** (runtime, manager, tool): declare file patterns like `.nvmrc`, `pyproject.toml`, `Cargo.toml`. Discovery runs when those files are present in the project directory.
* **Always-run integrations** (remote, filesystem): always run during discovery. These check whether the current directory falls within their sync scope (e.g., inside a Dropbox folder) using the sync client's own configuration.

When discovery finds a match, the integration suggests resources to attach. The Captain confirms before anything is attached or initialized.

When discovery finds nothing:

* Manual registration is always available via `orbit resource add`

Manual configuration must always be available.

---

# Beacons

A Beacon represents the most recent verified observation of an entity.

Beacons are observation records.

They connect desired state with observed reality.

Example:

```text
Planet:
  payment-api

Status:
  Healthy

Verified:
  2026-06-09

Observed:
  Repository Reachable
  Resources Installed
  Callsign Valid
```

Beacons may exist for:

* Galaxies
* Solar Systems
* Planets
* Callsigns
* Transponders
* Resources (any level)

---

# Drift

Drift occurs when observed reality differs from desired environment configuration state.

Drift is strictly about environment configuration — installed tools, configured credentials, accessible remotes, authenticated services. Drift is not about file contents, data state, sync completeness, or runtime behavior.

Examples of drift:

* tool not installed or wrong version
* credential expired or pointing to the wrong service
* remote unreachable
* manager not configured for the right runtime

Examples that are NOT drift:

* files inside a sync folder are out of date
* a database has stale data
* an upstream repository has new commits
* an application is producing unexpected output

Drift should be recorded in Beacons.

Drift must never silently alter desired state.

---

# The Six Command Lifecycle

Orbiter operations revolve around six primary commands.

```text
Survey
Chart
Jump

Scan
Calibrate

Retro
```

---

## Survey

Inspect metadata.

Purpose:

"What is this thing?"

Examples:

```bash
orbit survey payment-api
orbit survey acme
```

Survey shows desired state.

Survey does not show deltas.

Survey does not modify state.

---

## Chart

Preview changes.

Purpose:

"What would happen if I went there?"

Examples:

```bash
orbit chart payment-api
orbit chart acme
```

Chart computes a transition plan.

Chart shows:

* Callsign changes
* Transponder changes
* Resource changes
* Repository actions
* Navigation actions

Chart performs no mutations.

---

## Jump

Execute a transition.

Purpose:

"Take me there."

Examples:

```bash
orbit jump payment-api
orbit jump acme
```

Jump may:

* navigate
* clone repositories
* install resources
* activate Callsigns
* activate Transponders
* update Beacons

Jump is the primary workflow.

---

## Scan

Verify reality.

Purpose:

"What does reality currently look like?"

Examples:

```bash
orbit scan
orbit scan payment-api
orbit scan acme
```

Scan:

* verifies resources
* verifies repositories
* verifies credentials
* verifies identities

Scan updates Beacons.

Scan does not modify desired state.

---

## Calibrate

Reconcile drift.

Purpose:

"Bring reality and the Star Chart back into alignment."

Examples:

```bash
orbit calibrate
orbit calibrate payment-api
orbit calibrate acme
```

Calibration may:

* repair resources
* update references
* refresh credentials
* update registrations
* reconcile renamed entities

Calibration is the only operational workflow intended to reconcile drift.

---

## Retro

Retire obsolete entities.

Purpose:

"Remove what no longer belongs in the universe."

Examples:

```bash
orbit retro
orbit retro payment-api
orbit retro acme
```

Retro may:

* archive planets
* archive galaxies
* remove stale registrations
* prune aliases
* remove deprecated entries

Retro is lifecycle management, not drift management.

---

# Building The Universe

The six lifecycle commands operate the universe.

Separate build commands construct the Star Chart.

There are three build verbs:

* `add` — register an entity in the Star Chart (Beacon: unverified)
* `init` — provision an entity in the real world (Beacon: verified or failed)
* `attach` — wire two entities together in the graph

Update and removal are handled by the lifecycle commands:

* `orbit calibrate <alias>` — reconcile and update
* `orbit retro <alias>` — retire and remove

Examples:

```bash
orbit galaxy add acme
orbit galaxy init acme

orbit planet init payment-api
orbit planet init payment-api https://github.com/acme/payment-api

orbit callsign add kent-acme
orbit transponder add acme-github --role file --brand github --location ~/.ssh/id_ed25519_acme
orbit resource add nvm --role manager --brand nvm --manages '["node"]'

orbit attach kent-acme acme
orbit attach acme-github kent-acme
orbit attach nvm vessel
```

---

# Context Inference

Orbiter should use the current Star Chart context whenever possible.

Examples:

```bash
orbit planet add payment-api
```

may infer:

* current Galaxy
* current Solar System

from current navigation state.

Explicit flags should override inference.

Explicit flags should also cascade the hierarchy such that specifying a galaxy should not assume the current solar system when adding a planet.

Inference should reduce command verbosity.

Calibration remains available when assumptions are incorrect.

---

# Star Chart Integrity

All state-changing operations must follow:

```text
Prepare
Validate
Execute
Verify
Commit
```

The Star Chart should only be updated after successful execution and verification.

Failed operations must not leave the Star Chart in an invalid state.

---

# Portability

A Captain should be able to:

1. Copy the Star Chart to a new machine.
2. Install Orbiter.
3. Run `orbit scan`.
4. Run `orbit calibrate`.
5. Run `orbit jump`.

and reconstruct their working universe with minimal manual intervention.

---

# Non-Goals

Orbiter should not:

* require repository metadata files
* create repository-specific configuration files
* replace language package managers
* replace version managers
* become CI/CD tooling
* become project management software
* become infrastructure orchestration software

Orbiter exists to help a Captain efficiently navigate and operate a development universe spanning multiple organizations, projects, identities, credentials, and environments.

---

# Foundational Principle

**The Star Chart represents the desired universe.**

**Beacons represent the observed universe.**

**Chart predicts change.**

**Jump executes change.**

**Scan observes reality.**

**Calibrate reconciles reality.**

**Retro retires reality.**

Everything in Orbiter exists to support those principles.
