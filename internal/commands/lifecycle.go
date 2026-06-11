package commands

import (
	"github.com/spf13/cobra"
)

func newScanCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "scan [target]",
		Short: "Verify reality — \"What does reality currently look like?\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return NewExecutor(d.sc, d.renderer).Scan(cmd.Context(), target)
		},
	}
}

func newSurveyCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "survey [target]",
		Short: "Inspect metadata — \"What is this thing?\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return NewExecutor(d.sc, d.renderer).Survey(cmd.Context(), target)
		},
	}
}
