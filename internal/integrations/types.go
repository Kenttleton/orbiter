package integrations

import (
	"path/filepath"
	"strings"

	"github.com/Kenttleton/orbiter/internal/models"
)

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

// Path carries the Unix and Windows forms of a single absolute path.
// Both fields are exported for direct access and JSON serialization.
// Four Into* methods provide a uniform calling convention across all forms,
// including the two computed runtime forms (Native and Shell).
//
//	Unix      — exported field; always forward-slash; persisted in StarChart.
//	Windows   — exported field; always backslash; persisted in StarChart.
//	IntoUnix()    — method alias for Unix; use when a uniform call site matters.
//	IntoWindows() — method alias for Windows.
//	IntoNative()  — returns Windows on Windows, Unix everywhere else.
//	IntoShell()   — returns IntoNative() wrapped in double-quotes when it contains a space.
type Path struct {
	Unix    string `json:"unix"`    // persisted; always forward-slash
	Windows string `json:"windows"` // persisted; always backslash
}

func (p Path) IntoUnix() string    { return p.Unix }
func (p Path) IntoWindows() string { return p.Windows }

// IntoNative returns the OS-native separator form of the path.
func (p Path) IntoNative() string {
	if filepath.Separator == '\\' {
		return p.Windows
	}
	return p.Unix
}

// IntoShell returns the path for embedding in a shell command string.
// Wraps IntoNative() in double-quotes when it contains a space.
func (p Path) IntoShell() string {
	n := p.IntoNative()
	if strings.Contains(n, " ") {
		return `"` + n + `"`
	}
	return n
}

// DetectContext is passed to Detect. Files is populated only for file-pattern
// roles (runtime, manager, tool). Remote and shell integrations receive
// an empty Files map and inspect CWD or Env directly.
type DetectContext struct {
	Platform Platform          `json:"platform"`
	CWD      string            `json:"cwd"`
	Files    map[string]string `json:"files"`
	Env      map[string]string `json:"env"`
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
	Binaries     map[string]string                `json:"binaries,omitempty"`
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
	Transponder  models.Transponder
	Report       StateReport
	BeaconStatus string // "healthy" | "drifted" | "failed"
}

// TransponderCalibrateResult pairs a transponder with its calibration report.
type TransponderCalibrateResult struct {
	Transponder models.Transponder
	Report      StateReport
	Action      string // "healthy" | "calibrated" | "failed"
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

