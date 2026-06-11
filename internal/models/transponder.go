package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself.
// Role is the access mechanism (file, env, keychain, vault, agent) and is Orbiter-owned.
// Brand is the service the credential grants access to and is integration-owned.
type Transponder struct {
	ID        string    `db:"id"         json:"id"`
	Role      string    `db:"role"       json:"role"`
	Brand     string    `db:"brand"      json:"brand"`
	Location  string    `db:"location"   json:"location"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
