package starchart

import (
	"context"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

// ResourceScanResult is the scan outcome for one resource.
type ResourceScanResult struct {
	Resource     models.Resource
	Report       integrations.StateReport
	BeaconStatus string
}

// BranchScanResult holds scan results for all resources in the FILO hierarchy.
type BranchScanResult struct {
	Resources []ResourceScanResult
}

// ResourceCalibrateResult is the calibration outcome for one resource.
type ResourceCalibrateResult struct {
	Resource models.Resource
	Before   integrations.StateReport
	After    integrations.StateReport
	Action   string // "healthy", "calibrated", "failed"
}

// BranchCalibrateResult holds calibration results for all resources in the FILO hierarchy.
type BranchCalibrateResult struct {
	Resources []ResourceCalibrateResult
}

// ScanBranch scans all resources in the FILO hierarchy for entityID.
// Each resource receives its own level's transponders. Writes beacons as a side effect.
func (sc *StarChart) ScanBranch(ctx context.Context, entityID string) (BranchScanResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchScanResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchScanResult
	for _, level := range lb.Levels {
		for _, r := range level.Resources {
			rr, err := sc.scanResource(ctx, r, level, lb)
			if err != nil {
				return BranchScanResult{}, err
			}
			result.Resources = append(result.Resources, rr)
		}
	}
	return result, nil
}

// CalibrateBranch scans then calibrates drifted/failed resources. Writes beacons as a side effect.
func (sc *StarChart) CalibrateBranch(ctx context.Context, entityID string) (BranchCalibrateResult, error) {
	lb, err := sc.LeveledBranchCrawl(ctx, entityID)
	if err != nil {
		return BranchCalibrateResult{}, fmt.Errorf("crawl %s: %w", entityID, err)
	}

	var result BranchCalibrateResult
	for _, level := range lb.Levels {
		for _, r := range level.Resources {
			cr, err := sc.calibrateResource(ctx, r, level, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Resources = append(result.Resources, cr)
		}
	}
	return result, nil
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
