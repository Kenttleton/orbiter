package starchart

import (
	"context"
	"fmt"
	"time"

	"github.com/Kenttleton/orbiter/internal/models"
)

// GetVessel returns the single vessel record.
func (sc *StarChart) GetVessel(ctx context.Context) (models.Vessel, error) {
	var vessels []models.Vessel
	if err := sc.List(ctx, "vessel", &vessels); err != nil {
		return models.Vessel{}, err
	}
	if len(vessels) == 0 {
		return models.Vessel{}, ErrNotFound
	}
	return vessels[0], nil
}

// Attach creates a directed graph edge from fromName to toName.
// "vessel" resolves to the vessel entity without requiring an alias lookup.
//
// One-callsign-per-node: if fromName resolves to a callsign, the target node
// must not already have a callsign attached.
func (sc *StarChart) Attach(ctx context.Context, fromName, toName string) (models.Attachment, error) {
	from, err := sc.Resolve(ctx, fromName)
	if err != nil {
		return models.Attachment{}, fmt.Errorf("resolve %q: %w", fromName, err)
	}

	toID, err := sc.resolveAttachTarget(ctx, toName)
	if err != nil {
		return models.Attachment{}, fmt.Errorf("resolve %q: %w", toName, err)
	}

	if from.EntityType == models.EntityTypeCallsign {
		if err := sc.guardOneCallsign(ctx, toID); err != nil {
			return models.Attachment{}, err
		}
	}

	att := models.Attachment{
		ID:        models.NewID(models.EntityTypeAttachment),
		FromID:    from.ID,
		ToID:      toID,
		CreatedAt: time.Now().UTC(),
	}
	if err := sc.Insert(ctx, "attachments", att); err != nil {
		return models.Attachment{}, fmt.Errorf("create attachment: %w", err)
	}
	return att, nil
}

// resolveAttachTarget resolves the "to" side of an attachment.
// "vessel" is a reserved name that bypasses the alias table.
func (sc *StarChart) resolveAttachTarget(ctx context.Context, name string) (string, error) {
	if name == "vessel" {
		v, err := sc.GetVessel(ctx)
		if err != nil {
			return "", err
		}
		return v.ID, nil
	}
	a, err := sc.Resolve(ctx, name)
	if err != nil {
		return "", err
	}
	return a.ID, nil
}

// guardOneCallsign returns an error if toID already has a callsign attached.
func (sc *StarChart) guardOneCallsign(ctx context.Context, toID string) error {
	const q = `
        SELECT COUNT(*) FROM attachments a
        JOIN aliases al ON al.id = a.from_id
        WHERE a.to_id = ? AND al.entity_type = ?
    `
	row := sc.db.QueryRowContext(ctx, q, toID, models.EntityTypeCallsign)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check existing callsigns: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("node already has a callsign attached — detach it first via orbit retro")
	}
	return nil
}
