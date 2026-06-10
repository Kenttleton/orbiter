# Phase 1B: Star Chart Layer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `internal/starchart` package — SQLite connection with migration runner, generic CRUD methods (Insert/Get/List/Update/Delete), the Tx pipeline (Prepare → Validate → Execute → Verify → Commit), and the optimized Resolve method for alias → ID lookup.

**Architecture:** The starchart package owns the database connection and all query logic. Generic CRUD uses struct reflection on `db` tags. The Tx wrapper enforces the Star Chart integrity pipeline from the constitution. Resolve is a single query that checks `aliases.name` first, falls back to direct ID match — it is the only optimized query in this layer. All tests use `t.TempDir()` with a real SQLite file.

**Tech Stack:** `modernc.org/sqlite`, `database/sql`, `go:embed`, `github.com/stretchr/testify`

**Prerequisite:** Plan 1A must be complete. `internal/models` and `internal/migrations` packages must exist.

> Replace `github.com/Kenttleton/orbiter` with your actual module path throughout.

---

## File Map

| File | Purpose |
|---|---|
| `internal/starchart/db.go` | `Open`, `Close`, `SchemaVersion` — connection lifecycle |
| `internal/starchart/migrate.go` | Migration runner — applies pending SQL migrations |
| `internal/starchart/reflect.go` | Reflection helpers for struct ↔ SQL mapping |
| `internal/starchart/crud.go` | `Filter`, `Insert`, `Get`, `List`, `Update`, `Delete` |
| `internal/starchart/tx.go` | `Tx` wrapper — Prepare/Validate/Execute/Verify/Commit pipeline |
| `internal/starchart/resolve.go` | `Resolve` — optimized alias → ID lookup |
| `internal/starchart/db_test.go` | Tests for Open, Close, SchemaVersion |
| `internal/starchart/migrate_test.go` | Tests for migration idempotency |
| `internal/starchart/crud_test.go` | Tests for all CRUD operations |
| `internal/starchart/tx_test.go` | Tests for Tx commit and rollback |
| `internal/starchart/resolve_test.go` | Tests for Resolve by name and ID |

---

### Task 1: Add SQLite dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the SQLite driver**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Verify it appears in go.mod**

```bash
grep 'modernc.org/sqlite' go.mod
```

Expected: a `require` entry with a version number.

- [ ] **Step 3: Commit**

```
git add go.mod go.sum
git commit -m "chore: add modernc.org/sqlite pure-Go driver"
```

---

### Task 2: Implement DB open and migration runner (TDD)

**Files:**
- Create: `internal/starchart/db.go`
- Create: `internal/starchart/migrate.go`
- Create: `internal/starchart/db_test.go`
- Create: `internal/starchart/migrate_test.go`

- [ ] **Step 1: Write failing tests for Open**

Create `internal/starchart/db_test.go`:

```go
package starchart_test

import (
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")
    sc, err := starchart.Open(path)
    require.NoError(t, err)
    require.NotNil(t, sc)
    sc.Close()
}

func TestOpenCreatesParentDirectory(t *testing.T) {
    path := filepath.Join(t.TempDir(), "subdir", "nested", "test.db")
    sc, err := starchart.Open(path)
    require.NoError(t, err)
    sc.Close()
}

func TestOpenAppliesMigrations(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")
    sc, err := starchart.Open(path)
    require.NoError(t, err)
    defer sc.Close()

    version, err := sc.SchemaVersion()
    require.NoError(t, err)
    require.Equal(t, 1, version)
}

func TestOpenIdempotent(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")

    sc1, err := starchart.Open(path)
    require.NoError(t, err)
    sc1.Close()

    // Opening the same file again must not fail or double-apply migrations.
    sc2, err := starchart.Open(path)
    require.NoError(t, err)
    defer sc2.Close()

    version, err := sc2.SchemaVersion()
    require.NoError(t, err)
    require.Equal(t, 1, version)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/starchart/... 2>&1 | head -20
```

Expected: compile error — `starchart` package does not exist yet.

- [ ] **Step 3: Implement db.go**

Create `internal/starchart/db.go`:

```go
package starchart

import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "path/filepath"

    _ "modernc.org/sqlite"
)

// StarChart wraps the SQLite connection for the Star Chart database.
type StarChart struct {
    db *sql.DB
}

// Open opens or creates the Star Chart database at path, creating parent
// directories as needed, and applies any pending migrations.
func Open(path string) (*StarChart, error) {
    if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
        return nil, fmt.Errorf("create starchart directory: %w", err)
    }

    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("open starchart: %w", err)
    }

    // SQLite disables foreign key enforcement by default.
    if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
        db.Close()
        return nil, fmt.Errorf("enable foreign keys: %w", err)
    }

    sc := &StarChart{db: db}
    if err := sc.migrate(context.Background()); err != nil {
        db.Close()
        return nil, fmt.Errorf("run migrations: %w", err)
    }

    return sc, nil
}

// Close closes the underlying database connection.
func (sc *StarChart) Close() error {
    return sc.db.Close()
}

// SchemaVersion returns the highest applied migration version.
// Returns 0 if no migrations have been applied.
func (sc *StarChart) SchemaVersion() (int, error) {
    var version int
    row := sc.db.QueryRowContext(
        context.Background(),
        "SELECT COALESCE(MAX(version), 0) FROM schema_version",
    )
    if err := row.Scan(&version); err != nil {
        return 0, fmt.Errorf("query schema version: %w", err)
    }
    return version, nil
}
```

- [ ] **Step 4: Implement migrate.go**

Create `internal/starchart/migrate.go`:

```go
package starchart

import (
    "context"
    "fmt"
    "io/fs"
    "sort"
    "strings"

    "github.com/Kenttleton/orbiter/internal/migrations"
)

func (sc *StarChart) migrate(ctx context.Context) error {
    // Ensure schema_version table exists before querying it.
    _, err := sc.db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS schema_version (
            version    INTEGER PRIMARY KEY,
            applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        return fmt.Errorf("create schema_version: %w", err)
    }

    applied, err := sc.appliedVersions(ctx)
    if err != nil {
        return err
    }

    files, err := fs.Glob(migrations.FS, "*.sql")
    if err != nil {
        return fmt.Errorf("list migration files: %w", err)
    }
    sort.Strings(files)

    for _, name := range files {
        version := migrationVersion(name)
        if version == 0 {
            continue
        }
        if applied[version] {
            continue
        }
        data, err := migrations.FS.ReadFile(name)
        if err != nil {
            return fmt.Errorf("read migration %s: %w", name, err)
        }
        if err := sc.applyMigration(ctx, version, string(data)); err != nil {
            return fmt.Errorf("apply migration %s: %w", name, err)
        }
    }
    return nil
}

func (sc *StarChart) appliedVersions(ctx context.Context) (map[int]bool, error) {
    rows, err := sc.db.QueryContext(ctx, "SELECT version FROM schema_version")
    if err != nil {
        return nil, fmt.Errorf("query applied versions: %w", err)
    }
    defer rows.Close()

    applied := make(map[int]bool)
    for rows.Next() {
        var v int
        if err := rows.Scan(&v); err != nil {
            return nil, err
        }
        applied[v] = true
    }
    return applied, rows.Err()
}

func (sc *StarChart) applyMigration(ctx context.Context, version int, sql string) error {
    tx, err := sc.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    if _, err := tx.ExecContext(ctx, sql); err != nil {
        tx.Rollback()
        return fmt.Errorf("execute migration: %w", err)
    }
    if _, err := tx.ExecContext(ctx,
        "INSERT INTO schema_version (version) VALUES (?)", version,
    ); err != nil {
        tx.Rollback()
        return fmt.Errorf("record migration version: %w", err)
    }
    return tx.Commit()
}

// migrationVersion parses the version number from a filename like "0001_initial.sql".
// Returns 0 if the filename doesn't match the expected format.
func migrationVersion(name string) int {
    parts := strings.SplitN(name, "_", 2)
    if len(parts) < 1 {
        return 0
    }
    var v int
    fmt.Sscanf(parts[0], "%d", &v)
    return v
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/starchart/... -run TestOpen
```

Expected: all `TestOpen*` tests PASS.

- [ ] **Step 6: Write migration idempotency test**

Create `internal/starchart/migrate_test.go`:

```go
package starchart_test

import (
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func TestMigrateIdempotent(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")

    for i := 0; i < 3; i++ {
        sc, err := starchart.Open(path)
        require.NoError(t, err, "open attempt %d", i)

        v, err := sc.SchemaVersion()
        require.NoError(t, err)
        require.Equal(t, 1, v)
        sc.Close()
    }
}
```

- [ ] **Step 7: Run migration tests**

```bash
go test ./internal/starchart/... -run TestMigrate
```

Expected: PASS

- [ ] **Step 8: Commit**

```
git add internal/starchart/db.go internal/starchart/migrate.go \
        internal/starchart/db_test.go internal/starchart/migrate_test.go
git commit -m "feat: implement starchart Open with migration runner"
```

---

### Task 3: Implement reflection helpers

**Files:**
- Create: `internal/starchart/reflect.go`

These helpers are internal to the `starchart` package — not exported. They drive the generic CRUD methods by mapping `db` struct tags to SQL column names and scan destinations.

- [ ] **Step 1: Write reflect.go**

Create `internal/starchart/reflect.go`:

```go
package starchart

import (
    "database/sql"
    "fmt"
    "reflect"
    "strings"
)

// scanner abstracts *sql.Row and *sql.Rows for reflectScan.
type scanner interface {
    Scan(dest ...any) error
}

// reflectInsertFields extracts db-tagged field names, "?" placeholders, and
// values from a struct (or pointer to struct). Fields with tag "-" are skipped.
func reflectInsertFields(record any) (cols, placeholders []string, vals []any) {
    v := reflect.ValueOf(record)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    t := v.Type()
    for i := 0; i < t.NumField(); i++ {
        tag := t.Field(i).Tag.Get("db")
        if tag == "" || tag == "-" {
            continue
        }
        cols = append(cols, tag)
        placeholders = append(placeholders, "?")
        vals = append(vals, v.Field(i).Interface())
    }
    return
}

// reflectSelectCols returns the ordered list of db-tagged column names for a
// struct (or pointer to struct).
func reflectSelectCols(record any) []string {
    v := reflect.ValueOf(record)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    t := v.Type()
    var cols []string
    for i := 0; i < t.NumField(); i++ {
        tag := t.Field(i).Tag.Get("db")
        if tag == "" || tag == "-" {
            continue
        }
        cols = append(cols, tag)
    }
    return cols
}

// reflectScan scans a SQL row into a struct using db tags.
// dest must be a pointer to a struct.
func reflectScan(row scanner, dest any) error {
    v := reflect.ValueOf(dest)
    if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
        return fmt.Errorf("reflectScan: dest must be a pointer to a struct")
    }
    v = v.Elem()
    t := v.Type()
    var ptrs []any
    for i := 0; i < t.NumField(); i++ {
        tag := t.Field(i).Tag.Get("db")
        if tag == "" || tag == "-" {
            continue
        }
        ptrs = append(ptrs, v.Field(i).Addr().Interface())
    }
    return row.Scan(ptrs...)
}

// buildWhere constructs a WHERE clause and its argument list from filters.
// Returns empty string and nil if no filters are provided.
func buildWhere(filters []Filter) (string, []any) {
    if len(filters) == 0 {
        return "", nil
    }
    var conditions []string
    var vals []any
    for _, f := range filters {
        conditions = append(conditions, fmt.Sprintf("%s %s ?", f.Column, f.Op))
        vals = append(vals, f.Value)
    }
    return " WHERE " + strings.Join(conditions, " AND "), vals
}

// nullableString is a helper for scanning nullable TEXT columns into Go strings.
// An empty string is stored as NULL and scanned back as "".
type nullableString struct {
    s *string
}

func (n nullableString) Scan(value any) error {
    if value == nil {
        return nil
    }
    ns := sql.NullString{}
    if err := ns.Scan(value); err != nil {
        return err
    }
    if ns.Valid {
        *n.s = ns.String
    }
    return nil
}
```

- [ ] **Step 2: No separate test file needed** — reflect.go is tested indirectly via crud_test.go in the next task. Move on.

---

### Task 4: Implement generic CRUD methods (TDD)

**Files:**
- Create: `internal/starchart/crud.go`
- Create: `internal/starchart/crud_test.go`

- [ ] **Step 1: Write failing tests for CRUD**

Create `internal/starchart/crud_test.go`:

```go
package starchart_test

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func testDB(t *testing.T) *starchart.StarChart {
    t.Helper()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })
    return sc
}

func testAlias(id, name, entityType string) models.Alias {
    return models.Alias{
        ID:         id,
        Name:       name,
        EntityType: entityType,
        CreatedAt:  time.Now().UTC().Truncate(time.Second),
    }
}

func TestInsertAndGet(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    id := models.NewID(models.EntityTypePlanet)
    a := testAlias(id, "payment-api", models.EntityTypePlanet)

    require.NoError(t, sc.Insert(ctx, "aliases", a))

    var got models.Alias
    require.NoError(t, sc.Get(ctx, "aliases", id, &got))
    require.Equal(t, id, got.ID)
    require.Equal(t, "payment-api", got.Name)
    require.Equal(t, models.EntityTypePlanet, got.EntityType)
}

func TestGetNotFound(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    var got models.Alias
    err := sc.Get(ctx, "aliases", "nonexistent-id", &got)
    require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestList(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    ids := []string{
        models.NewID(models.EntityTypePlanet),
        models.NewID(models.EntityTypePlanet),
        models.NewID(models.EntityTypePlanet),
    }
    for _, id := range ids {
        require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet)))
    }

    var results []models.Alias
    require.NoError(t, sc.List(ctx, "aliases", &results,
        starchart.Filter{Column: "entity_type", Op: "=", Value: models.EntityTypePlanet},
    ))
    require.Len(t, results, 3)
}

func TestListEmpty(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    var results []models.Alias
    require.NoError(t, sc.List(ctx, "aliases", &results))
    require.Empty(t, results)
}

func TestUpdate(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    id := models.NewID(models.EntityTypePlanet)
    require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, "old-name", models.EntityTypePlanet)))

    require.NoError(t, sc.Update(ctx, "aliases", id, map[string]any{"name": "new-name"}))

    var got models.Alias
    require.NoError(t, sc.Get(ctx, "aliases", id, &got))
    require.Equal(t, "new-name", got.Name)
}

func TestDelete(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    id := models.NewID(models.EntityTypePlanet)
    require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet)))

    require.NoError(t, sc.Delete(ctx, "aliases", id))

    var got models.Alias
    err := sc.Get(ctx, "aliases", id, &got)
    require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestInsertDuplicateNameFails(t *testing.T) {
    ctx := context.Background()
    sc := testDB(t)

    id1 := models.NewID(models.EntityTypePlanet)
    id2 := models.NewID(models.EntityTypeGalaxy)
    require.NoError(t, sc.Insert(ctx, "aliases", testAlias(id1, "payment-api", models.EntityTypePlanet)))

    err := sc.Insert(ctx, "aliases", testAlias(id2, "payment-api", models.EntityTypeGalaxy))
    require.Error(t, err, "duplicate alias name must fail")
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/starchart/... -run TestInsert 2>&1 | head -5
```

Expected: compile error — `ErrNotFound` and `Filter` not defined yet.

- [ ] **Step 3: Implement crud.go**

Create `internal/starchart/crud.go`:

```go
package starchart

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "reflect"
    "strings"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("record not found")

// Filter represents a single WHERE clause condition.
type Filter struct {
    Column string
    Op     string // "=", "!=", "LIKE", ">", "<", etc.
    Value  any
}

// Insert inserts record into table. record must be a struct with db-tagged fields.
func (sc *StarChart) Insert(ctx context.Context, table string, record any) error {
    cols, placeholders, vals := reflectInsertFields(record)
    if len(cols) == 0 {
        return fmt.Errorf("insert: no db-tagged fields found on record")
    }
    q := fmt.Sprintf(
        "INSERT INTO %s (%s) VALUES (%s)",
        table,
        strings.Join(cols, ", "),
        strings.Join(placeholders, ", "),
    )
    _, err := sc.db.ExecContext(ctx, q, vals...)
    return err
}

// Get fetches a single record by ID from table into dest.
// Returns ErrNotFound if no row exists.
func (sc *StarChart) Get(ctx context.Context, table, id string, dest any) error {
    cols := reflectSelectCols(dest)
    q := fmt.Sprintf(
        "SELECT %s FROM %s WHERE id = ?",
        strings.Join(cols, ", "),
        table,
    )
    row := sc.db.QueryRowContext(ctx, q, id)
    if err := reflectScan(row, dest); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return ErrNotFound
        }
        return err
    }
    return nil
}

// List queries table with optional filters and appends results to dest.
// dest must be a pointer to a slice of structs with db-tagged fields.
func (sc *StarChart) List(ctx context.Context, table string, dest any, filters ...Filter) error {
    destVal := reflect.ValueOf(dest)
    if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
        return fmt.Errorf("list: dest must be a pointer to a slice")
    }
    elemType := destVal.Elem().Type().Elem()
    elem := reflect.New(elemType).Interface()

    cols := reflectSelectCols(elem)
    where, args := buildWhere(filters)
    q := fmt.Sprintf("SELECT %s FROM %s%s", strings.Join(cols, ", "), table, where)

    rows, err := sc.db.QueryContext(ctx, q, args...)
    if err != nil {
        return err
    }
    defer rows.Close()

    sliceVal := destVal.Elem()
    for rows.Next() {
        item := reflect.New(elemType).Interface()
        if err := reflectScan(rows, item); err != nil {
            return err
        }
        sliceVal.Set(reflect.Append(sliceVal, reflect.ValueOf(item).Elem()))
    }
    return rows.Err()
}

// Update applies field updates to the record identified by id in table.
// fields is a map of column name → new value.
func (sc *StarChart) Update(ctx context.Context, table, id string, fields map[string]any) error {
    if len(fields) == 0 {
        return nil
    }
    var sets []string
    var vals []any
    for col, val := range fields {
        sets = append(sets, col+" = ?")
        vals = append(vals, val)
    }
    vals = append(vals, id)
    q := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))
    _, err := sc.db.ExecContext(ctx, q, vals...)
    return err
}

// Delete removes the record identified by id from table.
func (sc *StarChart) Delete(ctx context.Context, table, id string) error {
    q := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
    _, err := sc.db.ExecContext(ctx, q, id)
    return err
}
```

- [ ] **Step 4: Run CRUD tests**

```bash
go test ./internal/starchart/... -run "TestInsert|TestGet|TestList|TestUpdate|TestDelete"
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```
git add internal/starchart/crud.go internal/starchart/reflect.go \
        internal/starchart/crud_test.go
git commit -m "feat: implement generic starchart CRUD with reflection-based struct mapping"
```

---

### Task 5: Implement the Tx pipeline (TDD)

**Files:**
- Create: `internal/starchart/tx.go`
- Create: `internal/starchart/tx_test.go`

The `Tx` wrapper enforces the Star Chart integrity pipeline: Prepare → Validate → Execute → Verify → Commit. If any stage returns an error the transaction rolls back automatically.

- [ ] **Step 1: Write failing Tx tests**

Create `internal/starchart/tx_test.go`:

```go
package starchart_test

import (
    "context"
    "errors"
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func TestTxCommitsOnSuccess(t *testing.T) {
    ctx := context.Background()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    defer sc.Close()

    id := models.NewID(models.EntityTypeGalaxy)

    err = sc.Tx(ctx, func(tx *starchart.Tx) error {
        return tx.Insert(ctx, "aliases", testAlias(id, "my-galaxy", models.EntityTypeGalaxy))
    })
    require.NoError(t, err)

    var got models.Alias
    require.NoError(t, sc.Get(ctx, "aliases", id, &got))
    require.Equal(t, "my-galaxy", got.Name)
}

func TestTxRollsBackOnError(t *testing.T) {
    ctx := context.Background()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    defer sc.Close()

    id := models.NewID(models.EntityTypeGalaxy)
    intentionalErr := errors.New("intentional failure")

    err = sc.Tx(ctx, func(tx *starchart.Tx) error {
        if err := tx.Insert(ctx, "aliases", testAlias(id, "should-rollback", models.EntityTypeGalaxy)); err != nil {
            return err
        }
        return intentionalErr
    })
    require.ErrorIs(t, err, intentionalErr)

    // Record must not exist after rollback.
    var got models.Alias
    require.ErrorIs(t, sc.Get(ctx, "aliases", id, &got), starchart.ErrNotFound)
}

func TestTxNestedInsert(t *testing.T) {
    ctx := context.Background()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    defer sc.Close()

    galaxyID := models.NewID(models.EntityTypeGalaxy)
    planetID := models.NewID(models.EntityTypePlanet)

    err = sc.Tx(ctx, func(tx *starchart.Tx) error {
        if err := tx.Insert(ctx, "aliases", testAlias(galaxyID, "acme", models.EntityTypeGalaxy)); err != nil {
            return err
        }
        if err := tx.Insert(ctx, "galaxies", models.Galaxy{ID: galaxyID}); err != nil {
            return err
        }
        if err := tx.Insert(ctx, "aliases", testAlias(planetID, "payment-api", models.EntityTypePlanet)); err != nil {
            return err
        }
        return tx.Insert(ctx, "planets", models.Planet{
            ID:       planetID,
            GalaxyID: galaxyID,
        })
    })
    require.NoError(t, err)

    var g models.Galaxy
    require.NoError(t, sc.Get(ctx, "galaxies", galaxyID, &g))
    var p models.Planet
    require.NoError(t, sc.Get(ctx, "planets", planetID, &p))
    require.Equal(t, galaxyID, p.GalaxyID)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/starchart/... -run TestTx 2>&1 | head -5
```

Expected: compile error — `Tx` type not yet defined.

- [ ] **Step 3: Implement tx.go**

Create `internal/starchart/tx.go`:

```go
package starchart

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
)

// Tx wraps a *sql.Tx and exposes the same CRUD surface as StarChart.
// Used within the Prepare → Validate → Execute → Verify → Commit pipeline.
type Tx struct {
    tx *sql.Tx
}

// Tx executes fn within a database transaction that enforces the Star Chart
// integrity pipeline. If fn returns an error the transaction is rolled back.
func (sc *StarChart) Tx(ctx context.Context, fn func(*Tx) error) error {
    sqlTx, err := sc.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin transaction: %w", err)
    }

    t := &Tx{tx: sqlTx}
    if err := fn(t); err != nil {
        if rbErr := sqlTx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
            return fmt.Errorf("rollback failed: %w (original: %v)", rbErr, err)
        }
        return err
    }

    if err := sqlTx.Commit(); err != nil {
        return fmt.Errorf("commit transaction: %w", err)
    }
    return nil
}

// Insert inserts record into table within the transaction.
func (t *Tx) Insert(ctx context.Context, table string, record any) error {
    cols, placeholders, vals := reflectInsertFields(record)
    if len(cols) == 0 {
        return fmt.Errorf("tx insert: no db-tagged fields found on record")
    }
    q := fmt.Sprintf(
        "INSERT INTO %s (%s) VALUES (%s)",
        table,
        strings.Join(cols, ", "),
        strings.Join(placeholders, ", "),
    )
    _, err := t.tx.ExecContext(ctx, q, vals...)
    return err
}

// Update applies field updates within the transaction.
func (t *Tx) Update(ctx context.Context, table, id string, fields map[string]any) error {
    if len(fields) == 0 {
        return nil
    }
    var sets []string
    var vals []any
    for col, val := range fields {
        sets = append(sets, col+" = ?")
        vals = append(vals, val)
    }
    vals = append(vals, id)
    q := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))
    _, err := t.tx.ExecContext(ctx, q, vals...)
    return err
}

// Delete removes a record within the transaction.
func (t *Tx) Delete(ctx context.Context, table, id string) error {
    q := fmt.Sprintf("DELETE FROM %s WHERE id = ?", table)
    _, err := t.tx.ExecContext(ctx, q, id)
    return err
}
```

- [ ] **Step 4: Run Tx tests**

```bash
go test ./internal/starchart/... -run TestTx
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```
git add internal/starchart/tx.go internal/starchart/tx_test.go
git commit -m "feat: implement starchart Tx pipeline with automatic rollback on error"
```

---

### Task 6: Implement Resolve (TDD)

**Files:**
- Create: `internal/starchart/resolve.go`
- Create: `internal/starchart/resolve_test.go`

`Resolve` is the single optimized query: it checks `aliases.name` first (human-readable), then falls back to a direct ID match. Returns `ErrNotFound` if neither matches.

- [ ] **Step 1: Write failing Resolve tests**

Create `internal/starchart/resolve_test.go`:

```go
package starchart_test

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func setupResolveDB(t *testing.T) (*starchart.StarChart, string) {
    t.Helper()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })

    id := models.NewID(models.EntityTypePlanet)
    err = sc.Insert(context.Background(), "aliases", testAlias(id, "payment-api", models.EntityTypePlanet))
    require.NoError(t, err)
    return sc, id
}

func TestResolveByName(t *testing.T) {
    ctx := context.Background()
    sc, id := setupResolveDB(t)

    alias, err := sc.Resolve(ctx, "payment-api")
    require.NoError(t, err)
    require.Equal(t, id, alias.ID)
    require.Equal(t, models.EntityTypePlanet, alias.EntityType)
}

func TestResolveByID(t *testing.T) {
    ctx := context.Background()
    sc, id := setupResolveDB(t)

    alias, err := sc.Resolve(ctx, id)
    require.NoError(t, err)
    require.Equal(t, id, alias.ID)
    require.Equal(t, "payment-api", alias.Name)
}

func TestResolveNotFound(t *testing.T) {
    ctx := context.Background()
    sc, _ := setupResolveDB(t)

    _, err := sc.Resolve(ctx, "does-not-exist")
    require.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestResolveNameTakesPrecedenceOverIDLookup(t *testing.T) {
    ctx := context.Background()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    defer sc.Close()

    // Insert an alias whose name equals its ID (the default case when no alias is given).
    id := models.NewID(models.EntityTypePlanet)
    err = sc.Insert(ctx, "aliases", testAlias(id, id, models.EntityTypePlanet))
    require.NoError(t, err)

    alias, err := sc.Resolve(ctx, id)
    require.NoError(t, err)
    require.Equal(t, id, alias.ID)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/starchart/... -run TestResolve 2>&1 | head -5
```

Expected: compile error — `Resolve` not yet defined.

- [ ] **Step 3: Implement resolve.go**

Create `internal/starchart/resolve.go`:

```go
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
```

- [ ] **Step 4: Run Resolve tests**

```bash
go test ./internal/starchart/... -run TestResolve
```

Expected: all PASS.

- [ ] **Step 5: Run the full starchart test suite**

```bash
go test ./internal/starchart/...
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```
git add internal/starchart/resolve.go internal/starchart/resolve_test.go
git commit -m "feat: implement starchart Resolve with name-first alias lookup"
```

---

### Task 7: Verify complete build

- [ ] **Step 1: Run all tests**

```bash
just test
```

Expected: all packages pass — `migrations`, `models`, `starchart`.

- [ ] **Step 2: Verify both binaries still compile**

```bash
just build
```

Expected: no errors.

- [ ] **Step 3: Tidy module**

```bash
go mod tidy
git diff go.mod go.sum
```

If there are changes:

```
git add go.mod go.sum
git commit -m "chore: tidy go module after starchart layer"
```
