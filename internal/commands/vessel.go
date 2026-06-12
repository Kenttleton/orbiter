package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
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

func newVesselInitCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the vessel and install bundled integrations",
		Args:  cobra.NoArgs,
		// Override parent PersistentPreRunE — vessel init needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog := bundle.CatalogEntries()
			if err := bundle.InstallSelected(catalog, core.Default); err != nil {
				return fmt.Errorf("install integrations: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %d integration(s)\n", len(catalog))
			return nil
		},
	}
}
