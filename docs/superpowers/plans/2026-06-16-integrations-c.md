# C/wasi-sdk Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `tool/vscode` written in C using wasi-sdk, demonstrating the C guest ABI as a reference path for systems-background integration authors.

**Architecture:** The C integration targets `wasm32-unknown-unknown` compiled with wasi-sdk's `clang`. With `-nostdlib`, standard C library functions are unavailable; all logic uses the three host ABI functions. This is the canonical showcase for C: host imports as `__attribute__((import_module, import_name))` declarations, handler exports via `__attribute__((visibility("default")))`, manual memory management in 64 KB stack buffers.

**Tech Stack:** wasi-sdk (provides `clang` with wasm target support), `wasm32-unknown-unknown` target, `-nostdlib`, `go test ./integrations/...`

## Global Constraints

- Target: `wasm32-unknown-unknown` via wasi-sdk clang
- `-nostdlib` — no C standard library
- All handlers exported via `__attribute__((visibility("default")))`
- Host imports declared via `__attribute__((import_module("orbiter"), import_name("<fn>")))`
- `WASI_SDK_PATH` environment variable must point to the wasi-sdk installation
- `integrations/bundle.go` `//go:embed` must be updated
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`
- Run `go test ./integrations/...` after completing this task

---

### Task 1: tool/vscode

**Files:**
- Create: `integrations/vscode/manifest.toml`
- Create: `integrations/vscode/generate.go`
- Create: `integrations/vscode/src/vscode.c`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_VSCode(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "vscode")
	if !ok {
		t.Fatal("vscode integration not registered")
	}

	t.Run("detect_vscode_dir", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".vscode/settings.json": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .vscode/ directory")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "tool" {
			t.Error("expected role=tool suggestion")
		}
		if report.Resources[0].Brand != "vscode" {
			t.Errorf("expected brand=vscode, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_vscode_launch", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".vscode/launch.json": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .vscode/launch.json")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .vscode/")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_VSCode -v
```

Expected: FAIL — "vscode integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/vscode/src
```

- [ ] **Step 4: Write src/vscode.c**

Create `integrations/vscode/src/vscode.c`:

```c
/*
 * VSCode integration — C/wasi-sdk guest
 *
 * Compiled with wasi-sdk clang targeting wasm32-unknown-unknown.
 * No C standard library (-nostdlib). All I/O via the three host functions.
 *
 * Host import syntax uses clang WebAssembly attributes:
 *   __attribute__((import_module("orbiter"), import_name("fn_name")))
 *
 * Export syntax for handler functions:
 *   __attribute__((visibility("default")))
 *
 * Memory: all buffers are stack-allocated. 64 KB per buffer is sufficient
 * for the JSON payloads this integration produces.
 */

/* ── Types ─────────────────────────────────────────────────────────────────── */
typedef unsigned int   uint32_t;
typedef unsigned char  uint8_t;
typedef int            int32_t;
typedef unsigned long  size_t;

/* ── Host ABI ─────────────────────────────────────────────────────────────── */

__attribute__((import_module("orbiter"), import_name("read_input")))
extern uint32_t host_read_input(uint8_t *ptr, uint32_t max);

__attribute__((import_module("orbiter"), import_name("write_output")))
extern void host_write_output(const uint8_t *ptr, uint32_t len);

__attribute__((import_module("orbiter"), import_name("run_command")))
extern uint32_t host_run_command(const uint8_t *spec_ptr, uint32_t spec_len,
                                  uint8_t *out_ptr, uint32_t out_max);

/* ── Minimal string helpers (no libc) ─────────────────────────────────────── */

static size_t cstrlen(const char *s) {
    size_t n = 0;
    while (s[n]) n++;
    return n;
}

/* Returns 1 if haystack contains needle, 0 otherwise. */
static int contains(const uint8_t *haystack, uint32_t hlen, const char *needle) {
    uint32_t nlen = (uint32_t)cstrlen(needle);
    if (nlen > hlen) return 0;
    for (uint32_t i = 0; i + nlen <= hlen; i++) {
        uint32_t match = 1;
        for (uint32_t j = 0; j < nlen; j++) {
            if (haystack[i + j] != (uint8_t)needle[j]) { match = 0; break; }
        }
        if (match) return 1;
    }
    return 0;
}

/* Append a C string to buf at offset *pos. Returns new offset. */
static uint32_t append(uint8_t *buf, uint32_t pos, const char *s) {
    uint32_t n = (uint32_t)cstrlen(s);
    for (uint32_t i = 0; i < n; i++) buf[pos + i] = (uint8_t)s[i];
    return pos + n;
}

/* ── Shared helpers ────────────────────────────────────────────────────────── */

#define BUF_SIZE 65536

/*
 * run_cmd builds the JSON command spec and calls host_run_command.
 * cmd and arg are C strings. out must be BUF_SIZE bytes.
 * Returns the number of output bytes written (trimmed).
 */
static uint32_t run_cmd(const char *cmd, const char *arg,
                         uint8_t *spec_buf, uint8_t *out_buf) {
    uint32_t pos = 0;
    pos = append(spec_buf, pos, "{\"cmd\":\"");
    pos = append(spec_buf, pos, cmd);
    if (arg != 0) {
        pos = append(spec_buf, pos, "\",\"args\":[\"");
        pos = append(spec_buf, pos, arg);
        pos = append(spec_buf, pos, "\"]}");
    } else {
        pos = append(spec_buf, pos, "\",\"args\":[]}");
    }
    uint32_t n = host_run_command(spec_buf, pos, out_buf, BUF_SIZE);
    /* Trim trailing newline/CR/space */
    while (n > 0 && (out_buf[n-1] == '\n' || out_buf[n-1] == '\r' || out_buf[n-1] == ' ')) {
        n--;
    }
    return n;
}

/*
 * write_state writes a StateReport JSON object.
 * version_out / version_len: output of a version command (may be 0/0).
 * error_str: error message string or NULL.
 * present/reachable/in_path: booleans.
 */
static void write_state(int present, int reachable, int in_path,
                         const char *binary_path,
                         const uint8_t *version_out, uint32_t version_len,
                         const char *error_str) {
    uint8_t out[BUF_SIZE];
    uint32_t pos = 0;
    pos = append(out, pos, "{\"present\":");
    pos = append(out, pos, present ? "true" : "false");
    pos = append(out, pos, ",\"reachable\":");
    pos = append(out, pos, reachable ? "true" : "false");
    pos = append(out, pos, ",\"in_path\":");
    pos = append(out, pos, in_path ? "true" : "false");
    pos = append(out, pos, ",\"manager\":\"system\"");
    if (binary_path && cstrlen(binary_path) > 0) {
        pos = append(out, pos, ",\"binary_path\":\"");
        pos = append(out, pos, binary_path);
        pos = append(out, pos, "\"");
    }
    if (error_str && cstrlen(error_str) > 0) {
        pos = append(out, pos, ",\"error\":\"");
        pos = append(out, pos, error_str);
        pos = append(out, pos, "\"");
    }
    if (version_out && version_len > 0) {
        pos = append(out, pos, ",\"observations\":[\"");
        for (uint32_t i = 0; i < version_len && pos < BUF_SIZE - 3; i++) {
            uint8_t c = version_out[i];
            if (c == '"') {
                out[pos++] = '\\';
                out[pos++] = '"';
            } else if (c == '\\') {
                out[pos++] = '\\';
                out[pos++] = '\\';
            } else {
                out[pos++] = c;
            }
        }
        pos = append(out, pos, "\"]");
    }
    out[pos++] = '}';
    host_write_output(out, pos);
}

/* ── Handlers ─────────────────────────────────────────────────────────────── */

__attribute__((visibility("default")))
void detect(void) {
    uint8_t input[BUF_SIZE];
    uint32_t n = host_read_input(input, BUF_SIZE);

    /* Detect .vscode/ directory by looking for keys like ".vscode/" in the
     * files map. The detection context contains a JSON "files" object whose
     * keys are relative paths. Presence of any key starting with ".vscode/"
     * means the project uses VSCode. */
    int has_vscode = contains(input, n, "\".vscode/")
                  || contains(input, n, "\".vscode\\\\"); /* Windows path escape */

    if (!has_vscode) {
        host_write_output((const uint8_t*)"{\"detected\":false}", 18);
        return;
    }
    host_write_output(
        (const uint8_t*)"{\"detected\":true,\"resources\":[{\"role\":\"tool\",\"brand\":\"vscode\"}]}",
        63
    );
}

__attribute__((visibility("default")))
void initialize(void) {
    uint8_t input[BUF_SIZE];
    host_read_input(input, BUF_SIZE);

    uint8_t spec_buf[BUF_SIZE];
    uint8_t which_out[BUF_SIZE];
    uint32_t which_n = run_cmd("which", "code", spec_buf, which_out);

    if (which_n == 0) {
        write_state(0, 0, 0, 0, 0, 0, "code binary not found in PATH");
        return;
    }

    /* Null-terminate for use as binary_path string */
    which_out[which_n] = '\0';

    uint8_t ver_spec[BUF_SIZE];
    uint8_t ver_out[BUF_SIZE];
    uint32_t ver_n = run_cmd("code", "--version", ver_spec, ver_out);

    write_state(1, 1, 1, (const char*)which_out, ver_out, ver_n, 0);
}

__attribute__((visibility("default")))
void scan(void) {
    initialize();
}

__attribute__((visibility("default")))
void calibrate(void) {
    uint8_t input[BUF_SIZE];
    host_read_input(input, BUF_SIZE);

    uint8_t spec_buf[BUF_SIZE];
    uint8_t ver_out[BUF_SIZE];
    uint32_t ver_n = run_cmd("code", "--version", spec_buf, ver_out);

    if (ver_n == 0) {
        write_state(0, 0, 0, 0, 0, 0, "code not found");
        return;
    }

    /* Prepend "calibrated: " to the version output */
    uint8_t calib_out[BUF_SIZE];
    uint32_t pos = 0;
    const char *prefix = "calibrated: ";
    while (prefix[pos]) { calib_out[pos] = (uint8_t)prefix[pos]; pos++; }
    for (uint32_t i = 0; i < ver_n && pos < BUF_SIZE - 1; i++) {
        calib_out[pos++] = ver_out[i];
    }

    write_state(1, 1, 1, 0, calib_out, pos, 0);
}
```

- [ ] **Step 5: Create generate.go**

Create `integrations/vscode/generate.go`:

```go
package vscode

//go:generate sh -c "${WASI_SDK_PATH}/bin/clang --target=wasm32-unknown-unknown -O2 -nostdlib -Wl,--no-entry -Wl,--export=detect -Wl,--export=initialize -Wl,--export=scan -Wl,--export=calibrate -o vscode.wasm src/vscode.c"
```

If `WASI_SDK_PATH` is not set, install wasi-sdk from https://github.com/WebAssembly/wasi-sdk/releases and set the environment variable:

```bash
export WASI_SDK_PATH=/path/to/wasi-sdk-<version>
```

- [ ] **Step 6: Create manifest.toml**

Create `integrations/vscode/manifest.toml`:

```toml
[integration]
brand = "vscode"
name = "Visual Studio Code"
description = "Detects and verifies the Visual Studio Code editor"
roles = ["tool"]

[detection]
files = [".vscode/settings.json", ".vscode/launch.json", ".vscode/extensions.json"]

[commands]
allowed = ["code", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 7: Update bundle.go embed**

Add `vscode/vscode.wasm vscode/manifest.toml` to the `//go:embed` directive in `integrations/bundle.go`.

- [ ] **Step 8: Compile**

```bash
cd integrations/vscode && go generate .
```

Expected: `vscode.wasm` created in `integrations/vscode/`. The wasm-ld linker step may warn about no `_start` entry point — this is expected with `--no-entry`.

If `WASI_SDK_PATH` is not set or wasi-sdk is not installed:

```bash
# Install wasi-sdk
curl -L https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-25/wasi-sdk-25.0-macos.tar.gz | tar xz
export WASI_SDK_PATH=$(pwd)/wasi-sdk-25.0
cd integrations/vscode && go generate .
```

- [ ] **Step 9: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_VSCode -v
```

Expected: detect_vscode_dir, detect_vscode_launch, detect_miss, and scan subtests pass.

- [ ] **Step 10: Run full suite**

```bash
go test ./integrations/... -v
```

Expected: all existing tests still pass.

- [ ] **Step 11: Commit**

```bash
git add integrations/vscode/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add tool/vscode C/wasi-sdk integration"
```
