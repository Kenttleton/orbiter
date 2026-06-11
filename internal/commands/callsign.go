package commands

import (
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newCallsignCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "callsign",
		Short: "Manage callsigns (identities)",
	}
	cmd.AddCommand(
		newCallsignAddCmd(d),
		newCallsignInitCmd(d),
	)
	return cmd
}

func newCallsignAddCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Register a callsign in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs, err := d.sc.CreateCallsign(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("callsign %q registered (%s)", args[0], cs.ID))
			return nil
		},
	}
}

func newCallsignInitCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Register and initialize a callsign, cascading to attached transponders",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				if !errors.Is(err, starchart.ErrNotFound) {
					return err
				}
				cs, err := d.sc.CreateCallsign(ctx, args[0])
				if err != nil {
					return err
				}
				alias.ID = cs.ID
			}
			if err := d.sc.InitCallsign(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("callsign %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
}
