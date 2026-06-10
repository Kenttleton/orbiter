# Phase 1C: Output, Resolver, Commands & TUI Scaffold — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the output rendering layer (styled + JSON renderers, progress step list), the resolver middleware, and the Cobra command tree with DI wiring. Wire the TUI runner stub. Connect both `cmd/orbit/main.go` and `cmd/orbiter/main.go` so `orbit --help` and `orbiter` both run end-to-end.

**Architecture:** `Renderer` and `Resolver` are interfaces injected into commands at startup via `PersistentPreRunE`. Commands never touch the Star Chart or resolve aliases directly — they receive pre-built dependencies. The TUI runner executes `orbit` as a subprocess with `--output json` and parses the result. All stub commands print "not yet implemented" and return cleanly.

**Tech Stack:** `github.com/spf13/cobra`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles/spinner`

**Prerequisites:** Plans 1A and 1B must be complete.

> Replace `github.com/Kenttleton/orbiter` with your actual module path throughout.

---

## File Map

| File | Purpose |
|---|---|
| `internal/output/renderer.go` | `Renderer` interface, `PlanStep`, `ProgressHandle` types |
| `internal/output/json.go` | `JSONRenderer` implementation |
| `internal/output/styled.go` | `StyledRenderer` — Lipgloss + Terraform-inspired |
| `internal/output/progress.go` | `ProgressList` — fraction counter + thematic step list |
| `internal/output/factory.go` | `NewRenderer(format string) Renderer` factory |
| `internal/output/json_test.go` | Tests for JSONRenderer |
| `internal/output/styled_test.go` | Tests for StyledRenderer |
| `internal/output/progress_test.go` | Tests for ProgressList step transitions |
| `internal/resolver/resolver.go` | `Resolver` interface + `StarChartResolver` implementation |
| `internal/resolver/resolver_test.go` | Tests for resolver (name, ID, not-found) |
| `internal/commands/root.go` | Cobra root command + DI wiring via PersistentPreRunE |
| `internal/commands/stubs.go` | All Six Command + CRUD stubs |
| `internal/tui/runner.go` | `Runner` — executes orbit subprocess, parses JSON |
| `internal/tui/model.go` | Bubble Tea model stubs for universe and beacon views |
| `cmd/orbit/main.go` | Wired CLI entry point |
| `cmd/orbiter/main.go` | Wired TUI entry point |

---

### Task 1: Add output and TUI dependencies

**Files:**

- Modify: `go.mod`

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: Verify entries in go.mod**

```bash
grep -E 'cobra|lipgloss|bubbletea|bubbles' go.mod
```

Expected: four `require` entries.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add cobra, lipgloss, bubbletea, and bubbles dependencies"
```

---

### Task 2: Write Renderer interface and core types

**Files:**

- Create: `internal/output/renderer.go`

- [ ] **Step 1: Write the renderer.go interface and types**

Create `internal/output/renderer.go`:

```go
package output

// Format constants for the output renderer.
const (
    FormatStyled = "styled"
    FormatJSON   = "json"
)

// PlanStep represents one step in a Chart or transition plan.
type PlanStep struct {
    Action      string // "add", "change", "remove"
    EntityType  string
    Name        string
    Description string
}

// ProgressStep represents one task in a running operation's step list.
type ProgressStep struct {
    // ThematicLabel is the sci-fi themed label shown in default mode.
    ThematicLabel string
    // PlainLabel is the operational description always shown alongside the thematic label,
    // and shown exclusively in verbose mode.
    PlainLabel string
}

// ProgressList tracks progress through a known set of steps.
// Designed for Six Command operations where all steps are known upfront.
type ProgressList interface {
    // Start begins rendering the step list with the given operation title.
    Start(title string)
    // Advance marks the current step complete and moves to the next.
    Advance()
    // Finish marks all steps complete and stops rendering.
    Finish()
    // Fail marks the current step as failed and stops rendering.
    Fail(err error)
}

// Renderer is the dependency-injected output interface for all commands.
// Commands never select their own renderer — it is provided at startup.
type Renderer interface {
    // Info prints an informational message.
    Info(msg string)
    // Success prints a success message.
    Success(msg string)
    // Warning prints a warning message.
    Warning(msg string)
    // Error prints an error message.
    Error(msg string)
    // Plan renders a list of planned changes (used by the Chart command).
    Plan(steps []PlanStep)
    // Table renders a tabular view with headers and rows.
    Table(headers []string, rows [][]string)
    // Progress returns a ProgressList for tracking a multi-step operation.
    Progress(steps []ProgressStep) ProgressList
    // JSON encodes v as JSON and writes it to stdout.
    // Used for machine-readable output (TUI, 3rd party wrappers).
    JSON(v any) error
    // IsVerbose reports whether verbose mode is enabled.
    IsVerbose() bool
}
```

- [ ] **Step 2: No test yet** — Renderer is an interface; it's tested via its implementations. Move on.

---

### Task 3: Implement JSONRenderer (TDD)

**Files:**

- Create: `internal/output/json.go`
- Create: `internal/output/json_test.go`

- [ ] **Step 1: Write failing JSON tests**

Create `internal/output/json_test.go`:

```go
package output_test

import (
    "bytes"
    "encoding/json"
    "testing"

    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/stretchr/testify/require"
)

func TestJSONRendererEncodesValue(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewJSONRenderer(&buf, false)

    type payload struct {
        Name string `json:"name"`
        ID   int    `json:"id"`
    }
    err := r.JSON(payload{Name: "payment-api", ID: 42})
    require.NoError(t, err)

    var got payload
    require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
    require.Equal(t, "payment-api", got.Name)
    require.Equal(t, 42, got.ID)
}

func TestJSONRendererInfoWritesNothing(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewJSONRenderer(&buf, false)
    r.Info("this should not appear in JSON mode")
    require.Empty(t, buf.String())
}

func TestJSONRendererIsVerbose(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewJSONRenderer(&buf, true)
    require.True(t, r.IsVerbose())

    r2 := output.NewJSONRenderer(&buf, false)
    require.False(t, r2.IsVerbose())
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/output/... -run TestJSON 2>&1 | head -5
```

Expected: compile error — package doesn't exist yet.

- [ ] **Step 3: Implement json.go**

Create `internal/output/json.go`:

```go
package output

import (
    "encoding/json"
    "io"
)

// JSONRenderer produces machine-readable JSON output.
// Human-facing methods (Info, Success, Warning, Error, Plan, Table, Progress)
// are intentional no-ops — JSON mode is for wrappers, not humans.
type JSONRenderer struct {
    w       io.Writer
    verbose bool
}

// NewJSONRenderer returns a JSONRenderer that writes to w.
func NewJSONRenderer(w io.Writer, verbose bool) *JSONRenderer {
    return &JSONRenderer{w: w, verbose: verbose}
}

func (r *JSONRenderer) Info(_ string)    {}
func (r *JSONRenderer) Success(_ string) {}
func (r *JSONRenderer) Warning(_ string) {}
func (r *JSONRenderer) Error(_ string)   {}

func (r *JSONRenderer) Plan(_ []PlanStep) {}

func (r *JSONRenderer) Table(_ []string, _ [][]string) {}

func (r *JSONRenderer) Progress(steps []ProgressStep) ProgressList {
    return &noopProgressList{}
}

func (r *JSONRenderer) JSON(v any) error {
    enc := json.NewEncoder(r.w)
    enc.SetIndent("", "  ")
    return enc.Encode(v)
}

func (r *JSONRenderer) IsVerbose() bool { return r.verbose }

// noopProgressList is returned by JSONRenderer.Progress — no terminal output.
type noopProgressList struct{}

func (n *noopProgressList) Start(_ string) {}
func (n *noopProgressList) Advance()       {}
func (n *noopProgressList) Finish()        {}
func (n *noopProgressList) Fail(_ error)   {}
```

- [ ] **Step 4: Run JSON tests**

```bash
go test ./internal/output/... -run TestJSON
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/output/renderer.go internal/output/json.go internal/output/json_test.go
git commit -m "feat: add Renderer interface and JSONRenderer implementation"
```

---

### Task 4: Implement StyledRenderer (TDD)

**Files:**

- Create: `internal/output/styled.go`
- Create: `internal/output/styled_test.go`

- [ ] **Step 1: Write failing styled tests**

Create `internal/output/styled_test.go`:

```go
package output_test

import (
    "bytes"
    "strings"
    "testing"

    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/stretchr/testify/require"
)

func TestStyledRendererInfoContainsMessage(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Info("scanning universe")
    require.Contains(t, buf.String(), "scanning universe")
}

func TestStyledRendererSuccessContainsMessage(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Success("jump complete")
    require.Contains(t, buf.String(), "jump complete")
}

func TestStyledRendererWarningContainsMessage(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Warning("drift detected")
    require.Contains(t, buf.String(), "drift detected")
}

func TestStyledRendererErrorContainsMessage(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Error("transponder failure")
    require.Contains(t, buf.String(), "transponder failure")
}

func TestStyledRendererPlanContainsActions(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Plan([]output.PlanStep{
        {Action: "add", EntityType: "planet", Name: "payment-api", Description: "Clone repository"},
        {Action: "change", EntityType: "callsign", Name: "kent-acme", Description: "Activate callsign"},
        {Action: "remove", EntityType: "resource", Name: "node-18", Description: "Deactivate old runtime"},
    })
    out := buf.String()
    require.Contains(t, out, "payment-api")
    require.Contains(t, out, "kent-acme")
    require.Contains(t, out, "node-18")
}

func TestStyledRendererTableRendersRows(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    r.Table(
        []string{"Name", "Status", "Verified"},
        [][]string{
            {"payment-api", "healthy", "2026-06-09"},
            {"website", "drifted", "2026-06-08"},
        },
    )
    out := buf.String()
    require.Contains(t, out, "payment-api")
    require.Contains(t, out, "drifted")
}

func TestStyledRendererJSONWritesJSON(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)
    err := r.JSON(map[string]string{"key": "value"})
    require.NoError(t, err)
    require.Contains(t, buf.String(), `"key"`)
}

func TestStyledRendererIsVerbose(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, true)
    require.True(t, r.IsVerbose())

    r2 := output.NewStyledRenderer(&buf, false)
    require.False(t, r2.IsVerbose())
}

func TestStyledRendererVerboseShowsNoTheme(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, true)
    r.Info("verbose message")
    out := buf.String()
    require.Contains(t, out, "verbose message")
    require.False(t, strings.Contains(out, "★"), "verbose mode should not show thematic decoration")
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/output/... -run TestStyled 2>&1 | head -5
```

Expected: compile error — `NewStyledRenderer` not defined.

- [ ] **Step 3: Implement styled.go**

Create `internal/output/styled.go`:

```go
package output

import (
    "encoding/json"
    "fmt"
    "io"
    "strings"

    "github.com/charmbracelet/lipgloss"
)

var (
    styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)  // bright green
    styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)  // bright yellow
    styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)   // bright red
    styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))             // bright blue
    styleDim     = lipgloss.NewStyle().Faint(true)
    styleAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))             // green
    styleChange  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))             // yellow
    styleRemove  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))              // red
    styleBold    = lipgloss.NewStyle().Bold(true)
)

// StyledRenderer produces human-readable, Terraform-inspired terminal output
// using Lipgloss for colors and styling.
type StyledRenderer struct {
    w       io.Writer
    verbose bool
}

// NewStyledRenderer returns a StyledRenderer that writes to w.
func NewStyledRenderer(w io.Writer, verbose bool) *StyledRenderer {
    return &StyledRenderer{w: w, verbose: verbose}
}

func (r *StyledRenderer) Info(msg string) {
    fmt.Fprintln(r.w, styleInfo.Render("  "+msg))
}

func (r *StyledRenderer) Success(msg string) {
    fmt.Fprintln(r.w, styleSuccess.Render("✓ "+msg))
}

func (r *StyledRenderer) Warning(msg string) {
    fmt.Fprintln(r.w, styleWarning.Render("⚠ "+msg))
}

func (r *StyledRenderer) Error(msg string) {
    fmt.Fprintln(r.w, styleError.Render("✗ "+msg))
}

func (r *StyledRenderer) Plan(steps []PlanStep) {
    adds := 0
    changes := 0
    removes := 0

    for _, s := range steps {
        var prefix string
        var style lipgloss.Style
        switch s.Action {
        case "add":
            prefix = "  + "
            style = styleAdd
            adds++
        case "change":
            prefix = "  ~ "
            style = styleChange
            changes++
        case "remove":
            prefix = "  - "
            style = styleRemove
            removes++
        default:
            prefix = "    "
            style = styleInfo
        }
        line := fmt.Sprintf("%s%s %s", prefix, styleBold.Render(s.Name), styleDim.Render("("+s.EntityType+")"))
        fmt.Fprintln(r.w, style.Render(line))
        if s.Description != "" {
            fmt.Fprintln(r.w, styleDim.Render("      "+s.Description))
        }
    }

    fmt.Fprintln(r.w)
    summary := fmt.Sprintf("Plan: %d to add, %d to change, %d to remove.", adds, changes, removes)
    fmt.Fprintln(r.w, styleBold.Render(summary))
}

func (r *StyledRenderer) Table(headers []string, rows [][]string) {
    // Compute column widths.
    widths := make([]int, len(headers))
    for i, h := range headers {
        widths[i] = len(h)
    }
    for _, row := range rows {
        for i, cell := range row {
            if i < len(widths) && len(cell) > widths[i] {
                widths[i] = len(cell)
            }
        }
    }

    // Header row.
    var headerCells []string
    for i, h := range headers {
        headerCells = append(headerCells, styleBold.Render(fmt.Sprintf("%-*s", widths[i], h)))
    }
    fmt.Fprintln(r.w, strings.Join(headerCells, "  "))

    // Separator.
    var separators []string
    for _, w := range widths {
        separators = append(separators, strings.Repeat("─", w))
    }
    fmt.Fprintln(r.w, styleDim.Render(strings.Join(separators, "  ")))

    // Data rows.
    for _, row := range rows {
        var cells []string
        for i, cell := range row {
            if i < len(widths) {
                cells = append(cells, fmt.Sprintf("%-*s", widths[i], cell))
            }
        }
        fmt.Fprintln(r.w, strings.Join(cells, "  "))
    }
}

func (r *StyledRenderer) Progress(steps []ProgressStep) ProgressList {
    return &styledProgressList{
        w:       r.w,
        steps:   steps,
        verbose: r.verbose,
        current: 0,
    }
}

func (r *StyledRenderer) JSON(v any) error {
    enc := json.NewEncoder(r.w)
    enc.SetIndent("", "  ")
    return enc.Encode(v)
}

func (r *StyledRenderer) IsVerbose() bool { return r.verbose }
```

- [ ] **Step 4: Run styled tests**

```bash
go test ./internal/output/... -run TestStyled
```

Expected: all PASS. (Note: lipgloss color codes in test output are fine — we're testing for message content, not ANSI codes.)

- [ ] **Step 5: Commit**

```bash
git add internal/output/styled.go internal/output/styled_test.go
git commit -m "feat: implement StyledRenderer with Lipgloss-based Terraform-inspired output"
```

---

### Task 5: Implement ProgressList (TDD)

**Files:**

- Create: `internal/output/progress.go`
- Create: `internal/output/progress_test.go`

The `styledProgressList` renders a persistent step list with fraction counter, thematic labels, and plain operational subtitles. Verbose mode suppresses the thematic label and streams plain text only.

- [ ] **Step 1: Write failing progress tests**

Create `internal/output/progress_test.go`:

```go
package output_test

import (
    "bytes"
    "strings"
    "testing"

    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/stretchr/testify/require"
)

func TestProgressListStartAndFinish(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)

    steps := []output.ProgressStep{
        {ThematicLabel: "Plotting course...", PlainLabel: "Cloning acme/payment-api"},
        {ThematicLabel: "Sweeping sector...", PlainLabel: "Scanning payment-api"},
    }

    pl := r.Progress(steps)
    pl.Start("Executing maneuver...")
    pl.Advance()
    pl.Finish()

    out := buf.String()
    require.Contains(t, out, "Executing maneuver")
    require.Contains(t, out, "1/2")
}

func TestProgressListVerboseShowsPlainLabels(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, true)

    steps := []output.ProgressStep{
        {ThematicLabel: "Plotting course...", PlainLabel: "Cloning acme/payment-api"},
    }

    pl := r.Progress(steps)
    pl.Start("Maneuver")
    pl.Finish()

    out := buf.String()
    require.Contains(t, out, "Cloning acme/payment-api")
    require.False(t, strings.Contains(out, "Plotting course"), "verbose mode must suppress thematic labels")
}

func TestProgressListFailSetsError(t *testing.T) {
    var buf bytes.Buffer
    r := output.NewStyledRenderer(&buf, false)

    steps := []output.ProgressStep{
        {ThematicLabel: "Acquiring resource...", PlainLabel: "Installing node v20"},
    }

    pl := r.Progress(steps)
    pl.Start("Executing maneuver...")
    pl.Fail(fmt.Errorf("installation failed"))

    out := buf.String()
    require.Contains(t, out, "installation failed")
}
```

- [ ] **Step 2: Add missing import to test file**

The test file uses `fmt` — add it to the imports:

```go
import (
    "bytes"
    "fmt"
    "strings"
    "testing"

    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/stretchr/testify/require"
)
```

- [ ] **Step 3: Run to confirm failure**

```bash
go test ./internal/output/... -run TestProgress 2>&1 | head -5
```

Expected: compile error — `styledProgressList` not yet defined (referenced by `StyledRenderer.Progress`).

- [ ] **Step 4: Implement progress.go**

Create `internal/output/progress.go`:

```go
package output

import (
    "fmt"
    "io"

    "github.com/charmbracelet/lipgloss"
)

var (
    styleStepDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // green
    styleStepActive  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))  // blue
    styleStepPending = lipgloss.NewStyle().Faint(true)
    styleStepFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // red
)

// styledProgressList renders a persistent numbered step list.
type styledProgressList struct {
    w       io.Writer
    steps   []ProgressStep
    verbose bool
    current int
    title   string
}

func (p *styledProgressList) Start(title string) {
    p.title = title
    fmt.Fprintln(p.w, styleBold.Render(title))
    fmt.Fprintln(p.w)
    p.render()
}

func (p *styledProgressList) Advance() {
    if p.current < len(p.steps) {
        p.current++
    }
    p.render()
}

func (p *styledProgressList) Finish() {
    p.current = len(p.steps)
    p.render()
    fmt.Fprintln(p.w)
    fmt.Fprintln(p.w, styleSuccess.Render("✓ Complete"))
}

func (p *styledProgressList) Fail(err error) {
    p.render()
    fmt.Fprintln(p.w)
    fmt.Fprintln(p.w, styleError.Render("✗ Failed: "+err.Error()))
}

func (p *styledProgressList) render() {
    total := len(p.steps)
    for i, step := range p.steps {
        fraction := fmt.Sprintf("[%d/%d]", i+1, total)
        var label string
        if p.verbose {
            label = step.PlainLabel
        } else {
            label = step.ThematicLabel + "  " + styleStepPending.Render(step.PlainLabel)
        }

        var line string
        switch {
        case i < p.current:
            line = styleStepDone.Render(fmt.Sprintf("  %s ✓ %s", fraction, label))
        case i == p.current:
            line = styleStepActive.Render(fmt.Sprintf("  %s ⠸ %s", fraction, label))
        default:
            line = styleStepPending.Render(fmt.Sprintf("  %s   %s", fraction, label))
        }
        fmt.Fprintln(p.w, line)
    }
}
```

- [ ] **Step 5: Run progress tests**

```bash
go test ./internal/output/... -run TestProgress
```

Expected: all PASS.

- [ ] **Step 6: Write and run the factory**

Create `internal/output/factory.go`:

```go
package output

import "os"

// NewRenderer returns the appropriate Renderer for the given format string.
// format must be FormatStyled or FormatJSON. Defaults to FormatStyled.
func NewRenderer(format string, verbose bool) Renderer {
    switch format {
    case FormatJSON:
        return NewJSONRenderer(os.Stdout, verbose)
    default:
        return NewStyledRenderer(os.Stdout, verbose)
    }
}
```

- [ ] **Step 7: Run all output tests**

```bash
go test ./internal/output/...
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/output/progress.go internal/output/progress_test.go \
        internal/output/factory.go
git commit -m "feat: implement ProgressList with thematic labels, verbose mode, and fraction counter"
```

---

### Task 6: Implement Resolver (TDD)

**Files:**

- Create: `internal/resolver/resolver.go`
- Create: `internal/resolver/resolver_test.go`

The `Resolver` is the alias → ID middleware layer. It wraps `StarChart.Resolve` behind an interface so commands can be tested without a real database.

- [ ] **Step 1: Write failing resolver tests**

Create `internal/resolver/resolver_test.go`:

```go
package resolver_test

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/resolver"
    "github.com/Kenttleton/orbiter/internal/starchart"
    "github.com/stretchr/testify/require"
)

func testSC(t *testing.T) *starchart.StarChart {
    t.Helper()
    sc, err := starchart.Open(filepath.Join(t.TempDir(), "test.db"))
    require.NoError(t, err)
    t.Cleanup(func() { sc.Close() })
    return sc
}

func seedAlias(t *testing.T, sc *starchart.StarChart, id, name, entityType string) {
    t.Helper()
    err := sc.Insert(context.Background(), "aliases", models.Alias{
        ID: id, Name: name, EntityType: entityType,
        CreatedAt: time.Now().UTC(),
    })
    require.NoError(t, err)
}

func TestResolverByName(t *testing.T) {
    ctx := context.Background()
    sc := testSC(t)
    id := models.NewID(models.EntityTypePlanet)
    seedAlias(t, sc, id, "payment-api", models.EntityTypePlanet)

    r := resolver.New(sc)
    alias, err := r.Resolve(ctx, "payment-api")
    require.NoError(t, err)
    require.Equal(t, id, alias.ID)
    require.Equal(t, models.EntityTypePlanet, alias.EntityType)
}

func TestResolverByID(t *testing.T) {
    ctx := context.Background()
    sc := testSC(t)
    id := models.NewID(models.EntityTypePlanet)
    seedAlias(t, sc, id, id, models.EntityTypePlanet)

    r := resolver.New(sc)
    alias, err := r.Resolve(ctx, id)
    require.NoError(t, err)
    require.Equal(t, id, alias.ID)
}

func TestResolverNotFound(t *testing.T) {
    ctx := context.Background()
    sc := testSC(t)

    r := resolver.New(sc)
    _, err := r.Resolve(ctx, "nonexistent")
    require.Error(t, err)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/resolver/... 2>&1 | head -5
```

Expected: compile error — package doesn't exist.

- [ ] **Step 3: Implement resolver.go**

Create `internal/resolver/resolver.go`:

```go
package resolver

import (
    "context"

    "github.com/Kenttleton/orbiter/internal/models"
    "github.com/Kenttleton/orbiter/internal/starchart"
)

// Resolver resolves a human-readable name or ID to a fully populated Alias.
// It is dependency-injected into all commands — commands never query the
// Star Chart for aliases directly.
type Resolver interface {
    Resolve(ctx context.Context, input string) (models.Alias, error)
}

// starChartResolver wraps StarChart.Resolve behind the Resolver interface.
type starChartResolver struct {
    sc *starchart.StarChart
}

// New returns a Resolver backed by the given StarChart.
func New(sc *starchart.StarChart) Resolver {
    return &starChartResolver{sc: sc}
}

func (r *starChartResolver) Resolve(ctx context.Context, input string) (models.Alias, error) {
    return r.sc.Resolve(ctx, input)
}
```

- [ ] **Step 4: Run resolver tests**

```bash
go test ./internal/resolver/...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/resolver.go internal/resolver/resolver_test.go
git commit -m "feat: implement Resolver middleware wrapping starchart alias lookup"
```

---

### Task 7: Implement Cobra root command with DI wiring

**Files:**

- Create: `internal/commands/root.go`

The root command opens the Star Chart, builds the Renderer and Resolver, and injects them via `PersistentPreRunE`. All subcommands receive dependencies through their `RunE` closure rather than global state.

- [ ] **Step 1: Write root.go**

Create `internal/commands/root.go`:

```go
package commands

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/Kenttleton/orbiter/internal/output"
    "github.com/Kenttleton/orbiter/internal/resolver"
    "github.com/Kenttleton/orbiter/internal/starchart"
)

// deps holds the dependency-injected resources available to all commands.
type deps struct {
    sc       *starchart.StarChart
    renderer output.Renderer
    resolver resolver.Resolver
}

// NewRootCommand builds and returns the orbit root Cobra command with DI wiring.
func NewRootCommand() *cobra.Command {
    var outputFormat string
    var verbose bool

    var d deps

    root := &cobra.Command{
        Use:   "orbit",
        Short: "Orbiter CLI — navigate and orchestrate your development universe",
        Long: `orbit is the command interface for Orbiter, a state-driven navigation
and environment orchestration platform for freelance and contract engineers.`,
        SilenceUsage:  true,
        SilenceErrors: true,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Resolve Star Chart path.
            chartPath := os.Getenv("ORBIT_STARCHART")
            if chartPath == "" {
                home, err := os.UserHomeDir()
                if err != nil {
                    return fmt.Errorf("resolve home directory: %w", err)
                }
                chartPath = home + "/.orbiter/starchart.db"
            }

            // Open Star Chart.
            sc, err := starchart.Open(chartPath)
            if err != nil {
                return fmt.Errorf("open star chart: %w", err)
            }
            d.sc = sc

            // Resolve output format: flag > env > default.
            format := outputFormat
            if format == "" {
                format = os.Getenv("ORBIT_OUTPUT")
            }
            if format == "" {
                format = output.FormatStyled
            }

            // Resolve verbose: flag > env.
            if !verbose {
                verbose = os.Getenv("ORBIT_VERBOSE") == "1"
            }

            d.renderer = output.NewRenderer(format, verbose)
            d.resolver = resolver.New(sc)
            return nil
        },
    }

    root.PersistentFlags().StringVar(&outputFormat, "output", "", "output format: styled (default) or json")
    root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output: plain labels and full tool output")

    // Register all subcommands.
    root.AddCommand(
        newSurveyCmd(&d),
        newChartCmd(&d),
        newJumpCmd(&d),
        newScanCmd(&d),
        newCalibrateCmd(&d),
        newRetroCmd(&d),
        newGalaxyCmd(&d),
        newSystemCmd(&d),
        newPlanetCmd(&d),
        newCallsignCmd(&d),
        newTransponderCmd(&d),
        newResourceCmd(&d),
        newVesselCmd(&d),
    )

    return root
}
```

- [ ] **Step 2: No test for root.go yet** — it will be tested end-to-end in Task 9. Move on.

---

### Task 8: Write all stub subcommands

**Files:**

- Create: `internal/commands/stubs.go`

All Six Commands and CRUD commands are stubbed here. Each prints a "not yet implemented" message using the renderer and returns `nil`. They will be replaced in Plans 2 and 3.

- [ ] **Step 1: Write stubs.go**

Create `internal/commands/stubs.go`:

```go
package commands

import (
    "github.com/spf13/cobra"
)

// --- Six Commands ---

func newSurveyCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "survey [target]",
        Short: "Inspect metadata — \"What is this thing?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("survey: not yet implemented")
            return nil
        },
    }
}

func newChartCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "chart [target]",
        Short: "Preview a transition — \"What would happen if I went there?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("chart: not yet implemented")
            return nil
        },
    }
}

func newJumpCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "jump [target]",
        Short: "Execute a transition — \"Take me there.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("jump: not yet implemented")
            return nil
        },
    }
}

func newScanCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "scan [target]",
        Short: "Verify reality — \"What does reality currently look like?\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("scan: not yet implemented")
            return nil
        },
    }
}

func newCalibrateCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "calibrate [target]",
        Short: "Reconcile drift — \"Bring reality and the Star Chart back into alignment.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("calibrate: not yet implemented")
            return nil
        },
    }
}

func newRetroCmd(d *deps) *cobra.Command {
    return &cobra.Command{
        Use:   "retro [target]",
        Short: "Retire obsolete entities — \"Remove what no longer belongs in the universe.\"",
        RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("retro: not yet implemented")
            return nil
        },
    }
}

// --- CRUD Commands ---

func newGalaxyCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "galaxy",
        Short: "Manage galaxies (organizations/clients)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("galaxy add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("galaxy edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a galaxy", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("galaxy remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newSystemCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "system",
        Short: "Manage solar systems (team subdivisions)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a solar system", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("system add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a solar system", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("system edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a solar system", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("system remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newPlanetCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "planet",
        Short: "Manage planets (projects)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a planet", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("planet add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "init", Short: "Initialize a planet from the current directory", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("planet init: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a planet", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("planet edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a planet", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("planet remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newCallsignCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "callsign",
        Short: "Manage callsigns (identities)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a callsign", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("callsign add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a callsign", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("callsign edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a callsign", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("callsign remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newTransponderCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "transponder",
        Short: "Manage transponders (credential pointers)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a transponder", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("transponder add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a transponder", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("transponder edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a transponder", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("transponder remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newResourceCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "resource",
        Short: "Manage resources (tooling, runtimes, capabilities)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a resource", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("resource add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a resource", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("resource edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a resource", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("resource remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newVesselCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "vessel",
        Short: "Manage the vessel (this workstation)",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "survey", Short: "Show vessel configuration", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel survey: not yet implemented")
            return nil
        }},
        newVesselDefaultsCmd(d),
        newVesselHistoryCmd(d),
    )
    return cmd
}

func newVesselDefaultsCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "defaults",
        Short: "Manage vessel-level defaults",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "add", Short: "Add a default", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel defaults add: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "edit", Short: "Edit a default", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel defaults edit: not yet implemented")
            return nil
        }},
        &cobra.Command{Use: "remove", Short: "Remove a default", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel defaults remove: not yet implemented")
            return nil
        }},
    )
    return cmd
}

func newVesselHistoryCmd(d *deps) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "history",
        Short: "Manage navigation history",
    }
    cmd.AddCommand(
        &cobra.Command{Use: "clean", Short: "Remove history older than retention period", RunE: func(cmd *cobra.Command, args []string) error {
            d.renderer.Info("vessel history clean: not yet implemented")
            return nil
        }},
    )
    return cmd
}
```

- [ ] **Step 2: No separate test** — stubs are exercised in the end-to-end test. Move on.

---

### Task 9: Wire cmd/orbit/main.go

**Files:**

- Modify: `cmd/orbit/main.go`

- [ ] **Step 1: Write the wired entry point**

Replace `cmd/orbit/main.go` with:

```go
package main

import (
    "fmt"
    "os"

    "github.com/Kenttleton/orbiter/internal/commands"
)

func main() {
    root := commands.NewRootCommand()
    if err := root.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

- [ ] **Step 2: Build and run orbit --help**

```bash
just build-orbit
./bin/orbit --help
```

Expected output includes:

```text
orbit is the command interface for Orbiter...

Usage:
  orbit [command]

Available Commands:
  calibrate   Reconcile drift...
  callsign    Manage callsigns...
  chart       Preview a transition...
  galaxy      Manage galaxies...
  jump        Execute a transition...
  planet      Manage planets...
  resource    Manage resources...
  retro       Retire obsolete entities...
  scan        Verify reality...
  survey      Inspect metadata...
  system      Manage solar systems...
  transponder Manage transponders...
  vessel      Manage the vessel...
```

- [ ] **Step 3: Smoke-test a stub command**

```bash
ORBIT_STARCHART=/tmp/test-orbit.db ./bin/orbit survey
```

Expected: `survey: not yet implemented` (styled output).

```bash
ORBIT_STARCHART=/tmp/test-orbit.db ./bin/orbit --output json survey
```

Expected: no output (JSON renderer suppresses non-JSON messages).

- [ ] **Step 4: Commit**

```bash
git add cmd/orbit/main.go internal/commands/
git commit -m "feat: wire orbit CLI with Cobra, DI renderer/resolver, and stub commands"
```

---

### Task 10: Write TUI runner and model stubs

**Files:**

- Create: `internal/tui/runner.go`
- Create: `internal/tui/model.go`

- [ ] **Step 1: Write the orbit subprocess runner**

Create `internal/tui/runner.go`:

```go
package tui

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
)

// Runner executes orbit subprocesses and parses their JSON output.
// orbiter uses this to drive all Star Chart operations without touching
// the database directly.
type Runner struct {
    orbitPath string // path to the orbit binary; defaults to "orbit" (from PATH)
}

// NewRunner returns a Runner that invokes the orbit binary.
// If orbitPath is empty, "orbit" is resolved from PATH.
func NewRunner(orbitPath string) *Runner {
    if orbitPath == "" {
        orbitPath = "orbit"
    }
    return &Runner{orbitPath: orbitPath}
}

// Run executes an orbit command with --output json and returns the raw JSON bytes.
// args should be the orbit subcommand and its arguments, e.g. ["survey", "payment-api"].
func (r *Runner) Run(ctx context.Context, args ...string) (json.RawMessage, error) {
    cmdArgs := append([]string{"--output", "json"}, args...)
    cmd := exec.CommandContext(ctx, r.orbitPath, cmdArgs...)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("orbit %v: %w\nstderr: %s", args, err, stderr.String())
    }

    return json.RawMessage(stdout.Bytes()), nil
}
```

- [ ] **Step 2: Write the Bubble Tea model stubs**

Create `internal/tui/model.go`:

```go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"
)

// UniverseModel is the Bubble Tea model for the universe view.
// Phase 1: stub — displays a placeholder message.
// Phase 5: will show the full entity tree from the Star Chart.
type UniverseModel struct{}

func NewUniverseModel() UniverseModel {
    return UniverseModel{}
}

func (m UniverseModel) Init() tea.Cmd {
    return nil
}

func (m UniverseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case tea.KeyMsg:
        return m, tea.Quit
    }
    return m, nil
}

func (m UniverseModel) View() string {
    return "orbiter universe view — not yet implemented\n\nPress any key to exit."
}

// BeaconModel is the Bubble Tea model for the beacon view.
// Phase 1: stub — displays a placeholder message.
// Phase 5: will show live Beacon status for all entities.
type BeaconModel struct{}

func NewBeaconModel() BeaconModel {
    return BeaconModel{}
}

func (m BeaconModel) Init() tea.Cmd {
    return nil
}

func (m BeaconModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case tea.KeyMsg:
        return m, tea.Quit
    }
    return m, nil
}

func (m BeaconModel) View() string {
    return "orbiter beacon view — not yet implemented\n\nPress any key to exit."
}
```

- [ ] **Step 3: Wire cmd/orbiter/main.go**

Replace `cmd/orbiter/main.go` with:

```go
package main

import (
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/Kenttleton/orbiter/internal/tui"
)

func main() {
    // Verify orbit is available in PATH before launching the TUI.
    runner := tui.NewRunner("")
    if _, err := runner.Run(nil, "version"); err != nil {
        // orbit not found or not working — warn but still launch the TUI stub.
        fmt.Fprintf(os.Stderr, "warning: orbit not found in PATH — some features may be unavailable\n")
    }

    model := tui.NewUniverseModel()
    p := tea.NewProgram(model, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintln(os.Stderr, "orbiter:", err)
        os.Exit(1)
    }
}
```

- [ ] **Step 4: Fix nil context in orbiter main.go**

The `runner.Run(nil, "version")` call passes nil as the context. Replace with `context.Background()`:

```go
package main

import (
    "context"
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/Kenttleton/orbiter/internal/tui"
)

func main() {
    runner := tui.NewRunner("")
    if _, err := runner.Run(context.Background(), "version"); err != nil {
        fmt.Fprintf(os.Stderr, "warning: orbit not found in PATH — TUI read operations will fail\n")
    }

    model := tui.NewUniverseModel()
    p := tea.NewProgram(model, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintln(os.Stderr, "orbiter:", err)
        os.Exit(1)
    }
}
```

- [ ] **Step 5: Build and smoke-test both binaries**

```bash
just build
./bin/orbit --help
ORBIT_STARCHART=/tmp/smoke.db ./bin/orbit survey
```

Expected: help text and "survey: not yet implemented".

```bash
./bin/orbiter
```

Expected: launches an alt-screen TUI showing the universe stub message. Press any key to exit.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/ cmd/orbiter/main.go
git commit -m "feat: wire orbiter TUI with Bubble Tea stubs and orbit subprocess runner"
```

---

### Task 11: Final verification

- [ ] **Step 1: Run the full test suite**

```bash
just test
```

Expected: all packages pass — `migrations`, `models`, `starchart`, `resolver`, `output`.

- [ ] **Step 2: Build both binaries**

```bash
just build
```

Expected: no errors.

- [ ] **Step 3: Verify cross-compilation to Linux**

```bash
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/orbit
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/orbiter
```

Expected: no errors. Confirms no CGo or platform-specific dependencies snuck in.

- [ ] **Step 4: Verify cross-compilation to Windows**

```bash
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/orbit
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/orbiter
```

Expected: no errors.

- [ ] **Step 5: Tidy module**

```bash
go mod tidy
git diff go.mod go.sum
```

If there are changes:

```bash
git add go.mod go.sum
git commit -m "chore: tidy go module after output/resolver/commands layer"
```

- [ ] **Step 6: Final commit tag**

Phase 1 scaffold is complete. Both binaries compile, the schema is applied, CRUD + Tx + Resolve are tested, the command tree is wired with DI, and cross-compilation to all platforms is verified.
