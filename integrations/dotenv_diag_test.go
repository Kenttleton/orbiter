package integrations_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestDotenvRawOutput(t *testing.T) {
	wasmBytes, err := os.ReadFile("/Users/utterback/Documents/personal/orbiter/integrations/dotenv/dotenv.wasm")
	if err != nil {
		t.Fatal(err)
	}
	reg := core.NewRegistry(nil)
	manifest := core.Manifest{
		Integration: core.ManifestIntegration{Brand: "dotenv", Roles: []string{"file"}},
		Commands: core.ManifestCommands{Allowed: []string{"which"}},
	}
	i, err := wasm.Load(context.Background(), manifest, wasmBytes, reg.Settings(), reg, func(_, _ string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	report := i.Scan(core.ResolvedContext{})
	fmt.Printf("Scan report: %+v\n", report)
}
