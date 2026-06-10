package starchart_test

import (
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	for i := 0; i < 3; i++ {
		sc, err := starchart.Open(path)
		require.NoError(t, err, "open attempt %d", i)

		v, err := sc.SchemaVersion()
		require.NoError(t, err)
		require.Equal(t, 1, v)
		sc.Close()
	}
}
