-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Global OrbitID registry and alias table.
-- Every entity is registered here when created.
-- name defaults to id when no human-readable alias is given.
-- Uniqueness of name is enforced globally across all entity types.
CREATE TABLE IF NOT EXISTS aliases (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    entity_type TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Single-row table representing the local workstation.
-- Single-row constraint enforced by application logic.
CREATE TABLE IF NOT EXISTS vessel (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS galaxies (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS solar_systems (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS planets (
    id              TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id       TEXT NOT NULL REFERENCES aliases(id),
    solar_system_id TEXT REFERENCES aliases(id),
    repo_url        TEXT,
    repo_path       TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Callsigns represent the Captain's active identity.
-- Scoped to a vessel or galaxy via entity_id.
CREATE TABLE IF NOT EXISTS callsigns (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Transponders are pointers to credential locations — never the credentials themselves.
-- Always linked to a callsign. Optionally narrowed to a specific entity.
CREATE TABLE IF NOT EXISTS transponders (
    id           TEXT PRIMARY KEY REFERENCES aliases(id),
    callsign_id  TEXT NOT NULL REFERENCES aliases(id),
    entity_id    TEXT REFERENCES aliases(id),
    service      TEXT NOT NULL,
    location     TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Resources represent tooling, runtimes, and capabilities.
-- Scoped to any entity via entity_id.
CREATE TABLE IF NOT EXISTS resources (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    kind       TEXT NOT NULL,
    manager    TEXT,
    version    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Defaults store configuration key/value pairs scoped to any entity.
-- Vessel-level defaults include output_format and history_retention_days.
CREATE TABLE IF NOT EXISTS defaults (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    entity_id  TEXT NOT NULL REFERENCES aliases(id),
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, key)
);

-- Immutable log of navigation events.
-- IDs here are NOT in the alias registry — they are log record IDs only.
CREATE TABLE IF NOT EXISTS navigation_history (
    id             TEXT PRIMARY KEY,
    from_entity_id TEXT REFERENCES aliases(id),
    to_entity_id   TEXT NOT NULL REFERENCES aliases(id),
    command        TEXT NOT NULL,
    occurred_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS navigation_history_occurred_idx
    ON navigation_history(occurred_at);

-- Most recent verified observation of an entity. One beacon per entity.
-- IDs here are NOT in the alias registry — they are observation record IDs only.
CREATE TABLE IF NOT EXISTS beacons (
    id           TEXT PRIMARY KEY,
    entity_id    TEXT NOT NULL REFERENCES aliases(id),
    status       TEXT NOT NULL,
    observations TEXT NOT NULL,
    verified_at  DATETIME NOT NULL,
    UNIQUE(entity_id)
);

INSERT INTO schema_version (version) VALUES (1);
