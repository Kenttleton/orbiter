package commands

import (
	"github.com/spf13/cobra"
)

// --- Six Commands ---

func newSurveyCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "survey [target]",
		Short: "Inspect metadata — \"What is this thing?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("survey: not yet implemented")
			return nil
		},
	}
}

func newChartCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "chart [target]",
		Short: "Preview a transition — \"What would happen if I went there?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("chart: not yet implemented")
			return nil
		},
	}
}

func newJumpCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "jump [target]",
		Short: "Execute a transition — \"Take me there.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("jump: not yet implemented")
			return nil
		},
	}
}

func newScanCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "scan [target]",
		Short: "Verify reality — \"What does reality currently look like?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("scan: not yet implemented")
			return nil
		},
	}
}

func newCalibrateCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "calibrate [target]",
		Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("calibrate: not yet implemented")
			return nil
		},
	}
}

func newRetroCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "retro [target]",
		Short: "Retire obsolete entities — \"Remove what no longer belongs in the universe.\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("retro: not yet implemented")
			return nil
		},
	}
}

// --- CRUD Commands ---

func newGalaxyCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "galaxy",
		Short: "Manage galaxies (organizations/clients)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("galaxy add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("galaxy edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("galaxy remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newSystemCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage solar systems (team subdivisions)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a solar system", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("system add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a solar system", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("system edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a solar system", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("system remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newPlanetCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "planet",
		Short: "Manage planets (projects)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a planet", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("planet add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "init", Short: "Initialize a planet from the current directory", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("planet init: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a planet", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("planet edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a planet", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("planet remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newCallsignCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "callsign",
		Short: "Manage callsigns (identities)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a callsign", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("callsign add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a callsign", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("callsign edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a callsign", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("callsign remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newTransponderCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transponder",
		Short: "Manage transponders (credential pointers)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a transponder", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("transponder add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a transponder", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("transponder edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a transponder", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("transponder remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newResourceCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Manage resources (tooling, runtimes, capabilities)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a resource", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("resource add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a resource", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("resource edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a resource", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("resource remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newVesselCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vessel",
		Short: "Manage the vessel (this workstation)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "survey", Short: "Show vessel configuration", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("vessel survey: not yet implemented")
			return nil
		}},
		newVesselDefaultsCmd(d),
		newVesselHistoryCmd(d),
	)
	return cmd
}

func newVesselDefaultsCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Manage vessel-level defaults",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add", Short: "Add a default", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("vessel defaults add: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "edit", Short: "Edit a default", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("vessel defaults edit: not yet implemented")
			return nil
		}},
		&cobra.Command{Use: "remove", Short: "Remove a default", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("vessel defaults remove: not yet implemented")
			return nil
		}},
	)
	return cmd
}

func newVesselHistoryCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage navigation history",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "clean", Short: "Remove history older than retention period", RunE: func(cmd *cobra.Command, args []string) error {
			d.renderer.Info("vessel history clean: not yet implemented")
			return nil
		}},
	)
	return cmd
}
