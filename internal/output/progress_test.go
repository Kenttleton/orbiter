package output_test

import (
	"bytes"
	"fmt"
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
