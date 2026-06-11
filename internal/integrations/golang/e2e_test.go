package golang_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	_ "github.com/Kenttleton/orbiter/internal/integrations/golang"
)

func TestGoIntegrationE2E(t *testing.T) {
	i, ok := integrations.Default.Get("runtime", "go")
	if !ok {
		t.Fatal("go integration not registered")
	}

	t.Run("init", func(t *testing.T) {
		report := i.Init(integrations.ResolvedContext{})
		t.Logf("Init: %+v", report)
		if !report.Present {
			t.Error("expected present=true (go is installed)")
		}
		if !report.Reachable {
			t.Error("expected reachable=true")
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"go.mod": "", "go.sum": ""},
			CWD:   "/tmp",
		})
		t.Logf("Detect(go.mod): %+v", report)
		if !report.Detected {
			t.Error("expected detected=true for project with go.mod")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"package.json": ""},
			CWD:   "/tmp",
		})
		t.Logf("Detect(no go.mod): %+v", report)
		if report.Detected {
			t.Error("expected detected=false for project without go.mod")
		}
	})

	t.Run("scan", func(t *testing.T) {
		report := i.Scan(integrations.ResolvedContext{})
		t.Logf("Scan: %+v", report)
		if !report.Present {
			t.Error("expected present=true")
		}
	})
}
