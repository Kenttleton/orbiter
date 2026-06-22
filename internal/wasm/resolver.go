package wasm

import "github.com/Kenttleton/orbiter/internal/integrations"

// ResolveBinaries resolves each declared binary name to an absolute path
// by delegating to the filesystem integration's FindBinary function.
// Returns a map of name → path; missing binaries have empty-string values.
func ResolveBinaries(names []string, osName string) map[string]string {
	result := make(map[string]string)
	for _, name := range names {
		result[name] = integrations.FindBinary(name, osName)
	}
	return result
}
