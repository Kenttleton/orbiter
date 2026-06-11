package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSystemCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage solar systems (team subdivisions)",
	}
	cmd.AddCommand(
		newSystemAddCmd(d),
		newSystemInitCmd(d),
	)
	return cmd
}

func newSystemAddCmd(d *deps) *cobra.Command {
	var galaxy string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a solar system in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			galAlias, err := d.sc.Resolve(cmd.Context(), galaxy)
			if err != nil {
				return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
			}
			sys, err := d.sc.CreateSolarSystem(cmd.Context(), args[0], galAlias.ID)
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("system %q registered under %q (%s)", args[0], galaxy, sys.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this system belongs to")
	_ = cmd.MarkFlagRequired("galaxy")
	return cmd
}

func newSystemInitCmd(d *deps) *cobra.Command {
	var galaxy string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Register and initialize a system, cascading to all planets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				galAlias, err := d.sc.Resolve(ctx, galaxy)
				if err != nil {
					return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
				}
				sys, err := d.sc.CreateSolarSystem(ctx, args[0], galAlias.ID)
				if err != nil {
					return err
				}
				alias.ID = sys.ID
			}
			if err := d.sc.InitSolarSystem(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("system %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
	cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this system belongs to (required when creating)")
	return cmd
}
