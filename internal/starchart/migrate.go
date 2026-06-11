package starchart

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Kenttleton/orbiter/internal/migrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

func (sc *StarChart) migrate(ctx context.Context) error {
	// Ensure schema_version table exists before querying it.
	_, err := sc.db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS schema_version (
            version    INTEGER PRIMARY KEY,
            applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
    `)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	applied, err := sc.appliedVersions(ctx)
	if err != nil {
		return err
	}

	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	sort.Strings(files)

	for _, name := range files {
		version := migrationVersion(name)
		if version == 0 {
			continue
		}
		if applied[version] {
			continue
		}
		data, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := sc.applyMigration(ctx, version, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return sc.seedVessel(ctx)
}

// seedVessel inserts the single vessel row if it does not already exist.
// The vessel alias uses the reserved name "vessel".
func (sc *StarChart) seedVessel(ctx context.Context) error {
	var count int
	row := sc.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM vessel")
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check vessel: %w", err)
	}
	if count > 0 {
		return nil
	}
	id := models.NewID(models.EntityTypeVessel)
	now := time.Now().UTC()
	tx, err := sc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin vessel seed: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO aliases (name, entity, created_at) VALUES (?, ?, ?)",
		"vessel", id, now,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("seed vessel alias: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO vessel (id, created_at) VALUES (?, ?)",
		id, now,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("seed vessel row: %w", err)
	}
	return tx.Commit()
}

func (sc *StarChart) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := sc.db.QueryContext(ctx, "SELECT version FROM schema_version")
	if err != nil {
		return nil, fmt.Errorf("query applied versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (sc *StarChart) applyMigration(ctx context.Context, version int, sqlContent string) error {
	// Strip any INSERT INTO schema_version statements from the SQL — the
	// migration runner records version tracking itself to avoid duplicates.
	sqlContent = stripSchemaVersionInserts(sqlContent)

	tx, err := sc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, sqlContent); err != nil {
		tx.Rollback()
		return fmt.Errorf("execute migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_version (version) VALUES (?)", version,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration version: %w", err)
	}
	return tx.Commit()
}

// stripSchemaVersionInserts removes lines that insert into schema_version,
// since the migration runner handles version tracking itself.
func stripSchemaVersionInserts(sql string) string {
	lines := strings.Split(sql, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(trimmed, "insert into schema_version") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// migrationVersion parses the version number from a filename like "0001_initial.sql".
// Returns 0 if the filename doesn't match the expected format.
func migrationVersion(name string) int {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 1 {
		return 0
	}
	var v int
	fmt.Sscanf(parts[0], "%d", &v)
	return v
}
