package models

import "time"

// Attachment is a directed graph edge wiring two entities together.
// FromID is the child entity (resource, callsign, transponder).
// ToID is the parent entity (vessel, galaxy, system, planet, or callsign).
// Attachment IDs are not registered in the aliases table.
type Attachment struct {
	ID        string    `db:"id"         json:"id"`
	FromID    string    `db:"from_id"    json:"from_id"`
	ToID      string    `db:"to_id"      json:"to_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
