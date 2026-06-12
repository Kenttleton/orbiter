package starchart_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/require"
)

func TestCreateGalaxy(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, err := sc.CreateGalaxy(ctx, "stride-build")
	require.NoError(t, err)
	require.NotEmpty(t, g.ID)

	alias, err := sc.Resolve(ctx, "stride-build")
	require.NoError(t, err)
	require.Equal(t, g.ID, alias.ID)

	parsed, err := models.ParseID(alias.ID)
	require.NoError(t, err)
	require.Equal(t, models.EntityTypeGalaxy, parsed.EntityType)

	beacon, err := sc.GetBeacon(ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreateGalaxyDuplicateNameErrors(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	_, err := sc.CreateGalaxy(ctx, "stride-build")
	require.NoError(t, err)
	_, err = sc.CreateGalaxy(ctx, "stride-build")
	require.Error(t, err)
}

func TestCreateSolarSystem(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	sys, err := sc.CreateSolarSystem(ctx, "platform", g.ID)
	require.NoError(t, err)
	require.Equal(t, g.ID, sys.GalaxyID)

	beacon, err := sc.GetBeacon(ctx, sys.ID)
	require.NoError(t, err)
	require.Equal(t, models.BeaconStatusUnverified, beacon.Status)
}

func TestCreatePlanet(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	p, err := sc.CreatePlanet(ctx, "payments-api", g.ID, "")
	require.NoError(t, err)
	require.Equal(t, g.ID, p.GalaxyID)
	require.Empty(t, p.SolarSystemID)
}

func TestCreateCallsign(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	cs, err := sc.CreateCallsign(ctx, "work-dev")
	require.NoError(t, err)
	require.NotEmpty(t, cs.ID)
}

func TestCreateTransponder(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	tp, err := sc.CreateTransponder(ctx, "work-github", "file", "github", `{"location":"~/.ssh/id_ed25519_work"}`)
	require.NoError(t, err)
	require.Equal(t, "file", tp.Role)
	require.Equal(t, "github", tp.Brand)
	require.Equal(t, `{"location":"~/.ssh/id_ed25519_work"}`, tp.Config)
}

func TestCreateResource(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	r, err := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
	require.NoError(t, err)
	require.Equal(t, "manager", r.Role)
	require.Equal(t, "nvm", r.Brand)
	require.Equal(t, `["node"]`, r.Manages)
}
