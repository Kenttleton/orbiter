package models

import "time"

// SolarSystem is an optional organizational subdivision within a Galaxy
// (e.g. "platform", "mobile").
type SolarSystem struct {
	ID        string    `db:"id"         json:"id"`
	GalaxyID  string    `db:"galaxy_id"  json:"galaxy_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
