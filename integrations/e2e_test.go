package integrations_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
	core "github.com/Kenttleton/orbiter/internal/integrations"
)

func setupBundleRegistry(t *testing.T) *core.Registry {
	t.Helper()
	reg := core.NewRegistry(nil)
	if err := integrations.InstallSelected(integrations.CatalogEntries(), reg); err != nil {
		t.Fatalf("InstallSelected: %v", err)
	}
	return reg
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
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{".git/config": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .git/config")
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

func TestBundledIntegrations_Go(t *testing.T) {
	reg := setupBundleRegistry(t)
	i, ok := reg.Get("runtime", "go")
	if !ok {
		t.Fatal("go integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(core.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(core.DetectContext{
			Files: map[string]string{"go.mod": "", "go.sum": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for go.mod")
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
