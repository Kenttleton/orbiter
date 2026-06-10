package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleDim     = lipgloss.NewStyle().Faint(true)
	styleAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleChange  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleRemove  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
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

	var headerCells []string
	for i, h := range headers {
		headerCells = append(headerCells, styleBold.Render(fmt.Sprintf("%-*s", widths[i], h)))
	}
	fmt.Fprintln(r.w, strings.Join(headerCells, "  "))

	var separators []string
	for _, w := range widths {
		separators = append(separators, strings.Repeat("─", w))
	}
	fmt.Fprintln(r.w, styleDim.Render(strings.Join(separators, "  ")))

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
