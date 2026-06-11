package wasm

import (
	"context"
	"encoding/json"
	"os/exec"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type callStateKey struct{}

type callState struct {
	input  []byte
	output []byte
}

// newRuntime creates a wazero Runtime with WASI support and the "orbiter" host
// module pre-instantiated. The returned runtime is ready to compile guest modules.
func newRuntime(ctx context.Context) (wazero.Runtime, error) {
	rt := wazero.NewRuntime(ctx)

	_, err := rt.NewHostModuleBuilder("orbiter").
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

	n := uint32(len(cs.input))
	if n > max {
		n = max
	}
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
// Executes a shell command described by a JSON spec and writes stdout to guest memory.
func runCommandFn(ctx context.Context, mod api.Module, stack []uint64) {
	specPtr := uint32(stack[0])
	specLen := uint32(stack[1])
	outPtr := uint32(stack[2])
	outMax := uint32(stack[3])

	specBytes, ok := mod.Memory().Read(specPtr, specLen)
	if !ok {
		stack[0] = 0
		return
	}

	var spec struct {
		Cmd  string   `json:"cmd"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		stack[0] = 0
		return
	}

	out, _ := exec.CommandContext(ctx, spec.Cmd, spec.Args...).Output()

	n := uint32(len(out))
	if n > outMax {
		n = outMax
	}
	if n > 0 {
		mod.Memory().Write(outPtr, out[:n])
	}
	stack[0] = uint64(n)
}
