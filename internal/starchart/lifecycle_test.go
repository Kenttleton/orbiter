package starchart_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanBranch_NoIntegration(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "go", "payment-api")
	require.NoError(t, err)

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	assert.Equal(t, r.ID, result.Resources[0].Resource.ID)
	// no integration registered → failed beacon
	assert.Equal(t, models.BeaconStatusFailed, result.Resources[0].BeaconStatus)
}

func TestCalibrateBranch_NoIntegration(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "go", "payment-api")
	require.NoError(t, err)

	result, err := sc.CalibrateBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	assert.Equal(t, "failed", result.Resources[0].Action)
}

// healthyIntegration reports the entity as present, reachable, and healthy.
type healthyIntegration struct{}

func (h *healthyIntegration) Meta() integrations.Manifest { return integrations.Manifest{} }
func (h *healthyIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: true}
}
func (h *healthyIntegration) Init(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Reachable: true, Manager: "test"}
}
func (h *healthyIntegration) Scan(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Reachable: true, Manager: "test"}
}
func (h *healthyIntegration) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Reachable: true, Manager: "test"}
}

func TestScanBranch_WithIntegration_Healthy(t *testing.T) {
	ctx := context.Background()

	reg := integrations.NewRegistry()
	reg.Register("runtime", "go", &healthyIntegration{})
	sc := testDBWithRegistry(t, reg)

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	r, err := sc.CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "go", "payment-api")
	require.NoError(t, err)

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	assert.Equal(t, r.ID, result.Resources[0].Resource.ID)
	assert.Equal(t, models.BeaconStatusHealthy, result.Resources[0].BeaconStatus)
}

func TestCalibrateBranch_HealthyResource_NoCalibrate(t *testing.T) {
	ctx := context.Background()

	reg := integrations.NewRegistry()
	reg.Register("runtime", "go", &healthyIntegration{})
	sc := testDBWithRegistry(t, reg)

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "go", "payment-api")
	require.NoError(t, err)

	result, err := sc.CalibrateBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	// resource was healthy → action should be "healthy", no calibration call
	assert.Equal(t, "healthy", result.Resources[0].Action)
}
