package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
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

// FindBinary resolves a binary name to its absolute path via the filesystem
// integration's path lookup. It delegates to the shell's FIND function defined
// in the Orbiter hook scripts, which is platform-aware (command -v on Unix,
// Get-Command on PowerShell). Falls back to sh/where.exe if FIND is unavailable.
func FindBinary(name, osName string) string {
	if osName == "windows" {
		return findBinaryWindows(name)
	}
	return findBinaryUnix(name)
}

func findBinaryUnix(name string) string {
	shell := os.Getenv("SHELL")

	// bash: hook exports FIND via export -f, available in child processes without -i
	if shell != "" && strings.HasSuffix(shell, "bash") {
		if p := runShellFind(shell, "-c", "FIND "+name); p != "" {
			return p
		}
	}

	// zsh/fish: invoke via profile so FIND is loaded from hook
	if shell != "" {
		flag := "-i"
		if strings.HasSuffix(shell, "fish") {
			flag = "-c"
		}
		if p := runShellFind(shell, flag, "FIND "+name); p != "" {
			return p
		}
	}

	// Fallback: POSIX command -v (no FIND dependency)
	out, err := exec.Command("sh", "-c", "command -v "+name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func findBinaryWindows(name string) string {
	// PowerShell profile defines FIND using Get-Command
	cmd := exec.Command("pwsh", "-NoLogo", "-NonInteractive", "-Command",
		". $PROFILE; FIND "+name)
	cmd.Stderr = nil
	if out, err := cmd.Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			return p
		}
	}
	// Fallback: where.exe
	out, err := exec.Command("where.exe", name).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[0])
}

func runShellFind(shell, flag, expr string) string {
	cmd := exec.Command(shell, flag, expr)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func init() {
	Default.Register(ResourceRoleFilesystem, "orbiter", &filesystemIntegration{})
}
