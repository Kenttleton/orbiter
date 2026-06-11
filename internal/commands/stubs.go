package commands

import (
	"github.com/spf13/cobra"
)

// --- Six Lifecycle Commands ---

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
