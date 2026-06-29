package wasm_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestRendererApproveFunc_confirm_yes(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("y\n"), false)
	fn := wasm.RendererApproveFunc(r)
	if !fn("git", "git status") {
		t.Error("expected approve = true for 'y' input")
	}
}

func TestRendererApproveFunc_confirm_no(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewStyledRendererWith(&buf, strings.NewReader("n\n"), false)
	fn := wasm.RendererApproveFunc(r)
	if fn("git", "git status") {
		t.Error("expected approve = false for 'n' input")
	}
}

func TestRendererApproveFunc_json_mode_blocks(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	fn := wasm.RendererApproveFunc(r)
	// JSON mode: Prompt emits event and returns ("", nil).
	// RendererApproveFunc must treat empty response as denial.
	if fn("git", "git status") {
		t.Error("JSON mode: empty response should be treated as denial")
	}
	// The captain_prompt event was emitted.
	if buf.Len() == 0 {
		t.Error("JSON mode: expected captain_prompt event on stdout")
	}
}
