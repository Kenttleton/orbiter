package starchart_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanFilesystemResource(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "somefile.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("test"), 0644))

	cases := []struct {
		name             string
		path             func() string
		wantPresent      bool
		wantReachable    bool
		wantBeaconStatus string
	}{
		{
			name:             "missing path",
			path:             func() string { return "/tmp/orbiter-test-missing-XYZ-999" },
			wantPresent:      false,
			wantReachable:    false,
			wantBeaconStatus: models.BeaconStatusDrifted,
		},
		{
			name:             "existing directory",
			path:             func() string { return tmpDir },
			wantPresent:      true,
			wantReachable:    true,
			wantBeaconStatus: models.BeaconStatusHealthy,
		},
		{
			name:             "existing file not dir",
			path:             func() string { return existingFile },
			wantPresent:      true,
			wantReachable:    false,
			wantBeaconStatus: models.BeaconStatusDrifted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := testDB(t)
			g, err := sc.CreateGalaxy(ctx, "acme")
			require.NoError(t, err)
			p, err := sc.CreatePlanet(ctx, "app", g.ID, "")
			require.NoError(t, err)
			config := fmt.Sprintf(`{"path":%q}`, tc.path())
			_, err = sc.CreateResource(ctx, "fs-test", integrations.ResourceRoleFilesystem, "orbiter", "[]", config)
			require.NoError(t, err)
			_, err = sc.Attach(ctx, "fs-test", "app")
			require.NoError(t, err)

			result, err := sc.ScanBranch(ctx, p.ID)
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)

			got := result.Resources[0]
			assert.Equal(t, tc.wantPresent, got.Report.Present, "Present mismatch")
			assert.Equal(t, tc.wantReachable, got.Report.Reachable, "Reachable mismatch")
			assert.Equal(t, tc.wantBeaconStatus, string(got.BeaconStatus), "BeaconStatus mismatch")
		})
	}
}

func TestCalibrateFilesystemResource(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "somefile.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("test"), 0644))

	cases := []struct {
		name             string
		path             func() string
		wantPresent      bool
		wantReachable    bool
		wantBeaconStatus string
		wantAction       string
	}{
		{
			name:             "missing path",
			path:             func() string { return "/tmp/orbiter-test-missing-XYZ-999" },
			wantPresent:      false,
			wantReachable:    false,
			wantBeaconStatus: models.BeaconStatusDrifted,
			wantAction:       "drifted",
		},
		{
			name:             "existing directory",
			path:             func() string { return tmpDir },
			wantPresent:      true,
			wantReachable:    true,
			wantBeaconStatus: models.BeaconStatusHealthy,
			wantAction:       "healthy",
		},
		{
			name:             "existing file not dir",
			path:             func() string { return existingFile },
			wantPresent:      true,
			wantReachable:    false,
			wantBeaconStatus: models.BeaconStatusDrifted,
			wantAction:       "drifted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := testDB(t)
			g, err := sc.CreateGalaxy(ctx, "acme")
			require.NoError(t, err)
			p, err := sc.CreatePlanet(ctx, "app", g.ID, "")
			require.NoError(t, err)
			config := fmt.Sprintf(`{"path":%q}`, tc.path())
			_, err = sc.CreateResource(ctx, "fs-test", integrations.ResourceRoleFilesystem, "orbiter", "[]", config)
			require.NoError(t, err)
			_, err = sc.Attach(ctx, "fs-test", "app")
			require.NoError(t, err)

			result, err := sc.CalibrateBranch(ctx, p.ID)
			require.NoError(t, err)
			require.Len(t, result.Resources, 1)

			got := result.Resources[0]
			assert.Equal(t, tc.wantPresent, got.Before.Present, "Before.Present mismatch")
			assert.Equal(t, tc.wantReachable, got.Before.Reachable, "Before.Reachable mismatch")
			assert.Equal(t, tc.wantAction, got.Action, "Action mismatch")

			// Verify the beacon was persisted with the expected status.
			b, err := sc.GetBeacon(ctx, got.Resource.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantBeaconStatus, b.Status, "persisted BeaconStatus mismatch")
		})
	}
}
