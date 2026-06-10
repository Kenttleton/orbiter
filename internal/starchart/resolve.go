package starchart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/models"
)

// Resolve looks up an entity by name or ID in the aliases table.
// Name lookup is tried first; if no match, falls back to direct ID lookup.
// Returns ErrNotFound if neither matches.
func (sc *StarChart) Resolve(ctx context.Context, input string) (models.Alias, error) {
	const q = `
		SELECT id, name, entity_type, created_at
		FROM aliases
		WHERE name = ? OR id = ?
		ORDER BY CASE WHEN name = ? THEN 0 ELSE 1 END
		LIMIT 1
	`
	row := sc.db.QueryRowContext(ctx, q, input, input, input)

	var a models.Alias
	err := row.Scan(&a.ID, &a.Name, &a.EntityType, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Alias{}, fmt.Errorf("%w: %q", ErrNotFound, input)
		}
		return models.Alias{}, fmt.Errorf("resolve %q: %w", input, err)
	}
	return a, nil
}
