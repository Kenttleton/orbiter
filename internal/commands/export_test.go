package commands

import (
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

// NewRootCommandForTest returns a root Cobra command wired to the given StarChart,
// bypassing the file-system open in PersistentPreRunE. The renderer defaults to
// plain/non-verbose so tests don't need to configure output.
func NewRootCommandForTest(sc *starchart.StarChart) *cobra.Command {
	d := &deps{
		sc:       sc,
		renderer: output.NewRenderer(output.FormatStyled, false),
	}

	root := &cobra.Command{
		Use:          "orbiter",
		SilenceUsage: true,
	}

	root.AddCommand(
		newRelayCmd(d),
	)

	return root
}
