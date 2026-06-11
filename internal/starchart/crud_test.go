package starchart_test

import (
	"context"
	"path/filepath"
	"testing"

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

func TestInsertAndGet(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, err := sc.CreateGalaxy(ctx, "stride-build")
	require.NoError(t, err)

	var got models.Galaxy
	require.NoError(t, sc.Get(ctx, "galaxies", g.ID, &got))
	require.Equal(t, g.ID, got.ID)
}

func TestGetNotFound(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	var got models.Galaxy
	err := sc.Get(ctx, "galaxies", "nonexistent-id", &got)
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestList(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	_, _ = sc.CreatePlanet(ctx, "payments-api", g.ID, "")
	_, _ = sc.CreatePlanet(ctx, "auth-service", g.ID, "")
	_, _ = sc.CreatePlanet(ctx, "notifications", g.ID, "")

	var results []models.Planet
	require.NoError(t, sc.List(ctx, "planets", &results,
		starchart.Filter{Column: "galaxy_id", Op: "=", Value: g.ID},
	))
	require.Len(t, results, 3)
}

func TestListEmpty(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")

	var results []models.Planet
	require.NoError(t, sc.List(ctx, "planets", &results,
		starchart.Filter{Column: "galaxy_id", Op: "=", Value: g.ID},
	))
	require.Empty(t, results)
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	r, err := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
	require.NoError(t, err)

	require.NoError(t, sc.Update(ctx, "resources", r.ID, map[string]any{"brand": "volta"}))

	var got models.Resource
	require.NoError(t, sc.Get(ctx, "resources", r.ID, &got))
	require.Equal(t, "volta", got.Brand)
	_ = g
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	r, err := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
	require.NoError(t, err)

	require.NoError(t, sc.Delete(ctx, "resources", r.ID))

	var got models.Resource
	err = sc.Get(ctx, "resources", r.ID, &got)
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestInsertDuplicateNameFails(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	_, err := sc.CreateGalaxy(ctx, "stride-build")
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "stride-build", "nonexistent-galaxy", "")
	require.Error(t, err, "duplicate alias name must fail")
}
