package commands

import (
	"context"

	"github.com/Kenttleton/orbiter/internal/integrations"
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

// ShellInit is the exported face of shellInit for tests.
type ShellInit struct{ s shellInit }

func NewShellInit(sc *starchart.StarChart, reg *integrations.Registry, dir string, env map[string]string, self string, load func() error) ShellInit {
	return ShellInit{shellInit{sc: sc, reg: reg, dir: dir, env: env, self: self, load: load}}
}

func (i ShellInit) Script(ctx context.Context) (string, error) { return i.s.script(ctx) }
