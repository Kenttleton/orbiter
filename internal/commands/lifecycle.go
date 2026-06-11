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

func newChartCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "chart [target]",
		Short: "Preview a transition — \"What would happen if I went there?\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return NewExecutor(d.sc, d.renderer).Chart(cmd.Context(), target)
		},
	}
}

func newCalibrateCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "calibrate [target]",
		Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return NewExecutor(d.sc, d.renderer).Calibrate(cmd.Context(), target)
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
