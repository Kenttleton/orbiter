package commands_test

import (
	"strings"
	"testing"

	integrations "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/commands"
)

func TestRenderCatalogChecklist_AllEntries(t *testing.T) {
	entries := []integrations.CatalogEntry{
		{Brand: "git", Name: "Git", Description: "Git version control", Roles: []string{"tool"}},
		{Brand: "go", Name: "Go Toolchain", Description: "Go runtime", Roles: []string{"runtime"}},
	}
	lines := commands.RenderCatalogChecklist(entries)
	if len(lines) != len(entries) {
		t.Fatalf("expected %d lines, got %d", len(entries), len(lines))
	}
	for i, line := range lines {
		if !strings.Contains(line, entries[i].Name) {
			t.Errorf("line %d missing name %q: %s", i, entries[i].Name, line)
		}
	}
}

func TestRenderCatalogChecklist_Empty(t *testing.T) {
	lines := commands.RenderCatalogChecklist(nil)
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for nil input, got %d", len(lines))
	}
}
