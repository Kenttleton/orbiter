package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/stretchr/testify/require"
)

func TestStyledRendererInfoContainsMessage(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Info("scanning universe")
	require.Contains(t, buf.String(), "scanning universe")
}

func TestStyledRendererSuccessContainsMessage(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Success("jump complete")
	require.Contains(t, buf.String(), "jump complete")
}

func TestStyledRendererWarningContainsMessage(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Warning("drift detected")
	require.Contains(t, buf.String(), "drift detected")
}

func TestStyledRendererErrorContainsMessage(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Error("transponder failure")
	require.Contains(t, buf.String(), "transponder failure")
}

func TestStyledRendererPlanContainsActions(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Plan([]output.PlanStep{
		{Action: "add", EntityType: "planet", Name: "payment-api", Description: "Clone repository"},
		{Action: "change", EntityType: "callsign", Name: "kent-acme", Description: "Activate callsign"},
		{Action: "remove", EntityType: "resource", Name: "node-18", Description: "Deactivate old runtime"},
	})
	out := buf.String()
	require.Contains(t, out, "payment-api")
	require.Contains(t, out, "kent-acme")
	require.Contains(t, out, "node-18")
}

func TestStyledRendererTableRendersRows(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Table(
		[]string{"Name", "Status", "Verified"},
		[][]string{
			{"payment-api", "healthy", "2026-06-09"},
			{"website", "drifted", "2026-06-08"},
		},
	)
	out := buf.String()
	require.Contains(t, out, "payment-api")
	require.Contains(t, out, "drifted")
}

func TestStyledRendererJSONWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	err := r.JSON(map[string]string{"key": "value"})
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"key"`)
}

func TestStyledRendererIsTrace(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, true)
	require.True(t, r.IsTrace())

	r2 := output.NewStyledRenderer(&buf, false)
	require.False(t, r2.IsTrace())
}

func TestStyledRendererTraceShowsNoTheme(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, true)
	r.Info("trace message")
	out := buf.String()
	require.Contains(t, out, "trace message")
	require.False(t, strings.Contains(out, "★"), "trace mode should not show thematic decoration")
}

func TestStyledRenderer_Prompt_Confirm_yes(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("y\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "CONFIRM", Message: "run git status?",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"confirmed":true}` {
		t.Errorf("resp = %q, want {\"confirmed\":true}", resp)
	}
}

func TestStyledRenderer_Prompt_Confirm_no(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("n\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "CONFIRM", Message: "run git status?",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"confirmed":false}` {
		t.Errorf("resp = %q, want {\"confirmed\":false}", resp)
	}
}

func TestStyledRenderer_Prompt_Select_single(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("2\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn:     "abc",
		Op:      "SELECT",
		Variant: "single",
		Message: "choose a binary",
		Options: []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"selected":"/usr/local/bin/brew"}` {
		t.Errorf("resp = %q, want selected /usr/local/bin/brew", resp)
	}
}

func TestStyledRenderer_Prompt_Text_short(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("hello world\n"), false)
	resp, err := r.Prompt(output.CaptainPrompt{
		Txn: "abc", Op: "TEXT", Variant: "short", Message: "enter a name",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp != `{"value":"hello world"}` {
		t.Errorf("resp = %q, want value:hello world", resp)
	}
}

// Result is for machine clients; styled mode renders its own tables.
func TestStyledRenderer_Result_writesNothing(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Result(output.Result{Success: true, Message: "surveyed"})
	require.Empty(t, buf.String())
}

// Chart always hands its steps to Plan, even when there are none; styled mode
// prints its own "nothing to chart" message instead of an empty plan summary.
func TestStyledRenderer_Plan_emptyWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRenderer(&buf, false)
	r.Plan(nil)
	require.Empty(t, buf.String())
}
