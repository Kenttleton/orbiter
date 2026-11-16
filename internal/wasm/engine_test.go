package wasm_test

import (
	"context"
	"os"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// guestWasm reads a prebuilt test guest. The guests are built by `just build-test-guests`; the
// tests never compile anything.
func guestWasm(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/guests/" + name + ".wasm")
	if err != nil {
		t.Fatalf("test guest %s: %v (run `just build-test-guests`)", name, err)
	}
	return b
}

func manifestFor(engine string) integrations.Manifest {
	var m integrations.Manifest
	m.Integration.Brand = "tester"
	m.Runtime.Engine = engine
	return m
}

// Both engines run the same guest to the same answer: the engine is a performance choice.
func TestLoad_EitherEngineRunsTheGuest(t *testing.T) {
	for _, engine := range []string{"", integrations.EngineInterpreter, integrations.EngineCompiler} {
		i, err := wasm.Load(context.Background(), manifestFor(engine), guestWasm(t, "echo"), nil, nil, wasm.Prompts{})
		if err != nil {
			t.Fatalf("engine %q: %v", engine, err)
		}
		if got := i.Engine(); got != manifestFor(engine).Runtime.EngineName() {
			t.Errorf("engine %q: Engine() = %q", engine, got)
		}
		report := i.Scan(integrations.ResolvedContext{})
		if report.Error != "" {
			t.Errorf("engine %q: scan error %q", engine, report.Error)
		}
	}
}

func TestLoad_AnUnknownEngineFailsToLoad(t *testing.T) {
	if _, err := wasm.Load(context.Background(), manifestFor("jit"), guestWasm(t, "echo"), nil, nil, wasm.Prompts{}); err == nil {
		t.Error("an integration declaring an unknown engine must not load")
	}
}
