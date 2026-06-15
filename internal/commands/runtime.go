package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)

// newQuarantineCmd adds an integration brand to quarantine.
func newQuarantineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quarantine <brand>",
		Short: "Quarantine an integration brand and disable it",
		Args:  cobra.ExactArgs(1),
		// No star chart needed.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			brand := args[0]
			if integrations.Default.IsQuarantined(brand) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is already quarantined\n", brand)
				return nil
			}
			if err := integrations.Default.QuarantineBrand(brand, "manual quarantine"); err != nil {
				return fmt.Errorf("quarantine %s: %w", brand, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is now quarantined\n", brand)
			return nil
		},
	}
}

// newUnquarantineCmd removes quarantine from an integration brand.
func newUnquarantineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unquarantine <brand>",
		Short: "Remove quarantine from an integration brand and re-enable it",
		Args:  cobra.ExactArgs(1),
		// No star chart needed.
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
			fmt.Fprintf(cmd.OutOrStdout(), "%s quarantine removed — run 'orbiter init' to reload\n", brand)
			return nil
		},
	}
}

// newHistoryCmd returns the history subcommand group.
//
//	orbiter history clean   → remove navigation history older than retention period
func newHistoryCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage orbiter navigation history",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "clean",
			Short: "Remove history older than retention period",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				d.renderer.Info("history clean: not yet implemented")
				return nil
			},
		},
	)
	return cmd
}
