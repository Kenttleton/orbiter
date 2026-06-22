const std = @import("std");

// ── Host ABI ──────────────────────────────────────────────────────────────────

extern "orbiter" fn read_input(ptr: [*]u8, max: u32) u32;
extern "orbiter" fn write_output(ptr: [*]const u8, len: u32) void;
extern "orbiter" fn run_command(spec_ptr: [*]const u8, spec_len: u32, out_ptr: [*]u8, out_max: u32) u32;

const BUF_SIZE: usize = 65536;

// Global memory for the FixedBufferAllocator used in detect().
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
    in_path: bool,
    binary_path: ?[]const u8,
    manager: []const u8,
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
    s.write(in_path) catch return;
    if (binary_path) |bp| {
        s.objectField("binary_path") catch return;
        s.write(bp) catch return;
    }
    s.objectField("manager") catch return;
    s.write(manager) catch return;
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

export fn detect() void {
    var fba = std.heap.FixedBufferAllocator.init(&stack_mem);
    const allocator = fba.allocator();

    var in_buf: [BUF_SIZE]u8 = undefined;
    const n = read_input(&in_buf, @intCast(in_buf.len));
    const input = in_buf[0..n];

    const parsed = std.json.parseFromSlice(std.json.Value, allocator, input, .{}) catch {
        write_output("{\"detected\":false}", 18);
        return;
    };
    defer parsed.deinit();

    const files = parsed.value.object.get("files") orelse {
        write_output("{\"detected\":false}", 18);
        return;
    };
    const has_tool_versions = files.object.contains(".tool-versions");
    if (!has_tool_versions) {
        write_output("{\"detected\":false}", 18);
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
    const written = writeJSON(result, &out_buf);
    write_output(written.ptr, @intCast(written.len));
}

export fn initialize() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    const input_len = read_input(&in_buf, @intCast(in_buf.len));
    const input = in_buf[0..input_len];

    // Try to extract binaries.asdf from context JSON
    // For now, use a simpler approach: check if input contains "asdf" in binaries
    var binary_path: []const u8 = "";
    if (std.mem.indexOf(u8, input, "\"asdf\"") != null) {
        // Try to extract the path value following "asdf":"
        if (std.mem.indexOf(u8, input, "\"asdf\":\"")) |pos| {
            const after_key = input[pos + 9..]; // skip "asdf":"
            if (std.mem.indexOf(u8, after_key, "\"")) |end_pos| {
                binary_path = after_key[0..end_pos];
            }
        }
    }

    var out_buf: [BUF_SIZE]u8 = undefined;
    if (binary_path.len == 0) {
        writeState(false, false, false, null, "system", "asdf not found in PATH", &.{}, &out_buf);
        return;
    }

    var spec2: [BUF_SIZE]u8 = undefined;
    var out2: [BUF_SIZE]u8 = undefined;
    const ver_args = [_][]const u8{"version"};
    const version = runCmd("asdf", &ver_args, &spec2, &out2);

    const obs = [_][]const u8{version};
    writeState(true, true, true, binary_path, "system", "", &obs, &out_buf);
}

export fn scan() void {
    initialize();
}

export fn calibrate() void {
    var in_buf: [BUF_SIZE]u8 = undefined;
    _ = read_input(&in_buf, @intCast(in_buf.len));

    var spec_buf: [BUF_SIZE]u8 = undefined;
    var ver_out: [BUF_SIZE]u8 = undefined;
    const ver_args = [_][]const u8{"version"};
    const version = runCmd("asdf", &ver_args, &spec_buf, &ver_out);

    var out_buf: [BUF_SIZE]u8 = undefined;
    if (version.len == 0) {
        writeState(false, false, false, null, "system", "asdf not found", &.{}, &out_buf);
        return;
    }

    var calib_msg: [256]u8 = undefined;
    const msg = std.fmt.bufPrint(&calib_msg, "calibrated: {s}", .{version}) catch version;
    const obs = [_][]const u8{msg};
    writeState(true, true, true, null, "system", "", &obs, &out_buf);
}
