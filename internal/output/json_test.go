package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/stretchr/testify/require"
)

func TestJSONRendererEncodesValue(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)

	type payload struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	err := r.JSON(payload{Name: "payment-api", ID: 42})
	require.NoError(t, err)

	var got payload
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, "payment-api", got.Name)
	require.Equal(t, 42, got.ID)
}

func TestJSONRendererInfoWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Info("this should not appear in JSON mode")
	require.Empty(t, buf.String())
}

func TestJSONRendererIsTrace(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, true)
	require.True(t, r.IsTrace())

	r2 := output.NewJSONRenderer(&buf, false)
	require.False(t, r2.IsTrace())
}

func TestJSONRenderer_Plan_emitsEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Plan([]output.PlanStep{
		{Action: "add", EntityType: "resource", Name: "brew", Description: "package manager"},
	})
	line := strings.TrimSpace(buf.String())
	var e output.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, line)
	}
	if e.Type != "plan" {
		t.Errorf("Type = %q, want plan", e.Type)
	}
	if len(e.Txn) != 16 {
		t.Errorf("Txn = %q, expected 16-char snowflake", e.Txn)
	}
	var steps []output.PlanStep
	if err := json.Unmarshal(e.Data, &steps); err != nil {
		t.Fatalf("unmarshal Data: %v", err)
	}
	if len(steps) != 1 || steps[0].Name != "brew" {
		t.Errorf("unexpected steps: %+v", steps)
	}
}

func TestJSONRenderer_Prompt_emitsCaptainPromptEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn:     "b4g3l2c9tx0s3n5q",
		Op:      "SELECT",
		Variant: "single",
		Message: "choose",
		Options: []string{"/a", "/b"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != "" {
		t.Errorf("resp = %q, want empty (async — response via relay)", resp)
	}
	line := strings.TrimSpace(buf.String())
	var e output.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, line)
	}
	if e.Type != "captain_prompt" {
		t.Errorf("Type = %q, want captain_prompt", e.Type)
	}
	if e.Txn != "b4g3l2c9tx0s3n5q" {
		t.Errorf("Txn = %q, want b4g3l2c9tx0s3n5q", e.Txn)
	}
	var cp output.CaptainPrompt
	if err := json.Unmarshal(e.Data, &cp); err != nil {
		t.Fatalf("unmarshal CaptainPrompt: %v", err)
	}
	if cp.Op != "SELECT" {
		t.Errorf("Op = %q, want SELECT", cp.Op)
	}
}

func TestJSONRenderer_Result_emitsResultEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Result(output.Result{Success: true, Message: "surveyed", Data: map[string]string{"k": "v"}})

	var e output.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, buf.String())
	}
	if e.Type != "result" || len(e.Txn) != 16 {
		t.Errorf("Type = %q Txn = %q, want result and a 16-char txn", e.Type, e.Txn)
	}
	var got output.Result
	if err := json.Unmarshal(e.Data, &got); err != nil {
		t.Fatalf("unmarshal Result: %v", err)
	}
	if !got.Success || got.Message != "surveyed" {
		t.Errorf("Result = %+v", got)
	}
}

func TestJSONRenderer_Error_emitsErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Error("record not found")

	var e output.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &e); err != nil {
		t.Fatalf("unmarshal Event: %v — raw: %s", err, buf.String())
	}
	if e.Type != "error" || len(e.Txn) != 16 {
		t.Errorf("Type = %q Txn = %q", e.Type, e.Txn)
	}
	var data struct{ Message string }
	if err := json.Unmarshal(e.Data, &data); err != nil || data.Message != "record not found" {
		t.Errorf("data = %s (err %v)", e.Data, err)
	}
}
