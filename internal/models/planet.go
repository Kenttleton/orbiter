package models

import "time"

// Planet represents a project — the primary navigation target.
// Galaxy and optional SolarSystem establish its position in the hierarchy.
// Repository association is managed via remote resource attachments.
type Planet struct {
	ID            string    `db:"id"              json:"id"`
	GalaxyID      string    `db:"galaxy_id"       json:"galaxy_id"`
	SolarSystemID string    `db:"solar_system_id" json:"solar_system_id,omitempty"`
	CreatedAt     time.Time `db:"created_at"      json:"created_at"`
}
