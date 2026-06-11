package commands

import (
	"github.com/spf13/cobra"
)

// --- Six Lifecycle Commands ---

func newChartCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "chart [target]",
		Short: "Preview a transition — \"What would happen if I went there?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("chart: not yet implemented")
			return nil
		},
	}
}

func newJumpCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "jump [target]",
		Short: "Execute a transition — \"Take me there.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("jump: not yet implemented")
			return nil
		},
	}
}

func newScanCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "scan [target]",
		Short: "Verify reality — \"What does reality currently look like?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("scan: not yet implemented")
			return nil
		},
	}
}

func newCalibrateCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "calibrate [target]",
		Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("calibrate: not yet implemented")
			return nil
		},
	}
}

func newRetroCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "retro [target]",
		Short: "Retire obsolete entities — \"Remove what no longer belongs in the universe.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("retro: not yet implemented")
			return nil
		},
	}
}

// --- Vessel Commands ---

func newVesselCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vessel",
		Short: "Manage the vessel (this workstation)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "survey",
			Short: "Show vessel configuration",
			RunE: func(cmd *cobra.Command, args []string) error {
				d.renderer.Info("vessel survey: not yet implemented")
				return nil
			},
		},
		newVesselDefaultsCmd(d),
		newVesselHistoryCmd(d),
	)
	return cmd
}

func newVesselDefaultsCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Manage vessel-level defaults",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "add",
			Short: "Add a default",
			RunE: func(cmd *cobra.Command, args []string) error {
				d.renderer.Info("vessel defaults add: not yet implemented")
				return nil
			},
		},
	)
	return cmd
}

func newVesselHistoryCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage navigation history",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "clean",
			Short: "Remove history older than retention period",
			RunE: func(cmd *cobra.Command, args []string) error {
				d.renderer.Info("vessel history clean: not yet implemented")
				return nil
			},
		},
	)
	return cmd
}
