package wasm

import (
	"runtime"
	"testing"
)

func TestResolveBinaries_EmptyList(t *testing.T) {
	result := ResolveBinaries([]string{}, runtime.GOOS)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestResolveBinaries_KnownBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	result := ResolveBinaries([]string{"sh"}, runtime.GOOS)
	if result["sh"] == "" {
		t.Error("expected non-empty path for sh")
	}
}

func TestResolveBinaries_UnknownBinary(t *testing.T) {
	result := ResolveBinaries([]string{"__orbiter_nonexistent_binary__"}, runtime.GOOS)
	if result["__orbiter_nonexistent_binary__"] != "" {
		t.Errorf("expected empty path for nonexistent binary")
	}
}
