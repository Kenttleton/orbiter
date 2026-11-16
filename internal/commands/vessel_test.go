package commands_test

import (
	"bytes"
	"os"
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
		{Brand: "golang", Name: "Go Toolchain", Description: "Go runtime", Roles: []string{"runtime"}},
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

// An integration is a folder in the integrations directory. Inspecting one reports what is true of
// it: quarantined, used (resources in the tree use it), idle (the folder is there and nothing
// uses it), or missing (there is no such folder, so there is nothing to inspect).

func TestIntegrationInspectInfo_Quarantined(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	_ = settings.Quarantine("git", "attempted undeclared command: rm")

	info := commands.IntegrationInspectInfo("git", settings, t.TempDir(), 0)
	if !info.Quarantined {
		t.Error("expected Quarantined = true")
	}
	if info.QuarantineReason == "" {
		t.Error("expected non-empty QuarantineReason")
	}
}

func TestIntegrationInspectInfo_NotQuarantined(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	info := commands.IntegrationInspectInfo("git", settings, t.TempDir(), 0)
	if info.Quarantined {
		t.Error("expected Quarantined = false")
	}
}

func TestIntegrationInspectInfo_FoundIsTheFolder(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	dir := t.TempDir()
	if got := commands.IntegrationInspectInfo("git", settings, dir, 0); got.Found {
		t.Error("no folder, so missing")
	}
	if err := os.MkdirAll(filepath.Join(dir, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "git", "manifest.toml"), []byte("[integration]\nbrand = \"git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := commands.IntegrationInspectInfo("git", settings, dir, 0); !got.Found {
		t.Error("the folder is there, so it is found")
	}
}

func TestIntegrationInspectInfo_UsedBy(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if got := commands.IntegrationInspectInfo("git", settings, t.TempDir(), 3); got.UsedBy != 3 {
		t.Errorf("UsedBy = %d, want 3", got.UsedBy)
	}
}

func TestWriteInspectReport_Quarantined(t *testing.T) {
	var buf bytes.Buffer
	info := commands.IntegrationInspectResult{
		Brand:            "git",
		Quarantined:      true,
		QuarantineReason: "attempted undeclared command: rm",
	}
	commands.WriteInspectReport(&buf, info)
	out := buf.String()
	for _, want := range []string{"QUARANTINED", "attempted undeclared command: rm", "orbiter unquarantine git"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %s", want, out)
		}
	}
}

func TestWriteInspectReport_InUse(t *testing.T) {
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "git", Found: true, UsedBy: 2})
	if !strings.Contains(buf.String(), "Status:      used") || !strings.Contains(buf.String(), "2") {
		t.Errorf("expected 'used' and the count: %s", buf.String())
	}
}

func TestWriteInspectReport_NotUsed(t *testing.T) {
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "git", Found: true})
	if !strings.Contains(buf.String(), "Status:      idle") {
		t.Errorf("expected 'idle': %s", buf.String())
	}
	if strings.Contains(buf.String(), "not installed") {
		t.Errorf("an integration whose folder is there is never 'not installed': %s", buf.String())
	}
}

func TestWriteInspectReport_NotFound(t *testing.T) {
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "nope"})
	if !strings.Contains(buf.String(), "Status:      missing") {
		t.Errorf("expected 'missing': %s", buf.String())
	}
}

func TestBuildChecklistItems_NotInstalled(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git", Description: "Git VCS"}, Installed: false},
	}
	items := commands.BuildChecklistItems(states)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Checked {
		t.Error("not-installed item should not be pre-checked")
	}
	if items[0].Badge != "" {
		t.Errorf("not-installed item should have no badge, got %q", items[0].Badge)
	}
	if items[0].Tag != "git" {
		t.Errorf("tag should be brand, got %q", items[0].Tag)
	}
}

func TestBuildChecklistItems_InstalledUpToDate(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git"}, Installed: true, ChecksumMatches: true},
	}
	items := commands.BuildChecklistItems(states)
	if !items[0].Checked {
		t.Error("installed item should be pre-checked")
	}
	if items[0].Badge != "" {
		t.Errorf("up-to-date item should have no badge, got %q", items[0].Badge)
	}
}

func TestBuildChecklistItems_UpgradeAvailable(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git"}, Installed: true, ChecksumMatches: false},
	}
	items := commands.BuildChecklistItems(states)
	if !items[0].Checked {
		t.Error("outdated item should still be pre-checked")
	}
	if items[0].Badge != "upgrade available" {
		t.Errorf("outdated item should have 'upgrade available' badge, got %q", items[0].Badge)
	}
}

func TestApplySelections_ExtractsSelected(t *testing.T) {
	dir := t.TempDir()
	entries := bundle.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	states, err := bundle.CatalogEntriesWithState(dir)
	if err != nil {
		t.Fatalf("CatalogEntriesWithState: %v", err)
	}
	selected := []commands.ChecklistItem{{Tag: entries[0].Brand}}

	if err := commands.ApplySelections(states, selected, dir); err != nil {
		t.Fatalf("ApplySelections: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, entries[0].Brand)); err != nil {
		t.Errorf("expected %s to be installed after ApplySelections: %v", entries[0].Brand, err)
	}
}

func TestApplySelections_RemovesDeselected(t *testing.T) {
	dir := t.TempDir()
	entries := bundle.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	// Pre-install all.
	if err := bundle.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("pre-install: %v", err)
	}

	states, err := bundle.CatalogEntriesWithState(dir)
	if err != nil {
		t.Fatalf("CatalogEntriesWithState: %v", err)
	}
	// Deselect all.
	if err := commands.ApplySelections(states, nil, dir); err != nil {
		t.Fatalf("ApplySelections: %v", err)
	}

	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Brand)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed after deselect", e.Brand)
		}
	}
}

// The Captain can see which engine an integration declared: the interpreter by default, the
// compiler when the author asked for it.
func TestIntegrationInspectInfo_ShowsTheDeclaredEngine(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	dir := t.TempDir()
	for brand, body := range map[string]string{
		"plain":  "[integration]\nbrand = \"plain\"\n",
		"fast":   "[integration]\nbrand = \"fast\"\n[runtime]\nengine = \"compiler\"\n",
		"broken": "[integration]\nbrand = \"broken\"\n[runtime]\nengine = \"jit\"\n",
	} {
		if err := os.MkdirAll(filepath.Join(dir, brand), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, brand, "manifest.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for brand, want := range map[string]string{"plain": "interpreter", "fast": "compiler", "broken": "jit", "absent": ""} {
		if got := commands.IntegrationInspectInfo(brand, settings, dir, 0).Engine; got != want {
			t.Errorf("%s: Engine = %q, want %q", brand, got, want)
		}
	}
}

func TestWriteInspectReport_ShowsTheEngine(t *testing.T) {
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "git", Found: true, Engine: "compiler"})
	if !strings.Contains(buf.String(), "Engine:      compiler") {
		t.Errorf("expected the engine line: %s", buf.String())
	}
}

func TestIntegrationInspectInfo_ShowsTheMemoryLimit(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	dir := t.TempDir()
	for brand, body := range map[string]string{
		"plain": "[integration]\nbrand = \"plain\"\n",
		"big":   "[integration]\nbrand = \"big\"\n[runtime]\nmemory_limit = 268435456\n",
	} {
		if err := os.MkdirAll(filepath.Join(dir, brand), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, brand, "manifest.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := commands.IntegrationInspectInfo("plain", settings, dir, 0).MemoryBytes; got != integrations.DefaultMemoryLimit {
		t.Errorf("default MemoryBytes = %d", got)
	}
	if got := commands.IntegrationInspectInfo("big", settings, dir, 0).MemoryBytes; got != 268435456 {
		t.Errorf("declared MemoryBytes = %d", got)
	}
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "big", Found: true, Engine: "interpreter", MemoryBytes: 268435456})
	if !strings.Contains(buf.String(), "Memory:      268435456 bytes") {
		t.Errorf("expected the memory line: %s", buf.String())
	}
}

// The Captain can read what environment an integration's commands are given.
func TestIntegrationInspectInfo_ShowsTheEnvironmentClaim(t *testing.T) {
	settings := integrations.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[integration]\nbrand = \"git\"\n[commands]\nenv = [\"HOME\", \"GIT_*\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "git", "manifest.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info := commands.IntegrationInspectInfo("git", settings, dir, 0)
	if len(info.Env) != 2 || info.Env[0] != "HOME" {
		t.Errorf("Env = %v", info.Env)
	}
	var buf bytes.Buffer
	commands.WriteInspectReport(&buf, info)
	if !strings.Contains(buf.String(), "Environment: HOME, GIT_*") {
		t.Errorf("expected the environment line: %s", buf.String())
	}
	buf.Reset()
	commands.WriteInspectReport(&buf, commands.IntegrationInspectResult{Brand: "x", Found: true, Engine: "interpreter"})
	if !strings.Contains(buf.String(), "Environment: (none)") {
		t.Errorf("an integration claiming nothing gets nothing, and says so: %s", buf.String())
	}
}
