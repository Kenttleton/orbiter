package models

import "time"

// Resource describes a tooling requirement, runtime, or capability
// scoped to any entity.
type Resource struct {
	ID        string    `db:"id"         json:"id"`
	EntityID  string    `db:"entity_id"  json:"entity_id"`
	Kind      string    `db:"kind"       json:"kind"`
	Manager   string    `db:"manager"    json:"manager,omitempty"`
	Version   string    `db:"version"    json:"version,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
