package models

import "time"

// Alias is a row in the global OrbitID registry.
// Every entity has exactly one alias. Name defaults to ID when no
// human-readable name is provided.
// EntityType constants are defined in id.go.
type Alias struct {
	ID         string    `db:"id"          json:"id"`
	Name       string    `db:"name"        json:"name"`
	EntityType string    `db:"entity_type" json:"entity_type"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}
