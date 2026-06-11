package commands_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarChartCmd_Registered(t *testing.T) {
	root := commands.NewRootCommand()
	cmd, _, err := root.Find([]string{"starchart"})
	require.NoError(t, err)
	assert.Equal(t, "starchart", cmd.Use)
}
