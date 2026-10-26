package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brewOnDisk reports Present only when the binary path Orbiter hands it exists,
// like a real integration that runs the binary it was given.
type brewOnDisk struct{}

func (brewOnDisk) Meta() integrations.Manifest {
	return integrations.Manifest{Integration: integrations.ManifestIntegration{Brand: "brew", Binaries: []string{"brew"}}}
}
func (brewOnDisk) Detect(integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{}
}
func (b brewOnDisk) report(ctx integrations.ResolvedContext) integrations.StateReport {
	p := ctx.Binaries["brew"]
	if p == "" {
		return integrations.StateReport{}
	}
	if _, err := os.Stat(p); err != nil {
		return integrations.StateReport{}
	}
	return integrations.StateReport{Present: true, Reachable: true, BinaryPath: p}
}
func (b brewOnDisk) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	return b.report(ctx)
}
func (b brewOnDisk) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	return b.report(ctx)
}
func (b brewOnDisk) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return b.report(ctx)
}

// lastPromptTxn returns the txn of the last captain_prompt event in out.
func lastPromptTxn(t *testing.T, out string) string {
	t.Helper()
	var txn string
	for _, line := range strings.Split(out, "\n") {
		var e output.Event
		if json.Unmarshal([]byte(line), &e) == nil && e.Type == "captain_prompt" {
			txn = e.Txn
		}
	}
	require.NotEmpty(t, txn, "no captain_prompt event in %q", out)
	return txn
}

func TestFindRecoveryLoop_StalePathToConfirmedNewPath(t *testing.T) {
	ctx := context.Background()
	reg := integrations.NewRegistry(nil)
	reg.Register("tool", "brew", brewOnDisk{})
	sc, err := starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })

	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	p, err := sc.CreatePlanet(ctx, "api", g.ID, "")
	require.NoError(t, err)
	res, err := sc.CreateResource(ctx, "brew", "tool", "brew", "[]", `{"binaries":{"brew":"/no/such/brew"}}`)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "brew", "api")
	require.NoError(t, err)

	real := filepath.Join(t.TempDir(), "brew")
	require.NoError(t, os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755))

	// 1. The stale record makes the integration report not-present; Orbiter does not
	//    rewrite the config on its own.
	scan, err := sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, scan.Resources, 1)
	assert.NotEqual(t, models.BeaconStatusHealthy, scan.Resources[0].BeaconStatus)
	recorded, err := sc.RecordedBinary(ctx, res.ID, "brew")
	require.NoError(t, err)
	assert.Equal(t, "/no/such/brew", recorded)

	// 2. Calibrate asks the shell to find it.
	out := &bytes.Buffer{}
	renderer := output.NewRendererTo(output.FormatJSON, false, out)
	dirs, err := commands.NewExecutor(sc, renderer).Calibrate(ctx, "api")
	require.NoError(t, err)
	require.Len(t, dirs, 2)
	session, findTxn := dirs[0].Key, dirs[1].Txn

	run := func(args ...string) {
		t.Helper()
		root := commands.NewRootCommandForTestWithRenderer(sc, renderer)
		root.SetOut(out)
		root.SetErr(out)
		root.SetArgs(args)
		require.NoError(t, root.Execute())
	}

	// 3. The shell reports one different location: a CONFIRM is raised and the old
	//    path is kept until the Captain approves.
	run("relay", "--txn="+findTxn, `--response={"status":"ok","matches":["`+real+`"]}`)
	run("uplink", "--session="+session)
	recorded, _ = sc.RecordedBinary(ctx, res.ID, "brew")
	assert.Equal(t, "/no/such/brew", recorded, "no auto-update on recovery")
	confirmTxn := lastPromptTxn(t, out.String())

	// 4. The Captain approves; uplink records the path and continues the calibrate.
	out.Reset()
	run("relay", "--txn="+confirmTxn, `--response={"confirmed":true}`)
	run("uplink", "--session="+session)

	recorded, _ = sc.RecordedBinary(ctx, res.ID, "brew")
	assert.Equal(t, filepath.ToSlash(real), recorded)
	sess, err := sc.GetSession(ctx, session)
	require.NoError(t, err)
	assert.Equal(t, "complete", sess.Status)
	assert.NotContains(t, out.String(), "FIND_BINARY", "the resumed calibrate needs no further FIND")

	// The beacon is already healthy: uplink re-ran the calibrate itself; this test
	// has not scanned since the stale-path scan in step 1.
	beacon, err := sc.GetBeacon(ctx, res.ID)
	require.NoError(t, err)
	assert.Equal(t, models.BeaconStatusHealthy, beacon.Status)

	scan, err = sc.ScanBranch(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, models.BeaconStatusHealthy, scan.Resources[0].BeaconStatus)
	assert.Empty(t, sc.NeededFinds(scan))
}

// countingBrew behaves like brewOnDisk and counts Calibrate calls.
type countingBrew struct {
	brewOnDisk
	calibrates *int
}

func (c countingBrew) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	*c.calibrates++
	return c.brewOnDisk.Calibrate(ctx)
}

// A FIND that ends without recording anything (zero matches, a declined CONFIRM,
// the same path again) must not leave the branch un-calibrated or loop.
func TestFindRecoveryLoop_UnresolvedFindStillCalibratesAndDoesNotLoop(t *testing.T) {
	ctx := context.Background()
	calls := 0
	reg := integrations.NewRegistry(nil)
	reg.Register("tool", "brew", countingBrew{calibrates: &calls})
	sc, err := starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	g, err := sc.CreateGalaxy(ctx, "acme")
	require.NoError(t, err)
	_, err = sc.CreatePlanet(ctx, "api", g.ID, "")
	require.NoError(t, err)
	_, err = sc.CreateResource(ctx, "brew", "tool", "brew", "[]", `{"binaries":{"brew":"/no/such/brew"}}`)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, "brew", "api")
	require.NoError(t, err)

	out := &bytes.Buffer{}
	renderer := output.NewRendererTo(output.FormatJSON, false, out)
	dirs, err := commands.NewExecutor(sc, renderer).Calibrate(ctx, "api")
	require.NoError(t, err)
	require.Len(t, dirs, 2)
	assert.Equal(t, 0, calls, "calibration waits for discovery")

	run := func(args ...string) {
		t.Helper()
		root := commands.NewRootCommandForTestWithRenderer(sc, renderer)
		root.SetOut(out)
		root.SetErr(out)
		root.SetArgs(args)
		require.NoError(t, root.Execute())
	}
	run("relay", "--txn="+dirs[1].Txn, `--response={"status":"ok","matches":[]}`)
	out.Reset()
	run("uplink", "--session="+dirs[0].Key)

	assert.Equal(t, 1, calls, "the flow continues and calibrates even though nothing was found")
	for _, line := range strings.Split(out.String(), "\n") {
		assert.False(t, strings.HasPrefix(line, "FIND_BINARY ") || strings.HasPrefix(line, "SESSION "),
			"the resumed calibrate must not start another FIND round, got %q", line)
	}
}
