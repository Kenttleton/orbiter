package commands_test

import (
	"context"
	"os"
	"testing"

	"github.com/Kenttleton/orbiter/internal/commands"
	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/require"
)

func openTestExecutor(t *testing.T) *commands.Executor {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "starchart-*.db")
	require.NoError(t, err)
	f.Close()
	sc, err := starchart.Open(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { sc.Close() })
	r := output.NewRenderer(output.FormatStyled, false)
	return commands.NewExecutor(sc, r)
}

func TestExecutor_Survey_NoTarget(t *testing.T) {
	exec := openTestExecutor(t)
	err := exec.Survey(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no target found")
}

func TestExecutor_Survey_WithTarget(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()
	_, err := exec.SC().CreateGalaxy(ctx, "acme")
	require.NoError(t, err)

	err = exec.Survey(ctx, "acme")
	require.NoError(t, err)
}

func TestExecutor_Scan_NoIntegration(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")
	_, _ = exec.SC().CreateResource(ctx, "go", "runtime", "go", "[]", "{}")
	_, _ = exec.SC().Attach(ctx, "go", "payment-api")

	err := exec.Scan(ctx, "payment-api")
	require.NoError(t, err)
}

func TestExecutor_Chart(t *testing.T) {
	exec := openTestExecutor(t)
	ctx := context.Background()

	g, _ := exec.SC().CreateGalaxy(ctx, "acme")
	_, _ = exec.SC().CreatePlanet(ctx, "payment-api", g.ID, "")

	err := exec.Chart(ctx, "payment-api")
	require.NoError(t, err)
}
