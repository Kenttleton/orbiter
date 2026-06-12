package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SettingsStore persists trust and quarantine state to ~/.orbiter/settings.json.
// Trust keys: (brand, fullCommandString). Quarantine keys: brand.
// Only "allow" and quarantine entries are written; Captain declines are ephemeral.
type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

type settingsFile struct {
	Trust      map[string]map[string]string `json:"trust,omitempty"`
	Quarantine map[string]quarantineEntry   `json:"quarantine,omitempty"`
}

// QuarantineInfo holds the details of a quarantine entry.
type QuarantineInfo struct {
	Reason string
	At     time.Time
}

type quarantineEntry struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// NewSettingsStore returns a SettingsStore backed by path.
// A missing file is treated as empty — no error.
func NewSettingsStore(path string) *SettingsStore {
	ss := &SettingsStore{path: path}
	ss.load()
	return ss
}

// IsAllowed returns true if the Captain has previously always-allowed
// this exact (brand, fullCommandString) pair.
func (ss *SettingsStore) IsAllowed(brand, fullCmd string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Trust == nil {
		return false
	}
	return ss.data.Trust[brand][fullCmd] == "allow"
}

// Allow records a permanent allow for (brand, fullCmd) and flushes to disk.
func (ss *SettingsStore) Allow(brand, fullCmd string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Trust == nil {
		ss.data.Trust = make(map[string]map[string]string)
	}
	if ss.data.Trust[brand] == nil {
		ss.data.Trust[brand] = make(map[string]string)
	}
	ss.data.Trust[brand][fullCmd] = "allow"
	return ss.flush()
}

// IsQuarantined returns true if brand has a quarantine entry.
func (ss *SettingsStore) IsQuarantined(brand string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Quarantine == nil {
		return false
	}
	_, ok := ss.data.Quarantine[brand]
	return ok
}

// QuarantineEntry returns the quarantine details for brand.
// Returns a zero QuarantineInfo if brand is not quarantined.
func (ss *SettingsStore) QuarantineEntry(brand string) QuarantineInfo {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.data.Quarantine == nil {
		return QuarantineInfo{}
	}
	e, ok := ss.data.Quarantine[brand]
	if !ok {
		return QuarantineInfo{}
	}
	return QuarantineInfo{Reason: e.Reason, At: e.At}
}

// Quarantine marks brand as quarantined and flushes to disk.
func (ss *SettingsStore) Quarantine(brand, reason string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Quarantine == nil {
		ss.data.Quarantine = make(map[string]quarantineEntry)
	}
	ss.data.Quarantine[brand] = quarantineEntry{Reason: reason, At: time.Now().UTC()}
	return ss.flush()
}

// Unquarantine removes the quarantine entry for brand and flushes to disk.
func (ss *SettingsStore) Unquarantine(brand string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.data.Quarantine != nil {
		delete(ss.data.Quarantine, brand)
	}
	return ss.flush()
}

func (ss *SettingsStore) load() {
	data, err := os.ReadFile(ss.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ss.data)
}

func (ss *SettingsStore) flush() error {
	if err := os.MkdirAll(filepath.Dir(ss.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ss.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ss.path, append(data, '\n'), 0600)
}

// DefaultSettings is the process-wide settings store at ~/.orbiter/settings.json.
var DefaultSettings = func() *SettingsStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return NewSettingsStore(filepath.Join(home, ".orbiter", "settings.json"))
}()
