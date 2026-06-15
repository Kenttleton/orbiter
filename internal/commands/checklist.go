package commands

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ChecklistItem is one row in the checklist.
// Tag is an opaque identifier (brand name) used by callers; it is never displayed.
// Badge is optional text shown after the label (e.g. "upgrade available").
type ChecklistItem struct {
	Label   string
	Tag     string
	Checked bool
	Badge   string
}

// ChecklistModel is a bubbletea model for an interactive multi-select checklist.
// It is designed to be relocatable: no commands-package types in its interface.
// Use NewChecklistModel to construct; call tea.NewProgram(m).Run() to drive it.
type ChecklistModel struct {
	Title  string
	Items  []ChecklistItem
	cursor int
	done   bool
}

// NewChecklistModel returns a ChecklistModel with the given title and items.
func NewChecklistModel(title string, items []ChecklistItem) ChecklistModel {
	return ChecklistModel{Title: title, Items: items}
}

// Cursor returns the current cursor position (0-indexed).
func (m ChecklistModel) Cursor() int { return m.cursor }

// Done returns true if the user confirmed the selection with Enter.
// Done is false when the user cancelled with q or ctrl+c.
func (m ChecklistModel) Done() bool { return m.done }

// Selected returns the subset of Items where Checked is true.
func (m ChecklistModel) Selected() []ChecklistItem {
	var out []ChecklistItem
	for _, item := range m.Items {
		if item.Checked {
			out = append(out, item)
		}
	}
	return out
}

func (m ChecklistModel) Init() tea.Cmd { return nil }

func (m ChecklistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case " ":
			if len(m.Items) > 0 {
				m.Items[m.cursor].Checked = !m.Items[m.cursor].Checked
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ChecklistModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.Title + "\n\n")
	for i, item := range m.Items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		checked := "[ ]"
		if item.Checked {
			checked = "[x]"
		}
		badge := ""
		if item.Badge != "" {
			badge = fmt.Sprintf(" (%s)", item.Badge)
		}
		fmt.Fprintf(&sb, "%s%s %s%s\n", cursor, checked, item.Label, badge)
	}
	sb.WriteString("\nup/down move  space toggle  enter confirm  q cancel\n")
	return sb.String()
}
