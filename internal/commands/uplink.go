package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newUplinkCmd(d *deps) *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "uplink",
		Short: "Process relayed transactions for a session and continue the operation",
		Long: `Process all relayed transactions for the given session.
Handles FIND_BINARY results (recording the resolved path, asking the Captain when
the result is ambiguous or would replace a recorded path) and SELECT, CONFIRM and
TEXT prompt responses. When no transactions are left pending, continues the
operation the session was opened for.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sessionID == "" {
				return fmt.Errorf("--session is required")
			}

			ctx := cmd.Context()

			sess, err := d.sc.GetSession(ctx, sessionID)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("session %q not found", sessionID)
			}
			if err != nil {
				return fmt.Errorf("get session: %w", err)
			}
			if sess.Status != "active" {
				return fmt.Errorf("session %q is not active (status: %s)", sessionID, sess.Status)
			}

			txns, err := d.sc.GetRelayedTransactions(ctx, sessionID)
			if err != nil {
				return fmt.Errorf("get transactions: %w", err)
			}

			if len(txns) == 0 {
				return nil // exit 0 silently
			}

			u := &uplinker{d: d, sessionID: sessionID, errw: cmd.ErrOrStderr()}
			for _, tx := range txns {
				if err := u.process(ctx, tx); err != nil {
					// Non-fatal: log and continue.
					fmt.Fprintf(cmd.ErrOrStderr(), "uplink: transaction %s (%s): %v\n", tx.ID, tx.Op, err)
				}
				_ = d.sc.MarkTransactionProcessed(ctx, tx.ID)
			}

			// A transaction still pending is a breakpoint waiting for the Captain:
			// the session stays active until the next relay + uplink.
			pending, err := d.sc.CountPendingTransactions(ctx, sessionID)
			if err != nil {
				return fmt.Errorf("count pending: %w", err)
			}
			if pending > 0 {
				return nil
			}

			if err := d.sc.UpdateSessionStatus(ctx, sessionID, "complete"); err != nil {
				return fmt.Errorf("update session: %w", err)
			}
			// Always continue the interrupted flow. The resumed run does not start
			// another FIND round, so an unresolved binary cannot block it or loop.
			return u.resume(ctx, cmd, sess.Meta)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to process (required)")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

// resume re-runs the operation the session was opened for, now that decisions are
// recorded in the StarChart. Sessions with an unknown op are left alone.
func (u *uplinker) resume(ctx context.Context, cmd *cobra.Command, meta string) error {
	var m struct {
		Op     string `json:"op"`
		Target string `json:"target"`
	}
	if json.Unmarshal([]byte(meta), &m) != nil || m.Op != "calibrate" {
		return nil
	}
	directives, err := NewExecutor(u.d.sc, u.d.renderer).CalibrateAfterFind(ctx, m.Target)
	if err != nil {
		return err
	}
	for _, dir := range directives {
		fmt.Fprintln(cmd.OutOrStdout(), dir.String())
	}
	return nil
}

// process dispatches one relayed transaction by op. FIND-originated SELECT and
// CONFIRM carry a resource_id; ones without it keep the original prompt behavior.
func (u *uplinker) process(ctx context.Context, tx starchart.Transaction) error {
	fromFind := parseFindArgs(tx.Args).ResourceID != ""
	switch tx.Op {
	case "FIND_BINARY":
		return u.findBinary(ctx, tx)
	case "FIND_FILE", "FIND_DIR", "FIND_ENV":
		return nil // not persisted yet: no integration declares [find]
	case "SELECT":
		if fromFind {
			return u.selected(ctx, tx)
		}
	case "CONFIRM":
		if fromFind {
			return u.confirmed(ctx, tx)
		}
	}
	return processTransaction(ctx, tx, u.d.renderer)
}

// processTransaction handles one relayed prompt transaction that is not tied to a
// FIND, by re-presenting it through the renderer.
func processTransaction(_ context.Context, tx starchart.Transaction, renderer output.Renderer) error {
	switch tx.Op {
	case "CONFIRM":
		_, err := renderer.Prompt(output.CaptainPrompt{
			Txn:     tx.ID,
			Op:      "CONFIRM",
			Message: tx.Payload,
		})
		return err

	case "SELECT":
		var args struct {
			Matches []string `json:"matches"`
		}
		_ = json.Unmarshal([]byte(tx.Args), &args)
		msg := tx.Payload
		if msg == "" {
			msg = "Multiple matches found"
		}
		_, err := renderer.Prompt(output.CaptainPrompt{
			Txn:     tx.ID,
			Op:      "SELECT",
			Message: msg,
			Options: args.Matches,
			Variant: "single",
		})
		return err

	case "TEXT":
		_, err := renderer.Prompt(output.CaptainPrompt{
			Txn:     tx.ID,
			Op:      "TEXT",
			Message: tx.Payload,
			Variant: "short",
		})
		return err

	default:
		return fmt.Errorf("unknown op %q", tx.Op)
	}
}
