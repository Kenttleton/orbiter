package starchart

import (
	"context"
	"fmt"

	"github.com/Kenttleton/orbiter/internal/models"
)

// RetroNode describes one entity in a retro plan.
type RetroNode struct {
	EntityID string
	Name     string
	Action   string // "retire" or "detach"
}

// RetroPlan is the computed cascade retire plan for a target entity.
type RetroPlan struct {
	TargetID   string
	TargetName string
	Nodes      []RetroNode
}

// PlanRetro computes the cascade retire plan for targetID.
// Collects all descendants; shared nodes (attached outside the retire set) get Action="detach".
// Unshared nodes get Action="retire".
func (sc *StarChart) PlanRetro(ctx context.Context, targetID string) (RetroPlan, error) {
	targetName, err := sc.aliasOf(ctx, targetID)
	if err != nil {
		return RetroPlan{}, fmt.Errorf("resolve target: %w", err)
	}

	subtree, err := sc.collectSubtree(ctx, targetID)
	if err != nil {
		return RetroPlan{}, err
	}

	retireSet := make(map[string]bool, len(subtree))
	for _, id := range subtree {
		retireSet[id] = true
	}

	plan := RetroPlan{TargetID: targetID, TargetName: targetName}
	for _, entityID := range subtree {
		name, _ := sc.aliasOf(ctx, entityID)
		shared, err := sc.hasAttachmentOutside(ctx, entityID, retireSet)
		if err != nil {
			return RetroPlan{}, err
		}
		action := "retire"
		if shared {
			action = "detach"
		}
		plan.Nodes = append(plan.Nodes, RetroNode{EntityID: entityID, Name: name, Action: action})
	}
	return plan, nil
}

// ExecuteRetro executes a RetroPlan in a single transaction.
// For each node:
//   - "retire": sets beacon to retired, deletes the entity row and its alias.
//   - "detach": removes all attachment edges from this node into the retire set.
func (sc *StarChart) ExecuteRetro(ctx context.Context, plan RetroPlan) error {
	// Build retire set for detach lookups.
	retireSet := make(map[string]bool, len(plan.Nodes))
	for _, n := range plan.Nodes {
		if n.Action == "retire" {
			retireSet[n.EntityID] = true
		}
	}

	return sc.Tx(ctx, func(t *Tx) error {
		for _, node := range plan.Nodes {
			switch node.Action {
			case "retire":
				// 1. Delete all attachment edges involving this entity.
				if _, err := t.tx.ExecContext(ctx,
					`DELETE FROM attachments WHERE from_id = ? OR to_id = ?`,
					node.EntityID, node.EntityID,
				); err != nil {
					return fmt.Errorf("delete attachments for %s: %w", node.EntityID, err)
				}

				// 2. Mark beacon as retired, then delete its row.
				if _, err := t.tx.ExecContext(ctx,
					`UPDATE beacons SET status = 'retired' WHERE entity_id = ?`,
					node.EntityID,
				); err != nil {
					return fmt.Errorf("mark beacon retired for %s: %w", node.EntityID, err)
				}
				if _, err := t.tx.ExecContext(ctx,
					`DELETE FROM beacons WHERE entity_id = ?`,
					node.EntityID,
				); err != nil {
					return fmt.Errorf("delete beacon for %s: %w", node.EntityID, err)
				}

				// 3. Delete the entity row from its table.
				table, err := tableForEntity(node.EntityID)
				if err != nil {
					return err
				}
				if err := t.Delete(ctx, table, node.EntityID); err != nil {
					return fmt.Errorf("delete entity %s from %s: %w", node.EntityID, table, err)
				}

				// 4. Delete the alias row (keyed on entity column, not id).
				if _, err := t.tx.ExecContext(ctx,
					`DELETE FROM aliases WHERE entity = ?`,
					node.EntityID,
				); err != nil {
					return fmt.Errorf("delete alias for %s: %w", node.EntityID, err)
				}

			case "detach":
				// Remove all edges from this node into the retire set.
				for toID := range retireSet {
					if _, err := t.tx.ExecContext(ctx,
						`DELETE FROM attachments WHERE from_id = ? AND to_id = ?`,
						node.EntityID, toID,
					); err != nil {
						return fmt.Errorf("detach %s from %s: %w", node.EntityID, toID, err)
					}
				}
			}
		}
		return nil
	})
}

// collectSubtree returns targetID plus all descendants in post-order (leaves first,
// target last). This ordering is required so ExecuteRetro deletes children before
// parents, satisfying FK constraints (e.g. planets reference galaxies via galaxy_id).
//
// Two kinds of children are traversed:
//  1. Attachment children: entities attached to id via the attachments table
//     (resources, transponders — from_id = child, to_id = parent).
//  2. Hierarchy children: structural children via FK columns
//     (solar_systems.galaxy_id, planets.galaxy_id, planets.solar_system_id).
func (sc *StarChart) collectSubtree(ctx context.Context, entityID string) ([]string, error) {
	visited := make(map[string]bool)
	var order []string

	var walk func(id string) error
	walk = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true

		// Attachment children (resources/transponders attached to this entity).
		attachRows, err := sc.db.QueryContext(ctx,
			`SELECT from_id FROM attachments WHERE to_id = ?`, id,
		)
		if err != nil {
			return fmt.Errorf("query attachment children of %s: %w", id, err)
		}
		var children []string
		for attachRows.Next() {
			var childID string
			if err := attachRows.Scan(&childID); err != nil {
				attachRows.Close()
				return err
			}
			children = append(children, childID)
		}
		attachRows.Close()
		if err := attachRows.Err(); err != nil {
			return err
		}

		// Hierarchy children (galaxy→systems, galaxy→planets, system→planets).
		hierarchyKids, err := sc.hierarchyChildrenOf(ctx, id)
		if err != nil {
			return err
		}
		children = append(children, hierarchyKids...)

		// Recurse children first (post-order = leaf before parent).
		for _, child := range children {
			if err := walk(child); err != nil {
				return err
			}
		}

		order = append(order, id)
		return nil
	}

	if err := walk(entityID); err != nil {
		return nil, err
	}
	return order, nil
}

// hierarchyChildrenOf returns the structural (FK-column) children of entityID.
// Galaxy → solar_systems + planets; SolarSystem → planets; others → none.
func (sc *StarChart) hierarchyChildrenOf(ctx context.Context, entityID string) ([]string, error) {
	if len(entityID) < 10 {
		return nil, nil
	}
	var ids []string
	switch entityID[8:10] {
	case models.EntityTypeGalaxy:
		sysIDs, err := sc.childIDsWhere(ctx, "solar_systems", "galaxy_id", entityID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, sysIDs...)
		planetIDs, err := sc.childIDsWhere(ctx, "planets", "galaxy_id", entityID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, planetIDs...)
	case models.EntityTypeSolarSystem:
		planetIDs, err := sc.childIDsWhere(ctx, "planets", "solar_system_id", entityID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, planetIDs...)
	}
	return ids, nil
}

// childIDsWhere returns the IDs of all rows in table where col = val.
// Table and column names are hardcoded call sites — not user input.
func (sc *StarChart) childIDsWhere(ctx context.Context, table, col, val string) ([]string, error) {
	rows, err := sc.db.QueryContext(ctx,
		`SELECT id FROM `+table+` WHERE `+col+` = ?`, val,
	)
	if err != nil {
		return nil, fmt.Errorf("query %s.%s=%s: %w", table, col, val, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// hasAttachmentOutside returns true if entityID is attached to any entity NOT in retireSet.
// We check both directions: entityID as from_id (attached to something outside) OR
// entityID as to_id (something outside is attached to it).
func (sc *StarChart) hasAttachmentOutside(ctx context.Context, entityID string, retireSet map[string]bool) (bool, error) {
	rows, err := sc.db.QueryContext(ctx,
		`SELECT from_id, to_id FROM attachments WHERE from_id = ? OR to_id = ?`,
		entityID, entityID,
	)
	if err != nil {
		return false, fmt.Errorf("query attachments for %s: %w", entityID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			return false, err
		}
		// The "other" side of the edge.
		other := toID
		if fromID == entityID {
			other = toID
		} else {
			other = fromID
		}
		if !retireSet[other] {
			return true, nil
		}
	}
	return false, rows.Err()
}

// aliasOf returns the alias name for entityID, or entityID itself if not found.
func (sc *StarChart) aliasOf(ctx context.Context, entityID string) (string, error) {
	var name string
	err := sc.db.QueryRowContext(ctx,
		`SELECT name FROM aliases WHERE entity = ?`, entityID,
	).Scan(&name)
	if err != nil {
		return entityID, nil //nolint:nilerr // fallback to ID when no alias exists
	}
	return name, nil
}

// tableForEntity maps an OrbitID's entity type bits (chars 8–9) to the table name.
func tableForEntity(entityID string) (string, error) {
	if len(entityID) < 10 {
		return "", fmt.Errorf("tableForEntity: id %q too short", entityID)
	}
	switch entityID[8:10] {
	case models.EntityTypeGalaxy:
		return "galaxies", nil
	case models.EntityTypeSolarSystem:
		return "solar_systems", nil
	case models.EntityTypePlanet:
		return "planets", nil
	case models.EntityTypeCallsign:
		return "callsigns", nil
	case models.EntityTypeTransponder:
		return "transponders", nil
	case models.EntityTypeResource:
		return "resources", nil
	default:
		return "", fmt.Errorf("tableForEntity: unknown entity type %q in id %q", entityID[8:10], entityID)
	}
}
