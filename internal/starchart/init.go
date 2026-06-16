package starchart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

// integrationProvider is the interface starchart needs from an integration registry.
// Defined here, at the consumer (idiomatic Go — not in the integrations package).
type integrationProvider interface {
	Get(role, brand string) (integrations.Integration, bool)
	All() []integrations.Integration
	AllForRole(role string) []integrations.Integration
}

// InitGalaxy cascades init to all planets in the galaxy.
func (sc *StarChart) InitGalaxy(ctx context.Context, galaxyID string) error {
	var planets []models.Planet
	if err := sc.List(ctx, "planets", &planets, Filter{Column: "galaxy_id", Op: "=", Value: galaxyID}); err != nil {
		return fmt.Errorf("list planets for galaxy %s: %w", galaxyID, err)
	}
	var errs []error
	for _, p := range planets {
		if err := sc.InitPlanet(ctx, p.ID); err != nil {
			errs = append(errs, err)
		}
	}
	status := models.BeaconStatusVerified
	if len(errs) > 0 {
		status = models.BeaconStatusDegraded
	}
	if err := sc.setBeaconStatus(ctx, galaxyID, status, nil); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// InitSolarSystem cascades init to all planets in the system.
func (sc *StarChart) InitSolarSystem(ctx context.Context, systemID string) error {
	var planets []models.Planet
	if err := sc.List(ctx, "planets", &planets, Filter{Column: "solar_system_id", Op: "=", Value: systemID}); err != nil {
		return fmt.Errorf("list planets for system %s: %w", systemID, err)
	}
	var errs []error
	for _, p := range planets {
		if err := sc.InitPlanet(ctx, p.ID); err != nil {
			errs = append(errs, err)
		}
	}
	status := models.BeaconStatusVerified
	if len(errs) > 0 {
		status = models.BeaconStatusDegraded
	}
	if err := sc.setBeaconStatus(ctx, systemID, status, nil); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// InitPlanet cascades init to all resources and callsigns attached to the planet.
func (sc *StarChart) InitPlanet(ctx context.Context, planetID string) error {
	resources, err := sc.resourcesAttachedTo(ctx, planetID)
	if err != nil {
		return fmt.Errorf("list resources for planet %s: %w", planetID, err)
	}
	callsigns, err := sc.callsignsAttachedTo(ctx, planetID)
	if err != nil {
		return fmt.Errorf("list callsigns for planet %s: %w", planetID, err)
	}

	var errs []error
	for _, r := range resources {
		if err := sc.InitResource(ctx, r.ID); err != nil {
			errs = append(errs, err)
		}
	}
	for _, cs := range callsigns {
		if err := sc.InitCallsign(ctx, cs.ID); err != nil {
			errs = append(errs, err)
		}
	}

	status := models.BeaconStatusVerified
	if len(errs) > 0 {
		status = models.BeaconStatusDegraded
	}
	if err := sc.setBeaconStatus(ctx, planetID, status, nil); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// InitCallsign cascades init to all transponders attached to the callsign.
func (sc *StarChart) InitCallsign(ctx context.Context, callsignID string) error {
	transponders, err := sc.transpondersAttachedTo(ctx, callsignID)
	if err != nil {
		return fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
	}
	var errs []error
	for _, tp := range transponders {
		if err := sc.InitTransponder(ctx, tp.ID); err != nil {
			errs = append(errs, err)
		}
	}
	status := models.BeaconStatusVerified
	if len(errs) > 0 {
		status = models.BeaconStatusDegraded
	}
	if err := sc.setBeaconStatus(ctx, callsignID, status, nil); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// InitResource provisions a resource by dispatching to its registered integration.
// If no integration is registered, the beacon is updated to BeaconStatusFailed.
// Returns an error only for Star Chart I/O failures — provisioning failures are
// recorded in the Beacon, not returned as errors.
func (sc *StarChart) InitResource(ctx context.Context, resourceID string) error {
	var r models.Resource
	if err := sc.Get(ctx, "resources", resourceID, &r); err != nil {
		return fmt.Errorf("get resource %s: %w", resourceID, err)
	}

	integration, ok := sc.integrations.Get(r.Role, r.Brand)
	if !ok {
		return sc.setBeaconStatus(ctx, resourceID, models.BeaconStatusFailed, []string{
			fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand),
		})
	}

	branch, err := sc.BranchCrawl(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("branch crawl for resource %s: %w", resourceID, err)
	}

	report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))
	return sc.applyStateReport(ctx, resourceID, report)
}

// InitTransponder provisions a transponder by dispatching to its registered integration.
func (sc *StarChart) InitTransponder(ctx context.Context, transponderID string) error {
	var tp models.Transponder
	if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
		return fmt.Errorf("get transponder %s: %w", transponderID, err)
	}

	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		return sc.setBeaconStatus(ctx, transponderID, models.BeaconStatusFailed, []string{
			fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
		})
	}

	branch, err := sc.BranchCrawl(ctx, transponderID)
	if err != nil {
		return fmt.Errorf("branch crawl for transponder %s: %w", transponderID, err)
	}

	report := integration.Init(BuildResolvedContextFromBranch(branch, integration.Meta()))
	return sc.applyStateReport(ctx, transponderID, report)
}

// applyStateReport converts an integration StateReport into a Beacon update.
func (sc *StarChart) applyStateReport(ctx context.Context, entityID string, report integrations.StateReport) error {
	status := models.BeaconStatusVerified
	obs := report.Observations
	if report.Error != "" {
		status = models.BeaconStatusFailed
		obs = append(obs, report.Error)
	} else if !report.Present {
		status = models.BeaconStatusFailed
		obs = append(obs, "integration reported entity not present after init")
	}
	return sc.setBeaconStatus(ctx, entityID, status, obs)
}

// setBeaconStatus updates a beacon by entity_id.
// Uses raw db.ExecContext with WHERE entity_id = ? because the generic Update
// uses the id column, but beacons are uniquely keyed by entity_id.
func (sc *StarChart) setBeaconStatus(ctx context.Context, entityID, status string, observations []string) error {
	if observations == nil {
		observations = []string{}
	}
	obs, err := json.Marshal(observations)
	if err != nil {
		return fmt.Errorf("marshal observations: %w", err)
	}
	_, err = sc.db.ExecContext(ctx,
		`UPDATE beacons SET status = ?, observations = ?, verified_at = ? WHERE entity_id = ?`,
		status, string(obs), time.Now().UTC(), entityID,
	)
	return err
}
