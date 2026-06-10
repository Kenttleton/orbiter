package starchart_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func setupResolveDB(t *testing.T) (*starchart.StarChart, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })

	id := models.NewID(models.EntityTypePlanet)
	err = sc.Insert(context.Background(), "aliases", testAlias(id, "payment-api", models.EntityTypePlanet))
	require.NoError(t, err)
	return sc, id
}

func TestResolveByName(t *testing.T) {
	ctx := context.Background()
	sc, id := setupResolveDB(t)

	alias, err := sc.Resolve(ctx, "payment-api")
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
	require.Equal(t, models.EntityTypePlanet, alias.EntityType)
}

func TestResolveByID(t *testing.T) {
	ctx := context.Background()
	sc, id := setupResolveDB(t)

	alias, err := sc.Resolve(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
	require.Equal(t, "payment-api", alias.Name)
}

func TestResolveNotFound(t *testing.T) {
	ctx := context.Background()
	sc, _ := setupResolveDB(t)

	_, err := sc.Resolve(ctx, "does-not-exist")
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestResolveNameTakesPrecedenceOverIDLookup(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	// Insert an alias whose name equals its ID (the default case when no alias is given).
	id := models.NewID(models.EntityTypePlanet)
	err = sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet))
	require.NoError(t, err)

	alias, err := sc.Resolve(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
}
