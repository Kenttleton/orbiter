package starchart_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingIntegration counts how many times Detect is called.
type countingIntegration struct {
	calls   int
	brand   string
	files   []string
	envRules []integrations.ManifestEnvRule
}

func (c *countingIntegration) Meta() integrations.Manifest {
	return integrations.Manifest{
		Integration: integrations.ManifestIntegration{Brand: c.brand, Roles: []string{"tool"}},
		Detection:   integrations.ManifestDetection{Files: c.files, Env: c.envRules},
	}
}
func (c *countingIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	c.calls++
	return integrations.DetectReport{Detected: false}
}
func (c *countingIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}
func (c *countingIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}
func (c *countingIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}

func openTestSC(t *testing.T) *starchart.StarChart {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "sc-*.db")
	require.NoError(t, err)
	f.Close()
	sc, err := starchart.Open(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	return sc
}

func TestDiscoverPlanet_EnvInContext(t *testing.T) {
	sc := openTestSC(t)
	reg := integrations.NewRegistry(nil)
	counter := &countingIntegration{brand: "test-env", files: []string{"go.mod"}}
	reg.Register("tool", "test-env", counter)
	sc.SetIntegrations(reg)

	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")
	os.WriteFile(gomod, []byte("module test"), 0644)

	t.Setenv("TEST_ENV_VAR", "hello")
	_, err := sc.DiscoverPlanet(context.Background(), dir)
	require.NoError(t, err)

	assert.Equal(t, 1, counter.calls, "integration with matching file should be called")
}

func TestDiscoverPlanet_PreFilterSkipsNonMatchingEnv(t *testing.T) {
	sc := openTestSC(t)
	reg := integrations.NewRegistry(nil)
	counter := &countingIntegration{
		brand:    "env-only",
		envRules: []integrations.ManifestEnvRule{{Key: "DEFINITELY_NOT_SET_XYZ"}},
	}
	reg.Register("tool", "env-only", counter)
	sc.SetIntegrations(reg)

	dir := t.TempDir()
	os.Unsetenv("DEFINITELY_NOT_SET_XYZ")
	_, err := sc.DiscoverPlanet(context.Background(), dir)
	require.NoError(t, err)

	assert.Equal(t, 0, counter.calls, "integration whose env rule doesn't match should be skipped")
}

func TestDiscoverPlanet_PreFilterSkipsNoMatchingFile(t *testing.T) {
	sc := openTestSC(t)
	reg := integrations.NewRegistry(nil)
	counter := &countingIntegration{
		brand: "file-only",
		files: []string{"Cargo.toml"},
	}
	reg.Register("tool", "file-only", counter)
	sc.SetIntegrations(reg)

	dir := t.TempDir()
	// no Cargo.toml in dir
	_, err := sc.DiscoverPlanet(context.Background(), dir)
	require.NoError(t, err)

	assert.Equal(t, 0, counter.calls, "integration whose file rule doesn't match should be skipped")
}
