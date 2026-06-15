package integrations

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

//go:embed golang/golang.wasm golang/manifest.toml git/git.wasm git/manifest.toml
var bundleFS embed.FS

// CatalogEntry describes a bundled integration available for installation.
type CatalogEntry struct {
	Brand       string
	Name        string
	Description string
	Roles       []string
}

// CatalogEntries returns catalog entries for all integrations bundled with Orbiter.
// The returned slice is always populated from the embedded filesystem at call time.
func CatalogEntries() []CatalogEntry {
	entries, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		return nil
	}
	var catalog []CatalogEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}
		catalog = append(catalog, CatalogEntry{
			Brand:       manifest.Integration.Brand,
			Name:        manifest.Integration.Name,
			Description: manifest.Integration.Description,
			Roles:       manifest.Integration.Roles,
		})
	}
	return catalog
}

// InstallSelected loads and registers the integrations matching the given
// catalog entries into the provided registry. Entries whose WASM file cannot
// be loaded are logged and skipped.
// approve is the Captain prompt func passed to each WASM integration; nil uses StdinApproveFunc.
func InstallSelected(entries []CatalogEntry, registry *core.Registry, approve wasm.ApproveFunc) error {
	dirs, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		return err
	}
	// Build a set of brands to install.
	wanted := make(map[string]bool, len(entries))
	for _, e := range entries {
		wanted[e.Brand] = true
	}

	ctx := context.Background()
	installed := make(map[string]bool, len(wanted))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()

		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}
		if !wanted[manifest.Integration.Brand] {
			continue
		}

		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			log.Printf("orbiter: wasm for %s: %v", name, err)
			continue
		}

		i, err := wasm.Load(ctx, manifest, wasmBytes, registry.Settings(), registry, approve)
		if err != nil {
			log.Printf("orbiter: load %s: %v", name, err)
			continue
		}
		for _, role := range manifest.Integration.Roles {
			registry.Register(role, manifest.Integration.Brand, i)
		}
		installed[manifest.Integration.Brand] = true
	}
	for brand := range wanted {
		if !installed[brand] {
			log.Printf("orbiter: catalog: brand %q not found in bundle", brand)
		}
	}
	return nil
}

// LoadInstalled scans dir for integration directories and loads any that
// contain a manifest.toml and a matching <name>.wasm file. Loaded integrations
// are registered in the provided registry.
// This is used to load third-party integrations the Captain has installed.
func LoadInstalled(dir string, registry *core.Registry) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty dir is not an error
		}
		return err
	}

	ctx := context.Background()
	for _, e := range entries {
		// Skip symlinks — third-party integration dirs must be real directories.
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		name := e.Name()
		manifestPath := filepath.Join(dir, name, "manifest.toml")
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			log.Printf("orbiter: parse manifest %s: %v", manifestPath, err)
			continue
		}

		wasmPath := filepath.Join(dir, name, name+".wasm")
		wasmBytes, err := os.ReadFile(wasmPath)
		if err != nil {
			log.Printf("orbiter: wasm for %s: %v", name, err)
			continue
		}

		i, err := wasm.Load(ctx, manifest, wasmBytes, core.DefaultSettings, registry, nil)
		if err != nil {
			log.Printf("orbiter: load %s: %v", name, err)
			continue
		}
		for _, role := range manifest.Integration.Roles {
			registry.Register(role, manifest.Integration.Brand, i)
		}
	}
	return nil
}

// DefaultIntegrationsDir returns the default directory for installed integrations.
// This is ~/.orbiter/integrations.
func DefaultIntegrationsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".orbiter/integrations"
	}
	return filepath.Join(home, ".orbiter", "integrations")
}
