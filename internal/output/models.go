package output

import "encoding/json"

// Event is the top-level JSON output envelope emitted to stdout in JSON mode.
// Every line of stdout in JSON mode is one Event serialized as a single-line JSON object.
// Txn is always non-empty — enables concurrent session tracing and full audit log.
type Event struct {
	Type string          `json:"type"` // plan | progress | result | error | captain_prompt
	Txn  string          `json:"txn"`  // NewID("tx") snowflake; always set
	Data json.RawMessage `json:"data"`
}

// CaptainPrompt is emitted when Orbiter needs input from the Captain or a connected client.
// Op names are generic and UI-agnostic — the client decides how to render each op.
type CaptainPrompt struct {
	Txn     string            `json:"txn"`
	Op      string            `json:"op"`               // SELECT | CONFIRM | TEXT
	Message string            `json:"message"`
	Options []string          `json:"options,omitempty"` // SELECT: list of choices
	Variant string            `json:"variant,omitempty"` // SELECT: "single"|"multi"; TEXT: "short"|"long"|"obscured"
	Meta    map[string]string `json:"meta,omitempty"`    // optional context for the client
}

// Result is the output of a completed Orbiter operation.
type Result struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
