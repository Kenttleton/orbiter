package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

func TestRelayCmd_storesResponse(t *testing.T) {
	sc, sessID, txnID := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID, "--response={\"confirmed\":true}"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, err := sc.GetRelayedTransactions(context.Background(), sessID)
	if err != nil {
		t.Fatalf("GetRelayedTransactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("len = %d, want 1", len(txns))
	}
	if txns[0].Response != `{"confirmed":true}` {
		t.Errorf("Response = %q", txns[0].Response)
	}
}

func TestRelayCmd_stdin_pipe(t *testing.T) {
	sc, sessID, txnID := setupRelayDB(t, "SELECT")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID})
	root.SetIn(strings.NewReader(`{"selected":"/usr/bin/brew"}`))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 1 || txns[0].Response != `{"selected":"/usr/bin/brew"}` {
		t.Errorf("unexpected relay state: %+v", txns)
	}
}

func TestRelayCmd_idempotent(t *testing.T) {
	sc, _, txnID := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=" + txnID, `--response={"confirmed":true}`})
	root.Execute()
	// relay again — should not error
	root2 := commands.NewRootCommandForTest(sc)
	root2.SetArgs([]string{"relay", "--txn=" + txnID, `--response={"confirmed":false}`})
	var errBuf bytes.Buffer
	root2.SetErr(&errBuf)
	if err := root2.Execute(); err != nil {
		t.Fatalf("second relay: %v", err)
	}
}

func TestRelayCmd_missingTxn(t *testing.T) {
	sc, _, _ := setupRelayDB(t, "CONFIRM")
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"relay", "--txn=nonexistent000000", `--response={"confirmed":true}`})
	// should exit 0 (idempotent — unknown txn is treated as already processed)
	if err := root.Execute(); err != nil {
		t.Errorf("missing txn should not error: %v", err)
	}
}

func setupRelayDB(t *testing.T, op string) (*starchart.StarChart, string, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{}`)
	txnID := models.NewID("tx")
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: op,
		Args: `{}`, Status: starchart.TransactionStatusPending,
	})
	return sc, sessID, txnID
}
