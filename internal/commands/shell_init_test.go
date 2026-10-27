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
	return integrations.DetectReport{Detected: f.detected}
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

// shellFixture installs each brand's manifest and hook under dir, as vessel init does.
func shellFixture(t *testing.T, brands ...string) (dir string, sc *starchart.StarChart) {
	t.Helper()
	dir = t.TempDir()
	for _, b := range brands {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, b), 0o755))
		manifest := "[integration]\nbrand = \"" + b + "\"\nroles = [\"shell\"]\n\n[shell]\nexports = [{ hook = \"hook." + b + "\", description = \"hook\" }]\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, b, "manifest.toml"), []byte(manifest), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, b, "hook."+b), []byte("# "+b+" hook, binary ::ORBITER::\n"), 0o644))
	}
	var err error
	sc, err = starchart.Open(filepath.Join(t.TempDir(), "test.db"))
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

func TestShellInit_FirstRunAsksIntegrationsThenRegistersTheOneThatAnswers(t *testing.T) {
	dir, sc := shellFixture(t, "alpha", "beta")
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: false})
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	loaded := false
	init := commands.NewShellInit(sc, reg, dir, map[string]string{}, "/bin/orbiter", func() error { loaded = true; return nil })

	script, err := init.Script(context.Background())
	require.NoError(t, err)

	assert.True(t, loaded, "the shell integrations are loaded only because there was no record")
	assert.Equal(t, "# beta hook, binary /bin/orbiter\n", script, "the answering integration's own hook, with the binary path filled in")
	assert.Equal(t, "beta", registeredShell(t, sc), "Orbiter records what it learned")
}

func TestShellInit_AfterRegistrationOrbiterUsesItsOwnRecordAndAsksNoOne(t *testing.T) {
	dir, sc := shellFixture(t, "alpha", "beta")
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	_, err := commands.NewShellInit(sc, reg, dir, nil, "/bin/orbiter", func() error { return nil }).Script(context.Background())
	require.NoError(t, err)

	// Second run: the registry holds integrations that would answer differently and fail
	// the test if asked, and loading must not happen.
	asked := 0
	reg2 := integrations.NewRegistry(nil)
	reg2.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true, asked: &asked})
	script, err := commands.NewShellInit(sc, reg2, dir, nil, "/bin/orbiter",
		func() error { t.Fatal("must not load integrations when the shell is already recorded"); return nil }).Script(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 0, asked)
	assert.Equal(t, "# beta hook, binary /bin/orbiter\n", script)
}

func TestShellInit_NoIntegrationClaimsTheEnvironment(t *testing.T) {
	dir, sc := shellFixture(t, "alpha")
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: false})
	_, err := commands.NewShellInit(sc, reg, dir, nil, "/bin/orbiter", func() error { return nil }).Script(context.Background())
	require.Error(t, err)
	assert.Equal(t, "", registeredShell(t, sc), "nothing is recorded when nothing answered")
}

func TestShellInit_SeveralAnswerYes_TheChoiceIsStable(t *testing.T) {
	dir, sc := shellFixture(t, "alpha", "beta")
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "beta", fakeShell{brand: "beta", detected: true})
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true})
	_, err := commands.NewShellInit(sc, reg, dir, nil, "/bin/orbiter", func() error { return nil }).Script(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "alpha", registeredShell(t, sc))
}

func TestShellInit_RecordedShellThatIsNotInstalled(t *testing.T) {
	dir, sc := shellFixture(t, "alpha")
	reg := integrations.NewRegistry(nil)
	reg.Register("shell", "alpha", fakeShell{brand: "alpha", detected: true})
	_, err := commands.NewShellInit(sc, reg, dir, nil, "/bin/orbiter", func() error { return nil }).Script(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "alpha")))
	_, err = commands.NewShellInit(sc, integrations.NewRegistry(nil), dir, nil, "/bin/orbiter", func() error { return nil }).Script(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vessel init")
}
