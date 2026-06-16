package starchart

import (
	"context"
	"fmt"
	"slices"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

// resourceRoleOrder defines the dispatch sequence for resources within a branch level.
// Filesystem must initialize before managers, managers before runtimes,
// runtimes before remotes, remotes before tools.
var resourceRoleOrder = []string{
	integrations.ResourceRoleFilesystem,
	integrations.ResourceRoleManager,
	integrations.ResourceRoleRuntime,
	integrations.ResourceRoleRemote,
	integrations.ResourceRoleTool,
}

// ResourceScanResult is the scan outcome for one resource.
type ResourceScanResult struct {
	Resource     models.Resource
	Report       integrations.StateReport
	BeaconStatus string
}

// BranchScanResult holds scan results for all resources and transponders in the FILO hierarchy.
type BranchScanResult struct {
	Resources    []ResourceScanResult
	Transponders []integrations.TransponderScanResult
}

// ResourceCalibrateResult is the calibration outcome for one resource.
type ResourceCalibrateResult struct {
	Resource models.Resource
	Before   integrations.StateReport
	After    integrations.StateReport
	Action   string // "healthy", "calibrated", "failed"
}

// BranchCalibrateResult holds calibration results for all resources and transponders in the FILO hierarchy.
type BranchCalibrateResult struct {
	Resources    []ResourceCalibrateResult
	Transponders []integrations.TransponderCalibrateResult
}

// ScanBranch scans all resources in the FILO hierarchy for entityID.
// Resources dispatch in role order (filesystem → manager → runtime → remote → tool).
// After resources, transponders at each level also dispatch Scan.
// Writes beacons as a side effect.
func (sc *StarChart) ScanBranch(ctx context.Context, entityID string) (BranchScanResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchScanResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchScanResult
	seenTransponder := make(map[string]bool)
	for _, level := range lb.Levels {
		for _, r := range sortedResources(level.Resources) {
			rr, err := sc.scanResource(ctx, r, level, lb)
			if err != nil {
				return BranchScanResult{}, err
			}
			result.Resources = append(result.Resources, rr)
		}
		for _, tp := range level.Transponders {
			key := tp.Role + "/" + tp.Brand
			if seenTransponder[key] {
				continue
			}
			seenTransponder[key] = true
			tr, err := sc.scanTransponder(ctx, tp, level, lb)
			if err != nil {
				return BranchScanResult{}, err
			}
			result.Transponders = append(result.Transponders, tr)
		}
	}
	return result, nil
}

// CalibrateBranch scans then calibrates drifted/failed resources.
// Resources dispatch in role order (filesystem → manager → runtime → remote → tool).
// After resources, transponders at each level also dispatch Calibrate.
// Writes beacons as a side effect.
func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string) (BranchCalibrateResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchCalibrateResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchCalibrateResult
	for _, level := range lb.Levels {
		for _, r := range sortedResources(level.Resources) {
			cr, err := sc.calibrateResource(ctx, r, level, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Resources = append(result.Resources, cr)
		}
		for _, tp := range level.Transponders {
			tr, err := sc.calibrateTransponder(ctx, tp, level, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Transponders = append(result.Transponders, tr)
		}
	}
	return result, nil
}

// sortedResources returns resources sorted by resourceRoleOrder.
// Resources with unknown roles are appended last, preserving their relative order.
func sortedResources(resources []models.Resource) []models.Resource {
	sorted := make([]models.Resource, len(resources))
	copy(sorted, resources)
	slices.SortStableFunc(sorted, func(a, b models.Resource) int {
		ai := roleIndex(a.Role)
		bi := roleIndex(b.Role)
		return ai - bi
	})
	return sorted
}

// roleIndex returns the position of role in resourceRoleOrder, or len+1 for unknown roles.
func roleIndex(role string) int {
	for i, r := range resourceRoleOrder {
		if r == role {
			return i
		}
	}
	return len(resourceRoleOrder)
}

func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, level BranchLevel, lb LeveledBranch) (ResourceScanResult, error) {
	integration, ok := sc.integrations.Get(r.Role, r.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, r.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", r.Role, r.Brand),
		}); err != nil {
			return ResourceScanResult{}, err
		}
		return ResourceScanResult{Resource: r, BeaconStatus: status}, nil
	}
	rc := BuildResolvedContextForResource(r, level, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, r.ID, status, report.Observations); err != nil {
		return ResourceScanResult{}, err
	}
	return ResourceScanResult{Resource: r, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, level BranchLevel, lb LeveledBranch) (ResourceCalibrateResult, error) {
	scanResult, err := sc.scanResource(ctx, r, level, lb)
	if err != nil {
		return ResourceCalibrateResult{}, err
	}
	if scanResult.BeaconStatus == models.BeaconStatusHealthy {
		return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "healthy"}, nil
	}
	integration, ok := sc.integrations.Get(r.Role, r.Brand)
	if !ok {
		return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "failed"}, nil
	}
	rc := BuildResolvedContextForResource(r, level, lb, integration.Meta())
	after := integration.Calibrate(rc)
	afterStatus := scanBeaconStatus(after)
	if err := sc.setBeaconStatus(ctx, r.ID, afterStatus, after.Observations); err != nil {
		return ResourceCalibrateResult{}, err
	}
	action := "calibrated"
	if afterStatus == models.BeaconStatusFailed || after.Error != "" {
		action = "failed"
	}
	return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, After: after, Action: action}, nil
}

func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (integrations.TransponderScanResult, error) {
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		status := models.BeaconStatusFailed
		if err := sc.setBeaconStatus(ctx, tp.ID, status, []string{
			fmt.Sprintf("no integration registered for %s/%s", tp.Role, tp.Brand),
		}); err != nil {
			return integrations.TransponderScanResult{}, err
		}
		return integrations.TransponderScanResult{Transponder: tp, BeaconStatus: status}, nil
	}
	rc := BuildResolvedContextForTransponder(tp, level, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
		return integrations.TransponderScanResult{}, err
	}
	return integrations.TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateTransponder(ctx context.Context, tp models.Transponder, level BranchLevel, lb LeveledBranch) (integrations.TransponderCalibrateResult, error) {
	tr, err := sc.scanTransponder(ctx, tp, level, lb)
	if err != nil {
		return integrations.TransponderCalibrateResult{}, err
	}
	if scanBeaconStatus(tr.Report) == models.BeaconStatusHealthy {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report}, nil
	}
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report}, nil
	}
	rc := BuildResolvedContextForTransponder(tp, level, lb, integration.Meta())
	after := integration.Calibrate(rc)
	return integrations.TransponderCalibrateResult{Transponder: tp, Report: after}, nil
}

// scanBeaconStatus maps a StateReport to a beacon status constant.
func scanBeaconStatus(report integrations.StateReport) string {
	if report.Error != "" {
		return models.BeaconStatusFailed
	}
	if !report.Present || !report.Reachable {
		return models.BeaconStatusDrifted
	}
	return models.BeaconStatusHealthy
}
