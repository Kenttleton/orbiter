package git

import (
	"context"
	_ "embed"
	"log"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations/wasm"
)

//go:embed git.wasm
var wasmBytes []byte

func init() {
	manifest := integrations.Manifest{
		Integration: integrations.ManifestIntegration{
			Type:  "resource",
			Role:  "tool",
			Brand: "git",
		},
		Detection: integrations.ManifestDetection{
			Files: []string{".git/config"},
		},
	}

	i, err := wasm.Load(context.Background(), manifest, wasmBytes)
	if err != nil {
		log.Printf("orbiter: failed to load git WASM integration: %v", err)
		return
	}
	integrations.Register("tool", "git", i)
}
