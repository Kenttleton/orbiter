package models

import "time"

// Galaxy represents an organization or client (e.g. "acme", "personal").
type Galaxy struct {
	ID        string    `db:"id"         json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
