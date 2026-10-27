package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vessel needs a shell, and a shell is a resource on the vessel like any other, so its
// drift is found the way every drift is found: a scan flags it. Reconciling it is the Captain's
// choice: Orbiter looks for shells through the installed integrations, shows what it found and
// what is missing, and replaces the shell with the one the Captain picks.

// shellsOnVessel lists the brands of the shell resources attached to the vessel.
func shellsOnVessel(t *testing.T, sc *starchart.StarChart) []string {
	t.Helper()
	vessel, err := sc.GetVessel(context.Background())
	require.NoError(t, err)
	att, err := sc.AttachedTo(context.Background(), vessel.ID)
	require.NoError(t, err)
	var out []string
	for _, r := range att.Resources {
		if r.Role == integrations.ResourceRoleShell {
			out = append(out, r.Brand)
		}
	}
	return out
}

// driftFixture records a shell on the vessel and returns an executor whose Captain answers
// prompts with answers (styled mode reads numbers from it).
func driftFixture(t *testing.T, reg *integrations.Registry, recordedBrand, recordedConfig, answers string) (*commands.Executor, *starchart.StarChart, *bytes.Buffer) {
	t.Helper()
	sc, err := starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	_, err = sc.CreateResource(ctx, recordedBrand, integrations.ResourceRoleShell, recordedBrand, "[]", recordedConfig)
	require.NoError(t, err)
	_, err = sc.Attach(ctx, recordedBrand, "vessel")
	require.NoError(t, err)
	buf := &bytes.Buffer{}
	r := output.NewStyledRendererWith(buf, strings.NewReader(answers), false)
	return commands.NewExecutor(sc, r), sc, buf
}

// The recorded shell's integration is not installed (the star chart came from another machine,
// or the integration was removed), and an installed integration finds a shell here.
func TestShellDrift_MissingIntegration_CaptainChoosesAnInstalledShell(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true, present: true})
	exec, sc, out := driftFixture(t, reg, "alpha", `{}`, "1\n")

	dirs, err := exec.Calibrate(context.Background(), "vessel")
	require.NoError(t, err)
	assert.Empty(t, dirs, "answered on the spot, nothing is left pending")

	shown := out.String()
	assert.Contains(t, shown, "alpha", "what is missing is named")
	assert.Contains(t, shown, "integration folder is not in the integrations directory", "and why")
	assert.Contains(t, shown, "beta", "the shells found are offered")
	assert.Equal(t, []string{"beta"}, shellsOnVessel(t, sc), "the chosen shell replaces the missing one")
}

// The shell's integration is there but the shell is not at the location recorded for it, which is
// what a star chart copied to a different machine looks like. FIND looks for it first (the first
// calibrate asks for that); when it finds nothing the run continues, and this is that continuation.
func TestShellDrift_RecordedLocationGone_IsDrift(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShellWithBinary{fakeShell{brand: "alpha", detected: false, present: false}})
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true, present: true})
	exec, sc, out := driftFixture(t, reg, "alpha", `{"binaries":{"alpha":"/gone/alpha"}}`, "1\n")

	first, err := exec.Calibrate(context.Background(), "vessel")
	require.NoError(t, err)
	require.NotEmpty(t, first)
	assert.Equal(t, "FIND_BINARY", first[len(first)-1].Op, "the shell is searched for before it is replaced")

	_, err = exec.CalibrateAfterFind(context.Background(), "vessel")
	require.NoError(t, err)

	assert.Contains(t, out.String(), "/gone/alpha", "the recorded location that no longer holds the shell")
	assert.Equal(t, []string{"beta"}, shellsOnVessel(t, sc))
}

// Nothing to choose from: the drift is flagged and the shell is left alone.
func TestShellDrift_NoShellFound_FlagsAndChangesNothing(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: false, present: true})
	exec, sc, out := driftFixture(t, reg, "alpha", `{}`, "")

	_, err := exec.Calibrate(context.Background(), "vessel")
	require.NoError(t, err)

	assert.Contains(t, out.String(), "alpha")
	assert.Contains(t, out.String(), "vessel init", "tells the Captain how to get an integration that can find a shell")
	assert.Equal(t, []string{"alpha"}, shellsOnVessel(t, sc))
}

// A shell whose location was never recorded is not drift: it has not been found yet, and finding
// it is the FIND flow's job, which comes first.
func TestShellDrift_NotYetFound_IsFindsBusinessNotDrift(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShellWithBinary{fakeShell{brand: "alpha", present: false}})
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true, present: true})
	exec, sc, out := driftFixture(t, reg, "alpha", `{}`, "1\n")

	dirs, err := exec.Calibrate(context.Background(), "vessel")
	require.NoError(t, err)

	require.NotEmpty(t, dirs)
	assert.Equal(t, "FIND_BINARY", dirs[len(dirs)-1].Op, "discovery of the shell's location is requested")
	assert.NotContains(t, out.String(), "Choose", "no replacement is offered")
	assert.Equal(t, []string{"alpha"}, shellsOnVessel(t, sc))
}

// fakeShellWithBinary declares the binary a shell needs found, as the real shells do.
type fakeShellWithBinary struct{ fakeShell }

func (f fakeShellWithBinary) Meta() integrations.Manifest {
	m := f.fakeShell.Meta()
	m.Integration.Binaries = []string{f.brand}
	return m
}

// Async (a client that is not attached): the prompt is announced with its txn, the run reports
// that it needs a decision in a session, and the client answers with relay and continues with
// uplink. The same choice is applied.
func TestShellDrift_AsyncClientAnswersWithRelayThenUplink(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true, present: true})
	_, sc, _ := driftFixture(t, reg, "alpha", `{}`, "")

	out := &bytes.Buffer{}
	renderer := output.NewRendererTo(output.FormatJSON, false, out)
	run := func(args ...string) {
		t.Helper()
		root := commands.NewRootCommandForTestWithRenderer(sc, renderer)
		root.SetOut(out)
		root.SetErr(out)
		root.SetArgs(args)
		require.NoError(t, root.Execute())
	}

	run("calibrate", "vessel")
	var promptTxn, session string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var e output.Event
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		switch e.Type {
		case "captain_prompt":
			promptTxn = e.Txn
		case "result":
			var res struct {
				Data struct {
					Status  string `json:"status"`
					Session string `json:"session"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(e.Data, &res))
			assert.Equal(t, "needs_decision", res.Data.Status)
			session = res.Data.Session
		}
	}
	require.NotEmpty(t, promptTxn, "the choice is announced as a captain_prompt")
	require.NotEmpty(t, session, "and the run says which session to continue")
	assert.Equal(t, []string{"alpha"}, shellsOnVessel(t, sc), "nothing changes until the Captain answers")

	out.Reset()
	run("relay", "--txn="+promptTxn, `--response={"selected":"beta"}`)
	run("uplink", "--session="+session)

	assert.Equal(t, []string{"beta"}, shellsOnVessel(t, sc))
}
