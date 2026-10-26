package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Kenttleton/orbiter/internal/models"
)

// JSONRenderer produces machine-readable JSON output.
// Human-facing methods (Info, Success, Warning, Error, Plan, Table, Progress)
// are intentional no-ops — JSON mode is for wrappers, not humans.
type JSONRenderer struct {
	w     io.Writer
	trace bool
	// in is set by Attach (-i): the client's stdin, from which prompt answers are read.
	in *bufio.Reader
}

// Attach makes prompts synchronous: after emitting a captain_prompt event, Prompt
// reads the answer from in as one JSON line, {"txn":"<id>","response":<json>}, where
// response has the same shape orbiter relay accepts. Lines for another txn or that
// are not valid JSON are skipped. A closed stdin makes Prompt fail.
// Without Attach, Prompt is asynchronous: it returns "" and the answer arrives
// through orbiter relay.
func (jr *JSONRenderer) Attach(in io.Reader) { jr.in = bufio.NewReader(in) }

// NewJSONRenderer returns a JSONRenderer that writes to w.
func NewJSONRenderer(w io.Writer, trace bool) *JSONRenderer {
	return &JSONRenderer{w: w, trace: trace}
}

func (r *JSONRenderer) Info(_ string)    {}
func (r *JSONRenderer) Success(_ string) {}
func (r *JSONRenderer) Warning(_ string) {}

// Error implements Renderer for JSON mode: one "error" event. A command that fails
// returns the error; the process wrapper (commands.Execute) reports it here.
func (jr *JSONRenderer) Error(msg string) {
	data, err := json.Marshal(map[string]string{"message": msg})
	if err != nil {
		return
	}
	jr.emitEvent("error", models.NewID("tx"), data)
}

func (jr *JSONRenderer) Plan(steps []PlanStep) {
	if steps == nil {
		steps = []PlanStep{}
	}
	data, err := json.Marshal(steps)
	if err != nil {
		return
	}
	jr.emitEvent("plan", models.NewID("tx"), data)
}

// Result implements Renderer for JSON mode: one "result" event carrying the outcome.
func (jr *JSONRenderer) Result(res Result) {
	data, err := json.Marshal(res)
	if err != nil {
		return
	}
	jr.emitEvent("result", models.NewID("tx"), data)
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

// Prompt implements Renderer for JSON mode. Emits a captain_prompt event.
// Unattached (the default) it returns ("", nil) and the Captain's response arrives
// asynchronously via orbiter relay. Attached (-i) it reads the response from stdin
// and returns it. The caller must insert the transaction record before calling Prompt.
func (jr *JSONRenderer) Prompt(p CaptainPrompt) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	jr.emitEvent("captain_prompt", p.Txn, data)
	if jr.in == nil {
		return "", nil
	}
	for {
		line, err := jr.in.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			var answer struct {
				Txn      string          `json:"txn"`
				Response json.RawMessage `json:"response"`
			}
			if json.Unmarshal([]byte(line), &answer) == nil && answer.Txn == p.Txn && len(answer.Response) > 0 {
				return string(answer.Response), nil
			}
		}
		if err != nil {
			return "", fmt.Errorf("prompt %s: stdin closed before an answer arrived: %w", p.Txn, err)
		}
	}
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
