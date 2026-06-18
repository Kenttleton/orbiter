package integrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

//go:embed golang/golang.wasm golang/manifest.toml git/git.wasm git/manifest.toml node/node.wasm node/manifest.toml make/make.wasm make/manifest.toml dotenv/dotenv.wasm dotenv/manifest.toml python/python.wasm python/manifest.toml rust/rust.wasm rust/manifest.toml brew/brew.wasm brew/manifest.toml uv/uv.wasm uv/manifest.toml rustup/rustup.wasm rustup/manifest.toml docker/docker.wasm docker/manifest.toml macos/macos.wasm macos/manifest.toml onepassword/onepassword.wasm onepassword/manifest.toml ssh/ssh.wasm ssh/manifest.toml nvm/nvm.wasm nvm/manifest.toml just/just.wasm just/manifest.toml shell/shell.wasm shell/manifest.toml asdf/asdf.wasm asdf/manifest.toml local/local.wasm local/manifest.toml vscode/vscode.wasm vscode/manifest.toml github/github.wasm github/manifest.toml
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
// approve is passed to each loaded WASMIntegration; nil uses StdinApproveFunc.
// This is used to load third-party integrations the Captain has installed,
// and is called at startup for every command that opens the StarChart.
func LoadInstalled(dir string, registry *core.Registry, approve wasm.ApproveFunc) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	ctx := context.Background()
	for _, e := range entries {
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

		i, err := wasm.Load(ctx, manifest, wasmBytes, registry.Settings(), registry, approve)
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

// InstalledInfo describes the on-disk state of one installed integration.
type InstalledInfo struct {
	Dir      string   // absolute path to the integration directory
	Checksum [32]byte // SHA256 of the installed .wasm file
}

// InstalledState reads dir and returns a map keyed by brand.
// Directories without a readable manifest.toml or matching .wasm are skipped.
// Returns an empty map (no error) when dir does not exist.
func InstalledState(dir string) (map[string]InstalledInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]InstalledInfo{}, nil
		}
		return nil, err
	}
	result := make(map[string]InstalledInfo)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		name := e.Name()
		entryDir := filepath.Join(dir, name)

		manifestBytes, err := os.ReadFile(filepath.Join(entryDir, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}

		wasmBytes, err := os.ReadFile(filepath.Join(entryDir, name+".wasm"))
		if err != nil {
			continue
		}
		result[manifest.Integration.Brand] = InstalledInfo{
			Dir:      entryDir,
			Checksum: sha256.Sum256(wasmBytes),
		}
	}
	return result, nil
}

// CatalogEntryState pairs a CatalogEntry with its current install status.
type CatalogEntryState struct {
	CatalogEntry
	Installed       bool // true if a directory for this brand exists in dir
	ChecksumMatches bool // true if the installed WASM byte-matches the bundled WASM
}

// CatalogEntriesWithState returns all catalog entries annotated with their
// install state from dir. Use this to populate the vessel init checklist.
func CatalogEntriesWithState(dir string) ([]CatalogEntryState, error) {
	installed, err := InstalledState(dir)
	if err != nil {
		return nil, err
	}
	bundled := bundledChecksums()

	entries := CatalogEntries()
	result := make([]CatalogEntryState, len(entries))
	for i, e := range entries {
		state := CatalogEntryState{CatalogEntry: e}
		if info, ok := installed[e.Brand]; ok {
			state.Installed = true
			if bc, ok := bundled[e.Brand]; ok {
				state.ChecksumMatches = bc == info.Checksum
			}
		}
		result[i] = state
	}
	return result, nil
}

// bundledChecksums returns a map from brand to SHA256 of the embedded WASM.
func bundledChecksums() map[string][32]byte {
	dirs, _ := fs.ReadDir(bundleFS, ".")
	result := make(map[string][32]byte)
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
		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			continue
		}
		result[manifest.Integration.Brand] = sha256.Sum256(wasmBytes)
	}
	return result
}

// ExtractSelected writes the WASM and manifest for each selected catalog entry
// to dir/<brand>/. Existing files are overwritten (safe upgrade path).
// The directory for dir is created if it does not exist.
func ExtractSelected(entries []CatalogEntry, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create integrations dir: %w", err)
	}

	dirs, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		return err
	}

	wanted := make(map[string]bool, len(entries))
	for _, e := range entries {
		wanted[e.Brand] = true
	}

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

		brand := manifest.Integration.Brand
		destDir := filepath.Join(dir, brand)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", brand, err)
		}

		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			return fmt.Errorf("read wasm for %s: %w", brand, err)
		}
		if err := os.WriteFile(filepath.Join(destDir, "manifest.toml"), manifestBytes, 0644); err != nil {
			return fmt.Errorf("write manifest for %s: %w", brand, err)
		}
		if err := os.WriteFile(filepath.Join(destDir, brand+".wasm"), wasmBytes, 0644); err != nil {
			return fmt.Errorf("write wasm for %s: %w", brand, err)
		}
		wanted[brand] = false // mark as written
	}
	for brand, stillWanted := range wanted {
		if stillWanted {
			return fmt.Errorf("brand %q not found in bundle", brand)
		}
	}
	return nil
}
