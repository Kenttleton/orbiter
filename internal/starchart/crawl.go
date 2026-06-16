package starchart

import (
	"context"
	"fmt"
	"runtime"
	"slices"

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

// BuildResolvedContextFromBranch filters a BranchContext using the integration's manifest
// dependency declarations, producing the ResolvedContext passed to the integration.
func BuildResolvedContextFromBranch(branch BranchContext, manifest integrations.Manifest) integrations.ResolvedContext {
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
	return len(whitelist) == 0 || slices.Contains(whitelist, brand)
}

// BranchLevel is one level in the FILO hierarchy for a branch crawl.
type BranchLevel struct {
	EntityID     string
	Resources    []models.Resource
	Callsign     *models.Callsign
	Transponders []models.Transponder
}

// LeveledBranch is the result of a full FILO hierarchy walk.
// Levels are ordered target-entity-first (index 0), vessel-last.
type LeveledBranch struct {
	Platform integrations.Platform
	Levels   []BranchLevel
}

// LeveledBranchCrawl walks from entityID up to vessel in FILO order.
// Two passes:
//
//	Pass 1 (root→leaf): compute effective callsign at each level. A level's
//	effective callsign is its own directly-attached callsign. Ancestors do not
//	pass their callsigns down — auth stays where attached.
//	Pass 2 (FILO): build dispatch levels, skipping levels with no resources.
func (sc *StarChart) LeveledBranchCrawl(ctx context.Context, entityID string) (LeveledBranch, error) {
	chain, err := sc.hierarchyChain(ctx, entityID)
	if err != nil {
		return LeveledBranch{}, fmt.Errorf("hierarchy chain for %s: %w", entityID, err)
	}

	// Pass 1: resolve the callsign and transponders for each level.
	type callsignEntry struct {
		cs  models.Callsign
		tps []models.Transponder
	}
	effectiveCallsign := make(map[string]*callsignEntry, len(chain))
	for _, levelID := range chain {
		callsigns, err := sc.callsignsAttachedTo(ctx, levelID)
		if err != nil {
			return LeveledBranch{}, fmt.Errorf("callsigns at level %s: %w", levelID, err)
		}
		if len(callsigns) == 0 {
			effectiveCallsign[levelID] = nil
			continue
		}
		cs := callsigns[0]
		tps, err := sc.transpondersAttachedTo(ctx, cs.ID)
		if err != nil {
			return LeveledBranch{}, fmt.Errorf("transponders for callsign %s: %w", cs.ID, err)
		}
		effectiveCallsign[levelID] = &callsignEntry{cs: cs, tps: tps}
	}

	// Pass 2: FILO — target first, vessel last.
	// Include a level if it has resources OR transponders (direct or via callsign).
	lb := LeveledBranch{Platform: currentPlatform()}
	for _, levelID := range chain {
		resources, err := sc.resourcesAttachedTo(ctx, levelID)
		if err != nil {
			return LeveledBranch{}, fmt.Errorf("resources at level %s: %w", levelID, err)
		}
		directTPs, err := sc.directTranspondersAttachedTo(ctx, levelID)
		if err != nil {
			return LeveledBranch{}, fmt.Errorf("direct transponders at level %s: %w", levelID, err)
		}
		var callsignTPs []models.Transponder
		var cs *models.Callsign
		if ce := effectiveCallsign[levelID]; ce != nil {
			cs = &ce.cs
			callsignTPs = ce.tps
		}
		if len(resources) == 0 && len(directTPs) == 0 && len(callsignTPs) == 0 {
			continue
		}
		level := BranchLevel{
			EntityID:  levelID,
			Resources: resources,
			Callsign:  cs,
			// direct first (specific — supersedes callsign transponders on FILO pop)
			Transponders: append(directTPs, callsignTPs...),
		}
		lb.Levels = append(lb.Levels, level)
	}
	return lb, nil
}

// hierarchyChain returns IDs from entityID up to vessel in FILO order (target first).
// Uses the entity type bits in the OrbitID (chars 8-9) to navigate the hierarchy.
func (sc *StarChart) hierarchyChain(ctx context.Context, entityID string) ([]string, error) {
	chain := []string{entityID}
	if len(entityID) < 10 {
		return chain, nil
	}
	switch entityID[8:10] {
	case "pl":
		var p models.Planet
		if err := sc.Get(ctx, "planets", entityID, &p); err != nil {
			return nil, fmt.Errorf("load planet %s: %w", entityID, err)
		}
		if p.SolarSystemID != "" {
			chain = append(chain, p.SolarSystemID)
		}
		chain = append(chain, p.GalaxyID)
	case "sy":
		var sys models.SolarSystem
		if err := sc.Get(ctx, "solar_systems", entityID, &sys); err != nil {
			return nil, fmt.Errorf("load system %s: %w", entityID, err)
		}
		chain = append(chain, sys.GalaxyID)
	case "gx":
		// falls through to vessel append
	case "vs":
		return chain, nil
	}
	var vesselID string
	if err := sc.db.QueryRowContext(ctx, `SELECT id FROM vessel LIMIT 1`).Scan(&vesselID); err != nil {
		return nil, fmt.Errorf("load vessel: %w", err)
	}
	chain = append(chain, vesselID)
	return chain, nil
}

// roleBranded is satisfied by models.Resource and models.Transponder via duck typing.
// Defined at the consumer (idiomatic Go — not in the models package).
type roleBranded interface {
	GetRole() string
	GetBrand() string
}

// collectFILO walks levels in order, collecting items whose role appears in deps
// and whose brand is accepted by that role's whitelist. First match per role/brand wins
// (FILO semantics: planet-first levels supersede ancestor levels).
func collectFILO[T roleBranded, R any](
	levels []BranchLevel,
	getItems func(BranchLevel) []T,
	wrap func(T) R,
	deps map[string][]string,
) map[string][]R {
	seen := make(map[string]bool)
	result := make(map[string][]R)
	for _, l := range levels {
		for _, item := range getItems(l) {
			brands, ok := deps[item.GetRole()]
			if !ok {
				continue
			}
			key := item.GetRole() + "/" + item.GetBrand()
			if brandAccepted(item.GetBrand(), brands) && !seen[key] {
				seen[key] = true
				result[item.GetRole()] = append(result[item.GetRole()], wrap(item))
			}
		}
	}
	return result
}

// BuildResolvedContext assembles the ResolvedContext for an integration dispatch.
// Resources and transponders are collected FILO across all branch levels
// (planet-first = narrow scope supersedes broad scope).
func BuildResolvedContext(self integrations.Entity, lb LeveledBranch, manifest integrations.Manifest) integrations.ResolvedContext {
	return integrations.ResolvedContext{
		Platform: lb.Platform,
		Self:     self,
		Resources: collectFILO(
			lb.Levels,
			func(l BranchLevel) []models.Resource { return l.Resources },
			func(r models.Resource) integrations.ResolvedResource { return integrations.ResolvedResource{Resource: r} },
			manifest.Dependencies.Resources,
		),
		Transponders: collectFILO(
			lb.Levels,
			func(l BranchLevel) []models.Transponder { return l.Transponders },
			func(tp models.Transponder) integrations.ResolvedTransponder {
				return integrations.ResolvedTransponder{Transponder: tp}
			},
			manifest.Dependencies.Transponders,
		),
	}
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

// directTranspondersAttachedTo returns transponders attached directly to entityID,
// without going through a callsign.
func (sc *StarChart) directTranspondersAttachedTo(ctx context.Context, entityID string) ([]models.Transponder, error) {
	const q = `
        SELECT tp.id, tp.role, tp.brand, tp.config, tp.created_at
        FROM transponders tp
        JOIN attachments a ON a.from_id = tp.id
        WHERE a.to_id = ?
    `
	rows, err := sc.db.QueryContext(ctx, q, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.Transponder
	for rows.Next() {
		var tp models.Transponder
		if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Config, &tp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, tp)
	}
	return result, rows.Err()
}

func (sc *StarChart) transpondersAttachedTo(ctx context.Context, callsignID string) ([]models.Transponder, error) {
	const q = `
        SELECT tp.id, tp.role, tp.brand, tp.config, tp.created_at
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
		if err := rows.Scan(&tp.ID, &tp.Role, &tp.Brand, &tp.Config, &tp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, tp)
	}
	return result, rows.Err()
}
