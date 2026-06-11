package starchart_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

// successIntegration always reports the entity as present and healthy.
type successIntegration struct{}

func (s *successIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: true}
}
func (s *successIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *successIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "test"}
}
func (s *successIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Manager: "test"}
}

func testDBWithRegistry(t *testing.T, reg *integrations.Registry) *starchart.StarChart {
	t.Helper()
	sc, err := starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return sc
}

func TestInitResourceNoIntegration(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)

	require.NoError(t, sc.InitResource(ctx, r.ID))

	beacon, err := sc.GetBeacon(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, models.BeaconStatusFailed, beacon.Status)
}

func TestInitResourceWithIntegration(t *testing.T) {
	ctx := context.Background()

	reg := integrations.NewRegistry()
	reg.Register("manager", "nvm", &successIntegration{})

	sc := testDBWithRegistry(t, reg)
	r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)

	require.NoError(t, sc.InitResource(ctx, r.ID))

	beacon, err := sc.GetBeacon(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, models.BeaconStatusVerified, beacon.Status)
}

func TestInitGalaxyCascadesToPlanets(t *testing.T) {
	ctx := context.Background()
	sc := testDB(t)

	g, _ := sc.CreateGalaxy(ctx, "stride-build")
	p, _ := sc.CreatePlanet(ctx, "payments-api", g.ID, "")
	r, _ := sc.CreateResource(ctx, "nvm-mgr", "manager", "nvm", `["node"]`, `{}`)
	sc.Attach(ctx, "nvm-mgr", "payments-api")

	require.NoError(t, sc.InitGalaxy(ctx, g.ID))

	// Planet beacon updated (not unverified anymore)
	pBeacon, err := sc.GetBeacon(ctx, p.ID)
	require.NoError(t, err)
	require.NotEqual(t, models.BeaconStatusUnverified, pBeacon.Status)

	// Resource beacon updated to failed (no integration registered)
	rBeacon, err := sc.GetBeacon(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, models.BeaconStatusFailed, rBeacon.Status)
}
