package migrations_test

import (
    "testing"

    "github.com/Kenttleton/orbiter/internal/migrations"
    "github.com/stretchr/testify/require"
)

func TestFSContainsMigrations(t *testing.T) {
    entries, err := migrations.FS.ReadDir(".")
    require.NoError(t, err)
    require.NotEmpty(t, entries, "migrations directory should contain at least one SQL file")
}

func TestInitialMigrationReadable(t *testing.T) {
    data, err := migrations.FS.ReadFile("0001_initial.sql")
    require.NoError(t, err)
    require.Contains(t, string(data), "schema_version")
    require.Contains(t, string(data), "aliases")
    require.Contains(t, string(data), "vessel")
}
