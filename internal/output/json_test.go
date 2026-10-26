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

// An empty plan is still a plan: a nil slice must encode as [] so clients can
// rely on chart's terminal event being a plan with a list.
func TestJSONRenderer_Plan_emptyIsAnEmptyList(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Plan(nil)
	var e output.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &e); err != nil {
		t.Fatalf("unmarshal: %v — raw %s", err, buf.String())
	}
	if e.Type != "plan" || string(e.Data) != "[]" {
		t.Errorf("Type = %q Data = %s, want plan with []", e.Type, e.Data)
	}
}

// -i attaches the client's stdin: the answer comes back as one JSON line carrying
// the txn it answers, with the same response shape orbiter relay accepts.
func TestJSONRenderer_Prompt_attachedReadsTheAnswerFromStdin(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Attach(strings.NewReader(
		`{"txn":"someothertxn0000","response":{"confirmed":false}}` + "\n" +
			`{"txn":"b4g3l2c9tx0s3n5q","response":{"confirmed":true}}` + "\n"))

	resp, err := r.Prompt(output.CaptainPrompt{Txn: "b4g3l2c9tx0s3n5q", Op: "CONFIRM", Message: "run it?"})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"confirmed":true}` {
		t.Errorf("resp = %q, want the response for the matching txn", resp)
	}
	if !strings.Contains(buf.String(), `"captain_prompt"`) {
		t.Errorf("the prompt event must still be emitted: %q", buf.String())
	}
}

func TestJSONRenderer_Prompt_attachedClosedStdinIsAnError(t *testing.T) {
	r := output.NewJSONRenderer(&bytes.Buffer{}, false)
	r.Attach(strings.NewReader(""))
	if _, err := r.Prompt(output.CaptainPrompt{Txn: "b4g3l2c9tx0s3n5q", Op: "CONFIRM"}); err == nil {
		t.Error("a client that closed stdin cannot answer: Prompt must fail, not wait forever")
	}
}

func TestJSONRenderer_Prompt_attachedIgnoresMalformedLines(t *testing.T) {
	r := output.NewJSONRenderer(&bytes.Buffer{}, false)
	r.Attach(strings.NewReader("not json\n" + `{"txn":"t1","response":{"value":"x"}}` + "\n"))
	resp, err := r.Prompt(output.CaptainPrompt{Txn: "t1", Op: "TEXT"})
	if err != nil || resp != `{"value":"x"}` {
		t.Errorf("resp = %q err = %v", resp, err)
	}
}
