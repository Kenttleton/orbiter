package models_test

import (
	"encoding/json"
	"testing"

	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	a := models.NewID(models.EntityTypePlanet)
	b := models.NewID(models.EntityTypePlanet)
	require.Len(t, a, 16)
	require.Len(t, b, 16)
	require.NotEqual(t, a, b, "successive IDs must differ")
}

func TestNewIDEmbeddsEntityType(t *testing.T) {
	id := models.NewID(models.EntityTypePlanet)
	require.Equal(t, models.EntityTypePlanet, id[8:10])
}

func TestIsID(t *testing.T) {
	valid := models.NewID(models.EntityTypeGalaxy)
	require.True(t, models.IsID(valid))
	require.False(t, models.IsID("payment-api"))
	require.False(t, models.IsID(""))
	require.False(t, models.IsID("tooshort"))
}

func TestParseID(t *testing.T) {
	id := models.NewID(models.EntityTypePlanet)
	parsed, err := models.ParseID(id)
	require.NoError(t, err)
	require.Equal(t, id, parsed.Raw)
	require.Equal(t, models.EntityTypePlanet, parsed.EntityType)
	require.False(t, parsed.Timestamp.IsZero())
}

func TestAliasJSONRoundtrip(t *testing.T) {
	id := models.NewID(models.EntityTypePlanet)
	a := models.Alias{
		ID:   id,
		Name: "payment-api",
	}
	data, err := json.Marshal(a)
	require.NoError(t, err)

	var got models.Alias
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, a.ID, got.ID)
	require.Equal(t, a.Name, got.Name)

	parsed, err := models.ParseID(got.ID)
	require.NoError(t, err)
	require.Equal(t, models.EntityTypePlanet, parsed.EntityType)
}

func TestEntityTypeConstants(t *testing.T) {
	types := []string{
		models.EntityTypeVessel,
		models.EntityTypeGalaxy,
		models.EntityTypeSolarSystem,
		models.EntityTypePlanet,
		models.EntityTypeCallsign,
		models.EntityTypeTransponder,
		models.EntityTypeResource,
		models.EntityTypeDefault,
		models.EntityTypeBeacon,
		models.EntityTypeNavHistory,
	}
	for _, et := range types {
		require.Len(t, et, 2, "entity type prefix must be 2 chars: %q", et)
	}
}

func TestBeaconStatusConstants(t *testing.T) {
	require.Equal(t, "unverified", models.BeaconStatusUnverified)
	require.Equal(t, "verified",   models.BeaconStatusVerified)
	require.Equal(t, "failed",     models.BeaconStatusFailed)
	require.Equal(t, "degraded",   models.BeaconStatusDegraded)
	require.Equal(t, "retired",    models.BeaconStatusRetired)
}

func TestEntityTypeAttachment(t *testing.T) {
	require.Equal(t, "at", models.EntityTypeAttachment)
	id := models.NewID(models.EntityTypeAttachment)
	parsed, err := models.ParseID(id)
	require.NoError(t, err)
	require.Equal(t, "at", parsed.EntityType)
}
