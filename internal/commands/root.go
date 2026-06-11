package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/resolver"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// deps holds the dependency-injected resources available to all commands.
type deps struct {
	sc       *starchart.StarChart
	renderer output.Renderer
	resolver resolver.Resolver
}

// NewRootCommand builds and returns the orbiter root Cobra command with DI wiring.
func NewRootCommand() *cobra.Command {
	var outputFormat string
	var verbose bool

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

			// Resolve verbose: flag > env.
			if !verbose {
				verbose = os.Getenv("ORBITER_VERBOSE") == "1"
			}

			d.renderer = output.NewRenderer(format, verbose)
			d.resolver = resolver.New(sc)
			return nil
		},
	}

	root.PersistentFlags().StringVar(&outputFormat, "output", "", "output format: styled (default) or json")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output: plain labels and full tool output")

	// Register all subcommands.
	root.AddCommand(
		newInitCmd(),
		newSurveyCmd(&d),
		newChartCmd(&d),
		newJumpCmd(&d),
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
		newStarChartCmd(&d),
		newAttachCmd(&d),
		newCompletionsCmd(root),
	)

	return root
}
