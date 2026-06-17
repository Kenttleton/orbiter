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

func readInput() []byte {
	buf := make([]byte, 64*1024)
	n := hostReadInput(ptrOf(buf), uint32(len(buf)))
	return buf[:n]
}

func writeRaw(b []byte) {
	hostWriteOutput(ptrOf(b), uint32(len(b)))
}

// runCmd builds a {"cmd":"...","args":[...]} spec with hand-rolled JSON.
// gjson/sjson crash on wasm-unknown due to unicode/utf16 internals.
func runCmd(cmd string, args ...string) string {
	spec := append(append(append([]byte(nil), `{"cmd":`...), jsonBytes(cmd)...), `,"args":[`...)
	for i, a := range args {
		if i > 0 {
			spec = append(spec, ',')
		}
		spec = append(spec, jsonBytes(a)...)
	}
	spec = append(spec, `]}`...)
	out := make([]byte, 64*1024)
	n := hostRunCommand(ptrOf(spec), uint32(len(spec)), ptrOf(out), uint32(len(out)))
	return strings.TrimSpace(string(out[:n]))
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
	s := string(input)
	hasMakefile := strings.Contains(s, `"Makefile"`)
	hasGNUmakefile := strings.Contains(s, `"GNUmakefile"`)
	if !hasMakefile && !hasGNUmakefile {
		writeRaw([]byte(`{"detected":false}`))
		return
	}
	version := runCmd("make", "--version")
	buf := append([]byte(`{"detected":true,"resources":[{"role":"tool","brand":"make","version":`),
		jsonBytes(parseMakeVersion(version))...)
	writeRaw(append(buf, `}]}`...))
}

//export initialize
func initialize() {
	readInput()
	binaryPath := runCmd("which", "make")
	if binaryPath == "" {
		writeState(false, false, false, "", "system", "make not found in PATH", nil)
		return
	}
	version := runCmd("make", "--version")
	writeState(true, true, true, binaryPath, "system", "", []string{version})
}

//export scan
func scan() { initialize() }

//export calibrate
func calibrate() {
	readInput()
	version := runCmd("make", "--version")
	if version == "" {
		writeState(false, false, false, "", "system", "make not found", nil)
		return
	}
	writeState(true, true, true, "", "system", "", []string{"calibrated: " + firstLine(version)})
}

func parseMakeVersion(s string) string {
	// "GNU Make 4.3\n..." → "4.3"
	s = firstLine(s)
	if idx := strings.LastIndex(s, " "); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
