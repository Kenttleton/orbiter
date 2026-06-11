package commands

import (
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newGalaxyCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "galaxy",
		Short: "Manage galaxies (organizations/clients)",
	}
	cmd.AddCommand(
		newGalaxyAddCmd(d),
		newGalaxyInitCmd(d),
	)
	return cmd
}

func newGalaxyAddCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Register a galaxy in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := d.sc.CreateGalaxy(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("galaxy %q registered (%s)", args[0], g.ID))
			return nil
		},
	}
}

func newGalaxyInitCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Register and initialize a galaxy, cascading to all children",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				if !errors.Is(err, starchart.ErrNotFound) {
					return err
				}
				g, err := d.sc.CreateGalaxy(ctx, args[0])
				if err != nil {
					return err
				}
				alias.ID = g.ID
			}
			if err := d.sc.InitGalaxy(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("galaxy %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
}
