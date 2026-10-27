package commands_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_Shell_CommandExists(t *testing.T) {
	root := commands.NewRootCommand()
	initCmd, _, err := root.Find([]string{"init"})
	require.NoError(t, err)
	assert.NotNil(t, initCmd)
}
