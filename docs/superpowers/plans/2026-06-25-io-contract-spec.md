# Orbiter I/O Contract Specification

> **Status:** Design Spec
> **Supersedes:** `2026-06-25-relay-uplink-protocol-spec.md`
> **Depends on:** [`2026-06-24-filesystem-spec.md`](2026-06-24-filesystem-spec.md) ✅ implemented, [`2026-06-23-find-interface-spec.md`](2026-06-23-find-interface-spec.md)
> **Blocks:** FIND spec implementation plan, relay/uplink implementation
> **Followed by:** [`2026-06-22-binary-resolution-shell-directive.md`](2026-06-22-binary-resolution-shell-directive.md)

**Goal:** Define Orbiter's complete I/O contract — the output models shared by both output modes, the synchronous stdin interaction path (styled mode), and the asynchronous relay/uplink callback path (JSON mode). All three share the same underlying structs. The renderer is a presentation concern; the models are the contract.

**Architecture:** Orbiter has two output modes selected by `--output` flag or `ORBITER_OUTPUT` env var. In **styled mode** (default), Orbiter renders output to the terminal and reads Captain responses from stdin synchronously. In **JSON mode**, Orbiter serializes the same models as a structured event stream to stdout and receives Captain responses asynchronously via `orbiter relay`. Both modes implement the same renderer interface against the same model types. External clients — TUI, web UI, native app, MCP server — consume JSON mode output and call `orbiter relay` to respond. Orbiter does not launch or manage any UI; the communication contract is open to any client.

**Tech Stack:** Go, SQLite (existing StarChart database), shell functions (bash, zsh, fish, PowerShell), Lipgloss (styled renderer)

---

## Global Constraints

- All output models are defined in `internal/output/` and shared by both renderers
- `StyledRenderer` is Orbiter's built-in default UI — a thin rendering wrapper around the models
- `JSONRenderer` serializes models to stdout; the client is responsible for presentation
- Orbiter never launches UI — it communicates via stdout and receives input via stdin (styled) or `orbiter relay` (JSON)
- Session IDs use `models.NewID("ss")` — 16-char base36 snowflake, `ss` at positions 8–9; creation timestamp is embedded, no `created_at` column needed
- Transaction IDs use `models.NewID("tx")` — same scheme, `tx` at positions 8–9; creation timestamp embedded
- `orbiter relay` carries no `--type` flag — op type is decoded from the transaction record
- `orbiter relay` accepts `--response` as a flag or reads from stdin (pipe) when `--response` is absent — avoids CLI argument length limits for large FIND results
- Orbiter supports multiple concurrent sessions; session state lives in the sessions table, not an env var
- Tables are defined in init SQL — no migrations; this codebase has no deployed users requiring schema upgrades
- Mode is selected once per invocation: `--output json` flag or `ORBITER_OUTPUT=json` env var; default is styled

---

## 1. Output Models

These structs are the contract. Both renderers receive them. The JSON envelope wraps them for JSON mode output.

### 1.1 Event Envelope (JSON mode only)

```go
// Event is the top-level JSON output envelope in JSON mode.
// Every line of stdout in JSON mode is one Event serialized as a single JSON object.
// Txn is always set — every event is a traceable transaction, not just prompts.
// This allows concurrent Orbiter sessions to be distinguished and gives a complete
// audit trail of all interactions between Orbiter and any client for debugging.
type Event struct {
    Type string          `json:"type"` // see event types below
    Txn  string          `json:"txn"`  // always non-empty; NewID("tx") snowflake
    Data json.RawMessage `json:"data"`
}
```

Event types:

| `type` | Awaits relay? | Description |
| ------ | ------------- | ----------- |
| `plan` | no | A set of planned changes (add/change/remove) |
| `progress` | no | A multi-step operation in progress |
| `result` | no | Operation completed; success or failure |
| `error` | no | Unrecoverable error |
| `captain_prompt` | yes | Captain input required; relay response expected |

All events are recorded in the transactions table. Only `captain_prompt` events transition to `relayed` via `orbiter relay` — all others are written as `processed` immediately on emit. This gives a complete ordered log of every Orbiter ↔ client exchange, queryable by session for debugging concurrent sessions.

In styled mode the event envelope is not used — the renderer calls its own typed methods directly. Styled mode still generates txn IDs internally for the transactions table; they are not visible in terminal output.

### 1.2 PlanStep

Already defined in `internal/output/renderer.go`. Documented here as part of the contract.

```go
type PlanStep struct {
    Action      string `json:"action"`       // "add" | "change" | "remove"
    EntityType  string `json:"entity_type"`
    Name        string `json:"name"`
    Description string `json:"description"`
}
```

Styled: renders as `+`/`~`/`-` prefixed lines with Lipgloss colours.
JSON: serialized inside an `Event{Type: "plan"}` with `Data: []PlanStep`.

### 1.3 ProgressStep

Already defined in `internal/output/renderer.go`. Documented here as part of the contract.

```go
type ProgressStep struct {
    ThematicLabel string `json:"thematic_label"` // sci-fi themed, for styled output
    PlainLabel    string `json:"plain_label"`     // semantic label, for JSON clients
    Status        string `json:"status"`          // "pending" | "running" | "done" | "failed"
}
```

Styled: renders thematic label with status indicator.
JSON: serialized inside an `Event{Type: "progress"}` with `Data: []ProgressStep`.

### 1.4 Result

```go
type Result struct {
    Success bool        `json:"success"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

Styled: success renders as `✓` prefix (green); failure as `✗` prefix (red).
JSON: serialized inside an `Event{Type: "result"}`.

### 1.5 CaptainPrompt

New. Used wherever Orbiter needs input from the Captain. Ops are generic and UI-agnostic — the client decides how to render each op; Orbiter only declares what kind of input it needs.

```go
type CaptainPrompt struct {
    Txn     string            `json:"txn"`
    Op      string            `json:"op"`               // SELECT | CONFIRM | TEXT
    Message string            `json:"message"`
    Options []string          `json:"options,omitempty"` // SELECT: the list of choices
    Variant string            `json:"variant,omitempty"` // see variant table below
    Meta    map[string]string `json:"meta,omitempty"`    // additional context for the client
}
```

Prompt ops:

| `op` | Description | Relay response shape |
| ---- | ----------- | -------------------- |
| `SELECT` | Choose from `options` | `{"selected": "..."}` (single) or `{"selected": ["...", "..."]}` (multi) |
| `CONFIRM` | Yes/no decision | `{"confirmed": true}` or `{"confirmed": false}` |
| `TEXT` | Free-form text input | `{"value": "..."}` |

Variants by op:

| `op` | `variant` | Description |
| ---- | --------- | ----------- |
| `SELECT` | `single` | Exactly one choice from `options` |
| `SELECT` | `multi` | One or more choices from `options` |
| `TEXT` | `short` | Single-line text input |
| `TEXT` | `long` | Multi-line text input |
| `TEXT` | `obscured` | Single-line, value masked (passwords, tokens, secrets) |
| `CONFIRM` | — | No variant; always yes/no |

Examples of how Orbiter uses these generic ops:

| Orbiter situation | Op | Notes |
| ----------------- | -- | ----- |
| Multiple binary locations found | `SELECT` | `variant: "single"`, `options`: found paths |
| Integration requests command approval | `CONFIRM` | `message`: brand + full command |
| Destructive operation acknowledgment | `CONFIRM` | — |
| Credential or token input | `TEXT` | `variant: "obscured"` |
| Branch description or notes | `TEXT` | `variant: "long"` |

Styled: renders each op as an appropriate terminal interaction (numbered list, y/N prompt, input field); blocks on stdin; returns the response inline.
JSON: serialized inside `Event{Type: "captain_prompt"}` — client renders and calls `orbiter relay`.

---

## 2. Renderer Interface

The renderer interface gains a `Prompt` method alongside existing methods. Both renderers implement the full interface.

```go
type Renderer interface {
    Info(msg string)
    Success(msg string)
    Warning(msg string)
    Error(msg string)
    Plan(steps []PlanStep)
    Progress(steps []ProgressStep)
    Table(headers []string, rows [][]string)
    JSON(v any)

    // Prompt presents a CaptainPrompt and returns the Captain's response as a JSON string.
    // The response shape matches the relay --response payload for the same op:
    //   SELECT single:  {"selected": "..."}
    //   SELECT multi:   {"selected": ["...", "..."]}
    //   CONFIRM:        {"confirmed": true|false}
    //   TEXT:           {"value": "..."}
    // Styled mode: blocks on stdin, collects input, serializes to JSON, returns response string.
    //   Records the transaction as processed immediately on selection.
    // JSON mode: emits Event{Type:"captain_prompt"} to stdout, returns ("", nil).
    //   Transaction is already recorded as pending before Prompt is called.
    //   Caller exits; response arrives as the --response payload of orbiter relay.
    //   Both paths produce the same JSON shape — callers parse response uniformly.
    Prompt(p CaptainPrompt) (response string, err error)
}
```

The command layer checks the return value: a non-empty `response` means the answer arrived synchronously (styled mode); empty `response` with nil error means the interaction is pending (JSON mode) and the caller should exit after persisting the transaction.

---

## 3. Stdout

### 3.1 Styled mode

The `StyledRenderer` calls its own typed render methods. Output goes directly to the terminal via Lipgloss. No envelope. Line-by-line human-readable output.

### 3.2 JSON mode

The `JSONRenderer` wraps every output call in the `Event` envelope and writes one JSON object per line to stdout (newline-delimited JSON). Clients read stdout line by line and parse each line as an `Event`.

```jsonc
// Example: a plan with two steps
{"type":"plan","txn":"a3f2k1b8tx9r2m4p","data":[{"action":"add","entity_type":"resource","name":"brew","description":"homebrew package manager"},{"action":"change","entity_type":"beacon","name":"brew","description":"path updated"}]}

// Example: a captain_prompt waiting for relay
{"type":"captain_prompt","txn":"b4g3l2c9tx0s3n5q","data":{"txn":"b4g3l2c9tx0s3n5q","op":"SELECT","variant":"single","message":"Multiple locations found for brew","options":["/opt/homebrew/bin/brew","/usr/local/bin/brew"]}}

// Example: result
{"type":"result","txn":"c5h4m3d0tx1t4o6r","data":{"success":true,"message":"branch calibrated"}}
```

---

## 4. Stdin (Sync — Styled Mode Only)

Synchronous Captain interactions read from `os.Stdin` directly. This path is only active in styled mode. In JSON mode `Prompt()` emits an event and returns immediately without reading stdin.

### Existing: command approval

`StdinApproveFunc` in `internal/wasm/host.go` is the current implementation. It presents the brand and full command string and reads `y/N`. This becomes `renderer.Prompt(CaptainPrompt{Op: "CONFIRM", Message: brand + " wants to run: " + cmd})` — routes through the renderer interface and becomes a `captain_prompt` event in JSON mode.

### New: path selection

When `orbiter uplink` finds multiple locations for a binary, `renderer.Prompt(CaptainPrompt{Op: "SELECT", Variant: "single", Options: matches})` renders a numbered list in styled mode and reads the Captain's selection from stdin.

### New: text and confirmation

`renderer.Prompt(CaptainPrompt{Op: "CONFIRM"})` for approvals and destructive operations. `renderer.Prompt(CaptainPrompt{Op: "TEXT", Variant: "obscured"})` for credential input.

---

## 5. Relay/Uplink (Async — JSON Mode)

### 5.1 When it applies

Any `renderer.Prompt()` call in JSON mode results in a pending transaction. The shell or client must call `orbiter relay` to provide the response, then `orbiter uplink` to continue processing.

FIND operations (INIT and drift recovery) always use the async path because shell FIND functions (`find /`, `Get-ChildItem -Recurse`) are OS-native global searches that cannot run inside Orbiter's Go process.

### 5.2 Entity type codes

Both sessions and transactions use `models.NewID("<code>")` — the same snowflake scheme as all Orbiter entities. Creation timestamp is embedded; no `created_at` column needed.

| Code | Entity | Example usage |
| ---- | ------ | ------------- |
| `tx` | Transaction | `models.NewID("tx")` — one per Orbiter↔client interaction |
| `ss` | Session | `models.NewID("ss")` — one per Orbiter invocation |

Snowflake layout:

| Position | Content |
| -------- | ------- |
| 0–7 | Base36 millisecond timestamp (epoch: 2025-01-01 UTC) |
| 8–9 | Entity type code (`tx` or `ss`) |
| 10–15 | Base36 random component |

```go
sessionID := models.NewID("ss") // e.g., "a3f2k1b8ss9r2m4p"
txnID     := models.NewID("tx") // e.g., "b4g3l2c9tx0s3n5q"
```

### 5.3 Database schema

**Note:** All table definitions live in the init SQL file (`internal/starchart/init.sql` or equivalent). No migrations — this codebase has no deployed users requiring schema upgrades. Modify init SQL directly.

#### Sessions table

Tracks active Orbiter invocations. Supports multiple concurrent sessions without relying on environment variables.

```sql
CREATE TABLE sessions (
    id       TEXT    PRIMARY KEY,                -- models.NewID("ss") snowflake
    status   TEXT    NOT NULL DEFAULT 'active',  -- 'active' | 'complete' | 'failed'
    meta     TEXT                                -- JSON; what the session is resolving (op, branch, etc.)
);

CREATE INDEX sessions_status_idx ON sessions (status);
```

The session ID embeds its creation timestamp — no `created_at` column needed. `meta` stores structured context (e.g. `{"op":"INIT","branch":"main"}`) so `orbiter uplink` and other tools know what a session was attempting when they resume it.

#### Transactions table

```sql
CREATE TABLE transactions (
    id         TEXT    PRIMARY KEY,           -- models.NewID("tx") snowflake
    session_id TEXT    NOT NULL,              -- references sessions(id)
    op         TEXT    NOT NULL,              -- see op table below
    args       TEXT    NOT NULL,              -- JSON: operation arguments
    status     TEXT    NOT NULL DEFAULT 'pending', -- TransactionStatus enum
    payload    TEXT,                          -- JSON: the event data Orbiter sent
    response   TEXT                           -- JSON: the client's relay response
);

CREATE INDEX transactions_session_status_idx ON transactions (session_id, status);
```

Column notes:

- `id`: snowflake embeds creation timestamp — no `created_at` column needed
- `session_id`: groups all transactions from one Orbiter invocation
- `payload`: the `Event.Data` JSON that Orbiter emitted (stored for audit/debug)
- `response`: the `--response` JSON from `orbiter relay` (NULL until relayed)

#### TransactionStatus Go enum

```go
type TransactionStatus string

const (
    TransactionStatusPending   TransactionStatus = "pending"
    TransactionStatusRelayed   TransactionStatus = "relayed"
    TransactionStatusProcessed TransactionStatus = "processed"
)
```

#### Op values

| `op` | Category | Initiated by | Responded by |
| ---- | -------- | ------------ | ------------ |
| `FIND_BINARY` | Discovery | Lifecycle (INIT / drift recovery) | Shell `FIND_BINARY` function |
| `FIND_FILE` | Discovery | Lifecycle (INIT only) | Shell `FIND_FILE` function |
| `FIND_DIR` | Discovery | Lifecycle (INIT / drift recovery) | Shell `FIND_DIR` function |
| `FIND_ENV` | Discovery | Lifecycle (INIT / drift recovery) | Shell `FIND_ENV` function |
| `SELECT` | Prompt | Any Orbiter operation needing a choice | Any client |
| `CONFIRM` | Prompt | Any Orbiter operation needing approval | Any client |
| `TEXT` | Prompt | Any Orbiter operation needing text input | Any client |

Discovery ops are responded to by shell FIND functions. Prompt ops are responded to by whatever client is consuming the JSON stream — TUI, web UI, native app, MCP server, or the built-in styled renderer reading stdin.

**Stale transaction cleanup:** On any Orbiter invocation, transactions with `status = pending` or `relayed` older than 24 hours are deleted. Age is derived from `models.ParseID(id).Timestamp()` — no `created_at` column needed.

### 5.4 FIND Directive Format

Orbiter emits FIND directives with the transaction ID embedded. The transaction record is written before the directive is emitted.

```
::ORBITER::FIND_BINARY:a3f2k1b8tx9r2m4p brew
::ORBITER::FIND_FILE:b4g3l2c9tx0s3n5q "*.json" /project
::ORBITER::FIND_DIR:c5h4m3d0tx1t4o6r ~/.config/orbiter
::ORBITER::FIND_ENV:d6i5n4e1tx2u5p7s HOMEBREW_PREFIX
```

### 5.5 Shell FIND Function Signatures

All four FIND functions accept `txn_id` as their first argument and call `orbiter relay` at the end.

```
FIND_BINARY <txn_id> <name>
FIND_FILE   <txn_id> <pattern> [location]
FIND_DIR    <txn_id> <path>
FIND_ENV    <txn_id> <name>
```

FIND_BINARY performs a global OS search — not a PATH walk. Returns all locations where the named executable exists on the filesystem. Results are piped to `orbiter relay` via stdin to avoid shell argument length limits (a recursive filesystem search can return hundreds of paths).

**bash:**
```bash
function FIND_BINARY() {
    local txn="$1"
    local name="$2"
    find / -type f -name "$name" -perm +111 2>/dev/null | \
        awk 'BEGIN{printf "{\"status\":\"ok\",\"matches\":["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]}"}' | \
        orbiter relay --txn="$txn"
}
export -f FIND_BINARY
```

**PowerShell:**
```powershell
function FIND_BINARY {
    param([string]$Txn, [string]$Name)
    $found = Get-ChildItem -Path C:\ -Filter $Name -Recurse -ErrorAction SilentlyContinue |
        Where-Object { -not $_.PSIsContainer } |
        ForEach-Object { "`"$($_.FullName -replace '\\', '/')`"" }
    if ($null -eq $found) { $found = @() }
    "{`"status`":`"ok`",`"matches`":[$($found -join ',')]}" | orbiter relay --txn=$Txn
}
```

### 5.6 `orbiter relay`

```
orbiter relay --txn=<id> [--response=<json>]
```

Stores one result into the transactions table. Idempotent — silently exits 0 if the transaction is already relayed.

When `--response` is omitted, `orbiter relay` reads the JSON response from stdin. This is the preferred path for FIND functions where result payloads can be large (hundreds of filesystem paths exceed shell argument length limits).

1. Read `--txn`; read `--response` flag or drain stdin if flag absent
2. Look up transaction by `id`
3. Verify `status == pending`; exit 0 if already relayed
4. `UPDATE transactions SET status='relayed', response=? WHERE id=?`
5. Exit 0

### 5.7 `orbiter uplink`

```
orbiter uplink --session=<id>
```

Processes all relayed transactions for the given session and continues the operation.

1. Look up session by `--session=<id>` in the sessions table; abort with error if not found or not `active`
2. Query `SELECT * FROM transactions WHERE session_id=? AND status='relayed'`
3. Process each transaction by `op`:
   - `FIND_BINARY` / `FIND_DIR` / `FIND_FILE` / `FIND_ENV`: ingest discovery results; if `FIND_BINARY` returns multiple matches, create a `SELECT` transaction and emit `captain_prompt` event — exit after emitting; client calls relay then uplink again
   - `SELECT`: record the Captain's chosen value in StarChart (path, option, etc.)
   - `CONFIRM`: record approval decision; continue operation gate
   - `TEXT`: record input value; continue operation
4. Mark processed transactions `status='processed'`
5. Report any remaining `pending` transactions as failed; continue with what was received
6. Emit final directives or result event
7. Update session `status='complete'` (or `'failed'` on unrecoverable error)
8. Exit 0

### 5.8 Session Lifecycle

```
Orbiter (INIT or CALIBRATE drift recovery, JSON mode)
  → INSERT sessions (id=NewID("ss"), status='active', meta={"op":"INIT",...})
  → emit ::ORBITER::SESSION:<session_id> to shell env setup
  → for each FIND needed:
      INSERT transaction (id=NewID("tx"), session_id, op, args, status='pending')
      emit ::ORBITER::<OP>:<txn_id> <args>
  → exit

Shell hook
  → capture session_id from ::ORBITER::SESSION directive
  → parse each ::ORBITER::<OP> directive
  → call FIND_<TYPE> <txn_id> <args>  ← each pipes result to orbiter relay internally
  → orbiter uplink --session=<session_id>

orbiter relay  (one invocation per FIND result)
  → drain stdin or read --response flag
  → UPDATE transaction: status='relayed', response=<json>
  → exit

orbiter uplink --session=<session_id>  (first pass)
  → process relayed FIND transactions
  → if FIND_BINARY returned multiple matches:
      INSERT transaction (SELECT, pending, payload=captain_prompt event)
      emit captain_prompt event to stdout
  → exit (client calls relay then uplink again for pending prompts)

orbiter relay  (client responds to SELECT captain_prompt)
  → UPDATE SELECT transaction: status='relayed', response=<json>
  → exit

orbiter uplink --session=<session_id>  (second pass)
  → process SELECT response
  → record resolved path/value in StarChart
  → UPDATE sessions: status='complete'
  → emit result event
  → exit
```

---

## 6. Cross-Platform

`orbiter relay` and `orbiter uplink` are CLI sub-commands of the Orbiter binary — the same binary on all platforms. No shell-specific transport.

| Shell | Platform | Global search tool |
| ----- | -------- | ------------------ |
| bash / zsh / fish | Linux / macOS / WSL | `find / -type f -name <n> -perm +111` |
| PowerShell | Windows | `Get-ChildItem -Path C:\ -Filter <n> -Recurse` |
| PowerShell | macOS | `find / -type f -name <n> -perm +111` via subshell |

---

## 7. Dependency Chain

```text
filesystem spec (✅ implemented: df1ff72..6f97bab)
    └── I/O contract spec [this]
          └── FIND spec implementation plan
                └── exec model refactor
                      └── Present/Reachable refactor
                            └── 2026-06-22-binary-resolution-shell-directive.md
```

**This spec defines:** output model types (`Event`, `CaptainPrompt`, `PlanStep`, `ProgressStep`, `Result`), renderer interface `Prompt` method, stdout event envelope (JSON mode), stdin sync path (styled mode), relay/uplink async path, transaction table schema, transaction op types, FIND function signatures with txn_id, `orbiter relay` and `orbiter uplink` sub-command contracts.

**FIND spec implementation plan defines:** global search implementations per shell, lifecycle.go changes that emit FIND directives, Captain path-selection UX.

**Exec model refactor defines:** how integrations invoke commands after FIND results are resolved into `ResolvedContext`.
