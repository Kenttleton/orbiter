package integrations

const (
	ResourceRoleManager    = "manager"
	ResourceRoleRuntime    = "runtime"
	ResourceRoleTool       = "tool"
	ResourceRoleRemote     = "remote"
	ResourceRoleFilesystem = "filesystem"
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
	ResourceRoleManager:     IntegrationTypeResource,
	ResourceRoleRuntime:     IntegrationTypeResource,
	ResourceRoleTool:        IntegrationTypeResource,
	ResourceRoleRemote:      IntegrationTypeResource,
	ResourceRoleFilesystem:  IntegrationTypeResource,
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
