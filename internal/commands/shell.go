package commands

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
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

func vesselInitRun(out io.Writer, yes bool) error {
	dir := bundle.DefaultIntegrationsDir()
	states, err := bundle.CatalogEntriesWithState(dir)
	if err != nil {
		return fmt.Errorf("check integration state: %w", err)
	}

	if yes {
		// Non-interactive: install all catalog entries.
		entries := make([]bundle.CatalogEntry, len(states))
		for i, s := range states {
			entries[i] = s.CatalogEntry
		}
		if err := bundle.ExtractSelected(entries, dir); err != nil {
			return fmt.Errorf("install integrations: %w", err)
		}
		fmt.Fprintf(out, "Installed %d integration(s)\n", len(entries))
		return nil
	}

	items := BuildChecklistItems(states)
	if len(items) == 0 {
		fmt.Fprintln(out, "No integrations available in catalog.")
		return nil
	}

	initial := NewChecklistModel("Select integrations to install:", items)
	result, err := tea.NewProgram(initial).Run()
	if err != nil {
		return fmt.Errorf("checklist: %w", err)
	}

	final := result.(ChecklistModel)
	if !final.Done() {
		fmt.Fprintln(out, "Cancelled.")
		return nil
	}

	if err := ApplySelections(states, final.Selected(), dir); err != nil {
		return fmt.Errorf("apply selections: %w", err)
	}

	installed := len(final.Selected())
	removed := 0
	selectedBrands := make(map[string]bool, len(final.Selected()))
	for _, sel := range final.Selected() {
		selectedBrands[sel.Tag] = true
	}
	for _, s := range states {
		if s.Installed && !selectedBrands[s.Brand] {
			removed++
		}
	}
	fmt.Fprintf(out, "Installed %d integration(s)", installed)
	if removed > 0 {
		fmt.Fprintf(out, ", removed %d", removed)
	}
	fmt.Fprintln(out)
	return nil
}

// newInitCmd returns the target-aware init command.
//
//	orbiter init           → vessel init (install bundled integrations)
//	orbiter init shell     → print shell integration script
//	orbiter init vessel    → vessel init (same as no target)
func newInitCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
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
				return vesselInitRun(cmd.OutOrStdout(), yes)
			default:
				return fmt.Errorf("unknown init target %q — use 'shell' or 'vessel'", target)
			}
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install all integrations without interactive selection")
	return cmd
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
