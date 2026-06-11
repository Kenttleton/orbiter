package starchart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/models"
)

// Resolve looks up an entity by alias name or entity orbit ID.
// Name lookup is tried first; if no match, falls back to entity ID lookup.
// Returns ErrNotFound if neither matches.
func (sc *StarChart) Resolve(ctx context.Context, input string) (models.Alias, error) {
	const q = `
		SELECT entity, name, created_at
		FROM aliases
		WHERE name = ? OR entity = ?
		ORDER BY CASE WHEN name = ? THEN 0 ELSE 1 END
		LIMIT 1
	`
	row := sc.db.QueryRowContext(ctx, q, input, input, input)

	var a models.Alias
	err := row.Scan(&a.ID, &a.Name, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Alias{}, fmt.Errorf("%w: %q", ErrNotFound, input)
		}
		return models.Alias{}, fmt.Errorf("resolve %q: %w", input, err)
	}
	return a, nil
}
