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

func testDB(t *testing.T) *starchart.StarChart {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return sc
}

func testAlias(id, name, entityType string) models.Alias {
	return models.Alias{
		ID:         id,
		Name:       name,
		EntityType: entityType,
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}
}

func TestInsertAndGet(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	id := models.NewID(models.EntityTypePlanet)
	a := testAlias(id, "payment-api", models.EntityTypePlanet)

	require.NoError(t, sc.Insert(ctx, "aliases", a))

	var got models.Alias
	require.NoError(t, sc.Get(ctx, "aliases", id, &got))
	require.Equal(t, id, got.ID)
	require.Equal(t, "payment-api", got.Name)
	require.Equal(t, models.EntityTypePlanet, got.EntityType)
}

func TestGetNotFound(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	var got models.Alias
	err := sc.Get(ctx, "aliases", "nonexistent-id", &got)
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestList(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	ids := []string{
		models.NewID(models.EntityTypePlanet),
		models.NewID(models.EntityTypePlanet),
		models.NewID(models.EntityTypePlanet),
	}
	for _, id := range ids {
		require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet)))
	}

	var results []models.Alias
	require.NoError(t, sc.List(ctx, "aliases", &results,
		starchart.Filter{Column: "entity_type", Op: "=", Value: models.EntityTypePlanet},
	))
	require.Len(t, results, 3)
}

func TestListEmpty(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	// A fresh DB contains only the seeded vessel alias; filtering by a
	// non-vessel entity type must return an empty slice.
	var results []models.Alias
	require.NoError(t, sc.List(ctx, "aliases", &results,
		starchart.Filter{Column: "entity_type", Op: "=", Value: models.EntityTypePlanet},
	))
	require.Empty(t, results)
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	id := models.NewID(models.EntityTypePlanet)
	require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, "old-name", models.EntityTypePlanet)))

	require.NoError(t, sc.Update(ctx, "aliases", id, map[string]any{"name": "new-name"}))

	var got models.Alias
	require.NoError(t, sc.Get(ctx, "aliases", id, &got))
	require.Equal(t, "new-name", got.Name)
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	id := models.NewID(models.EntityTypePlanet)
	require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet)))

	require.NoError(t, sc.Delete(ctx, "aliases", id))

	var got models.Alias
	err := sc.Get(ctx, "aliases", id, &got)
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestInsertDuplicateNameFails(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	id1 := models.NewID(models.EntityTypePlanet)
	id2 := models.NewID(models.EntityTypeGalaxy)
	require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id1, "payment-api", models.EntityTypePlanet)))

	err := sc.Insert(ctx, "aliases", testAlias(id2, "payment-api", models.EntityTypeGalaxy))
	require.Error(t, err, "duplicate alias name must fail")
}
