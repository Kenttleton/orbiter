package commands_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

func TestUplinkCmd_processesConfirm(t *testing.T) {
	sc, sessID, txnID := setupUplinkDB(t, "CONFIRM", `{"confirmed":true}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Transaction should be processed.
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 0 {
		t.Errorf("expected 0 relayed transactions after uplink, got %d", len(txns))
	}
	sess, _ := sc.GetSession(context.Background(), sessID)
	if sess.Status != "complete" {
		t.Errorf("session status = %q, want complete", sess.Status)
	}
	_ = txnID
}

func TestUplinkCmd_processesSelect(t *testing.T) {
	sc, sessID, txnID := setupUplinkDB(t, "SELECT", `{"selected":"/usr/bin/brew"}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	txns, _ := sc.GetRelayedTransactions(context.Background(), sessID)
	if len(txns) != 0 {
		t.Errorf("expected 0 relayed after uplink, got %d", len(txns))
	}
	_ = txnID
}

func TestUplinkCmd_processesText(t *testing.T) {
	sc, sessID, _ := setupUplinkDB(t, "TEXT", `{"value":"mysecret"}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestUplinkCmd_FIND_stub(t *testing.T) {
	// FIND ops should not error — they are stubbed for now.
	sc, sessID, _ := setupUplinkDB(t, "FIND_BINARY", `{"status":"ok","matches":["/usr/bin/brew"]}`)
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=" + sessID})
	if err := root.Execute(); err != nil {
		t.Fatalf("FIND_BINARY stub should not error: %v", err)
	}
}

func TestUplinkCmd_missingSession(t *testing.T) {
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.Close() })
	root := commands.NewRootCommandForTest(sc)
	root.SetArgs([]string{"uplink", "--session=nonexistentss000"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func setupUplinkDB(t *testing.T, op, relayResponse string) (*starchart.StarChart, string, string) {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	sessID := models.NewID("ss")
	sc.InsertSession(ctx, sessID, `{"op":"TEST"}`)
	txnID := models.NewID("tx")
	argsJSON, _ := json.Marshal(map[string]string{"name": "brew"})
	sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txnID, SessionID: sessID, Op: op,
		Args: string(argsJSON), Status: starchart.TransactionStatusPending,
	})
	sc.RelayTransaction(ctx, txnID, relayResponse)
	return sc, sessID, txnID
}
