package resolver_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/resolver"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func testSC(t *testing.T) *starchart.StarChart {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return sc
}

func seedAlias(t *testing.T, sc *starchart.StarChart, id, name, entityType string) {
	t.Helper()
	err := sc.Insert(context.Background(), "aliases", models.Alias{
		ID: id, Name: name, EntityType: entityType,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
}

func TestResolverByName(t *testing.T) {
	ctx := context.Background()
	sc := testSC(t)
	id := models.NewID(models.EntityTypePlanet)
	seedAlias(t, sc, id, "payment-api", models.EntityTypePlanet)

	r := resolver.New(sc)
	alias, err := r.Resolve(ctx, "payment-api")
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
	require.Equal(t, models.EntityTypePlanet, alias.EntityType)
}

func TestResolverByID(t *testing.T) {
	ctx := context.Background()
	sc := testSC(t)
	id := models.NewID(models.EntityTypePlanet)
	seedAlias(t, sc, id, id, models.EntityTypePlanet)

	r := resolver.New(sc)
	alias, err := r.Resolve(ctx, id)
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
}

func TestResolverNotFound(t *testing.T) {
	ctx := context.Background()
	sc := testSC(t)

	r := resolver.New(sc)
	_, err := r.Resolve(ctx, "nonexistent")
	require.Error(t, err)
}
