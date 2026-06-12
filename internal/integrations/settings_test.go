package integrations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestSettingsStore_Trust(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)

	if ss.IsAllowed("git", "git version") {
		t.Error("should not be allowed before Allow is called")
	}

	if err := ss.Allow("git", "git version"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !ss.IsAllowed("git", "git version") {
		t.Error("should be allowed after Allow")
	}

	if ss.IsAllowed("nvm", "git version") {
		t.Error("brand must match exactly")
	}

	if ss.IsAllowed("git", "git clone https://example.com/repo") {
		t.Error("full command string must match exactly")
	}
}

func TestSettingsStore_Quarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)

	if ss.IsQuarantined("bad-integration") {
		t.Error("should not be quarantined before Quarantine is called")
	}

	if err := ss.Quarantine("bad-integration", "attempted banned command: bash"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	if !ss.IsQuarantined("bad-integration") {
		t.Error("should be quarantined after Quarantine")
	}

	entry := ss.QuarantineEntry("bad-integration")
	if entry.Reason != "attempted banned command: bash" {
		t.Errorf("reason = %q", entry.Reason)
	}
	if entry.At.IsZero() {
		t.Error("at should be set")
	}
}

func TestSettingsStore_Unquarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	_ = ss.Quarantine("nvm", "attempted undeclared command: curl")

	if err := ss.Unquarantine("nvm"); err != nil {
		t.Fatalf("Unquarantine: %v", err)
	}
	if ss.IsQuarantined("nvm") {
		t.Error("should not be quarantined after Unquarantine")
	}
}

func TestSettingsStore_Persists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss1 := integrations.NewSettingsStore(path)
	_ = ss1.Allow("nvm", "nvm install 20.11.0")
	_ = ss1.Quarantine("bad", "attempted banned command: bash")

	ss2 := integrations.NewSettingsStore(path)
	if !ss2.IsAllowed("nvm", "nvm install 20.11.0") {
		t.Error("trust entry should persist")
	}
	if !ss2.IsQuarantined("bad") {
		t.Error("quarantine entry should persist")
	}
}

func TestSettingsStore_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	if ss.IsAllowed("git", "git version") {
		t.Error("missing file should return false, not panic")
	}
	if ss.IsQuarantined("any") {
		t.Error("missing file should return false for quarantine")
	}
}

func TestSettingsStore_JSONShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	ss := integrations.NewSettingsStore(path)
	_ = ss.Allow("git", "git version")
	_ = ss.Quarantine("bad", "attempted banned command: bash")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"trust"`) {
		t.Errorf("missing trust key: %s", s)
	}
	if !strings.Contains(s, `"quarantine"`) {
		t.Errorf("missing quarantine key: %s", s)
	}
	if !strings.Contains(s, `"git version"`) {
		t.Errorf("missing command string: %s", s)
	}
}
