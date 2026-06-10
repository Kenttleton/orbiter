package starchart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("record not found")

// Filter represents a single WHERE clause condition.
type Filter struct {
	Column string
	Op     string // "=", "!=", "LIKE", ">", "<", etc.
	Value  any
}

// Insert inserts record into table. record must be a struct with db-tagged fields.
func (sc *StarChart) Insert(ctx context.Context, table string, record any) error {
	cols, placeholders, vals := reflectInsertFields(record)
	if len(cols) == 0 {
		return fmt.Errorf("insert: no db-tagged fields found on record")
	}
	q := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
	_, err := sc.db.ExecContext(ctx, q, vals...)
	return err
}

// Get fetches a single record by ID from table into dest.
// Returns ErrNotFound if no row exists.
func (sc *StarChart) Get(ctx context.Context, table, id string, dest any) error {
	cols := reflectSelectCols(dest)
	q := fmt.Sprintf(
		"SELECT %s FROM %s WHERE id = ?",
		strings.Join(cols, ", "),
		table,
	)
	row := sc.db.QueryRowContext(ctx, q, id)
	if err := reflectScan(row, dest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// List queries table with optional filters and appends results to dest.
// dest must be a pointer to a slice of structs with db-tagged fields.
func (sc *StarChart) List(ctx context.Context, table string, dest any, filters ...Filter) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("list: dest must be a pointer to a slice")
	}
	elemType := destVal.Elem().Type().Elem()
	elem := reflect.New(elemType).Interface()

	cols := reflectSelectCols(elem)
	where, args := buildWhere(filters)
	q := fmt.Sprintf("SELECT %s FROM %s%s", strings.Join(cols, ", "), table, where)

	rows, err := sc.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	sliceVal := destVal.Elem()
	for rows.Next() {
		item := reflect.New(elemType).Interface()
		if err := reflectScan(rows, item); err != nil {
			return err
		}
		sliceVal.Set(reflect.Append(sliceVal, reflect.ValueOf(item).Elem()))
	}
	return rows.Err()
}

// Update applies field updates to the record identified by id in table.
// fields is a map of column name → new value.
func (sc *StarChart) Update(ctx context.Context, table, id string, fields map[string]any) error {
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
	_, err := sc.db.ExecContext(ctx, q, vals...)
	return err
}

// Delete removes the record identified by id from table.
func (sc *StarChart) Delete(ctx context.Context, table, id string) error {
	q := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
	_, err := sc.db.ExecContext(ctx, q, id)
	return err
}
