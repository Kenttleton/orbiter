package orbiter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/integrations/orbiter"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeShellRC(path string) integrations.ResolvedContext {
	return integrations.ResolvedContext{
		Platform: integrations.Platform{OS: "linux", Arch: "amd64"},
		Self: models.Resource{
			ID:     "r-sh-01",
			Role:   "shell",
			Brand:  "orbiter",
			Config: `{"path":"` + path + `"}`,
		},
		Resources:    map[string][]integrations.ResolvedResource{},
		Transponders: map[string][]integrations.ResolvedTransponder{},
	}
}

func TestShell_Scan_Present(t *testing.T) {
	dir := t.TempDir()
	si := orbiter.NewShell()
	report := si.Scan(makeShellRC(dir))
	assert.True(t, report.Present)
	assert.True(t, report.Reachable)
	assert.Equal(t, dir, report.InstallDir)
}

func TestShell_Scan_Missing(t *testing.T) {
	si := orbiter.NewShell()
	report := si.Scan(makeShellRC("/tmp/orbiter-does-not-exist-xyz-999"))
	assert.False(t, report.Present)
	assert.Equal(t, "/tmp/orbiter-does-not-exist-xyz-999", report.InstallDir)
}

func TestShell_Init_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newproject")
	si := orbiter.NewShell()
	report := si.Init(makeShellRC(dir))
	require.True(t, report.Present)
	assert.Equal(t, dir, report.InstallDir)
	_, err := os.Stat(dir)
	assert.NoError(t, err)
}

func TestShell_Registered(t *testing.T) {
	i, ok := integrations.Default.Get("shell", "orbiter")
	require.True(t, ok, "shell/orbiter should be registered")
	m := i.Meta()
	require.Len(t, m.Integration.Roles, 1)
	assert.Equal(t, "shell", m.Integration.Roles[0])
	assert.Equal(t, "orbiter", m.Integration.Brand)
}
