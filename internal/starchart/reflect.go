package starchart

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// scanner abstracts *sql.Row and *sql.Rows for reflectScan.
type scanner interface {
	Scan(dest ...any) error
}

// reflectInsertFields extracts db-tagged field names, "?" placeholders, and
// values from a struct (or pointer to struct). Fields with tag "-" are skipped.
func reflectInsertFields(record any) (cols, placeholders []string, vals []any) {
	v := reflect.ValueOf(record)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		cols = append(cols, tag)
		placeholders = append(placeholders, "?")
		vals = append(vals, v.Field(i).Interface())
	}
	return
}

// reflectSelectCols returns the ordered list of db-tagged column names for a
// struct (or pointer to struct).
func reflectSelectCols(record any) []string {
	v := reflect.ValueOf(record)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	var cols []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		cols = append(cols, tag)
	}
	return cols
}

// reflectScan scans a SQL row into a struct using db tags.
// dest must be a pointer to a struct.
func reflectScan(row scanner, dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("reflectScan: dest must be a pointer to a struct")
	}
	v = v.Elem()
	t := v.Type()
	var ptrs []any
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		ptrs = append(ptrs, v.Field(i).Addr().Interface())
	}
	return row.Scan(ptrs...)
}

// buildWhere constructs a WHERE clause and its argument list from filters.
// Returns empty string and nil if no filters are provided.
func buildWhere(filters []Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conditions []string
	var vals []any
	for _, f := range filters {
		conditions = append(conditions, fmt.Sprintf("%s %s ?", f.Column, f.Op))
		vals = append(vals, f.Value)
	}
	return " WHERE " + strings.Join(conditions, " AND "), vals
}

// nullableString is a helper for scanning nullable TEXT columns into Go strings.
// An empty string is stored as NULL and scanned back as "".
type nullableString struct {
	s *string
}

func (n nullableString) Scan(value any) error {
	if value == nil {
		return nil
	}
	ns := sql.NullString{}
	if err := ns.Scan(value); err != nil {
		return err
	}
	if ns.Valid {
		*n.s = ns.String
	}
	return nil
}
