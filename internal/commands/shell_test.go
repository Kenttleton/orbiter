package commands_test

import (
	"strings"
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

// The shell is detected from the environment. Detection cannot rely on BASH_VERSION or
// ZSH_VERSION alone: shells keep those as shell variables and do not export them, so a
// child process such as orbiter never sees them. The login shell ($SHELL) is the fallback.
func TestShellScriptFor(t *testing.T) {
	const self = "/opt/orbiter/bin/orbiter"
	marker := map[string]string{
		"bash": "PROMPT_COMMAND", "zsh": "add-zsh-hook", "fish": "function _orbiter_hook", "powershell": "Invoke-Orbiter",
	}
	cases := []struct {
		name string
		env  map[string]string
		want string // brand, "" for an error
	}{
		{"zsh login shell (the macOS default)", map[string]string{"SHELL": "/bin/zsh"}, "zsh"},
		{"bash login shell", map[string]string{"SHELL": "/bin/bash"}, "bash"},
		{"fish login shell with a long path", map[string]string{"SHELL": "/usr/local/bin/fish"}, "fish"},
		{"pwsh login shell", map[string]string{"SHELL": "/usr/local/bin/pwsh"}, "powershell"},
		{"an exported version variable wins", map[string]string{"FISH_VERSION": "3.7", "SHELL": "/bin/zsh"}, "fish"},
		{"nothing to go on", map[string]string{}, ""},
		{"unsupported login shell", map[string]string{"SHELL": "/bin/tcsh"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script, err := commands.ShellScriptFor(c.env, self)
			if c.want == "" {
				if err == nil {
					t.Fatalf("want an error, got a script")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(script, marker[c.want]) {
				t.Errorf("want the %s hook (marker %q)", c.want, marker[c.want])
			}
			if strings.Contains(script, "::ORBITER::") || !strings.Contains(script, self) {
				t.Errorf("the placeholder must be replaced by the binary path")
			}
		})
	}
}
