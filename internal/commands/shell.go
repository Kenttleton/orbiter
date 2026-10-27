package commands

import (
	"context"
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
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// shellInit decides which integration operates as the Captain's shell and returns its hook.
//
// Orbiter knows no shell by name. A shell integration is anything registered in the shell
// role. When Orbiter has a record of which one the Captain uses (a shell resource attached to
// the vessel) it simply delegates to it. Only when it has no record does it go out, in, out:
// ask each registered shell integration whether this environment is its own, take the answer,
// record it, and delegate. The Captain can swap the recorded shell later.
type shellInit struct {
	sc   *starchart.StarChart
	reg  *core.Registry
	dir  string            // where integrations are installed
	env  map[string]string // the environment offered to integrations that are asked to detect
	self string            // this binary's path, filled into the hook
	load func() error      // loads the installed shell integrations into reg
}

func (s shellInit) script(ctx context.Context) (string, error) {
	brand, err := s.recorded(ctx)
	if err != nil {
		return "", err
	}
	if brand == "" {
		if brand, err = s.detectAndRegister(ctx); err != nil {
			return "", err
		}
	}
	return s.hook(brand)
}

// recorded returns the brand of the shell resource attached to the vessel ("" if none).
func (s shellInit) recorded(ctx context.Context) (string, error) {
	vessel, err := s.sc.GetVessel(ctx)
	if err != nil {
		return "", fmt.Errorf("find the vessel: %w", err)
	}
	att, err := s.sc.AttachedTo(ctx, vessel.ID)
	if err != nil {
		return "", fmt.Errorf("read the vessel's attachments: %w", err)
	}
	for _, r := range att.Resources {
		if r.Role == core.ResourceRoleShell {
			return r.Brand, nil
		}
	}
	return "", nil
}

// detectAndRegister asks every registered shell integration to detect, records the first
// (by brand, so the choice is stable) that answers yes, and returns its brand.
func (s shellInit) detectAndRegister(ctx context.Context) (string, error) {
	if s.load != nil {
		if err := s.load(); err != nil {
			return "", fmt.Errorf("load shell integrations: %w", err)
		}
	}
	candidates := s.reg.AllForRole(core.ResourceRoleShell)
	slices.SortFunc(candidates, func(a, b core.Integration) int {
		return strings.Compare(a.Meta().Integration.Brand, b.Meta().Integration.Brand)
	})
	for _, c := range candidates {
		if !c.Detect(core.DetectContext{Env: s.env}).Detected {
			continue
		}
		brand := c.Meta().Integration.Brand
		if _, err := s.sc.CreateResource(ctx, brand, core.ResourceRoleShell, brand, "[]", "{}"); err != nil {
			// A resource of that name may already exist unattached; attach it.
			if _, rerr := s.sc.Resolve(ctx, brand); rerr != nil {
				return "", fmt.Errorf("record shell %s: %w", brand, err)
			}
		}
		if _, err := s.sc.Attach(ctx, brand, "vessel"); err != nil {
			return "", fmt.Errorf("attach shell %s to the vessel: %w", brand, err)
		}
		fmt.Fprintf(os.Stderr, "orbiter: registered %s as this vessel's shell\n", brand)
		return brand, nil
	}
	return "", fmt.Errorf("no installed shell integration reported this environment as its own — run 'orbiter vessel init' to install the bundled integrations")
}

// hook reads the hook script the integration's manifest declares, from the install
// directory, and fills in this binary's path.
func (s shellInit) hook(brand string) (string, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(s.dir, brand, "manifest.toml"))
	if err != nil {
		return "", fmt.Errorf("shell %q is registered but not installed — run 'orbiter vessel init': %w", brand, err)
	}
	var m core.Manifest
	if _, err := toml.Decode(string(manifestBytes), &m); err != nil {
		return "", fmt.Errorf("parse the manifest of %s: %w", brand, err)
	}
	hookFile := m.Shell.HookFile()
	if hookFile == "" {
		return "", fmt.Errorf("%s operates as a shell but declares no hook", brand)
	}
	script, err := os.ReadFile(filepath.Join(s.dir, brand, hookFile))
	if err != nil {
		return "", fmt.Errorf("read the hook of %s: %w", brand, err)
	}
	return strings.ReplaceAll(string(script), "::ORBITER::", s.self), nil
}

// printShellScript prints the shell integration script. It is what the Captain's profile
// evaluates, so nothing else may be written to stdout.
func (d *deps) printShellScript(ctx context.Context) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	dir := bundle.DefaultIntegrationsDir()
	script, err := shellInit{
		sc: d.sc, reg: core.Default, dir: dir, env: osEnvMap(), self: self,
		load: func() error {
			return bundle.LoadInstalled(dir, core.Default, wasm.RendererPrompts(d.renderer, d.sc), core.ResourceRoleShell)
		},
	}.script(ctx)
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
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
func newInitCmd(d *deps) *cobra.Command {
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
		// Override the parent PersistentPreRunE: init must not load every integration. It opens
		// the star chart (shell init reads and writes what Orbiter knows about the shell).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return d.prepareLight() },
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			switch target {
			case "shell":
				return d.printShellScript(cmd.Context())
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
func newShellCmd(d *deps) *cobra.Command {
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
			// Override the parent PersistentPreRunE: this runs in every new terminal, so it must not
			// load every integration. It opens the star chart (the recorded shell lives there).
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return d.prepareLight() },
			RunE: func(cmd *cobra.Command, args []string) error {
				return d.printShellScript(cmd.Context())
			},
		},
	)
	return cmd
}
