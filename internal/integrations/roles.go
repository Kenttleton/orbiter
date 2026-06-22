package integrations

const (
	ResourceRoleFilesystem  = "filesystem"
	ResourceRoleManager     = "manager"
	ResourceRoleRuntime     = "runtime"
	ResourceRoleTool        = "tool"
	ResourceRoleRemote      = "remote"
	ResourceRoleShell       = "shell"
	ResourceRoleExport      = "export"
	ResourceRoleMultiplexer = "multiplexer"
)

const (
	TransponderRoleFile     = "file"
	TransponderRoleEnv      = "env"
	TransponderRoleKeychain = "keychain"
	TransponderRoleVault    = "vault"
	TransponderRoleAgent    = "agent"
)

const (
	IntegrationTypeResource    = "resource"
	IntegrationTypeTransponder = "transponder"
)

// RoleTypes maps every role to its type ("resource" or "transponder").
// Orbiter owns this mapping statically — integrations never declare their type.
var RoleTypes = map[string]string{
	ResourceRoleFilesystem:  IntegrationTypeResource,
	ResourceRoleManager:     IntegrationTypeResource,
	ResourceRoleRuntime:     IntegrationTypeResource,
	ResourceRoleTool:        IntegrationTypeResource,
	ResourceRoleRemote:      IntegrationTypeResource,
	ResourceRoleShell:       IntegrationTypeResource,
	ResourceRoleExport:      IntegrationTypeResource,
	ResourceRoleMultiplexer: IntegrationTypeResource,
	TransponderRoleFile:     IntegrationTypeTransponder,
	TransponderRoleEnv:      IntegrationTypeTransponder,
	TransponderRoleKeychain: IntegrationTypeTransponder,
	TransponderRoleVault:    IntegrationTypeTransponder,
	TransponderRoleAgent:    IntegrationTypeTransponder,
}

// RoleType returns "resource", "transponder", or "" for unknown roles.
func RoleType(role string) string {
	return RoleTypes[role]
}
