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

Transponders represent credentials, secrets, and authentication material. 

Storing keys and secrets is strictly prohibited. 

Instead Orbiter stores locations and services that contain this information to integrate with.

Examples:

* GitHub credentials
* AWS credentials
* Azure credentials
* SSH keys
* API tokens
* 1Password

Galaxy transitions may automatically suggest Transponder changes.

---

# Resources

Resources describe tooling, dependencies, runtimes, and capabilities.

Resources may exist at multiple scopes.

Resources are not part of the navigation hierarchy.

They form a separate inheritance hierarchy.

## Orbiter Resources

Machine-wide tooling.

Examples:

* Git
* Docker
* VSCode
* Rider
* Homebrew

## Callsign Resources

Resources associated with a Callsign.

Examples:

* identity-specific tooling
* account-specific configuration

## Transponder Resources

Resources associated with authentication systems.

Examples:

* credential providers
* secret managers
* certificate locations
* Environment Variables

## Galaxy Resources

Organization-wide requirements.

Examples:

* VPN clients
* approved IDEs
* security tooling

## Solar System Resources

Team-specific requirements.

Examples:

* shared tooling
* internal utilities

## Planet Resources

Project-specific requirements and where to manage them. 

Some planets may require management outside of the global default.

Examples:

* Node.js managed by nvm
* Python managed by uv
* Rust managed by rustup
* .NET
* pnpm
* cargo

---

# Resource Management Philosophy

Orbiter coordinates existing ecosystem tooling.

Orbiter does not replace ecosystem tooling.

Preferred version managers:

| Ecosystem | Manager              |
| --------- | -------------------- |
| Node.js   | nvm                  |
| Python    | uv                   |
| Ruby      | rbenv                |
| Rust      | rustup               |
| .NET      | official SDK tooling |
| Go        | native tooling       |

Orbiter orchestrates these tools.

Orbiter should not reimplement them.

---

# Discovery

Orbiter should support automatic discovery whenever possible.

Examples:

Node.js

* package.json
* .nvmrc

Python

* pyproject.toml

Rust

* Cargo.toml

Go

* go.mod

.NET

* global.json
* project files

When discovery succeeds:

* infer dependencies
* infer versions
* infer tooling

When discovery fails:

* allow manual registration

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

Drift occurs when observed reality differs from desired state.

Examples:

* repository unavailable
* credential expired
* resource missing
* renamed repository
* renamed identity

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

Separate CRUD operations build the universe.

Examples:

```bash
orbit galaxy add
orbit galaxy edit
orbit galaxy remove

orbit system add
orbit system edit
orbit system remove

orbit planet add
orbit planet init
orbit planet edit
orbit planet remove

orbit callsign add
orbit transponder add
orbit resource add
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
