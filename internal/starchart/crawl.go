package starchart

import (
	"context"
	"fmt"
	"runtime"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

// BranchContext is the raw crawl result for an entity.
// Assembled by BranchCrawl; filtered by BuildResolvedContext.
type BranchContext struct {
	Platform     integrations.Platform
	EntityID     string
	Resources    []models.Resource
	Transponders []models.Transponder
	Callsigns    []models.Callsign
}

// BranchCrawl collects the resources, transponders, and callsigns reachable
// from entityID via the attachment graph (direct attachments only, Phase 2).
func (sc *StarChart) BranchCrawl(ctx context.Context, entityID string) (BranchContext, error) {
	branch := BranchContext{
		Platform: currentPlatform(),
		EntityID: entityID,
	}

	resources, err := sc.resourcesAttachedTo(ctx, entityID)
	if err != nil {
		return BranchContext{}, fmt.Errorf("crawl resources for %s: %w", entityID, err)
	}
	branch.Resources = resources

	callsigns, err := sc.callsignsAttachedTo(ctx, entityID)
	if err != nil {
		return BranchContext{}, fmt.Errorf("crawl callsigns for %s: %w", entityID, err)
	}
	branch.Callsigns = callsigns

	for _, cs := range callsigns {
		tps, err := sc.transpondersAttachedTo(ctx, cs.ID)
		if err != nil {
			return BranchContext{}, fmt.Errorf("crawl transponders for callsign %s: %w", cs.ID, err)
		}
		branch.Transponders = append(branch.Transponders, tps...)
	}

	return branch, nil
}

// BuildResolvedContext filters a BranchContext using the integration's manifest
// dependency declarations, producing the ResolvedContext passed to the integration.
func BuildResolvedContext(branch BranchContext, manifest integrations.Manifest) integrations.ResolvedContext {
	rc := integrations.ResolvedContext{
		Platform:     branch.Platform,
		Resources:    make(map[string][]integrations.ResolvedResource),
		Transponders: make(map[string][]integrations.ResolvedTransponder),
	}
	for role, brands := range manifest.Dependencies.Resources {
		for _, r := range branch.Resources {
			if r.Role == role && brandAccepted(r.Brand, brands) {
				rc.Resources[role] = append(rc.Resources[role], integrations.ResolvedResource{Resource: r})
			}
		}
	}
	for role, brands := range manifest.Dependencies.Transponders {
		for _, tp := range branch.Transponders {
			if tp.Role == role && brandAccepted(tp.Brand, brands) {
				rc.Transponders[role] = append(rc.Transponders[role], integrations.ResolvedTransponder{Transponder: tp})
			}
		}
	}
	return rc
}

func currentPlatform() integrations.Platform {
	return integrations.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// brandAccepted returns true if brand is in the whitelist or the whitelist is empty.
func brandAccepted(brand string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, b := range whitelist {
		if b == brand {
			return true
		}
	}
	return false
}

func (sc *StarChart) resourcesAttachedTo(ctx context.Context, nodeID string) ([]models.Resource, error) {
	const q = `
        SELECT r.id, r.role, r.brand, r.manages, r.config, r.created_at
        FROM resources r
        JOIN attachments a ON a.from_id = r.id
        WHERE a.to_id = ?
    `
	rows, err := sc.db.QueryContext(ctx, q, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.Resource
	for rows.Next() {
		var r models.Resource
		if err := rows.Scan(&r.ID, &r.Role, &r.Brand, &r.Manages, &r.Config, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (sc *StarChart) callsignsAttachedTo(ctx context.Context, nodeID string) ([]models.Callsign, error) {
	const q = `
        SELECT cs.id, cs.created_at
        FROM callsigns cs
        JOIN attachments a ON a.from_id = cs.id
        WHERE a.to_id = ?
    `
	rows, err := sc.db.QueryContext(ctx, q, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.Callsign
	for rows.Next() {
		var cs models.Callsign
		if err := rows.Scan(&cs.ID, &cs.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, cs)
	}
	return result, rows.Err()
}

func (sc *StarChart) transpondersAttachedTo(ctx context.Context, callsignID string) ([]models.Transponder, error) {
	const q = `
        SELECT tp.id, tp.role, tp.brand, tp.location, tp.created_at
        FROM transponders tp
        JOIN attachments a ON a.from_id = tp.id
        WHERE a.to_id = ?
    `
	rows, err := sc.db.QueryContext(ctx, q, callsignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.Transponder
	for rows.Next() {
		var tp models.Transponder
		if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Location, &tp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, tp)
	}
	return result, rows.Err()
}
