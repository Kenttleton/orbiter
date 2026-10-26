package commands

import "github.com/Kenttleton/orbiter/internal/output"

// EntityResult is the JSON result of the build commands (add and init). Status is
// the entity's beacon status and is set by init. Galaxy and System locate a
// system or planet; Role and Brand describe a resource or transponder.
type EntityResult struct {
	Type       string             `json:"type"`
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Status     string             `json:"status,omitempty"`
	Role       string             `json:"role,omitempty"`
	Brand      string             `json:"brand,omitempty"`
	Galaxy     string             `json:"galaxy,omitempty"`
	System     string             `json:"system,omitempty"`
	Discovered []DiscoveryOutcome `json:"discovered,omitempty"`
}

// DiscoveryOutcome is what planet init --discover did with one suggestion.
type DiscoveryOutcome struct {
	Role    string `json:"role"`
	Brand   string `json:"brand"`
	Outcome string `json:"outcome"` // attached | skipped | attach_failed
	Error   string `json:"error,omitempty"`
}

// AttachResult is the JSON result of attach.
type AttachResult struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// done reports a command's structured outcome to machine clients. In styled mode
// the command has already printed its own message and this is a no-op.
func (d *deps) done(message string, data any) {
	d.renderer.Result(output.Result{Success: true, Message: message, Data: data})
}
