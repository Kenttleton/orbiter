package integrations

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"path"

	"github.com/BurntSushi/toml"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

//go:embed golang/golang.wasm golang/manifest.toml
var bundleFS embed.FS

func init() {
	entries, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		log.Printf("orbiter: read bundle: %v", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()

		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			log.Printf("orbiter: manifest for %s: %v", name, err)
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			log.Printf("orbiter: parse manifest for %s: %v", name, err)
			continue
		}

		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			log.Printf("orbiter: wasm for %s: %v", name, err)
			continue
		}

		i, err := wasm.Load(context.Background(), manifest, wasmBytes)
		if err != nil {
			log.Printf("orbiter: load %s: %v", name, err)
			continue
		}
		core.Register(manifest.Integration.Role, manifest.Integration.Brand, i)
	}
}
