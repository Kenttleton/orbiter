package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

// printShellScript prints the shell integration script for the detected shell.
func printShellScript() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	script, err := shellScriptFor(osEnvMap(), self)
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}

// shellScriptFor returns the hook script for the detected shell, with the binary path
// filled in. The shell is detected first from the manifests' detection rules over the
// exported environment, then from the login shell ($SHELL). The rules alone are not
// enough: shells keep BASH_VERSION and ZSH_VERSION as shell variables and do not export
// them, so a child process such as orbiter never sees them.
func shellScriptFor(env map[string]string, self string) (string, error) {
	type candidate struct {
		brand    string
		manifest core.Manifest
	}
	var candidates []candidate
	for _, entry := range bundle.CatalogEntries() {
		if !slices.Contains(entry.Roles, core.ResourceRoleShell) {
			continue
		}
		manifestBytes, err := bundle.BundleFS.ReadFile(entry.Brand + "/manifest.toml")
		if err != nil {
			continue
		}
		var m core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &m); err != nil {
			continue
		}
		candidates = append(candidates, candidate{entry.Brand, m})
	}

	render := func(c candidate) (string, bool) {
		hookFile := c.manifest.Shell.HookFile()
		if hookFile == "" {
			return "", false
		}
		script, err := bundle.BundleFS.ReadFile(c.brand + "/" + hookFile)
		if err != nil {
			return "", false
		}
		return strings.ReplaceAll(string(script), "::ORBITER::", self), true
	}

	for _, c := range candidates {
		if c.manifest.Detection.MatchesAny(nil, env) {
			if script, ok := render(c); ok {
				return script, nil
			}
		}
	}
	// The login shell: "/bin/zsh" selects the zsh integration, "pwsh" the powershell one.
	if login := strings.TrimSuffix(filepath.Base(env["SHELL"]), ".exe"); login != "." && login != "" {
		if login == "pwsh" {
			login = "powershell"
		}
		for _, c := range candidates {
			if c.brand == login {
				if script, ok := render(c); ok {
					return script, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no shell integration detected — run orbiter in bash, zsh, fish, or pwsh")
}

// osEnvMap parses os.Environ() into a key→value map.
func osEnvMap() map[string]string {
	raw := os.Environ()
	m := make(map[string]string, len(raw))
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func vesselInitRun(out io.Writer, yes bool) (VesselInitResult, error) {
	dir := bundle.DefaultIntegrationsDir()
	states, err := bundle.CatalogEntriesWithState(dir)
	if err != nil {
		return VesselInitResult{}, fmt.Errorf("check integration state: %w", err)
	}

	if yes {
		// Non-interactive: install all catalog entries.
		entries := make([]bundle.CatalogEntry, len(states))
		for i, s := range states {
			entries[i] = s.CatalogEntry
		}
		if err := bundle.ExtractSelected(entries, dir); err != nil {
			return VesselInitResult{}, fmt.Errorf("install integrations: %w", err)
		}
		fmt.Fprintf(out, "Installed %d integration(s)\n", len(entries))
		return VesselInitResult{Installed: len(entries)}, nil
	}

	items := BuildChecklistItems(states)
	if len(items) == 0 {
		fmt.Fprintln(out, "No integrations available in catalog.")
		return VesselInitResult{}, nil
	}

	initial := NewChecklistModel("Select integrations to install:", items)
	result, err := tea.NewProgram(initial).Run()
	if err != nil {
		return VesselInitResult{}, fmt.Errorf("checklist: %w", err)
	}

	final := result.(ChecklistModel)
	if !final.Done() {
		fmt.Fprintln(out, "Cancelled.")
		return VesselInitResult{}, nil
	}

	if err := ApplySelections(states, final.Selected(), dir); err != nil {
		return VesselInitResult{}, fmt.Errorf("apply selections: %w", err)
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
	return VesselInitResult{Installed: installed, Removed: removed}, nil
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
				_, err := vesselInitRun(cmd.OutOrStdout(), yes)
				return err
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
