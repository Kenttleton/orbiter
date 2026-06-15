package commands_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	bundle "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/commands"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)

// fakeIntegration satisfies integrations.Integration for test registration.
type fakeIntegration struct{ manifest integrations.Manifest }

func (f fakeIntegration) Meta() integrations.Manifest                                { return f.manifest }
func (f fakeIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport { return integrations.DetectReport{} }
func (f fakeIntegration) Init(_ integrations.ResolvedContext) integrations.StateReport  { return integrations.StateReport{} }
func (f fakeIntegration) Scan(_ integrations.ResolvedContext) integrations.StateReport  { return integrations.StateReport{} }
func (f fakeIntegration) Calibrate(_ integrations.ResolvedContext) integrations.StateReport {
	return integrations.StateReport{}
}

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

func TestIntegrationInspectInfo_Registered(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	reg := integrations.NewRegistry(settings)
	reg.Register("tool", "git", fakeIntegration{})

	info := commands.IntegrationInspectInfo("git", settings, reg)
	if !info.Registered {
		t.Error("expected Registered = true when integration is in registry")
	}
}

func TestWriteInspectReport_Quarantined(t *testing.T) {
	var buf bytes.Buffer
	info := commands.IntegrationInspectResult{
		Brand:            "git",
		Quarantined:      true,
		QuarantineReason: "attempted banned command: bash",
	}
	commands.WriteInspectReport(&buf, info)
	out := buf.String()
	if !strings.Contains(out, "QUARANTINED") {
		t.Errorf("missing QUARANTINED in output: %s", out)
	}
	if !strings.Contains(out, "attempted banned command: bash") {
		t.Errorf("missing reason in output: %s", out)
	}
	if !strings.Contains(out, "orbiter unquarantine git") {
		t.Errorf("missing restore hint in output: %s", out)
	}
}

func TestWriteInspectReport_Active(t *testing.T) {
	var buf bytes.Buffer
	info := commands.IntegrationInspectResult{Brand: "git", Registered: true}
	commands.WriteInspectReport(&buf, info)
	if !strings.Contains(buf.String(), "active") {
		t.Errorf("expected 'active' in output: %s", buf.String())
	}
}

func TestWriteInspectReport_NotInstalled(t *testing.T) {
	var buf bytes.Buffer
	info := commands.IntegrationInspectResult{Brand: "git"}
	commands.WriteInspectReport(&buf, info)
	if !strings.Contains(buf.String(), "not installed") {
		t.Errorf("expected 'not installed' in output: %s", buf.String())
	}
}
