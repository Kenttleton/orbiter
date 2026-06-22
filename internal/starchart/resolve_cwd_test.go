package starchart_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Kenttleton/orbiter/internal/starchart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shellConfig(t *testing.T, path string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"path": path})
	require.NoError(t, err)
	return string(b)
}

func TestResolveCWD_ExactMatch(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	_, _ = sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme"))
	_, _ = sc.Attach(ctx, "acme-path", "acme")

	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	_, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme/payment-api"))
	_, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

	alias, err := sc.ResolveCWD(ctx, "/home/kent/acme")
	require.NoError(t, err)
	assert.Equal(t, g.ID, alias.ID)
	_ = p
}

func TestResolveCWD_LongestPrefix(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	_, _ = sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme"))
	_, _ = sc.Attach(ctx, "acme-path", "acme")

	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	_, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme/payment-api"))
	_, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

	alias, err := sc.ResolveCWD(ctx, "/home/kent/acme/payment-api/src/handlers")
	require.NoError(t, err)
	assert.Equal(t, p.ID, alias.ID)
}

func TestResolveCWD_NoMatch(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	_, err := sc.ResolveCWD(ctx, "/home/kent/other-project")
	assert.ErrorIs(t, err, starchart.ErrNotFound)
}

func TestResolveCWD_ExactMatchBeatsLongerPrefix(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	_, _ = sc.CreateResource(ctx, "acme-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme"))
	_, _ = sc.Attach(ctx, "acme-path", "acme")

	_, _ = sc.CreatePlanet(ctx, "payment-api", g.ID, "")
	_, _ = sc.CreateResource(ctx, "payment-api-path", "filesystem", "orbiter", "[]", shellConfig(t, "/home/kent/acme/payment-api"))
	_, _ = sc.Attach(ctx, "payment-api-path", "payment-api")

	alias, err := sc.ResolveCWD(ctx, "/home/kent/acme")
	require.NoError(t, err)
	assert.Equal(t, g.ID, alias.ID, "exact match on galaxy beats planet prefix")
}
