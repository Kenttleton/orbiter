package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ApproveFunc is called when a WASM integration attempts a command that has not
// been previously allowed. It must print a prompt and return true if the Captain
// approves this run, or false to deny.
type ApproveFunc func(brand, fullCmd string) bool

// StdinApproveFunc is the default ApproveFunc, used when running interactively.
func StdinApproveFunc(brand, fullCmd string) bool {
	fmt.Fprintf(os.Stderr, "\n  orbiter: integration %q wants to run:\n    %s\n  Allow? [y/N] ", brand, fullCmd)
	var response string
	fmt.Fscan(os.Stdin, &response)
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// AlwaysAllowFunc is called after Captain approval to ask whether this exact
// command should always be allowed. Returns true to persist the trust entry.
type AlwaysAllowFunc func(brand, fullCmd string) bool

// StdinAlwaysAllowFunc is the default AlwaysAllowFunc.
func StdinAlwaysAllowFunc(brand, fullCmd string) bool {
	fmt.Fprintf(os.Stderr, "  Always allow this exact command for %q? [y/N] ", brand)
	var response string
	fmt.Fscan(os.Stdin, &response)
	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

// WASMIntegration wraps a pool of live WASM module instances and implements integrations.Integration.
// Exported functions (detect, initialize, scan, calibrate) are called Lambda-style: stateless,
// independently invocable, no restart cost. The channel-based pool allows N concurrent calls
// where N = manifest.Runtime.PoolSize (defaults to 1).
type WASMIntegration struct {
	manifest    integrations.Manifest
	pool        chan api.Module
	approve     ApproveFunc
	alwaysAllow AlwaysAllowFunc
	settings    *integrations.SettingsStore
	registry    *integrations.Registry
}

// Load compiles wasmBytes, instantiates the module, and returns a WASMIntegration
// ready to serve calls. The module stays alive for the lifetime of the integration.
// If approve is nil, StdinApproveFunc is used.
func Load(ctx context.Context, manifest integrations.Manifest, wasmBytes []byte, settings *integrations.SettingsStore, registry *integrations.Registry, approve ApproveFunc) (*WASMIntegration, error) {
	if approve == nil {
		approve = StdinApproveFunc
	}
	alwaysAllow := StdinAlwaysAllowFunc

	rt := SharedRuntime(ctx)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	poolSize := manifest.Runtime.PoolSize
	if poolSize <= 0 {
		poolSize = 1
	}

	pool := make(chan api.Module, poolSize)
	for i := range poolSize {
		mod, err := rt.InstantiateModule(ctx, compiled,
			wazero.NewModuleConfig().
				WithName("").
				WithStdout(io.Discard).
				WithStderr(io.Discard),
		)
		if err != nil {
			close(pool)
			for m := range pool {
				m.Close(ctx)
			}
			compiled.Close(ctx)
			return nil, fmt.Errorf("instantiate wasm pool[%d]: %w", i, err)
		}
		pool <- mod
	}

	return &WASMIntegration{
		manifest:    manifest,
		pool:        pool,
		approve:     approve,
		alwaysAllow: alwaysAllow,
		settings:    settings,
		registry:    registry,
	}, nil
}

func (w *WASMIntegration) Meta() integrations.Manifest { return w.manifest }

func (w *WASMIntegration) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "detect", input)
	if err != nil {
		return integrations.DetectReport{}
	}
	var report integrations.DetectReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.DetectReport{}
	}
	return report
}

func (w *WASMIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "initialize", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

func (w *WASMIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "scan", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

func (w *WASMIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	input, _ := json.Marshal(ctx)
	out, err := w.invoke(context.Background(), "calibrate", input)
	if err != nil {
		return integrations.StateReport{Error: err.Error()}
	}
	var report integrations.StateReport
	if err := json.Unmarshal(out, &report); err != nil {
		return integrations.StateReport{Error: fmt.Sprintf("unmarshal response: %v", err)}
	}
	report = FilterExports(report, w.manifest.Shell.AllowedEnvs())
	return report
}

// invoke checks out a module from the pool, calls fn, then returns the module.
// callState is threaded through context so host functions can read/write the
// JSON payload without any shared mutable state on the struct.
// On error the module is closed and discarded rather than returned to the pool,
// because a WASM trap may leave the module in a corrupted state.
func (w *WASMIntegration) invoke(ctx context.Context, fn string, input []byte) (_ []byte, retErr error) {
	var mod api.Module
	select {
	case mod = <-w.pool:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() {
		if retErr != nil {
			mod.Close(ctx) // discard; pool shrinks by 1
		} else {
			w.pool <- mod
		}
	}()

	cs := &callState{
		input:       input,
		brand:       w.manifest.Integration.Brand,
		allowed:     w.manifest.Commands.AllowedCmds(),
		timeout:     w.manifest.Commands.TimeoutSeconds,
		approve:     w.approve,
		alwaysAllow: w.alwaysAllow,
		settings:    w.settings,
		registry:    w.registry,
	}
	ctx = context.WithValue(ctx, callStateKey{}, cs)

	exported := mod.ExportedFunction(fn)
	if exported == nil {
		return nil, fmt.Errorf("function %q not exported by wasm module", fn)
	}
	if _, err := exported.Call(ctx); err != nil {
		return nil, fmt.Errorf("call %q: %w", fn, err)
	}
	return cs.output, nil
}

// FilterExports removes entries from report.Exports whose keys are not in
// the manifest's Shell.Exports allowlist. If the allowlist is empty, all
// exports are stripped. Returns a shallow copy of report with the filtered map.
func FilterExports(report integrations.StateReport, allowed []string) integrations.StateReport {
	if report.Exports == nil {
		return report
	}
	if len(allowed) == 0 {
		report.Exports = nil
		return report
	}
	allowSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowSet[k] = struct{}{}
	}
	filtered := make(map[string]string)
	for k, v := range report.Exports {
		if _, ok := allowSet[k]; ok {
			filtered[k] = v
		}
	}
	report.Exports = filtered
	return report
}

var _ integrations.Integration = (*WASMIntegration)(nil)
