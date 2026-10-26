package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
)

// RenderCatalogChecklist returns one display line per catalog entry, suitable
// for a terminal checklist prompt. Each line is: "Name — Description (roles: role1, role2)"
func RenderCatalogChecklist(entries []bundle.CatalogEntry) []string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		line := fmt.Sprintf("%s — %s (roles: %s)", e.Name, e.Description, strings.Join(e.Roles, ", "))
		lines = append(lines, line)
	}
	return lines
}

// IntegrationInspectResult holds the display data for vessel inspect.
type IntegrationInspectResult struct {
	Brand            string
	Quarantined      bool
	QuarantineReason string
	QuarantineAt     time.Time
	Registered       bool // true if at least one role is registered in the registry
}

// IntegrationInspectInfo builds an IntegrationInspectResult for the given brand.
func IntegrationInspectInfo(brand string, settings *integrations.SettingsStore, registry *integrations.Registry) IntegrationInspectResult {
	result := IntegrationInspectResult{Brand: brand}
	if settings.IsQuarantined(brand) {
		entry := settings.QuarantineEntry(brand)
		result.Quarantined = true
		result.QuarantineReason = entry.Reason
		result.QuarantineAt = entry.At
	}
	// Check if registered for any role by checking all catalog entries.
	catalog := bundle.CatalogEntries()
	for _, e := range catalog {
		if e.Brand == brand {
			for _, role := range e.Roles {
				if _, ok := registry.Get(role, brand); ok {
					result.Registered = true
					break
				}
			}
			break
		}
	}
	return result
}

// inspectLines is the human-readable inspection report, one entry per line.
func inspectLines(info IntegrationInspectResult) []string {
	lines := []string{"Integration: " + info.Brand}
	switch {
	case info.Quarantined:
		lines = append(lines,
			"Status:      QUARANTINED",
			"Reason:      "+info.QuarantineReason,
			"Since:       "+info.QuarantineAt.Format(time.RFC3339),
			"",
			"To restore:  orbiter unquarantine "+info.Brand)
	case info.Registered:
		lines = append(lines, "Status:      active")
	default:
		lines = append(lines, "Status:      not installed")
	}
	return lines
}

// WriteInspectReport writes a human-readable inspection report to w.
func WriteInspectReport(w io.Writer, info IntegrationInspectResult) {
	for _, l := range inspectLines(info) {
		fmt.Fprintln(w, l)
	}
}

// reportInspect renders an inspection through the renderer: styled mode prints the
// report, JSON mode emits one result.
func reportInspect(r output.Renderer, info IntegrationInspectResult) {
	for _, l := range inspectLines(info) {
		r.Info(l)
	}
	res := InspectResult{Brand: info.Brand, Quarantined: info.Quarantined}
	switch {
	case info.Quarantined:
		since := info.QuarantineAt
		res.Status, res.Reason, res.Since = "quarantined", info.QuarantineReason, &since
	case info.Registered:
		res.Status = "active"
	default:
		res.Status = "not_installed"
	}
	r.Result(output.Result{Success: true, Message: "inspect " + info.Brand, Data: res})
}

// BuildChecklistItems converts CatalogEntryState slice into ChecklistItem slice
// for vessel init. Pre-checks installed entries and adds "upgrade available" badges
// for entries whose installed WASM differs from the bundled version.
func BuildChecklistItems(states []bundle.CatalogEntryState) []ChecklistItem {
	items := make([]ChecklistItem, len(states))
	for i, s := range states {
		badge := ""
		if s.Installed && !s.ChecksumMatches {
			badge = "upgrade available"
		}
		items[i] = ChecklistItem{
			Label:   fmt.Sprintf("%s — %s (roles: %s)", s.Name, s.Description, strings.Join(s.Roles, ", ")),
			Tag:     s.Brand,
			Checked: s.Installed,
			Badge:   badge,
		}
	}
	return items
}

// ApplySelections extracts selected integrations to dir and removes directories
// for integrations that were previously installed but are no longer selected.
func ApplySelections(states []bundle.CatalogEntryState, selected []ChecklistItem, dir string) error {
	selectedBrands := make(map[string]bool, len(selected))
	for _, item := range selected {
		selectedBrands[item.Tag] = true
	}

	// Remove deselected integrations that were previously installed.
	for _, s := range states {
		if s.Installed && !selectedBrands[s.Brand] {
			if err := os.RemoveAll(filepath.Join(dir, s.Brand)); err != nil {
				return fmt.Errorf("remove %s: %w", s.Brand, err)
			}
		}
	}

	// Extract selected entries.
	var toExtract []bundle.CatalogEntry
	for _, s := range states {
		if selectedBrands[s.Brand] {
			toExtract = append(toExtract, s.CatalogEntry)
		}
	}
	return bundle.ExtractSelected(toExtract, dir)
}

// newVesselCmd is the parent of the vessel subcommands (this workstation).
func newVesselCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vessel",
		Short: "Manage the vessel (this workstation)",
	}
	cmd.AddCommand(
		newVesselInitCmd(d),
		newVesselInspectCmd(d),
		// alias: orbiter vessel unquarantine → orbiter unquarantine
		newVesselUnquarantineCmd(d),
	)
	return cmd
}

func newVesselInitCmd(d *deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the vessel and install bundled integrations",
		Args:  cobra.NoArgs,
		// Override parent PersistentPreRunE — vessel init needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { d.setupOutput(); return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if d.jsonMode {
				out = io.Discard // JSON mode: the result is the only output
			}
			res, err := vesselInitRun(out, yes)
			if err != nil {
				return err
			}
			d.done("vessel initialized", res)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install all integrations without interactive selection")
	return cmd
}

func newVesselInspectCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:               "inspect <brand>",
		Short:             "Show details for an installed integration",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { d.setupOutput(); return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			info := IntegrationInspectInfo(args[0], integrations.DefaultSettings, integrations.Default)
			reportInspect(d.renderer, info)
			return nil
		},
	}
}

func newVesselUnquarantineCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:               "unquarantine <brand>",
		Short:             "Remove quarantine from an integration and re-enable it",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { d.setupOutput(); return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return unquarantineBrand(d, args[0])
		},
	}
}
