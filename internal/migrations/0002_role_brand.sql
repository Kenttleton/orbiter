-- Drop tables that have incompatible schemas from migration 0001.
-- No deployed database exists, so this is safe.
DROP TABLE IF EXISTS resources;
DROP TABLE IF EXISTS transponders;
DROP TABLE IF EXISTS callsigns;
DROP TABLE IF EXISTS planets;

-- Recreate planets without repo_url and repo_path.
CREATE TABLE IF NOT EXISTS planets (
    id              TEXT PRIMARY KEY REFERENCES aliases(id),
    galaxy_id       TEXT NOT NULL REFERENCES aliases(id),
    solar_system_id TEXT REFERENCES aliases(id),
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recreate callsigns without entity_id.
-- Scope is now managed via the attachments graph.
CREATE TABLE IF NOT EXISTS callsigns (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recreate transponders with role+brand model.
-- role is Orbiter-owned (file, env, keychain, vault, agent).
-- brand is integration-owned (any string; validated at init time).
CREATE TABLE IF NOT EXISTS transponders (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    role       TEXT NOT NULL,
    brand      TEXT NOT NULL,
    location   TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recreate resources with role+brand model.
-- role is Orbiter-owned (manager, runtime, tool, remote, filesystem).
-- brand is integration-owned (any string; validated at init time).
-- manages is a JSON array of brands this manager controls.
-- config is a JSON object for integration-specific configuration.
CREATE TABLE IF NOT EXISTS resources (
    id         TEXT PRIMARY KEY REFERENCES aliases(id),
    role       TEXT NOT NULL,
    brand      TEXT NOT NULL,
    manages    TEXT NOT NULL DEFAULT '[]',
    config     TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Attachment graph: directed edges wiring entities together.
-- from_id is the child entity; to_id is the parent.
-- IDs here are NOT in the aliases registry.
-- UNIQUE(from_id, to_id) prevents duplicate edges.
CREATE TABLE IF NOT EXISTS attachments (
    id         TEXT PRIMARY KEY,
    from_id    TEXT NOT NULL REFERENCES aliases(id),
    to_id      TEXT NOT NULL REFERENCES aliases(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_id, to_id)
);

INSERT INTO schema_version (version) VALUES (2);
