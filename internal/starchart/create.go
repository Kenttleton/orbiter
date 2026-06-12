package starchart

import (
	"context"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
)

// GetBeacon returns the beacon for an entity. Returns ErrNotFound if none exists.
func (sc *StarChart) GetBeacon(ctx context.Context, entityID string) (models.Beacon, error) {
	var beacons []models.Beacon
	if err := sc.List(ctx, "beacons", &beacons, Filter{Column: "entity_id", Op: "=", Value: entityID}); err != nil {
		return models.Beacon{}, err
	}
	if len(beacons) == 0 {
		return models.Beacon{}, ErrNotFound
	}
	return beacons[0], nil
}

// createEntity inserts alias + entity + beacon atomically.
// entityFn inserts the entity row within the transaction.
func (sc *StarChart) createEntity(ctx context.Context, id, name string, entityFn func(*Tx) error) error {
	now := time.Now().UTC()
	return sc.Tx(ctx, func(t *Tx) error {
		if err := t.Insert(ctx, "aliases", models.AliasInsert{
			Name: name, Entity: id, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := entityFn(t); err != nil {
			return err
		}
		return t.Insert(ctx, "beacons", models.Beacon{
			ID:           models.NewID(models.EntityTypeBeacon),
			EntityID:     id,
			Status:       models.BeaconStatusUnverified,
			Observations: "[]",
			VerifiedAt:   now,
		})
	})
}

// CreateGalaxy registers a new galaxy in the Star Chart.
func (sc *StarChart) CreateGalaxy(ctx context.Context, name string) (models.Galaxy, error) {
	id := models.NewID(models.EntityTypeGalaxy)
	g := models.Galaxy{ID: id, CreatedAt: time.Now().UTC()}
	return g, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "galaxies", g)
	})
}

// CreateSolarSystem registers a new solar system under a galaxy.
func (sc *StarChart) CreateSolarSystem(ctx context.Context, name, galaxyID string) (models.SolarSystem, error) {
	id := models.NewID(models.EntityTypeSolarSystem)
	sys := models.SolarSystem{ID: id, GalaxyID: galaxyID, CreatedAt: time.Now().UTC()}
	return sys, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "solar_systems", sys)
	})
}

// CreatePlanet registers a new planet under a galaxy and optional solar system.
func (sc *StarChart) CreatePlanet(ctx context.Context, name, galaxyID, solarSystemID string) (models.Planet, error) {
	id := models.NewID(models.EntityTypePlanet)
	p := models.Planet{ID: id, GalaxyID: galaxyID, SolarSystemID: solarSystemID, CreatedAt: time.Now().UTC()}
	return p, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "planets", p)
	})
}

// CreateCallsign registers a new callsign.
func (sc *StarChart) CreateCallsign(ctx context.Context, name string) (models.Callsign, error) {
	id := models.NewID(models.EntityTypeCallsign)
	cs := models.Callsign{ID: id, CreatedAt: time.Now().UTC()}
	return cs, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "callsigns", cs)
	})
}

// CreateTransponder registers a new transponder.
// Role is Orbiter-owned (file, env, keychain, vault, agent).
// Brand is integration-owned — any string is accepted.
// config is a JSON object (e.g. `{"location":"/path"}` for file transponders); defaults to `{}`.
func (sc *StarChart) CreateTransponder(ctx context.Context, name, role, brand, config string) (models.Transponder, error) {
	if config == "" {
		config = "{}"
	}
	id := models.NewID(models.EntityTypeTransponder)
	tp := models.Transponder{ID: id, Role: role, Brand: brand, Config: config, CreatedAt: time.Now().UTC()}
	return tp, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "transponders", tp)
	})
}

// CreateResource registers a new resource.
// Role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
// Brand is integration-owned — any string is accepted.
// manages is a JSON array (e.g. `["node"]`); config is a JSON object.
func (sc *StarChart) CreateResource(ctx context.Context, name, role, brand, manages, config string) (models.Resource, error) {
	id := models.NewID(models.EntityTypeResource)
	r := models.Resource{ID: id, Role: role, Brand: brand, Manages: manages, Config: config, CreatedAt: time.Now().UTC()}
	return r, sc.createEntity(ctx, id, name, func(t *Tx) error {
		return t.Insert(ctx, "resources", r)
	})
}
