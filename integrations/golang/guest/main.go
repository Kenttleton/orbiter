//go:build tinygo

package main

import (
	"strings"
	"unsafe"
)

//go:wasmimport orbiter read_input
func hostReadInput(ptr, max uint32) uint32

//go:wasmimport orbiter write_output
func hostWriteOutput(ptr, length uint32)

//go:wasmimport orbiter run_command
func hostRunCommand(specPtr, specLen, outPtr, max uint32) uint32

func main() {}

func ptrOf(b []byte) uint32 { return uint32(uintptr(unsafe.Pointer(&b[0]))) }

// inputBuf and cmdOut are package-level buffers so TinyGo's allocator never
// reuses their backing arrays for other heap objects while they're in use.
// This avoids a TinyGo wasm-unknown aliasing bug where the GC reclaims a
// large local []byte and then reallocates it for the writeState output buffer,
// causing overlap between the command output and the JSON being built.
var inputBuf = make([]byte, 64*1024)
var cmdOut = make([]byte, 64*1024)

func readInput() []byte {
	n := hostReadInput(ptrOf(inputBuf), uint32(len(inputBuf)))
	return inputBuf[:n]
}

func writeRaw(b []byte) {
	hostWriteOutput(ptrOf(b), uint32(len(b)))
}

// runCmd builds a {"cmd":"...","args":[...]} spec with hand-rolled JSON.
// sjson array append (-1) triggers gjson's UTF-16 path and crashes on wasm-unknown.
func runCmd(cmd string, args ...string) string {
	spec := append(append(append([]byte(nil), `{"cmd":`...), jsonBytes(cmd)...), `,"args":[`...)
	for i, a := range args {
		if i > 0 {
			spec = append(spec, ',')
		}
		spec = append(spec, jsonBytes(a)...)
	}
	spec = append(spec, `]}`...)
	n := hostRunCommand(ptrOf(spec), uint32(len(spec)), ptrOf(cmdOut), uint32(len(cmdOut)))
	return strings.TrimSpace(string(cmdOut[:n]))
}

// jsonBytes returns a JSON-quoted string as []byte without reflection or Builder.
func jsonBytes(s string) []byte {
	const hex = "0123456789abcdef"
	buf := []byte{'"'}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf = append(buf, '\\', '"')
		case '\\':
			buf = append(buf, '\\', '\\')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
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

func boolBytes(v bool) []byte {
	if v {
		return []byte("true")
	}
	return []byte("false")
}

func writeState(present, reachable, inPath bool, binaryPath, manager, errMsg string, observations []string) {
	buf := append(append([]byte(`{"present":`), boolBytes(present)...), `,"reachable":`...)
	buf = append(buf, boolBytes(reachable)...)
	buf = append(buf, `,"in_path":`...)
	buf = append(buf, boolBytes(inPath)...)
	buf = append(buf, `,"manager":`...)
	buf = append(buf, jsonBytes(manager)...)
	if binaryPath != "" {
		buf = append(buf, `,"binary_path":`...)
		buf = append(buf, jsonBytes(binaryPath)...)
	}
	if errMsg != "" {
		buf = append(buf, `,"error":`...)
		buf = append(buf, jsonBytes(errMsg)...)
	}
	if len(observations) > 0 {
		buf = append(buf, `,"observations":[`...)
		for i, o := range observations {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, jsonBytes(o)...)
		}
		buf = append(buf, ']')
	}
	writeRaw(append(buf, '}'))
}

//export detect
func detect() {
	input := readInput()
	// Use strings.Contains to find "go.mod" key in the files map — gjson's
	// backslash-escaped path handling for keys with dots is unreliable on
	// wasm-unknown. The raw JSON will contain the literal string `"go.mod"`.
	if !strings.Contains(string(input), `"go.mod"`) {
		writeRaw([]byte(`{"detected":false}`))
		return
	}
	version := runCmd("go", "version")
	buf := append([]byte(`{"detected":true,"resources":[{"role":"runtime","brand":"golang","version":`),
		jsonBytes(parseGoVersion(version))...)
	writeRaw(append(buf, `}]}`...))
}

// extractBinaryPath extracts ctx.binaries["name"] from raw ResolvedContext JSON.
// The JSON shape is: {"binaries":{"go":"/usr/local/go/bin/go",...},...}
// Uses simple string scanning since gjson is unavailable in wasm-unknown.
func extractBinaryPath(input []byte, name string) string {
	s := string(input)
	key := `"binaries":{`
	start := strings.Index(s, key)
	if start < 0 {
		return ""
	}
	sub := s[start+len(key):]
	end := strings.Index(sub, "}")
	if end >= 0 {
		sub = sub[:end]
	}
	needle := `"` + name + `":"`
	idx := strings.Index(sub, needle)
	if idx < 0 {
		return ""
	}
	rest := sub[idx+len(needle):]
	close := strings.Index(rest, `"`)
	if close < 0 {
		return ""
	}
	return rest[:close]
}

//export initialize
func initialize() {
	binaryPath := extractBinaryPath(readInput(), "go")
	present := binaryPath != ""
	if !present {
		writeState(false, false, false, "", "system", "go binary not found", nil)
		return
	}
	version := runCmd("go", "version")
	reachable := version != ""
	writeState(true, reachable, reachable, binaryPath, "system", "", []string{version})
}

//export scan
func scan() { initialize() }

//export calibrate
func calibrate() {
	readInput()
	version := runCmd("go", "version")
	if version == "" {
		writeState(false, false, false, "", "system", "go binary not found", nil)
		return
	}
	writeState(true, true, true, "", "system", "", []string{"calibrated: " + version})
}

func parseGoVersion(s string) string {
	s = strings.TrimPrefix(s, "go version go")
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return s[:idx]
	}
	return s
}
