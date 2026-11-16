package commands

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
)

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

// ScanItem is one scanned resource or transponder; Report is the integration's
// full StateReport.
type ScanItem struct {
	Kind   string                   `json:"kind"` // resource | transponder
	ID     string                   `json:"id"`
	Role   string                   `json:"role"`
	Brand  string                   `json:"brand"`
	Status string                   `json:"status"`
	Report integrations.StateReport `json:"report"`
}

// ScanResult is the JSON result of scan.
type ScanResult struct {
	Target   string     `json:"target"`
	TargetID string     `json:"target_id"`
	Items    []ScanItem `json:"items"`
}

// CalibrateItem is one calibrated resource or transponder. Before is absent for
// transponders, whose calibration reports only the outcome.
type CalibrateItem struct {
	Kind   string                    `json:"kind"`
	ID     string                    `json:"id"`
	Role   string                    `json:"role"`
	Brand  string                    `json:"brand"`
	Action string                    `json:"action"`
	Before *integrations.StateReport `json:"before,omitempty"`
	After  integrations.StateReport  `json:"after"`
}

// CalibrateResult is the JSON result of a completed calibrate.
type CalibrateResult struct {
	Target   string          `json:"target"`
	TargetID string          `json:"target_id"`
	Items    []CalibrateItem `json:"items"`
}

// RetroNode is one entity a retro plan retires.
type RetroNode struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// RetroResult is the JSON result of retro. Retired is false when there was
// nothing to retire or the Captain declined.
type RetroResult struct {
	Target  string      `json:"target"`
	Retired bool        `json:"retired"`
	Nodes   []RetroNode `json:"nodes"`
}

// QuarantineResult is the JSON result of quarantine and unquarantine. Changed is
// false when the brand was already in the requested state.
type QuarantineResult struct {
	Brand       string `json:"brand"`
	Quarantined bool   `json:"quarantined"`
	Changed     bool   `json:"changed"`
}

// InspectResult is the JSON result of vessel inspect and survey <brand>.
type InspectResult struct {
	Brand       string     `json:"brand"`
	Status      string     `json:"status"` // quarantined | used | idle | missing
	UsedBy      int        `json:"used_by"`
	Engine      string     `json:"engine,omitempty"` // interpreter | compiler
	MemoryBytes int64      `json:"memory_bytes,omitempty"`
	Env         []string   `json:"env,omitempty"` // variables claimed for commands; nothing else is passed
	Quarantined bool       `json:"quarantined"`
	Reason      string     `json:"reason,omitempty"`
	Since       *time.Time `json:"since,omitempty"`
}

// VesselInitResult is the JSON result of vessel init.
type VesselInitResult struct {
	Installed int `json:"installed"`
	Removed   int `json:"removed"`
}

// DiscoveryRequest is one thing the client must find and relay (see orbiter relay).
type DiscoveryRequest struct {
	Txn   string `json:"txn"`
	Op    string `json:"op"` // FIND_BINARY | FIND_FILE | FIND_DIR | FIND_ENV
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// PendingDiscovery is the JSON result of a command that cannot continue until the client
// answers: relay each answer, then run uplink --session. Status is needs_discovery when it
// asks the client to find things (Requests), or needs_decision when it asks the Captain to
// choose (a captain_prompt event already carries the transaction to answer).
type PendingDiscovery struct {
	Status   string             `json:"status"` // needs_discovery | needs_decision
	Session  string             `json:"session"`
	Requests []DiscoveryRequest `json:"requests"`
}

// emitDirectives reports directives a command produced. Styled mode prints the
// shell line protocol (the hook acts on it). JSON mode reports the session and its
// discovery requests as one result, because a client that is not a shell answers
// them itself.
func (d *deps) emitDirectives(out io.Writer, dirs []Directive) {
	if !d.jsonMode {
		for _, dir := range dirs {
			fmt.Fprintln(out, dir.String())
		}
		return
	}
	if len(dirs) == 0 {
		return
	}
	pending := PendingDiscovery{Status: "needs_discovery", Requests: []DiscoveryRequest{}}
	for _, dir := range dirs {
		switch {
		case dir.Op == "SESSION":
			pending.Session = dir.Key
		case strings.HasPrefix(dir.Op, "FIND_"):
			pending.Requests = append(pending.Requests, DiscoveryRequest{Txn: dir.Txn, Op: dir.Op, Key: dir.Key, Value: dir.Value})
		}
	}
	if len(pending.Requests) == 0 {
		pending.Status = "needs_decision"
		d.done("decision needed", pending)
		return
	}
	d.done("discovery needed", pending)
}

// redactedReport returns r with every export value replaced, keeping the names.
// Exports are environment variables bound for the shell and may be secrets; a
// result a client receives (and may log) must name them but never carry them.
func redactedReport(r integrations.StateReport) integrations.StateReport {
	if len(r.Exports) == 0 {
		return r
	}
	masked := make(map[string]string, len(r.Exports))
	for k := range r.Exports {
		masked[k] = "[redacted]"
	}
	r.Exports = masked
	return r
}

// registerSuggestions registers what discovery suggested and attaches each resource to
// attachTo. One step serves every level of the map: a planet's `init --discover` and the
// vessel's shell use the same code.
func registerSuggestions(ctx context.Context, sc *starchart.StarChart, suggestions []integrations.SuggestedResource, attachTo string) []DiscoveryOutcome {
	var outcomes []DiscoveryOutcome
	for _, s := range suggestions {
		r, err := sc.CreateResource(ctx, s.Brand, s.Role, s.Brand, "[]", "{}")
		if err != nil {
			outcomes = append(outcomes, DiscoveryOutcome{Role: s.Role, Brand: s.Brand, Outcome: "skipped", Error: err.Error()})
			continue
		}
		if _, err := sc.Attach(ctx, r.ID, attachTo); err != nil {
			outcomes = append(outcomes, DiscoveryOutcome{Role: s.Role, Brand: s.Brand, Outcome: "attach_failed", Error: err.Error()})
			continue
		}
		outcomes = append(outcomes, DiscoveryOutcome{Role: s.Role, Brand: s.Brand, Outcome: "attached"})
	}
	return outcomes
}
