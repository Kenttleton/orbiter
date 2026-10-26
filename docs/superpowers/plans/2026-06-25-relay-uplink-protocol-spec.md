# Relay/Uplink Protocol Specification

> **Status:** Superseded by [`2026-06-25-io-contract-spec.md`](2026-06-25-io-contract-spec.md)
> **Depends on:** [`2026-06-24-filesystem-spec.md`](2026-06-24-filesystem-spec.md) ✅ implemented, [`2026-06-23-find-interface-spec.md`](2026-06-23-find-interface-spec.md)
> **Blocks:** FIND spec implementation plan
> **Followed by:** [`2026-06-22-binary-resolution-shell-directive.md`](2026-06-22-binary-resolution-shell-directive.md)

**Goal:** Define the shell→Orbiter data transport used during FIND operations (INIT and drift recovery). Shell FIND functions perform OS-native global searches and relay results back to Orbiter via `orbiter relay`. Orbiter processes all accumulated results via `orbiter uplink`.

**Architecture:** Orbiter creates a transaction record before emitting each FIND directive. Shell FIND functions perform the global search and call `orbiter relay --txn --data` with the result. When all FIND operations complete, the shell hook calls `orbiter uplink`, which reads all relayed transactions for the current session, processes results (prompting the Captain if multiple matches exist), and emits final directives. Both `relay` and `uplink` are stateless CLI sub-commands of the Orbiter binary — no daemon, no socket, no HTTP.

**Tech Stack:** Go, SQLite (existing StarChart database), shell functions (bash, zsh, fish, PowerShell)

---

## Global Constraints

- Transaction IDs use `models.NewID("tx")` — 16-char base36 snowflake, `tx` at positions 8–9
- `orbiter relay` carries no `--type` flag — operation type is decoded from the transaction record
- `orbiter relay` and `orbiter uplink` are sub-commands of the existing Orbiter CLI binary
- No HTTP, no daemon, no persistent socket
- Must work identically across: bash/Linux, bash/macOS, bash/WSL, zsh/macOS, fish/Linux, fish/macOS, PowerShell/Windows, PowerShell/macOS
- Transactions table lives in the existing StarChart SQLite database (same file, same connection path)
- `ORBITER_SESSION_ID` env var groups all transactions from one Orbiter invocation

---

## 1. Why This Protocol Exists

FIND operations during INIT and drift recovery require OS-native global filesystem search — `find /`, `Get-ChildItem -Recurse`. Go's `filepath.Walk` from `/` is prohibitively slow and does not leverage OS filesystem indexes. These searches must be performed by the shell using OS-native tools.

Orbiter cannot receive results from the shell through stdout (already used for directives to the shell hook) or by holding a connection open (Orbiter is stateless — it emits directives and exits). The relay/uplink protocol solves this: Orbiter writes pending transactions to the database before exiting, the shell performs discovery and calls `orbiter relay` per result, then `orbiter uplink` reads and processes all responses in a fresh invocation.

This is also the **unified input interface** for all shell→Orbiter data. Any future operation where the shell needs to return structured data to Orbiter (TUI responses, user selections, export values) uses the same `orbiter relay --txn --data` pattern with a corresponding transaction record.

---

## 2. Transaction ID Format

Transaction IDs follow the existing Orbiter snowflake scheme defined in `internal/models/id.go`:

| Position | Content |
| -------- | ------- |
| 0–7 | Base36 millisecond timestamp (epoch: 2025-01-01 UTC) |
| 8–9 | Entity type code |
| 10–15 | Base36 random component |

New entity type code:

| Code | Entity |
| ---- | ------ |
| `tx` | Transaction |

```go
txnID := models.NewID("tx")
// e.g., "a3f2k1b8tx9r2m4p"
```

The FIND operation type (`FIND_BINARY`, `FIND_FILE`, `FIND_DIR`, `FIND_ENV`) is stored in the transaction record, not encoded in the ID. The `tx` code identifies the record as a transaction; the specific operation lives in the `op` column.

---

## 3. Transactions Table

New table added to the existing StarChart SQLite database:

```sql
CREATE TABLE transactions (
    id          TEXT    PRIMARY KEY,
    session_id  TEXT    NOT NULL,
    op          TEXT    NOT NULL,
    args        TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'pending',
    data        TEXT,
    created_at  INTEGER NOT NULL
);

CREATE INDEX transactions_session_status_idx ON transactions (session_id, status);
```

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | TEXT | `NewID("tx")` snowflake |
| `session_id` | TEXT | Groups all transactions from one Orbiter invocation; passed to shell as `ORBITER_SESSION_ID` |
| `op` | TEXT | `FIND_BINARY` \| `FIND_FILE` \| `FIND_DIR` \| `FIND_ENV` |
| `args` | TEXT | JSON — operation arguments e.g. `{"name":"brew"}`, `{"pattern":"*.json","location":"/project"}` |
| `status` | TEXT | `pending` → `relayed` → `processed` |
| `data` | TEXT | JSON response from the shell; NULL until relayed |
| `created_at` | INTEGER | Unix milliseconds |

**Stale transaction cleanup:** On any Orbiter invocation, transactions with `status = pending` or `relayed` older than 24 hours are deleted. A session that never completed `uplink` leaves no permanent residue.

---

## 4. FIND Directive Format

Orbiter embeds the transaction ID in each FIND directive. The transaction record is written to the database **before** the directive is emitted.

```text
::ORBITER::FIND_BINARY:a3f2k1b8tx9r2m4p brew
::ORBITER::FIND_FILE:b4g3l2c9tx0s3n5q "*.json" /project
::ORBITER::FIND_DIR:c5h4m3d0tx1t4o6r ~/.config/orbiter
::ORBITER::FIND_ENV:d6i5n4e1tx2u5p7s HOMEBREW_PREFIX
```

Format: `::ORBITER::<OP>:<txn_id> <args>`

The shell hook parses each directive, extracts the txn ID and args, and calls the matching FIND function.

---

## 5. Shell FIND Function Signatures

All FIND functions accept `txn_id` as their first argument. This is the only change from the FIND spec's original signatures — the search logic and output format are unchanged.

```text
FIND_BINARY <txn_id> <name>
FIND_FILE   <txn_id> <pattern> [location]
FIND_DIR    <txn_id> <path>
FIND_ENV    <txn_id> <name>
```

Each function performs its operation and ends by calling `orbiter relay`. The relay call is the last action — the function's only observable side effect is the transaction update.

**bash example:**

```bash
function FIND_BINARY() {
    local txn="$1"
    local name="$2"
    local matches
    matches=$(find / -type f -name "$name" -perm +111 2>/dev/null | \
        awk 'BEGIN{printf "["} NR>1{printf ","} {printf "\"%s\"", $0} END{printf "]"}')
    orbiter relay --txn="$txn" --data="{\"status\":\"ok\",\"matches\":${matches:-[]}}"
}
export -f FIND_BINARY
```

**PowerShell example:**

```powershell
function FIND_BINARY {
    param([string]$Txn, [string]$Name)
    $found = Get-ChildItem -Path C:\ -Filter $Name -Recurse -ErrorAction SilentlyContinue |
        Where-Object { -not $_.PSIsContainer } |
        ForEach-Object { "`"$($_.FullName -replace '\\', '/')`"" }
    if ($null -eq $found) { $found = @() }
    $data = "{`"status`":`"ok`",`"matches`":[$($found -join ',')]}"
    orbiter relay --txn=$Txn --data=$data
}
```

---

## 6. `orbiter relay`

Stores one FIND result into the transactions table.

```text
orbiter relay --txn=<id> --data=<json>
```

**Behavior:**

1. Read `--txn` and `--data` from flags
2. Look up the transaction by `id` in the database
3. If `status != pending`: exit 0 silently (idempotent — shell may retry on error)
4. Parse `--data` as JSON for basic validity; store raw if parse fails (uplink handles error data)
5. `UPDATE transactions SET status='relayed', data=? WHERE id=?`
6. Exit 0

`orbiter relay` is a thin database write. It does not process the data, make decisions, or emit directives.

---

## 7. `orbiter uplink`

Processes all relayed transactions for the current session and continues the Orbiter operation.

```text
orbiter uplink
```

**Behavior:**

1. Read `ORBITER_SESSION_ID` from env — abort with clear error if absent
2. Query: `SELECT * FROM transactions WHERE session_id = ? AND status = 'relayed'`
3. For each transaction, process according to `op`:

   | `op` | Action |
   | ---- | ------ |
   | `FIND_BINARY` | Parse matches array. If empty: record binary as not found. If one: record path. If multiple: prompt Captain to choose (interactive terminal prompt listing all found paths). Record chosen path in StarChart. |
   | `FIND_FILE` | Record discovered file paths for the pattern. |
   | `FIND_DIR` | Record directory existence and path. |
   | `FIND_ENV` | Record env var value. |

4. Mark each processed transaction `status = 'processed'`
5. Check for any remaining `pending` transactions — report each as failed (relay never arrived) and continue with what was received
6. Emit final directives (DIR, SET, etc.) based on resolved data
7. Exit 0

---

## 8. Session Lifecycle

```text
Orbiter (INIT or CALIBRATE drift recovery)
  → generate ORBITER_SESSION_ID = NewID("tx")[0:8]  (timestamp prefix only — session is not an entity)
  → export ORBITER_SESSION_ID to shell env
  → for each FIND needed:
      INSERT INTO transactions (id=NewID("tx"), session_id, op, args, status='pending', created_at)
      emit ::ORBITER::<OP>:<txn_id> <args> to stdout
  → exit

Shell hook
  → reads each ::ORBITER:: directive from Orbiter's stdout
  → for each FIND directive: calls FIND_BINARY/FILE/DIR/ENV <txn_id> <args>
  → each FIND function → orbiter relay --txn=<id> --data=<json>
  → after all directives processed: orbiter uplink

orbiter relay  (one invocation per FIND result)
  → UPDATE transaction: status=relayed, data=<json>
  → exit

orbiter uplink  (one invocation at end)
  → process all relayed transactions for this session
  → prompt Captain where needed
  → record results in StarChart
  → emit final directives
  → exit
```

---

## 9. Cross-Platform

`orbiter relay` and `orbiter uplink` are CLI sub-commands of the Orbiter binary — the same binary on all platforms. No shell-specific transport.

| Shell | Platform | Calls |
| ----- | -------- | ----- |
| bash | Linux / macOS / WSL | `orbiter relay ...` / `orbiter uplink` |
| zsh | macOS | same |
| fish | Linux / macOS | same |
| PowerShell | Windows / macOS | `orbiter relay ...` / `orbiter uplink` |

On Windows, `orbiter` resolves to `orbiter.exe`. The SQLite database path follows the existing StarChart convention, which is already cross-platform.

FIND functions use OS-native global search tools:

| Shell | Search tool |
| ----- | ----------- |
| bash / zsh / fish | `find / -type f -name <name> -perm +111` |
| PowerShell (Windows) | `Get-ChildItem -Path C:\ -Filter <name> -Recurse` |
| PowerShell (macOS) | `find / -type f -name <name> -perm +111` via `bash -c` or native PS |

---

## 10. Dependency Chain

```text
filesystem spec (✅ implemented: df1ff72..6f97bab)
    └── relay/uplink protocol spec [this]
          └── FIND spec implementation plan
                └── exec model refactor
                      └── Present/Reachable refactor
                            └── 2026-06-22-binary-resolution-shell-directive.md
```

**This spec defines:** transaction ID format (`tx` entity code), transactions table schema, `orbiter relay` and `orbiter uplink` sub-command contracts, FIND function signature update (txn_id as first arg), session lifecycle, and the unified shell→Orbiter data interface.

**FIND spec implementation plan defines:** the global search implementations per shell, Captain prompt UX for multiple matches, and the lifecycle.go changes that emit FIND directives and call `orbiter uplink`.

**Exec model refactor defines:** how integrations invoke commands through the shell after FIND results are resolved into `ResolvedContext`.
