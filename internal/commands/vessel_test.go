package commands_test

import (
	"path/filepath"
	"strings"
	"testing"

	bundle "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/commands"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)

func TestRenderCatalogChecklist_AllEntries(t *testing.T) {
	entries := []bundle.CatalogEntry{
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

func TestIntegrationInspectInfo_Quarantined(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	_ = settings.Quarantine("git", "attempted banned command: bash")
	reg := integrations.NewRegistry(settings)

	info := commands.IntegrationInspectInfo("git", settings, reg)
	if !info.Quarantined {
		t.Error("expected Quarantined = true")
	}
	if info.QuarantineReason == "" {
		t.Error("expected non-empty QuarantineReason")
	}
}

func TestIntegrationInspectInfo_NotQuarantined(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	reg := integrations.NewRegistry(settings)

	info := commands.IntegrationInspectInfo("git", settings, reg)
	if info.Quarantined {
		t.Error("expected Quarantined = false")
	}
}
