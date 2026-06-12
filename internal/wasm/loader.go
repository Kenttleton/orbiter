package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WASMIntegration wraps a live WASM module instance and implements integrations.Integration.
// The module is instantiated once; exported functions (detect, initialize, scan, calibrate)
// are called directly — Lambda-style: stateless, independently invocable, no restart cost.
// mu serializes concurrent calls for Phase 2.5; Phase 4 will replace it with a pool.
type WASMIntegration struct {
	manifest integrations.Manifest
	mod      api.Module
	mu       sync.Mutex
}

// Load compiles wasmBytes, instantiates the module, and returns a WASMIntegration
// ready to serve calls. The module stays alive for the lifetime of the integration.
func Load(ctx context.Context, manifest integrations.Manifest, wasmBytes []byte) (*WASMIntegration, error) {
	rt := SharedRuntime(ctx)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	mod, err := rt.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().
			WithName("").
			WithStdout(io.Discard).
			WithStderr(io.Discard),
	)
	if err != nil {
		compiled.Close(ctx)
		return nil, fmt.Errorf("instantiate wasm module: %w", err)
	}

	return &WASMIntegration{manifest: manifest, mod: mod}, nil
}

func (w *WASMIntegration) Meta() integrations.Manifest { return w.manifest }

func (w *WASMIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "detect", input)
	if err != nil {
		return integrations.DetectReport{}
	}
	var report integrations.DetectReport
	json.Unmarshal(out, &report)
	return report
}

func (w *WASMIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "initialize", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	json.Unmarshal(out, &report)
	return report
}

func (w *WASMIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "scan", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	json.Unmarshal(out, &report)
	return report
}

func (w *WASMIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "calibrate", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	json.Unmarshal(out, &report)
	return report
}

// invoke calls a named exported function on the live module instance.
// callState is threaded through context so host functions can read/write the
// JSON payload without any shared mutable state on the struct.
func (w *WASMIntegration) invoke(ctx context.Context, fn string, input []byte) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cs := &callState{input: input}
	ctx = context.WithValue(ctx, callStateKey{}, cs)

	exported := w.mod.ExportedFunction(fn)
	if exported == nil {
		return nil, fmt.Errorf("function %q not exported by wasm module", fn)
	}

	if _, err := exported.Call(ctx); err != nil {
		return nil, fmt.Errorf("call %q: %w", fn, err)
	}
	return cs.output, nil
}

var _ integrations.Integration = (*WASMIntegration)(nil)
