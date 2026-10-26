package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

// uplinker processes relayed transactions for one session. Each interaction is a
// breakpoint: a result that needs the Captain creates a new transaction in the
// same session; uplink returns once it is emitted (async) or answered (styled).
type uplinker struct {
	d         *deps
	sessionID string
	errw      io.Writer
}

// findArgs is the JSON stored in a FIND-originated transaction's args.
type findArgs struct {
	ResourceID string   `json:"resource_id"`
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	New        string   `json:"new"`
	Old        string   `json:"old"`
	Matches    []string `json:"matches"`
}

func parseFindArgs(args string) findArgs {
	var a findArgs
	_ = json.Unmarshal([]byte(args), &a)
	return a
}

// findBinary handles a relayed FIND_BINARY result.
func (u *uplinker) findBinary(ctx context.Context, tx starchart.Transaction) error {
	a := parseFindArgs(tx.Args)
	res, err := starchart.ParseFindResponse(tx.Op, tx.Response)
	if err != nil {
		return err
	}
	if len(res.Matches) == 0 {
		return fmt.Errorf("%s not found by the shell", a.Key)
	}
	if len(res.Matches) > 1 {
		options := make([]string, 0, len(res.Matches))
		for _, m := range res.Matches {
			options = append(options, m.IntoNative())
		}
		return u.ask(ctx, output.CaptainPrompt{
			Op:      "SELECT",
			Variant: "single",
			Message: fmt.Sprintf("Multiple locations found for %s", a.Key),
			Options: options,
		}, map[string]any{"resource_id": a.ResourceID, "key": a.Key, "matches": options})
	}

	found := res.Matches[0]
	old, err := u.d.sc.RecordedBinary(ctx, a.ResourceID, a.Key)
	if err != nil {
		return err
	}
	if old == "" {
		return u.record(ctx, a.ResourceID, a.Key, found.Unix)
	}
	if prev, perr := integrations.ParsePath(old); perr == nil && prev.Unix == found.Unix {
		return nil // already recorded at this path; nothing to change
	}
	// A different path is already recorded: never replace it without the Captain.
	return u.ask(ctx, output.CaptainPrompt{
		Op:      "CONFIRM",
		Message: fmt.Sprintf("%s moved from %s to %s, update?", a.Key, old, found.Unix),
	}, map[string]any{"resource_id": a.ResourceID, "key": a.Key, "new": found.Unix, "old": old})
}

// selected handles a relayed SELECT that originated from a FIND.
func (u *uplinker) selected(ctx context.Context, tx starchart.Transaction) error {
	a := parseFindArgs(tx.Args)
	var r struct {
		Selected string `json:"selected"`
	}
	if err := json.Unmarshal([]byte(tx.Response), &r); err != nil || r.Selected == "" {
		return fmt.Errorf("SELECT response has no selection")
	}
	return u.record(ctx, a.ResourceID, a.Key, r.Selected)
}

// confirmed handles a relayed CONFIRM that originated from a FIND.
func (u *uplinker) confirmed(ctx context.Context, tx starchart.Transaction) error {
	a := parseFindArgs(tx.Args)
	var r struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := json.Unmarshal([]byte(tx.Response), &r); err != nil {
		return fmt.Errorf("CONFIRM response is not valid JSON: %w", err)
	}
	if !r.Confirmed {
		return nil // declined: the recorded path stays as it is
	}
	return u.record(ctx, a.ResourceID, a.Key, a.New)
}

func (u *uplinker) record(ctx context.Context, resourceID, name, path string) error {
	return u.d.sc.RecordBinary(ctx, resourceID, name, path)
}

// ask opens a new transaction in this session and prompts the Captain. A styled
// renderer answers synchronously and the answer is processed straight away; a
// JSON renderer returns no answer and the transaction waits for relay.
func (u *uplinker) ask(ctx context.Context, p output.CaptainPrompt, args map[string]any) error {
	txn := models.NewID("tx")
	p.Txn = txn
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := u.d.sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txn, SessionID: u.sessionID, Op: p.Op, Args: string(argsJSON),
		Status: starchart.TransactionStatusPending, Payload: string(payload),
	}); err != nil {
		return err
	}
	resp, err := u.d.renderer.Prompt(p)
	if err != nil {
		return fmt.Errorf("prompt %s (transaction %s stays pending): %w", p.Op, txn, err)
	}
	if resp == "" {
		return nil // answer arrives later through orbiter relay
	}
	if err := u.d.sc.RelayTransaction(ctx, txn, resp); err != nil {
		return err
	}
	tx, err := u.d.sc.GetTransaction(ctx, txn)
	if err != nil {
		return err
	}
	if err := u.process(ctx, tx); err != nil {
		fmt.Fprintf(u.errw, "uplink: transaction %s (%s): %v\n", tx.ID, tx.Op, err)
	}
	return u.d.sc.MarkTransactionProcessed(ctx, txn)
}
