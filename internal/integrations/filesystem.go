package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// ParsePath converts any path string into a Path carrying all separator forms.
// Accepts any of the four output forms as input: shell-quoted (strips double-quotes),
// Windows absolute (drive letter), Unix absolute (forward-slash), or native OS path.
// Also expands ~ and resolves relative paths to absolute.
// Returns a zero Path and nil error for empty input — empty signals "not found".
func ParsePath(raw string) (Path, error) {
	if raw == "" {
		return Path{}, nil
	}

	// Strip shell quoting — a Shell-form path may arrive wrapped in double-quotes.
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}

	// Tilde expansion.
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Path{}, fmt.Errorf("parse path: resolve home: %w", err)
		}
		if raw == "~" {
			raw = home
		} else {
			raw = home + raw[1:]
		}
	}

	// Windows absolute path: drive letter + colon (e.g. C:\ or C:/).
	// filepath.Abs on a non-Windows host treats this as a relative path and
	// prepends CWD, producing a wrong result. Handle it directly instead.
	if len(raw) >= 3 && raw[1] == ':' {
		c := raw[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return Path{
				Unix:    strings.ReplaceAll(raw, `\`, "/"),
				Windows: strings.ReplaceAll(raw, "/", `\`),
			}, nil
		}
	}

	// For all other forms (Unix absolute, relative, ~-expanded native):
	// filepath.Abs resolves relative paths; ToSlash and ReplaceAll fill both fields.
	abs, err := filepath.Abs(raw)
	if err != nil {
		return Path{}, fmt.Errorf("parse path: %w", err)
	}
	unix := filepath.ToSlash(abs)
	return Path{
		Unix:    unix,
		Windows: strings.ReplaceAll(unix, "/", `\`),
	}, nil
}

// PathFromConfig reads the "path" key from a resource's config JSON and
// returns the parsed form. Returns a zero Path on any failure.
func PathFromConfig(ctx ResolvedContext) Path {
	if ctx.Self == nil {
		return Path{}
	}
	var cfg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(ctx.Self.GetConfig()), &cfg); err != nil {
		return Path{}
	}
	p, err := ParsePath(cfg.Path)
	if err != nil {
		return Path{}
	}
	return p
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
