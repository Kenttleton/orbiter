package starchart_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func setupResolveDB(t *testing.T) (*starchart.StarChart, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })

	entityID := models.NewID(models.EntityTypePlanet)
	err = sc.Insert(context.Background(), "aliases", models.AliasInsert{
		Name: "payment-api", Entity: entityID, CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	return sc, entityID
}

func TestResolveByName(t *testing.T) {
	ctx := context.Background()
	sc, entityID := setupResolveDB(t)

	alias, err := sc.Resolve(ctx, "payment-api")
	require.NoError(t, err)
	require.Equal(t, entityID, alias.ID)

	parsed, err := models.ParseID(alias.ID)
	require.NoError(t, err)
	require.Equal(t, models.EntityTypePlanet, parsed.EntityType)
}

func TestResolveByEntityID(t *testing.T) {
	ctx := context.Background()
	sc, entityID := setupResolveDB(t)

	alias, err := sc.Resolve(ctx, entityID)
	require.NoError(t, err)
	require.Equal(t, entityID, alias.ID)
	require.Equal(t, "payment-api", alias.Name)
}

func TestResolveNotFound(t *testing.T) {
	ctx := context.Background()
	sc, _ := setupResolveDB(t)

	_, err := sc.Resolve(ctx, "does-not-exist")
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestResolveNameTakesPrecedenceOverEntityIDLookup(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	// Insert an alias whose name equals the entity ID — both lookups hit the same row.
	entityID := models.NewID(models.EntityTypePlanet)
	err = sc.Insert(ctx, "aliases", models.AliasInsert{
		Name: entityID, Entity: entityID, CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	alias, err := sc.Resolve(ctx, entityID)
	require.NoError(t, err)
	require.Equal(t, entityID, alias.ID)
}
