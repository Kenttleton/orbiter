package models

import "time"

// NavigationHistory is an immutable log entry recording a navigation event.
type NavigationHistory struct {
	ID           string    `db:"id"             json:"id"`
	FromEntityID string    `db:"from_entity_id" json:"from_entity_id,omitempty"`
	ToEntityID   string    `db:"to_entity_id"   json:"to_entity_id"`
	Command      string    `db:"command"        json:"command"`
	OccurredAt   time.Time `db:"occurred_at"    json:"occurred_at"`
}
