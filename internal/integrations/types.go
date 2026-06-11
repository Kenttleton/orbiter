package integrations

import "github.com/Kenttleton/orbiter/internal/models"

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
	Self         *models.Resource                 `json:"self,omitempty"`
	Resources    map[string][]ResolvedResource    `json:"resources"`
	Transponders map[string][]ResolvedTransponder `json:"transponders"`
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

// StateReport is returned by Init, Scan, and Calibrate.
// Manager is always populated — every installation has a manager
// (nvm, homebrew, apt, the OS itself, or "source").
type StateReport struct {
	Present      bool           `json:"present"`
	Reachable    bool           `json:"reachable"`
	BinaryPath   string         `json:"binary_path,omitempty"`
	InstallDir   string         `json:"install_dir,omitempty"`
	InPath       bool           `json:"in_path"`
	Manager      string         `json:"manager"`
	Config       map[string]any `json:"config,omitempty"`
	Observations []string       `json:"observations,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// Manifest is the parsed content of an integration's manifest.toml.
type Manifest struct {
	Integration  ManifestIntegration  `toml:"integration"`
	Detection    ManifestDetection    `toml:"detection"`
	Dependencies ManifestDependencies `toml:"dependencies"`
}

// ManifestIntegration is the [integration] section of a manifest.toml.
type ManifestIntegration struct {
	Type  string `toml:"type"`  // "resource" | "transponder"
	Role  string `toml:"role"`
	Brand string `toml:"brand"`
}

// ManifestDetection is the [detection] section of a manifest.toml.
type ManifestDetection struct {
	Files []string `toml:"files"`
}

// ManifestDependencies is the [dependencies] section of a manifest.toml.
// Keys are roles. Values are brand whitelists (empty slice = any brand accepted).
type ManifestDependencies struct {
	Resources    map[string][]string `toml:"resources"`
	Transponders map[string][]string `toml:"transponders"`
}
