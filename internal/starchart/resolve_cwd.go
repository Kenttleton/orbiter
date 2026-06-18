package starchart

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/models"
)

// ResolveCWD finds the hierarchy entity whose attached shell/orbiter resource
// most specifically matches cwd. Exact match takes priority; among prefix matches,
// the longest matching path prefix wins (CSS selector specificity logic).
// Returns ErrNotFound if no shell resource path matches or prefixes cwd.
func (sc *StarChart) ResolveCWD(ctx context.Context, cwd string) (models.Alias, error) {
	const q = `
        SELECT a.entity, a.name, r.config
        FROM resources r
        JOIN attachments att ON att.from_id = r.id
        JOIN aliases a ON a.entity = att.to_id
        WHERE r.role = 'shell' AND r.brand = 'orbiter'
          AND r.config != '' AND r.config != '{}'
    `
	rows, err := sc.db.QueryContext(ctx, q)
	if err != nil {
		return models.Alias{}, fmt.Errorf("query shell resources: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		models.Alias
		path string
	}
	var best candidate
	bestLen := -1

	for rows.Next() {
		var id, name, configJSON string
		if err := rows.Scan(&id, &name, &configJSON); err != nil {
			return models.Alias{}, fmt.Errorf("scan shell row: %w", err)
		}
		var cfg struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.Path == "" {
			continue
		}
		c := candidate{
			Alias: models.Alias{ID: id, Name: name},
			path:  cfg.Path,
		}
		if c.path == cwd {
			return c.Alias, nil
		}
		prefix := c.path
		if len(prefix) > 0 && prefix[len(prefix)-1] != '/' {
			prefix += "/"
		}
		if len(c.path) > bestLen && len(cwd) >= len(prefix) && cwd[:len(prefix)] == prefix {
			best = c
			bestLen = len(c.path)
		}
	}
	if err := rows.Err(); err != nil {
		return models.Alias{}, fmt.Errorf("iterate shell rows: %w", err)
	}

	if bestLen == -1 {
		return models.Alias{}, fmt.Errorf("%w: no shell resource path matches %q", ErrNotFound, cwd)
	}
	return best.Alias, nil
}
