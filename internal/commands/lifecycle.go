package commands

import (
	"fmt"
	"os"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/Kenttleton/orbiter/internal/tui"
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

func newRetroCmd(d *deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "retro [target]",
		Short: "Retire an entity — \"Remove what no longer belongs.\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return NewExecutor(d.sc, d.renderer).Retro(cmd.Context(), target, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func newHookCmd(d *deps) *cobra.Command {
	var cwd, current string
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  "Emit context directives for shell hook (called automatically on cd)",
		Hidden: true,
		Args:   cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			chartPath := os.Getenv("ORBITER_STARCHART")
			if chartPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return nil
				}
				chartPath = home + "/.orbiter/starchart.db"
			}
			sc, err := starchart.Open(chartPath)
			if err != nil {
				return nil // not initialized; hook silently does nothing
			}
			d.sc = sc
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if d.sc == nil {
				return nil
			}
			if cwd == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return nil
				}
			}
			exec := NewExecutor(d.sc, d.renderer)
			directives, err := exec.Hook(ctx, cwd, current)
			if err != nil {
				return err
			}
			for _, dir := range directives {
				fmt.Println(dir.String())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "current working directory")
	cmd.Flags().StringVar(&current, "current", "", "currently active planet ID")
	return cmd
}

func newJumpCmd(d *deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "jump [target]",
		Short: "Execute a transition — \"Take me there.\"",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			if target == "starchart" {
				return tui.Run(d.sc)
			}
			exec := NewExecutor(d.sc, d.renderer)
			directives, err := exec.Jump(cmd.Context(), target, yes)
			if err != nil {
				return err
			}
			for _, dir := range directives {
				fmt.Println(dir.String())
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}
