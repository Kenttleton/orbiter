package output_test

import (
	"encoding/json"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	data, _ := json.Marshal([]string{"step1"})
	e := output.Event{Type: "plan", Txn: "a3f2k1b8tx9r2m4p", Data: data}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got output.Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "plan" {
		t.Errorf("Type = %q, want %q", got.Type, "plan")
	}
	if got.Txn != "a3f2k1b8tx9r2m4p" {
		t.Errorf("Txn = %q, want %q", got.Txn, "a3f2k1b8tx9r2m4p")
	}
}

func TestCaptainPrompt_JSONRoundTrip(t *testing.T) {
	cp := output.CaptainPrompt{
		Txn:     "b4g3l2c9tx0s3n5q",
		Op:      "SELECT",
		Message: "choose a path",
		Options: []string{"/a", "/b"},
		Variant: "single",
	}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got output.CaptainPrompt
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Op != "SELECT" {
		t.Errorf("Op = %q, want SELECT", got.Op)
	}
	if len(got.Options) != 2 {
		t.Errorf("Options len = %d, want 2", len(got.Options))
	}
}

func TestResult_JSONRoundTrip(t *testing.T) {
	r := output.Result{Success: true, Message: "branch calibrated"}
	b, _ := json.Marshal(r)
	var got output.Result
	json.Unmarshal(b, &got)
	if !got.Success {
		t.Error("Success = false, want true")
	}
	if got.Message != "branch calibrated" {
		t.Errorf("Message = %q, want %q", got.Message, "branch calibrated")
	}
}

func TestPlanStep_JSONTags(t *testing.T) {
	ps := output.PlanStep{Action: "add", EntityType: "resource", Name: "brew", Description: "package mgr"}
	b, _ := json.Marshal(ps)
	if string(b) != `{"action":"add","entity_type":"resource","name":"brew","description":"package mgr"}` {
		t.Errorf("unexpected JSON: %s", b)
	}
}

func TestProgressStep_StatusField(t *testing.T) {
	ps := output.ProgressStep{ThematicLabel: "Scanning", PlainLabel: "scan", Status: "running"}
	b, _ := json.Marshal(ps)
	var m map[string]string
	json.Unmarshal(b, &m)
	if m["status"] != "running" {
		t.Errorf("status = %q, want running", m["status"])
	}
}
