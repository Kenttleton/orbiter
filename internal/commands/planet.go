package commands

import (
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newPlanetCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "planet",
		Short: "Manage planets (projects)",
	}
	cmd.AddCommand(
		newPlanetAddCmd(d),
		newPlanetInitCmd(d),
	)
	return cmd
}

func newPlanetAddCmd(d *deps) *cobra.Command {
	var galaxy, system string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a planet in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			galAlias, err := d.sc.Resolve(ctx, galaxy)
			if err != nil {
				return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
			}
			var sysID string
			if system != "" {
				sysAlias, err := d.sc.Resolve(ctx, system)
				if err != nil {
					return fmt.Errorf("system %q not found: %w", system, err)
				}
				sysID = sysAlias.ID
			}
			p, err := d.sc.CreatePlanet(ctx, args[0], galAlias.ID, sysID)
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("planet %q registered under %q (%s)", args[0], galaxy, p.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this planet belongs to")
	cmd.Flags().StringVar(&system, "system", "", "solar system this planet belongs to (optional)")
	_ = cmd.MarkFlagRequired("galaxy")
	return cmd
}

func newPlanetInitCmd(d *deps) *cobra.Command {
	var galaxy, system string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Register and initialize a planet, cascading to all attached resources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				if !errors.Is(err, starchart.ErrNotFound) {
					return err
				}
				galAlias, err := d.sc.Resolve(ctx, galaxy)
				if err != nil {
					return fmt.Errorf("galaxy %q not found: %w", galaxy, err)
				}
				var sysID string
				if system != "" {
					sysAlias, err := d.sc.Resolve(ctx, system)
					if err != nil {
						return fmt.Errorf("system %q not found: %w", system, err)
					}
					sysID = sysAlias.ID
				}
				p, err := d.sc.CreatePlanet(ctx, args[0], galAlias.ID, sysID)
				if err != nil {
					return err
				}
				alias.ID = p.ID
			}
			if err := d.sc.InitPlanet(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("planet %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
	cmd.Flags().StringVar(&galaxy, "galaxy", "", "galaxy this planet belongs to (required when creating)")
	cmd.Flags().StringVar(&system, "system", "", "solar system this planet belongs to (optional)")
	return cmd
}
