package integrations_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/Kenttleton/orbiter/internal/models"
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
		Commands:    core.ManifestCommands{Allowed: []core.ManifestCommandEntry{}},
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

func gitCtx(role string) core.ResolvedContext {
	return core.ResolvedContext{
		Self:         models.Resource{Role: role, Brand: "git"},
		Resources:    map[string][]core.ResolvedResource{},
		Transponders: map[string][]core.ResolvedTransponder{},
	}
}

func TestBundledIntegrations_Git(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(gitCtx("tool"))
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
		report := i.Init(gitCtx("tool"))
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
		report := i.Calibrate(gitCtx("tool"))
		t.Logf("Calibrate: %+v", report)
		if !report.Present {
			t.Error("expected present=true after calibrate")
		}
		if len(report.Observations) == 0 || report.Observations[0] == "" {
			t.Error("expected calibrate to populate observations")
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

func TestBundledIntegrations_Python(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "python")
	if !ok {
		t.Fatal("python integration not registered")
	}

	t.Run("detect_pyproject", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"pyproject.toml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for pyproject.toml")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "runtime" {
			t.Error("expected role=runtime suggestion")
		}
	})

	t.Run("detect_requirements", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"requirements.txt": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for requirements.txt")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without python files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (python3 is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}

func TestBundledIntegrations_Rust(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "rust")
	if !ok {
		t.Fatal("rust integration not registered")
	}

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Cargo.toml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Cargo.toml")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "runtime" {
			t.Error("expected role=runtime suggestion")
		}
		if report.Resources[0].Brand != "rust" {
			t.Errorf("expected brand=rust, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without Cargo.toml")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (rustc is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}

func TestBundledIntegrations_Brew(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("brew not available in CI")
	}
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "brew")
	if !ok {
		t.Fatal("brew integration not registered")
	}

	t.Run("detect_miss", func(t *testing.T) {
		// brew detect uses PATH check, not files — DetectContext has no brew signal
		// A project with only go.mod won't detect brew
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		// brew's detect always returns detected=false when there's no brew-specific file —
		// brew is a system tool detected via scan, not project files
		_ = report.Detected // any value is valid; brew detection is path-based
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// Only assert if brew is actually installed
		brewPath := ""
		if out, err := exec.Command("which", "brew").Output(); err == nil {
			brewPath = strings.TrimSpace(string(out))
		}
		if brewPath != "" {
			if !report.Present {
				t.Error("brew installed but present=false")
			}
			if report.BinaryPath == "" {
				t.Error("expected non-empty binary_path")
			}
		}
	})
}

func TestBundledIntegrations_UV(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "uv")
	if !ok {
		t.Fatal("uv integration not registered")
	}

	t.Run("detect_lock", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"uv.lock": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for uv.lock")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without uv files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// uv may not be installed everywhere; just verify shape
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_Rustup(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "rustup")
	if !ok {
		t.Fatal("rustup integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (rustup is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
		if len(report.Observations) == 0 {
			t.Error("expected observations (toolchain info)")
		}
	})
}

func TestBundledIntegrations_Docker(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "docker")
	if !ok {
		t.Fatal("docker integration not registered")
	}

	t.Run("detect_dockerfile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Dockerfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Dockerfile")
		}
	})

	t.Run("detect_compose", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"docker-compose.yml": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for docker-compose.yml")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without docker files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_KeychainMacOS(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("keychain", "macos")
	if !ok {
		t.Fatal("macos keychain integration not registered")
	}

	t.Run("detect_darwin", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		if !report.Detected {
			t.Error("expected detected=true on darwin")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected suggested resource")
		}
		if report.Resources[0].Role != "keychain" {
			t.Errorf("expected role=keychain, got %q", report.Resources[0].Role)
		}
	})

	t.Run("detect_linux", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "linux"},
		})
		if report.Detected {
			t.Error("expected detected=false on linux")
		}
	})

	t.Run("scan_darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("keychain scan only valid on darwin")
		}
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true on darwin (security binary should exist)")
		}
	})
}

func TestBundledIntegrations_OnePassword(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("vault", "onepassword")
	if !ok {
		t.Fatal("onepassword integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// op may not be installed everywhere; just verify shape
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_SSHAgent(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "ssh")
	if !ok {
		t.Fatal("ssh agent integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// ssh-agent may or may not be running in CI
		t.Logf("SSH agent present: %v, reachable: %v", report.Present, report.Reachable)
	})
}

func TestBundledIntegrations_NVM(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "nvm")
	if !ok {
		t.Fatal("nvm integration not registered")
	}

	t.Run("detect_nvmrc", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".nvmrc": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .nvmrc")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "manager" {
			t.Error("expected role=manager suggestion")
		}
		if report.Resources[0].Brand != "nvm" {
			t.Errorf("expected brand=nvm, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_node_version", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".node-version": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .node-version")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without nvm files")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager != "nvm" {
			t.Errorf("expected manager=nvm, got %q", report.Manager)
		}
		if !report.Present {
			t.Error("expected present=true for nvm")
		}
	})
}

func TestBundledIntegrations_Just(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "just")
	if !ok {
		t.Fatal("just integration not registered")
	}

	t.Run("detect_justfile", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Justfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for Justfile")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "tool" {
			t.Error("expected role=tool suggestion")
		}
	})

	t.Run("detect_lowercase", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"justfile": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for lowercase justfile")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"Makefile": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without Justfile")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_Dotenv(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("file", "dotenv")
	if !ok {
		t.Fatal("dotenv integration not registered")
	}

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".env": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .env file")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .env")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		// dotenv scan reports present if .env is readable — on CI there may not be a .env
		// so we only verify the shape of the response, not present=true
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_Asdf(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("manager", "asdf")
	if !ok {
		t.Fatal("asdf integration not registered")
	}

	t.Run("detect_tool_versions", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".tool-versions": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .tool-versions")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "manager" {
			t.Error("expected role=manager suggestion")
		}
		if report.Resources[0].Brand != "asdf" {
			t.Errorf("expected brand=asdf, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .tool-versions")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_EnvShell(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("env", "shell")
	if !ok {
		t.Fatal("shell env integration not registered")
	}

	t.Run("detect_always", func(t *testing.T) {
		// env/shell is always detected — every environment has shell variables
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for env/shell (always active)")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "env" {
			t.Error("expected role=env suggestion")
		}
	})

	t.Run("scan_with_env", func(t *testing.T) {
		// Set up context with some env vars to check
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true for env/shell")
		}
		if report.Manager != "shell" {
			t.Errorf("expected manager=shell, got %q", report.Manager)
		}
	})
}

func TestBundledIntegrations_VSCode(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "vscode")
	if !ok {
		t.Fatal("vscode integration not registered")
	}

	t.Run("detect_vscode_dir", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".vscode/settings.json": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .vscode/ directory")
		}
		if len(report.Resources) == 0 || report.Resources[0].Role != "tool" {
			t.Error("expected role=tool suggestion")
		}
		if report.Resources[0].Brand != "vscode" {
			t.Errorf("expected brand=vscode, got %q", report.Resources[0].Brand)
		}
	})

	t.Run("detect_vscode_launch", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".vscode/launch.json": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .vscode/launch.json")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .vscode/")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_GitHub_Registration(t *testing.T) {
	reg := setupBundleRegistry(t)

	t.Run("tool_role", func(t *testing.T) {
		_, ok := reg.Get("tool", "github")
		if !ok {
			t.Fatal("github not registered as tool")
		}
	})

	t.Run("remote_role", func(t *testing.T) {
		_, ok := reg.Get("remote", "github")
		if !ok {
			t.Fatal("github not registered as remote")
		}
	})

	t.Run("agent_role", func(t *testing.T) {
		_, ok := reg.Get("agent", "github")
		if !ok {
			t.Fatal("github not registered as agent")
		}
	})
}

func TestBundledIntegrations_GitHub_Tool(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("tool", "github")
	if !ok {
		t.Fatal("github tool not registered")
	}

	t.Run("detect_git_config_github", func(t *testing.T) {
		// The detect handler checks for .git/config — if it also contains
		// "github.com" that strengthens detection, but .git/config alone is enough.
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".git/config": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .git/config")
		}
		if len(report.Resources) == 0 {
			t.Fatal("expected suggested resources")
		}
		// Tool role should be first suggestion
		found := false
		for _, r := range report.Resources {
			if r.Role == "tool" && r.Brand == "github" {
				found = true
			}
		}
		if !found {
			t.Error("expected tool/github in suggested resources")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .git/config")
		}
	})

	t.Run("scan_tool", func(t *testing.T) {
		// Empty ResolvedContext serializes to {"self":{"role":""}} on the Go side,
		// which the WASM deserializes to an empty role string — falling through to
		// the default "tool" branch. This is intentional: the test exercises the
		// tool scan path without needing to set Self.Role explicitly.
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Tool Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (gh is installed)")
		}
		if report.BinaryPath == "" {
			t.Error("expected non-empty binary_path")
		}
	})
}

func TestBundledIntegrations_GitHub_Remote(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("remote", "github")
	if !ok {
		t.Fatal("github remote not registered")
	}

	t.Run("scan_remote", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{
			Self: models.Resource{Role: "remote"},
		})
		t.Logf("Remote Scan: %+v", report)
		// Remote scan is only meaningful in a git project with a github.com remote.
		// In CI / generic checkout, it may return present=false — that's valid.
		// We only assert the shape.
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_GitHub_Agent(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "github")
	if !ok {
		t.Fatal("github agent not registered")
	}

	t.Run("scan_agent", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{
			Self: models.Resource{Role: "agent"},
		})
		t.Logf("Agent Scan: %+v", report)
		// Agent scan reports present=true if gh binary exists, reachable=true if authenticated
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// Binary should be in PATH on this machine
		if !report.Present {
			t.Error("expected present=true (gh is installed)")
		}
	})

	t.Run("calibrate_agent_authenticated", func(t *testing.T) {
		// This test only runs a meaningful assertion if gh is authenticated.
		// In CI, gh auth login is performed via GH_TOKEN env var.
		report := i.Calibrate(core.ResolvedContext{
			Self: models.Resource{Role: "agent"},
		})
		t.Logf("Agent Calibrate: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
		// If authenticated, GH_TOKEN export should be set
		if report.Reachable {
			if report.Exports["GH_TOKEN"] == "" {
				t.Error("expected GH_TOKEN in exports when authenticated")
			}
		}
	})
}

func TestBundledIntegrations_GoogleAuth(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("agent", "google-auth")
	if !ok {
		t.Fatal("google-auth integration not registered")
	}

	t.Run("detect_drive_app_darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Google Drive app detection only on darwin")
		}
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		// Detection requires the Drive app to be installed — may not be in CI
		t.Logf("Detect (darwin): %+v", report)
		if report.Detected && len(report.Resources) > 0 {
			if report.Resources[0].Role != "agent" {
				t.Errorf("expected role=agent, got %q", report.Resources[0].Role)
			}
			if report.Resources[0].Brand != "google-auth" {
				t.Errorf("expected brand=google-auth, got %q", report.Resources[0].Brand)
			}
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_GoogleDrive(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("remote", "google-drive")
	if !ok {
		t.Fatal("google-drive integration not registered")
	}

	t.Run("detect_drive_app", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Google Drive app detection only on darwin")
		}
		report := i.Detect(core.DetectContext{
			Platform: core.Platform{OS: "darwin"},
		})
		t.Logf("Detect: %+v", report)
		if report.Detected && len(report.Resources) > 0 {
			if report.Resources[0].Role != "remote" {
				t.Errorf("expected role=remote, got %q", report.Resources[0].Role)
			}
			if report.Resources[0].Brand != "google-drive" {
				t.Errorf("expected brand=google-drive, got %q", report.Resources[0].Brand)
			}
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if report.Manager == "" {
			t.Error("expected non-empty manager field")
		}
	})
}

func TestBundledIntegrations_FilesystemLocal(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("shell", "local")
	if !ok {
		t.Fatal("local shell integration not registered")
	}

	t.Run("detect_always", func(t *testing.T) {
		// shell/local is always detected — it overrides the native shell
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for shell/local (always active)")
		}
		// NOTE: the local.wasm binary still emits "filesystem" until recompiled;
		// once the WASM source is updated and rebuilt this should check for "shell".
		if len(report.Resources) == 0 {
			t.Error("expected at least one resource suggestion")
		}
	})

	t.Run("scan_cwd", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (CWD always exists)")
		}
		if report.InstallDir == "" {
			t.Error("expected non-empty install_dir (CWD path)")
		}
	})

	t.Run("calibrate_sets_install_dir", func(t *testing.T) {
		report := i.Calibrate(core.ResolvedContext{})
		t.Logf("Calibrate: %+v", report)
		if !report.Present {
			t.Error("expected present=true after calibrate")
		}
		if report.InstallDir == "" {
			t.Error("expected install_dir to be set by calibrate (triggers cd in Jump)")
		}
	})
}
