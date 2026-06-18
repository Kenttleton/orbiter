# Integrations

Integrations are the bridge between Orbiter's Star Chart and the real tools installed in a developer's environment. Each integration knows how to detect, initialize, scan, and calibrate one specific `role + brand` pairing — for example `runtime/go` or `manager/nvm`.

---

## Model

An integration is a **stateless WASM module**. The host (Orbiter) is the API gateway; the module is the container; each exported function is a serverless handler. No shared runtime state survives between calls.

```
Host (Orbiter)
│
├── detect(DetectContext) → DetectReport
├── initialize(ResolvedContext) → StateReport
├── scan(ResolvedContext) → StateReport
└── calibrate(ResolvedContext) → StateReport
```

This mirrors the AWS Lambda model: the module is instantiated once at startup and kept alive. Each handler call is independently invokable — idempotent and side-effect-free from the host's perspective. Phase 4 will add instance pooling; for now a `sync.Mutex` serializes concurrent calls per integration.

---

## Security

### Threat model

An integration runs code on the Captain's machine. The threat is a module that looks legitimate (copied manifest, plausible brand name) but executes malicious commands — installing malware, exfiltrating keys, establishing persistence.

Defense is layered:

**WASM sandbox** — The module cannot access the filesystem, network, or OS directly. The only way to interact with the outside world is through the three host-imported functions: `read_input`, `write_output`, and `run_command`. The sandbox eliminates entire attack classes.

**`[commands]` allowlist** — Every executable an integration may pass to `run_command` must be declared in the manifest's `[commands] allowed` list. The host rejects any call for an undeclared executable and returns a zero-length result. The integration cannot silently expand its access beyond what it declared upfront.

**Audit log** — Every `run_command` invocation — allowed or rejected — is written to `~/.orbiter/audit.log` as a JSON line with timestamp, brand, command, arguments, exit code, and duration. Rejected calls include `"rejected": true` and the reason. The audit log is the primary forensic tool.

```jsonl
{"ts":"2026-06-11T14:32:01Z","brand":"gh","cmd":"gh","args":["auth","status"],"exit":0,"duration_ms":142}
{"ts":"2026-06-11T14:32:02Z","brand":"gh","cmd":"curl","args":["evil.com"],"exit":-1,"rejected":true,"reason":"not in allowlist"}
```

**No ambient environment** — Subprocesses spawned by `run_command` inherit only `PATH`. No secrets, tokens, or session credentials leak via the environment.

**Validated shell exports** — Integrations communicate required shell state through `StateReport.Exports`. Every key is validated against the manifest's `[shell] exports` allowlist before Orbiter emits any shell directive. Undeclared keys are dropped and logged. Integrations never write to the Captain's shell directly.

**No stored secrets** — API keys, tokens, and passwords are never stored in config or the database. They are collected transiently via interactive prompts (`NeedsInput`/`Responses`) and discarded after use.

### Trust tiers

| Tier | Source | Trust level |
| --- | --- | --- |
| Bundled | `integrations/` in this repo | Reviewed and trusted |
| Third-party | `~/.orbiter/integrations/` | Captain installs explicitly; Orbiter warns on unsigned modules |

### Limitations

The `[commands]` allowlist declares capability claims — it does not prevent an allowed command from receiving malicious arguments. For example, an integration that declares `git` in its allowlist could construct a `git` invocation with harmful flags. The audit log surfaces this; detection is the Captain's responsibility.

**Future work:** WASM module signing and verification, subprocess sandboxing (seccomp/namespaces on Linux, sandbox profiles on macOS).

---

## Handler Contracts

### `detect`

Called during `planet init --discover`. Receives a `DetectContext` describing the project directory and returns whether this integration should be suggested.

**Input** (`DetectContext`):
```json
{
  "platform": { "os": "darwin", "arch": "arm64" },
  "cwd": "/path/to/project",
  "files": { "go.mod": "", "go.sum": "" }
}
```

**Output** (`DetectReport`):
```json
{
  "detected": true,
  "resources": [
    { "role": "runtime", "brand": "go", "version": "1.25.1" }
  ]
}
```

### `initialize`

Called when a resource is first provisioned. Verifies the tool is present and reachable. Returns a `StateReport`.

### `scan`

Called during `orbit survey` to check current state without making changes.

### `calibrate`

Called during `orbit jump` to align the environment to the expected state.

**Output** (`StateReport`) for all three:
```json
{
  "present": true,
  "reachable": true,
  "binary_path": "/usr/local/go/bin/go",
  "in_path": true,
  "manager": "system",
  "observations": ["go version go1.25.1 darwin/arm64"]
}
```

---

## Guest ABI

The WASM module communicates with the host through three imported functions in the `orbiter` module namespace:

```
orbiter.read_input(ptr i32, max i32) -> n i32
    Copies the call's input JSON into guest memory at ptr.
    Returns the number of bytes written (≤ max).

orbiter.write_output(ptr i32, len i32)
    Reads len bytes from guest memory at ptr and stores
    them as this call's output.

orbiter.run_command(specPtr i32, specLen i32, outPtr i32, max i32) -> n i32
    Executes a host command described by a JSON spec in guest memory.
    Writes stdout into guest memory at outPtr.
    Returns bytes written (≤ max).
```

The `run_command` spec is:
```json
{ "cmd": "go", "args": ["version"] }
```

Payload limit: 64 KB per direction. Phase 4 will enforce this at the host boundary.

---

## Building a WASM Integration

### Target and Toolchain

Use **TinyGo** with `-target=wasm-unknown`. Do not use standard Go (`GOOS=wasip1`) or TinyGo's `-target=wasi`.

| Target | `_start` behavior | Exported functions callable? |
|---|---|---|
| `GOOS=wasip1` (std Go) | calls `proc_exit(0)` on return | No — module dies after instantiation |
| TinyGo `-target=wasi` | same as above | No |
| TinyGo `-target=wasm-unknown` | generates `_initialize`, no `proc_exit` | Yes — Lambda model |

The `wasm-unknown` reactor target generates `_initialize` (a TinyGo runtime artifact for one-time module setup — not one of the integration handlers) plus your exported handler functions directly callable on the live instance. `_initialize` is distinct from the `initialize` handler defined below; the naming overlap is coincidental.

### Known TinyGo `wasm-unknown` Restrictions

Two standard library patterns crash at runtime and must be avoided in guest code:

**1. `encoding/json`**

`json.Marshal` and `json.Unmarshal` use `sync.Map` internally to cache type encoders. Under `wasm-unknown`, TinyGo's hashmap runtime (`hashmapInterfaceGet`) accesses out-of-bounds memory:

```
wasm error: out of bounds memory access
  main.runtime.hashmapGet
  main.runtime.hashmapInterfaceGet
  main.(*sync.Map).LoadOrStore
  main.encoding/json.typeEncoder   ← crash
```

**Fix:** Build all JSON manually using `[]byte` append. Do not use `encoding/json` in guest code.

**2. `strings.Builder`**

`strings.Builder.copyCheck()` stores `unsafe.Pointer(b)` (pointer to self) and compares it on each write. Under `wasm-unknown`, this comparison triggers a `unreachable` trap:

```
wasm error: unreachable
  main.(*strings.Builder).copyCheck
  main.(*strings.Builder).WriteString
```

**Fix:** Use `[]byte` and `append` for all string building. `strings.TrimSpace`, `strings.Contains`, and `strings.IndexByte` are safe — only `strings.Builder` is affected.

### Function Exports

Use TinyGo's C-style export syntax, not `//go:wasmexport`:

```go
//export detect
func detect() { ... }

//export initialize
func initialize() { ... }

//export scan
func scan() { ... }

//export calibrate
func calibrate() { ... }
```

`//go:wasmexport` is Go 1.24+ standard library syntax and is not supported by TinyGo.

### Host Function Imports

```go
//go:wasmimport orbiter read_input
func hostReadInput(ptr, max uint32) uint32

//go:wasmimport orbiter write_output
func hostWriteOutput(ptr, length uint32)

//go:wasmimport orbiter run_command
func hostRunCommand(specPtr, specLen, outPtr, max uint32) uint32
```

`//go:wasmimport` works in both TinyGo and standard Go WASM.

---

## Package Structure

Each integration is a Go package under `internal/integrations/<brand>/`:

```
internal/integrations/golang/
├── generate.go          # //go:generate directive
├── golang.wasm          # compiled binary (committed)
├── register.go          # init() self-registers with integrations.Default
└── guest/
    └── main.go          # TinyGo guest code (//go:build tinygo)
```

### `generate.go`

```go
package golang

//go:generate tinygo build -o golang.wasm -target=wasm-unknown ./guest/
```

Run with `go generate ./internal/integrations/golang/` or `just generate`.

### `register.go`

```go
package golang

import (
    "context"
    _ "embed"
    "log"

    "github.com/Kenttleton/orbiter/internal/integrations"
    "github.com/Kenttleton/orbiter/internal/integrations/wasm"
)

//go:embed golang.wasm
var wasmBytes []byte

func init() {
    manifest := integrations.Manifest{
        Integration: integrations.ManifestIntegration{
            Type:  "resource",
            Role:  "runtime",
            Brand: "go",
        },
        Detection: integrations.ManifestDetection{
            Files: []string{"go.mod", "go.sum"},
        },
    }
    i, err := wasm.Load(context.Background(), manifest, wasmBytes)
    if err != nil {
        log.Printf("orbiter: failed to load Go WASM integration: %v", err)
        return
    }
    integrations.Register("runtime", "go", i)
}
```

The WASM binary is embedded at compile time and shipped with the `orbit` binary. No external plugin directory is needed at runtime for bundled integrations.

### `guest/main.go`

The guest must have build tag `//go:build tinygo` so the standard Go toolchain ignores it (it imports `unsafe` in ways that only TinyGo handles correctly for this target). Keep `func main() {}` — TinyGo requires it.

### JSON on wasm-unknown

`encoding/json`, `strings.Builder`, `github.com/tidwall/gjson`, and
`github.com/tidwall/sjson` all have runtime issues on `wasm-unknown`. Use
hand-rolled `[]byte` append for both reading and writing JSON.

**Input parsing** — use `strings.Contains` to detect keys:

```go
// Detect whether "go.mod" appears as a JSON key in the files map.
if !strings.Contains(string(input), `"go.mod"`) { ... }
```

**Output building** — use `jsonBytes` and `[]byte` append:

```go
// jsonBytes returns a JSON-quoted []byte without encoding/json or strings.Builder.
func jsonBytes(s string) []byte {
    const hex = "0123456789abcdef"
    buf := []byte{'"'}
    for i := 0; i < len(s); i++ {
        c := s[i]
        switch c {
        case '"':  buf = append(buf, '\\', '"')
        case '\\': buf = append(buf, '\\', '\\')
        case '\n': buf = append(buf, '\\', 'n')
        case '\r': buf = append(buf, '\\', 'r')
        case '\t': buf = append(buf, '\\', 't')
        default:
            if c < 0x20 {
                buf = append(buf, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
            } else {
                buf = append(buf, c)
            }
        }
    }
    return append(buf, '"')
}
```

Note: `github.com/tidwall/gjson` and `github.com/tidwall/sjson` compile under
TinyGo but crash at runtime on `wasm-unknown` due to their use of
`unicode/utf16` internals. Do not use them in guest code.

---

## Adding a New Integration

There are two delivery paths depending on whether the integration ships with the `orbit` binary or is loaded at runtime.

### Bundled integrations (compile-time)

Bundled integrations live under `internal/integrations/<brand>/` and are compiled directly into the `orbit` binary.

1. Create the directory with `manifest.toml` and your WASM guest package.
2. Run `just build` (or `go generate ./internal/integrations/`).

`go:generate` scans every subdirectory of `internal/integrations/` for a `manifest.toml`, validates the manifest structure, and regenerates `internal/integrations/register.go`:

```go
// Code generated by go generate. DO NOT EDIT.
package integrations

import (
    _ "github.com/Kenttleton/orbiter/internal/integrations/golang"
    _ "github.com/Kenttleton/orbiter/internal/integrations/nvm"
    // ... all discovered packages
)
```

The blank imports trigger each package's `init()`, which calls `integrations.Register`. No manual edits anywhere — drop the directory, run `just build`.

### Runtime integrations (WASM plugin directories)

Phase 3 adds scanning at startup for externally installed integrations. Orbiter checks two locations in order:

```text
<directory containing orbit binary>/integrations/<brand>/
    manifest.toml
    integration.wasm

~/.orbiter/integrations/<brand>/
    manifest.toml
    integration.wasm
```

Any directory containing both files is loaded as a WASM integration and registered at the same `role/brand` key. Bundled (compile-time) integrations take precedence over runtime ones for the same key.

This path does not exist yet — it is Phase 3 scope.

---

## Multi-Role Integrations

A single brand can serve multiple roles by listing them in `[integration] roles`:

```toml
[integration]
brand = "github"
name = "GitHub"
roles = ["tool", "remote", "agent"]
```

The registry registers the integration once per declared role. When a role-specific handler is invoked, the host passes `role` in the `ResolvedContext` so the guest can dispatch to role-specific behavior:

```rust
let role = ctx.role.as_str();
match role {
    "remote" => scan_remote(),
    "agent"  => scan_agent(),
    _        => scan_tool(),
}
```

**Agent role exports:** Agents that need to export credentials to the shell declare the export key in `[shell] exports` in the manifest and write them to the `exports` map in `StateReport`. The host emits `export KEY=VALUE` to the shell eval output during `jump`:

```toml
[shell]
exports = ["GH_TOKEN"]
```

Other integrations that consume this credential declare:

```toml
[dependencies]
  [dependencies.transponders]
  agent = ["github"]
```

---

## Guest Language Guide

The guest ABI (`read_input`, `write_output`, `run_command`) is language-agnostic. Any language that compiles to WASM and can import host functions can implement an integration. Language choice matters most for binary size, standard library availability, and how much boilerplate you need to handle linear memory.

### Rust — recommended for new integrations

Rust has the best production-grade WASM story of any systems language. The `wasm32-unknown-unknown` target works out of the box with `cargo`:

```toml
# Cargo.toml
[lib]
crate-type = ["cdylib"]
```

```bash
cargo build --target wasm32-unknown-unknown --release
# output: target/wasm32-unknown-unknown/release/my_integration.wasm
```

Rust's WASM output is typically 50–200 KB stripped, compared to TinyGo's ~100 KB. No standard library surprises — you can use `serde_json` freely since Rust's WASM targets have a stable, well-tested allocator. `wasm-opt` (from Binaryen) can shrink binaries further.

Declaring host imports in Rust:

```rust
extern "C" {
    fn read_input(ptr: u32, max: u32) -> u32;
    fn write_output(ptr: u32, len: u32);
    fn run_command(spec_ptr: u32, spec_len: u32, out_ptr: u32, max: u32) -> u32;
}
```

Exporting handlers:

```rust
#[no_mangle]
pub extern "C" fn detect() { ... }

#[no_mangle]
pub extern "C" fn initialize() { ... }
```

### AssemblyScript — easiest for TypeScript developers

AssemblyScript compiles TypeScript-like syntax directly to WASM. Binary sizes are small (~20–60 KB) and memory management is straightforward. JSON support is available via `assemblyscript-json`.

### C / C++ via Emscripten or wasi-sdk

Mature toolchains. `wasi-sdk` with `--target=wasm32-unknown-unknown` gives you a C standard library and predictable memory behavior. Good option if the integration wraps an existing C library.

### Zig

First-class `wasm32-freestanding` support, tiny binaries, no hidden runtime surprises. Zig's `comptime` makes it easy to generate the ABI glue without macros. A reasonable choice if you already know Zig.

### Why TinyGo for the bundled Go integration

TinyGo was chosen for the `runtime/go` integration because the codebase is already Go and co-locating the guest with the host minimizes context switching. That said, two concessions were made:

- **`encoding/json` is unusable.** TinyGo's `wasm-unknown` target crashes inside `json.Marshal` because `sync.Map` (used by the type encoder cache) performs out-of-bounds memory accesses under this target's runtime. All JSON is hand-built using `[]byte` append.
- **`strings.Builder` is unusable.** `Builder.copyCheck()` stores `unsafe.Pointer(self)` and compares it on each write. Under `wasm-unknown`, this comparison traps with `unreachable`. All string construction uses `[]byte` append instead.

If a new integration needs richer standard library support (complex JSON parsing, HTTP clients, etc.) and TinyGo's constraints are prohibitive, **Rust is the better choice** — its WASM targets have none of these restrictions.

### Language comparison

| Language | Binary size | JSON | Std library in WASM | Notes |
| --- | --- | --- | --- | --- |
| Rust | 50–200 KB | `serde_json` works | Excellent | Recommended for new integrations |
| AssemblyScript | 20–60 KB | Via library | Limited | Good for simple detection logic |
| TinyGo (`wasm-unknown`) | ~100 KB | Manual only | Partial (see above) | Used for bundled Go integration |
| C / wasi-sdk | 30–150 KB | Via library | Good | Best when wrapping a C library |
| Zig | 20–100 KB | Manual or via pkg | Good | Minimal runtime surprises |
| Standard Go (`wasip1`) | 3–10 MB | Works | Full | Binary too large; `_start`→`proc_exit` prevents Lambda model |

---

## Host Runtime (`internal/integrations/wasm/`)

The host side is two files:

**`host.go`** — wazero runtime factory + three host functions (`read_input`, `write_output`, `run_command`). Call state is threaded through `context.WithValue` so host functions can access per-call input/output without any shared mutable state on the struct.

**`loader.go`** — `WASMIntegration`: loads a WASM binary once, holds the live module, implements `integrations.Integration`. The `invoke` method sets up `callState`, calls the exported function, and returns the output bytes. Marshaling to/from the `integrations` types (`DetectContext`, `StateReport`, etc.) happens here using standard `encoding/json` — only the guest is restricted.

---

## Phase Roadmap

| Phase | Status | Scope |
| --- | --- | --- |
| 2.5 | Done | wazero host, TinyGo guest, `runtime/go`, `--discover` flag |
| 3 | Next | Full lifecycle commands: `jump`, `scan`, `survey`, `chart`, `calibrate`, `retro` — binary rename (`orbit` → `orbiter`), Executor shared pipeline, CWD resolution, Beacon state writes, shell integration |
| 3.5 | Planned | `remote/github` integration (Rust WASM) — first integration required to exercise the full lifecycle; empirical test of the Rust guest ABI |
| 4 | Planned | Integration hardening: instance pooling, manifest auto-discovery via `go:generate`, 64 KB payload enforcement, TOML manifest parsing, runtime plugin directories, multi-language testing and documentation |
| 5 | Planned | TUI: `orbiter starchart` universe and beacon views, Bubble Tea progress display for `orbiter jump` — polish layer over the completed CLI |
