package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAttachCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "attach <from> <to>",
		Short: "Wire two entities together in the graph",
		Long: `attach creates a directed edge in the Star Chart graph.

<from> is the child entity, <to> is the parent. "vessel" is a reserved
target meaning the global vessel (available everywhere, all contexts).

Examples:
  orbiter attach work-github  work-dev         # transponder → callsign
  orbiter attach work-dev     freelance-work   # callsign → galaxy
  orbiter attach node-version-mgr  vessel     # resource → vessel (global)
  orbiter attach node-version-mgr  payments-api  # resource → planet (scoped)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			att, err := d.sc.Attach(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			d.renderer.Success(fmt.Sprintf("%q → %q attached (%s)", args[0], args[1], att.ID))
			return nil
		},
	}
}
