package wasm_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

func TestSharedRuntime_SameInstance(t *testing.T) {
	ctx := context.Background()
	r1 := wasm.SharedRuntime(ctx)
	r2 := wasm.SharedRuntime(ctx)
	if r1 != r2 {
		t.Error("SharedRuntime should return the same instance every call")
	}
}

func TestSharedRuntime_NotNil(t *testing.T) {
	ctx := context.Background()
	r := wasm.SharedRuntime(ctx)
	if r == nil {
		t.Error("SharedRuntime returned nil")
	}
}

func TestRuntimeFor_OneSharedRuntimePerEngine(t *testing.T) {
	ctx := context.Background()
	interp, err := wasm.RuntimeFor(ctx, integrations.EngineInterpreter, 0)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := wasm.RuntimeFor(ctx, integrations.EngineCompiler, 0)
	if err != nil {
		t.Fatal(err)
	}
	if interp == compiler {
		t.Error("the two engines must be two runtimes")
	}
	again, _ := wasm.RuntimeFor(ctx, integrations.EngineInterpreter, 0)
	if again != interp {
		t.Error("an engine's runtime is shared by every integration that declares it")
	}
	if wasm.SharedRuntime(ctx) != interp {
		t.Error("the default runtime is the interpreter")
	}
}

func TestRuntimeFor_UnknownEngineFails(t *testing.T) {
	if _, err := wasm.RuntimeFor(context.Background(), "jit", 0); err == nil {
		t.Error("an unknown engine must fail, not fall back")
	}
}
