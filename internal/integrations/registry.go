package integrations

import "strings"

// Integration must be implemented by every registered integration.
// The interface is defined here so implementors have a single import target.
// Consumers (starchart) define their own narrower interface for what they call.
type Integration interface {
	Detect(ctx DetectContext) DetectReport
	Init(ctx ResolvedContext) StateReport
	Scan(ctx ResolvedContext) StateReport
	Calibrate(ctx ResolvedContext) StateReport
}

// Registry holds a set of integrations keyed by "role/brand".
type Registry struct {
	entries map[string]Integration
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: map[string]Integration{}}
}

// Register adds an integration for the given role and brand.
// Calling Register with the same role+brand overwrites the previous entry.
func (r *Registry) Register(role, brand string, i Integration) {
	r.entries[role+"/"+brand] = i
}

// Get returns the integration for a given role+brand pair.
func (r *Registry) Get(role, brand string) (Integration, bool) {
	i, ok := r.entries[role+"/"+brand]
	return i, ok
}

// AllForRole returns all integrations registered for a given role.
// Used during detection for always-run roles (remote, filesystem).
func (r *Registry) AllForRole(role string) []Integration {
	prefix := role + "/"
	var result []Integration
	for key, i := range r.entries {
		if strings.HasPrefix(key, prefix) {
			result = append(result, i)
		}
	}
	return result
}

// Default is the process-wide registry. Integration init() functions call
// Default.Register to self-register. Wired into StarChart via Open().
var Default = NewRegistry()

// Register adds an integration to the Default registry.
// Called by integration init() functions.
func Register(role, brand string, i Integration) {
	Default.Register(role, brand, i)
}
