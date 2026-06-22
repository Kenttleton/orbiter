package integrations

import (
	"encoding/json"
	"os"
)

// filesystemIntegration is the built-in implementation of the filesystem role.
// It manages local directory paths: stat for scan, mkdir -p for init/calibrate.
// Orbiter owns this directly — the brand space has no meaningful variation
// (all implementations would just call os.Stat / os.MkdirAll), so it lives
// here rather than as a WASM integration.
type filesystemIntegration struct{}

func (f *filesystemIntegration) Meta() Manifest {
	return Manifest{
		Integration: ManifestIntegration{
			Brand: "orbiter",
			Roles: []string{ResourceRoleFilesystem},
		},
	}
}

func (f *filesystemIntegration) Detect(_ DetectContext) DetectReport {
	return DetectReport{Detected: false}
}

func (f *filesystemIntegration) Init(ctx ResolvedContext) StateReport {
	path := pathFromConfig(ctx)
	if path == "" {
		return StateReport{Error: "no path in resource config"}
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return StateReport{InstallDir: path, Error: err.Error()}
	}
	return StateReport{Present: true, Reachable: true, InstallDir: path}
}

func (f *filesystemIntegration) Scan(ctx ResolvedContext) StateReport {
	path := pathFromConfig(ctx)
	if path == "" {
		return StateReport{Error: "no path in resource config"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return StateReport{Present: false, InstallDir: path}
	}
	return StateReport{
		Present:    true,
		Reachable:  info.IsDir(),
		InstallDir: path,
	}
}

func (f *filesystemIntegration) Calibrate(ctx ResolvedContext) StateReport {
	return f.Init(ctx)
}

func pathFromConfig(ctx ResolvedContext) string {
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
	Default.Register(ResourceRoleFilesystem, "orbiter", &filesystemIntegration{})
}
