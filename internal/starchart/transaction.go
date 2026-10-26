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
		toNullableString(t.Payload), toNullableString(t.Response),
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

// GetTransaction returns one transaction by ID (sql.ErrNoRows when absent).
func (sc *StarChart) GetTransaction(ctx context.Context, id string) (Transaction, error) {
	var t Transaction
	var status string
	err := sc.db.QueryRowContext(ctx,
		`SELECT id, session_id, op, args, status, COALESCE(payload,''), COALESCE(response,'')
		 FROM transactions WHERE id=?`, id,
	).Scan(&t.ID, &t.SessionID, &t.Op, &t.Args, &status, &t.Payload, &t.Response)
	t.Status = TransactionStatus(status)
	return t, err
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

// toNullableString returns nil for an empty string (stored as SQL NULL) or the string value.
func toNullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
