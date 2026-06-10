package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself.
// Always linked to a Callsign. Optionally narrowed to a specific entity.
type Transponder struct {
	ID         string    `db:"id"           json:"id"`
	CallsignID string    `db:"callsign_id"  json:"callsign_id"`
	EntityID   string    `db:"entity_id"    json:"entity_id,omitempty"`
	Service    string    `db:"service"      json:"service"`
	Location   string    `db:"location"     json:"location"`
	CreatedAt  time.Time `db:"created_at"   json:"created_at"`
}
