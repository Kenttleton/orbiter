package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// UniverseModel is the Bubble Tea model for the universe view.
// Phase 1: stub — displays a placeholder message.
type UniverseModel struct{}

func NewUniverseModel() UniverseModel {
	return UniverseModel{}
}

func (m UniverseModel) Init() tea.Cmd {
	return nil
}

func (m UniverseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m UniverseModel) View() string {
	return "orbiter universe view — not yet implemented\n\nPress any key to exit."
}

// BeaconModel is the Bubble Tea model for the beacon view.
// Phase 1: stub — displays a placeholder message.
type BeaconModel struct{}

func NewBeaconModel() BeaconModel {
	return BeaconModel{}
}

func (m BeaconModel) Init() tea.Cmd {
	return nil
}

func (m BeaconModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m BeaconModel) View() string {
	return "orbiter beacon view — not yet implemented\n\nPress any key to exit."
}
