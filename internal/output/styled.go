package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	w     io.Writer
	trace bool
	stdin io.Reader
}

// NewStyledRenderer returns a StyledRenderer that writes to w.
func NewStyledRenderer(w io.Writer, trace bool) *StyledRenderer {
	return &StyledRenderer{w: w, trace: trace, stdin: os.Stdin}
}

// NewStyledRendererWith creates a StyledRenderer with an explicit stdin reader.
// Use in tests to inject a strings.NewReader; production callers use NewStyledRenderer.
func NewStyledRendererWith(w io.Writer, r io.Reader, trace bool) *StyledRenderer {
	return &StyledRenderer{w: w, trace: trace, stdin: r}
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
	if len(steps) == 0 {
		return
	}
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
		trace:   r.trace,
		current: 0,
	}
}

func (r *StyledRenderer) JSON(v any) error {
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Result implements Renderer. Styled mode has already rendered its own output.
func (r *StyledRenderer) Result(_ Result) {}

func (r *StyledRenderer) IsTrace() bool { return r.trace }

// Prompt implements Renderer. Renders the prompt to the terminal and reads the
// Captain's response from stdin. Returns the response serialized as a JSON string.
func (sr *StyledRenderer) Prompt(p CaptainPrompt) (string, error) {
	out := sr.promptWriter()
	switch p.Op {
	case "CONFIRM":
		fmt.Fprintf(out, "\n  %s\n  Allow? [y/N] ", p.Message)
		var ans string
		fmt.Fscan(sr.stdin, &ans)
		confirmed := strings.ToLower(strings.TrimSpace(ans)) == "y"
		b, _ := json.Marshal(map[string]bool{"confirmed": confirmed})
		return string(b), nil

	case "SELECT":
		fmt.Fprintf(out, "\n  %s\n", p.Message)
		for i, opt := range p.Options {
			fmt.Fprintf(out, "    %d. %s\n", i+1, opt)
		}
		if p.Variant == "multi" {
			fmt.Fprint(out, "  Enter numbers separated by spaces: ")
			var line string
			fmt.Fscan(sr.stdin, &line)
			parts := strings.Fields(line)
			var selected []string
			for _, part := range parts {
				idx := 0
				fmt.Sscan(part, &idx)
				if idx >= 1 && idx <= len(p.Options) {
					selected = append(selected, p.Options[idx-1])
				}
			}
			b, _ := json.Marshal(map[string][]string{"selected": selected})
			return string(b), nil
		}
		// single
		fmt.Fprint(out, "  Enter number: ")
		var idx int
		fmt.Fscan(sr.stdin, &idx)
		if idx < 1 || idx > len(p.Options) {
			return "", fmt.Errorf("selection %d out of range [1,%d]", idx, len(p.Options))
		}
		b, _ := json.Marshal(map[string]string{"selected": p.Options[idx-1]})
		return string(b), nil

	case "TEXT":
		if p.Variant == "obscured" {
			fmt.Fprintf(out, "\n  %s (input hidden): ", p.Message)
		} else {
			fmt.Fprintf(out, "\n  %s: ", p.Message)
		}
		scanner := bufio.NewScanner(sr.stdin)
		scanner.Scan()
		value := scanner.Text()
		b, _ := json.Marshal(map[string]string{"value": value})
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown prompt op: %q", p.Op)
	}
}

// promptWriter returns where prompt text goes. The shell hook captures orbiter's
// stdout to read directives, so on a real terminal (writing to os.Stdout) a prompt
// there would be invisible while orbiter waits for an answer: send it to stderr.
func (sr *StyledRenderer) promptWriter() io.Writer {
	if f, ok := sr.w.(*os.File); ok && f == os.Stdout {
		return os.Stderr
	}
	return sr.w
}
