package wasm

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type callStateKey struct{}

type callState struct {
	input       []byte
	output      []byte
	brand       string
	allowed     []string // from manifest.Commands.Allowed
	timeout     int      // manifest.Commands.TimeoutSeconds (0 = no limit)
	approve     ApproveFunc
	alwaysAllow AlwaysAllowFunc
	settings    *integrations.SettingsStore
	registry    *integrations.Registry
}

// bannedCommands is a hardcoded list of executables that can never run regardless
// of manifest declarations. Attempting to run any of these auto-quarantines the
// integration.
var bannedCommands = []string{
	"bash", "sh", "zsh", "fish", "dash", "csh", "tcsh", "ksh",
	"pwsh", "powershell", "cmd.exe", "rm", "sudo", "su",
	"chmod", "chown", "dd", "mkfs", "fdisk",
}

// newRuntime creates a wazero Runtime with WASI support and the "orbiter" host
// module pre-instantiated. The returned runtime is ready to compile guest modules.
func newRuntime(ctx context.Context) (wazero.Runtime, error) {
	rt := wazero.NewRuntime(ctx)

	// AssemblyScript modules always import env.abort (message, fileName, line, col).
	// Provide a no-op implementation so AS-compiled guests can be instantiated.
	_, err := rt.NewHostModuleBuilder("env").
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
// Applies a six-stage security gate before executing any host command:
//
//  1. Quarantine check — blocked integrations return immediately
//  2. Banlist check   — hardcoded dangerous executables are always denied
//  3. Allowlist check — command must appear in manifest.Commands.Allowed (if set)
//  4. Trust check     — previously always-allowed commands run without a prompt
//  5. Captain prompt  — interactive approval for untrusted commands
//  6. Execute         — run with optional timeout; write output to guest memory
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

	// Stage 2: Banlist check
	if slices.Contains(bannedCommands, executableBase) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "quarantined",
			Reason:  "executable on banlist: " + executableBase,
		})
		if cs.registry != nil {
			_ = cs.registry.QuarantineBrand(cs.brand, "attempted banned command: "+executableBase)
		}
		return nil, 1
	}

	// Stage 3: Allowlist check (only enforced when manifest declares allowed commands)
	if len(cs.allowed) > 0 && !slices.Contains(cs.allowed, executableBase) {
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

	// Stage 4: Trust check — already always-allowed, skip prompt
	if cs.settings != nil && cs.settings.IsAllowed(cs.brand, fullCmd) {
		AuditLog(AuditEntry{
			At:      time.Now().UTC(),
			Brand:   cs.brand,
			Command: fullCmd,
			Outcome: "allowed",
		})
		return execute(ctx, cs, executable, args)
	}

	// Stage 5: Captain prompt
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

	// Stage 6: Execute
	return execute(ctx, cs, executable, args)
}

// execute runs executable with args and an optional timeout, returning combined output.
func execute(ctx context.Context, cs *callState, executable string, args []string) ([]byte, uint32) {
	var cmd *exec.Cmd
	if cs.timeout > 0 {
		tctx, cancel := context.WithTimeout(ctx, time.Duration(cs.timeout)*time.Second)
		defer cancel()
		cmd = exec.CommandContext(tctx, executable, args...)
	} else {
		cmd = exec.Command(executable, args...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, 1
	}
	return out, 0
}

