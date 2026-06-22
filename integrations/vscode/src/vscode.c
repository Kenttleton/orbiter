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
 * Memory: all large buffers are global (linear memory), not stack-allocated,
 * to avoid WASM shadow stack overflow.
 */

/* ── Types ─────────────────────────────────────────────────────────────────── */
typedef unsigned int   uint32_t;
typedef unsigned char  uint8_t;
typedef int            int32_t;
typedef unsigned long  size_t;

/* ── Compiler builtins (needed by optimizer even with -nostdlib) ────────────── */
void *memcpy(void *dst, const void *src, size_t n) {
    uint8_t *d = (uint8_t *)dst;
    const uint8_t *s = (const uint8_t *)src;
    for (size_t i = 0; i < n; i++) d[i] = s[i];
    return dst;
}

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

/* Append a C string to buf at offset pos. Returns new offset. */
static uint32_t append(uint8_t *buf, uint32_t pos, const char *s) {
    uint32_t n = (uint32_t)cstrlen(s);
    for (uint32_t i = 0; i < n; i++) buf[pos + i] = (uint8_t)s[i];
    return pos + n;
}

/* ── Global buffers (in WASM linear memory, not shadow stack) ─────────────── */

#define BUF_SIZE 4096

static uint8_t g_input[BUF_SIZE];
static uint8_t g_spec[BUF_SIZE];
static uint8_t g_cmd_out[BUF_SIZE];
static uint8_t g_out[BUF_SIZE];
static uint8_t g_tmp[BUF_SIZE];

/* ── Shared helpers ────────────────────────────────────────────────────────── */

/*
 * run_cmd builds the JSON command spec and calls host_run_command.
 * Uses global g_spec for the spec JSON and writes output to out_buf.
 * Returns the number of output bytes written (trimmed).
 */
static uint32_t run_cmd(const char *cmd, const char *arg, uint8_t *out_buf) {
    uint32_t pos = 0;
    pos = append(g_spec, pos, "{\"cmd\":\"");
    pos = append(g_spec, pos, cmd);
    if (arg != 0) {
        pos = append(g_spec, pos, "\",\"args\":[\"");
        pos = append(g_spec, pos, arg);
        pos = append(g_spec, pos, "\"]}");
    } else {
        pos = append(g_spec, pos, "\",\"args\":[]}");
    }
    uint32_t n = host_run_command(g_spec, pos, out_buf, BUF_SIZE);
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
    uint32_t pos = 0;
    pos = append(g_out, pos, "{\"present\":");
    pos = append(g_out, pos, present ? "true" : "false");
    pos = append(g_out, pos, ",\"reachable\":");
    pos = append(g_out, pos, reachable ? "true" : "false");
    pos = append(g_out, pos, ",\"in_path\":");
    pos = append(g_out, pos, in_path ? "true" : "false");
    pos = append(g_out, pos, ",\"manager\":\"system\"");
    if (binary_path && cstrlen(binary_path) > 0) {
        pos = append(g_out, pos, ",\"binary_path\":\"");
        pos = append(g_out, pos, binary_path);
        pos = append(g_out, pos, "\"");
    }
    if (error_str && cstrlen(error_str) > 0) {
        pos = append(g_out, pos, ",\"error\":\"");
        pos = append(g_out, pos, error_str);
        pos = append(g_out, pos, "\"");
    }
    if (version_out && version_len > 0) {
        pos = append(g_out, pos, ",\"observations\":[\"");
        for (uint32_t i = 0; i < version_len && pos < BUF_SIZE - 6; i++) {
            uint8_t c = version_out[i];
            if (c == '"') {
                g_out[pos++] = '\\';
                g_out[pos++] = '"';
            } else if (c == '\\') {
                g_out[pos++] = '\\';
                g_out[pos++] = '\\';
            } else if (c == '\n') {
                g_out[pos++] = '\\';
                g_out[pos++] = 'n';
            } else if (c == '\r') {
                g_out[pos++] = '\\';
                g_out[pos++] = 'r';
            } else if (c == '\t') {
                g_out[pos++] = '\\';
                g_out[pos++] = 't';
            } else if (c < 0x20) {
                /* Skip other control characters */
            } else {
                g_out[pos++] = c;
            }
        }
        pos = append(g_out, pos, "\"]");
    }
    g_out[pos++] = '}';
    host_write_output(g_out, pos);
}

/* ── Handlers ─────────────────────────────────────────────────────────────── */

__attribute__((visibility("default")))
void detect(void) {
    uint32_t n = host_read_input(g_input, BUF_SIZE);

    /* Detect .vscode/ directory by looking for keys like ".vscode/" in the
     * files map. The detection context contains a JSON "files" object whose
     * keys are relative paths. Presence of any key starting with ".vscode/"
     * means the project uses VSCode. */
    int has_vscode = contains(g_input, n, "\".vscode/")
                  || contains(g_input, n, "\".vscode\\\\"); /* Windows path escape */

    if (!has_vscode) {
        host_write_output((const uint8_t*)"{\"detected\":false}", 18);
        return;
    }
    host_write_output(
        (const uint8_t*)"{\"detected\":true,\"resources\":[{\"role\":\"tool\",\"brand\":\"vscode\"}]}",
        64
    );
}

/* Extract binary path from binaries.code in the JSON input */
static size_t extract_binary_path(const uint8_t *input, uint32_t input_len,
                                   uint8_t *out_buf) {
    const char *needle = "\"code\":\"";
    size_t needle_len = 8; // strlen("\"code\":\"")

    for (uint32_t i = 0; i + needle_len < input_len; i++) {
        int match = 1;
        for (size_t j = 0; j < needle_len; j++) {
            if (input[i + j] != (uint8_t)needle[j]) {
                match = 0;
                break;
            }
        }
        if (match) {
            /* Found "code":" — read until closing quote */
            uint32_t pos = 0;
            uint32_t src = i + needle_len;
            while (src < input_len && pos < BUF_SIZE - 1) {
                if (input[src] == '"') {
                    out_buf[pos] = '\0';
                    return pos;
                }
                if (input[src] == '\\' && src + 1 < input_len) {
                    src++; /* skip escape char */
                }
                out_buf[pos++] = input[src];
                src++;
            }
        }
    }
    return 0;
}

__attribute__((visibility("default")))
void initialize(void) {
    uint32_t input_len = host_read_input(g_input, BUF_SIZE);

    uint32_t which_n = extract_binary_path(g_input, input_len, g_cmd_out);

    if (which_n == 0) {
        write_state(0, 0, 0, 0, 0, 0, "code binary not found");
        return;
    }

    /* Copy output to g_tmp for binary_path (null-terminate) */
    for (uint32_t i = 0; i < which_n && i < BUF_SIZE - 1; i++) {
        g_tmp[i] = g_cmd_out[i];
    }
    g_tmp[which_n] = '\0';

    /* Run version command — reuses g_cmd_out */
    uint32_t ver_n = run_cmd("code", "--version", g_cmd_out);

    write_state(1, 1, 1, (const char*)g_tmp, g_cmd_out, ver_n, 0);
}

__attribute__((visibility("default")))
void scan(void) {
    initialize();
}

__attribute__((visibility("default")))
void calibrate(void) {
    host_read_input(g_input, BUF_SIZE);

    uint32_t ver_n = run_cmd("code", "--version", g_cmd_out);

    if (ver_n == 0) {
        write_state(0, 0, 0, 0, 0, 0, "code not found");
        return;
    }

    /* Prepend "calibrated: " to the version output in g_tmp */
    uint32_t pos = 0;
    const char *prefix = "calibrated: ";
    while (prefix[pos] && pos < BUF_SIZE - 1) {
        g_tmp[pos] = (uint8_t)prefix[pos];
        pos++;
    }
    for (uint32_t i = 0; i < ver_n && pos < BUF_SIZE - 1; i++) {
        g_tmp[pos++] = g_cmd_out[i];
    }

    write_state(1, 1, 1, 0, g_tmp, pos, 0);
}
