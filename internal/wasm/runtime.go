package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
)

var (
	sharedRT wazero.Runtime
	once     sync.Once
)

// SharedRuntime returns the process-wide wazero runtime with the "orbiter" host
// module pre-instantiated. Panics on first-call failure (startup-time misconfiguration).
func SharedRuntime(ctx context.Context) wazero.Runtime {
	once.Do(func() {
		rt, err := newRuntime(ctx)
		if err != nil {
			panic(fmt.Sprintf("wasm: init shared runtime: %v", err))
		}
		sharedRT = rt
	})
	return sharedRT
}
