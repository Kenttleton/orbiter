package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/resolver"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// deps holds the dependency-injected resources available to all commands.
type deps struct {
	sc       *starchart.StarChart
	renderer output.Renderer
	resolver resolver.Resolver
	// verbose is the root -v flag: data volume. Read commands show more detail.
	// Process transparency is --trace, owned by the renderer (Renderer.IsTrace).
	verbose bool
	// jsonMode is set once the output format is resolved (PersistentPreRunE).
	jsonMode bool
}

// NewRootCommand builds and returns the orbiter root Cobra command with DI wiring.
func NewRootCommand() *cobra.Command {
	root, _ := newRoot()
	return root
}

// Execute runs the orbiter CLI and returns the process exit code. A failing command
// is reported in the active output format: an "error" event on stdout in JSON mode
// (stderr stays empty), plain text on stderr otherwise. Exit code is 1 on any error.
func Execute() int {
	root, d := newRoot()
	err := root.Execute()
	if err == nil {
		return 0
	}
	if d.renderer != nil && d.jsonMode {
		d.renderer.Error(err.Error())
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	return 1
}

func newRoot() (*cobra.Command, *deps) {
	var outputFormat string
	var trace bool

	var d deps

	root := &cobra.Command{
		Use:   "orbiter",
		Short: "Orbiter CLI — navigate and orchestrate your development universe",
		Long: `orbiter is the command interface for Orbiter, a state-driven navigation
and environment orchestration platform for freelance and contract engineers.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Resolve Star Chart path.
			chartPath := os.Getenv("ORBITER_STARCHART")
			if chartPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home directory: %w", err)
				}
				chartPath = home + "/.orbiter/starchart.db"
			}

			// Open Star Chart.
			sc, err := starchart.Open(chartPath)
			if err != nil {
				return fmt.Errorf("open star chart: %w", err)
			}
			d.sc = sc

			// Resolve output format: flag > env > default.
			format := outputFormat
			if format == "" {
				format = os.Getenv("ORBITER_OUTPUT")
			}
			if format == "" {
				format = output.FormatStyled
			}

			// Resolve verbose (data volume): flag > env.
			if !d.verbose {
				d.verbose = os.Getenv("ORBITER_VERBOSE") == "1"
			}
			// Resolve trace (process transparency): flag > env.
			if !trace {
				trace = os.Getenv("ORBITER_TRACE") == "1"
			}

			d.jsonMode = format == output.FormatJSON
			d.renderer = output.NewRenderer(format, trace)
			d.resolver = resolver.New(sc)

			// Load integrations installed to disk into the Default registry.
			// Non-fatal: a missing integrations dir is silently skipped (LoadInstalled handles it).
			if err := bundle.LoadInstalled(
				bundle.DefaultIntegrationsDir(),
				integrations.Default,
				wasm.RendererApproveFunc(d.renderer),
			); err != nil {
				fmt.Fprintf(os.Stderr, "orbiter: load integrations: %v\n", err)
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&outputFormat, "output", "", "output format: styled (default) or json")
	root.PersistentFlags().BoolVarP(&d.verbose, "verbose", "v", false, "show more detail (data volume); ORBITER_VERBOSE=1")
	root.PersistentFlags().BoolVarP(&trace, "trace", "t", false, "show the underlying process: plain labels and raw tool output; ORBITER_TRACE=1")

	// Register all subcommands.
	root.AddCommand(
		newInitCmd(),
		newShellCmd(),
		newSurveyCmd(&d),
		newChartCmd(&d),
		newJumpCmd(&d),
		newHookCmd(&d),
		newScanCmd(&d),
		newCalibrateCmd(&d),
		newRetroCmd(&d),
		newGalaxyCmd(&d),
		newSystemCmd(&d),
		newPlanetCmd(&d),
		newCallsignCmd(&d),
		newTransponderCmd(&d),
		newResourceCmd(&d),
		newVesselCmd(&d),
		newQuarantineCmd(),
		newUnquarantineCmd(),
		newStarChartCmd(&d),
		newAttachCmd(&d),
		newRelayCmd(&d),
		newUplinkCmd(&d),
		newCompletionsCmd(root),
	)

	return root, &d
}
