package models

import "time"

// Transponder is a pointer to a credential location or auth service.
// It never stores the credential itself — only enough config to locate or reach it.
// Role is the access mechanism (file, env, keychain, vault, agent) and is Orbiter-owned.
// Brand is the service the credential grants access to and is integration-owned.
// Config is a JSON object whose shape is role-specific (see docs/integrations.md).
type Transponder struct {
	ID        string    `db:"id"         json:"id"`
	Role      string    `db:"role"       json:"role"`
	Brand     string    `db:"brand"      json:"brand"`
	Config    string    `db:"config"     json:"config"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (t Transponder) GetID() string     { return t.ID }
func (t Transponder) GetRole() string   { return t.Role }
func (t Transponder) GetBrand() string  { return t.Brand }
func (t Transponder) GetConfig() string { return t.Config }
