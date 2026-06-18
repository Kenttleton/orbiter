package starchart

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kenttleton/orbiter/internal/integrations"
	_ "modernc.org/sqlite"
)

// StarChart wraps the SQLite connection for the Star Chart database.
type StarChart struct {
	db           *sql.DB
	integrations integrationProvider
}

// Open opens or creates the Star Chart database at path, creating parent
// directories as needed, and applies any pending migrations.
func Open(path string) (*StarChart, error) {
	return OpenWithRegistry(path, integrations.Default)
}

// OpenWithRegistry opens or creates the Star Chart database at path with a
// custom integration registry, creating parent directories as needed, and
// applies any pending migrations.
func OpenWithRegistry(path string, reg integrationProvider) (*StarChart, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create starchart directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open starchart: %w", err)
	}

	// SQLite disables foreign key enforcement by default.
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	sc := &StarChart{db: db, integrations: reg}
	if err := sc.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return sc, nil
}

// Close closes the underlying database connection.
func (sc *StarChart) Close() error {
	return sc.db.Close()
}

// SetIntegrations replaces the integration registry (used in tests).
func (sc *StarChart) SetIntegrations(r *integrations.Registry) {
	sc.integrations = r
}

// Integration returns the registered integration for role+brand, or (nil, false).
func (sc *StarChart) Integration(role, brand string) (integrations.Integration, bool) {
	if sc.integrations == nil {
		return nil, false
	}
	return sc.integrations.Get(role, brand)
}

// SchemaVersion returns the highest applied migration version.
// Returns 0 if no migrations have been applied.
func (sc *StarChart) SchemaVersion() (int, error) {
	var version int
	row := sc.db.QueryRowContext(
		context.Background(),
		"SELECT COALESCE(MAX(version), 0) FROM schema_version",
	)
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("query schema version: %w", err)
	}
	return version, nil
}
