package integrations

import (
	"runtime"
	"testing"
)

func TestFindBinary_KnownBinary(t *testing.T) {
	// sh exists on all non-Windows platforms this test runs on
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	path := FindBinary("sh", runtime.GOOS)
	if path == "" {
		t.Error("expected non-empty path for sh")
	}
}

func TestFindBinary_UnknownBinary(t *testing.T) {
	path := FindBinary("__orbiter_nonexistent_binary__", runtime.GOOS)
	if path != "" {
		t.Errorf("expected empty path for nonexistent binary, got %q", path)
	}
}
