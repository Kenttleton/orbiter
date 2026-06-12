package wasm_test

import (
	"context"
	"testing"

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
