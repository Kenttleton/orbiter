package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newRelayCmd(d *deps) *cobra.Command {
	var txnID string
	var response string

	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Store a relay response for a pending transaction",
		Long: `Store the client's JSON response for a pending transaction, advancing it to 'relayed'.
Accepts --response=<json> or reads JSON from stdin when --response is omitted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if txnID == "" {
				return fmt.Errorf("--txn is required")
			}

			payload := response
			if payload == "" {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				payload = string(b)
			}
			if payload == "" {
				return fmt.Errorf("response required: provide --response=<json> or pipe JSON to stdin")
			}

			ctx := cmd.Context()

			// Clean up stale transactions opportunistically.
			_ = d.sc.DeleteStaleTransactions(ctx)

			if err := d.sc.RelayTransaction(ctx, txnID, payload); err != nil {
				return fmt.Errorf("relay: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&txnID, "txn", "", "transaction ID to relay (required)")
	cmd.Flags().StringVar(&response, "response", "", "JSON response payload (reads stdin if omitted)")
	return cmd
}
