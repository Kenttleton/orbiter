package models

import "time"

// Beacon status values.
const (
	BeaconStatusHealthy = "healthy"
	BeaconStatusDrifted = "drifted"
	BeaconStatusUnknown = "unknown"
)

// Beacon is the most recent verified observation of an entity.
// One beacon exists per entity. Updated by Scan and Jump.
// Observations is a JSON array of observation strings.
type Beacon struct {
	ID           string    `db:"id"           json:"id"`
	EntityID     string    `db:"entity_id"    json:"entity_id"`
	Status       string    `db:"status"       json:"status"`
	Observations string    `db:"observations" json:"observations"`
	VerifiedAt   time.Time `db:"verified_at"  json:"verified_at"`
}
