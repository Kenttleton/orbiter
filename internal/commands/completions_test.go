package commands_test

import (
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionsCmd_BashOutput(t *testing.T) {
	root := commands.NewRootCommand()
	root.SetArgs([]string{"completions", "bash"})
	var buf strings.Builder
	root.SetOut(&buf)

	err := root.Execute()
	require.NoError(t, err)
	// Bash completions script should contain the binary name.
	assert.Contains(t, buf.String(), "orbiter")
}

func TestCompletionsCmd_ZshOutput(t *testing.T) {
	root := commands.NewRootCommand()
	root.SetArgs([]string{"completions", "zsh"})
	var buf strings.Builder
	root.SetOut(&buf)

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "orbiter")
}

func TestCompletionsCmd_InvalidShell(t *testing.T) {
	root := commands.NewRootCommand()
	root.SetArgs([]string{"completions", "nushell"})
	var buf strings.Builder
	root.SetOut(&buf)

	err := root.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}

func TestCompletionsCmd_ExistsInTree(t *testing.T) {
	root := commands.NewRootCommand()
	completionsCmd, _, err := root.Find([]string{"completions"})
	require.NoError(t, err)
	assert.NotNil(t, completionsCmd)
	assert.Equal(t, "completions [bash|zsh|fish|powershell]", completionsCmd.Use)
}
