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

// --- ABI helpers (no encoding/json, no strings.Builder — both unstable on wasm-unknown) ---

func ptrOf(b []byte) uint32 { return uint32(uintptr(unsafe.Pointer(&b[0]))) }

func readInput() []byte {
	buf := make([]byte, 64*1024)
	n := hostReadInput(ptrOf(buf), uint32(len(buf)))
	return buf[:n]
}

func writeRaw(b []byte) {
	hostWriteOutput(ptrOf(b), uint32(len(b)))
}

// runCmd executes a host command. The spec is a hand-built JSON byte slice to
// avoid both encoding/json reflection and strings.Builder (both crash on wasm-unknown).
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

// hasKey checks whether the JSON byte slice contains a given key.
func hasKey(input []byte, key string) bool {
	needle := `"` + key + `"`
	return strings.Contains(string(input), needle)
}

// --- output builders ([]byte append only) ---

type suggestedResource struct {
	Role    string
	Brand   string
	Version string
}

func writeDetectReport(detected bool, resources []suggestedResource) {
	if !detected {
		writeRaw([]byte(`{"detected":false}`))
		return
	}
	buf := []byte(`{"detected":true,"resources":[`)
	for i, r := range resources {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, `{"role":`...)
		buf = append(buf, jsonBytes(r.Role)...)
		buf = append(buf, `,"brand":`...)
		buf = append(buf, jsonBytes(r.Brand)...)
		if r.Version != "" {
			buf = append(buf, `,"version":`...)
			buf = append(buf, jsonBytes(r.Version)...)
		}
		buf = append(buf, '}')
	}
	writeRaw(append(buf, `]}`...))
}

type stateReport struct {
	Present      bool
	Reachable    bool
	BinaryPath   string
	InPath       bool
	Manager      string
	Error        string
	Observations []string
}

func boolBytes(v bool) []byte {
	if v {
		return []byte("true")
	}
	return []byte("false")
}

func writeStateReport(r stateReport) {
	buf := append(append([]byte(`{"present":`), boolBytes(r.Present)...), `,"reachable":`...)
	buf = append(buf, boolBytes(r.Reachable)...)
	if r.BinaryPath != "" {
		buf = append(buf, `,"binary_path":`...)
		buf = append(buf, jsonBytes(r.BinaryPath)...)
	}
	buf = append(buf, `,"in_path":`...)
	buf = append(buf, boolBytes(r.InPath)...)
	buf = append(buf, `,"manager":`...)
	buf = append(buf, jsonBytes(r.Manager)...)
	if r.Error != "" {
		buf = append(buf, `,"error":`...)
		buf = append(buf, jsonBytes(r.Error)...)
	}
	if len(r.Observations) > 0 {
		buf = append(buf, `,"observations":[`...)
		for i, o := range r.Observations {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, jsonBytes(o)...)
		}
		buf = append(buf, ']')
	}
	writeRaw(append(buf, '}'))
}

// --- exported handlers (Lambda-style: stateless, independently callable) ---

//export detect
func detect() {
	input := readInput()
	if !hasKey(input, "go.mod") {
		writeDetectReport(false, nil)
		return
	}
	version := runCmd("go", "version")
	writeDetectReport(true, []suggestedResource{{
		Role:    "runtime",
		Brand:   "go",
		Version: parseGoVersion(version),
	}})
}

//export initialize
func initialize() {
	readInput()
	binaryPath := runCmd("which", "go")
	if binaryPath == "" {
		writeStateReport(stateReport{Manager: "system", Error: "go binary not found in PATH"})
		return
	}
	version := runCmd("go", "version")
	writeStateReport(stateReport{
		Present:    true,
		Reachable:  true,
		BinaryPath: binaryPath,
		InPath:     true,
		Manager:    "system",
		Observations: []string{version},
	})
}

//export scan
func scan() { initialize() }

//export calibrate
func calibrate() {
	readInput()
	version := runCmd("go", "version")
	if version == "" {
		writeStateReport(stateReport{Manager: "system", Error: "go binary not found"})
		return
	}
	writeStateReport(stateReport{
		Present:   true,
		Reachable: true,
		InPath:    true,
		Manager:   "system",
		Observations: []string{"calibrated: " + version},
	})
}

func parseGoVersion(s string) string {
	s = strings.TrimPrefix(s, "go version go")
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return s[:idx]
	}
	return s
}
