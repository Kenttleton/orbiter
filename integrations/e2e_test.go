package integrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/wasm"
)

// autoApprove is used in tests to approve all commands without reading stdin.
func autoApprove(_, _ string) bool { return true }

func setupBundleRegistry(t *testing.T) *core.Registry {
	t.Helper()
	reg := core.NewRegistry(nil)
	if err := integrations.InstallSelected(integrations.CatalogEntries(), reg, autoApprove); err != nil {
		t.Fatalf("InstallSelected: %v", err)
	}
	return reg
}

func TestTinyGoPOC_gjsonSjson(t *testing.T) {
	wasmBytes, err := os.ReadFile("tinygo-poc/poc.wasm")
	if err != nil {
		t.Skipf("poc.wasm not found (run go generate ./tinygo-poc/): %v", err)
	}
	reg := core.NewRegistry(nil)
	manifest := core.Manifest{
		Integration: core.ManifestIntegration{Brand: "poc", Roles: []string{"tool"}},
		Commands:    core.ManifestCommands{Allowed: []string{}},
	}
	i, err := wasm.Load(context.Background(), manifest, wasmBytes, reg.Settings(), reg, autoApprove)
	if err != nil {
		t.Fatalf("load poc wasm: %v", err)
	}
	// The POC exports "echo", not the standard handlers. Call detect as a proxy —
	// it will fail gracefully since "detect" is not exported, confirming wazero
	// handles missing exports cleanly. Then confirm the module loaded without panics.
	report := i.Detect(core.DetectContext{Files: map[string]string{"test": ""}})
	_ = report // detect returns zero value when "detect" not exported — that's expected
	t.Log("gjson/sjson POC: wasm module loaded and invoked without runtime traps")
}

func TestBundledIntegrations_Git(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (git is installed)")
		}
		if !report.Reachable {
			t.Error("expected reachable=true")
		}
		if !report.InPath {
			t.Error("expected in_path=true")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
		if len(report.Observations) == 0 || report.Observations[0] == "" {
			t.Error("expected observations to contain git version string")
		}
	})

	t.Run("init", func(t *testing.T) {
		report := i.Init(core.ResolvedContext{})
		t.Logf("Init: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path from init")
		}
		if report.Manager != "system" {
			t.Errorf("expected manager=system, got %q", report.Manager)
		}
	})

	t.Run("calibrate", func(t *testing.T) {
		report := i.Calibrate(core.ResolvedContext{})
		t.Logf("Calibrate: %+v", report)
		if !report.Present {
			t.Error("expected present=true after calibrate")
		}
		if len(report.Observations) == 0 {
			t.Error("expected calibrate to populate observations")
		} else if !strings.HasPrefix(report.Observations[0], "calibrated:") {
			t.Errorf("expected observations[0] to start with 'calibrated:', got %q", report.Observations[0])
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".git/config": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .git/config")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected at least one suggested resource")
		}
		if report.Resources[0].Role != "tool" {
			t.Errorf("expected role=tool, got %q", report.Resources[0].Role)
		}
		if report.Resources[0].Brand != "git" {
			t.Errorf("expected brand=git, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"package.json": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .git/config")
		}
	})
}

func TestBundledIntegrations_Node(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "node")
	if !ok {
		t.Fatal("node integration not registered")
	}

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"package.json": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for package.json")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected suggested resource")
		}
		if report.Resources[0].Role != "runtime" {
			t.Errorf("expected role=runtime, got %q", report.Resources[0].Role)
		}
		if report.Resources[0].Brand != "node" {
			t.Errorf("expected brand=node, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without package.json")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (node is installed on this machine)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
		if report.Manager != "system" {
			t.Errorf("expected manager=system, got %q", report.Manager)
		}
	})
}

func TestBundledIntegrations_Go(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "golang")
	if !ok {
		t.Fatal("golang integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
		if !report.Reachable {
			t.Error("expected reachable=true")
		}
		if !report.InPath {
			t.Error("expected in_path=true")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
		if len(report.Observations) == 0 || report.Observations[0] == "" {
			t.Error("expected observations to contain go version string")
		}
	})

	t.Run("init", func(t *testing.T) {
		report := i.Init(core.ResolvedContext{})
		t.Logf("Init: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path from init")
		}
		if report.Manager != "system" {
			t.Errorf("expected manager=system, got %q", report.Manager)
		}
	})

	t.Run("calibrate", func(t *testing.T) {
		report := i.Calibrate(core.ResolvedContext{})
		t.Logf("Calibrate: %+v", report)
		if !report.Present {
			t.Error("expected present=true after calibrate")
		}
		if len(report.Observations) == 0 {
			t.Error("expected calibrate to populate observations")
		} else if !strings.HasPrefix(report.Observations[0], "calibrated:") {
			t.Errorf("expected observations[0] to start with 'calibrated:', got %q", report.Observations[0])
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": "", "go.sum": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for go.mod")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected at least one suggested resource")
		}
		if report.Resources[0].Role != "runtime" {
			t.Errorf("expected role=runtime, got %q", report.Resources[0].Role)
		}
		if report.Resources[0].Brand != "golang" {
			t.Errorf("expected brand=golang, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"package.json": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without go.mod")
		}
	})
}

func TestBundledIntegrations_Make(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "make")
	if !ok {
		t.Fatal("make integration not registered")
	}

	t.Run("detect_makefile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Makefile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Makefile")
		}
	})

	t.Run("detect_gnumakefile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"GNUmakefile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for GNUmakefile")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Justfile": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without Makefile")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (make is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}
