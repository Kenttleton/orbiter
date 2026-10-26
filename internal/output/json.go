package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Kenttleton/orbiter/internal/models"
)

// JSONRenderer produces machine-readable JSON output.
// Human-facing methods (Info, Success, Warning, Error, Plan, Table, Progress)
// are intentional no-ops — JSON mode is for wrappers, not humans.
type JSONRenderer struct {
	w       io.Writer
	trace bool
}

// NewJSONRenderer returns a JSONRenderer that writes to w.
func NewJSONRenderer(w io.Writer, trace bool) *JSONRenderer {
	return &JSONRenderer{w: w, trace: trace}
}

func (r *JSONRenderer) Info(_ string)    {}
func (r *JSONRenderer) Success(_ string) {}
func (r *JSONRenderer) Warning(_ string) {}
func (r *JSONRenderer) Error(_ string)   {}

func (jr *JSONRenderer) Plan(steps []PlanStep) {
	data, err := json.Marshal(steps)
	if err != nil {
		return
	}
	jr.emitEvent("plan", models.NewID("tx"), data)
}

func (r *JSONRenderer) Table(_ []string, _ [][]string) {}

func (r *JSONRenderer) Progress(steps []ProgressStep) ProgressList {
	return &noopProgressList{}
}

func (r *JSONRenderer) JSON(v any) error {
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (r *JSONRenderer) IsTrace() bool { return r.trace }

// Prompt implements Renderer for JSON mode. Emits a captain_prompt event and returns ("", nil).
// The caller must insert the transaction record before calling Prompt.
// The Captain's response arrives asynchronously via orbiter relay.
func (jr *JSONRenderer) Prompt(p CaptainPrompt) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	jr.emitEvent("captain_prompt", p.Txn, data)
	return "", nil
}

func (jr *JSONRenderer) emitEvent(eventType, txn string, data json.RawMessage) {
	e := Event{Type: eventType, Txn: txn, Data: data}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintln(jr.w, string(b))
}

// noopProgressList is returned by JSONRenderer.Progress — no terminal output.
type noopProgressList struct{}

func (n *noopProgressList) Start(_ string) {}
func (n *noopProgressList) Advance()       {}
func (n *noopProgressList) Finish()        {}
func (n *noopProgressList) Fail(_ error)   {}
