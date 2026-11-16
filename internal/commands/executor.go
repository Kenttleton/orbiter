package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bundle "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// Directive is a single neutral instruction for the shell wrapper to interpret.
// Op is one of "DIR" (change directory), "SET" (export env var), "UNSET" (unset env var),
// "DEPART", "SESSION" (session ID in Key), or "FIND_BINARY|FIND_FILE|FIND_DIR|FIND_ENV"
// (a discovery request tagged with Txn).
// String() emits the line-protocol format: "OP KEY=VALUE", "OP PATH", or "OP TXN KEY [VALUE]".
type Directive struct {
	Op    string // "DIR", "SET", "UNSET", "SESSION", "FIND_*"
	Txn   string // FIND_*: transaction ID
	Key   string // SET/UNSET: variable name; SESSION: session ID; FIND_*: name, pattern or path
	Value string // DIR: path, SET: variable value, FIND_FILE: optional location
}

func (d Directive) String() string {
	// A line break in any field would smuggle a second directive into the shell.
	if strings.ContainsAny(d.Txn+d.Key+d.Value, "\r\n") && (d.Op == "SESSION" || strings.HasPrefix(d.Op, "FIND_")) {
		return ""
	}
	switch d.Op {
	case "DIR":
		return "DIR " + d.Value
	case "SET":
		key := strings.ReplaceAll(d.Key, "\n", "\\n")
		val := strings.ReplaceAll(d.Value, "\n", "\\n")
		return "SET " + key + "=" + val
	case "UNSET":
		return "UNSET " + d.Key
	case "DEPART":
		return "DEPART"
	case "SESSION":
		return "SESSION " + d.Key
	case "FIND_BINARY", "FIND_ENV", "FIND_DIR":
		return d.Op + " " + d.Txn + " " + d.Key
	case "FIND_FILE":
		if d.Value != "" {
			return "FIND_FILE " + d.Txn + " " + d.Key + " " + d.Value
		}
		return "FIND_FILE " + d.Txn + " " + d.Key
	}
	return ""
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

// Hook resolves cwd to a planet and returns directives for the shell hook to eval.
// Returns []Directive{{Op:"DEPART"}} if currentPlanet is set but cwd no longer matches any planet.
// Returns SET ORBITER_PLANET + any allowed shell exports if a new planet is entered.
// Returns nil, nil when no context change occurred (cwd not a planet and none was active).
// Returns non-nil error only on infrastructure failures (DB, I/O); callers surface to stderr.
func (e *Executor) Hook(ctx context.Context, cwd, currentPlanet string) ([]Directive, error) {
	cwd = filepath.Clean(cwd)

	alias, err := e.sc.ResolveCWD(ctx, cwd)
	if err != nil {
		if currentPlanet != "" {
			return []Directive{{Op: "DEPART"}}, nil
		}
		return nil, nil
	}

	if alias.ID == currentPlanet {
		return nil, nil
	}

	directives := []Directive{{Op: "SET", Key: "ORBITER_PLANET", Value: alias.ID}}

	scanResult, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return directives, nil
	}
	for _, r := range scanResult.Resources {
		i, ok := integrations.Default.Get(r.Resource.Role, r.Resource.Brand)
		if !ok {
			continue
		}
		allowedEnvs := i.Meta().Shell.AllowedEnvs()
		allowed := make(map[string]bool, len(allowedEnvs))
		for _, k := range allowedEnvs {
			allowed[k] = true
		}
		for k, v := range r.Report.Exports {
			if allowed[k] {
				directives = append(directives, Directive{Op: "SET", Key: k, Value: v})
			}
		}
	}
	return directives, nil
}

// isIntegrationFolder returns true if target names a folder in the integrations directory.
func isIntegrationFolder(target string) bool {
	_, err := os.Stat(filepath.Join(bundle.DefaultIntegrationsDir(), target, "manifest.toml"))
	return err == nil
}

// Survey renders the desired state and last beacon for the target entity.
// If target is an integration in the integrations folder, it shows integration state instead.
func (e *Executor) Survey(ctx context.Context, target string, opts SurveyOptions) error {
	if target != "" && isIntegrationFolder(target) {
		info, err := inspectBrand(ctx, e.sc, target)
		if err != nil {
			return err
		}
		reportInspect(e.renderer, info)
		return nil
	}

	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	if err := e.surveySummary(ctx, alias); err != nil {
		return err
	}
	return e.finishSurvey(ctx, alias.ID, alias.Name, opts)
}

// surveySummary prints the target's own summary tables (effective state, including
// what is inherited through the hierarchy).
func (e *Executor) surveySummary(ctx context.Context, alias models.Alias) error {
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
	items := []ScanItem{}
	for _, tr := range result.Transponders {
		items = append(items, transponderScanItem(tr))
	}
	defer e.renderer.Result(output.Result{Success: true, Message: "scan " + alias.Name,
		Data: ScanResult{Target: alias.Name, TargetID: alias.ID, Items: items}})
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

func transponderScanItem(tr integrations.TransponderScanResult) ScanItem {
	return ScanItem{Kind: "transponder", ID: tr.Transponder.ID, Role: tr.Transponder.Role,
		Brand: tr.Transponder.Brand, Status: tr.BeaconStatus, Report: redactedReport(tr.Report)}
}

func (e *Executor) scanTransponderEntity(ctx context.Context, alias models.Alias) error {
	tr, err := e.sc.ScanTransponder(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan transponder %s: %w", alias.Name, err)
	}
	defer e.renderer.Result(output.Result{Success: true, Message: "scan " + alias.Name,
		Data: ScanResult{Target: alias.Name, TargetID: alias.ID, Items: []ScanItem{transponderScanItem(tr)}}})
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
	items := []ScanItem{}
	for _, r := range result.Resources {
		items = append(items, ScanItem{Kind: "resource", ID: r.Resource.ID, Role: r.Resource.Role,
			Brand: r.Resource.Brand, Status: r.BeaconStatus, Report: redactedReport(r.Report)})
	}
	for _, tr := range result.Transponders {
		items = append(items, transponderScanItem(tr))
	}
	defer e.renderer.Result(output.Result{Success: true, Message: "scan " + alias.Name,
		Data: ScanResult{Target: alias.Name, TargetID: alias.ID, Items: items}})
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

	// chart always ends in a plan, even an empty one (styled mode prints only the
	// message below for an empty plan).
	if len(result.Resources) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no resources to chart", alias.Name))
		e.renderer.Plan(nil)
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
// Returns neutral directives for the shell wrapper to interpret after jump.
// Human-readable output is written to stderr; directives to stdout.
// If confirmed is false, renders the plan and prompts interactively.
func (e *Executor) Jump(ctx context.Context, target string, confirmed bool) ([]Directive, error) {
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

	// Phase 4: build neutral directives.
	var directives []Directive

	for _, r := range calibResult.Resources {
		if r.Resource.Role != integrations.ResourceRoleFilesystem {
			continue
		}
		dir := r.After.InstallDir
		if dir == "" {
			dir = r.Before.InstallDir
		}
		if dir != "" {
			directives = append(directives, Directive{Op: "DIR", Value: dir})
			break
		}
	}

	// Collect shell exports from all calibrated resources (filtered by manifest allowlist).
	for _, r := range calibResult.Resources {
		i, ok := e.sc.Integration(r.Resource.Role, r.Resource.Brand)
		if !ok {
			continue
		}
		allowedEnvs := i.Meta().Shell.AllowedEnvs()
		allowed := make(map[string]bool, len(allowedEnvs))
		for _, k := range allowedEnvs {
			allowed[k] = true
		}
		after := r.After.Exports
		if after == nil {
			after = r.Before.Exports
		}
		for k, v := range after {
			if allowed[k] {
				directives = append(directives, Directive{Op: "SET", Key: k, Value: v})
			}
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

	nodes := make([]RetroNode, 0, len(plan.Nodes))
	for _, n := range plan.Nodes {
		nodes = append(nodes, RetroNode{Name: n.Name, Action: n.Action})
	}
	retired := false
	defer func() {
		e.renderer.Result(output.Result{Success: true, Message: "retro " + alias.Name,
			Data: RetroResult{Target: alias.Name, Retired: retired, Nodes: nodes}})
	}()

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

	retired = true
	e.renderer.Success(fmt.Sprintf("%s retired", alias.Name))
	return nil
}

// Calibrate reconciles drift for the target entity.
func (e *Executor) Calibrate(ctx context.Context, target string) ([]Directive, error) {
	return e.calibrate(ctx, target, false)
}

// CalibrateAfterFind is Calibrate for a flow that has already been through a FIND
// round: discovery is not started again, so a binary the Captain could not or
// would not resolve cannot block calibration or loop through endless sessions.
func (e *Executor) CalibrateAfterFind(ctx context.Context, target string) ([]Directive, error) {
	return e.calibrate(ctx, target, true)
}

func (e *Executor) calibrate(ctx context.Context, target string, skipFinds bool) ([]Directive, error) {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	parsed, _ := models.ParseID(alias.ID)
	switch parsed.EntityType {
	case models.EntityTypeCallsign:
		result, err := e.sc.CalibrateCallsign(ctx, alias.ID)
		if err != nil {
			return nil, fmt.Errorf("calibrate callsign %s: %w", alias.Name, err)
		}
		items := []CalibrateItem{}
		for _, tr := range result.Transponders {
			items = append(items, transponderCalibrateItem(tr))
		}
		defer e.renderer.Result(output.Result{Success: true, Message: "calibrate " + alias.Name,
			Data: CalibrateResult{Target: alias.Name, TargetID: alias.ID, Items: items}})
		if len(result.Transponders) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil, nil
		}
		var rows [][]string
		for _, tr := range result.Transponders {
			obs := ""
			if tr.Report.Error != "" {
				obs = tr.Report.Error
			}
			rows = append(rows, []string{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.Action, obs})
		}
		e.renderer.Table([]string{"transponder", "action", "observation"}, rows)
		return nil, nil
	case models.EntityTypeTransponder:
		tr, err := e.sc.CalibrateTransponder(ctx, alias.ID)
		if err != nil {
			return nil, fmt.Errorf("calibrate transponder %s: %w", alias.Name, err)
		}
		defer e.renderer.Result(output.Result{Success: true, Message: "calibrate " + alias.Name,
			Data: CalibrateResult{Target: alias.Name, TargetID: alias.ID, Items: []CalibrateItem{transponderCalibrateItem(tr)}}})
		obs := ""
		if tr.Report.Error != "" {
			obs = tr.Report.Error
		}
		e.renderer.Table([]string{"transponder", "action", "observation"}, [][]string{
			{tr.Transponder.Role + "/" + tr.Transponder.Brand, tr.Action, obs},
		})
		return nil, nil
	default:
		scan, err := e.sc.ScanBranch(ctx, alias.ID)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", alias.Name, err)
		}
		// Discovery first: a binary with no recorded path, or one its integration reports
		// not-present, is resolved through a FIND round trip before any calibration runs.
		// uplink continues the flow once the Captain has decided.
		if !skipFinds {
			if needed := e.sc.NeededFinds(scan); len(needed) > 0 {
				return e.requestFinds(ctx, "calibrate", alias.Name, needed)
			}
		}
		// A shell that has drifted (its integration gone, or the shell no longer where it was
		// recorded) is the Captain's choice. FIND has had its turn by now.
		if pending, err := e.handleShellDrift(ctx, alias, scan); err != nil || len(pending) > 0 {
			return pending, err
		}
		result, err := e.sc.CalibrateBranch(ctx, alias.ID)
		if err != nil {
			return nil, fmt.Errorf("calibrate %s: %w", alias.Name, err)
		}
		items := []CalibrateItem{}
		for _, r := range result.Resources {
			before := redactedReport(r.Before)
			items = append(items, CalibrateItem{Kind: "resource", ID: r.Resource.ID, Role: r.Resource.Role,
				Brand: r.Resource.Brand, Action: r.Action, Before: &before, After: redactedReport(r.After)})
		}
		defer e.renderer.Result(output.Result{Success: true, Message: "calibrate " + alias.Name,
			Data: CalibrateResult{Target: alias.Name, TargetID: alias.ID, Items: items}})
		if len(result.Resources) == 0 {
			e.renderer.Info(fmt.Sprintf("%s: nothing to calibrate", alias.Name))
			return nil, nil
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
		return nil, nil
	}
}

// requestFinds opens a session recording what to continue (meta: op + target),
// writes one pending transaction per request, and returns the directives that ask
// the shell to run them. The rows exist before the directives are returned.
func (e *Executor) requestFinds(ctx context.Context, op, target string, needed []starchart.ResourceFind) ([]Directive, error) {
	meta, err := json.Marshal(map[string]string{"op": op, "target": target})
	if err != nil {
		return nil, err
	}
	sessionID := models.NewID("ss")
	if err := e.sc.InsertSession(ctx, sessionID, string(meta)); err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	directives := []Directive{{Op: "SESSION", Key: sessionID}}
	for _, n := range needed {
		args, err := json.Marshal(map[string]string{
			"resource_id": n.ResourceID,
			"key":         n.Request.Key,
			"value":       n.Request.Value,
		})
		if err != nil {
			return nil, err
		}
		txn := models.NewID("tx")
		if err := e.sc.InsertTransaction(ctx, starchart.Transaction{
			ID: txn, SessionID: sessionID, Op: n.Request.Op, Args: string(args),
			Status: starchart.TransactionStatusPending,
		}); err != nil {
			return nil, fmt.Errorf("record %s: %w", n.Request.Op, err)
		}
		directives = append(directives, Directive{Op: n.Request.Op, Txn: txn, Key: n.Request.Key, Value: n.Request.Value})
	}
	e.renderer.Info(fmt.Sprintf("%s: locating %d binary path(s) before calibrating", target, len(needed)))
	return directives, nil
}

func transponderCalibrateItem(tr integrations.TransponderCalibrateResult) CalibrateItem {
	return CalibrateItem{Kind: "transponder", ID: tr.Transponder.ID, Role: tr.Transponder.Role,
		Brand: tr.Transponder.Brand, Action: tr.Action, After: redactedReport(tr.Report)}
}
