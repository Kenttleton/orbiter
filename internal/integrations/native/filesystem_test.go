package native_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations/native"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeRC(path string) integrations.ResolvedContext {
	return integrations.ResolvedContext{
		Platform: integrations.Platform{OS: "linux", Arch: "amd64"},
		Self: models.Resource{
			ID:     "r-fs-01",
			Role:   "filesystem",
			Brand:  "orbiter",
			Config: `{"path":"` + path + `"}`,
		},
		Resources:    map[string][]integrations.ResolvedResource{},
		Transponders: map[string][]integrations.ResolvedTransponder{},
	}
}

func TestFilesystemOrbiter_Scan_Present(t *testing.T) {
	dir := t.TempDir()
	fi := native.NewFilesystemOrbiter()
	report := fi.Scan(makeRC(dir))
	assert.True(t, report.Present)
	assert.True(t, report.Reachable)
	assert.Equal(t, dir, report.InstallDir)
}

func TestFilesystemOrbiter_Scan_Missing(t *testing.T) {
	fi := native.NewFilesystemOrbiter()
	report := fi.Scan(makeRC("/tmp/orbiter-does-not-exist-xyz-999"))
	assert.False(t, report.Present)
	assert.Equal(t, "/tmp/orbiter-does-not-exist-xyz-999", report.InstallDir)
}

func TestFilesystemOrbiter_Init_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newproject")
	fi := native.NewFilesystemOrbiter()
	report := fi.Init(makeRC(dir))
	require.True(t, report.Present)
	assert.Equal(t, dir, report.InstallDir)
	_, err := os.Stat(dir)
	assert.NoError(t, err)
}

func TestFilesystemOrbiter_Registered(t *testing.T) {
	i, ok := integrations.Default.Get("filesystem", "orbiter")
	require.True(t, ok, "filesystem/orbiter should be registered")
	m := i.Meta()
	require.Len(t, m.Integration.Roles, 1)
	assert.Equal(t, "filesystem", m.Integration.Roles[0])
	assert.Equal(t, "orbiter", m.Integration.Brand)
}
