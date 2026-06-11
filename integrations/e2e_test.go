package integrations_test

import (
	"testing"

	_ "github.com/Kenttleton/orbiter/integrations"
	"github.com/Kenttleton/orbiter/internal/integrations"
)

func TestBundledIntegrations_Git(t *testing.T) {
	i, ok := integrations.Default.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(integrations.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true (git is installed)")
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{".git/config": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for .git/config")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"package.json": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without .git/config")
		}
	})
}

func TestBundledIntegrations_Go(t *testing.T) {
	i, ok := integrations.Default.Get("runtime", "go")
	if !ok {
		t.Fatal("go integration not registered")
	}

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(integrations.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"go.mod": "", "go.sum": ""},
		})
		if !report.Detected {
			t.Error("expected detected=true for go.mod")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"package.json": ""},
		})
		if report.Detected {
			t.Error("expected detected=false without go.mod")
		}
	})
}
