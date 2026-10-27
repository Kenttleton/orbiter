package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// The vessel needs a shell to operate. A shell is a resource on the vessel like any other, so
// its drift is found by a scan like any other. Reconciling it is the Captain's choice: Orbiter
// looks for shells through the installed integrations (the same discovery as everywhere),
// shows what it found and what is missing, and replaces the shell with the one the Captain
// picks. Orbiter names no shell.

// shellDrift is a shell resource on the vessel that no longer works.
type shellDrift struct {
	resourceID string
	brand      string
	reason     string
}

// findShellDrift returns the vessel's shell if it has drifted: its integration is not
// installed, or its shell is not at the location recorded for it. A shell with no recorded
// location has not been found yet, which is the FIND flow's business and not drift.
func (e *Executor) findShellDrift(ctx context.Context, scan starchart.BranchScanResult) (shellDrift, bool) {
	for _, r := range scan.Resources {
		if r.Resource.Role != integrations.ResourceRoleShell || r.BeaconStatus == models.BeaconStatusHealthy {
			continue
		}
		if _, installed := e.sc.Integration(r.Resource.Role, r.Resource.Brand); !installed {
			return shellDrift{r.Resource.ID, r.Resource.Brand, "its integration folder is not in the integrations directory"}, true
		}
		recorded, err := e.sc.RecordedBinaries(ctx, r.Resource.ID)
		if err != nil || len(recorded) == 0 {
			continue
		}
		var path string
		for _, p := range recorded {
			path = p
			break
		}
		return shellDrift{r.Resource.ID, r.Resource.Brand, fmt.Sprintf("it was not found at its recorded location %s", path)}, true
	}
	return shellDrift{}, false
}

// knownShells asks the installed integrations which shells they find here: the vessel's
// discovery, keeping the suggestions that operate as shells, sorted so the order is stable.
func knownShells(ctx context.Context, sc *starchart.StarChart) ([]integrations.SuggestedResource, error) {
	suggestions, err := sc.DiscoverVessel(ctx)
	if err != nil {
		return nil, err
	}
	var shells []integrations.SuggestedResource
	for _, s := range suggestions {
		if s.Role == integrations.ResourceRoleShell {
			shells = append(shells, s)
		}
	}
	slices.SortFunc(shells, func(a, b integrations.SuggestedResource) int { return strings.Compare(a.Brand, b.Brand) })
	return shells, nil
}

// handleShellDrift flags a drifted shell and, when the installed integrations find shells,
// asks the Captain to choose one. Answered on the spot (styled terminal, or a client attached
// with -i) the choice is applied immediately. Otherwise the prompt stays pending in a session:
// the returned directives tell the client to answer with relay and continue with uplink.
func (e *Executor) handleShellDrift(ctx context.Context, alias models.Alias, scan starchart.BranchScanResult) ([]Directive, error) {
	drift, ok := e.findShellDrift(ctx, scan)
	if !ok {
		return nil, nil
	}
	head := fmt.Sprintf("The vessel's shell %q has drifted: %s.", drift.brand, drift.reason)

	known, err := knownShells(ctx, e.sc)
	if err != nil {
		return nil, fmt.Errorf("look for shells: %w", err)
	}
	if len(known) == 0 {
		e.renderer.Warning(head + " No shell integration in the integrations directory found a shell here ('orbiter vessel init' copies the bundled ones there).")
		return nil, nil
	}
	options := make([]string, 0, len(known))
	for _, k := range known {
		options = append(options, k.Brand)
	}
	message := fmt.Sprintf("%s\nShells found by the integrations in the integrations directory: %s.\nChoose the shell this vessel should use.", head, strings.Join(options, ", "))

	meta, _ := json.Marshal(map[string]string{"op": "calibrate", "target": alias.Name})
	session, err := e.sc.OpenSession(ctx, string(meta))
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	txn := models.NewID("tx")
	prompt := output.CaptainPrompt{Txn: txn, Op: "SELECT", Variant: "single", Message: message, Options: options}
	args, _ := json.Marshal(map[string]any{"kind": "shell", "resource_id": drift.resourceID, "brand": drift.brand, "options": options})
	payload, _ := json.Marshal(prompt)
	if err := e.sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txn, SessionID: session, Op: "SELECT", Args: string(args),
		Status: starchart.TransactionStatusPending, Payload: string(payload),
	}); err != nil {
		return nil, fmt.Errorf("record the choice: %w", err)
	}

	resp, err := e.renderer.Prompt(prompt)
	if err != nil {
		e.renderer.Warning(head + " No choice was made, so the shell is unchanged.")
		_ = e.sc.UpdateSessionStatus(ctx, session, "failed")
		return nil, nil
	}
	if resp == "" {
		return []Directive{{Op: "SESSION", Key: session}}, nil // answered later through relay and uplink
	}
	if err := e.sc.RelayTransaction(ctx, txn, resp); err != nil {
		return nil, err
	}
	var answer struct {
		Selected string `json:"selected"`
	}
	if err := json.Unmarshal([]byte(resp), &answer); err != nil || answer.Selected == "" {
		return nil, fmt.Errorf("the answer to the shell choice names no shell")
	}
	if err := reconcileShell(ctx, e.sc, drift.resourceID, drift.brand, answer.Selected); err != nil {
		return nil, err
	}
	_ = e.sc.MarkTransactionProcessed(ctx, txn)
	_ = e.sc.UpdateSessionStatus(ctx, session, "complete")
	e.renderer.Success(fmt.Sprintf("%s replaced by %s as this vessel's shell", drift.brand, answer.Selected))
	return nil, nil
}

// reconcileShell replaces the drifted shell with the one the Captain chose: the chosen shell
// is registered and attached to the vessel through the same step discovery always uses, and the
// drifted one is retired, as `orbiter retro` would. Choosing the shell that is already
// recorded keeps it.
func reconcileShell(ctx context.Context, sc *starchart.StarChart, oldResourceID, oldBrand, chosen string) error {
	if chosen == oldBrand {
		return nil
	}
	known, err := knownShells(ctx, sc)
	if err != nil {
		return err
	}
	var pick *integrations.SuggestedResource
	for i := range known {
		if known[i].Brand == chosen {
			pick = &known[i]
		}
	}
	if pick == nil {
		return fmt.Errorf("shell %q is no longer offered by the installed integrations", chosen)
	}
	vessel, err := sc.GetVessel(ctx)
	if err != nil {
		return fmt.Errorf("find the vessel: %w", err)
	}
	for _, o := range registerSuggestions(ctx, sc, []integrations.SuggestedResource{*pick}, vessel.ID) {
		if o.Outcome != "attached" {
			return fmt.Errorf("register shell %s: %s", o.Brand, o.Error)
		}
	}
	plan, err := sc.PlanRetro(ctx, oldResourceID)
	if err != nil {
		return fmt.Errorf("plan retiring %s: %w", oldBrand, err)
	}
	if err := sc.ExecuteRetro(ctx, plan); err != nil {
		return fmt.Errorf("retire %s: %w", oldBrand, err)
	}
	return nil
}
