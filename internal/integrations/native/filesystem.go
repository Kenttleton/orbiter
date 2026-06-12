package native

import (
	"encoding/json"
	"os"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

var filesystemOrbiterManifest = integrations.Manifest{
	Integration: integrations.ManifestIntegration{
		Brand: "orbiter",
		Roles: []string{integrations.ResourceRoleFilesystem},
	},
}

type filesystemOrbiter struct{}

// NewFilesystemOrbiter returns a filesystemOrbiter for testing.
func NewFilesystemOrbiter() integrations.Integration {
	return &filesystemOrbiter{}
}

func (f *filesystemOrbiter) Meta() integrations.Manifest {
	return filesystemOrbiterManifest
}

func (f *filesystemOrbiter) Detect(ctx integrations.DetectContext) integrations.DetectReport {
	return integrations.DetectReport{Detected: false}
}

func (f *filesystemOrbiter) Init(ctx integrations.ResolvedContext) integrations.StateReport {
	path := pathFromSelf(ctx)
	if path == "" {
		return integrations.StateReport{Error: "no path in resource config"}
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return integrations.StateReport{InstallDir: path, Error: err.Error()}
	}
	return integrations.StateReport{Present: true, Reachable: true, InstallDir: path}
}

func (f *filesystemOrbiter) Scan(ctx integrations.ResolvedContext) integrations.StateReport {
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

func (f *filesystemOrbiter) Calibrate(ctx integrations.ResolvedContext) integrations.StateReport {
	return f.Init(ctx)
}

func pathFromSelf(ctx integrations.ResolvedContext) string {
	if ctx.Self == nil {
		return ""
	}
	var cfg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(ctx.Self.Config), &cfg); err != nil {
		return ""
	}
	return cfg.Path
}

func init() {
	integrations.Register(integrations.ResourceRoleFilesystem, "orbiter", &filesystemOrbiter{})
}
