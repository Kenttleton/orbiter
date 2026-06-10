package starchart_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func TestTxCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	id := models.NewID(models.EntityTypeGalaxy)

	err = sc.Tx(ctx, func(tx *starchart.Tx) error {
		return tx.Insert(ctx, "aliases", testAlias(id, "my-galaxy", models.EntityTypeGalaxy))
	})
	require.NoError(t, err)

	var got models.Alias
	require.NoError(t, sc.Get(ctx, "aliases", id, &got))
	require.Equal(t, "my-galaxy", got.Name)
}

func TestTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	id := models.NewID(models.EntityTypeGalaxy)
	intentionalErr := errors.New("intentional failure")

	err = sc.Tx(ctx, func(tx *starchart.Tx) error {
		if err := tx.Insert(ctx, "aliases", testAlias(id, "should-rollback", models.EntityTypeGalaxy)); err != nil {
			return err
		}
		return intentionalErr
	})
	require.ErrorIs(t, err, intentionalErr)

	var got models.Alias
	require.ErrorIs(t, sc.Get(ctx, "aliases", id, &got), starchart.ErrNotFound)
}

func TestTxNestedInsert(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	galaxyID := models.NewID(models.EntityTypeGalaxy)
	planetID := models.NewID(models.EntityTypePlanet)

	err = sc.Tx(ctx, func(tx *starchart.Tx) error {
		if err := tx.Insert(ctx, "aliases", testAlias(galaxyID, "acme", models.EntityTypeGalaxy)); err != nil {
			return err
		}
		if err := tx.Insert(ctx, "galaxies", models.Galaxy{ID: galaxyID}); err != nil {
			return err
		}
		if err := tx.Insert(ctx, "aliases", testAlias(planetID, "payment-api", models.EntityTypePlanet)); err != nil {
			return err
		}
		return tx.Insert(ctx, "planets", models.Planet{
			ID:       planetID,
			GalaxyID: galaxyID,
		})
	})
	require.NoError(t, err)

	var g models.Galaxy
	require.NoError(t, sc.Get(ctx, "galaxies", galaxyID, &g))
	var p models.Planet
	require.NoError(t, sc.Get(ctx, "planets", planetID, &p))
	require.Equal(t, galaxyID, p.GalaxyID)
}
