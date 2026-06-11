package starchart_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
		return tx.Insert(ctx, "aliases", models.AliasInsert{
			Name: "my-galaxy", Entity: id, CreatedAt: time.Now().UTC(),
		})
	})
	require.NoError(t, err)

	alias, err := sc.Resolve(ctx, "my-galaxy")
	require.NoError(t, err)
	require.Equal(t, id, alias.ID)
	require.Equal(t, "my-galaxy", alias.Name)
}

func TestTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	id := models.NewID(models.EntityTypeGalaxy)
	intentionalErr := errors.New("intentional failure")

	err = sc.Tx(ctx, func(tx *starchart.Tx) error {
		if err := tx.Insert(ctx, "aliases", models.AliasInsert{
			Name: "should-rollback", Entity: id, CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		return intentionalErr
	})
	require.ErrorIs(t, err, intentionalErr)

	_, err = sc.Resolve(ctx, "should-rollback")
	require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestTxNestedInsert(t *testing.T) {
	ctx := context.Background()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer sc.Close()

	galaxyID := models.NewID(models.EntityTypeGalaxy)
	planetID := models.NewID(models.EntityTypePlanet)

	err = sc.Tx(ctx, func(tx *starchart.Tx) error {
		if err := tx.Insert(ctx, "aliases", models.AliasInsert{
			Name: "acme", Entity: galaxyID, CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err := tx.Insert(ctx, "galaxies", models.Galaxy{ID: galaxyID}); err != nil {
			return err
		}
		if err := tx.Insert(ctx, "aliases", models.AliasInsert{
			Name: "payment-api", Entity: planetID, CreatedAt: time.Now().UTC(),
		}); err != nil {
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
