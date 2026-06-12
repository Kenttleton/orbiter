package integrations

import "github.com/Kenttleton/orbiter/internal/models"

// Entity is the uniform interface implemented by both Resource and Transponder.
// ResolvedContext.Self is Entity so the same dispatch path serves both.
type Entity interface {
	GetID() string
	GetRole() string
	GetBrand() string
	GetConfig() string
}

// InputRequest describes a single credential prompt the integration needs.
// The host collects responses and calls the integration again with Responses populated.
type InputRequest struct {
	Key    string `json:"key"`
	Prompt string `json:"prompt"`
	Masked bool   `json:"masked"`
}

// Platform describes the host operating system and architecture.
type Platform struct {
	OS   string `json:"os"`   // "darwin" | "linux" | "windows"
	Arch string `json:"arch"` // "amd64" | "arm64"
}

// DetectContext is passed to Detect. Files is populated only for file-pattern
// roles (runtime, manager, tool). Remote and filesystem integrations receive
// an empty Files map and inspect CWD directly.
type DetectContext struct {
	Platform Platform          `json:"platform"`
	CWD      string            `json:"cwd"`
	Files    map[string]string `json:"files"`
}

// DetectReport is returned by Detect.
type DetectReport struct {
	Detected  bool               `json:"detected"`
	Resources []SuggestedResource `json:"resources,omitempty"`
}

// SuggestedResource is one resource suggestion produced by detection.
// Brand is integration-owned — the integration knows what it detected.
type SuggestedResource struct {
	Role    string         `json:"role"`
	Brand   string         `json:"brand"`
	Version string         `json:"version,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}

// ResolvedContext is the boundary struct passed to Init, Scan, and Calibrate.
// Assembled by the starchart branch crawl, filtered by manifest dependencies.
// All fields are JSON-serializable for Phase 3 WASM compatibility.
type ResolvedContext struct {
	Platform     Platform                         `json:"platform"`
	Self         Entity                           `json:"self,omitempty"`
	Resources    map[string][]ResolvedResource    `json:"resources"`
	Transponders map[string][]ResolvedTransponder `json:"transponders"`
	Responses    map[string]string                `json:"responses,omitempty"`
}

// ResolvedResource wraps a resource from the branch with its StateReport
// if it has already been initialized earlier in the execution graph.
type ResolvedResource struct {
	Resource    models.Resource `json:"resource"`
	StateReport *StateReport    `json:"state_report,omitempty"`
}

// ResolvedTransponder wraps a transponder from the branch.
type ResolvedTransponder struct {
	Transponder models.Transponder `json:"transponder"`
}

// TransponderScanResult pairs a transponder with its scan report.
type TransponderScanResult struct {
	Transponder models.Transponder
	Report      StateReport
}

// TransponderCalibrateResult pairs a transponder with its calibration report.
type TransponderCalibrateResult struct {
	Transponder models.Transponder
	Report      StateReport
}

// StateReport is returned by Init, Scan, and Calibrate.
// Manager is always populated — every installation has a manager
// (nvm, homebrew, apt, the OS itself, or "source").
type StateReport struct {
	Present      bool              `json:"present"`
	Reachable    bool              `json:"reachable"`
	BinaryPath   string            `json:"binary_path,omitempty"`
	InstallDir   string            `json:"install_dir,omitempty"`
	InPath       bool              `json:"in_path"`
	Manager      string            `json:"manager"`
	Config       map[string]any    `json:"config,omitempty"`
	Observations []string          `json:"observations,omitempty"`
	Error        string            `json:"error,omitempty"`
	NeedsInput   []InputRequest    `json:"needs_input,omitempty"`
	Exports      map[string]string `json:"exports,omitempty"`
}

