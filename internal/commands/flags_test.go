package commands_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The global flags follow docs/cli-conventions.md: -v volume, -t trace,
// -i interactive (attached: answers arrive on stdin), -a is a per-command scope flag.
func TestRootGlobalFlags(t *testing.T) {
	root := commands.NewRootCommand()
	for name, short := range map[string]string{"verbose": "v", "trace": "t", "interactive": "i", "output": ""} {
		f := root.PersistentFlags().Lookup(name)
		require.NotNil(t, f, "--%s must be a global flag", name)
		assert.Equal(t, short, f.Shorthand, "--%s shorthand", name)
	}
}
