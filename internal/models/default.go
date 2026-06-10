package models

import "time"

// Well-known default keys stored at the vessel level.
const (
	DefaultKeyOutputFormat     = "output_format"
	DefaultKeyHistoryRetention = "history_retention_days"
)

// Default is a key/value configuration entry scoped to any entity.
type Default struct {
	ID        string    `db:"id"         json:"id"`
	EntityID  string    `db:"entity_id"  json:"entity_id"`
	Key       string    `db:"key"        json:"key"`
	Value     string    `db:"value"      json:"value"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
