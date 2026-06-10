package models

import "time"

// Planet represents a project — the primary navigation target.
type Planet struct {
	ID            string    `db:"id"              json:"id"`
	GalaxyID      string    `db:"galaxy_id"       json:"galaxy_id"`
	SolarSystemID string    `db:"solar_system_id" json:"solar_system_id,omitempty"`
	RepoURL       string    `db:"repo_url"        json:"repo_url,omitempty"`
	RepoPath      string    `db:"repo_path"       json:"repo_path,omitempty"`
	CreatedAt     time.Time `db:"created_at"      json:"created_at"`
}
