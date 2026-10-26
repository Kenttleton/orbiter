package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/models"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// findFixture is a session holding one relayed transaction whose args name a resource.
type findFixture struct {
	sc     *starchart.StarChart
	sess   string
	txn    string
	resID  string
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func setupFind(t *testing.T, op, config, args, response string) *findFixture {
	t.Helper()
	sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	ctx := context.Background()
	res, err := sc.CreateResource(ctx, "brew", "tool", "brew", "[]", config)
	require.NoError(t, err)
	sess := models.NewID("ss")
	require.NoError(t, sc.InsertSession(ctx, sess, `{"op":"TEST"}`))
	txn := models.NewID("tx")
	full := args
	if full == "" {
		full = `{"resource_id":"` + res.ID + `","key":"brew","value":""}`
	} else {
		full = `{"resource_id":"` + res.ID + `",` + args[1:]
	}
	require.NoError(t, sc.InsertTransaction(ctx, starchart.Transaction{
		ID: txn, SessionID: sess, Op: op, Args: full, Status: starchart.TransactionStatusPending,
	}))
	require.NoError(t, sc.RelayTransaction(ctx, txn, response))
	return &findFixture{sc: sc, sess: sess, txn: txn, resID: res.ID, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
}

// uplink runs `orbiter uplink` with a JSON renderer, so prompts stay pending.
func (f *findFixture) uplink(t *testing.T) error {
	t.Helper()
	root := commands.NewRootCommandForTestWithRenderer(f.sc, output.NewRendererTo(output.FormatJSON, false, f.stdout))
	root.SetOut(f.stdout)
	root.SetErr(f.stderr)
	root.SetArgs([]string{"uplink", "--session=" + f.sess})
	return root.Execute()
}

func (f *findFixture) recorded(t *testing.T) string {
	t.Helper()
	p, err := f.sc.RecordedBinary(context.Background(), f.resID, "brew")
	require.NoError(t, err)
	return p
}

func TestUplinkFind_SingleMatchRecordsBinary(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{}`, "", `{"status":"ok","matches":["/opt/homebrew/bin/brew"]}`)
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/opt/homebrew/bin/brew", f.recorded(t))
	sess, _ := f.sc.GetSession(context.Background(), f.sess)
	assert.Equal(t, "complete", sess.Status)
}

func TestUplinkFind_ZeroMatchesLeavesConfigAlone(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{}`, "", `{"status":"ok","matches":[]}`)
	require.NoError(t, f.uplink(t))
	assert.Empty(t, f.recorded(t))
	assert.Contains(t, f.stderr.String(), "not found")
}

func TestUplinkFind_TwoMatchesRaisesSelectAndStaysActive(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{}`, "", `{"status":"ok","matches":["/opt/homebrew/bin/brew","/usr/local/bin/brew"]}`)
	require.NoError(t, f.uplink(t))
	assert.Empty(t, f.recorded(t), "ambiguous result must not be recorded")
	sess, _ := f.sc.GetSession(context.Background(), f.sess)
	assert.Equal(t, "active", sess.Status)
	n, err := f.sc.CountPendingTransactions(context.Background(), f.sess)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a new SELECT transaction is pending")
	assert.Contains(t, f.stdout.String(), `"captain_prompt"`)
}

func TestUplinkFind_InvalidJSONChangesNothing(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{"keep":"me"}`, "", `not json at all`)
	require.NoError(t, f.uplink(t))
	assert.Empty(t, f.recorded(t))
}

func TestUplinkFind_DifferentRecordedPathRequiresConfirm(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{"binaries":{"brew":"/old/brew"}}`, "", `{"status":"ok","matches":["/opt/homebrew/bin/brew"]}`)
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/old/brew", f.recorded(t), "no auto-update on recovery")
	n, _ := f.sc.CountPendingTransactions(context.Background(), f.sess)
	assert.Equal(t, 1, n, "a CONFIRM transaction is pending")
}

func TestUplinkFind_ConfirmApprovedRecordsNewPath(t *testing.T) {
	f := setupFind(t, "CONFIRM", `{"binaries":{"brew":"/old/brew"}}`,
		`{"key":"brew","new":"/opt/homebrew/bin/brew","old":"/old/brew"}`, `{"confirmed":true}`)
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/opt/homebrew/bin/brew", f.recorded(t))
}

func TestUplinkFind_ConfirmDeclinedKeepsOldPath(t *testing.T) {
	f := setupFind(t, "CONFIRM", `{"binaries":{"brew":"/old/brew"}}`,
		`{"key":"brew","new":"/opt/homebrew/bin/brew","old":"/old/brew"}`, `{"confirmed":false}`)
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/old/brew", f.recorded(t))
}

func TestUplinkFind_SelectRecordsChosenPath(t *testing.T) {
	f := setupFind(t, "SELECT", `{}`, `{"key":"brew","matches":["/a/brew","/b/brew"]}`, `{"selected":"/b/brew"}`)
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/b/brew", f.recorded(t))
}

func TestUplinkFind_RelayToUnknownTxnChangesNothing(t *testing.T) {
	f := setupFind(t, "FIND_BINARY", `{}`, "", `{"status":"ok","matches":["/x/brew"]}`)
	root := commands.NewRootCommandForTest(f.sc)
	root.SetArgs([]string{"relay", "--txn=doesnotexisttx000", "--response", `{"status":"ok","matches":[]}`})
	require.NoError(t, root.Execute())
	// The real transaction is untouched and still processes normally.
	require.NoError(t, f.uplink(t))
	assert.Equal(t, "/x/brew", f.recorded(t))
}

// Merged-/usr Linux lists /bin and /usr/bin on PATH where /bin is a symlink: the
// same binary reached two ways is one candidate, not an ambiguity for the Captain.
func TestUplinkFind_SameFileThroughSymlinkIsNotAmbiguous(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "usr", "bin")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	real := filepath.Join(realDir, "brew")
	require.NoError(t, os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755))
	link := filepath.Join(root, "bin")
	require.NoError(t, os.Symlink(realDir, link))

	resp, _ := json.Marshal(map[string]any{"status": "ok", "matches": []string{
		real, filepath.Join(link, "brew"), realDir + "//brew",
	}})
	f := setupFind(t, "FIND_BINARY", `{}`, "", string(resp))
	require.NoError(t, f.uplink(t))

	assert.Equal(t, filepath.ToSlash(real), f.recorded(t), "recorded without asking")
	n, err := f.sc.CountPendingTransactions(context.Background(), f.sess)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "no SELECT for one file reached several ways")
}
