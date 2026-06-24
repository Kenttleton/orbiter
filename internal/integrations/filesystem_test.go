package integrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestParsePath_Empty(t *testing.T) {
	got, err := ParsePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoUnix() != "" || got.IntoWindows() != "" || got.IntoNative() != "" || got.IntoShell() != "" {
		t.Errorf("expected zero Path for empty input, got %+v", got)
	}
}

func TestParsePath_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath("/usr/local/bin/brew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoNative() != "/usr/local/bin/brew" {
		t.Errorf("Native: expected /usr/local/bin/brew, got %q", got.IntoNative())
	}
	if got.IntoShell() != "/usr/local/bin/brew" {
		t.Errorf("Shell: expected /usr/local/bin/brew (no spaces), got %q", got.IntoShell())
	}
	if got.IntoUnix() != "/usr/local/bin/brew" {
		t.Errorf("Unix: expected /usr/local/bin/brew, got %q", got.IntoUnix())
	}
	if got.IntoWindows() != `\usr\local\bin\brew` {
		t.Errorf("Windows: expected \\usr\\local\\bin\\brew, got %q", got.IntoWindows())
	}
}

func TestParsePath_SpaceInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath("/home/captain/my projects/bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Shell must wrap in double-quotes when Native contains a space
	if got.IntoShell() != `"/home/captain/my projects/bin"` {
		t.Errorf("Shell: expected quoted path, got %q", got.IntoShell())
	}
	// Native and Unix are raw (unquoted)
	if got.IntoNative() != "/home/captain/my projects/bin" {
		t.Errorf("Native: expected unquoted path, got %q", got.IntoNative())
	}
}

func TestParsePath_ShellQuoted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path test")
	}
	got, err := ParsePath(`"/home/captain/my projects/bin"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Shell-quoted input must produce the same result as the unquoted form
	if got.IntoUnix() != "/home/captain/my projects/bin" {
		t.Errorf("Unix: expected unquoted path, got %q", got.IntoUnix())
	}
	if got.IntoShell() != `"/home/captain/my projects/bin"` {
		t.Errorf("Shell: expected re-quoted output, got %q", got.IntoShell())
	}
}

func TestParsePath_WindowsForm(t *testing.T) {
	got, err := ParsePath(`C:\Users\captain\bin`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IntoUnix() != "C:/Users/captain/bin" {
		t.Errorf("Unix: expected C:/Users/captain/bin, got %q", got.IntoUnix())
	}
	if got.IntoWindows() != `C:\Users\captain\bin` {
		t.Errorf("Windows: expected C:\\Users\\captain\\bin, got %q", got.IntoWindows())
	}
	// Shell must not wrap when no spaces present
	if strings.Contains(got.IntoShell(), `"`) {
		t.Errorf("Shell: expected no quotes for path without spaces, got %q", got.IntoShell())
	}
}

func TestParsePath_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := ParsePath("~/bin/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNative := filepath.Join(home, "bin", "foo")
	wantUnix := filepath.ToSlash(wantNative)
	wantWindows := strings.ReplaceAll(wantUnix, "/", `\`)
	if got.IntoNative() != wantNative {
		t.Errorf("Native: expected %q, got %q", wantNative, got.IntoNative())
	}
	if got.IntoUnix() != wantUnix {
		t.Errorf("Unix: expected %q, got %q", wantUnix, got.IntoUnix())
	}
	if got.IntoWindows() != wantWindows {
		t.Errorf("Windows: expected %q, got %q", wantWindows, got.IntoWindows())
	}
}

func TestParsePath_TildeAlone(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := ParsePath("~")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNative := home
	wantUnix := filepath.ToSlash(home)
	wantWindows := strings.ReplaceAll(wantUnix, "/", `\`)
	if got.IntoNative() != wantNative {
		t.Errorf("Native: expected %q, got %q", wantNative, got.IntoNative())
	}
	if got.IntoUnix() != wantUnix {
		t.Errorf("Unix: expected %q, got %q", wantUnix, got.IntoUnix())
	}
	if got.IntoWindows() != wantWindows {
		t.Errorf("Windows: expected %q, got %q", wantWindows, got.IntoWindows())
	}
}

func TestParsePath_WindowsNativeMethod(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows native-form test")
	}
	got, err := ParsePath(`C:\Users\captain\bin`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Native() must return backslash form on Windows
	if strings.Contains(got.IntoNative(), "/") {
		t.Errorf("Native() on Windows must not contain forward slashes, got %q", got.IntoNative())
	}
	// Shell must equal Native when no spaces present
	if got.IntoShell() != got.IntoNative() {
		t.Errorf("Shell: expected equal to Native when no spaces, got %q", got.IntoShell())
	}
	// Unix must use forward slashes
	if strings.Contains(got.IntoUnix(), `\`) {
		t.Errorf("Unix() must not contain backslashes, got %q", got.IntoUnix())
	}
}

func TestPathFromConfig_EmptyConfig(t *testing.T) {
	ctx := ResolvedContext{}
	got := PathFromConfig(ctx)
	if got.IntoUnix() != "" || got.IntoWindows() != "" {
		t.Errorf("expected empty Path for nil Self, got %+v", got)
	}
}
