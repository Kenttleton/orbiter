package starchart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
)

// resourceRoleOrder defines the dispatch sequence for resources within a branch level.
// Shell must initialize before managers, managers before runtimes,
// runtimes before remotes, remotes before tools. Export runs after tools
// (reads their state); multiplexer runs after export.
var resourceRoleOrder = []string{
	integrations.ResourceRoleFilesystem,
	integrations.ResourceRoleShell,
	integrations.ResourceRoleManager,
	integrations.ResourceRoleRuntime,
	integrations.ResourceRoleRemote,
	integrations.ResourceRoleTool,
	integrations.ResourceRoleExport,
	integrations.ResourceRoleMultiplexer,
}

// writeConfigFiles writes path→content pairs declared in StateReport.Config["write_files"]
// by an export-role integration. It returns the (possibly modified) report with any write
// errors appended to Observations and set on Error.
func writeConfigFiles(report integrations.StateReport) integrations.StateReport {
	raw, ok := report.Config["write_files"]
	if !ok {
		return report
	}
	files, ok := raw.(map[string]any)
	if !ok {
		return report
	}
	for path, v := range files {
		content, ok := v.(string)
		if !ok {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			msg := fmt.Sprintf("writeConfigFiles: mkdir %s: %v", filepath.Dir(path), err)
			report.Observations = append(report.Observations, msg)
			report.Error = msg
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			msg := fmt.Sprintf("writeConfigFiles: write %s: %v", path, err)
			report.Observations = append(report.Observations, msg)
			report.Error = msg
		}
	}
	return report
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

// CallsignScanResult holds scan results for all transponders in a callsign.
type CallsignScanResult struct {
	Transponders []integrations.TransponderScanResult
}

// CallsignCalibrateResult holds calibration results for all transponders in a callsign.
type CallsignCalibrateResult struct {
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
			rr, err := sc.scanResource(ctx, r, lb)
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
			tr, err := sc.scanTransponder(ctx, tp, lb)
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
	seenTransponder := make(map[string]bool)
	for _, level := range lb.Levels {
		for _, r := range sortedResources(level.Resources) {
			cr, err := sc.calibrateResource(ctx, r, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Resources = append(result.Resources, cr)
		}
		for _, tp := range level.Transponders {
			key := tp.Role + "/" + tp.Brand
			if seenTransponder[key] {
				continue
			}
			seenTransponder[key] = true
			tr, err := sc.calibrateTransponder(ctx, tp, lb)
			if err != nil {
				return BranchCalibrateResult{}, err
			}
			result.Transponders = append(result.Transponders, tr)
		}
	}
	return result, nil
}

// ScanCallsign scans all transponders attached to callsignID in isolation.
// No branch resources are provided — only the transponder's own config is available.
// Updates beacons as a side effect.
func (sc *StarChart) ScanCallsign(ctx context.Context, callsignID string) (CallsignScanResult, error) {
	tps, err := sc.transpondersAttachedTo(ctx, callsignID)
	if err != nil {
		return CallsignScanResult{}, fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
	}
	emptyLB := LeveledBranch{Platform: currentPlatform()}
	var result CallsignScanResult
	for _, tp := range tps {
		tr, err := sc.scanTransponder(ctx, tp, emptyLB)
		if err != nil {
			return CallsignScanResult{}, err
		}
		result.Transponders = append(result.Transponders, tr)
	}
	return result, nil
}

// CalibrateCallsign scans then calibrates drifted/failed transponders in callsignID.
func (sc *StarChart) CalibrateCallsign(ctx context.Context, callsignID string) (CallsignCalibrateResult, error) {
	tps, err := sc.transpondersAttachedTo(ctx, callsignID)
	if err != nil {
		return CallsignCalibrateResult{}, fmt.Errorf("list transponders for callsign %s: %w", callsignID, err)
	}
	emptyLB := LeveledBranch{Platform: currentPlatform()}
	var result CallsignCalibrateResult
	for _, tp := range tps {
		tr, err := sc.calibrateTransponder(ctx, tp, emptyLB)
		if err != nil {
			return CallsignCalibrateResult{}, err
		}
		result.Transponders = append(result.Transponders, tr)
	}
	return result, nil
}

// TranspondersForCallsign returns all transponders attached to callsignID.
func (sc *StarChart) TranspondersForCallsign(ctx context.Context, callsignID string) ([]models.Transponder, error) {
	return sc.transpondersAttachedTo(ctx, callsignID)
}

// ScanTransponder scans a single transponder in isolation.
func (sc *StarChart) ScanTransponder(ctx context.Context, transponderID string) (integrations.TransponderScanResult, error) {
	var tp models.Transponder
	if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
		return integrations.TransponderScanResult{}, fmt.Errorf("get transponder %s: %w", transponderID, err)
	}
	return sc.scanTransponder(ctx, tp, LeveledBranch{Platform: currentPlatform()})
}

// CalibrateTransponder calibrates a single transponder in isolation.
func (sc *StarChart) CalibrateTransponder(ctx context.Context, transponderID string) (integrations.TransponderCalibrateResult, error) {
	var tp models.Transponder
	if err := sc.Get(ctx, "transponders", transponderID, &tp); err != nil {
		return integrations.TransponderCalibrateResult{}, fmt.Errorf("get transponder %s: %w", transponderID, err)
	}
	return sc.calibrateTransponder(ctx, tp, LeveledBranch{Platform: currentPlatform()})
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

func (sc *StarChart) scanResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceScanResult, error) {
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
	rc := BuildResolvedContext(r, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, r.ID, status, report.Observations); err != nil {
		return ResourceScanResult{}, err
	}
	return ResourceScanResult{Resource: r, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateResource(ctx context.Context, r models.Resource, lb LeveledBranch) (ResourceCalibrateResult, error) {
	scanResult, err := sc.scanResource(ctx, r, lb)
	if err != nil {
		return ResourceCalibrateResult{}, err
	}
	// Export-role resources must always run Calibrate (to write config files),
	// even when the scan already reports healthy.
	if scanResult.BeaconStatus == models.BeaconStatusHealthy && r.Role != integrations.ResourceRoleExport {
		return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "healthy"}, nil
	}
	integration, ok := sc.integrations.Get(r.Role, r.Brand)
	if !ok {
		return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, Action: "failed"}, nil
	}
	rc := BuildResolvedContext(r, lb, integration.Meta())
	after := integration.Calibrate(rc)
	if r.Role == integrations.ResourceRoleExport {
		after = writeConfigFiles(after)
	}
	afterStatus := scanBeaconStatus(after)
	if err := sc.setBeaconStatus(ctx, r.ID, afterStatus, after.Observations); err != nil {
		return ResourceCalibrateResult{}, err
	}
	action := "calibrated"
	if scanResult.BeaconStatus == models.BeaconStatusHealthy {
		action = "healthy"
	}
	if afterStatus == models.BeaconStatusFailed || after.Error != "" {
		action = "failed"
	}
	return ResourceCalibrateResult{Resource: r, Before: scanResult.Report, After: after, Action: action}, nil
}

func (sc *StarChart) scanTransponder(ctx context.Context, tp models.Transponder, lb LeveledBranch) (integrations.TransponderScanResult, error) {
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
	rc := BuildResolvedContext(tp, lb, integration.Meta())
	report := integration.Scan(rc)
	status := scanBeaconStatus(report)
	if err := sc.setBeaconStatus(ctx, tp.ID, status, report.Observations); err != nil {
		return integrations.TransponderScanResult{}, err
	}
	return integrations.TransponderScanResult{Transponder: tp, Report: report, BeaconStatus: status}, nil
}

func (sc *StarChart) calibrateTransponder(ctx context.Context, tp models.Transponder, lb LeveledBranch) (integrations.TransponderCalibrateResult, error) {
	tr, err := sc.scanTransponder(ctx, tp, lb)
	if err != nil {
		return integrations.TransponderCalibrateResult{}, err
	}
	if tr.BeaconStatus == models.BeaconStatusHealthy {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report, Action: "healthy"}, nil
	}
	integration, ok := sc.integrations.Get(tp.Role, tp.Brand)
	if !ok {
		return integrations.TransponderCalibrateResult{Transponder: tp, Report: tr.Report, Action: "failed"}, nil
	}
	rc := BuildResolvedContext(tp, lb, integration.Meta())
	after := integration.Calibrate(rc)
	afterStatus := scanBeaconStatus(after)
	if err := sc.setBeaconStatus(ctx, tp.ID, afterStatus, after.Observations); err != nil {
		return integrations.TransponderCalibrateResult{}, err
	}
	action := "calibrated"
	if afterStatus == models.BeaconStatusFailed || after.Error != "" {
		action = "failed"
	}
	return integrations.TransponderCalibrateResult{Transponder: tp, Report: after, Action: action}, nil
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
