//go:build tinygo

package main

import (
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

//go:wasmimport orbiter read_input
func hostReadInput(ptr, max uint32) uint32

//go:wasmimport orbiter write_output
func hostWriteOutput(ptr, length uint32)

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

// echo reads "message" from input JSON and writes {"echo": <value>} as output.
//
//export echo
func echo() {
	input := readInput()
	msg := gjson.GetBytes(input, "message").String()
	out, err := sjson.Set("", "echo", msg)
	if err != nil {
		writeRaw([]byte(`{"echo":"error"}`))
		return
	}
	writeRaw([]byte(out))
}
