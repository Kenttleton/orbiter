package output

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleStepDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleStepActive  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleStepPending = lipgloss.NewStyle().Faint(true)
	styleStepFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
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

// ensure styleStepFailed is used to satisfy the compiler when not referenced directly.
var _ = styleStepFailed
