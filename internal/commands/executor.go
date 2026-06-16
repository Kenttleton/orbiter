package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	bundle "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// ShellDirective is a single directive for the shell function to eval after jump.
type ShellDirective struct {
	Op    string // "cd" or "export"
	Key   string // env var name (export only)
	Value string
}

func (d ShellDirective) String() string {
	if d.Op == "cd" {
		return fmt.Sprintf("cd %q", d.Value)
	}
	return fmt.Sprintf("export %s=%q", d.Key, d.Value)
}

// Executor owns the shared pipeline for all six lifecycle commands.
type Executor struct {
	sc       *starchart.StarChart
	renderer output.Renderer
}

// NewExecutor constructs an Executor with the given StarChart and Renderer.
func NewExecutor(sc *starchart.StarChart, r output.Renderer) *Executor {
	return &Executor{sc: sc, renderer: r}
}

// SC returns the underlying StarChart (for tests that need to set up entities).
func (e *Executor) SC() *starchart.StarChart { return e.sc }

// resolveTarget resolves an explicit target name, or falls back to CWD-based resolution.
func (e *Executor) resolveTarget(ctx context.Context, target string) (models.Alias, error) {
	if target != "" {
		return e.sc.Resolve(ctx, target)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return models.Alias{}, fmt.Errorf("get working directory: %w", err)
	}
	alias, err := e.sc.ResolveCWD(ctx, cwd)
	if err != nil {
		return models.Alias{}, fmt.Errorf("no target found: not in a known entity directory (use an explicit target name)")
	}
	return alias, nil
}

// isCatalogBrand returns true if target matches a bundled integration brand.
func isCatalogBrand(target string) bool {
	for _, e := range bundle.CatalogEntries() {
		if e.Brand == target {
			return true
		}
	}
	return false
}

// Survey renders the desired state and last beacon for the target entity.
// If target is a known integration brand, it shows integration state instead.
func (e *Executor) Survey(ctx context.Context, target string) error {
	if target != "" && isCatalogBrand(target) {
		info := IntegrationInspectInfo(target, integrations.DefaultSettings, integrations.Default)
		WriteInspectReport(os.Stdout, info)
		return nil
	}

	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}

	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		tps, err := e.sc.TranspondersForCallsign(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("survey callsign %s: %w", alias.Name, err)
		}
		rows := [][]string{
			{"callsign", alias.Name},
			{"id", alias.ID},
			{"transponders", fmt.Sprintf("%d", len(tps))},
		}
		e.renderer.Table([]string{"field", "value"}, rows)
		if len(tps) > 0 {
			var tpRows [][]string
			for _, tp := range tps {
				b, _ := e.sc.GetBeacon(ctx, tp.ID)
				tpRows = append(tpRows, []string{tp.Role + "/" + tp.Brand, tp.ID, b.Status})
			}
			e.renderer.Table([]string{"transponder", "id", "status"}, tpRows)
		}
		return nil
	case models.EntityTypeTransponder:
		var tp models.Transponder
		if err := e.sc.Get(ctx, "transponders", alias.ID, &tp); err != nil {
			return fmt.Errorf("survey transponder %s: %w", alias.Name, err)
		}
		b, _ := e.sc.GetBeacon(ctx, tp.ID)
		rows := [][]string{
			{"transponder", alias.Name},
			{"id", alias.ID},
			{"role", tp.Role},
			{"brand", tp.Brand},
			{"status", b.Status},
		}
		e.renderer.Table([]string{"field", "value"}, rows)
		return nil
	}

	branch, err := e.sc.BranchCrawl(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("crawl %s: %w", alias.Name, err)
	}

	beacon, err := e.sc.GetBeacon(ctx, alias.ID)
	if err != nil && !errors.Is(err, starchart.ErrNotFound) {
		return fmt.Errorf("get beacon for %s: %w", alias.Name, err)
	}

	rows := [][]string{
		{"entity", alias.Name},
		{"id", alias.ID},
		{"status", beacon.Status},
		{"resources", fmt.Sprintf("%d", len(branch.Resources))},
		{"transponders", fmt.Sprintf("%d", len(branch.Transponders))},
		{"callsigns", fmt.Sprintf("%d", len(branch.Callsigns))},
	}
	e.renderer.Table([]string{"field", "value"}, rows)

	if len(branch.Resources) > 0 {
		var resRows [][]string
		for _, r := range branch.Resources {
			rb, _ := e.sc.GetBeacon(ctx, r.ID)
			resRows = append(resRows, []string{r.Role + "/" + r.Brand, r.ID, rb.Status})
		}
		e.renderer.Table([]string{"resource", "id", "status"}, resRows)
	}
	return nil
}

// Scan verifies current reality for the target entity and updates beacons.
func (e *Executor) Scan(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		return e.scanCallsignEntity(ctx, alias)
	case models.EntityTypeTransponder:
		return e.scanTransponderEntity(ctx, alias)
	default:
		return e.scanBranchEntity(ctx, alias)
	}
}

func (e *Executor) scanCallsignEntity(ctx context.Context, alias models.Alias) error {
	result, err := e.sc.ScanCallsign(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan callsign %s: %w", alias.Name, err)
	}
	if len(result.Transponders) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no transponders attached", alias.Name))
		return nil
	}
	var rows [][]string
	for _, tr := range result.Transponders {
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		} else if len(tr.Report.Observations) > 0 {
			obs = tr.Report.Observations[0]
		}
		rows = append(rows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs})
	}
	e.renderer.Table([]string{"transponder", "status", "observation"}, rows)
	return nil
}

func (e *Executor) scanTransponderEntity(ctx context.Context, alias models.Alias) error {
	tr, err := e.sc.ScanTransponder(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan transponder %s: %w", alias.Name, err)
	}
	obs := ""
	if tr.Report.Error != "" {
		obs = tr.Report.Error
	} else if len(tr.Report.Observations) > 0 {
		obs = tr.Report.Observations[0]
	}
	e.renderer.Table([]string{"transponder", "status", "observation"}, [][]string{
		{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs},
	})
	return nil
}

func (e *Executor) scanBranchEntity(ctx context.Context, alias models.Alias) error {
	result, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan %s: %w", alias.Name, err)
	}
	var resourceRows [][]string
	for _, r := range result.Resources {
		obs := ""
		if r.Report.Error != "" {
			obs = r.Report.Error
		} else if len(r.Report.Observations) > 0 {
			obs = r.Report.Observations[0]
		}
		resourceRows = append(resourceRows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.BeaconStatus, obs})
	}
	var transponderRows [][]string
	for _, tr := range result.Transponders {
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		} else if len(tr.Report.Observations) > 0 {
			obs = tr.Report.Observations[0]
		}
		transponderRows = append(transponderRows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.BeaconStatus, obs})
	}
	if len(resourceRows) == 0 && len(transponderRows) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no resources attached", alias.Name))
		return nil
	}
	if len(resourceRows) > 0 {
		e.renderer.Table([]string{"resource", "status", "observation"}, resourceRows)
	}
	if len(transponderRows) > 0 {
		e.renderer.Table([]string{"transponder", "status", "observation"}, transponderRows)
	}
	return nil
}

// Chart computes and renders the terraform-style plan for the target entity.
// Calls ScanBranch — beacons are updated as a side effect.
func (e *Executor) Chart(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}

	result, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("chart %s: %w", alias.Name, err)
	}

	if len(result.Resources) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no resources to chart", alias.Name))
		return nil
	}

	var steps []output.PlanStep
	for _, r := range result.Resources {
		desc := ""
		if r.Report.Error != "" {
			desc = r.Report.Error
		}
		steps = append(steps, output.PlanStep{
			Action:      beaconToAction(r.BeaconStatus),
			EntityType:  r.Resource.Role,
			Name:        r.Resource.Role + "/" + r.Resource.Brand,
			Description: desc,
		})
	}
	e.renderer.Plan(steps)
	return nil
}

// beaconToAction maps beacon status to a PlanStep action verb.
func beaconToAction(status string) string {
	switch status {
	case "healthy":
		return "no-op"
	case "drifted", "unverified":
		return "change"
	case "failed":
		return "add"
	default:
		return "change"
	}
}

// Jump executes a full transition to the target entity.
// Returns shell directives for the shell function to eval.
// Human-readable output is written to stderr; shell directives to stdout.
// If confirmed is false, renders the plan and prompts interactively.
func (e *Executor) Jump(ctx context.Context, target string, confirmed bool) ([]ShellDirective, error) {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}

	// Phase 1: chart — show what will happen.
	scanResult, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", alias.Name, err)
	}

	// Render plan to stderr.
	stderrRenderer := output.NewRendererTo(output.FormatStyled, false, os.Stderr)
	var steps []output.PlanStep
	for _, r := range scanResult.Resources {
		steps = append(steps, output.PlanStep{
			Action:     beaconToAction(r.BeaconStatus),
			EntityType: r.Resource.Role,
			Name:       r.Resource.Brand,
		})
	}
	if len(steps) > 0 {
		stderrRenderer.Plan(steps)
	}

	// Phase 2: confirm.
	if !confirmed {
		fmt.Fprintf(os.Stderr, "\nExecute maneuver? [y/N] ")
		var response string
		fmt.Fscanln(os.Stdin, &response)
		if response != "y" && response != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil, nil
		}
	}

	// Phase 3: execute — calibrate all drifted/failed resources.
	calibResult, err := e.sc.CalibrateBranch(ctx, alias.ID)
	if err != nil {
		return nil, fmt.Errorf("execute jump for %s: %w", alias.Name, err)
	}

	// Render execution results to stderr.
	var execRows [][]string
	for _, r := range calibResult.Resources {
		execRows = append(execRows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.Action})
	}
	if len(execRows) > 0 {
		stderrRenderer.Table([]string{"resource", "action"}, execRows)
	}

	// Phase 4: build shell directives.
	var directives []ShellDirective

	// cd directive: first filesystem resource with an InstallDir in After (calibrated)
	// or Before (already healthy — scan report has the dir, After is zero).
	for _, r := range calibResult.Resources {
		if r.Resource.Role != integrations.ResourceRoleFilesystem {
			continue
		}
		if r.After.InstallDir != "" {
			directives = append(directives, ShellDirective{Op: "cd", Value: r.After.InstallDir})
			break
		}
		if r.Before.InstallDir != "" {
			directives = append(directives, ShellDirective{Op: "cd", Value: r.Before.InstallDir})
			break
		}
	}

	return directives, nil
}

// Retro plans and optionally executes retirement of an entity.
// Human-readable output goes to the configured renderer.
// If confirmed is false, renders the plan and prompts interactively.
func (e *Executor) Retro(ctx context.Context, target string, confirmed bool) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}

	plan, err := e.sc.PlanRetro(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("plan retro for %s: %w", alias.Name, err)
	}

	if len(plan.Nodes) == 0 {
		e.renderer.Success(fmt.Sprintf("%s has no entities to retire", alias.Name))
		return nil
	}

	// Render the plan as a table.
	rows := make([][]string, len(plan.Nodes))
	for i, n := range plan.Nodes {
		rows[i] = []string{n.Name, n.Action}
	}
	e.renderer.Table([]string{"entity", "action"}, rows)

	if !confirmed {
		fmt.Fprintf(os.Stderr, "\nExecute retro? [y/N] ")
		var response string
		fmt.Fscanln(os.Stdin, &response)
		if response != "y" && response != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := e.sc.ExecuteRetro(ctx, plan); err != nil {
		return fmt.Errorf("execute retro for %s: %w", alias.Name, err)
	}

	e.renderer.Success(fmt.Sprintf("%s retired", alias.Name))
	return nil
}

// Calibrate reconciles drift for the target entity.
func (e *Executor) Calibrate(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		result, err := e.sc.CalibrateCallsign(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate callsign %s: %w", alias.Name, err)
		}
		if len(result.Transponders) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil
		}
		var rows [][]string
		for _, tr := range result.Transponders {
			obs := ""
			if tr.Report.Error != "" {
				obs = tr.Report.Error
			}
			rows = append(rows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, obs})
		}
		e.renderer.Table([]string{"transponder", "observation"}, rows)
		return nil
	case models.EntityTypeTransponder:
		tr, err := e.sc.CalibrateTransponder(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate transponder %s: %w", alias.Name, err)
		}
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		}
		e.renderer.Table([]string{"transponder", "observation"}, [][]string{
			{tr.Transponder.Role + "/" + tr.Transponder.Brand, obs},
		})
		return nil
	default:
		result, err := e.sc.CalibrateBranch(ctx, alias.ID)
		if err != nil {
			return fmt.Errorf("calibrate %s: %w", alias.Name, err)
		}
		if len(result.Resources) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil
		}
		var rows [][]string
		for _, r := range result.Resources {
			obs := ""
			if r.After.Error != "" {
				obs = r.After.Error
			} else if len(r.After.Observations) > 0 {
				obs = r.After.Observations[0]
			} else if len(r.Before.Observations) > 0 {
				obs = r.Before.Observations[0]
			}
			rows = append(rows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.Action, obs})
		}
		e.renderer.Table([]string{"resource", "action", "observation"}, rows)
		return nil
	}
}
