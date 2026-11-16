package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
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

// IntegrationInspectResult holds the display data for vessel inspect. An integration is a
// folder in the integrations directory: Found says the folder is there, UsedBy how many
// resources and transponders in the tree use it.
type IntegrationInspectResult struct {
	Brand            string
	Found            bool
	Engine           string   // the engine the manifest declares; "" when there is no manifest
	Env              []string // the variables the manifest claims for its commands; nothing else is passed
	MemoryBytes      int64    // the guest memory limit the manifest declares (or the default); 0 when there is no manifest
	UsedBy           int
	Quarantined      bool
	QuarantineReason string
	QuarantineAt     time.Time
}

// IntegrationInspectInfo builds an IntegrationInspectResult for the given brand from the
// integrations directory, the settings and the number of users the star chart reports.
func IntegrationInspectInfo(brand string, settings *integrations.SettingsStore, dir string, usedBy int) IntegrationInspectResult {
	result := IntegrationInspectResult{Brand: brand, UsedBy: usedBy}
	if data, err := os.ReadFile(filepath.Join(dir, brand, "manifest.toml")); err == nil {
		result.Found = true
		var m integrations.Manifest
		if _, err := toml.Decode(string(data), &m); err == nil {
			result.Engine = m.Runtime.EngineName()
			result.MemoryBytes = m.Runtime.MemoryLimit()
			result.Env = m.Commands.Env
		}
	}
	if settings.IsQuarantined(brand) {
		entry := settings.QuarantineEntry(brand)
		result.Quarantined = true
		result.QuarantineReason = entry.Reason
		result.QuarantineAt = entry.At
	}
	return result
}

// inspectStatus is the one word for where an integration stands: quarantined, used (the tree
// uses it), idle (the folder is there and nothing uses it) or missing (no folder).
func inspectStatus(info IntegrationInspectResult) string {
	switch {
	case info.Quarantined:
		return "quarantined"
	case !info.Found:
		return "missing"
	case info.UsedBy > 0:
		return "used"
	default:
		return "idle"
	}
}

// inspectLines is the human-readable inspection report, one entry per line.
func inspectLines(info IntegrationInspectResult) []string {
	lines := []string{"Integration: " + info.Brand}
	switch inspectStatus(info) {
	case "quarantined":
		lines = append(lines,
			"Status:      QUARANTINED",
			"Reason:      "+info.QuarantineReason,
			"Since:       "+info.QuarantineAt.Format(time.RFC3339),
			"",
			"To restore:  orbiter unquarantine "+info.Brand)
	case "used":
		lines = append(lines, "Status:      used", fmt.Sprintf("Used by:     %d in the tree", info.UsedBy))
	case "idle":
		lines = append(lines, "Status:      idle")
	default:
		lines = append(lines, "Status:      missing")
	}
	if info.Engine != "" {
		lines = append(lines, "Engine:      "+info.Engine)
	}
	if info.MemoryBytes > 0 {
		lines = append(lines, fmt.Sprintf("Memory:      %d bytes", info.MemoryBytes))
	}
	if info.Found {
		claim := "(none)"
		if len(info.Env) > 0 {
			claim = strings.Join(info.Env, ", ")
		}
		lines = append(lines, "Environment: "+claim)
	}
	return lines
}

// inspectBrand reports on an integration: the folder, the settings and the star chart.
func inspectBrand(ctx context.Context, sc *starchart.StarChart, brand string) (IntegrationInspectResult, error) {
	used, err := sc.CountUsing(ctx, brand)
	if err != nil {
		return IntegrationInspectResult{}, err
	}
	return IntegrationInspectInfo(brand, integrations.DefaultSettings, bundle.DefaultIntegrationsDir(), used), nil
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
	res := InspectResult{Brand: info.Brand, Status: inspectStatus(info), UsedBy: info.UsedBy, Engine: info.Engine, MemoryBytes: info.MemoryBytes, Env: info.Env, Quarantined: info.Quarantined}
	if info.Quarantined {
		since := info.QuarantineAt
		res.Reason, res.Since = info.QuarantineReason, &since
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
		Short:             "Show details for an integration in the integrations folder",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return d.prepareLight() },
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := inspectBrand(cmd.Context(), d.sc, args[0])
			if err != nil {
				return err
			}
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
