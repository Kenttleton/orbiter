package integrations_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

func TestExtractSelected_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	if err := integrations.ExtractSelected(entries[:1], dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}

	brand := entries[0].Brand
	if _, err := os.Stat(filepath.Join(dir, brand, "manifest.toml")); err != nil {
		t.Errorf("manifest.toml not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, brand, brand+".wasm")); err != nil {
		t.Errorf("%s.wasm not created: %v", brand, err)
	}
}

func TestExtractSelected_Idempotent(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("first extract: %v", err)
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("second extract (idempotent): %v", err)
	}
}

func TestInstalledState_EmptyDir(t *testing.T) {
	state, err := integrations.InstalledState(t.TempDir())
	if err != nil {
		t.Fatalf("InstalledState on empty dir: %v", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state, got %d entries", len(state))
	}
}

func TestInstalledState_MissingDir(t *testing.T) {
	state, err := integrations.InstalledState("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("InstalledState on missing dir should not error: %v", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state for missing dir, got %d entries", len(state))
	}
}

func TestInstalledState_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}
	state, err := integrations.InstalledState(dir)
	if err != nil {
		t.Fatalf("InstalledState: %v", err)
	}
	for _, e := range entries {
		if _, ok := state[e.Brand]; !ok {
			t.Errorf("expected brand %q in installed state after extract", e.Brand)
		}
	}
}

func TestCatalogEntriesWithState_NoneInstalled(t *testing.T) {
	states, err := integrations.CatalogEntriesWithState(t.TempDir())
	if err != nil {
		t.Fatalf("CatalogEntriesWithState: %v", err)
	}
	for _, s := range states {
		if s.Installed {
			t.Errorf("brand %q should not be installed in empty dir", s.Brand)
		}
	}
}

func TestCatalogEntriesWithState_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}
	states, err := integrations.CatalogEntriesWithState(dir)
	if err != nil {
		t.Fatalf("CatalogEntriesWithState: %v", err)
	}
	for _, s := range states {
		if !s.Installed {
			t.Errorf("brand %q should be installed after extract", s.Brand)
		}
		if !s.ChecksumMatches {
			t.Errorf("brand %q checksum should match bundled after fresh extract", s.Brand)
		}
	}
}

// An installed integration carries every file its manifest declares, so Orbiter can
// delegate to it later from the install directory alone: a shell integration's hook
// script is declared in its manifest and must be installed beside it.
func TestExtractSelected_InstallsFilesTheManifestDeclares(t *testing.T) {
	dir := t.TempDir()
	var shells []integrations.CatalogEntry
	for _, e := range integrations.CatalogEntries() {
		if slices.Contains(e.Roles, "shell") {
			shells = append(shells, e)
		}
	}
	if len(shells) == 0 {
		t.Skip("no shell integrations in the catalog")
	}
	if err := integrations.ExtractSelected(shells, dir); err != nil {
		t.Fatal(err)
	}
	for _, e := range shells {
		manifestBytes, err := os.ReadFile(filepath.Join(dir, e.Brand, "manifest.toml"))
		if err != nil {
			t.Fatal(err)
		}
		var m core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &m); err != nil {
			t.Fatal(err)
		}
		hook := m.Shell.HookFile()
		if hook == "" {
			t.Errorf("%s declares no hook", e.Brand)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Brand, hook)); err != nil {
			t.Errorf("%s: declared hook %s was not installed: %v", e.Brand, hook, err)
		}
	}
}
