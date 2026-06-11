package commands

import (
	"github.com/Kenttleton/orbiter/internal/tui"
	"github.com/spf13/cobra"
)

func newStarChartCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "starchart",
		Short: "Open the interactive Star Chart TUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(d.sc)
		},
	}
}
