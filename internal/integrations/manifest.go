package integrations

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

// ManifestDetection is the [detection] section.
type ManifestDetection struct {
	Files []string `toml:"files"`
}

// ManifestDependencies is the [dependencies] section.
// Keys are roles. Values are brand allowlists (empty = any brand accepted).
type ManifestDependencies struct {
	Resources    map[string][]string `toml:"resources"`
	Transponders map[string][]string `toml:"transponders"`
}

// ManifestCommands is the [commands] section.
// Allowed lists every executable the integration may call via run_command.
// The host rejects any call for an executable not listed here.
type ManifestCommands struct {
	Allowed        []string `toml:"allowed"`
	TimeoutSeconds int      `toml:"timeout_seconds"`
}

// ManifestShell is the [shell] section.
// Exports lists env var names the integration may include in StateReport.Exports.
// The host drops any export key not declared here.
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
// PoolSize is the number of concurrent WASM instances (default 4 if zero).
// InputBufferKB and OutputBufferKB are guest buffer size hints (default 8 if zero).
type ManifestRuntime struct {
	PoolSize       int `toml:"pool_size"`
	InputBufferKB  int `toml:"input_buffer_kb"`
	OutputBufferKB int `toml:"output_buffer_kb"`
}
