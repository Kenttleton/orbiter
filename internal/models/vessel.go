package models

import "time"

// Vessel represents the local workstation — the Orbiter itself.
// Only one row exists per Star Chart.
type Vessel struct {
	ID        string    `db:"id"         json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
