package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)

// newQuarantineCmd adds an integration brand to quarantine.
func newQuarantineCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "quarantine <brand>",
		Short: "Quarantine an integration brand and disable it",
		Args:  cobra.ExactArgs(1),
		// No star chart needed, but output (including JSON) still is.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { d.setupOutput(); return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			brand := args[0]
			if integrations.Default.IsQuarantined(brand) {
				d.renderer.Info(fmt.Sprintf("%s is already quarantined", brand))
				d.done("already quarantined", QuarantineResult{Brand: brand, Quarantined: true})
				return nil
			}
			if err := integrations.Default.QuarantineBrand(brand, "manual quarantine"); err != nil {
				return fmt.Errorf("quarantine %s: %w", brand, err)
			}
			d.renderer.Info(fmt.Sprintf("%s is now quarantined", brand))
			d.done("quarantined", QuarantineResult{Brand: brand, Quarantined: true, Changed: true})
			return nil
		},
	}
}

// newUnquarantineCmd removes quarantine from an integration brand.
func newUnquarantineCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "unquarantine <brand>",
		Short: "Remove quarantine from an integration brand and re-enable it",
		Args:  cobra.ExactArgs(1),
		// No star chart needed, but output (including JSON) still is.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { d.setupOutput(); return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return unquarantineBrand(d, args[0])
		},
	}
}

// unquarantineBrand is shared by unquarantine and vessel unquarantine.
func unquarantineBrand(d *deps, brand string) error {
	if !integrations.DefaultSettings.IsQuarantined(brand) {
		d.renderer.Info(fmt.Sprintf("%s is not quarantined", brand))
		d.done("not quarantined", QuarantineResult{Brand: brand})
		return nil
	}
	if err := integrations.Default.UnquarantineBrand(brand); err != nil {
		return fmt.Errorf("unquarantine %s: %w", brand, err)
	}
	d.renderer.Info(fmt.Sprintf("%s quarantine removed — run 'orbiter init' to reload", brand))
	d.done("quarantine removed", QuarantineResult{Brand: brand, Changed: true})
	return nil
}
