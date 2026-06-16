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

	reg := integrations.NewRegistry(nil)
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

	reg := integrations.NewRegistry(nil)
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

func TestScanBranch_ResourceOrder(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

	// Create resources in REVERSE role order to verify they scan in correct order
	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payment-api")
	_, _ = sc.CreateResource(ctx, "node-rt", "runtime", "node", "[]", "{}")
	_, _ = sc.Attach(ctx, "node-rt", "payment-api")
	_, _ = sc.CreateResource(ctx, "fs", "filesystem", "orbiter", "[]", `{"path":"/tmp"}`)
	_, _ = sc.Attach(ctx, "fs", "payment-api")

	results, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)

	// Verify all three resources were dispatched
	require.Len(t, results.Resources, 3)

	// Verify filesystem came first in the ordered output
	assert.Equal(t, "filesystem", results.Resources[0].Resource.Role,
		"filesystem must scan before runtime")
	assert.Equal(t, "runtime", results.Resources[1].Resource.Role,
		"runtime must scan before remote")
	assert.Equal(t, "remote", results.Resources[2].Resource.Role)
}

func TestScanBranch_TransponderPass(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

	// Resource required: LeveledBranchCrawl skips levels with no resources.
	// The transponder pass runs at the same level as its resources.
	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payment-api")

	_, _ = sc.CreateCallsign(ctx, "kent-acme")
	tp, _ := sc.CreateTransponder(ctx, "acme-gh", "file", "github", `{"location":"/home/kent/.ssh/id_ed25519_acme"}`)
	_, _ = sc.Attach(ctx, "acme-gh", "kent-acme")
	_, _ = sc.Attach(ctx, "kent-acme", "payment-api")

	results, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)

	require.Len(t, results.Transponders, 1, "transponder pass must run at levels with resources")
	assert.Equal(t, tp.ID, results.Transponders[0].Transponder.ID,
		"transponder should match what was attached")
}

func TestScanBranch_GalaxyCallsignFlowsToPlanet(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payments", g.ID, "")

	// resource on planet so its level is included
	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payments")

	// callsign on galaxy — galaxy has NO resources, was previously skipped
	_, _ = sc.CreateCallsign(ctx, "corp-keys")
	tp, _ := sc.CreateTransponder(ctx, "corp-token", "file", "github", `{"location":"/tmp/token"}`)
	_, _ = sc.Attach(ctx, "corp-token", "corp-keys")
	_, _ = sc.Attach(ctx, "corp-keys", "acme")

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1,
		"galaxy callsign transponder must flow to planet scan even when galaxy has no resources")
	assert.Equal(t, tp.ID, result.Transponders[0].Transponder.ID)
	assert.NotEmpty(t, result.Transponders[0].BeaconStatus)
}

func TestScanCallsign_NoTransponders(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	cs, err := sc.CreateCallsign(ctx, "empty-keys")
	require.NoError(t, err)

	result, err := sc.ScanCallsign(ctx, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, result.Transponders)
}

func TestScanCallsign_WithTransponder(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	cs, _ := sc.CreateCallsign(ctx, "my-keys")
	tp, _ := sc.CreateTransponder(ctx, "gh-token", "file", "github", `{"location":"/tmp/token"}`)
	_, _ = sc.Attach(ctx, "gh-token", "my-keys")

	result, err := sc.ScanCallsign(ctx, cs.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1)
	assert.Equal(t, tp.ID, result.Transponders[0].Transponder.ID)
	// no integration registered → beacon status failed
	assert.Equal(t, models.BeaconStatusFailed, result.Transponders[0].BeaconStatus)
}

func TestScanTransponder_Isolation(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	tp, err := sc.CreateTransponder(ctx, "solo-token", "file", "github", `{"location":"/tmp/token"}`)
	require.NoError(t, err)
	// No callsign, no entity attachment — isolated scan

	result, err := sc.ScanTransponder(ctx, tp.ID)
	require.NoError(t, err)
	assert.Equal(t, tp.ID, result.Transponder.ID)
	assert.Equal(t, models.BeaconStatusFailed, result.BeaconStatus) // no integration
}

func TestCalibrateCallsign_NoTransponders(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	cs, err := sc.CreateCallsign(ctx, "empty-keys")
	require.NoError(t, err)

	result, err := sc.CalibrateCallsign(ctx, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, result.Transponders)
}

func TestCalibrateTransponder_SetsBeacon(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	tp, err := sc.CreateTransponder(ctx, "solo-token", "file", "github", `{"location":"/tmp/token"}`)
	require.NoError(t, err)

	result, err := sc.CalibrateTransponder(ctx, tp.ID)
	require.NoError(t, err)
	assert.Equal(t, tp.ID, result.Transponder.ID)
	assert.Equal(t, models.BeaconStatusFailed, result.Action)
	// no integration → beacon should be written as failed
	b, err := sc.GetBeacon(ctx, tp.ID)
	require.NoError(t, err)
	assert.Equal(t, models.BeaconStatusFailed, b.Status)
}

func TestScanBranch_DirectTransponderSupersedesCallsign(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payments", g.ID, "")

	_, _ = sc.CreateResource(ctx, "gh-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "gh-remote", "payments")

	// callsign with file/github transponder on planet
	_, _ = sc.CreateCallsign(ctx, "corp-keys")
	_, _ = sc.CreateTransponder(ctx, "corp-token", "file", "github", `{"location":"/tmp/corp"}`)
	_, _ = sc.Attach(ctx, "corp-token", "corp-keys")
	_, _ = sc.Attach(ctx, "corp-keys", "payments")

	// direct file/github transponder on planet — same role/brand, should supersede
	direct, _ := sc.CreateTransponder(ctx, "personal-token", "file", "github", `{"location":"/tmp/personal"}`)
	_, _ = sc.Attach(ctx, "personal-token", "payments")

	result, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, result.Transponders, 1,
		"direct transponder supersedes callsign transponder of same role/brand")
	assert.Equal(t, direct.ID, result.Transponders[0].Transponder.ID)
}
