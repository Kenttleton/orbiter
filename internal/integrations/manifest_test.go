package integrations_test

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestManifest_ParseNewFormat(t *testing.T) {
	const src = `
[integration]
brand = "gh"
name = "GitHub CLI"
description = "Manage GitHub repos, PRs, and auth via the gh CLI"
roles = ["tool", "keychain"]

[commands]
timeout_seconds = 30
allowed = [
  { cmd = "gh",    description = "GitHub CLI operations" },
  { cmd = "git",   description = "VCS operations" },
  { cmd = "which", description = "Locate executables in PATH" },
]

[shell]
exports = [
  { envs = ["GH_TOKEN"],      description = "GitHub API token for authenticated CLI operations", sensitive = true },
  { envs = ["GITHUB_TOKEN"],  description = "Alternative GitHub token variable", sensitive = true },
]

[[config.fields]]
key = "username"
type = "string"
required = false
description = "GitHub username"

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
`
	var m integrations.Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Integration.Brand != "gh" {
		t.Errorf("brand = %q, want %q", m.Integration.Brand, "gh")
	}
	if m.Integration.Name != "GitHub CLI" {
		t.Errorf("name = %q, want %q", m.Integration.Name, "GitHub CLI")
	}
	if m.Integration.Description == "" {
		t.Error("description should not be empty")
	}
	if len(m.Integration.Roles) != 2 || m.Integration.Roles[0] != "tool" || m.Integration.Roles[1] != "keychain" {
		t.Errorf("roles = %v", m.Integration.Roles)
	}
	if len(m.Commands.Allowed) != 3 || m.Commands.Allowed[0].Cmd != "gh" {
		t.Errorf("commands.allowed = %v", m.Commands.Allowed)
	}
	if m.Commands.TimeoutSeconds != 30 {
		t.Errorf("timeout_seconds = %d", m.Commands.TimeoutSeconds)
	}
	if len(m.Shell.Exports) != 2 || m.Shell.Exports[0].Envs[0] != "GH_TOKEN" || !m.Shell.Exports[0].Sensitive {
		t.Errorf("shell.exports = %v", m.Shell.Exports)
	}
	if len(m.Config.Fields) != 1 || m.Config.Fields[0].Key != "username" {
		t.Errorf("config.fields = %v", m.Config.Fields)
	}
	if m.Runtime.PoolSize != 4 {
		t.Errorf("pool_size = %d", m.Runtime.PoolSize)
	}
	if m.Runtime.InputBufferKB != 8 || m.Runtime.OutputBufferKB != 8 {
		t.Errorf("buffer hints = %d/%d", m.Runtime.InputBufferKB, m.Runtime.OutputBufferKB)
	}
}

func TestManifestShell_HookFile(t *testing.T) {
	const src = `
[shell]
exports = [
  { hook = "hook.bash", description = "Bash hook script" },
  { envs = ["MY_VAR"], description = "some env", sensitive = false },
]
`
	var m integrations.Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := m.Shell.HookFile(); got != "hook.bash" {
		t.Errorf("HookFile() = %q, want %q", got, "hook.bash")
	}
	envs := m.Shell.AllowedEnvs()
	if len(envs) != 1 || envs[0] != "MY_VAR" {
		t.Errorf("AllowedEnvs() = %v, want [MY_VAR]", envs)
	}
}

func TestManifestShell_HookFile_None(t *testing.T) {
	s := integrations.ManifestShell{}
	if got := s.HookFile(); got != "" {
		t.Errorf("HookFile() on empty shell = %q, want empty", got)
	}
}

func TestManifestDetection_MatchesAny_NoRules(t *testing.T) {
	d := integrations.ManifestDetection{}
	// no rules = always matches (integration may have WASM detect logic)
	if !d.MatchesAny(nil, nil) {
		t.Error("empty detection should match anything")
	}
}

func TestManifestDetection_MatchesAny_FileHit(t *testing.T) {
	d := integrations.ManifestDetection{Files: []string{"go.mod"}}
	files := map[string]string{"go.mod": "", "README.md": ""}
	if !d.MatchesAny(files, nil) {
		t.Error("go.mod present should match")
	}
}

func TestManifestDetection_MatchesAny_FileMiss(t *testing.T) {
	d := integrations.ManifestDetection{Files: []string{"go.mod"}}
	files := map[string]string{"package.json": ""}
	if d.MatchesAny(files, nil) {
		t.Error("no matching file should not match")
	}
}

func TestManifestDetection_MatchesAny_EnvKeyPresent(t *testing.T) {
	d := integrations.ManifestDetection{
		Env: []integrations.ManifestEnvRule{{Key: "ZSH_VERSION"}},
	}
	env := map[string]string{"ZSH_VERSION": "5.9", "USER": "kent"}
	if !d.MatchesAny(nil, env) {
		t.Error("ZSH_VERSION present should match")
	}
}

func TestManifestDetection_MatchesAny_EnvKeyAbsent(t *testing.T) {
	d := integrations.ManifestDetection{
		Env: []integrations.ManifestEnvRule{{Key: "ZSH_VERSION"}},
	}
	env := map[string]string{"BASH_VERSION": "5.2"}
	if d.MatchesAny(nil, env) {
		t.Error("ZSH_VERSION absent should not match")
	}
}

func TestManifestDetection_MatchesAny_EnvPattern(t *testing.T) {
	d := integrations.ManifestDetection{
		Env: []integrations.ManifestEnvRule{{Key: "SHELL", Pattern: "zsh"}},
	}
	if !d.MatchesAny(nil, map[string]string{"SHELL": "/usr/bin/zsh"}) {
		t.Error("SHELL containing 'zsh' should match")
	}
	if d.MatchesAny(nil, map[string]string{"SHELL": "/usr/bin/bash"}) {
		t.Error("SHELL not containing 'zsh' should not match")
	}
}

func TestManifestDetection_MatchesAny_FileOrEnvEither(t *testing.T) {
	d := integrations.ManifestDetection{
		Files: []string{"go.mod"},
		Env:   []integrations.ManifestEnvRule{{Key: "GOPATH"}},
	}
	// env hit only
	if !d.MatchesAny(nil, map[string]string{"GOPATH": "/home/kent/go"}) {
		t.Error("env hit should match even without file hit")
	}
	// file hit only
	if !d.MatchesAny(map[string]string{"go.mod": ""}, nil) {
		t.Error("file hit should match even without env hit")
	}
	// neither
	if d.MatchesAny(map[string]string{"package.json": ""}, map[string]string{"NODE_ENV": "dev"}) {
		t.Error("neither file nor env hit should not match")
	}
}

func TestManifest_ParseEnvDetection(t *testing.T) {
	const src = `
[integration]
brand = "zsh"
roles = ["shell"]

[detection]
env = [
  { key = "ZSH_VERSION" },
  { key = "SHELL", pattern = "zsh" },
]
`
	var m integrations.Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(m.Detection.Env) != 2 {
		t.Fatalf("expected 2 env rules, got %d", len(m.Detection.Env))
	}
	if m.Detection.Env[0].Key != "ZSH_VERSION" {
		t.Errorf("rule[0].key = %q, want ZSH_VERSION", m.Detection.Env[0].Key)
	}
	if m.Detection.Env[1].Pattern != "zsh" {
		t.Errorf("rule[1].pattern = %q, want zsh", m.Detection.Env[1].Pattern)
	}
}

func TestManifest_Binaries(t *testing.T) {
	raw := `
[integration]
brand = "brew"
name = "Homebrew"
description = "Package manager"
roles = ["manager"]
binaries = ["brew"]

[commands]
allowed = [
    { cmd = "brew", description = "Run brew commands" },
]
`
	var m integrations.Manifest
	if _, err := toml.Decode(raw, &m); err != nil {
		t.Fatalf("toml decode: %v", err)
	}
	if len(m.Integration.Binaries) != 1 || m.Integration.Binaries[0] != "brew" {
		t.Errorf("expected binaries=[brew], got %v", m.Integration.Binaries)
	}
}

func TestRoleType(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"manager", "resource"},
		{"runtime", "resource"},
		{"tool", "resource"},
		{"remote", "resource"},
		{"shell", "resource"},
		{"file", "transponder"},
		{"env", "transponder"},
		{"keychain", "transponder"},
		{"vault", "transponder"},
		{"agent", "transponder"},
		{"unknown", ""},
	}
	for _, tc := range tests {
		if got := integrations.RoleType(tc.role); got != tc.want {
			t.Errorf("RoleType(%q) = %q, want %q", tc.role, got, tc.want)
		}
	}
}

func TestManifest_ParseFindSection(t *testing.T) {
	const src = `
[integration]
brand = "brew"
binaries = ["brew"]

[find]
files = ["Brewfile"]
dirs  = ["/opt/homebrew"]
envs  = ["HOMEBREW_PREFIX"]
`
	var m integrations.Manifest
	if _, err := toml.Decode(src, &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Find.Files; len(got) != 1 || got[0] != "Brewfile" {
		t.Errorf("Files = %v", got)
	}
	if got := m.Find.Dirs; len(got) != 1 || got[0] != "/opt/homebrew" {
		t.Errorf("Dirs = %v", got)
	}
	if got := m.Find.Envs; len(got) != 1 || got[0] != "HOMEBREW_PREFIX" {
		t.Errorf("Envs = %v", got)
	}
}

// An author declares the engine that runs the integration's wasm in [runtime]. The interpreter is
// the default: it translates nothing at start-up and generates no native code.
func TestManifestRuntime_EngineDefaultsToTheInterpreter(t *testing.T) {
	var m integrations.Manifest
	if _, err := toml.Decode("[integration]\nbrand = \"x\"\n", &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Runtime.EngineName(); got != integrations.EngineInterpreter {
		t.Errorf("EngineName() = %q, want %q", got, integrations.EngineInterpreter)
	}
	if err := m.Runtime.ValidateEngine(); err != nil {
		t.Errorf("an absent engine is the default, not an error: %v", err)
	}
}

func TestManifestRuntime_EngineIsDeclared(t *testing.T) {
	var m integrations.Manifest
	if _, err := toml.Decode("[runtime]\nengine = \"compiler\"\n", &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Runtime.EngineName(); got != integrations.EngineCompiler {
		t.Errorf("EngineName() = %q, want %q", got, integrations.EngineCompiler)
	}
	if err := m.Runtime.ValidateEngine(); err != nil {
		t.Errorf("compiler is a valid engine: %v", err)
	}
}

func TestManifestRuntime_AnUnknownEngineIsRejected(t *testing.T) {
	var m integrations.Manifest
	if _, err := toml.Decode("[runtime]\nengine = \"jit\"\n", &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Runtime.ValidateEngine(); err == nil {
		t.Error("an unknown engine must be rejected, not silently run on some default")
	}
}

// An author may declare how much memory the guest needs, in bytes; omitted, the host default applies.
func TestManifestRuntime_MemoryDefaultsAndIsDeclaredInBytes(t *testing.T) {
	var none, set integrations.Manifest
	if _, err := toml.Decode("[integration]\nbrand = \"x\"\n", &none); err != nil {
		t.Fatal(err)
	}
	if _, err := toml.Decode("[runtime]\nmemory_limit = 8388608\n", &set); err != nil {
		t.Fatal(err)
	}
	if got := none.Runtime.MemoryLimit(); got != integrations.DefaultMemoryLimit {
		t.Errorf("default = %d bytes, want %d", got, integrations.DefaultMemoryLimit)
	}
	if got := set.Runtime.MemoryLimit(); got != 8388608 {
		t.Errorf("declared = %d bytes, want 8388608", got)
	}
	if got := set.Runtime.MemoryPages(); got != 128 {
		t.Errorf("8388608 bytes is %d pages, want 128", got)
	}
}

// A limit that is not a whole number of 64 KiB pages rounds up: a guest is never given less than it asked for.
func TestManifestRuntime_MemoryRoundsUpToWholePages(t *testing.T) {
	if got := (integrations.ManifestRuntime{MemoryBytes: 65537}).MemoryPages(); got != 2 {
		t.Errorf("65537 bytes is %d pages, want 2", got)
	}
}

func TestManifestRuntime_MemoryOutOfRangeIsRejected(t *testing.T) {
	for _, n := range []int64{-1, 1, 65535, integrations.MaxMemoryLimit + 1} {
		if err := (integrations.ManifestRuntime{MemoryBytes: n}).ValidateMemory(); err == nil {
			t.Errorf("%d bytes must be rejected (negative, under one page, or past the ceiling)", n)
		}
	}
	for _, n := range []int64{0, 65536, integrations.MaxMemoryLimit} {
		if err := (integrations.ManifestRuntime{MemoryBytes: n}).ValidateMemory(); err != nil {
			t.Errorf("%d bytes is allowed: %v", n, err)
		}
	}
}

// The environment a command receives is exactly what the manifest claims: nothing is implicit,
// PATH included. A name ending in * claims every variable with that prefix.
func TestManifestCommands_ChildEnvIsExactlyWhatTheManifestClaims(t *testing.T) {
	c := integrations.ManifestCommands{Env: []string{"PATH", "HOME", "GIT_*"}}
	environ := []string{"PATH=/bin", "HOME=/h", "GIT_DIR=/g", "GIT_SSH=ssh", "GITHUB_TOKEN=secret", "AWS_SECRET_ACCESS_KEY=x", "HOMEBREW_PREFIX=/opt"}
	got := c.ChildEnv(environ)
	want := map[string]bool{"PATH=/bin": true, "HOME=/h": true, "GIT_DIR=/g": true, "GIT_SSH=ssh": true}
	if len(got) != len(want) {
		t.Fatalf("ChildEnv = %v, want exactly %v", got, want)
	}
	for _, kv := range got {
		if !want[kv] {
			t.Errorf("unclaimed variable passed through: %s", kv)
		}
	}
}

func TestManifestCommands_PathIsNotImplicit(t *testing.T) {
	got := integrations.ManifestCommands{Env: []string{"HOME"}}.ChildEnv([]string{"PATH=/bin", "HOME=/h"})
	if len(got) != 1 || got[0] != "HOME=/h" {
		t.Errorf("ChildEnv = %v: PATH must be claimed like any other variable", got)
	}
}

func TestManifestCommands_NoClaimMeansAnEmptyEnvironment(t *testing.T) {
	if got := (integrations.ManifestCommands{}).ChildEnv([]string{"PATH=/bin", "HOME=/h"}); len(got) != 0 {
		t.Errorf("ChildEnv = %v, want nothing", got)
	}
}
