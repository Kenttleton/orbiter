package integrations

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Integration must be implemented by every registered integration.
// The interface is defined here so implementors have a single import target.
// Consumers (starchart) define their own narrower interface for what they call.
type Integration interface {
	Meta() Manifest
	Detect(ctx DetectContext) DetectReport
	Init(ctx ResolvedContext) StateReport
	Scan(ctx ResolvedContext) StateReport
	Calibrate(ctx ResolvedContext) StateReport
}

// Registry holds a set of integrations keyed by "role/brand".
// All methods are goroutine-safe. Integrations can be registered, deregistered,
// quarantined, and unquarantined at runtime without a process restart.
type Registry struct {
	mu          sync.RWMutex
	entries     map[string]Integration // key: "role/brand"
	quarantined map[string]bool        // key: brand; in-memory mirror of settings quarantine
	settings    *SettingsStore
}

// NewRegistry returns an empty Registry backed by the given settings store.
// Quarantine state from settings is loaded into memory immediately.
// Pass nil for settings to get an in-memory-only registry (useful in tests).
func NewRegistry(settings *SettingsStore) *Registry {
	r := &Registry{
		entries:     make(map[string]Integration),
		quarantined: make(map[string]bool),
		settings:    settings,
	}
	if settings != nil {
		settings.mu.RLock()
		for brand := range settings.data.Quarantine {
			r.quarantined[brand] = true
		}
		settings.mu.RUnlock()
	}
	return r
}

// Register adds or replaces the integration for the given role and brand.
func (r *Registry) Register(role, brand string, i Integration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[role+"/"+brand] = i
}

// Deregister removes the integration for the given role and brand.
func (r *Registry) Deregister(role, brand string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, role+"/"+brand)
}

// Get returns the integration for role+brand. Returns (nil, false) if not registered
// or if the brand is currently quarantined.
func (r *Registry) Get(role, brand string) (Integration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.quarantined[brand] {
		return nil, false
	}
	i, ok := r.entries[role+"/"+brand]
	return i, ok
}

// All returns every non-quarantined integration.
func (r *Registry) All() []Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Integration, 0, len(r.entries))
	for key, i := range r.entries {
		if !r.quarantined[brandFromKey(key)] {
			result = append(result, i)
		}
	}
	return result
}

// AllForRole returns all non-quarantined integrations for a given role.
func (r *Registry) AllForRole(role string) []Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prefix := role + "/"
	var result []Integration
	for key, i := range r.entries {
		if strings.HasPrefix(key, prefix) && !r.quarantined[brandFromKey(key)] {
			result = append(result, i)
		}
	}
	return result
}

// IsQuarantined returns true if brand is currently quarantined.
func (r *Registry) IsQuarantined(brand string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.quarantined[brand]
}

// QuarantineBrand marks brand as quarantined in memory and persists to settings.json.
// The integration goes offline immediately. Prints a warning to stderr.
func (r *Registry) QuarantineBrand(brand, reason string) error {
	r.mu.Lock()
	r.quarantined[brand] = true
	r.mu.Unlock()

	fmt.Fprintf(os.Stderr,
		"\n  orbiter: integration %q quarantined — %s\n  Review: orbiter vessel inspect %s\n\n",
		brand, reason, brand,
	)

	if r.settings != nil {
		return r.settings.Quarantine(brand, reason)
	}
	return nil
}

// UnquarantineBrand clears the quarantine flag in memory and in settings.json.
// The integration is available immediately — no restart required.
func (r *Registry) UnquarantineBrand(brand string) error {
	r.mu.Lock()
	delete(r.quarantined, brand)
	r.mu.Unlock()

	if r.settings != nil {
		return r.settings.Unquarantine(brand)
	}
	return nil
}

// Default is the process-wide registry, backed by DefaultSettings.
var Default = NewRegistry(DefaultSettings)

// Register adds an integration to the Default registry.
// Called by integration init() functions.
func Register(role, brand string, i Integration) {
	Default.Register(role, brand, i)
}

func brandFromKey(key string) string {
	if i := strings.Index(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}
