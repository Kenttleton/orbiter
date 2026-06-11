package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

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

// Survey renders the desired state and last beacon for the target entity.
func (e *Executor) Survey(ctx context.Context, target string) error {
	alias, err := e.resolveTarget(ctx, target)
	if err != nil {
		return err
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

	result, err := e.sc.ScanBranch(ctx, alias.ID)
	if err != nil {
		return fmt.Errorf("scan %s: %w", alias.Name, err)
	}

	var rows [][]string
	for _, r := range result.Resources {
		obs := ""
		if r.Report.Error != "" {
			obs = r.Report.Error
		} else if len(r.Report.Observations) > 0 {
			obs = r.Report.Observations[0]
		}
		rows = append(rows, []string{r.Resource.Role + "/" + r.Resource.Brand, r.BeaconStatus, obs})
	}

	if len(rows) == 0 {
		e.renderer.Info(fmt.Sprintf("%s: no resources attached", alias.Name))
		return nil
	}
	e.renderer.Table([]string{"resource", "status", "observation"}, rows)
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
