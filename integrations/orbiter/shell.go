package orbiter

import (
	"encoding/json"
	"os"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

var shellManifest = integrations.Manifest{
	Integration: integrations.ManifestIntegration{
		Brand: "orbiter",
		Roles: []string{integrations.ResourceRoleShell},
	},
}

type shellIntegration struct{}

// NewShell returns a shellIntegration for testing.
func NewShell() integrations.Integration {
	return &shellIntegration{}
}

func (s *shellIntegration) Meta() integrations.Manifest {
	return shellManifest
}

func (s *shellIntegration) Detect(_ integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: false}
}

func (s *shellIntegration) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	path := pathFromSelf(ctx)
	if path == "" {
		return integrations.StateReport{Error: "no path in resource config"}
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return integrations.StateReport{InstallDir: path, Error: err.Error()}
	}
	return integrations.StateReport{Present: true, Reachable: true, InstallDir: path}
}

func (s *shellIntegration) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
	path := pathFromSelf(ctx)
	if path == "" {
		return integrations.StateReport{Error: "no path in resource config"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return integrations.StateReport{Present: false, InstallDir: path}
	}
	return integrations.StateReport{
		Present:    true,
		Reachable:  info.IsDir(),
		InstallDir: path,
	}
}

func (s *shellIntegration) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return s.Init(ctx)
}

func pathFromSelf(ctx integrations.ResolvedContext) string {
	if ctx.Self == nil {
		return ""
	}
	var cfg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(ctx.Self.GetConfig()), &cfg); err != nil {
		return ""
	}
	return cfg.Path
}

func init() {
	integrations.Register(integrations.ResourceRoleShell, "orbiter", &shellIntegration{})
}
