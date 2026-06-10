package starchart_test

import (
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	sc, err := starchart.Open(path)
	require.NoError(t, err)
	require.NotNil(t, sc)
	sc.Close()
}

func TestOpenCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "nested", "test.db")
	sc, err := starchart.Open(path)
	require.NoError(t, err)
	sc.Close()
}

func TestOpenAppliesMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	sc, err := starchart.Open(path)
	require.NoError(t, err)
	defer sc.Close()

	version, err := sc.SchemaVersion()
	require.NoError(t, err)
	require.Equal(t, 1, version)
}

func TestOpenIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	sc1, err := starchart.Open(path)
	require.NoError(t, err)
	sc1.Close()

	sc2, err := starchart.Open(path)
	require.NoError(t, err)
	defer sc2.Close()

	version, err := sc2.SchemaVersion()
	require.NoError(t, err)
	require.Equal(t, 1, version)
}
