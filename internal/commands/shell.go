package commands

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)

//go:embed shell/orbiter.bash
var bashScript string

//go:embed shell/orbiter.zsh
var zshScript string

//go:embed shell/orbiter.fish
var fishScript string

func printShellScript() error {
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
}

func vesselInitRun(out io.Writer) error {
	catalog := bundle.CatalogEntries()
	if err := bundle.InstallSelected(catalog, integrations.Default, nil); err != nil {
		return fmt.Errorf("install integrations: %w", err)
	}
	fmt.Fprintf(out, "Installed %d integration(s)\n", len(catalog))
	return nil
}

// newInitCmd returns the target-aware init command.
//
//	orbiter init           → vessel init (install bundled integrations)
//	orbiter init shell     → print shell integration script
//	orbiter init vessel    → vessel init (same as no target)
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [shell|vessel]",
		Short: "Initialize orbiter or a target — \"Make it ready.\"",
		Long: `Initialize orbiter or a named target.

  orbiter init           Initialize the vessel (installs bundled integrations)
  orbiter init vessel    Same as orbiter init
  orbiter init shell     Print the shell integration script

For shell integration add this to your profile:
  bash/zsh:  eval "$(orbiter init shell)"
  fish:      orbiter init shell | source`,
		Args: cobra.MaximumNArgs(1),
		// Override parent PersistentPreRunE — init needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			switch target {
			case "shell":
				return printShellScript()
			case "vessel", "":
				return vesselInitRun(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unknown init target %q — use 'shell' or 'vessel'", target)
			}
		},
	}
}

// newShellCmd returns the shell subcommand group.
//
//	orbiter shell init     → print shell integration script
func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage shell integration — built-in target",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: "Print the shell integration script",
			Long: `Print the shell integration script for your current shell.

Add this to your shell profile:
  bash/zsh:  eval "$(orbiter shell init)"
  fish:      orbiter shell init | source`,
			Args: cobra.NoArgs,
			// Override parent PersistentPreRunE — shell init needs no star chart.
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
			RunE: func(cmd *cobra.Command, args []string) error {
				return printShellScript()
			},
		},
	)
	return cmd
}
