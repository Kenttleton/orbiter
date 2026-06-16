package commands_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransponderAdd_LocationBuildsJSON(t *testing.T) {
	// Point ORBITER_STARCHART to a temp DB so the command uses our DB.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("ORBITER_STARCHART", dbPath)

	root := commands.NewRootCommand()
	root.SetArgs([]string{"transponder", "add", "gh-token",
		"--role", "file",
		"--brand", "github",
		"--location", "/home/kent/.github_token",
	})
	err := root.Execute()
	require.NoError(t, err)

	// Open the same DB and read back the transponder config.
	sc, err := starchart.Open(dbPath)
	require.NoError(t, err)
	defer sc.Close()

	alias, err := sc.Resolve(context.Background(), "gh-token")
	require.NoError(t, err)

	var tp models.Transponder
	err = sc.Get(context.Background(), "transponders", alias.ID, &tp)
	require.NoError(t, err)

	var cfg map[string]string
	require.NoError(t, json.Unmarshal([]byte(tp.Config), &cfg), "config must be valid JSON, got: %s", tp.Config)
	assert.Equal(t, "/home/kent/.github_token", cfg["location"])
}

func TestTransponderInit_LocationBuildsJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("ORBITER_STARCHART", dbPath)

	root := commands.NewRootCommand()
	root.SetArgs([]string{"transponder", "init", "gh-token-init",
		"--role", "file",
		"--brand", "github",
		"--location", "/home/kent/.github_token",
	})
	err := root.Execute()
	require.NoError(t, err)

	sc, err := starchart.Open(dbPath)
	require.NoError(t, err)
	defer sc.Close()

	alias, err := sc.Resolve(context.Background(), "gh-token-init")
	require.NoError(t, err)

	var tp models.Transponder
	err = sc.Get(context.Background(), "transponders", alias.ID, &tp)
	require.NoError(t, err)

	var cfg map[string]string
	require.NoError(t, json.Unmarshal([]byte(tp.Config), &cfg), "config must be valid JSON, got: %s", tp.Config)
	assert.Equal(t, "/home/kent/.github_token", cfg["location"])
}
