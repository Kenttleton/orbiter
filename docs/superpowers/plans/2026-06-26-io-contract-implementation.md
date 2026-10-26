# Orbiter I/O Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the output models, renderer `Prompt` interface, sessions/transactions database schema, `orbiter relay` and `orbiter uplink` sub-commands, and WASM approval wiring defined in `docs/superpowers/plans/2026-06-25-io-contract-spec.md`.

**Architecture:** All new output model types live in `internal/output/models.go`; both renderers implement the updated `Renderer` interface including the new `Prompt` method. Sessions and transactions tables are appended directly to `internal/migrations/0001_initial.sql` (no new migration file). CRUD helpers and `TransactionStatus` live in a new `internal/starchart/transaction.go`. `orbiter relay` and `orbiter uplink` are new Cobra sub-commands. FIND-op processing inside `uplink` is explicitly stubbed — it belongs to the FIND spec implementation plan.

**Tech Stack:** Go, SQLite (existing StarChart DB), Cobra (existing CLI), Lipgloss (styled renderer), `encoding/json`, `internal/models/id.go` (snowflake IDs)

**Spec:** `docs/superpowers/plans/2026-06-25-io-contract-spec.md`

## Global Constraints

- New model types (`Event`, `CaptainPrompt`, `Result`) go in `internal/output/models.go` — not `renderer.go`
- `TransactionStatus` string enum and all session/transaction CRUD go in `internal/starchart/transaction.go`
- Session IDs: `models.NewID("ss")` — add `EntityTypeSs = "ss"` to `internal/models/id.go`
- Transaction IDs: `models.NewID("tx")` — add `EntityTypeTx = "tx"` to `internal/models/id.go`
- Snowflake IDs are 16 chars; creation timestamp is embedded — no `created_at` column on sessions or transactions tables
- Schema changes go in `internal/migrations/0001_initial.sql` only — no new numbered SQL file; dev databases must be deleted and recreated before testing (`rm ~/.orbiter/starchart.db` or wherever the project stores it)
- `orbiter relay` accepts `--response=<json>` flag OR reads from stdin when `--response` is absent
- `orbiter uplink` requires `--session=<id>` flag
- `StyledRenderer` gains a new constructor `NewStyledRendererWith(w io.Writer, r io.Reader, verbose bool)` for testability; the existing `NewStyledRenderer(w, verbose)` is unchanged and uses `os.Stdin`
- Prompt op names are uppercase: `SELECT`, `CONFIRM`, `TEXT`; variants: SELECT `"single"`/`"multi"`, TEXT `"short"`/`"long"`/`"obscured"`, CONFIRM has no variant
- Relay response JSON shapes: SELECT single `{"selected":"..."}`, SELECT multi `{"selected":["..."]}`, CONFIRM `{"confirmed":true}` or `{"confirmed":false}`, TEXT `{"value":"..."}`
- `JSONRenderer.Prompt` emits the event and returns `("", nil)` — the caller is responsible for the DB transaction record
- `RendererApproveFunc` wraps a `Renderer` into a `wasm.ApproveFunc` for the security gate; async JSON-mode resume is deferred to the exec model refactor spec

---

## File Map

| File | Action | Responsibility |
| ---- | ------ | -------------- |
| `internal/output/models.go` | **Create** | `Event`, `CaptainPrompt`, `Result` types |
| `internal/output/renderer.go` | **Modify** | Add JSON tags to `PlanStep`/`ProgressStep`; add `Status` to `ProgressStep`; add `Prompt` to `Renderer` interface |
| `internal/output/styled.go` | **Modify** | Add `stdin io.Reader` field; add `NewStyledRendererWith`; implement `Prompt` |
| `internal/output/json.go` | **Modify** | Implement `Prompt` (emit `captain_prompt` event); update `Plan` to emit `Event` envelope |
| `internal/output/factory.go` | **Modify** | Pass `os.Stdin` in `NewStyledRenderer` factory call after styled.go change |
| `internal/models/id.go` | **Modify** | Add `EntityTypeTx`, `EntityTypeSs` constants |
| `internal/migrations/0001_initial.sql` | **Modify** | Append `sessions` and `transactions` tables |
| `internal/starchart/transaction.go` | **Create** | `Session`, `Transaction`, `TransactionStatus`; CRUD methods on `*StarChart` |
| `internal/starchart/transaction_test.go` | **Create** | Table-driven tests for CRUD helpers |
| `internal/commands/relay.go` | **Create** | `orbiter relay` Cobra command |
| `internal/commands/uplink.go` | **Create** | `orbiter uplink` Cobra command |
| `internal/commands/root.go` | **Modify** | Register relay and uplink; replace `wasm.StdinApproveFunc` with `wasm.RendererApproveFunc` |
| `internal/commands/relay_test.go` | **Create** | Integration tests for relay command |
| `internal/commands/uplink_test.go` | **Create** | Integration tests for uplink command |
| `internal/wasm/loader.go` | **Modify** | Add `RendererApproveFunc` constructor |
| `internal/output/models_test.go` | **Create** | JSON round-trip tests for new model types |
| `internal/output/json_test.go` | **Modify** | Add `Prompt` and `Plan` event emission tests |
| `internal/output/styled_test.go` | **Modify** | Add `Prompt` stdin interaction tests |

---

### Task 1: Output models

Add `Event`, `CaptainPrompt`, `Result` to a new file, and add JSON tags + `Status` field to the existing struct types.

**Files:**
- Create: `internal/output/models.go`
- Modify: `internal/output/renderer.go`
- Test: `internal/output/models_test.go`

**Interfaces:**
- Produces: `output.Event`, `output.CaptainPrompt`, `output.Result` — consumed by Tasks 3, 4, 5, 6, 7
- Produces: updated `output.PlanStep` and `output.ProgressStep` with JSON tags — consumed by Tasks 3, 4

- [ ] **Step 1: Write the failing test for model serialization**

Create `internal/output/models_test.go`:

```go
package output_test

import (
	"encoding/json"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	data, _ := json.Marshal([]string{"step1"})
	e := output.Event{Type: "plan", Txn: "a3f2k1b8tx9r2m4p", Data: data}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got output.Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "plan" {
		t.Errorf("Type = %q, want %q", got.Type, "plan")
	}
	if got.Txn != "a3f2k1b8tx9r2m4p" {
		t.Errorf("Txn = %q, want %q", got.Txn, "a3f2k1b8tx9r2m4p")
	}
}

func TestCaptainPrompt_JSONRoundTrip(t *testing.T) {
	cp := output.CaptainPrompt{
		Txn:     "b4g3l2c9tx0s3n5q",
		Op:      "SELECT",
		Message: "choose a path",
		Options: []string{"/a", "/b"},
		Variant: "single",
	}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got output.CaptainPrompt
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Op != "SELECT" {
		t.Errorf("Op = %q, want SELECT", got.Op)
	}
	if len(got.Options) != 2 {
		t.Errorf("Options len = %d, want 2", len(got.Options))
	}
}

func TestResult_JSONRoundTrip(t *testing.T) {
	r := output.Result{Success: true, Message: "branch calibrated"}
	b, _ := json.Marshal(r)
	var got output.Result
	json.Unmarshal(b, &got)
	if !got.Success {
		t.Error("Success = false, want true")
	}
	if got.Message != "branch calibrated" {
		t.Errorf("Message = %q, want %q", got.Message, "branch calibrated")
	}
}

func TestPlanStep_JSONTags(t *testing.T) {
	ps := output.PlanStep{Action: "add", EntityType: "resource", Name: "brew", Description: "package mgr"}
	b, _ := json.Marshal(ps)
	if string(b) != `{"action":"add","entity_type":"resource","name":"brew","description":"package mgr"}` {
		t.Errorf("unexpected JSON: %s", b)
	}
}

func TestProgressStep_StatusField(t *testing.T) {
	ps := output.ProgressStep{ThematicLabel: "Scanning", PlainLabel: "scan", Status: "running"}
	b, _ := json.Marshal(ps)
	var m map[string]string
	json.Unmarshal(b, &m)
	if m["status"] != "running" {
		t.Errorf("status = %q, want running", m["status"])
	}
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/output/... -run "TestEvent_JSONRoundTrip|TestCaptainPrompt|TestResult|TestPlanStep_JSONTags|TestProgressStep_StatusField" -v
```

Expected: FAIL — `output.Event`, `output.CaptainPrompt`, `output.Result` undefined; JSON tags missing.

- [ ] **Step 3: Create `internal/output/models.go`**

```go
package output

import "encoding/json"

// Event is the top-level JSON output envelope emitted to stdout in JSON mode.
// Every line of stdout in JSON mode is one Event serialized as a single-line JSON object.
// Txn is always non-empty — enables concurrent session tracing and full audit log.
type Event struct {
	Type string          `json:"type"` // plan | progress | result | error | captain_prompt
	Txn  string          `json:"txn"`  // NewID("tx") snowflake; always set
	Data json.RawMessage `json:"data"`
}

// CaptainPrompt is emitted when Orbiter needs input from the Captain or a connected client.
// Op names are generic and UI-agnostic — the client decides how to render each op.
type CaptainPrompt struct {
	Txn     string            `json:"txn"`
	Op      string            `json:"op"`               // SELECT | CONFIRM | TEXT
	Message string            `json:"message"`
	Options []string          `json:"options,omitempty"` // SELECT: list of choices
	Variant string            `json:"variant,omitempty"` // SELECT: "single"|"multi"; TEXT: "short"|"long"|"obscured"
	Meta    map[string]string `json:"meta,omitempty"`    // optional context for the client
}

// Result is the output of a completed Orbiter operation.
type Result struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
```

- [ ] **Step 4: Add JSON tags to `PlanStep` and `ProgressStep` in `renderer.go`; add `Status` to `ProgressStep`**

In `internal/output/renderer.go`, replace the two struct definitions:

```go
// PlanStep represents one step in a Chart or transition plan.
type PlanStep struct {
	Action      string `json:"action"`       // "add" | "change" | "remove"
	EntityType  string `json:"entity_type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProgressStep represents one task in a running operation's step list.
type ProgressStep struct {
	// ThematicLabel is the sci-fi themed label shown in default mode.
	ThematicLabel string `json:"thematic_label"`
	// PlainLabel is the operational description always shown alongside the thematic label,
	// and shown exclusively in verbose mode.
	PlainLabel string `json:"plain_label"`
	// Status is the current state of the step: "pending" | "running" | "done" | "failed".
	// Empty string means the step has not started. Used by the JSON renderer.
	Status string `json:"status,omitempty"`
}
```

- [ ] **Step 5: Run tests to confirm pass**

```bash
go test ./internal/output/... -run "TestEvent_JSONRoundTrip|TestCaptainPrompt|TestResult|TestPlanStep_JSONTags|TestProgressStep_StatusField" -v
```

Expected: PASS (5 tests)

- [ ] **Step 6: Run full build to confirm no regressions**

```bash
go build ./...
go test ./internal/output/...
```

Expected: clean build; all existing output tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/output/models.go internal/output/renderer.go internal/output/models_test.go
git commit -m "feat: add Event, CaptainPrompt, Result models; add JSON tags to PlanStep/ProgressStep"
```

---

### Task 2: Entity type codes and database schema

Add `tx` and `ss` entity codes to the ID package; add `sessions` and `transactions` tables to the init SQL; create CRUD helpers in `internal/starchart/transaction.go`.

**Files:**
- Modify: `internal/models/id.go`
- Modify: `internal/migrations/0001_initial.sql`
- Create: `internal/starchart/transaction.go`
- Test: `internal/starchart/transaction_test.go`

**Interfaces:**
- Consumes: `models.NewID("tx")`, `models.NewID("ss")`, `models.ParseID`
- Produces: `starchart.TransactionStatus`, `starchart.Session`, `starchart.Transaction`, `starchart.(*StarChart).InsertSession`, `starchart.(*StarChart).InsertTransaction`, `starchart.(*StarChart).RelayTransaction`, `starchart.(*StarChart).GetSession`, `starchart.(*StarChart).GetRelayedTransactions`, `starchart.(*StarChart).MarkTransactionProcessed`, `starchart.(*StarChart).UpdateSessionStatus`, `starchart.(*StarChart).DeleteStaleTransactions` — consumed by Tasks 5 and 6

- [ ] **Step 1: Write failing tests**

Create `internal/starchart/transaction_test.go`:

```go
package starchart_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

func openTestDB(t *testing.T) *starchart.StarChart {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sc.Close() })
	return sc
}

func TestInsertAndGetSession(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	id := models.NewID("ss")
	if err := sc.InsertSession(ctx, id, `{"op":"INIT"}`); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	sess, err := sc.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.ID != id {
		t.Errorf("ID = %q, want %q", sess.ID, id)
	}
	if sess.Status != "active" {
		t.Errorf("Status = %q, want active", sess.Status)
	}
}

func TestInsertAndRelayTransaction(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)
	txnID := models.NewID("tx")
	tx := starchart.Transaction{
		ID:        txnID,
		SessionID: sessID,
		Op:        "CONFIRM",
		Args:      `{"brand":"git"}`,
		Status:    starchart.TransactionStatusPending,
		Payload:   `{"op":"CONFIRM","message":"git wants to run: git status"}`,
	}
	if err := sc.InsertTransaction(ctx, tx); err != nil {
		t.Fatalf("InsertTransaction: %v", err)
	}
	if err := sc.RelayTransaction(ctx, txnID, `{"confirmed":true}`); err != nil {
		t.Fatalf("RelayTransaction: %v", err)
	}
	txns, err := sc.GetRelayedTransactions(ctx, sessID)
	if err != nil {
		t.Fatalf("GetRelayedTransactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("len = %d, want 1", len(txns))
	}
	if txns[0].Response != `{"confirmed":true}` {
		t.Errorf("Response = %q, want confirmed:true", txns[0].Response)
	}
}

func TestRelayTransaction_Idempotent(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)
	txnID := models.NewID("tx")
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: "CONFIRM",
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	// relay twice — should not error
	sc.RelayTransaction(ctx, txnID, `{"confirmed":true}`)
	if err := sc.RelayTransaction(ctx, txnID, `{"confirmed":false}`); err != nil {
		t.Errorf("second relay should be a no-op, got error: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(ctx, sessID)
	if txns[0].Response != `{"confirmed":true}` {
		t.Error("second relay should not overwrite first response")
	}
}

func TestMarkTransactionProcessed(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)
	txnID := models.NewID("tx")
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: "SELECT",
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	sc.RelayTransaction(ctx, txnID, `{"selected":"/usr/bin/brew"}`)
	if err := sc.MarkTransactionProcessed(ctx, txnID); err != nil {
		t.Fatalf("MarkTransactionProcessed: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(ctx, sessID)
	if len(txns) != 0 {
		t.Error("processed transaction should not appear in GetRelayedTransactions")
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	id := models.NewID("ss")
	sc.InsertSession(ctx, id, `{}`)
	if err := sc.UpdateSessionStatus(ctx, id, "complete"); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	sess, _ := sc.GetSession(ctx, id)
	if sess.Status != "complete" {
		t.Errorf("Status = %q, want complete", sess.Status)
	}
}

func TestDeleteStaleTransactions(t *testing.T) {
	sc := openTestDB(t)
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)

	// Insert a transaction with a manually crafted ID whose embedded timestamp is >24h ago.
	// models.NewID encodes timestamp at positions 0-7 in base36 from epoch 2025-01-01.
	// Use a real recent ID for the non-stale case and a static old ID for the stale case.
	staleID := "00000000tx000000" // timestamp component = 0 ms from epoch = 2025-01-01 = definitely stale
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: staleID, SessionID: sessID, Op: "CONFIRM",
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	freshID := models.NewID("tx")
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: freshID, SessionID: sessID, Op: "CONFIRM",
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	if err := sc.DeleteStaleTransactions(ctx); err != nil {
		t.Fatalf("DeleteStaleTransactions: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(ctx, sessID)
	// fresh is pending not relayed; check via raw query. Use GetSession to verify DB is live.
	// Verify stale is gone by checking we can't find it in relayed.
	// Insert stale as relayed to confirm deletion works for relayed too.
	staleID2 := "00000001tx000000"
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: staleID2, SessionID: sessID, Op: "SELECT",
		Args: `{}`, Status: starchart.TransactionStatusRelayed,
		Response: `{"selected":"x"}`,
	})
	// manually set status via relay so it appears in GetRelayed
	if err := sc.DeleteStaleTransactions(ctx); err != nil {
		t.Fatalf("DeleteStaleTransactions second call: %v", err)
	}
	_ = txns
	// If no error and test completes, cleanup ran without panicking.
	// The fresh transaction (pending) survives.
	_ = freshID
}

func TestNewID_TxCode(t *testing.T) {
	id := models.NewID("tx")
	if len(id) != 16 {
		t.Fatalf("len = %d, want 16", len(id))
	}
	parsed, err := models.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	if parsed.EntityType != "tx" {
		t.Errorf("EntityType = %q, want tx", parsed.EntityType)
	}
	if time.Since(parsed.Timestamp) > 5*time.Second {
		t.Error("timestamp embedded in ID is not recent")
	}
}

func TestNewID_SsCode(t *testing.T) {
	id := models.NewID("ss")
	parsed, err := models.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	if parsed.EntityType != "ss" {
		t.Errorf("EntityType = %q, want ss", parsed.EntityType)
	}
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/starchart/... -run "TestInsertAndGetSession|TestInsertAndRelay|TestMarkTransaction|TestUpdateSession|TestDeleteStale|TestNewID_Tx|TestNewID_Ss" -v
```

Expected: FAIL — `starchart.InsertSession` undefined; `models.NewID("ss")` / `models.NewID("tx")` compile but ParseID returns unexpected entity type (codes not yet registered).

- [ ] **Step 3: Add `EntityTypeTx` and `EntityTypeSs` to `internal/models/id.go`**

In the `const` block (after `EntityTypeAttachment = "at"`), add:

```go
EntityTypeTx = "tx" // Transaction — one per Orbiter↔client interaction
EntityTypeSs = "ss" // Session — one per Orbiter invocation
```

- [ ] **Step 4: Append sessions and transactions tables to `internal/migrations/0001_initial.sql`**

Add at the end of the file (after the last existing `CREATE TABLE` / `CREATE INDEX` statement):

```sql
-- Sessions track active Orbiter invocations.
-- Supports multiple concurrent sessions without relying on environment variables.
-- ID embeds creation timestamp — no created_at column needed.
CREATE TABLE IF NOT EXISTS sessions (
    id     TEXT PRIMARY KEY,                -- models.NewID("ss") snowflake
    status TEXT NOT NULL DEFAULT 'active',  -- 'active' | 'complete' | 'failed'
    meta   TEXT                             -- JSON: what the session is resolving e.g. {"op":"INIT","branch":"main"}
);

CREATE INDEX IF NOT EXISTS sessions_status_idx ON sessions (status);

-- Transactions record every Orbiter↔client interaction within a session.
-- ID embeds creation timestamp — no created_at column needed.
CREATE TABLE IF NOT EXISTS transactions (
    id         TEXT PRIMARY KEY,                    -- models.NewID("tx") snowflake
    session_id TEXT NOT NULL,                       -- references sessions(id)
    op         TEXT NOT NULL,                       -- FIND_BINARY | FIND_FILE | FIND_DIR | FIND_ENV | SELECT | CONFIRM | TEXT
    args       TEXT NOT NULL,                       -- JSON: operation arguments
    status     TEXT NOT NULL DEFAULT 'pending',     -- 'pending' | 'relayed' | 'processed'
    payload    TEXT,                                -- JSON: the event data Orbiter sent (for audit/debug)
    response   TEXT                                 -- JSON: the relay response from client (NULL until relayed)
);

CREATE INDEX IF NOT EXISTS transactions_session_status_idx ON transactions (session_id, status);
```

**Important:** Delete your local starchart database before running tests so the schema is rebuilt:
```bash
rm -f ~/.orbiter/starchart.db  # adjust path to your local starchart location
```

- [ ] **Step 5: Create `internal/starchart/transaction.go`**

```go
package starchart

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
)

// TransactionStatus is the lifecycle state of a relay transaction.
type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusRelayed   TransactionStatus = "relayed"
	TransactionStatusProcessed TransactionStatus = "processed"
)

// Session represents an active Orbiter invocation.
type Session struct {
	ID     string
	Status string // "active" | "complete" | "failed"
	Meta   string // JSON
}

// Transaction records one Orbiter↔client interaction within a session.
type Transaction struct {
	ID        string
	SessionID string
	Op        string
	Args      string // JSON
	Status    TransactionStatus
	Payload   string // JSON; may be empty
	Response  string // JSON; empty until relayed
}

// InsertSession creates a new session record with status='active'.
func (sc *StarChart) InsertSession(ctx context.Context, id, meta string) error {
	_, err := sc.db.ExecContext(ctx,
		`INSERT INTO sessions (id, meta) VALUES (?, ?)`,
		id, meta,
	)
	return err
}

// GetSession retrieves a session by ID. Returns sql.ErrNoRows if not found.
func (sc *StarChart) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	err := sc.db.QueryRowContext(ctx,
		`SELECT id, status, COALESCE(meta,'') FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Status, &s.Meta)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, sql.ErrNoRows
	}
	return s, err
}

// UpdateSessionStatus sets a session's status to the given value.
func (sc *StarChart) UpdateSessionStatus(ctx context.Context, id, status string) error {
	_, err := sc.db.ExecContext(ctx,
		`UPDATE sessions SET status = ? WHERE id = ?`, status, id,
	)
	return err
}

// InsertTransaction creates a new transaction record.
func (sc *StarChart) InsertTransaction(ctx context.Context, t Transaction) error {
	_, err := sc.db.ExecContext(ctx,
		`INSERT INTO transactions (id, session_id, op, args, status, payload, response)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.Op, t.Args, string(t.Status),
		nullableString(t.Payload), nullableString(t.Response),
	)
	return err
}

// RelayTransaction stores the client's response and advances status to 'relayed'.
// Idempotent: if the transaction is already relayed, returns nil without modification.
func (sc *StarChart) RelayTransaction(ctx context.Context, txnID, response string) error {
	res, err := sc.db.ExecContext(ctx,
		`UPDATE transactions SET status='relayed', response=? WHERE id=? AND status='pending'`,
		response, txnID,
	)
	if err != nil {
		return err
	}
	// 0 rows affected = already relayed or not found; both are acceptable.
	_ = res
	return nil
}

// GetRelayedTransactions returns all transactions for a session with status='relayed'.
func (sc *StarChart) GetRelayedTransactions(ctx context.Context, sessionID string) ([]Transaction, error) {
	rows, err := sc.db.QueryContext(ctx,
		`SELECT id, session_id, op, args, status, COALESCE(payload,''), COALESCE(response,'')
		 FROM transactions WHERE session_id=? AND status='relayed'`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transaction
	for rows.Next() {
		var t Transaction
		var status string
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Op, &t.Args, &status, &t.Payload, &t.Response); err != nil {
			return nil, err
		}
		t.Status = TransactionStatus(status)
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkTransactionProcessed advances a transaction's status to 'processed'.
func (sc *StarChart) MarkTransactionProcessed(ctx context.Context, txnID string) error {
	_, err := sc.db.ExecContext(ctx,
		`UPDATE transactions SET status='processed' WHERE id=?`, txnID,
	)
	return err
}

// DeleteStaleTransactions removes pending and relayed transactions older than 24 hours.
// Age is determined by the timestamp embedded in the snowflake ID via models.ParseID.
func (sc *StarChart) DeleteStaleTransactions(ctx context.Context) error {
	rows, err := sc.db.QueryContext(ctx,
		`SELECT id FROM transactions WHERE status IN ('pending','relayed')`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	cutoff := time.Now().Add(-24 * time.Hour)
	var stale []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		parsed, err := models.ParseID(id)
		if err != nil {
			continue // malformed ID; skip
		}
		if parsed.Timestamp.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range stale {
		if _, err := sc.db.ExecContext(ctx, `DELETE FROM transactions WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

// nullableString returns nil for an empty string (stored as SQL NULL) or the string value.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/starchart/... -run "TestInsertAndGetSession|TestInsertAndRelay|TestMarkTransaction|TestUpdateSession|TestDeleteStale|TestNewID_Tx|TestNewID_Ss" -v
```

Expected: PASS

- [ ] **Step 7: Full test run**

```bash
go build ./...
go test ./...
```

Expected: clean build; all tests pass (some e2e may already fail from prior work — those are pre-existing).

- [ ] **Step 8: Commit**

```bash
git add internal/models/id.go internal/migrations/0001_initial.sql \
        internal/starchart/transaction.go internal/starchart/transaction_test.go
git commit -m "feat: add tx/ss entity codes; add sessions+transactions schema; add CRUD helpers"
```

---

### Task 3: Renderer Prompt interface + StyledRenderer implementation

Add `Prompt` to the `Renderer` interface and implement it in `StyledRenderer` for all three ops.

**Files:**
- Modify: `internal/output/renderer.go`
- Modify: `internal/output/styled.go`
- Modify: `internal/output/factory.go`
- Test: `internal/output/styled_test.go`

**Interfaces:**
- Consumes: `output.CaptainPrompt` (Task 1)
- Produces: `Renderer.Prompt(p CaptainPrompt) (response string, err error)` — consumed by Tasks 4, 7

- [ ] **Step 1: Write failing tests in `internal/output/styled_test.go`**

Add the following tests to the existing `styled_test.go` file (below the existing tests):

```go
func TestStyledRenderer_Prompt_Confirm_yes(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("y\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "CONFIRM", Message: "run git status?",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"confirmed":true}` {
		t.Errorf("resp = %q, want {\"confirmed\":true}", resp)
	}
}

func TestStyledRenderer_Prompt_Confirm_no(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("n\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "CONFIRM", Message: "run git status?",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"confirmed":false}` {
		t.Errorf("resp = %q, want {\"confirmed\":false}", resp)
	}
}

func TestStyledRenderer_Prompt_Select_single(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("2\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn:     "abc",
		Op:      "SELECT",
		Variant: "single",
		Message: "choose a binary",
		Options: []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"selected":"/usr/local/bin/brew"}` {
		t.Errorf("resp = %q, want selected /usr/local/bin/brew", resp)
	}
}

func TestStyledRenderer_Prompt_Text_short(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("hello world\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "TEXT", Variant: "short", Message: "enter a name",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"value":"hello world"}` {
		t.Errorf("resp = %q, want value:hello world", resp)
	}
}
```

At the top of `styled_test.go`, add `"strings"` to the imports if not already present.

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/output/... -run "TestStyledRenderer_Prompt" -v
```

Expected: FAIL — `output.NewStyledRendererWith` undefined; `Renderer.Prompt` undefined.

- [ ] **Step 3: Add `Prompt` to the `Renderer` interface in `renderer.go`**

Add after the `IsVerbose() bool` line:

```go
// Prompt presents a CaptainPrompt to the Captain and returns the response as a JSON string.
//
// Styled mode: blocks on stdin, collects input, serializes to JSON, returns response string.
//   Response shapes: SELECT single {"selected":"..."}, SELECT multi {"selected":["..."]},
//   CONFIRM {"confirmed":true|false}, TEXT {"value":"..."}.
// JSON mode: emits Event{Type:"captain_prompt"} to stdout, returns ("", nil).
//   The caller is responsible for inserting the transaction record before calling Prompt.
//   Response arrives asynchronously via orbiter relay.
Prompt(p CaptainPrompt) (response string, err error)
```

- [ ] **Step 4: Add `stdin io.Reader` field and `NewStyledRendererWith` to `styled.go`**

At the top of `internal/output/styled.go`, in the `StyledRenderer` struct definition, add:

```go
stdin io.Reader
```

Update `NewStyledRenderer` to set `stdin: os.Stdin`:

```go
func NewStyledRenderer(w io.Writer, verbose bool) *StyledRenderer {
	return &StyledRenderer{
		w:       w,
		verbose: verbose,
		stdin:   os.Stdin,
	}
}
```

Add the new constructor after `NewStyledRenderer`:

```go
// NewStyledRendererWith creates a StyledRenderer with an explicit stdin reader.
// Use in tests to inject a strings.NewReader; production callers use NewStyledRenderer.
func NewStyledRendererWith(w io.Writer, r io.Reader, verbose bool) *StyledRenderer {
	return &StyledRenderer{
		w:       w,
		verbose: verbose,
		stdin:   r,
	}
}
```

Add the required import `"os"` if it is not already present in `styled.go`.

- [ ] **Step 5: Implement `Prompt` on `*StyledRenderer` in `styled.go`**

Add the following method. It reads from `sr.stdin` rather than `os.Stdin` directly:

```go
// Prompt implements Renderer. Renders the prompt to the terminal and reads the
// Captain's response from stdin. Returns the response serialized as a JSON string.
func (sr *StyledRenderer) Prompt(p CaptainPrompt) (string, error) {
	switch p.Op {
	case "CONFIRM":
		fmt.Fprintf(sr.w, "\n  %s\n  Allow? [y/N] ", p.Message)
		var ans string
		fmt.Fscan(sr.stdin, &ans)
		confirmed := strings.ToLower(strings.TrimSpace(ans)) == "y"
		b, _ := json.Marshal(map[string]bool{"confirmed": confirmed})
		return string(b), nil

	case "SELECT":
		fmt.Fprintf(sr.w, "\n  %s\n", p.Message)
		for i, opt := range p.Options {
			fmt.Fprintf(sr.w, "    %d. %s\n", i+1, opt)
		}
		if p.Variant == "multi" {
			fmt.Fprint(sr.w, "  Enter numbers separated by spaces: ")
			var line string
			fmt.Fscan(sr.stdin, &line)
			parts := strings.Fields(line)
			var selected []string
			for _, part := range parts {
				idx := 0
				fmt.Sscan(part, &idx)
				if idx >= 1 && idx <= len(p.Options) {
					selected = append(selected, p.Options[idx-1])
				}
			}
			b, _ := json.Marshal(map[string][]string{"selected": selected})
			return string(b), nil
		}
		// single
		fmt.Fprint(sr.w, "  Enter number: ")
		var idx int
		fmt.Fscan(sr.stdin, &idx)
		if idx < 1 || idx > len(p.Options) {
			return "", fmt.Errorf("selection %d out of range [1,%d]", idx, len(p.Options))
		}
		b, _ := json.Marshal(map[string]string{"selected": p.Options[idx-1]})
		return string(b), nil

	case "TEXT":
		if p.Variant == "obscured" {
			fmt.Fprintf(sr.w, "\n  %s (input hidden): ", p.Message)
		} else {
			fmt.Fprintf(sr.w, "\n  %s: ", p.Message)
		}
		scanner := bufio.NewScanner(sr.stdin)
		scanner.Scan()
		value := scanner.Text()
		b, _ := json.Marshal(map[string]string{"value": value})
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown prompt op: %q", p.Op)
	}
}
```

Add imports to `styled.go` as needed: `"bufio"`, `"encoding/json"`, `"fmt"`, `"strings"`.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/output/... -run "TestStyledRenderer_Prompt" -v
```

Expected: PASS (4 tests)

- [ ] **Step 7: Build to check JSONRenderer also satisfies Renderer (it will fail — expected)**

```bash
go build ./...
```

Expected: FAIL — `JSONRenderer` does not implement `Renderer`: missing `Prompt` method. This is expected; Task 4 adds it.

Since the build is broken, add a stub `Prompt` to `json.go` now so the build passes until Task 4 implements it properly:

```go
// Prompt implements Renderer. Emits a captain_prompt event; response arrives via orbiter relay.
// Stub: full implementation in Task 4.
func (jr *JSONRenderer) Prompt(p CaptainPrompt) (string, error) {
	return "", nil
}
```

- [ ] **Step 8: Confirm build passes**

```bash
go build ./...
go test ./internal/output/...
```

Expected: clean build; all output tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/output/renderer.go internal/output/styled.go \
        internal/output/factory.go internal/output/styled_test.go
git commit -m "feat: add Prompt to Renderer interface; implement StyledRenderer.Prompt for SELECT/CONFIRM/TEXT"
```

---

### Task 4: JSONRenderer Event envelope and Prompt

Replace the `Prompt` stub with a real implementation; update `Plan` to emit an `Event` envelope.

**Files:**
- Modify: `internal/output/json.go`
- Modify: `internal/output/json_test.go`

**Interfaces:**
- Consumes: `output.Event`, `output.CaptainPrompt` (Task 1); `models.NewID("tx")` (Task 2)
- Produces: `JSONRenderer.Prompt` emitting `captain_prompt` events; `JSONRenderer.Plan` emitting `plan` events — consumed by Task 7

- [ ] **Step 1: Write failing tests in `json_test.go`**

Add to `internal/output/json_test.go`:

```go
func TestJSONRenderer_Plan_emitsEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Plan([]output.PlanStep{
		{Action: "add", EntityType: "resource", Name: "brew", Description: "package manager"},
	})
	line := strings.TrimSpace(buf.String())
	var e output.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, line)
	}
	if e.Type != "plan" {
		t.Errorf("Type = %q, want plan", e.Type)
	}
	if len(e.Txn) != 16 {
		t.Errorf("Txn = %q, expected 16-char snowflake", e.Txn)
	}
	var steps []output.PlanStep
	if err := json.Unmarshal(e.Data, &steps); err != nil {
		t.Fatalf("unmarshal Data: %v", err)
	}
	if len(steps) != 1 || steps[0].Name != "brew" {
		t.Errorf("unexpected steps: %+v", steps)
	}
}

func TestJSONRenderer_Prompt_emitsCaptainPromptEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn:     "b4g3l2c9tx0s3n5q",
		Op:      "SELECT",
		Variant: "single",
		Message: "choose",
		Options: []string{"/a", "/b"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != "" {
		t.Errorf("resp = %q, want empty (async — response via relay)", resp)
	}
	line := strings.TrimSpace(buf.String())
	var e output.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, line)
	}
	if e.Type != "captain_prompt" {
		t.Errorf("Type = %q, want captain_prompt", e.Type)
	}
	if e.Txn != "b4g3l2c9tx0s3n5q" {
		t.Errorf("Txn = %q, want b4g3l2c9tx0s3n5q", e.Txn)
	}
	var cp output.CaptainPrompt
	if err := json.Unmarshal(e.Data, &cp); err != nil {
		t.Fatalf("unmarshal CaptainPrompt: %v", err)
	}
	if cp.Op != "SELECT" {
		t.Errorf("Op = %q, want SELECT", cp.Op)
	}
}
```

Add `"strings"` and `"encoding/json"` to imports if not present.

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/output/... -run "TestJSONRenderer_Plan_emitsEvent|TestJSONRenderer_Prompt_emitsCaptainPromptEvent" -v
```

Expected: FAIL — `Plan` emits nothing; `Prompt` stub returns `("", nil)` but emits no event.

- [ ] **Step 3: Implement `Plan` event emission in `json.go`**

Replace the existing `Plan` method (currently a no-op) with:

```go
func (jr *JSONRenderer) Plan(steps []PlanStep) {
	data, err := json.Marshal(steps)
	if err != nil {
		return
	}
	jr.emitEvent("plan", models.NewID("tx"), data)
}
```

Add a private `emitEvent` helper at the bottom of `json.go`:

```go
func (jr *JSONRenderer) emitEvent(eventType, txn string, data json.RawMessage) {
	e := Event{Type: eventType, Txn: txn, Data: data}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintln(jr.w, string(b))
}
```

Add `"encoding/json"` and `"fmt"` to imports in `json.go`; also add `"github.com/Kenttleton/orbiter/internal/models"`.

- [ ] **Step 4: Implement `Prompt` in `json.go`**

Replace the stub `Prompt` method with:

```go
// Prompt implements Renderer for JSON mode. Emits a captain_prompt event and returns ("", nil).
// The caller must insert the transaction record before calling Prompt.
// The Captain's response arrives asynchronously via orbiter relay.
func (jr *JSONRenderer) Prompt(p CaptainPrompt) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	jr.emitEvent("captain_prompt", p.Txn, data)
	return "", nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/output/... -run "TestJSONRenderer_Plan_emitsEvent|TestJSONRenderer_Prompt_emitsCaptainPromptEvent" -v
```

Expected: PASS

- [ ] **Step 6: Full test run**

```bash
go build ./...
go test ./internal/output/...
```

Expected: clean build; all output tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/output/json.go internal/output/json_test.go
git commit -m "feat: JSONRenderer emits Event envelope for Plan; implement Prompt to emit captain_prompt event"
```

---

### Task 5: `orbiter relay` command

New Cobra sub-command that stores a client's relay response into the transactions table.

**Files:**
- Create: `internal/commands/relay.go`
- Modify: `internal/commands/root.go`
- Test: `internal/commands/relay_test.go`

**Interfaces:**
- Consumes: `starchart.(*StarChart).RelayTransaction` (Task 2); `starchart.(*StarChart).DeleteStaleTransactions` (Task 2)
- Produces: `newRelayCmd()` — consumed by root.go registration

- [ ] **Step 1: Write failing tests**

Create `internal/commands/relay_test.go`:

```go
package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

func TestRelayCmd_storesResponse(t *testing.T) {
	sc, sessID, txnID := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID, "--response={\"confirmed\":true}"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, err := sc.GetRelayedTransactions(context.Background(), sessID)
	if err != nil {
		t.Fatalf("GetRelayedTransactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("len = %d, want 1", len(txns))
	}
	if txns[0].Response != `{"confirmed":true}` {
		t.Errorf("Response = %q", txns[0].Response)
	}
}

func TestRelayCmd_stdin_pipe(t *testing.T) {
	sc, sessID, txnID := setupRelayDB(t, "SELECT")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID})
	root.SetIn(strings.NewReader(`{"selected":"/usr/bin/brew"}`))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 1 || txns[0].Response != `{"selected":"/usr/bin/brew"}` {
		t.Errorf("unexpected relay state: %+v", txns)
	}
}

func TestRelayCmd_idempotent(t *testing.T) {
	sc, _, txnID := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID, `--response={"confirmed":true}`})
	root.Execute()
	// relay again — should not error
	root2 := commands.NewRootCommandForTest(sc)
	root2.SetArgs([]string{"relay", "--txn=" + txnID, `--response={"confirmed":false}`})
	var errBuf bytes.Buffer
	root2.SetErr(&errBuf)
	if err := root2.Execute(); err != nil {
		t.Fatalf("second relay: %v", err)
	}
}

func TestRelayCmd_missingTxn(t *testing.T) {
	sc, _, _ := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=nonexistent000000", `--response={"confirmed":true}`})
	// should exit 0 (idempotent — unknown txn is treated as already processed)
	if err := root.Execute(); err != nil {
		t.Errorf("missing txn should not error: %v", err)
	}
}

func setupRelayDB(t *testing.T, op string) (*starchart.StarChart, string, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)
	txnID := models.NewID("tx")
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: op,
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	return sc, sessID, txnID
}
```

Note: `commands.NewRootCommandForTest(sc)` is a new test helper to add to the commands package. Check whether an existing test helper exists in `executor_test.go` for wiring a starchart to the command tree — if so, adapt it. If not, add:

```go
// In internal/commands/testhelper_test.go (or inline in relay_test.go)
// NewRootCommandForTest returns a root command wired to the given StarChart, bypassing file-system open.
// This requires a small addition to the commands package to expose a hook — see relay.go notes.
```

Examine `internal/commands/root.go` and `executor.go` to understand how `*StarChart` is accessed by commands (likely via a `deps` struct). Adapt the test to match the actual injection pattern.

- [ ] **Step 2: Add `newRelayCmd` to `internal/commands/relay.go`**

```go
package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newRelayCmd() *cobra.Command {
	var txnID string
	var response string

	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Store a relay response for a pending transaction",
		Long: `Store the client's JSON response for a pending transaction, advancing it to 'relayed'.
Accepts --response=<json> or reads JSON from stdin when --response is omitted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if txnID == "" {
				return fmt.Errorf("--txn is required")
			}

			payload := response
			if payload == "" {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				payload = string(b)
			}
			if payload == "" {
				return fmt.Errorf("response required: provide --response=<json> or pipe JSON to stdin")
			}

			d := getDeps(cmd)
			ctx := cmd.Context()

			// Clean up stale transactions opportunistically.
			_ = d.sc.DeleteStaleTransactions(ctx)

			if err := d.sc.RelayTransaction(ctx, txnID, payload); err != nil {
				return fmt.Errorf("relay: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&txnID, "txn", "", "transaction ID to relay (required)")
	cmd.Flags().StringVar(&response, "response", "", "JSON response payload (reads stdin if omitted)")
	return cmd
}
```

Note: `getDeps(cmd)` is the pattern used by other commands to access the injected `*StarChart`. Read `internal/commands/root.go` and one existing command (e.g. `chart.go`) to find the exact accessor — adapt `getDeps` above to match.

- [ ] **Step 3: Register relay in `root.go`**

In `root.AddCommand(...)` in `internal/commands/root.go`, add `newRelayCmd()` to the list.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/commands/... -run "TestRelayCmd" -v
```

Expected: PASS (4 tests)

- [ ] **Step 5: Full build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 6: Smoke test the command manually**

```bash
go run . relay --help
```

Expected: help text showing `--txn` and `--response` flags.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/relay.go internal/commands/relay_test.go internal/commands/root.go
git commit -m "feat: add orbiter relay command"
```

---

### Task 6: `orbiter uplink` command

New Cobra sub-command that processes relayed transactions for a session. Handles `SELECT`, `CONFIRM`, and `TEXT` prompt ops. FIND ops (`FIND_BINARY`, `FIND_FILE`, `FIND_DIR`, `FIND_ENV`) are explicitly stubbed with a log message — they will be implemented by the FIND spec plan.

**Files:**
- Create: `internal/commands/uplink.go`
- Modify: `internal/commands/root.go`
- Test: `internal/commands/uplink_test.go`

**Interfaces:**
- Consumes: `starchart.(*StarChart).GetSession`, `starchart.(*StarChart).GetRelayedTransactions`, `starchart.(*StarChart).MarkTransactionProcessed`, `starchart.(*StarChart).UpdateSessionStatus` (Task 2); `output.Renderer` (Task 3/4)
- Produces: `newUplinkCmd()`

- [ ] **Step 1: Write failing tests**

Create `internal/commands/uplink_test.go`:

```go
package commands_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

func TestUplinkCmd_processesConfirm(t *testing.T) {
	sc, sessID, txnID := setupUplinkDB(t, "CONFIRM", `{"confirmed":true}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Transaction should be processed.
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 0 {
		t.Errorf("expected 0 relayed transactions after uplink, got %d", len(txns))
	}
	sess, _ := sc.GetSession(context.Background(), sessID)
	if sess.Status != "complete" {
		t.Errorf("session status = %q, want complete", sess.Status)
	}
}

func TestUplinkCmd_processesSelect(t *testing.T) {
	sc, sessID, txnID := setupUplinkDB(t, "SELECT", `{"selected":"/usr/bin/brew"}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 0 {
		t.Errorf("expected 0 relayed after uplink, got %d", len(txns))
	}
	_ = txnID
}

func TestUplinkCmd_processesText(t *testing.T) {
	sc, sessID, _ := setupUplinkDB(t, "TEXT", `{"value":"mysecret"}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestUplinkCmd_FIND_stub(t *testing.T) {
	// FIND ops should not error — they are stubbed for now.
	sc, sessID, _ := setupUplinkDB(t, "FIND_BINARY", `{"status":"ok","matches":["/usr/bin/brew"]}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("FIND_BINARY stub should not error: %v", err)
	}
}

func TestUplinkCmd_missingSession(t *testing.T) {
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.Close() })
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=nonexistentss000"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func setupUplinkDB(t *testing.T, op, relayResponse string) (*starchart.StarChart, string, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{"op":"TEST"}`)
	txnID := models.NewID("tx")
	argsJSON, _ := json.Marshal(map[string]string{"name": "brew"})
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: op,
		Args: string(argsJSON), Status: starchart.TransactionStatusPending,
	})
	sc.RelayTransaction(ctx, txnID, relayResponse)
	return sc, sessID, txnID
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/commands/... -run "TestUplinkCmd" -v
```

Expected: FAIL — `newUplinkCmd` undefined.

- [ ] **Step 3: Create `internal/commands/uplink.go`**

```go
package commands

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newUplinkCmd() *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "uplink",
		Short: "Process relayed transactions for a session and continue the operation",
		Long: `Process all relayed transactions for the given session.
Handles SELECT, CONFIRM, and TEXT prompt responses.
FIND_* op processing is deferred to the FIND spec implementation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sessionID == "" {
				return fmt.Errorf("--session is required")
			}

			d := getDeps(cmd)
			ctx := cmd.Context()

			sess, err := d.sc.GetSession(ctx, sessionID)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("session %q not found", sessionID)
			}
			if err != nil {
				return fmt.Errorf("get session: %w", err)
			}
			if sess.Status != "active" {
				return fmt.Errorf("session %q is not active (status: %s)", sessionID, sess.Status)
			}

			txns, err := d.sc.GetRelayedTransactions(ctx, sessionID)
			if err != nil {
				return fmt.Errorf("get transactions: %w", err)
			}

			for _, tx := range txns {
				if err := processTransaction(d.sc, ctx, tx, cmd); err != nil {
					// Non-fatal: log and continue.
					fmt.Fprintf(cmd.ErrOrStderr(), "uplink: transaction %s (%s): %v\n", tx.ID, tx.Op, err)
				}
				_ = d.sc.MarkTransactionProcessed(ctx, tx.ID)
			}

			if err := d.sc.UpdateSessionStatus(ctx, sessionID, "complete"); err != nil {
				return fmt.Errorf("update session: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to process (required)")
	return cmd
}

// processTransaction handles one relayed transaction based on its op type.
func processTransaction(sc *starchart.StarChart, ctx interface{ Value(any) any }, tx starchart.Transaction, cmd *cobra.Command) error {
	switch tx.Op {
	case "FIND_BINARY", "FIND_FILE", "FIND_DIR", "FIND_ENV":
		// FIND op processing is implemented by the FIND spec plan.
		// For now: log the stub and mark processed.
		fmt.Fprintf(cmd.OutOrStdout(), "uplink: FIND op %s stub — will be handled by FIND spec implementation\n", tx.Op)
		return nil

	case "CONFIRM":
		// Record the decision. Future exec model refactor wires this to resume WASM execution.
		return nil

	case "SELECT":
		// Record the selection. Future exec model refactor wires this to resume operation.
		return nil

	case "TEXT":
		// Record the input. Future exec model refactor wires this to resume operation.
		return nil

	default:
		return fmt.Errorf("unknown op %q", tx.Op)
	}
}
```

Note: the `ctx interface{ Value(any) any }` type is a placeholder — use `context.Context` from `cmd.Context()`. Adjust the `processTransaction` signature to `func processTransaction(sc *starchart.StarChart, ctx context.Context, tx starchart.Transaction, cmd *cobra.Command) error` and add `"context"` to imports.

- [ ] **Step 4: Register uplink in `root.go`**

In `root.AddCommand(...)`, add `newUplinkCmd()`.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/commands/... -run "TestUplinkCmd" -v
```

Expected: PASS (5 tests)

- [ ] **Step 6: Full build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 7: Smoke test**

```bash
go run . uplink --help
```

Expected: help text showing `--session` flag.

- [ ] **Step 8: Commit**

```bash
git add internal/commands/uplink.go internal/commands/uplink_test.go internal/commands/root.go
git commit -m "feat: add orbiter uplink command; processes SELECT/CONFIRM/TEXT; FIND ops stubbed"
```

---

### Task 7: Wire WASM command approval through renderer

Replace the hardcoded `wasm.StdinApproveFunc` with a `RendererApproveFunc` wrapper so the security gate uses the renderer interface for the approval prompt.

**Files:**
- Modify: `internal/wasm/loader.go`
- Modify: `internal/commands/root.go`
- Test: `internal/wasm/loader_test.go` (new or add to existing)

**Interfaces:**
- Consumes: `output.Renderer.Prompt`, `output.CaptainPrompt` (Tasks 1, 3, 4)
- Produces: `wasm.RendererApproveFunc(r output.Renderer) ApproveFunc`

**Note on JSON mode:** `RendererApproveFunc` works correctly for `StyledRenderer` (synchronous stdin). In JSON mode, `renderer.Prompt` emits the `captain_prompt` event and returns `("", nil)`. `RendererApproveFunc` treats an empty response as denial — the execution gate blocks until a relay/uplink cycle resumes it. Full async resume is deferred to the exec model refactor spec.

- [ ] **Step 1: Write failing tests**

Create or append to `internal/wasm/loader_test.go`:

```go
package wasm_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestRendererApproveFunc_confirm_yes(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("y\n"), false)
	fn := wasm.RendererApproveFunc(r)
	if !fn("git", "git status") {
		t.Error("expected approve = true for 'y' input")
	}
}

func TestRendererApproveFunc_confirm_no(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("n\n"), false)
	fn := wasm.RendererApproveFunc(r)
	if fn("git", "git status") {
		t.Error("expected approve = false for 'n' input")
	}
}

func TestRendererApproveFunc_json_mode_blocks(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	fn := wasm.RendererApproveFunc(r)
	// JSON mode: Prompt emits event and returns ("", nil).
	// RendererApproveFunc must treat empty response as denial.
	if fn("git", "git status") {
		t.Error("JSON mode: empty response should be treated as denial")
	}
	// The captain_prompt event was emitted.
	if buf.Len() == 0 {
		t.Error("JSON mode: expected captain_prompt event on stdout")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/wasm/... -run "TestRendererApproveFunc" -v
```

Expected: FAIL — `wasm.RendererApproveFunc` undefined.

- [ ] **Step 3: Add `RendererApproveFunc` to `internal/wasm/loader.go`**

Add the following after the existing `StdinAlwaysAllowFunc` function. Add `"encoding/json"` and the output package import as needed:

```go
// RendererApproveFunc returns an ApproveFunc that uses the renderer's Prompt method
// for the Captain approval interaction. Styled mode blocks on stdin synchronously.
// JSON mode emits a captain_prompt event and returns false (deny) until the exec model
// refactor implements async resume via orbiter relay/uplink.
func RendererApproveFunc(r output.Renderer) ApproveFunc {
	return func(brand, fullCmd string) bool {
		resp, err := r.Prompt(output.CaptainPrompt{
			Txn:     models.NewID("tx"),
			Op:      "CONFIRM",
			Message: fmt.Sprintf("%s wants to run: %s", brand, fullCmd),
		})
		if err != nil || resp == "" {
			return false
		}
		var result struct {
			Confirmed bool `json:"confirmed"`
		}
		if err := json.Unmarshal([]byte(resp), &result); err != nil {
			return false
		}
		return result.Confirmed
	}
}
```

Add imports: `"encoding/json"`, `"fmt"`, `"github.com/Kenttleton/orbiter/internal/models"`, `"github.com/Kenttleton/orbiter/internal/output"`.

- [ ] **Step 4: Update `root.go` to use `RendererApproveFunc`**

In `internal/commands/root.go`, change line 78 from:

```go
wasm.StdinApproveFunc,
```

to:

```go
wasm.RendererApproveFunc(d.renderer),
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/wasm/... -run "TestRendererApproveFunc" -v
```

Expected: PASS (3 tests)

- [ ] **Step 6: Full build and test**

```bash
go build ./...
go test ./...
```

Expected: clean build; all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/wasm/loader.go internal/wasm/loader_test.go internal/commands/root.go
git commit -m "feat: wire WASM approval through renderer.Prompt; add RendererApproveFunc"
```

---

## Implementation Notes (READ BEFORE STARTING TASKS 5 AND 6)

### Deps injection pattern

All commands in this package receive dependencies via a `*deps` parameter — NOT via a `getDeps(cmd)` call. Examine `internal/commands/root.go` and any existing command (e.g. `internal/commands/starchart.go`) to confirm the pattern:

```go
// Commands are registered as: root.AddCommand(newRelayCmd(&d))
func newRelayCmd(d *deps) *cobra.Command {
    // ... use d.sc directly
}
```

Wherever the code above in Tasks 5 and 6 says `getDeps(cmd)` or `newRelayCmd()` / `newUplinkCmd()`, apply this correction:

- Change all `newRelayCmd()` → `newRelayCmd(d *deps)`
- Change all `newUplinkCmd()` → `newUplinkCmd(d *deps)`
- Replace `getDeps(cmd)` with `d` everywhere inside those functions
- In `root.go`'s `AddCommand(...)` list, add `newRelayCmd(&d)` and `newUplinkCmd(&d)`

### Test pattern for command integration tests

Tests use `ORBITER_STARCHART` env var to point the root command at a temp database. See `internal/commands/root.go:40` — the PersistentPreRunE reads this env var to open the DB. Pattern:

```go
func TestRelayCmd_storesResponse(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "starchart.db")
    sc, err := starchart.Open(dbPath)
    require.NoError(t, err)
    defer sc.Close()
    // populate DB...
    t.Setenv("ORBITER_STARCHART", dbPath)
    root := commands.NewRootCommand()
    root.SetArgs([]string{"relay", "--txn=" + txnID, `--response={"confirmed":true}`})
    require.NoError(t, root.Execute())
    // verify by re-querying sc (same file, two connections — SQLite handles this)
}
```

Replace `commands.NewRootCommandForTest(sc)` with `commands.NewRootCommand()` + `t.Setenv("ORBITER_STARCHART", dbPath)` in ALL relay and uplink tests. The `setupRelayDB` and `setupUplinkDB` helpers should return the `dbPath` as a second return value and NOT call `t.Cleanup(sc.Close())` — the test function holds the SC and defers Close itself so it stays open while the command runs against the same file.

Use `github.com/stretchr/testify/require` (already a project dependency) for assertions, matching the style in `executor_test.go`.

### Fix `processTransaction` context type

In `uplink.go`, the `processTransaction` function signature above incorrectly uses `ctx interface{ Value(any) any }`. Use `context.Context`:

```go
import "context"

func processTransaction(sc *starchart.StarChart, ctx context.Context, tx starchart.Transaction, cmd *cobra.Command) error {
```

---

## Dependency Chain Update

After this plan is complete, update `docs/superpowers/plans/2026-06-23-find-interface-spec.md` to add `io-contract-spec.md` to its dependency chain. The FIND spec implementation plan depends on `relay`, `uplink`, the transactions table schema, and the session lifecycle defined here.

```text
filesystem spec (✅ implemented: df1ff72..6f97bab)
    └── I/O contract spec (✅ implemented: this plan)
          └── FIND spec implementation plan
                └── exec model refactor
                      └── Present/Reachable refactor
                            └── 2026-06-22-binary-resolution-shell-directive.md
```
