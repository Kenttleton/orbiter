package commands

import (
	"github.com/spf13/cobra"
)

// --- Vessel Commands ---

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
