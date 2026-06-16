package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newTransponderCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transponder",
		Short: "Manage transponders (credential pointers)",
	}
	cmd.AddCommand(
		newTransponderAddCmd(d),
		newTransponderInitCmd(d),
	)
	return cmd
}

func newTransponderAddCmd(d *deps) *cobra.Command {
	var role, brand, location string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a transponder in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgBytes, err := json.Marshal(map[string]string{"location": location})
			if err != nil {
				return fmt.Errorf("build transponder config: %w", err)
			}
			config := string(cfgBytes)
			tp, err := d.sc.CreateTransponder(cmd.Context(), args[0], role, brand, config)
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("transponder %q registered (%s/%s) (%s)", args[0], role, brand, tp.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "access mechanism: file | env | keychain | vault | agent")
	cmd.Flags().StringVar(&brand, "brand", "", "service brand (any string — validated by integration at init time)")
	cmd.Flags().StringVar(&location, "location", "", "credential location: file path, env var name, vault path, etc.")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("brand")
	_ = cmd.MarkFlagRequired("location")
	return cmd
}

func newTransponderInitCmd(d *deps) *cobra.Command {
	var role, brand, location string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Register and provision a transponder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				if !errors.Is(err, starchart.ErrNotFound) {
					return err
				}
				cfgBytes, err := json.Marshal(map[string]string{"location": location})
				if err != nil {
					return fmt.Errorf("build transponder config: %w", err)
				}
				config := string(cfgBytes)
				tp, err := d.sc.CreateTransponder(ctx, args[0], role, brand, config)
				if err != nil {
					return err
				}
				alias.ID = tp.ID
			}
			if err := d.sc.InitTransponder(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("transponder %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "access mechanism (required when creating)")
	cmd.Flags().StringVar(&brand, "brand", "", "service brand (required when creating)")
	cmd.Flags().StringVar(&location, "location", "", "credential location (required when creating)")
	return cmd
}
