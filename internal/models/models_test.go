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
	a := models.Alias{
		ID:         models.NewID(models.EntityTypePlanet),
		Name:       "payment-api",
		EntityType: models.EntityTypePlanet,
	}
	data, err := json.Marshal(a)
	require.NoError(t, err)

	var got models.Alias
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, a.ID, got.ID)
	require.Equal(t, a.Name, got.Name)
	require.Equal(t, a.EntityType, got.EntityType)
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
