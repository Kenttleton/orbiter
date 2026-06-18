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

// ManifestCommands is the [commands] section.
// Allowed lists every executable the integration may call via run_command.
// The host enforces this allowlist at Stage 2 of runCommandFn (Phase 4 Task 6).
type ManifestCommands struct {
	Allowed        []string `toml:"allowed"`
	TimeoutSeconds int      `toml:"timeout_seconds"`
}

// ManifestShell is the [shell] section.
// Exports lists env var names the integration may write to StateReport.Exports.
// The host enforces this allowlist after dispatch (Phase 4 Task 7).
type ManifestShell struct {
	Exports []string `toml:"exports"`
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
