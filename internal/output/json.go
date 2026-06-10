package output

import (
	"encoding/json"
	"io"
)

// JSONRenderer produces machine-readable JSON output.
// Human-facing methods (Info, Success, Warning, Error, Plan, Table, Progress)
// are intentional no-ops — JSON mode is for wrappers, not humans.
type JSONRenderer struct {
	w       io.Writer
	verbose bool
}

// NewJSONRenderer returns a JSONRenderer that writes to w.
func NewJSONRenderer(w io.Writer, verbose bool) *JSONRenderer {
	return &JSONRenderer{w: w, verbose: verbose}
}

func (r *JSONRenderer) Info(_ string)    {}
func (r *JSONRenderer) Success(_ string) {}
func (r *JSONRenderer) Warning(_ string) {}
func (r *JSONRenderer) Error(_ string)   {}

func (r *JSONRenderer) Plan(_ []PlanStep) {}

func (r *JSONRenderer) Table(_ []string, _ [][]string) {}

func (r *JSONRenderer) Progress(steps []ProgressStep) ProgressList {
	return &noopProgressList{}
}

func (r *JSONRenderer) JSON(v any) error {
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (r *JSONRenderer) IsVerbose() bool { return r.verbose }

// noopProgressList is returned by JSONRenderer.Progress — no terminal output.
type noopProgressList struct{}

func (n *noopProgressList) Start(_ string) {}
func (n *noopProgressList) Advance()       {}
func (n *noopProgressList) Finish()        {}
func (n *noopProgressList) Fail(_ error)   {}
