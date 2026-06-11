package starchart_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeveledBranchCrawl_PlanetLevel(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

	_, _ = sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	_, _ = sc.Attach(ctx, "node", "payment-api")

	cs, _ := sc.CreateCallsign(ctx, "kent-acme")
	_, _ = sc.CreateTransponder(ctx, "acme-gh", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
	_, _ = sc.Attach(ctx, "acme-gh", "kent-acme")
	_, _ = sc.Attach(ctx, "kent-acme", "payment-api")

	lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
	require.NoError(t, err)

	require.Len(t, lb.Levels, 1)
	assert.Len(t, lb.Levels[0].Resources, 1)
	assert.Equal(t, "runtime", lb.Levels[0].Resources[0].Role)
	require.NotNil(t, lb.Levels[0].Callsign)
	assert.Equal(t, cs.ID, lb.Levels[0].Callsign.ID)
	assert.Len(t, lb.Levels[0].Transponders, 1)
	assert.Equal(t, "file", lb.Levels[0].Transponders[0].Role)
}

func TestLeveledBranchCrawl_SkipsEmptyLevels(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

	_, _ = sc.CreateCallsign(ctx, "kent-acme-galaxy")
	_, _ = sc.CreateTransponder(ctx, "acme-gh-galaxy", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
	_, _ = sc.Attach(ctx, "acme-gh-galaxy", "kent-acme-galaxy")
	_, _ = sc.Attach(ctx, "kent-acme-galaxy", "acme")

	_, _ = sc.CreateResource(ctx, "node", "runtime", "node", "[]", "{}")
	_, _ = sc.Attach(ctx, "node", "payment-api")

	lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
	require.NoError(t, err)

	require.Len(t, lb.Levels, 1)
	assert.Equal(t, p.ID, lb.Levels[0].EntityID)
	assert.Nil(t, lb.Levels[0].Callsign)
	assert.Empty(t, lb.Levels[0].Transponders)
}

func TestLeveledBranchCrawl_TwoLevels(t *testing.T) {
	sc := testDB(t)
	ctx := context.Background()

	g, _ := sc.CreateGalaxy(ctx, "acme")
	p, _ := sc.CreatePlanet(ctx, "payment-api", g.ID, "")

	_, _ = sc.CreateResource(ctx, "nvm", "manager", "nvm", "[]", "{}")
	_, _ = sc.Attach(ctx, "nvm", "acme")
	csGalaxy, _ := sc.CreateCallsign(ctx, "kent-acme-galaxy")
	_, _ = sc.CreateTransponder(ctx, "acme-npm-token", "env", "npm", "NPM_TOKEN")
	_, _ = sc.Attach(ctx, "acme-npm-token", "kent-acme-galaxy")
	_, _ = sc.Attach(ctx, "kent-acme-galaxy", "acme")

	_, _ = sc.CreateResource(ctx, "github-remote", "remote", "github", "[]", "{}")
	_, _ = sc.Attach(ctx, "github-remote", "payment-api")
	csPlanet, _ := sc.CreateCallsign(ctx, "kent-acme-planet")
	_, _ = sc.CreateTransponder(ctx, "acme-gh-key", "file", "github", "/home/kent/.ssh/id_ed25519_acme")
	_, _ = sc.Attach(ctx, "acme-gh-key", "kent-acme-planet")
	_, _ = sc.Attach(ctx, "kent-acme-planet", "payment-api")

	lb, err := sc.LeveledBranchCrawl(ctx, p.ID)
	require.NoError(t, err)

	require.Len(t, lb.Levels, 2)
	assert.Equal(t, p.ID, lb.Levels[0].EntityID, "planet level first (FILO)")
	assert.Equal(t, csPlanet.ID, lb.Levels[0].Callsign.ID)
	assert.Equal(t, g.ID, lb.Levels[1].EntityID, "galaxy level second")
	assert.Equal(t, csGalaxy.ID, lb.Levels[1].Callsign.ID)
}
