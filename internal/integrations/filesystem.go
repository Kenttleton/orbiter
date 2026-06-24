package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

