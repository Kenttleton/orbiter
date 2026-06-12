-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Global alias registry.
-- Both name and entity orbit ID are globally unique and interchangeable in
-- commands. Entity type is encoded in the orbit ID (chars 9-10) — no
-- separate entity_type column is needed.
CREATE TABLE IF NOT EXISTS aliases (
    name        TEXT PRIMARY KEY,         -- human-readable alias (globally unique)
    entity      TEXT NOT NULL UNIQUE,      -- entity orbit ID (globally unique; type in ID)
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Explicit index on entity for fast reverse lookup (name PRIMARY KEY covers forward).
CREATE INDEX IF NOT EXISTS aliases_entity_idx ON aliases(entity);

-- Single-row table representing the local workstation.
-- Single-row constraint enforced by application logic.
CREATE TABLE IF NOT EXISTS vessel (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS galaxies (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS solar_systems (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    galaxy_id  TEXT NOT NULL REFERENCES aliases(entity),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS planets (
    id              TEXT PRIMARY KEY REFERENCES aliases(entity),
    galaxy_id       TEXT NOT NULL REFERENCES aliases(entity),
    solar_system_id TEXT REFERENCES aliases(entity),
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Callsigns represent the Captain's active identity.
-- Scope is determined by attachments to hierarchy nodes.
CREATE TABLE IF NOT EXISTS callsigns (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Transponders are pointers to credential locations — never the credentials themselves.
-- role is Orbiter-owned (file, env, keychain, vault, agent).
-- brand is integration-owned (any string; validated at init time).
-- config is a JSON object whose shape is role-specific (see docs/integrations.md).
CREATE TABLE IF NOT EXISTS transponders (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    role       TEXT NOT NULL,
    brand      TEXT NOT NULL,
    config     TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Resources represent tooling, runtimes, and capabilities.
-- role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
-- brand is integration-owned (any string; validated at init time).
-- manages is a JSON array of brands this manager controls.
-- config is a JSON object for integration-specific configuration.
CREATE TABLE IF NOT EXISTS resources (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    role       TEXT NOT NULL,
    brand      TEXT NOT NULL,
    manages    TEXT NOT NULL DEFAULT '[]',
    config     TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Defaults store configuration key/value pairs scoped to any entity.
-- Vessel-level defaults include output_format and history_retention_days.
CREATE TABLE IF NOT EXISTS defaults (
    id         TEXT PRIMARY KEY REFERENCES aliases(entity),
    entity_id  TEXT NOT NULL REFERENCES aliases(entity),
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, key)
);

-- Immutable log of navigation events.
-- IDs here are NOT in the alias registry — they are log record IDs only.
CREATE TABLE IF NOT EXISTS navigation_history (
    id             TEXT PRIMARY KEY,
    from_entity_id TEXT REFERENCES aliases(entity),
    to_entity_id   TEXT NOT NULL REFERENCES aliases(entity),
    command        TEXT NOT NULL,
    occurred_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS navigation_history_occurred_idx
    ON navigation_history(occurred_at);

-- Most recent verified observation of an entity. One beacon per entity.
-- IDs here are NOT in the alias registry — they are observation record IDs only.
CREATE TABLE IF NOT EXISTS beacons (
    id           TEXT PRIMARY KEY,
    entity_id    TEXT NOT NULL REFERENCES aliases(entity),
    status       TEXT NOT NULL,
    observations TEXT NOT NULL,
    verified_at  DATETIME NOT NULL,
    UNIQUE(entity_id)
);

-- Attachment graph: directed edges wiring entities together.
-- from_id is the child entity (resource, callsign, transponder).
-- to_id is the parent entity (vessel, galaxy, system, planet, or callsign).
-- IDs here are NOT in the aliases registry.
CREATE TABLE IF NOT EXISTS attachments (
    id         TEXT PRIMARY KEY,
    from_id    TEXT NOT NULL REFERENCES aliases(entity),
    to_id      TEXT NOT NULL REFERENCES aliases(entity),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_id, to_id)
);

INSERT INTO schema_version (version) VALUES (1);
