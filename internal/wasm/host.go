package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type callStateKey struct{}

type callState struct {
	// hostNanos is the time spent inside host functions that wait on something outside the guest
	// (the Captain, a command); hostSince is when the current host call began, or 0 when the
	// guest is computing. The guest's compute budget does not count either.
	hostNanos   atomic.Int64
	hostSince   atomic.Int64
	input       []byte
	output      []byte
	brand       string
	allowed     []string                      // from manifest.Commands.Allowed
	commands    integrations.ManifestCommands // the manifest's claim, for the child environment
	timeout     int                           // manifest.Commands.TimeoutSeconds (0 = no limit)
	approve     ApproveFunc
	alwaysAllow AlwaysAllowFunc
	settings    *integrations.SettingsStore
	registry    *integrations.Registry
}

// runtimeConfig is the wazero configuration for an engine.
func runtimeConfig(engine string, memoryPages uint32) (wazero.RuntimeConfig, error) {
	switch engine {
	case integrations.EngineInterpreter:
		return limited(wazero.NewRuntimeConfigInterpreter(), memoryPages), nil
	case integrations.EngineCompiler:
		return limited(wazero.NewRuntimeConfigCompiler(), memoryPages), nil
	}
	return nil, fmt.Errorf("unknown runtime engine %q", engine)
}

// limited applies the limits every engine shares: a ceiling on guest memory, and a guest that stops
// when its call's context ends (that is how GuestTimeout takes effect).
func limited(cfg wazero.RuntimeConfig, memoryPages uint32) wazero.RuntimeConfig {
	return cfg.WithMemoryLimitPages(memoryPages).WithCloseOnContextDone(true)
}

// newRuntime creates a wazero Runtime for an engine with the "orbiter" host module
// pre-instantiated. The returned runtime is ready to compile guest modules.
func newRuntime(ctx context.Context, engine string, memoryPages uint32) (wazero.Runtime, error) {
	cfg, err := runtimeConfig(engine, memoryPages)
	if err != nil {
		return nil, err
	}
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)

	// AssemblyScript modules always import env.abort (message, fileName, line, col).
	// Provide a no-op implementation so AS-compiled guests can be instantiated.
	_, err = rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(func(_ context.Context, _ api.Module, _ []uint64) {}),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{},
		).
		Export("abort").
		Instantiate(ctx)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	_, err = rt.NewHostModuleBuilder("orbiter").
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(readInputFn),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{api.ValueTypeI32},
		).
		Export("read_input").
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(writeOutputFn),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{},
		).
		Export("write_output").
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(runCommandFn),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{api.ValueTypeI32},
		).
		Export("run_command").
		Instantiate(ctx)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}

	return rt, nil
}

func (cs *callState) enterHost() { cs.hostSince.Store(time.Now().UnixNano()) }

func (cs *callState) leaveHost() {
	if since := cs.hostSince.Swap(0); since != 0 {
		cs.hostNanos.Add(time.Now().UnixNano() - since)
	}
}

// guestElapsed is the time the guest has spent computing in this call: wall time since start,
// minus the time spent in host functions that waited on someone else.
func (cs *callState) guestElapsed(start time.Time) time.Duration {
	now := time.Now().UnixNano()
	host := cs.hostNanos.Load()
	if since := cs.hostSince.Load(); since != 0 {
		host += now - since
	}
	return time.Duration(now - start.UnixNano() - host)
}

// readInputFn implements orbiter.read_input(ptr, max) -> n.
// Copies callState.input into guest memory at ptr, returns bytes written.
func readInputFn(ctx context.Context, mod api.Module, stack []uint64) {
	ptr := uint32(stack[0])
	max := uint32(stack[1])

	cs, _ := ctx.Value(callStateKey{}).(*callState)
	if cs == nil {
		stack[0] = 0
		return
	}

	n := min(uint32(len(cs.input)), max)
	if n > 0 {
		mod.Memory().Write(ptr, cs.input[:n])
	}
	stack[0] = uint64(n)
}

// writeOutputFn implements orbiter.write_output(ptr, len).
// Reads guest memory at ptr and stores it in callState.output.
func writeOutputFn(ctx context.Context, mod api.Module, stack []uint64) {
	ptr := uint32(stack[0])
	length := uint32(stack[1])

	cs, _ := ctx.Value(callStateKey{}).(*callState)
	if cs == nil {
		return
	}

	data, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return
	}
	cs.output = make([]byte, len(data))
	copy(cs.output, data)
}

// runCommandFn implements orbiter.run_command(specPtr, specLen, outPtr, max) -> n.
// Applies a five-stage security gate before executing any host command. The policy is
// transparency: everything is banned except the commands the integration's human-readable
// manifest declares, so there is no list of forbidden names.
//
//  1. Quarantine check — blocked integrations return immediately
//  2. Allowlist check  — the command must be in manifest.Commands.Allowed; an integration that
//     declares nothing may run nothing, and one that strays is quarantined
//  3. Trust check      — previously always-allowed commands run without a prompt
//  4. Captain prompt   — approval for commands not yet trusted
//  5. Execute          — run with optional timeout; write output to guest memory
func runCommandFn(ctx context.Context, mod api.Module, stack []uint64) {
	specPtr := uint32(stack[0])
	specLen := uint32(stack[1])
	outPtr := uint32(stack[2])
	outMax := uint32(stack[3])

	cs, _ := ctx.Value(callStateKey{}).(*callState)
	if cs == nil {
		stack[0] = 0
		return
	}
	cs.enterHost()
	defer cs.leaveHost()

	specBytes, ok := mod.Memory().Read(specPtr, specLen)
	if !ok {
		stack[0] = 0
		return
	}

	// Build fullCmd from the JSON spec {cmd, args} sent by the guest.
	var spec struct {
		Cmd  string   `json:"cmd"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		stack[0] = 0
		return
	}
	parts := append([]string{spec.Cmd}, spec.Args...)
	fullCmd := strings.Join(parts, " ")
	executable := spec.Cmd

	out, code := runGate(ctx, cs, fullCmd, executable, spec.Args)
	if code != 0 {
		stack[0] = 0 // 0 bytes written; guest handles empty output
		return
	}

	n := min(uint32(len(out)), outMax)
	if n > 0 {
		mod.Memory().Write(outPtr, out[:n])
	}
	stack[0] = uint64(n)
}

// runGate applies the six-stage security gate and, on success, executes the
// command. It returns the combined output and a return code (0 = success, 1 = error/denied).
// Extracting the gate to a standalone function makes it directly unit-testable
// without a live wazero module.
func runGate(ctx context.Context, cs *callState, fullCmd, executable string, args []string) ([]byte, uint32) {
	executableBase := filepath.Base(executable)
	// The allowlist names commands, not paths: a path whose last element matches a declared name
	// is some other program, so it counts as undeclared.
	if executable != executableBase || strings.ContainsRune(executable, 0) {
		executableBase = executable
	}

	// Stage 1: Quarantine check
	if cs.registry != nil && cs.registry.IsQuarantined(cs.brand) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "quarantined",
			Reason:  "integration is quarantined",
		})
		return nil, 1
	}

	// Stage 2: Allowlist check. Only what the manifest declares may run.
	if !slices.Contains(cs.allowed, executableBase) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "denied",
			Reason:  "not in manifest allowlist",
		})
		if cs.registry != nil {
			_ = cs.registry.QuarantineBrand(cs.brand, "attempted undeclared command: "+executableBase)
		}
		return nil, 1
	}

	// Stage 3: Trust check — already always-allowed, skip prompt
	if cs.settings != nil && cs.settings.IsAllowed(cs.brand, fullCmd) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "allowed",
		})
		return execute(ctx, cs, executable, args)
	}

	// Stage 4: Captain prompt
	approveFn := cs.approve
	if approveFn == nil {
		approveFn = StdinApproveFunc
	}
	if !approveFn(cs.brand, fullCmd) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "captain_denied",
		})
		return nil, 1
	}
	AuditLog(AuditEntry{
		At:      time.Now().UTC(),
		Brand:   cs.brand,
		Command: fullCmd,
		Outcome: "captain_approved",
	})

	// Ask whether to always-allow this exact command.
	if cs.settings != nil {
		alwaysAllowFn := cs.alwaysAllow
		if alwaysAllowFn == nil {
			alwaysAllowFn = StdinAlwaysAllowFunc
		}
		if alwaysAllowFn(cs.brand, fullCmd) {
			_ = cs.settings.Allow(cs.brand, fullCmd)
		}
	}

	// Stage 5: Execute
	return execute(ctx, cs, executable, args)
}

// execute runs executable with args, returning combined output. A command that runs past its
// timeout (the manifest's, or DefaultCommandTimeout) or writes more than MaxCommandOutput is killed
// and counts as a failure.
func execute(ctx context.Context, cs *callState, executable string, args []string) ([]byte, uint32) {
	timeout := DefaultCommandTimeout
	if cs.timeout > 0 {
		timeout = time.Duration(cs.timeout) * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, executable, args...)
	cmd.Env = cs.commands.ChildEnv(os.Environ()) // exactly what the manifest claims
	cmd.WaitDelay = time.Second                  // do not wait on a grandchild that keeps the pipe open
	out := &cappedBuffer{max: MaxCommandOutput, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, out

	if err := cmd.Run(); err != nil || out.exceeded {
		return out.buf.Bytes(), 1
	}
	return out.buf.Bytes(), 0
}

// cappedBuffer collects output up to max bytes; past that it stops the command.
type cappedBuffer struct {
	buf      bytes.Buffer
	max      int
	exceeded bool
	cancel   context.CancelFunc
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); len(p) > room {
		c.buf.Write(p[:max(room, 0)])
		c.exceeded = true
		c.cancel()
		return len(p), nil
	}
	return c.buf.Write(p)
}
