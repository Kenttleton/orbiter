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

func TestRoleType(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"manager", "resource"},
		{"runtime", "resource"},
		{"tool", "resource"},
		{"remote", "resource"},
		{"filesystem", "resource"},
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
