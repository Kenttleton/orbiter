package wasm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry records a single host-function call.
type AuditEntry struct {
	At      time.Time `json:"at"`
	Brand   string    `json:"brand"`
	Command string    `json:"command"`
	Outcome string    `json:"outcome"` // "allowed", "denied", "quarantined", "captain_approved", "captain_denied"
	Reason  string    `json:"reason,omitempty"`
}

// AuditLog appends an entry to ~/.orbiter/audit.log (newline-delimited JSON).
// Missing parent directory is created automatically.
// Errors are silently ignored — audit log failure must not break normal operation.
func AuditLog(entry AuditEntry) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".orbiter", "audit.log")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}
