package starchart

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Kenttleton/orbiter/internal/integrations"
)

// DiscoverPlanet runs all registered integrations' Detect against cwd and returns
// suggested resources. The caller is responsible for adding and attaching them.
func (sc *StarChart) DiscoverPlanet(ctx context.Context, cwd string) ([]integrations.SuggestedResource, error) {
	if sc.integrations == nil {
		return nil, nil
	}

	files, err := listFiles(cwd)
	if err != nil {
		return nil, err
	}

	dc := integrations.DetectContext{
		Platform: currentPlatform(),
		CWD:      cwd,
		Files:    files,
	}

	var suggestions []integrations.SuggestedResource
	for _, i := range sc.integrations.All() {
		report := i.Detect(dc)
		if report.Detected {
			suggestions = append(suggestions, report.Resources...)
		}
	}
	return suggestions, nil
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
