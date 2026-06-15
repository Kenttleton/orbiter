package commands_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Kenttleton/orbiter/internal/commands"
)

func applyKey(m commands.ChecklistModel, s string) commands.ChecklistModel {
	var msg tea.Msg
	switch s {
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	updated, _ := m.Update(msg)
	return updated.(commands.ChecklistModel)
}

func TestChecklistModel_SpaceTogglesChecked(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "Item A", Tag: "a", Checked: false},
	})
	m = applyKey(m, " ")
	if !m.Items[0].Checked {
		t.Error("expected item to be checked after space")
	}
	m = applyKey(m, " ")
	if m.Items[0].Checked {
		t.Error("expected item to be unchecked after second space")
	}
}

func TestChecklistModel_Navigation(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a"},
		{Label: "B", Tag: "b"},
		{Label: "C", Tag: "c"},
	})
	if m.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", m.Cursor())
	}
	m = applyKey(m, "down")
	if m.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", m.Cursor())
	}
	m = applyKey(m, "down")
	if m.Cursor() != 2 {
		t.Errorf("cursor should be 2 after second down, got %d", m.Cursor())
	}
	// Down at last item stays
	m = applyKey(m, "down")
	if m.Cursor() != 2 {
		t.Errorf("cursor should stay at 2 at bottom, got %d", m.Cursor())
	}
	m = applyKey(m, "up")
	if m.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", m.Cursor())
	}
}

func TestChecklistModel_EnterSetsDone(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a", Checked: true},
	})
	if m.Done() {
		t.Fatal("done should be false before enter")
	}
	m = applyKey(m, "enter")
	if !m.Done() {
		t.Error("done should be true after enter")
	}
}

func TestChecklistModel_QuitDoesNotSetDone(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a"},
	})
	m = applyKey(m, "q")
	if m.Done() {
		t.Error("done should be false after q")
	}
}

func TestChecklistModel_Selected(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a", Checked: true},
		{Label: "B", Tag: "b", Checked: false},
		{Label: "C", Tag: "c", Checked: true},
	})
	selected := m.Selected()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}
	if selected[0].Tag != "a" || selected[1].Tag != "c" {
		t.Errorf("unexpected selected items: %v", selected)
	}
}

func TestChecklistModel_ViewContainsLabels(t *testing.T) {
	m := commands.NewChecklistModel("Pick one:", []commands.ChecklistItem{
		{Label: "Alpha", Tag: "a", Badge: "upgrade available"},
		{Label: "Beta", Tag: "b", Checked: true},
	})
	view := m.View()
	for _, want := range []string{"Alpha", "Beta", "upgrade available", "Pick one:"} {
		if !containsString(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
