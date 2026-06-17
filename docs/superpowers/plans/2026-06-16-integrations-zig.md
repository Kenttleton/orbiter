# Zig Integrations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `manager/asdf` and `filesystem/local` written in Zig, demonstrating the `wasm32-freestanding` guest target with `std.json`.

**Architecture:** Zig's `wasm32-freestanding` target produces minimal WASM with no hidden runtime. `std.json` handles all JSON. The `filesystem/local` integration returns `InstallDir` in its `StateReport` from `calibrate`, which causes the host's `Jump` command to automatically emit a `cd <path>` directive — no new host mechanism is required.

> **ORBITER_CWD clarification:** The spec references an `ORBITER_CWD` host-reserved export key pattern. This was written before discovering that `internal/commands/executor.go` (lines 357–371) already implements `cd` emission for filesystem resources via `StateReport.InstallDir`. The `filesystem/local` implementation in this plan uses `InstallDir` — not `ORBITER_CWD` — matching the existing host behavior. No changes to `executor.go` or `internal/wasm/loader.go` are needed.

**Tech Stack:** Zig 0.14+, `wasm32-freestanding` target, `std.json`, `go test ./integrations/...`

## Global Constraints

- Target: `wasm32-freestanding` (NOT wasm32-wasi)
- All Zig integrations export `detect`, `initialize`, `scan`, `calibrate` via `export fn`
- Host ABI imported via `extern "orbiter"` function declarations
- Memory: use `std.heap.FixedBufferAllocator` with a fixed 128 KB stack buffer — no page allocator needed for these integrations
- `integrations/bundle.go` `//go:embed` must be updated for each integration
- Tests added to `integrations/e2e_test.go`
- Module path: `github.com/Kenttleton/orbiter`
- Run `go test ./integrations/...` after each integration

---

## Common Zig Pattern

Every Zig integration uses this file layout:

```
integrations/<brand>/
├── build.zig
├── generate.go
├── manifest.toml
└── src/
    └── main.zig
```

### Host ABI (identical for all Zig integrations, include inline in each main.zig)

```zig
// Host function imports — identical for all Zig integrations
extern "orbiter" fn read_input(ptr: [*]u8, max: u32) u32;
extern "orbiter" fn write_output(ptr: [*]const u8, len: u32) void;
extern "orbiter" fn run_command(spec_ptr: [*]const u8, spec_len: u32, out_ptr: [*]u8, out_max: u32) u32;

const BUF_SIZE: usize = 65536;

fn readInput(buf: []u8) []u8 {
    const n = read_input(buf.ptr, @intCast(buf.len));
    return buf[0..n];
}

fn writeOutput(data: []const u8) void {
    write_output(data.ptr, @intCast(data.len));
}

// runCmd runs cmd with args. Returns the trimmed output string.
// spec_buf is used to build the command JSON. out_buf receives the output.
fn runCmd(
    allocator: std.mem.Allocator,
    cmd: []const u8,
    args: []const []const u8,
    spec_buf: []u8,
    out_buf: []u8,
) []u8 {
    var fbs = std.io.fixedBufferStream(spec_buf);
    const writer = fbs.writer();
    std.json.stringify(.{ .cmd = cmd, .args = args }, .{}, writer) catch {
        return out_buf[0..0];
    };
    const spec = fbs.getWritten();
    const n = run_command(spec.ptr, @intCast(spec.len), out_buf.ptr, @intCast(out_buf.len));
    const raw = out_buf[0..n];
    // Trim trailing whitespace
    var end = raw.len;
    while (end > 0 and (raw[end - 1] == '\n' or raw[end - 1] == '\r' or raw[end - 1] == ' ')) {
        end -= 1;
    }
    return raw[0..end];
}
```

### build.zig (per-integration — only brand name changes)

```zig
const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.resolveTargetQuery(.{
        .cpu_arch = .wasm32,
        .os_tag = .freestanding,
    });
    const optimize = b.standardOptimizeOption(.{});
    const lib = b.addSharedLibrary(.{
        .name = "<brand>",
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });
    b.installArtifact(lib);
}
```

Output: `zig-out/lib/<brand>.wasm`

### generate.go (per-integration)

```go
package <brand>

//go:generate sh -c "zig build -Doptimize=ReleaseSmall && cp zig-out/lib/<brand>.wasm ."
```

---

### Task 1: manager/asdf

**Files:**
- Create: `integrations/asdf/build.zig`
- Create: `integrations/asdf/generate.go`
- Create: `integrations/asdf/manifest.toml`
- Create: `integrations/asdf/src/main.zig`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_Asdf(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "asdf")
	if !ok {
		t.Fatal("asdf integration not registered")
	}

	t.Run("detect_tool_versions", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".tool-versions": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .tool-versions")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "manager" {
			t.Error("expected role=manager suggestion")
		}
		if report.Resources[0].Brand != "asdf" {
			t.Errorf("expected brand=asdf, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .tool-versions")
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
go test ./integrations/... -run TestBundledIntegrations_Asdf -v
```

Expected: FAIL — "asdf integration not registered"

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p integrations/asdf/src
```

- [ ] **Step 4: Create build.zig**

Create `integrations/asdf/build.zig`:

```zig
const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.resolveTargetQuery(.{
        .cpu_arch = .wasm32,
        .os_tag = .freestanding,
    });
    const optimize = b.standardOptimizeOption(.{});
    const lib = b.addSharedLibrary(.{
        .name = "asdf",
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });
    b.installArtifact(lib);
}
```

- [ ] **Step 5: Write src/main.zig**

Create `integrations/asdf/src/main.zig`:

```zig
const std = @import("std");

// ── Host ABI ──────────────────────────────────────────────────────────────────

extern "orbiter" fn read_input(ptr: [*]u8, max: u32) u32;
extern "orbiter" fn write_output(ptr: [*]const u8, len: u32) void;
extern "orbiter" fn run_command(spec_ptr: [*]const u8, spec_len: u32, out_ptr: [*]u8, out_max: u32) u32;

const BUF_SIZE: usize = 65536;

fn readInput(buf: []u8) []u8 {
    const n = read_input(buf.ptr, @intCast(buf.len));
    return buf[0..n];
}

fn writeOutput(data: []const u8) void {
    write_output(data.ptr, @intCast(data.len));
}

fn runCmd(
    cmd: []const u8,
    args: []const []const u8,
    spec_buf: []u8,
    out_buf: []u8,
) []u8 {
    var fbs = std.io.fixedBufferStream(spec_buf);
    std.json.stringify(.{ .cmd = cmd, .args = args }, .{}, fbs.writer()) catch return out_buf[0..0];
    const spec = fbs.getWritten();
    const n = run_command(spec.ptr, @intCast(spec.len), out_buf.ptr, @intCast(out_buf.len));
    const raw = out_buf[0..n];
    var end = raw.len;
    while (end > 0 and (raw[end - 1] == '\n' or raw[end - 1] == '\r' or raw[end - 1] == ' ')) end -= 1;
    return raw[0..end];
}

// ── Shared state report builder ────────────────────────────────────────────────

const StateReport = struct {
    present: bool,
    reachable: bool,
    binary_path: ?[]const u8 = null,
    in_path: bool,
    manager: []const u8,
    @"error": []const u8 = "",
    observations: []const []const u8 = &[_][]const u8{},
};

fn writeState(report: StateReport, out_buf: []u8) void {
    var fbs = std.io.fixedBufferStream(out_buf);
    std.json.stringify(report, .{ .emit_null_optional_fields = false }, fbs.writer()) catch {
        writeOutput("{\"present\":false,\"error\":\"json serialize failed\"}");
        return;
    };
    writeOutput(fbs.getWritten());
}

// ── Handlers ──────────────────────────────────────────────────────────────────

var stack_mem: [256 * 1024]u8 = undefined;

export fn detect() void {
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    var in_buf: [BUF_SIZE]u8 = undefined;
    const input = readInput(&in_buf);

    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        writeOutput("{\"detected\":false}");
        return;
    };
    defer parsed.deinit();

    const files = parsed.value.object.get("files") orelse {
        writeOutput("{\"detected\":false}");
        return;
    };
    const has_tool_versions = files.object.contains(".tool-versions");
    if (!has_tool_versions) {
        writeOutput("{\"detected\":false}");
        return;
    }

    const result = .{
        .detected = true,
        .resources = [_]struct {
            role: []const u8,
            brand: []const u8,
        }{.{ .role = "manager", .brand = "asdf" }},
    };
    var out_buf: [BUF_SIZE]u8 = undefined;
    var fbs = std.io.fixedBufferStream(&out_buf);
    std.json.stringify(result, .{}, fbs.writer()) catch {
        writeOutput("{\"detected\":true}");
        return;
    };
    writeOutput(fbs.getWritten());
}

export fn initialize() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    _ = readInput(&in_buf);

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var cmd_out: [BUF_SIZE]u8 = undefined;

    const which_args = [_][]const u8{"asdf"};
    const binary_path = runCmd("which", &which_args, &spec_buf, &cmd_out);

    if (binary_path.len == 0) {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(.{
            .present = false,
            .reachable = false,
            .in_path = false,
            .manager = "system",
            .@"error" = "asdf not found in PATH",
        }, &out_buf);
        return;
    }

    var ver_spec: [BUF_SIZE]u8 = undefined;
    var ver_out: [BUF_SIZE]u8 = undefined;
    const ver_args = [_][]const u8{"version"};
    const version = runCmd("asdf", &ver_args, &ver_spec, &ver_out);

    const obs = [_][]const u8{version};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(.{
        .present = true,
        .reachable = true,
        .binary_path = binary_path,
        .in_path = true,
        .manager = "system",
        .observations = &obs,
    }, &out_buf);
}

export fn scan() void {
    initialize();
}

export fn calibrate() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    _ = readInput(&in_buf);

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var ver_out: [BUF_SIZE]u8 = undefined;
    const ver_args = [_][]const u8{"version"};
    const version = runCmd("asdf", &ver_args, &spec_buf, &ver_out);

    if (version.len == 0) {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(.{
            .present = false,
            .reachable = false,
            .in_path = false,
            .manager = "system",
            .@"error" = "asdf not found",
        }, &out_buf);
        return;
    }

    // Build "calibrated: <version>" in a fixed buffer
    var calib_msg: [128]u8 = undefined;
    const msg = std.fmt.bufPrint(&calib_msg, "calibrated: {s}", .{version}) catch version;
    const obs = [_][]const u8{msg};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(.{
        .present = true,
        .reachable = true,
        .in_path = true,
        .manager = "system",
        .observations = &obs,
    }, &out_buf);
}
```

- [ ] **Step 6: Create generate.go**

Create `integrations/asdf/generate.go`:

```go
package asdf

//go:generate sh -c "zig build -Doptimize=ReleaseSmall && cp zig-out/lib/asdf.wasm ."
```

- [ ] **Step 7: Create manifest.toml**

Create `integrations/asdf/manifest.toml`:

```toml
[integration]
brand = "asdf"
name = "asdf"
description = "Scans and verifies the asdf version manager"
roles = ["manager"]

[detection]
files = [".tool-versions"]

[commands]
allowed = ["asdf", "which"]
timeout_seconds = 30

[runtime]
pool_size = 4
input_buffer_kb = 8
output_buffer_kb = 8
```

- [ ] **Step 8: Update bundle.go embed**

Add `asdf/asdf.wasm asdf/manifest.toml` to the `//go:embed` directive in `integrations/bundle.go`.

- [ ] **Step 9: Compile**

```bash
cd integrations/asdf && go generate .
```

Expected: `asdf.wasm` created in `integrations/asdf/`.

- [ ] **Step 10: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_Asdf -v
```

Expected: detect_tool_versions, detect_miss, scan subtests all pass.

- [ ] **Step 11: Commit**

```bash
git add integrations/asdf/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add manager/asdf Zig integration"
```

---

### Task 2: filesystem/local

**Files:**
- Create: `integrations/local/build.zig`
- Create: `integrations/local/generate.go`
- Create: `integrations/local/manifest.toml`
- Create: `integrations/local/src/main.zig`
- Modify: `integrations/bundle.go`
- Modify: `integrations/e2e_test.go`

**How `cd` works:** When `calibrate` returns a `StateReport` with a non-empty `install_dir`, the host's `Jump` command (in `internal/commands/executor.go`, lines 357–371) automatically emits `cd <path>` to the shell eval output and records the directive. The `filesystem/local` integration returns `install_dir` from its `calibrate` handler — no special export key or manifest declaration is needed beyond a `filesystem` role.

The native `filesystem/orbiter` integration (pure Go, in `internal/integrations/native/filesystem.go`) handles the default case. When captains install `filesystem/local`, it overrides the native integration. The Zig integration intentionally mirrors the same `StateReport` shape so the host handles it identically.

- [ ] **Step 1: Write failing test**

Add to `integrations/e2e_test.go`:

```go
func TestBundledIntegrations_FilesystemLocal(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("filesystem", "local")
	if !ok {
		t.Fatal("local filesystem integration not registered")
	}

	t.Run("detect_always", func(t *testing.T) {
		// filesystem/local is always detected — it overrides the native filesystem
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for filesystem/local (always active)")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "filesystem" {
			t.Error("expected role=filesystem suggestion")
		}
	})

	t.Run("scan_cwd", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (CWD always exists)")
		}
		if report.InstallDir == "" {
			t.Error("expected non-empty install_dir (CWD path)")
		}
	})

	t.Run("calibrate_sets_install_dir", func(t *testing.T) {
		report := i.Calibrate(core.ResolvedContext{})
		t.Logf("Calibrate: %+v", report)
		if !report.Present {
			t.Error("expected present=true after calibrate")
		}
		if report.InstallDir == "" {
			t.Error("expected install_dir to be set by calibrate (triggers cd in Jump)")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./integrations/... -run TestBundledIntegrations_FilesystemLocal -v
```

Expected: FAIL — "local filesystem integration not registered"

- [ ] **Step 3: Verify InstallDir field exists in StateReport**

```bash
grep -r "InstallDir\|install_dir" internal/integrations/
```

Expected: find `InstallDir` in the StateReport struct definition. If it does not exist, look at `internal/integrations/state.go` or similar for the struct and add the field before proceeding with this task.

Verify how executor.go reads it:

```bash
grep -A 15 "ResourceRoleFilesystem" internal/commands/executor.go
```

Expected: confirm lines like:
```go
if r.After.InstallDir != "" {
    directives = append(directives, ShellDirective{Op: "cd", Value: r.After.InstallDir})
```

This confirms the host already handles `cd` from `InstallDir` — no host changes are needed.

- [ ] **Step 4: Create directory structure**

```bash
mkdir -p integrations/local/src
```

- [ ] **Step 5: Create build.zig**

Create `integrations/local/build.zig`:

```zig
const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.resolveTargetQuery(.{
        .cpu_arch = .wasm32,
        .os_tag = .freestanding,
    });
    const optimize = b.standardOptimizeOption(.{});
    const lib = b.addSharedLibrary(.{
        .name = "local",
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });
    b.installArtifact(lib);
}
```

- [ ] **Step 6: Write src/main.zig**

Create `integrations/local/src/main.zig`:

```zig
const std = @import("std");

// ── Host ABI ──────────────────────────────────────────────────────────────────

extern "orbiter" fn read_input(ptr: [*]u8, max: u32) u32;
extern "orbiter" fn write_output(ptr: [*]const u8, len: u32) void;
extern "orbiter" fn run_command(spec_ptr: [*]const u8, spec_len: u32, out_ptr: [*]u8, out_max: u32) u32;

const BUF_SIZE: usize = 65536;

fn readInput(buf: []u8) []u8 {
    const n = read_input(buf.ptr, @intCast(buf.len));
    return buf[0..n];
}

fn writeOutput(data: []const u8) void {
    write_output(data.ptr, @intCast(data.len));
}

fn runCmd(
    cmd: []const u8,
    args: []const []const u8,
    spec_buf: []u8,
    out_buf: []u8,
) []u8 {
    var fbs = std.io.fixedBufferStream(spec_buf);
    std.json.stringify(.{ .cmd = cmd, .args = args }, .{}, fbs.writer()) catch return out_buf[0..0];
    const spec = fbs.getWritten();
    const n = run_command(spec.ptr, @intCast(spec.len), out_buf.ptr, @intCast(out_buf.len));
    const raw = out_buf[0..n];
    var end = raw.len;
    while (end > 0 and (raw[end - 1] == '\n' or raw[end - 1] == '\r' or raw[end - 1] == ' ')) end -= 1;
    return raw[0..end];
}

// StateReport matches the host's expected JSON shape for filesystem resources.
// install_dir is the key field: when set, executor.go emits "cd <install_dir>"
// during the Jump lifecycle.
const StateReport = struct {
    present: bool,
    reachable: bool,
    in_path: bool,
    manager: []const u8,
    install_dir: []const u8 = "",
    @"error": []const u8 = "",
    observations: []const []const u8 = &[_][]const u8{},
};

fn writeState(report: StateReport, out_buf: []u8) void {
    var fbs = std.io.fixedBufferStream(out_buf);
    std.json.stringify(report, .{ .emit_null_optional_fields = false }, fbs.writer()) catch {
        writeOutput("{\"present\":false,\"error\":\"json serialize failed\"}");
        return;
    };
    writeOutput(fbs.getWritten());
}

var stack_mem: [256 * 1024]u8 = undefined;

// filesystem/local is always detected — it provides the working directory
// for every project and overrides the native filesystem/orbiter.
export fn detect() void {
    const result = .{
        .detected = true,
        .resources = [_]struct {
            role: []const u8,
            brand: []const u8,
        }{.{ .role = "filesystem", .brand = "local" }},
    };
    var out_buf: [BUF_SIZE]u8 = undefined;
    var fbs = std.io.fixedBufferStream(&out_buf);
    std.json.stringify(result, .{}, fbs.writer()) catch {
        writeOutput("{\"detected\":true}");
        return;
    };
    writeOutput(fbs.getWritten());
}

// initialize and scan: stat the CWD and report it as install_dir.
// The host already knows the CWD from the ResolvedContext, but returning it
// here as install_dir is what triggers the cd directive in Jump.
export fn initialize() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    const input = readInput(&in_buf);
    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(.{ .present = false, .reachable = false, .in_path = false, .manager = "local", .@"error" = "parse error" }, &out_buf);
        return;
    };
    defer parsed.deinit();

    // Read install_dir from context if set; fall back to "." (current directory)
    var install_dir: []const u8 = ".";
    if (parsed.value.object.get("install_dir")) |dir| {
        if (dir == .string and dir.string.len > 0) {
            install_dir = dir.string;
        }
    }

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var stat_out: [BUF_SIZE]u8 = undefined;
    const stat_args = [_][]const u8{ "-c", "%F", install_dir };
    const stat_result = runCmd("stat", &stat_args, &spec_buf, &stat_out);
    const exists = stat_result.len > 0 and !std.mem.startsWith(u8, stat_result, "stat:");

    const obs_msg = if (exists) "directory exists" else "directory not found";
    const obs = [_][]const u8{obs_msg};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(.{
        .present = exists,
        .reachable = exists,
        .in_path = false,
        .manager = "local",
        .install_dir = install_dir,
        .observations = &obs,
    }, &out_buf);
}

export fn scan() void {
    initialize();
}

// calibrate: create the directory if it doesn't exist, then set install_dir.
// The non-empty install_dir in the returned StateReport causes executor.go
// to emit "cd <install_dir>" during the Jump lifecycle.
export fn calibrate() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    const input = readInput(&in_buf);
    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(.{ .present = false, .reachable = false, .in_path = false, .manager = "local", .@"error" = "parse error" }, &out_buf);
        return;
    };
    defer parsed.deinit();

    var install_dir: []const u8 = ".";
    if (parsed.value.object.get("install_dir")) |dir| {
        if (dir == .string and dir.string.len > 0) {
            install_dir = dir.string;
        }
    }

    // Check if directory exists
    var spec_buf: [BUF_SIZE]u8 = undefined;
    var stat_out: [BUF_SIZE]u8 = undefined;
    const stat_args = [_][]const u8{ "-c", "%F", install_dir };
    const stat_result = runCmd("stat", &stat_args, &spec_buf, &stat_out);
    const exists = stat_result.len > 0 and !std.mem.startsWith(u8, stat_result, "stat:");

    if (!exists) {
        // Create the directory
        var mkdir_spec: [BUF_SIZE]u8 = undefined;
        var mkdir_out: [BUF_SIZE]u8 = undefined;
        const mkdir_args = [_][]const u8{ "-p", install_dir };
        _ = runCmd("mkdir", &mkdir_args, &mkdir_spec, &mkdir_out);
    }

    // Return install_dir — this is what triggers cd in the host's Jump command
    var msg_buf: [256]u8 = undefined;
    const msg = std.fmt.bufPrint(&msg_buf, "calibrated: {s}", .{install_dir}) catch install_dir;
    const obs = [_][]const u8{msg};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(.{
        .present = true,
        .reachable = true,
        .in_path = false,
        .manager = "local",
        .install_dir = install_dir,
        .observations = &obs,
    }, &out_buf);
}
```

- [ ] **Step 7: Create generate.go**

Create `integrations/local/generate.go`:

```go
package local

//go:generate sh -c "zig build -Doptimize=ReleaseSmall && cp zig-out/lib/local.wasm ."
```

- [ ] **Step 8: Create manifest.toml**

Create `integrations/local/manifest.toml`:

```toml
[integration]
brand = "local"
name = "Local Filesystem"
description = "Manages local directory state and working directory for the jump lifecycle"
roles = ["filesystem"]

[commands]
allowed = ["stat", "mkdir", "pwd"]
timeout_seconds = 15

[runtime]
pool_size = 2
input_buffer_kb = 8
output_buffer_kb = 8
```

Note: no `[shell] exports` section — `cd` is driven by `install_dir` in `StateReport`, not by an exported key. The host handles it natively without requiring an exports declaration.

- [ ] **Step 9: Update bundle.go embed**

Add `local/local.wasm local/manifest.toml` to the `//go:embed` directive.

- [ ] **Step 10: Compile**

```bash
cd integrations/local && go generate .
```

Expected: `local.wasm` created in `integrations/local/`.

- [ ] **Step 11: Run tests**

```bash
go test ./integrations/... -run TestBundledIntegrations_FilesystemLocal -v
```

Expected: detect_always, scan_cwd, and calibrate_sets_install_dir subtests pass.

Verify `install_dir` is set by calibrate — this is what triggers the `cd` in the host:

```bash
go test ./integrations/... -run TestBundledIntegrations_FilesystemLocal/calibrate_sets_install_dir -v
```

Expected: `install_dir != ""` assertion passes.

- [ ] **Step 12: Run full suite**

```bash
go test ./integrations/... -v
```

Expected: all tests pass.

- [ ] **Step 13: Commit**

```bash
git add integrations/local/ integrations/bundle.go integrations/e2e_test.go
git commit -m "feat: add filesystem/local Zig integration (cd via StateReport.InstallDir)"
```
