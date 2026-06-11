package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Runner executes orbiter subprocesses and parses their JSON output.
// orbiter starchart uses this to drive all Star Chart operations without touching
// the database directly.
type Runner struct {
	orbitPath string
}

// NewRunner returns a Runner that invokes the orbiter binary.
// If orbitPath is empty, "orbiter" is resolved from PATH.
func NewRunner(orbitPath string) *Runner {
	if orbitPath == "" {
		orbitPath = "orbiter"
	}
	return &Runner{orbitPath: orbitPath}
}

// Run executes an orbiter command with --output json and returns the raw JSON bytes.
// args should be the orbiter subcommand and its arguments, e.g. ["survey", "payment-api"].
func (r *Runner) Run(ctx context.Context, args ...string) (json.RawMessage, error) {
	cmdArgs := append([]string{"--output", "json"}, args...)
	cmd := exec.CommandContext(ctx, r.orbitPath, cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("orbiter %v: %w\nstderr: %s", args, err, stderr.String())
	}

	return json.RawMessage(stdout.Bytes()), nil
}
