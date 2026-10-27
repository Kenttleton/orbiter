package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Orbiter knows no shell by name. A shell integration is anything registered in the shell
// role; these tests use made-up brands to show that.

// fakeShell is an integration operating in the shell role.
type fakeShell struct {
	brand    string
	detected bool
	asked    *int // incremented each time Orbiter asks it to detect
}

func (f fakeShell) Meta() integrations.Manifest {
	return integrations.Manifest{Integration: integrations.ManifestIntegration{
		Brand: f.brand, Roles: []string{integrations.ResourceRoleShell},
	}}
}
func (f fakeShell) Detect(integrations.DetectContext) integrations.DetectReport {
	if f.asked != nil {
		*f.asked++
	}
	if !f.detected {
		return integrations.DetectReport{}
	}
	// The suggestion shape every integration uses: a resource to register, by role and brand.
	return integrations.DetectReport{Detected: true, Resources: []integrations.SuggestedResource{
		{Role: integrations.ResourceRoleShell, Brand: f.brand},
	}}
}
func (f fakeShell) Init(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}
func (f fakeShell) Scan(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}
func (f fakeShell) Calibrate(integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}

// shellFixture installs each brand's manifest and hook under dir, as vessel init does, and
// opens a star chart whose registry is reg.
func shellFixture(t *testing.T, reg *integrations.Registry, brands ...string) (dir string, sc *starchart.StarChart) {
	t.Helper()
	dir = t.TempDir()
	for _, b := range brands {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, b), 0o755))
		manifest := "[integration]\nbrand = \"" + b + "\"\nroles = [\"shell\"]\n\n[shell]\nexports = [{ hook = \"hook." + b + "\", description = \"hook\" }]\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, b, "manifest.toml"), []byte(manifest), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, b, "hook."+b), []byte("# "+b+" hook, binary ::ORBITER::\n"), 0o644))
	}
	var err error
	sc, err = starchart.OpenWithRegistry(filepath.Join(t.TempDir(), "test.db"), reg)
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return dir, sc
}

func registeredShell(t *testing.T, sc *starchart.StarChart) string {
	t.Helper()
	vessel, err := sc.GetVessel(context.Background())
	require.NoError(t, err)
	att, err := sc.AttachedTo(context.Background(), vessel.ID)
	require.NoError(t, err)
	for _, r := range att.Resources {
		if r.Role == integrations.ResourceRoleShell {
			return r.Brand
		}
	}
	return ""
}

func newShell(sc *starchart.StarChart, dir string, load func() error) commands.ShellInit {
	return commands.NewShellInit(sc, dir, "/bin/orbiter", load)
}

// With no record of a shell, Orbiter discovers the vessel's shell the way it discovers a
// planet's resources: it asks the registered integrations to detect, registers what they
// suggest, and attaches it. Then it delegates to that integration.
func TestShellInit_NoRecordMeansDiscoveryThenRegistration(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: false})
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	dir, sc := shellFixture(t, reg, "alpha", "beta")
	loaded := false

	script, err := newShell(sc, dir, func() error { loaded = true; return nil }).Script(context.Background())
	require.NoError(t, err)

	assert.True(t, loaded, "the shell integrations are loaded only because there was no record")
	assert.Equal(t, "# beta hook, binary /bin/orbiter\n", script, "the answering integration's own hook, with the binary path filled in")
	assert.Equal(t, "beta", registeredShell(t, sc), "the shell is a resource on the vessel, like any other")
}

func TestShellInit_AfterRegistrationOrbiterUsesItsOwnRecordAndAsksNoOne(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	dir, sc := shellFixture(t, reg, "alpha", "beta")
	_, err := newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.NoError(t, err)

	// Later: the registry holds an integration that would answer differently and counts being
	// asked; loading must not happen either.
	asked := 0
	reg2 := integrations.NewRegistry(nil)
	reg2.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true, asked: &asked})
	sc.SetIntegrations(reg2)
	script, err := newShell(sc, dir, func() error { t.Fatal("must not load integrations when the shell is already recorded"); return nil }).
		Script(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 0, asked)
	assert.Equal(t, "# beta hook, binary /bin/orbiter\n", script)
}

func TestShellInit_NoIntegrationClaimsTheEnvironment(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: false})
	dir, sc := shellFixture(t, reg, "alpha")
	_, err := newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.Error(t, err)
	assert.Equal(t, "", registeredShell(t, sc), "nothing is recorded when nothing answered")
}

// Discovery suggestions for roles that are not shells are not shell init's business: it takes
// the shell suggestions and leaves the rest unregistered.
func TestShellInit_OnlyShellSuggestionsAreRegistered(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true})
	reg.Register("tool", "gamma", suggestsTool{})
	dir, sc := shellFixture(t, reg, "alpha")
	_, err := newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.NoError(t, err)

	vessel, err := sc.GetVessel(context.Background())
	require.NoError(t, err)
	att, err := sc.AttachedTo(context.Background(), vessel.ID)
	require.NoError(t, err)
	require.Len(t, att.Resources, 1, "only the shell was registered on the vessel")
	assert.Equal(t, "alpha", att.Resources[0].Brand)
}

// suggestsTool is an integration that detects and suggests a non-shell resource.
type suggestsTool struct{ fakeShell }

func (suggestsTool) Meta() integrations.Manifest {
	return integrations.Manifest{Integration: integrations.ManifestIntegration{Brand: "gamma", Roles: []string{"tool"}}}
}
func (suggestsTool) Detect(integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: true, Resources: []integrations.SuggestedResource{{Role: "tool", Brand: "gamma"}}}
}

func TestShellInit_SeveralAnswerYes_TheChoiceIsStable(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true})
	dir, sc := shellFixture(t, reg, "alpha", "beta")
	script, err := newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "# alpha hook, binary /bin/orbiter\n", script)
	assert.Equal(t, "alpha", registeredShell(t, sc))
}

func TestShellInit_RecordedShellThatIsNotInstalled(t *testing.T) {
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true})
	dir, sc := shellFixture(t, reg, "alpha")
	_, err := newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "alpha")))
	_, err = newShell(sc, dir, func() error { return nil }).Script(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vessel init")
}
