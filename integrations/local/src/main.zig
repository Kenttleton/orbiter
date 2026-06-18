const std = @import("std");

// ── Host ABI ──────────────────────────────────────────────────────────────────

extern "orbiter" fn read_input(ptr: [*]u8, max: u32) u32;
extern "orbiter" fn write_output(ptr: [*]const u8, len: u32) void;
extern "orbiter" fn run_command(spec_ptr: [*]const u8, spec_len: u32, out_ptr: [*]u8, out_max: u32) u32;

const BUF_SIZE: usize = 65536;

// Global memory for FixedBufferAllocator used in handlers.
var stack_mem: [256 * 1024]u8 = undefined;

// ── Helpers ───────────────────────────────────────────────────────────────────

/// Serialize val to JSON into buf. Returns the written slice.
fn writeJSON(val: anytype, buf: []u8) []u8 {
    var writer = std.Io.Writer.fixed(buf);
    std.json.Stringify.value(val, .{}, &writer) catch return buf[0..0];
    return std.Io.Writer.buffered(&writer);
}

/// Trim trailing whitespace from a slice.
fn trimRight(s: []u8) []u8 {
    var end = s.len;
    while (end > 0 and (s[end - 1] == '\n' or s[end - 1] == '\r' or s[end - 1] == ' ')) {
        end -= 1;
    }
    return s[0..end];
}

/// Run cmd with args; returns trimmed stdout (or empty slice on failure).
fn runCmd(cmd: []const u8, args: []const []const u8, spec_buf: []u8, out_buf: []u8) []u8 {
    const spec = writeJSON(.{ .cmd = cmd, .args = args }, spec_buf);
    if (spec.len == 0) return out_buf[0..0];
    const n = run_command(spec.ptr, @intCast(spec.len), out_buf.ptr, @intCast(out_buf.len));
    return trimRight(out_buf[0..n]);
}

/// Write a StateReport JSON object and emit it via write_output.
fn writeState(
    present: bool,
    reachable: bool,
    manager: []const u8,
    install_dir: []const u8,
    err: []const u8,
    observations: []const []const u8,
    out_buf: []u8,
) void {
    var writer = std.Io.Writer.fixed(out_buf);
    var s = std.json.Stringify{ .writer = &writer };
    s.beginObject() catch {
        write_output("{\"present\":false,\"error\":\"json failed\"}", 37);
        return;
    };
    s.objectField("present") catch return;
    s.write(present) catch return;
    s.objectField("reachable") catch return;
    s.write(reachable) catch return;
    s.objectField("in_path") catch return;
    s.write(false) catch return;
    s.objectField("manager") catch return;
    s.write(manager) catch return;
    if (install_dir.len > 0) {
        s.objectField("install_dir") catch return;
        s.write(install_dir) catch return;
    }
    if (err.len > 0) {
        s.objectField("error") catch return;
        s.write(err) catch return;
    }
    if (observations.len > 0) {
        s.objectField("observations") catch return;
        s.beginArray() catch return;
        for (observations) |obs| {
            s.write(obs) catch return;
        }
        s.endArray() catch return;
    }
    s.endObject() catch return;
    const written = std.Io.Writer.buffered(&writer);
    write_output(written.ptr, @intCast(written.len));
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// shell/local is always detected — it provides the working directory
// for every project and overrides the native shell/orbiter.
export fn detect() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    _ = read_input(&in_buf, @intCast(in_buf.len));

    const result = .{
        .detected = true,
        .resources = [_]struct {
            role: []const u8,
            brand: []const u8,
        }{.{ .role = "shell", .brand = "local" }},
    };
    var out_buf: [BUF_SIZE]u8 = undefined;
    const written = writeJSON(result, &out_buf);
    write_output(written.ptr, @intCast(written.len));
}

// getCwd runs `pwd` via the host and returns the trimmed result.
// Falls back to "." if pwd fails.
fn getCwd(spec_buf: []u8, out_buf: []u8) []u8 {
    const pwd_args = [_][]const u8{};
    const result = runCmd("pwd", &pwd_args, spec_buf, out_buf);
    if (result.len > 0) return result;
    out_buf[0] = '.';
    return out_buf[0..1];
}

// resolveInstallDir reads install_dir from parsed JSON context; if absent or
// empty, runs pwd to obtain the CWD.
fn resolveInstallDir(
    parsed: std.json.Value,
    spec_buf: []u8,
    out_buf: []u8,
) []u8 {
    if (parsed.object.get("install_dir")) |dir| {
        if (dir == .string and dir.string.len > 0) {
            // Copy into out_buf so lifetime is stable
            const len = @min(dir.string.len, out_buf.len);
            @memcpy(out_buf[0..len], dir.string[0..len]);
            return out_buf[0..len];
        }
    }
    return getCwd(spec_buf, out_buf);
}

export fn initialize() void {
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    var in_buf: [BUF_SIZE]u8 = undefined;
    const n = read_input(&in_buf, @intCast(in_buf.len));
    const input = in_buf[0..n];

    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(false, false, "local", "", "parse error", &.{}, &out_buf);
        return;
    };
    defer parsed.deinit();

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var dir_buf: [BUF_SIZE]u8 = undefined;
    const install_dir = resolveInstallDir(parsed.value, &spec_buf, &dir_buf);

    // The local filesystem represents the CWD — it always exists.
    // If install_dir came from pwd, the directory is guaranteed to exist.
    const obs = [_][]const u8{"directory exists"};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(true, true, "local", install_dir, "", &obs, &out_buf);
}

export fn scan() void {
    initialize();
}

// calibrate: ensure directory exists, then return install_dir.
// The non-empty install_dir in the returned StateReport causes executor.go
// to emit "cd <install_dir>" during the Jump lifecycle.
export fn calibrate() void {
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    var in_buf: [BUF_SIZE]u8 = undefined;
    const n = read_input(&in_buf, @intCast(in_buf.len));
    const input = in_buf[0..n];

    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        var out_buf: [BUF_SIZE]u8 = undefined;
        writeState(false, false, "local", "", "parse error", &.{}, &out_buf);
        return;
    };
    defer parsed.deinit();

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var dir_buf: [BUF_SIZE]u8 = undefined;
    const install_dir = resolveInstallDir(parsed.value, &spec_buf, &dir_buf);

    // Ensure the directory exists; mkdir -p is a no-op if it already exists.
    var mkdir_spec: [BUF_SIZE]u8 = undefined;
    var mkdir_out: [BUF_SIZE]u8 = undefined;
    const mkdir_args = [_][]const u8{ "-p", install_dir };
    _ = runCmd("mkdir", &mkdir_args, &mkdir_spec, &mkdir_out);

    var msg_buf: [512]u8 = undefined;
    const msg = std.fmt.bufPrint(&msg_buf, "calibrated: {s}", .{install_dir}) catch install_dir;
    const obs = [_][]const u8{msg};
    var out_buf: [BUF_SIZE]u8 = undefined;
    writeState(true, true, "local", install_dir, "", &obs, &out_buf);
}
