package git_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	_ "github.com/Kenttleton/orbiter/internal/integrations/git"
)

func TestGitIntegrationE2E(t *testing.T) {
	i, ok := integrations.Default.Get("tool", "git")
	if !ok {
		t.Fatal("git integration not registered")
	}

	t.Run("init", func(t *testing.T) {
		report := i.Init(integrations.ResolvedContext{})
		t.Logf("Init: %+v", report)
		if !report.Present {
			t.Error("expected present=true (git is installed)")
		}
		if !report.Reachable {
			t.Error("expected reachable=true")
		}
	})

	t.Run("detect_hit", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{".git/config": ""},
			CWD:   "/tmp",
		})
		t.Logf("Detect(.git/config): %+v", report)
		if !report.Detected {
			t.Error("expected detected=true for project with .git/config")
		}
	})

	t.Run("detect_miss", func(t *testing.T) {
		report := i.Detect(integrations.DetectContext{
			Files: map[string]string{"package.json": ""},
			CWD:   "/tmp",
		})
		t.Logf("Detect(no .git): %+v", report)
		if report.Detected {
			t.Error("expected detected=false for project without .git/config")
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
