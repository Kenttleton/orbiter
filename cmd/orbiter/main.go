package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Kenttleton/orbiter/internal/tui"
)

func main() {
	runner := tui.NewRunner("")
	if _, err := runner.Run(context.Background(), "version"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: orbit not found in PATH — TUI read operations will fail\n")
	}

	model := tui.NewUniverseModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "orbiter:", err)
		os.Exit(1)
	}
}
