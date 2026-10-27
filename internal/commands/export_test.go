package commands

import (
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

// NewRootCommandForTest returns a root Cobra command wired to the given StarChart,
// bypassing the file-system open in PersistentPreRunE. The renderer defaults to
// plain/non-trace so tests don't need to configure output.
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
		newUplinkCmd(d),
	)

	return root
}

// NewRootCommandForTestWithRenderer is NewRootCommandForTest with a caller-chosen
// renderer (for example a JSON renderer, whose prompts stay pending).
func NewRootCommandForTestWithRenderer(sc *starchart.StarChart, r output.Renderer) *cobra.Command {
	_, isJSON := r.(*output.JSONRenderer)
	d := &deps{sc: sc, renderer: r, jsonMode: isJSON}
	root := &cobra.Command{Use: "orbiter", SilenceUsage: true}
	root.AddCommand(newRelayCmd(d), newUplinkCmd(d), newCalibrateCmd(d))
	return root
}

// ShellScriptFor exposes shell selection for tests.
func ShellScriptFor(env map[string]string, self string) (string, error) {
	return shellScriptFor(env, self)
}
