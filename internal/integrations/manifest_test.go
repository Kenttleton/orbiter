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
allowed = ["gh", "git", "which"]
timeout_seconds = 30

[shell]
exports = ["GH_TOKEN", "GITHUB_TOKEN"]

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
	if len(m.Commands.Allowed) != 3 || m.Commands.Allowed[0] != "gh" {
		t.Errorf("commands.allowed = %v", m.Commands.Allowed)
	}
	if m.Commands.TimeoutSeconds != 30 {
		t.Errorf("timeout_seconds = %d", m.Commands.TimeoutSeconds)
	}
	if len(m.Shell.Exports) != 2 || m.Shell.Exports[0] != "GH_TOKEN" {
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
