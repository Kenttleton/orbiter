package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newResourceCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources (tooling, runtimes, capabilities)",
	}
	cmd.AddCommand(
		newResourceAddCmd(d),
		newResourceInitCmd(d),
	)
	return cmd
}

func newResourceAddCmd(d *deps) *cobra.Command {
	var role, brand, manages, config string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a resource in the Star Chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if manages == "" {
				manages = "[]"
			}
			if config == "" {
				config = "{}"
			}
			r, err := d.sc.CreateResource(cmd.Context(), args[0], role, brand, manages, config)
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("resource %q registered (%s/%s) (%s)", args[0], role, brand, r.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "resource role: manager | runtime | tool | remote | filesystem")
	cmd.Flags().StringVar(&brand, "brand", "", "resource brand (any string — validated by integration at init time)")
	cmd.Flags().StringVar(&manages, "manages", "", `JSON array of brands this manager controls, e.g. '["node","npm"]'`)
	cmd.Flags().StringVar(&config, "config", "", `JSON object of resource configuration`)
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("brand")
	return cmd
}

func newResourceInitCmd(d *deps) *cobra.Command {
	var role, brand, manages, config string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Register and provision a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if manages == "" {
				manages = "[]"
			}
			if config == "" {
				config = "{}"
			}
			alias, err := d.sc.Resolve(ctx, args[0])
			if err != nil {
				r, err := d.sc.CreateResource(ctx, args[0], role, brand, manages, config)
				if err != nil {
					return err
				}
				alias.ID = r.ID
			}
			if err := d.sc.InitResource(ctx, alias.ID); err != nil {
				return err
			}
			beacon, _ := d.sc.GetBeacon(ctx, alias.ID)
			d.renderer.Success(fmt.Sprintf("resource %q initialized — status: %s", args[0], beacon.Status))
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "resource role (required when creating)")
	cmd.Flags().StringVar(&brand, "brand", "", "resource brand (required when creating)")
	cmd.Flags().StringVar(&manages, "manages", "", "JSON array of brands this manager controls")
	cmd.Flags().StringVar(&config, "config", "", "JSON object of resource configuration")
	return cmd
}
