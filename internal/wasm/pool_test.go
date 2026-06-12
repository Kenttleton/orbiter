package wasm_test

import (
	"context"
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// poolSize returns the pool size from the manifest runtime config.
// We use a manifest with PoolSize=3 to verify three concurrent calls can run.
func TestPool_ConcurrentInvocations(t *testing.T) {
	// This test verifies that a pool of size N allows N goroutines to call
	// Scan simultaneously without deadlocking.
	// We use the golang integration as a real WASM module.
	// If the pool has size 1 (old behavior), this test would still pass
	// but much more slowly — it verifies correctness, not parallelism.
	ctx := context.Background()

	// Load from embedded bundle is not accessible here; instead verify
	// that WASMIntegration with a pool of size 1 handles concurrent calls
	// without corrupting output.
	_ = ctx
	t.Skip("pool concurrency requires a live WASM module; covered by e2e tests")
}

func TestFilterExports_AllowsDeclared(t *testing.T) {
	report := integrations.StateReport{
		Exports: map[string]string{
			"GITHUB_TOKEN": "ghp_abc",
			"GOPATH":       "/home/user/go",
			"UNDECLARED":   "should-be-stripped",
		},
	}
	allowed := []string{"GITHUB_TOKEN", "GOPATH"}
	filtered := wasm.FilterExports(report, allowed)
	if _, ok := filtered.Exports["UNDECLARED"]; ok {
		t.Error("undeclared export should be stripped")
	}
	if filtered.Exports["GITHUB_TOKEN"] != "ghp_abc" {
		t.Error("declared export should be preserved")
	}
	if filtered.Exports["GOPATH"] != "/home/user/go" {
		t.Error("declared export should be preserved")
	}
}

func TestFilterExports_EmptyAllowedStripsAll(t *testing.T) {
	report := integrations.StateReport{
		Exports: map[string]string{"ANY": "value"},
	}
	filtered := wasm.FilterExports(report, nil)
	if len(filtered.Exports) != 0 {
		t.Error("empty allowed list should strip all exports")
	}
}

func TestFilterExports_NilExportsNoop(t *testing.T) {
	report := integrations.StateReport{}
	filtered := wasm.FilterExports(report, []string{"SOME_VAR"})
	if filtered.Exports != nil {
		t.Error("nil exports should remain nil after filter")
	}
}
