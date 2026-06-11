package golang

import (
	"context"
	_ "embed"
	"log"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations/wasm"
)

//go:embed golang.wasm
var wasmBytes []byte

func init() {
	manifest := integrations.Manifest{
		Integration: integrations.ManifestIntegration{
			Type:  "resource",
			Role:  "runtime",
			Brand: "go",
		},
		Detection: integrations.ManifestDetection{
			Files: []string{"go.mod", "go.sum"},
		},
	}

	i, err := wasm.Load(context.Background(), manifest, wasmBytes)
	if err != nil {
		log.Printf("orbiter: failed to load Go WASM integration: %v", err)
		return
	}
	integrations.Register("runtime", "go", i)
}
