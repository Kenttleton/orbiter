package commands

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed shell/orbiter.bash
var bashScript string

//go:embed shell/orbiter.zsh
var zshScript string

//go:embed shell/orbiter.fish
var fishScript string

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Print shell integration script",
		Long: `Print the shell integration script for your shell.

Add this to your shell profile:

  bash/zsh:  eval "$(orbiter init)"
  fish:      orbiter init | source`,
		Args: cobra.NoArgs,
		// Override parent PersistentPreRunE — init needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve binary path: %w", err)
			}
			self, err = filepath.EvalSymlinks(self)
			if err != nil {
				return fmt.Errorf("resolve symlinks: %w", err)
			}

			shell := os.Getenv("SHELL")
			var script string
			switch {
			case strings.HasSuffix(shell, "fish"):
				script = fishScript
			case strings.HasSuffix(shell, "zsh"):
				script = zshScript
			default:
				script = bashScript
			}

			fmt.Print(strings.ReplaceAll(script, "::ORBITER::", self))
			return nil
		},
	}
}
