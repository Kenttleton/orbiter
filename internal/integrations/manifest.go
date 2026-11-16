package integrations

import (
	"fmt"
	"strings"
)

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
	Find         ManifestFind         `toml:"find"`
}

// ManifestFind is the [find] section: discovery Orbiter performs on the
// integration's behalf before any handler runs. Binaries are declared in
// [integration] binaries.
type ManifestFind struct {
	Files []string `toml:"files"` // glob patterns, no spaces, searched under the CWD
	Dirs  []string `toml:"dirs"`  // known paths whose existence is checked
	Envs  []string `toml:"envs"`  // exact env var names
}

// ManifestIntegration is the [integration] section.
// Type is absent — Orbiter infers resource vs transponder from its static role taxonomy.
type ManifestIntegration struct {
	Brand       string   `toml:"brand"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Roles       []string `toml:"roles"`
	Binaries    []string `toml:"binaries"`
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
	// Env lists the environment variables the commands need, PATH included when they need it. A
	// command receives these (when Orbiter has them) and nothing else. A name ending in * claims
	// every variable with that prefix.
	Env []string `toml:"env"`
}

// ChildEnv filters environ (KEY=VALUE entries) down to exactly what the manifest claims.
func (c ManifestCommands) ChildEnv(environ []string) []string {
	out := make([]string, 0, len(c.Env)+1)
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if c.claims(key) {
			out = append(out, kv)
		}
	}
	return out
}

func (c ManifestCommands) claims(key string) bool {
	for _, name := range c.Env {
		if prefix, ok := strings.CutSuffix(name, "*"); ok {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		} else if key == name {
			return true
		}
	}
	return false
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

// The engines that can run an integration's wasm. Both enforce the same sandbox; they differ in
// when the cost is paid. The interpreter translates nothing, so an integration starts at once and
// runs slower; the compiler translates the module to native code on every load, which is only
// worth it for a guest that does heavy work.
const (
	EngineInterpreter = "interpreter"
	EngineCompiler    = "compiler"
)

// Guest memory, in bytes: what a guest gets when its manifest does not say, and the most a manifest
// may ask for. WasmPageSize is the unit wasm allocates in.
const (
	WasmPageSize       = 64 * 1024
	DefaultMemoryLimit = 64 << 20
	MaxMemoryLimit     = 1 << 30
)

// ManifestRuntime is the [runtime] section: how the integration's wasm runs.
// Engine is interpreter (the default) or compiler. MemoryBytes is the most linear memory the guest
// may have, in bytes (default DefaultMemoryLimit, ceiling MaxMemoryLimit). PoolSize is how many
// instances may run at once. InputBufferKB and OutputBufferKB are guest buffer size hints
// (default 8 if zero).
type ManifestRuntime struct {
	Engine         string `toml:"engine"`
	MemoryBytes    int64  `toml:"memory_limit"`
	PoolSize       int    `toml:"pool_size"`
	InputBufferKB  int    `toml:"input_buffer_kb"`
	OutputBufferKB int    `toml:"output_buffer_kb"`
}

// EngineName returns the declared engine, or the interpreter when none is declared.
func (r ManifestRuntime) EngineName() string {
	if r.Engine == "" {
		return EngineInterpreter
	}
	return r.Engine
}

// ValidateEngine reports an error for an engine that is neither interpreter nor compiler.
func (r ManifestRuntime) ValidateEngine() error {
	switch r.EngineName() {
	case EngineInterpreter, EngineCompiler:
		return nil
	}
	return fmt.Errorf("unknown runtime engine %q (want %q or %q)", r.Engine, EngineInterpreter, EngineCompiler)
}

// MemoryLimit returns the declared memory limit in bytes, or the default when none is declared.
func (r ManifestRuntime) MemoryLimit() int64 {
	if r.MemoryBytes == 0 {
		return DefaultMemoryLimit
	}
	return r.MemoryBytes
}

// MemoryPages is the memory limit in wasm pages, rounded up so a guest never gets less than it asked for.
func (r ManifestRuntime) MemoryPages() uint32 {
	return uint32((r.MemoryLimit() + WasmPageSize - 1) / WasmPageSize)
}

// ValidateMemory reports an error for a limit that is negative, under one page, or past the ceiling.
func (r ManifestRuntime) ValidateMemory() error {
	if r.MemoryBytes != 0 && (r.MemoryBytes < WasmPageSize || r.MemoryBytes > MaxMemoryLimit) {
		return fmt.Errorf("memory_limit %d out of range (omit it for the default of %d bytes; at least %d, at most %d)",
			r.MemoryBytes, DefaultMemoryLimit, WasmPageSize, MaxMemoryLimit)
	}
	return nil
}
