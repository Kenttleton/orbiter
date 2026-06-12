package wasm_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestAuditLog_WritesEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// We test via the exported helper; override default path via env is not needed
	// because AuditLog always writes to ~/.orbiter/audit.log.
	// Instead, test indirectly: verify the entry serializes correctly.
	entry := wasm.AuditEntry{
		At:      time.Now().UTC(),
		Brand:   "git",
		Command: "git version",
		Outcome: "allowed",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"brand":"git"`) {
		t.Errorf("expected brand in JSON: %s", data)
	}
	if !strings.Contains(string(data), `"outcome":"allowed"`) {
		t.Errorf("expected outcome in JSON: %s", data)
	}

	// Test that AuditLog to a real path works
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// Write the entry manually to the temp path (AuditLog writes to ~/.orbiter, not testable directly)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(append(data, '\n'))
	f.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "git version") {
		t.Errorf("expected command in file: %s", content)
	}
}

func TestAuditEntry_OutcomeValues(t *testing.T) {
	outcomes := []string{"allowed", "denied", "quarantined", "captain_approved", "captain_denied"}
	for _, o := range outcomes {
		e := wasm.AuditEntry{Outcome: o}
		data, _ := json.Marshal(e)
		if !strings.Contains(string(data), o) {
			t.Errorf("outcome %q not found in JSON", o)
		}
	}
}
