package models

import "time"

// Callsign represents the Captain's active identity.
// Scope is determined by attachments to hierarchy nodes.
type Callsign struct {
	ID        string    `db:"id"         json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
