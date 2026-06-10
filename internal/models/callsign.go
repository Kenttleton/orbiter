package models

import "time"

// Callsign represents the Captain's active identity (e.g. "kent-acme").
// Scoped to a vessel or galaxy via EntityID.
type Callsign struct {
	ID        string    `db:"id"         json:"id"`
	EntityID  string    `db:"entity_id"  json:"entity_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
