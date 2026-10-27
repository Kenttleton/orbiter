package starchart

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// DiscoverPlanet runs all registered integrations' Detect against cwd and returns
// suggested resources. Pre-filters by manifest detection rules before calling WASM.
func (sc *StarChart) DiscoverPlanet(ctx context.Context, cwd string) ([]integrations.SuggestedResource, error) {
	if sc.integrations == nil {
		return nil, nil
	}
	files, err := listFiles(cwd)
	if err != nil {
		return nil, err
	}
	return sc.discover(integrations.DetectContext{
		Platform: currentPlatform(),
		CWD:      cwd,
		Files:    files,
		Env:      osEnvMap(),
	}), nil
}

// DiscoverVessel is the same discovery for the vessel: this machine's environment, with no
// project files. An integration whose manifest rules need files is never asked.
func (sc *StarChart) DiscoverVessel(ctx context.Context) ([]integrations.SuggestedResource, error) {
	if sc.integrations == nil {
		return nil, nil
	}
	cwd, _ := os.Getwd()
	return sc.discover(integrations.DetectContext{
		Platform: currentPlatform(),
		CWD:      cwd,
		Files:    map[string]string{},
		Env:      osEnvMap(),
	}), nil
}

// discover asks every registered integration whose manifest rules match to detect, and
// returns what they suggest. An integration with no rules is always asked: it decides.
func (sc *StarChart) discover(dc integrations.DetectContext) []integrations.SuggestedResource {
	var suggestions []integrations.SuggestedResource
	for _, i := range sc.integrations.All() {
		if !i.Meta().Detection.MatchesAny(dc.Files, dc.Env) {
			continue
		}
		if report := i.Detect(dc); report.Detected {
			suggestions = append(suggestions, report.Resources...)
		}
	}
	return suggestions
}

// osEnvMap parses os.Environ() into a key→value map.
func osEnvMap() map[string]string {
	raw := os.Environ()
	m := make(map[string]string, len(raw))
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

// listFiles returns a map of filename → "" for every file directly in dir.
func listFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files[filepath.Base(e.Name())] = ""
		}
	}
	return files, nil
}
