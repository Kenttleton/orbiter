package commands

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
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

// WriteInspectReport writes a human-readable inspection report to w.
func WriteInspectReport(w io.Writer, info IntegrationInspectResult) {
	fmt.Fprintf(w, "Integration: %s\n", info.Brand)
	if info.Quarantined {
		fmt.Fprintf(w, "Status:      QUARANTINED\n")
		fmt.Fprintf(w, "Reason:      %s\n", info.QuarantineReason)
		fmt.Fprintf(w, "Since:       %s\n", info.QuarantineAt.Format(time.RFC3339))
		fmt.Fprintf(w, "\nTo restore:  orbiter vessel unquarantine %s\n", info.Brand)
	} else if info.Registered {
		fmt.Fprintf(w, "Status:      active\n")
	} else {
		fmt.Fprintf(w, "Status:      not installed\n")
	}
}

func newVesselInitCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the vessel and install bundled integrations",
		Args:  cobra.NoArgs,
		// Override parent PersistentPreRunE — vessel init needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog := bundle.CatalogEntries()
			if err := bundle.InstallSelected(catalog, integrations.Default, nil); err != nil {
				return fmt.Errorf("install integrations: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %d integration(s)\n", len(catalog))
			return nil
		},
	}
}

func newVesselInspectCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <brand>",
		Short: "Show details for an installed integration",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			brand := args[0]
			info := IntegrationInspectInfo(brand, integrations.DefaultSettings, integrations.Default)
			WriteInspectReport(cmd.OutOrStdout(), info)
			return nil
		},
	}
}

func newVesselUnquarantineCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "unquarantine <brand>",
		Short: "Remove quarantine from an integration and re-enable it",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			brand := args[0]
			if !integrations.DefaultSettings.IsQuarantined(brand) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is not quarantined\n", brand)
				return nil
			}
			if err := integrations.Default.UnquarantineBrand(brand); err != nil {
				return fmt.Errorf("unquarantine %s: %w", brand, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is now active\n", brand)
			return nil
		},
	}
}
