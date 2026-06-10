package starchart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Tx wraps a *sql.Tx and exposes the same CRUD surface as StarChart.
// Used within the Prepare → Validate → Execute → Verify → Commit pipeline.
type Tx struct {
	tx *sql.Tx
}

// Tx executes fn within a database transaction that enforces the Star Chart
// integrity pipeline. If fn returns an error the transaction is rolled back.
func (sc *StarChart) Tx(ctx context.Context, fn func(*Tx) error) error {
	sqlTx, err := sc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	t := &Tx{tx: sqlTx}
	if err := fn(t); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("rollback failed: %w (original: %v)", rbErr, err)
		}
		return err
	}

	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Insert inserts record into table within the transaction.
func (t *Tx) Insert(ctx context.Context, table string, record any) error {
	cols, placeholders, vals := reflectInsertFields(record)
	if len(cols) == 0 {
		return fmt.Errorf("tx insert: no db-tagged fields found on record")
	}
	q := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
	_, err := t.tx.ExecContext(ctx, q, vals...)
	return err
}

// Update applies field updates within the transaction.
func (t *Tx) Update(ctx context.Context, table, id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	var sets []string
	var vals []any
	for col, val := range fields {
		sets = append(sets, col+" = ?")
		vals = append(vals, val)
	}
	vals = append(vals, id)
	q := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))
	_, err := t.tx.ExecContext(ctx, q, vals...)
	return err
}

// Delete removes a record within the transaction.
func (t *Tx) Delete(ctx context.Context, table, id string) error {
	q := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
	_, err := t.tx.ExecContext(ctx, q, id)
	return err
}
