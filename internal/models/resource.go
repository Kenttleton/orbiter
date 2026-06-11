package models

import "time"

// Resource describes a tooling requirement, runtime, or capability.
// Role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
// Brand identifies the specific implementation and is integration-owned —
// Orbiter accepts any brand string; the integration registry determines validity.
// Manages is a JSON array of brands this resource controls (non-empty for role=manager).
// Config is a JSON object for integration-specific configuration.
type Resource struct {
	ID        string    `db:"id"         json:"id"`
	Role      string    `db:"role"       json:"role"`
	Brand     string    `db:"brand"      json:"brand"`
	Manages   string    `db:"manages"    json:"manages"`
	Config    string    `db:"config"     json:"config"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
