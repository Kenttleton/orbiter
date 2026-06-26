package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/spf13/cobra"
)

func newUplinkCmd(d *deps) *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "uplink",
		Short: "Process relayed transactions for a session and continue the operation",
		Long: `Process all relayed transactions for the given session.
Handles SELECT, CONFIRM, and TEXT prompt responses.
FIND_* op processing is deferred to the FIND spec implementation.`,
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

			for _, tx := range txns {
				if err := processTransaction(ctx, tx, cmd); err != nil {
					// Non-fatal: log and continue.
					fmt.Fprintf(cmd.ErrOrStderr(), "uplink: transaction %s (%s): %v\n", tx.ID, tx.Op, err)
				}
				_ = d.sc.MarkTransactionProcessed(ctx, tx.ID)
			}

			if err := d.sc.UpdateSessionStatus(ctx, sessionID, "complete"); err != nil {
				return fmt.Errorf("update session: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to process (required)")
	return cmd
}

// processTransaction handles one relayed transaction based on its op type.
func processTransaction(ctx context.Context, tx starchart.Transaction, cmd *cobra.Command) error {
	switch tx.Op {
	case "FIND_BINARY", "FIND_FILE", "FIND_DIR", "FIND_ENV":
		// FIND op processing is implemented by the FIND spec plan.
		// For now: log the stub and mark processed.
		fmt.Fprintf(cmd.OutOrStdout(), "uplink: FIND op %s stub — will be handled by FIND spec implementation\n", tx.Op)
		return nil

	case "CONFIRM":
		// Record the decision. Future exec model refactor wires this to resume WASM execution.
		return nil

	case "SELECT":
		// Record the selection. Future exec model refactor wires this to resume operation.
		return nil

	case "TEXT":
		// Record the input. Future exec model refactor wires this to resume operation.
		return nil

	default:
		return fmt.Errorf("unknown op %q", tx.Op)
	}
}
