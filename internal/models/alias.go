package models

import "time"

// AliasInsert is the write shape for the aliases table.
// Used by starchart internally when creating entities.
type AliasInsert struct {
	Name      string    `db:"name"`
	Entity    string    `db:"entity"`
	CreatedAt time.Time `db:"created_at"`
}

// Alias is the read view returned by StarChart.Resolve.
// ID holds the entity orbit ID; entity type is encoded in the orbit ID
// at chars 9-10 and can be extracted via ParseID.
type Alias struct {
	ID        string    `db:"entity"     json:"id"`
	Name      string    `db:"name"       json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
