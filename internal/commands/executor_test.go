package commands_test

import (
	"context"
	"os"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	_ "github.com/Kenttleton/orbiter/integrations/orbiter"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExportIntegration is a test-only integration that declares MY_TOKEN in its
// shell exports allowlist and returns both MY_TOKEN and SECRET in its scan report.
// This lets us verify that Hook only emits SET for declared keys.
type stubExportIntegration struct{}

func (stubExportIntegration) Meta() integrations.Manifest {
	return integrations.Manifest{
		Integration: integrations.ManifestIntegration{
			Brand: "stub-export",
			Roles: []string{"env"},
		},
		Shell: integrations.ManifestShell{
			Exports: []integrations.ManifestShellExport{
				{Envs: []string{"MY_TOKEN"}, Description: "test token", Sensitive: false},
			},
		},
	}
}

func (stubExportIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{}
}

func (stubExportIntegration) Init(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Reachable: true}
}

func (stubExportIntegration) Scan(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{
		Present:   true,
		Reachable: true,
		Exports: map[string]string{
			"MY_TOKEN": "abc",
			"SECRET":   "xyz",
		},
	}
}

func (stubExportIntegration) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{Present: true, Reachable: true}
}

func openTestExecutor(t *testing.T) *commands.Executor {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "starchart-*.db")
	require.NoError(t, err)
	f.Close()
	sc, err := starchart.Open(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	r := output.NewRenderer(output.FormatStyled, false)
	return commands.NewExecutor(sc, r)
}

func TestExecutor_Survey_NoTarget(t *testing.T) {
	exec := openTestExecutor(t)
	err := exec.Survey(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no target found")
}

func TestExecutor_Survey_WithTarget(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()
	_, err := exec.SC().CreateGalaxy(ctx, "acme")
	require.NoError(t, err)

	err = exec.Survey(ctx, "acme")
	require.NoError(t, err)
}

func TestExecutor_Scan_NoIntegration(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
	_, _ = exec.SC().CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	_, _ = exec.SC().Attach(ctx, "go", "payment-api")

	err := exec.Scan(ctx, "payment-api")
	require.NoError(t, err)
}

func TestExecutor_Chart(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

	err := exec.Chart(ctx, "payment-api")
	require.NoError(t, err)
}

func TestExecutor_Calibrate(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

	err := exec.Calibrate(ctx, "payment-api")
	require.NoError(t, err)
}

func TestExecutor_Retro_Confirmed(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "retro-galaxy")
	_, _ = exec.SC().CreatePlanet(ctx, "retro-planet", g.ID, "")

	// Retro the planet — confirmed=true skips the interactive prompt.
	err := exec.Retro(ctx, "retro-planet", true)
	require.NoError(t, err)

	// Planet should no longer be resolvable.
	_, err = exec.SC().Resolve(ctx, "retro-planet")
	assert.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestExecutor_Scan_Callsign_NoTransponders(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateCallsign(ctx, "my-keys")
	require.NoError(t, err)

	// scan callsign — should not error (routes to ScanCallsign)
	err = exec.Scan(ctx, "my-keys")
	require.NoError(t, err)
}

func TestExecutor_Survey_Callsign(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateCallsign(ctx, "my-keys")
	require.NoError(t, err)

	err = exec.Survey(ctx, "my-keys")
	require.NoError(t, err)
}

func TestExecutor_Scan_Transponder(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	_, err := exec.SC().CreateTransponder(ctx, "solo-token", "file", "github", `{"location":"/tmp/t"}`)
	require.NoError(t, err)

	err = exec.Scan(ctx, "solo-token")
	require.NoError(t, err)
}

func TestDirective_String_DIR(t *testing.T) {
	d := commands.Directive{Op: "DIR", Value: "/home/kent/work"}
	assert.Equal(t, "DIR /home/kent/work", d.String())
}

func TestDirective_String_SET(t *testing.T) {
	d := commands.Directive{Op: "SET", Key: "NODE_VERSION", Value: "20"}
	assert.Equal(t, "SET NODE_VERSION=20", d.String())
}

func TestDirective_String_SET_WithSpaces(t *testing.T) {
	d := commands.Directive{Op: "SET", Key: "GREETING", Value: "hello world"}
	assert.Equal(t, "SET GREETING=hello world", d.String())
}

func TestDirective_String_SET_NewlineEscaped(t *testing.T) {
	d := commands.Directive{Op: "SET", Key: "FOO", Value: "bar\nbaz"}
	assert.Equal(t, `SET FOO=bar\nbaz`, d.String())
}

func TestDirective_String_UNSET(t *testing.T) {
	d := commands.Directive{Op: "UNSET", Key: "NODE_VERSION"}
	assert.Equal(t, "UNSET NODE_VERSION", d.String())
}

func TestExecutor_Jump_Confirmed(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

	path := t.TempDir()
	config := `{"path":"` + path + `"}`
	_, _ = exec.SC().CreateResource(ctx, "payment-api-path", "shell", "orbiter", "[]", config)
	_, _ = exec.SC().Attach(ctx, "payment-api-path", "payment-api")

	// confirmed=true skips the interactive prompt
	directives, err := exec.Jump(ctx, "payment-api", true)
	require.NoError(t, err)
	require.NotEmpty(t, directives)
	assert.Equal(t, "DIR", directives[0].Op)
	assert.Equal(t, path, directives[0].Value)
	assert.Equal(t, "DIR "+path, directives[0].String())
	assert.NotContains(t, directives[0].String(), "cd ")
}

func TestExecutor_Jump_EmitsNeutralDirectives(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme2")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api2", g.ID, "")
	path := t.TempDir()
	_, _ = exec.SC().CreateResource(ctx, "root-res", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
	_, _ = exec.SC().Attach(ctx, "root-res", "payment-api2")

	directives, err := exec.Jump(ctx, "payment-api2", true)
	require.NoError(t, err)
	require.NotEmpty(t, directives)

	assert.Equal(t, "DIR", directives[0].Op)
	assert.Equal(t, path, directives[0].Value)
	assert.Equal(t, "DIR "+path, directives[0].String())
	assert.NotContains(t, directives[0].String(), "cd ")
}

func TestExecutor_Hook_SamePlanet_NoOutput(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
	path := t.TempDir()
	_, _ = exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
	_, _ = exec.SC().Attach(ctx, "root", "payment-api")

	alias, _ := exec.SC().Resolve(ctx, "payment-api")
	directives, err := exec.Hook(ctx, path, alias.ID)
	require.NoError(t, err)
	assert.Empty(t, directives, "same planet should return no directives")
}

func TestExecutor_Hook_NewPlanet_EmitsSet(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
	path := t.TempDir()
	_, _ = exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
	_, _ = exec.SC().Attach(ctx, "root", "payment-api")

	directives, err := exec.Hook(ctx, path, "")
	require.NoError(t, err)
	require.NotEmpty(t, directives)

	var foundPlanet bool
	for _, d := range directives {
		if d.Op == "SET" && d.Key == "ORBITER_PLANET" {
			foundPlanet = true
			assert.Equal(t, p.ID, d.Value)
		}
	}
	assert.True(t, foundPlanet, "Hook must emit SET ORBITER_PLANET")
}

func TestExecutor_Hook_LeavingPlanet_EmitsDepart(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	p, _ := exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
	path := t.TempDir()
	_, _ = exec.SC().CreateResource(ctx, "root", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
	_, _ = exec.SC().Attach(ctx, "root", "payment-api")

	directives, err := exec.Hook(ctx, "/tmp", p.ID)
	require.NoError(t, err)
	require.Len(t, directives, 1)
	assert.Equal(t, "DEPART", directives[0].Op)
}

func TestExecutor_Hook_NoMatchNoCurrent_Silent(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	directives, err := exec.Hook(ctx, "/tmp/orbiter-unknown-xyz", "")
	require.NoError(t, err)
	assert.Empty(t, directives)
}

// TestExecutor_Hook_ExportAllowlist verifies that Hook only emits SET directives
// for env vars declared in the integration's shell exports allowlist.
// MY_TOKEN is declared; SECRET is not — the hook must emit the former and suppress the latter.
func TestExecutor_Hook_ExportAllowlist(t *testing.T) {
	// Register the stub integration into the process-wide Default registry.
	// Deregister on cleanup so the stub doesn't bleed into other tests.
	integrations.Default.Register("env", "stub-export", stubExportIntegration{})
	t.Cleanup(func() { integrations.Default.Deregister("env", "stub-export") })

	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "allowlist-galaxy")
	_, _ = exec.SC().CreatePlanet(ctx, "allowlist-planet", g.ID, "")
	path := t.TempDir()
	_, _ = exec.SC().CreateResource(ctx, "allowlist-shell", "shell", "orbiter", "[]", `{"path":"`+path+`"}`)
	_, _ = exec.SC().Attach(ctx, "allowlist-shell", "allowlist-planet")
	_, _ = exec.SC().CreateResource(ctx, "allowlist-env", "env", "stub-export", "[]", "{}")
	_, _ = exec.SC().Attach(ctx, "allowlist-env", "allowlist-planet")

	directives, err := exec.Hook(ctx, path, "")
	require.NoError(t, err)
	require.NotEmpty(t, directives)

	setKeys := map[string]string{}
	for _, d := range directives {
		if d.Op == "SET" {
			setKeys[d.Key] = d.Value
		}
	}

	assert.Equal(t, "abc", setKeys["MY_TOKEN"], "declared export MY_TOKEN must be emitted")
	assert.NotContains(t, setKeys, "SECRET", "undeclared export SECRET must be suppressed")
}
