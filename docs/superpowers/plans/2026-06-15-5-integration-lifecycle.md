# Integration Lifecycle Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the full integration lifecycle: `orbiter init` (and its alias `orbiter vessel init`) extracts selected WASM integrations to disk with an interactive bubbletea checklist (idempotent, upgrade-aware), `LoadInstalled` loads them at startup via `PersistentPreRunE`, and copy-pasted third-party integrations in `~/.orbiter/integrations/` are automatically discovered.

**Architecture:** Five tasks in dependency order. Task 1 adds disk extraction + install state reading to `bundle.go`. Task 2 adds the `approve` param to `LoadInstalled` (mirrors `InstallSelected`). Task 3 builds the reusable bubbletea checklist component. Task 4 rewrites the shared `vesselInitRun` in `shell.go` using the checklist with pre-checks and version badges, and adds a `--yes` flag to `newInitCmd` for non-interactive use. Task 5 wires `LoadInstalled` into `PersistentPreRunE` and removes the dead blank import from `main.go`.

**Tech Stack:** Go 1.25, bubbletea v1.3.10 (already in go.mod), crypto/sha256

---

## File Map

**New files:**
- `internal/commands/checklist.go` — bubbletea multi-select ChecklistModel (pure, no commands-package deps; designed for extraction to a UI package in the TUI phase)
- `internal/commands/checklist_test.go` — unit tests for ChecklistModel without launching a TUI program
- `integrations/extract_test.go` — tests for ExtractSelected, InstalledState, CatalogEntriesWithState

**Modified files:**
- `integrations/bundle.go` — add `InstalledInfo`, `CatalogEntryState`, `InstalledState`, `ExtractSelected`, `CatalogEntriesWithState`; update `LoadInstalled` to accept `approve wasm.ApproveFunc`
- `integrations/catalog_test.go` — update `TestInstallSelected_RegistersInRegistry` for new signature; add `TestLoadInstalled_*` tests
- `internal/commands/vessel.go` — export `BuildChecklistItems` and `ApplySelections` (pure helpers, no TUI dependency)
- `internal/commands/vessel_test.go` — add tests for `BuildChecklistItems` and `ApplySelections`
- `internal/commands/shell.go` — rewrite `vesselInitRun` with checklist TUI; add `--yes` flag to `newInitCmd` for non-interactive/CI use; update `newVesselInitCmd` to accept and thread the `--yes` flag
- `internal/commands/root.go` — add `LoadInstalled` call in `PersistentPreRunE`
- `cmd/orbiter/main.go` — remove dead blank import `_ "github.com/Kenttleton/orbiter/integrations"`
- `cmd/orbiter/testdata/script/01-init.txt` — update `orbiter init` assertions to use `--yes` flag for non-interactive test path

---

## Task 1: Disk extraction + install state in bundle.go

**Purpose:** `vessel init` needs to write WASM + manifest files to `~/.orbiter/integrations/<brand>/`. `LoadInstalled` (and the checklist pre-population) needs to know what's already on disk and whether the installed WASM matches the bundled version.

**Files:**
- Modify: `integrations/bundle.go`
- Create: `integrations/extract_test.go`

### Background: embed FS layout vs. on-disk layout

The embedded FS uses the Go source directory name as the directory (e.g., `golang/golang.wasm`), but the brand is "go". On disk, Orbiter uses the brand name: `~/.orbiter/integrations/go/go.wasm`. `LoadInstalled` reads directory name from disk and expects `<dirname>/<dirname>.wasm` — so the directory name on disk must match the brand.

- [ ] **Step 1: Write the failing tests**

Create `integrations/extract_test.go`:

```go
package integrations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kenttleton/orbiter/integrations"
)

func TestExtractSelected_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	if err := integrations.ExtractSelected(entries[:1], dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}

	brand := entries[0].Brand
	if _, err := os.Stat(filepath.Join(dir, brand, "manifest.toml")); err != nil {
		t.Errorf("manifest.toml not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, brand, brand+".wasm")); err != nil {
		t.Errorf("%s.wasm not created: %v", brand, err)
	}
}

func TestExtractSelected_Idempotent(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("first extract: %v", err)
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("second extract (idempotent): %v", err)
	}
}

func TestInstalledState_EmptyDir(t *testing.T) {
	state, err := integrations.InstalledState(t.TempDir())
	if err != nil {
		t.Fatalf("InstalledState on empty dir: %v", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state, got %d entries", len(state))
	}
}

func TestInstalledState_MissingDir(t *testing.T) {
	state, err := integrations.InstalledState("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("InstalledState on missing dir should not error: %v", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state for missing dir, got %d entries", len(state))
	}
}

func TestInstalledState_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}
	state, err := integrations.InstalledState(dir)
	if err != nil {
		t.Fatalf("InstalledState: %v", err)
	}
	for _, e := range entries {
		if _, ok := state[e.Brand]; !ok {
			t.Errorf("expected brand %q in installed state after extract", e.Brand)
		}
	}
}

func TestCatalogEntriesWithState_NoneInstalled(t *testing.T) {
	states := integrations.CatalogEntriesWithState(t.TempDir())
	for _, s := range states {
		if s.Installed {
			t.Errorf("brand %q should not be installed in empty dir", s.Brand)
		}
	}
}

func TestCatalogEntriesWithState_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}
	states := integrations.CatalogEntriesWithState(dir)
	for _, s := range states {
		if !s.Installed {
			t.Errorf("brand %q should be installed after extract", s.Brand)
		}
		if !s.ChecksumMatches {
			t.Errorf("brand %q checksum should match bundled after fresh extract", s.Brand)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./integrations/... -run "TestExtractSelected|TestInstalledState|TestCatalogEntriesWithState" -count=1
```

Expected: FAIL — `ExtractSelected`, `InstalledState`, `CatalogEntriesWithState` undefined.

- [ ] **Step 3: Add types and functions to bundle.go**

Add `"crypto/sha256"` and `"fmt"` to bundle.go imports (keep existing imports).

Add after the `DefaultIntegrationsDir` function:

```go
// InstalledInfo describes the on-disk state of one installed integration.
type InstalledInfo struct {
	Dir      string   // absolute path to the integration directory
	Checksum [32]byte // SHA256 of the installed .wasm file
}

// InstalledState reads dir and returns a map keyed by brand.
// Directories without a readable manifest.toml or matching .wasm are skipped.
// Returns an empty map (no error) when dir does not exist.
func InstalledState(dir string) (map[string]InstalledInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]InstalledInfo{}, nil
		}
		return nil, err
	}
	result := make(map[string]InstalledInfo)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		name := e.Name()
		entryDir := filepath.Join(dir, name)

		manifestBytes, err := os.ReadFile(filepath.Join(entryDir, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}

		wasmBytes, err := os.ReadFile(filepath.Join(entryDir, name+".wasm"))
		if err != nil {
			continue
		}
		result[manifest.Integration.Brand] = InstalledInfo{
			Dir:      entryDir,
			Checksum: sha256.Sum256(wasmBytes),
		}
	}
	return result, nil
}

// CatalogEntryState pairs a CatalogEntry with its current install status.
type CatalogEntryState struct {
	CatalogEntry
	Installed       bool // true if a directory for this brand exists in dir
	ChecksumMatches bool // true if the installed WASM byte-matches the bundled WASM
}

// CatalogEntriesWithState returns all catalog entries annotated with their
// install state from dir. Use this to populate the vessel init checklist.
func CatalogEntriesWithState(dir string) []CatalogEntryState {
	installed, _ := InstalledState(dir)
	bundled := bundledChecksums()

	entries := CatalogEntries()
	result := make([]CatalogEntryState, len(entries))
	for i, e := range entries {
		state := CatalogEntryState{CatalogEntry: e}
		if info, ok := installed[e.Brand]; ok {
			state.Installed = true
			if bc, ok := bundled[e.Brand]; ok {
				state.ChecksumMatches = bc == info.Checksum
			}
		}
		result[i] = state
	}
	return result
}

// bundledChecksums returns a map from brand to SHA256 of the embedded WASM.
func bundledChecksums() map[string][32]byte {
	dirs, _ := fs.ReadDir(bundleFS, ".")
	result := make(map[string][32]byte)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}
		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			continue
		}
		result[manifest.Integration.Brand] = sha256.Sum256(wasmBytes)
	}
	return result
}

// ExtractSelected writes the WASM and manifest for each selected catalog entry
// to dir/<brand>/. Existing files are overwritten (safe upgrade path).
// The directory for dir is created if it does not exist.
func ExtractSelected(entries []CatalogEntry, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create integrations dir: %w", err)
	}

	dirs, err := fs.ReadDir(bundleFS, ".")
	if err != nil {
		return err
	}

	wanted := make(map[string]bool, len(entries))
	for _, e := range entries {
		wanted[e.Brand] = true
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()

		manifestBytes, err := bundleFS.ReadFile(path.Join(name, "manifest.toml"))
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			continue
		}
		if !wanted[manifest.Integration.Brand] {
			continue
		}

		brand := manifest.Integration.Brand
		destDir := filepath.Join(dir, brand)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", brand, err)
		}

		wasmBytes, err := bundleFS.ReadFile(path.Join(name, name+".wasm"))
		if err != nil {
			return fmt.Errorf("read wasm for %s: %w", brand, err)
		}
		if err := os.WriteFile(filepath.Join(destDir, "manifest.toml"), manifestBytes, 0644); err != nil {
			return fmt.Errorf("write manifest for %s: %w", brand, err)
		}
		if err := os.WriteFile(filepath.Join(destDir, brand+".wasm"), wasmBytes, 0644); err != nil {
			return fmt.Errorf("write wasm for %s: %w", brand, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./integrations/... -run "TestExtractSelected|TestInstalledState|TestCatalogEntriesWithState" -count=1
```

Expected: PASS all 7 tests.

- [ ] **Step 5: Commit**

```bash
git add integrations/bundle.go integrations/extract_test.go
git commit -m "feat: disk extraction + install state for integration lifecycle"
```

---

## Task 2: LoadInstalled approve param + tests

**Purpose:** `LoadInstalled` currently hardcodes `core.DefaultSettings` and `nil` for approve (which defaults to `StdinApproveFunc`). Both should mirror the `InstallSelected` pattern: use `registry.Settings()` and accept an explicit `approve` parameter. This also makes `LoadInstalled` testable without stdin or global state.

**Files:**
- Modify: `integrations/bundle.go`
- Modify: `integrations/catalog_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `integrations/catalog_test.go` (inside the existing `package integrations_test`):

```go
func TestLoadInstalled_EmptyDir(t *testing.T) {
	reg := core.NewRegistry(nil)
	approve := func(_, _ string) bool { return true }
	if err := integrations.LoadInstalled(t.TempDir(), reg, approve); err != nil {
		t.Fatalf("LoadInstalled on empty dir: %v", err)
	}
}

func TestLoadInstalled_AfterExtract(t *testing.T) {
	dir := t.TempDir()
	entries := integrations.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}
	if err := integrations.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("ExtractSelected: %v", err)
	}

	reg := core.NewRegistry(nil)
	approve := func(_, _ string) bool { return true }
	if err := integrations.LoadInstalled(dir, reg, approve); err != nil {
		t.Fatalf("LoadInstalled: %v", err)
	}

	for _, e := range entries {
		for _, role := range e.Roles {
			if _, ok := reg.Get(role, e.Brand); !ok {
				t.Errorf("expected %s/%s registered after LoadInstalled", role, e.Brand)
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./integrations/... -run "TestLoadInstalled" -count=1
```

Expected: FAIL — `LoadInstalled` called with wrong number of args.

- [ ] **Step 3: Update LoadInstalled signature in bundle.go**

Replace the entire `LoadInstalled` function:

```go
// LoadInstalled scans dir for integration directories and loads any that
// contain a manifest.toml and a matching <name>.wasm file. Loaded integrations
// are registered in the provided registry.
// approve is passed to each loaded WASMIntegration; nil uses StdinApproveFunc.
// This is used to load third-party integrations the Captain has installed,
// and is called at startup for every command that opens the StarChart.
func LoadInstalled(dir string, registry *core.Registry, approve wasm.ApproveFunc) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	ctx := context.Background()
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		name := e.Name()
		manifestPath := filepath.Join(dir, name, "manifest.toml")
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest core.Manifest
		if _, err := toml.Decode(string(manifestBytes), &manifest); err != nil {
			log.Printf("orbiter: parse manifest %s: %v", manifestPath, err)
			continue
		}

		wasmPath := filepath.Join(dir, name, name+".wasm")
		wasmBytes, err := os.ReadFile(wasmPath)
		if err != nil {
			log.Printf("orbiter: wasm for %s: %v", name, err)
			continue
		}

		i, err := wasm.Load(ctx, manifest, wasmBytes, registry.Settings(), registry, approve)
		if err != nil {
			log.Printf("orbiter: load %s: %v", name, err)
			continue
		}
		for _, role := range manifest.Integration.Roles {
			registry.Register(role, manifest.Integration.Brand, i)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run all integration tests**

```
go test ./integrations/... -count=1
```

Expected: PASS all tests (including the new `TestLoadInstalled_*` tests).

- [ ] **Step 5: Commit**

```bash
git add integrations/bundle.go integrations/catalog_test.go
git commit -m "feat: LoadInstalled accepts approve func, uses registry.Settings()"
```

---

## Task 3: Bubbletea checklist component

**Purpose:** `vessel init` needs an interactive multi-select checkbox. The component lives in `internal/commands/` for now but is designed to be cleanly relocatable when the TUI phase arrives — no commands-package types in its interface, no cobra imports.

**Files:**
- Create: `internal/commands/checklist.go`
- Create: `internal/commands/checklist_test.go`

### Key design decisions
- `Update` uses `msg.String()` comparisons (reliable across bubbletea versions)
- `Items` is exported on the struct so callers can inspect state after `Run()`
- `Cursor()` method exposes cursor position for tests
- `Done()` is false when user presses `q` or `ctrl+c` — caller checks this before acting
- No lipgloss styling yet — plain ASCII; TUI phase will add styling

- [ ] **Step 1: Write the failing tests**

Create `internal/commands/checklist_test.go`:

```go
package commands_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Kenttleton/orbiter/internal/commands"
)

func applyKey(m commands.ChecklistModel, s string) commands.ChecklistModel {
	var msg tea.Msg
	switch s {
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	updated, _ := m.Update(msg)
	return updated.(commands.ChecklistModel)
}

func TestChecklistModel_SpaceTogglesChecked(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "Item A", Tag: "a", Checked: false},
	})
	m = applyKey(m, " ")
	if !m.Items[0].Checked {
		t.Error("expected item to be checked after space")
	}
	m = applyKey(m, " ")
	if m.Items[0].Checked {
		t.Error("expected item to be unchecked after second space")
	}
}

func TestChecklistModel_Navigation(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a"},
		{Label: "B", Tag: "b"},
		{Label: "C", Tag: "c"},
	})
	if m.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", m.Cursor())
	}
	m = applyKey(m, "down")
	if m.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", m.Cursor())
	}
	m = applyKey(m, "down")
	if m.Cursor() != 2 {
		t.Errorf("cursor should be 2 after second down, got %d", m.Cursor())
	}
	// Down at last item stays
	m = applyKey(m, "down")
	if m.Cursor() != 2 {
		t.Errorf("cursor should stay at 2 at bottom, got %d", m.Cursor())
	}
	m = applyKey(m, "up")
	if m.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", m.Cursor())
	}
}

func TestChecklistModel_EnterSetsDone(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a", Checked: true},
	})
	if m.Done() {
		t.Fatal("done should be false before enter")
	}
	m = applyKey(m, "enter")
	if !m.Done() {
		t.Error("done should be true after enter")
	}
}

func TestChecklistModel_QuitDoesNotSetDone(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a"},
	})
	m = applyKey(m, "q")
	if m.Done() {
		t.Error("done should be false after q")
	}
}

func TestChecklistModel_Selected(t *testing.T) {
	m := commands.NewChecklistModel("Test", []commands.ChecklistItem{
		{Label: "A", Tag: "a", Checked: true},
		{Label: "B", Tag: "b", Checked: false},
		{Label: "C", Tag: "c", Checked: true},
	})
	selected := m.Selected()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}
	if selected[0].Tag != "a" || selected[1].Tag != "c" {
		t.Errorf("unexpected selected items: %v", selected)
	}
}

func TestChecklistModel_ViewContainsLabels(t *testing.T) {
	m := commands.NewChecklistModel("Pick one:", []commands.ChecklistItem{
		{Label: "Alpha", Tag: "a", Badge: "upgrade available"},
		{Label: "Beta", Tag: "b", Checked: true},
	})
	view := m.View()
	for _, want := range []string{"Alpha", "Beta", "upgrade available", "Pick one:"} {
		if !containsString(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/commands/... -run "TestChecklistModel" -count=1
```

Expected: FAIL — `commands.ChecklistModel`, `commands.ChecklistItem`, `commands.NewChecklistModel` undefined.

- [ ] **Step 3: Create internal/commands/checklist.go**

```go
package commands

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ChecklistItem is one row in the checklist.
// Tag is an opaque identifier (brand name) used by callers; it is never displayed.
// Badge is optional text shown after the label (e.g. "upgrade available").
type ChecklistItem struct {
	Label   string
	Tag     string
	Checked bool
	Badge   string
}

// ChecklistModel is a bubbletea model for an interactive multi-select checklist.
// It is designed to be relocatable: no commands-package types in its interface.
// Use NewChecklistModel to construct; call tea.NewProgram(m).Run() to drive it.
type ChecklistModel struct {
	Title  string
	Items  []ChecklistItem
	cursor int
	done   bool
}

// NewChecklistModel returns a ChecklistModel with the given title and items.
func NewChecklistModel(title string, items []ChecklistItem) ChecklistModel {
	return ChecklistModel{Title: title, Items: items}
}

// Cursor returns the current cursor position (0-indexed).
func (m ChecklistModel) Cursor() int { return m.cursor }

// Done returns true if the user confirmed the selection with Enter.
// Done is false when the user cancelled with q or ctrl+c.
func (m ChecklistModel) Done() bool { return m.done }

// Selected returns the subset of Items where Checked is true.
func (m ChecklistModel) Selected() []ChecklistItem {
	var out []ChecklistItem
	for _, item := range m.Items {
		if item.Checked {
			out = append(out, item)
		}
	}
	return out
}

func (m ChecklistModel) Init() tea.Cmd { return nil }

func (m ChecklistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case " ":
			m.Items[m.cursor].Checked = !m.Items[m.cursor].Checked
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ChecklistModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.Title + "\n\n")
	for i, item := range m.Items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		checked := "[ ]"
		if item.Checked {
			checked = "[x]"
		}
		badge := ""
		if item.Badge != "" {
			badge = fmt.Sprintf(" (%s)", item.Badge)
		}
		fmt.Fprintf(&sb, "%s%s %s%s\n", cursor, checked, item.Label, badge)
	}
	sb.WriteString("\n↑/↓ move  space toggle  enter confirm  q cancel\n")
	return sb.String()
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/commands/... -run "TestChecklistModel" -count=1
```

Expected: PASS all 6 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/checklist.go internal/commands/checklist_test.go
git commit -m "feat: bubbletea checklist component for vessel init"
```

---

## Task 4: vessel init rewrite with checklist TUI

**Purpose:** Replace the simple `vesselInitRun` with an interactive checklist: show current install state, extract selected integrations to disk, and remove unselected previously-installed integrations. Both `orbiter init` and `orbiter vessel init` call `vesselInitRun` — updating it upgrades both routes. A `--yes` flag bypasses the TUI for CI and testscript.

**Files:**

- Modify: `internal/commands/vessel.go` — add exported helpers `BuildChecklistItems`, `ApplySelections`
- Modify: `internal/commands/vessel_test.go` — tests for the two helpers
- Modify: `internal/commands/shell.go` — rewrite `vesselInitRun(out io.Writer, yes bool)`, add `--yes` to `newInitCmd` and thread it through; update `newVesselInitCmd` similarly

### Key context: command routing

As of the command taxonomy restructuring (2026-06-15), both `orbiter init` and `orbiter vessel init` call `vesselInitRun(out io.Writer)` in `shell.go`. Rewriting `vesselInitRun` automatically upgrades both routes. `BuildChecklistItems` and `ApplySelections` live in `vessel.go` because they are integration-domain logic; `shell.go` calls them.

- [ ] **Step 1: Write the failing tests**

Add to `internal/commands/vessel_test.go` (inside `package commands_test`):

```go
import (
	// existing imports...
	"os"
	"path/filepath"

	bundle "github.com/Kenttleton/orbiter/integrations"
	// existing imports remain
)
```

Add these test functions:

```go
func TestBuildChecklistItems_NotInstalled(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git", Description: "Git VCS"}, Installed: false},
	}
	items := commands.BuildChecklistItems(states)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Checked {
		t.Error("not-installed item should not be pre-checked")
	}
	if items[0].Badge != "" {
		t.Errorf("not-installed item should have no badge, got %q", items[0].Badge)
	}
	if items[0].Tag != "git" {
		t.Errorf("tag should be brand, got %q", items[0].Tag)
	}
}

func TestBuildChecklistItems_InstalledUpToDate(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git"}, Installed: true, ChecksumMatches: true},
	}
	items := commands.BuildChecklistItems(states)
	if !items[0].Checked {
		t.Error("installed item should be pre-checked")
	}
	if items[0].Badge != "" {
		t.Errorf("up-to-date item should have no badge, got %q", items[0].Badge)
	}
}

func TestBuildChecklistItems_UpgradeAvailable(t *testing.T) {
	states := []bundle.CatalogEntryState{
		{CatalogEntry: bundle.CatalogEntry{Brand: "git", Name: "Git"}, Installed: true, ChecksumMatches: false},
	}
	items := commands.BuildChecklistItems(states)
	if !items[0].Checked {
		t.Error("outdated item should still be pre-checked")
	}
	if items[0].Badge != "upgrade available" {
		t.Errorf("outdated item should have 'upgrade available' badge, got %q", items[0].Badge)
	}
}

func TestApplySelections_ExtractsSelected(t *testing.T) {
	dir := t.TempDir()
	entries := bundle.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	states := bundle.CatalogEntriesWithState(dir)
	selected := []commands.ChecklistItem{{Tag: entries[0].Brand}}

	if err := commands.ApplySelections(states, selected, dir); err != nil {
		t.Fatalf("ApplySelections: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, entries[0].Brand)); err != nil {
		t.Errorf("expected %s to be installed after ApplySelections: %v", entries[0].Brand, err)
	}
}

func TestApplySelections_RemovesDeselected(t *testing.T) {
	dir := t.TempDir()
	entries := bundle.CatalogEntries()
	if len(entries) == 0 {
		t.Skip("no catalog entries")
	}

	// Pre-install all.
	if err := bundle.ExtractSelected(entries, dir); err != nil {
		t.Fatalf("pre-install: %v", err)
	}

	states := bundle.CatalogEntriesWithState(dir)
	// Deselect all.
	if err := commands.ApplySelections(states, nil, dir); err != nil {
		t.Fatalf("ApplySelections: %v", err)
	}

	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Brand)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed after deselect", e.Brand)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/commands/... -run "TestBuildChecklistItems|TestApplySelections" -count=1
```

Expected: FAIL — `commands.BuildChecklistItems`, `commands.ApplySelections` undefined.

- [ ] **Step 3: Add BuildChecklistItems and ApplySelections to vessel.go**

Add these imports to vessel.go:

```go
import (
	// existing imports...
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	bundle "github.com/Kenttleton/orbiter/integrations"
	integrations "github.com/Kenttleton/orbiter/internal/integrations"
)
```

Add the two exported helpers before `newVesselInitCmd`:

```go
// BuildChecklistItems converts CatalogEntryState slice into ChecklistItem slice
// for vessel init. Pre-checks installed entries and adds "upgrade available" badges
// for entries whose installed WASM differs from the bundled version.
func BuildChecklistItems(states []bundle.CatalogEntryState) []ChecklistItem {
	items := make([]ChecklistItem, len(states))
	for i, s := range states {
		badge := ""
		if s.Installed && !s.ChecksumMatches {
			badge = "upgrade available"
		}
		items[i] = ChecklistItem{
			Label:   fmt.Sprintf("%s — %s (roles: %s)", s.Name, s.Description, strings.Join(s.Roles, ", ")),
			Tag:     s.Brand,
			Checked: s.Installed,
			Badge:   badge,
		}
	}
	return items
}

// ApplySelections extracts selected integrations to dir and removes directories
// for integrations that were previously installed but are no longer selected.
func ApplySelections(states []bundle.CatalogEntryState, selected []ChecklistItem, dir string) error {
	selectedBrands := make(map[string]bool, len(selected))
	for _, item := range selected {
		selectedBrands[item.Tag] = true
	}

	// Remove deselected integrations that were previously installed.
	for _, s := range states {
		if s.Installed && !selectedBrands[s.Brand] {
			if err := os.RemoveAll(filepath.Join(dir, s.Brand)); err != nil {
				return fmt.Errorf("remove %s: %w", s.Brand, err)
			}
		}
	}

	// Extract selected entries.
	var toExtract []bundle.CatalogEntry
	for _, s := range states {
		if selectedBrands[s.Brand] {
			toExtract = append(toExtract, s.CatalogEntry)
		}
	}
	return bundle.ExtractSelected(toExtract, dir)
}
```

- [ ] **Step 4: Rewrite vesselInitRun in shell.go and add --yes flag**

`vesselInitRun` is the shared entry point called by both `newInitCmd` and `newVesselInitCmd`. Update its signature to `vesselInitRun(out io.Writer, yes bool) error`. When `yes` is true, skip the TUI and install all entries (existing non-interactive path). When false, run the checklist.

Add `--yes` flag to `newInitCmd` and pass it through to `vesselInitRun`. Update `newVesselInitCmd` the same way.

Replace `vesselInitRun` in `internal/commands/shell.go`:

```go
func vesselInitRun(out io.Writer, yes bool) error {
	dir := bundle.DefaultIntegrationsDir()
	states := bundle.CatalogEntriesWithState(dir)

	if yes {
		// Non-interactive: install all catalog entries.
		entries := make([]bundle.CatalogEntry, len(states))
		for i, s := range states {
			entries[i] = s.CatalogEntry
		}
		if err := bundle.ExtractSelected(entries, dir); err != nil {
			return fmt.Errorf("install integrations: %w", err)
		}
		fmt.Fprintf(out, "Installed %d integration(s)\n", len(entries))
		return nil
	}

	items := BuildChecklistItems(states)
	if len(items) == 0 {
		fmt.Fprintln(out, "No integrations available in catalog.")
		return nil
	}

	initial := NewChecklistModel("Select integrations to install:", items)
	result, err := tea.NewProgram(initial).Run()
	if err != nil {
		return fmt.Errorf("checklist: %w", err)
	}

	final := result.(ChecklistModel)
	if !final.Done() {
		fmt.Fprintln(out, "Cancelled.")
		return nil
	}

	if err := ApplySelections(states, final.Selected(), dir); err != nil {
		return fmt.Errorf("apply selections: %w", err)
	}

	installed := len(final.Selected())
	removed := 0
	selectedBrands := make(map[string]bool, len(final.Selected()))
	for _, sel := range final.Selected() {
		selectedBrands[sel.Tag] = true
	}
	for _, s := range states {
		if s.Installed && !selectedBrands[s.Brand] {
			removed++
		}
	}
	fmt.Fprintf(out, "Installed %d integration(s)", installed)
	if removed > 0 {
		fmt.Fprintf(out, ", removed %d", removed)
	}
	fmt.Fprintln(out)
	return nil
}
```

Update `newInitCmd` in `shell.go` to bind and pass the flag:

```go
func newInitCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		// ... (Use, Short, Long, Args unchanged)
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			switch target {
			case "shell":
				return printShellScript()
			case "vessel", "":
				return vesselInitRun(cmd.OutOrStdout(), yes)
			default:
				return fmt.Errorf("unknown init target %q — use 'shell' or 'vessel'", target)
			}
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install all integrations without interactive selection")
	return cmd
}
```

Update `newVesselInitCmd` in `vessel.go` the same way: bind `--yes` and pass it to `vesselInitRun`.

Add required imports to `shell.go`: `tea "github.com/charmbracelet/bubbletea"`. `BuildChecklistItems`, `ApplySelections`, `NewChecklistModel`, `ChecklistModel` come from `vessel.go` (same package — no import needed).

- [ ] **Step 5: Update testscript 01-init.txt**

The testscript for init must use `--yes` since TUI cannot run in a non-interactive pipe. Update `cmd/orbiter/testdata/script/01-init.txt`:

```
# No-arg init → vessel init (--yes skips TUI, installs all)
exec orbiter init --yes
stdout 'Installed 2 integration\(s\)'

exec orbiter init vessel --yes
stdout 'Installed 2 integration\(s\)'

exec orbiter vessel init --yes
stdout 'Installed 2 integration\(s\)'

# Shell target is unaffected — no --yes needed
exec orbiter init shell
stdout 'function orbiter'

exec orbiter shell init
stdout 'function orbiter'

! exec orbiter init badtarget
stderr 'unknown init target'
```

- [ ] **Step 6: Run all tests including testscript**

```
go test ./... -count=1
```

Expected: PASS all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/commands/vessel.go internal/commands/vessel_test.go \
        internal/commands/shell.go cmd/orbiter/testdata/script/01-init.txt
git commit -m "feat: orbiter init interactive checklist with upgrade detection and --yes flag"
```

---

## Task 5: Startup LoadInstalled in PersistentPreRunE + main.go cleanup

**Purpose:** Every lifecycle command (scan, calibrate, jump, chart, etc.) must find integrations already registered. `LoadInstalled` in `PersistentPreRunE` gives them that. The now-dead blank import `_ "github.com/Kenttleton/orbiter/integrations"` in `main.go` can be removed since `root.go` will import the bundle package explicitly. The native filesystem integration blank import `_ "github.com/Kenttleton/orbiter/internal/integrations/native"` must stay — that package has an `init()` that registers the native integration.

**Files:**
- Modify: `internal/commands/root.go`
- Modify: `cmd/orbiter/main.go`

- [ ] **Step 1: Add LoadInstalled call to PersistentPreRunE in root.go**

In `root.go`, add these imports:

```go
bundle "github.com/Kenttleton/orbiter/integrations"
"github.com/Kenttleton/orbiter/internal/integrations"
"github.com/Kenttleton/orbiter/internal/wasm"
```

In `PersistentPreRunE`, add after opening the StarChart (after `d.sc = sc`) and before output/resolver setup:

```go
// Load integrations installed to disk into the Default registry.
// Non-fatal: a missing integrations dir is silently skipped (LoadInstalled handles it).
if err := bundle.LoadInstalled(
    bundle.DefaultIntegrationsDir(),
    integrations.Default,
    wasm.StdinApproveFunc,
); err != nil {
    fmt.Fprintf(os.Stderr, "orbiter: load integrations: %v\n", err)
}
```

- [ ] **Step 2: Remove dead blank import from main.go**

In `cmd/orbiter/main.go`, remove this line:

```go
_ "github.com/Kenttleton/orbiter/integrations"
```

Keep this line (it's still needed for the native filesystem integration's `init()`):

```go
_ "github.com/Kenttleton/orbiter/internal/integrations/native"
```

- [ ] **Step 3: Build to verify no import errors**

```
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run the full test suite**

```
go test ./... -count=1
```

Expected: PASS all packages.

- [ ] **Step 5: Verify with a real install + restart simulation**

This simulates the full lifecycle: interactive init → disk extraction → LoadInstalled on next startup:

```bash
# Interactive: checklist appears, select git and go, press enter
orbiter init

# Or non-interactive for scripted verification:
orbiter init --yes

ls ~/.orbiter/integrations/
# Expected: git/  go/
ls ~/.orbiter/integrations/git/
# Expected: git.wasm  manifest.toml
ls ~/.orbiter/integrations/go/
# Expected: go.wasm  manifest.toml

# Verify survey reflects active state now that startup loads integrations:
orbiter survey git
# Expected: Status: active  (not "not installed")

orbiter survey go
# Expected: Status: active
```

- [ ] **Step 6: Commit**

```bash
git add internal/commands/root.go cmd/orbiter/main.go
git commit -m "feat: load installed integrations at startup via PersistentPreRunE"
```

---

## Self-Review

**Spec coverage:**

| Requirement | Task |
|-------------|------|
| Existing integrations (git, go) continue to work | Covered — all existing tests still pass |
| Bundled integrations install to `~/.orbiter/integrations/` | Task 1 (ExtractSelected) + Task 4 (orbiter init calls it) |
| Interactive selection during orbiter init | Task 3 (checklist) + Task 4 (TUI in vesselInitRun) |
| `--yes` flag for CI/non-interactive use | Task 4 (added to newInitCmd + newVesselInitCmd) |
| Idempotent orbiter init | Task 1 (ExtractSelected overwrites) + Task 4 (pre-checks installed state) |
| Version mismatch detection | Task 1 (CatalogEntriesWithState checksums) + Task 4 (badge + keeps checked) |
| Uninstall on deselect | Task 4 (ApplySelections removes dirs) |
| LoadInstalled picks up copy-pasted integrations | Task 2 (approve param) + Task 5 (startup call) |
| All lifecycle commands find integrations | Task 5 (PersistentPreRunE) |
| `orbiter survey <brand>` shows integration state | **Already implemented** (command taxonomy restructuring 2026-06-15) |
| No useless blank import | Task 5 (main.go cleanup) |

**Placeholder scan:** No TBDs, no "implement later", no "add error handling". All code blocks are complete.

**Type consistency:**
- `ChecklistItem` defined in Task 3 (`checklist.go`), used in Task 4 (`vessel.go`) — matches.
- `CatalogEntryState` defined in Task 1 (`bundle.go`), used in Task 4 — matches.
- `InstalledInfo` defined in Task 1, used only in `InstalledState` return — matches.
- `BuildChecklistItems` takes `[]bundle.CatalogEntryState`, returns `[]ChecklistItem` — consistent.
- `ApplySelections` takes `[]bundle.CatalogEntryState`, `[]ChecklistItem`, `string` — consistent.
- `LoadInstalled(dir string, registry *core.Registry, approve wasm.ApproveFunc)` — consistent with `InstallSelected` pattern.
