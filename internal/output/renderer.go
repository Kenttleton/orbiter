package output

// Format constants for the output renderer.
const (
	FormatStyled = "styled"
	FormatJSON   = "json"
)

// PlanStep represents one step in a Chart or transition plan.
type PlanStep struct {
	Action      string `json:"action"`       // "add" | "change" | "remove"
	EntityType  string `json:"entity_type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProgressStep represents one task in a running operation's step list.
type ProgressStep struct {
	// ThematicLabel is the sci-fi themed label shown in default mode.
	ThematicLabel string `json:"thematic_label"`
	// PlainLabel is the operational description always shown alongside the thematic label,
	// and shown exclusively in trace mode (--trace).
	PlainLabel string `json:"plain_label"`
	// Status is the current state of the step: "pending" | "running" | "done" | "failed".
	// Empty string means the step has not started. Used by the JSON renderer.
	Status string `json:"status,omitempty"`
}

// ProgressList tracks progress through a known set of steps.
// Designed for Six Command operations where all steps are known upfront.
type ProgressList interface {
	// Start begins rendering the step list with the given operation title.
	Start(title string)
	// Advance marks the current step complete and moves to the next.
	Advance()
	// Finish marks all steps complete and stops rendering.
	Finish()
	// Fail marks the current step as failed and stops rendering.
	Fail(err error)
}

// Renderer is the dependency-injected output interface for all commands.
// Commands never select their own renderer — it is provided at startup.
type Renderer interface {
	// Info prints an informational message.
	Info(msg string)
	// Success prints a success message.
	Success(msg string)
	// Warning prints a warning message.
	Warning(msg string)
	// Error prints an error message.
	Error(msg string)
	// Plan renders a list of planned changes (used by the Chart command).
	Plan(steps []PlanStep)
	// Table renders a tabular view with headers and rows.
	Table(headers []string, rows [][]string)
	// Progress returns a ProgressList for tracking a multi-step operation.
	Progress(steps []ProgressStep) ProgressList
	// JSON encodes v as JSON and writes it to stdout.
	// Used for machine-readable output (TUI, 3rd party wrappers).
	JSON(v any) error
	// IsTrace reports whether trace mode (--trace) is enabled: plain operational
	// labels and the underlying tools' raw output. This is process transparency,
	// not data volume; volume is the command's own -v flag.
	IsTrace() bool
	// Prompt presents a CaptainPrompt to the Captain and returns the response as a JSON string.
	//
	// Styled mode: blocks on stdin, collects input, serializes to JSON, returns response string.
	//   Response shapes: SELECT single {"selected":"..."}, SELECT multi {"selected":["..."]},
	//   CONFIRM {"confirmed":true|false}, TEXT {"value":"..."}.
	// JSON mode: emits Event{Type:"captain_prompt"} to stdout, returns ("", nil).
	//   The caller is responsible for inserting the transaction record before calling Prompt.
	//   Response arrives asynchronously via orbiter relay.
	Prompt(p CaptainPrompt) (response string, err error)
}
