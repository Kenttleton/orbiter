package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/tetratelabs/wazero"
)

// runtimes holds one wazero runtime per engine, created the first time an integration declares
// that engine. A runtime nobody asks for is never created, so an engine nobody uses costs nothing.
var (
	runtimesMu sync.Mutex
	runtimes   = map[string]wazero.Runtime{}
)

// RuntimeFor returns the process-wide runtime for an engine ("" is the default, the interpreter)
// and a memory ceiling in pages (0 is the default), with the "orbiter" host module pre-instantiated.
// Integrations that declare the same engine and memory share a runtime.
func RuntimeFor(ctx context.Context, engine string, memoryPages uint32) (wazero.Runtime, error) {
	if engine == "" {
		engine = integrations.EngineInterpreter
	}
	if memoryPages == 0 {
		memoryPages = integrations.ManifestRuntime{}.MemoryPages()
	}
	key := fmt.Sprintf("%s/%d", engine, memoryPages)
	runtimesMu.Lock()
	defer runtimesMu.Unlock()
	if rt, ok := runtimes[key]; ok {
		return rt, nil
	}
	rt, err := newRuntime(ctx, engine, memoryPages)
	if err != nil {
		return nil, err
	}
	runtimes[key] = rt
	return rt, nil
}

// SharedRuntime returns the default (interpreter) runtime. Panics on failure: the default engine
// cannot fail to start unless the process is misconfigured.
func SharedRuntime(ctx context.Context) wazero.Runtime {
	rt, err := RuntimeFor(ctx, integrations.EngineInterpreter, 0)
	if err != nil {
		panic(fmt.Sprintf("wasm: init shared runtime: %v", err))
	}
	return rt
}
