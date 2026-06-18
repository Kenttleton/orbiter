package integrations

import "strings"

// Manifest is the parsed content of an integration's manifest.toml.
// All sections are optional — the host applies defaults where fields are zero.
type Manifest struct {
	Integration  ManifestIntegration  `toml:"integration"`
	Detection    ManifestDetection    `toml:"detection"`
	Dependencies ManifestDependencies `toml:"dependencies"`
	Commands     ManifestCommands     `toml:"commands"`
	Shell        ManifestShell        `toml:"shell"`
	Config       ManifestConfig       `toml:"config"`
	Runtime      ManifestRuntime      `toml:"runtime"`
}

// ManifestIntegration is the [integration] section.
// Type is absent — Orbiter infers resource vs transponder from its static role taxonomy.
type ManifestIntegration struct {
	Brand       string   `toml:"brand"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Roles       []string `toml:"roles"`
}

// ManifestEnvRule is one env-var detection condition in the [detection] section.
// If Pattern is non-empty, the env var's value must contain it as a substring.
// If Pattern is empty, the env var need only be present with a non-empty value.
type ManifestEnvRule struct {
	Key     string `toml:"key"`
	Pattern string `toml:"pattern"`
}

// ManifestDetection is the [detection] section.
type ManifestDetection struct {
	Files []string          `toml:"files"`
	Env   []ManifestEnvRule `toml:"env"`
}

// MatchesAny reports whether at least one detection rule is satisfied.
// Returns true when there are no rules — the integration may have custom WASM detect logic.
func (d ManifestDetection) MatchesAny(files map[string]string, env map[string]string) bool {
	if len(d.Files) == 0 && len(d.Env) == 0 {
		return true
	}
	for _, f := range d.Files {
		if _, ok := files[f]; ok {
			return true
		}
	}
	for _, rule := range d.Env {
		val, ok := env[rule.Key]
		if !ok || val == "" {
			continue
		}
		if rule.Pattern == "" || strings.Contains(val, rule.Pattern) {
			return true
		}
	}
	return false
}

// ManifestDependencies is the [dependencies] section.
// Keys are roles. Values are brand allowlists (empty = any brand accepted).
type ManifestDependencies struct {
	Resources    map[string][]string `toml:"resources"`
	Transponders map[string][]string `toml:"transponders"`
}

// ManifestCommandEntry declares one command the integration may invoke via run_command.
// Description is shown to the Captain during the approval workflow.
type ManifestCommandEntry struct {
	Cmd         string `toml:"cmd"`
	Description string `toml:"description"`
}

// ManifestCommands is the [commands] section.
// Allowed lists every executable the integration may call via run_command.
// The host enforces this allowlist at Stage 3 of runCommandFn.
type ManifestCommands struct {
	Allowed        []ManifestCommandEntry `toml:"allowed"`
	TimeoutSeconds int                    `toml:"timeout_seconds"`
}

// AllowedCmds returns the flat list of executable names from the allowlist.
func (c ManifestCommands) AllowedCmds() []string {
	cmds := make([]string, 0, len(c.Allowed))
	for _, e := range c.Allowed {
		cmds = append(cmds, e.Cmd)
	}
	return cmds
}

// ManifestShellExport declares one export in the [shell] section.
// Exactly one of Hook or Envs must be set per entry.
// Hook is the filename of a static hook script (relative to the integration directory).
// Envs lists env var names the integration may write to StateReport.Exports.
// Description is shown during Captain approval.
// Sensitive marks env vars that should be redacted in logs and UI (ignored for hook entries).
type ManifestShellExport struct {
	Hook        string   `toml:"hook"`
	Envs        []string `toml:"envs"`
	Description string   `toml:"description"`
	Sensitive   bool     `toml:"sensitive"`
}

// ManifestShell is the [shell] section.
// Exports is the auditable contract: each entry is either a hook script declaration
// or a group of env vars the integration may write to StateReport.Exports.
type ManifestShell struct {
	Exports []ManifestShellExport `toml:"exports"`
}

// HookFile returns the filename of the hook script declared in exports, or "" if none.
func (s ManifestShell) HookFile() string {
	for _, e := range s.Exports {
		if e.Hook != "" {
			return e.Hook
		}
	}
	return ""
}

// AllowedEnvs returns the flat list of env var names across all env export declarations.
func (s ManifestShell) AllowedEnvs() []string {
	var envs []string
	for _, e := range s.Exports {
		envs = append(envs, e.Envs...)
	}
	return envs
}

// ManifestConfig describes configuration fields the integration accepts.
// Used by Orbiter for guided setup UI and input validation.
type ManifestConfig struct {
	Fields []ManifestConfigField `toml:"fields"`
}

// ManifestConfigField describes one config field.
type ManifestConfigField struct {
	Key         string `toml:"key"`
	Type        string `toml:"type"`
	Required    bool   `toml:"required"`
	Description string `toml:"description"`
}

// ManifestRuntime is the [runtime] section — performance hints.
// PoolSize drives the module pool size created at load time (Phase 4 Task 7).
// InputBufferKB and OutputBufferKB are guest buffer size hints (default 8 if zero).
type ManifestRuntime struct {
	PoolSize       int `toml:"pool_size"`
	InputBufferKB  int `toml:"input_buffer_kb"`
	OutputBufferKB int `toml:"output_buffer_kb"`
}
