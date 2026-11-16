package starchart

import (
	"context"

	"github.com/Kenttleton/orbiter/internal/models"
)

// Node is one hierarchy entity (vessel, galaxy, solar system or planet) with its alias name.
type Node struct {
	ID   string
	Type string // models.EntityType* code
	Name string
}

// AttachedResource, AttachedTransponder and AttachedCallsign are what is wired
// directly to a node (attachments, not inherited through the hierarchy).
type AttachedResource struct {
	ID, Name, Role, Brand, Manages, Config string
}

type AttachedTransponder struct {
	ID, Name, Role, Brand, Config string
}

type AttachedCallsign struct {
	ID, Name string
}

// Attached groups the entities attached directly to one node.
type Attached struct {
	Resources    []AttachedResource
	Transponders []AttachedTransponder
	Callsigns    []AttachedCallsign
}

// ChildrenOf returns the hierarchy children of id, ordered by name: a vessel's
// galaxies; a galaxy's solar systems and the planets that sit directly in it; a
// solar system's planets. Planets and other entity types have none.
func (sc *StarChart) ChildrenOf(ctx context.Context, id string) ([]Node, error) {
	parsed, err := models.ParseID(id)
	if err != nil {
		return nil, err
	}
	var out []Node
	add := func(entityType, query string, args ...any) error {
		rows, err := sc.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			n := Node{Type: entityType}
			if err := rows.Scan(&n.ID, &n.Name); err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	}
	switch parsed.EntityType {
	case models.EntityTypeVessel:
		err = add(models.EntityTypeGalaxy,
			`SELECT g.id, a.name FROM galaxies g JOIN aliases a ON a.entity = g.id ORDER BY a.name`)
	case models.EntityTypeGalaxy:
		if err = add(models.EntityTypeSolarSystem,
			`SELECT s.id, a.name FROM solar_systems s JOIN aliases a ON a.entity = s.id
			 WHERE s.galaxy_id = ? ORDER BY a.name`, id); err == nil {
			err = add(models.EntityTypePlanet,
				`SELECT p.id, a.name FROM planets p JOIN aliases a ON a.entity = p.id
				 WHERE p.galaxy_id = ? AND COALESCE(p.solar_system_id, '') = '' ORDER BY a.name`, id)
		}
	case models.EntityTypeSolarSystem:
		err = add(models.EntityTypePlanet,
			`SELECT p.id, a.name FROM planets p JOIN aliases a ON a.entity = p.id
			 WHERE p.solar_system_id = ? ORDER BY a.name`, id)
	}
	return out, err
}

// AttachedTo returns the resources, transponders and callsigns attached directly
// to nodeID, ordered by name.
func (sc *StarChart) AttachedTo(ctx context.Context, nodeID string) (Attached, error) {
	var out Attached

	rows, err := sc.db.QueryContext(ctx,
		`SELECT r.id, a2.name, r.role, r.brand, COALESCE(r.manages,''), COALESCE(r.config,'')
		 FROM resources r
		 JOIN attachments at ON at.from_id = r.id
		 JOIN aliases a2 ON a2.entity = r.id
		 WHERE at.to_id = ? ORDER BY a2.name`, nodeID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r AttachedResource
		if err := rows.Scan(&r.ID, &r.Name, &r.Role, &r.Brand, &r.Manages, &r.Config); err != nil {
			rows.Close()
			return out, err
		}
		out.Resources = append(out.Resources, r)
	}
	rows.Close()

	rows, err = sc.db.QueryContext(ctx,
		`SELECT t.id, a2.name, t.role, t.brand, COALESCE(t.config,'')
		 FROM transponders t
		 JOIN attachments at ON at.from_id = t.id
		 JOIN aliases a2 ON a2.entity = t.id
		 WHERE at.to_id = ? ORDER BY a2.name`, nodeID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var t AttachedTransponder
		if err := rows.Scan(&t.ID, &t.Name, &t.Role, &t.Brand, &t.Config); err != nil {
			rows.Close()
			return out, err
		}
		out.Transponders = append(out.Transponders, t)
	}
	rows.Close()

	rows, err = sc.db.QueryContext(ctx,
		`SELECT c.id, a2.name
		 FROM callsigns c
		 JOIN attachments at ON at.from_id = c.id
		 JOIN aliases a2 ON a2.entity = c.id
		 WHERE at.to_id = ? ORDER BY a2.name`, nodeID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c AttachedCallsign
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return out, err
		}
		out.Callsigns = append(out.Callsigns, c)
	}
	return out, rows.Err()
}

// CountUsing returns how many resources and transponders in the tree are of the given brand,
// that is, how many things use that integration.
func (sc *StarChart) CountUsing(ctx context.Context, brand string) (int, error) {
	var n int
	err := sc.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM resources WHERE brand = ?)
		      + (SELECT COUNT(*) FROM transponders WHERE brand = ?)`, brand, brand).Scan(&n)
	return n, err
}
